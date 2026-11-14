// Package graphingest query handlers
package graphingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/cache"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// defaultMaxConcurrent is the default bounded concurrency for entity fetches
const defaultMaxConcurrent = 10

// queryHandlers maps each verb graph-ingest answers, by its name in graph.QueryVerbs, to
// its handler.
func (c *Component) queryHandlers() map[string]func(context.Context, []byte) ([]byte, error) {
	return map[string]func(context.Context, []byte) ([]byte, error){
		"entity": c.handleQueryEntityNATS,
		"batch":  c.handleQueryBatchNATS,
		"prefix": c.handleQueryPrefixNATS,
		// Served once the table declares its row; setupQueryHandlers subscribes by row.
		"prefixIDs": c.handleQueryPrefixIDsNATS,
	}
}

// setupQueryHandlers subscribes to the subject of every verb the table in graph declares
// for graph-ingest, and to nothing else. A declared verb with no handler
// here fails Start: serving fewer verbs than the table declares would leave callers
// with no responder for a subject they were told is served.
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	handlers := c.queryHandlers()
	var subjects []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphIngest {
			continue
		}
		handler, ok := handlers[verb.Name]
		if !ok {
			return fmt.Errorf("query verb %q (%s) is declared for graph-ingest but has no handler", verb.Name, verb.Subject)
		}
		sub, err := c.subscribeForRequests(ctx, verb.Subject, handler)
		if err != nil {
			return fmt.Errorf("subscribe %s query: %w", verb.Name, err)
		}
		c.subscriptions = append(c.subscriptions, sub)
		subjects = append(subjects, verb.Subject)
	}
	c.logger.Info("query handlers registered", "subjects", subjects)
	return nil
}

// handleQueryEntityNATS handles single entity query requests via NATS request/reply
func (c *Component) handleQueryEntityNATS(ctx context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Parse request
	var req graph.EntityRequest
	if err := json.Unmarshal(data, &req); err != nil {
		// SemStreams ADR-060: invalid request → invalid class + Code invalid_request, so a
		// consumer reads ce.Code instead of sniffing the message.
		return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest,
			fmt.Errorf("invalid request: %w", err))
	}

	// Validate before any authority I/O. Exact reads never reinterpret malformed
	// identity as absence.
	if err := semtypes.ValidateEntityID(req.ID); err != nil {
		return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest,
			fmt.Errorf("invalid request: entity ID: %w", err))
	}

	entity, revision, err := c.readEntity(ctx, req.ID)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			// HTTP semantics (400 vs 404) live at the gateway. SemStreams ADR-060: not-found
			// classifies as Invalid at the wire boundary and carries the stable
			// Code entity_not_found, so a gateway routes 404 off ce.Code rather
			// than substring-sniffing the message.
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound,
				fmt.Errorf("not found: %s", req.ID))
		}
		if graph.IsStateContractError(err) {
			return nil, err // already the stored-value rule's one classification
		}
		return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal,
			fmt.Errorf("internal error: %w", err))
	}
	return json.Marshal(graph.ExactEntity{
		Entity:     entity.Clone(),
		KVRevision: revision,
	})
}

// handleQueryBatchNATS handles batch entity query requests via NATS request/reply
func (c *Component) handleQueryBatchNATS(ctx context.Context, data []byte) ([]byte, error) {
	// Create context with timeout for KV operations
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Parse request
	var req graph.EntityBatchRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, rejectInvalid(graph.ErrorCodeInvalidRequest, fmt.Errorf("invalid request: %w", err))
	}
	if len(req.IDs) == 0 {
		return json.Marshal(graph.EntityBatchResponse{})
	}

	// Fetch entities with bounded concurrency and cache.
	//
	// Timed because this handler's 10 s budget and its caller's deadline are set
	// independently: a caller that gives up first sees an opaque "context deadline
	// exceeded" that no server-side log records. Same reasoning as reportBatchMissing
	// below: the symptom reaches the user several hops from the fact.
	fetchStart := time.Now()
	entities, missing, err := c.fetchEntitiesConcurrent(ctx, req.IDs, defaultMaxConcurrent)
	fetchElapsed := time.Since(fetchStart)
	if err != nil {
		c.logger.Warn("entity batch fetch failed",
			slog.Int("requested", len(req.IDs)),
			slog.Duration("elapsed", fetchElapsed),
			slog.String("error", err.Error()))
		return nil, classifyEntityQueryError(err)
	}
	c.logger.Debug("entity batch fetch",
		slog.Int("requested", len(req.IDs)),
		slog.Int("returned", len(entities)),
		slog.Int("missing", len(missing)),
		slog.Duration("elapsed", fetchElapsed))
	c.reportBatchMissing(req.IDs, missing)

	resp := graph.EntityBatchResponse{Entities: entities}
	for _, id := range missing {
		resp.Missing = append(resp.Missing, graph.MissingEntity{ID: id, Reason: batchMissingReason(id)})
	}
	return json.Marshal(resp)
}

// batchMissingReason classifies one unhydrated requested ID. An empty ID was never
// looked up, so `not_found` would assert something unobserved — the exact move this
// change exists to stop. It is malformed input, which is a per-ID fault: `error`.
//
// One function so the wire and the counter cannot drift; they did, briefly.
func batchMissingReason(id string) graph.MissingReason {
	if id == "" {
		return graph.MissingError
	}
	return graph.MissingNotFound
}

// reportBatchMissing makes partial hydration observable: it records whether a dropped
// ID's KV read was genuinely not-found at the moment of failure, and that a batch came
// back short at all, which otherwise reaches the user as a missing search result
// several hops away.
//
// The log carries the IDs (bounded) because "which one" is the whole question; the
// counter carries only counts, since entity IDs are an unbounded label space.
func (c *Component) reportBatchMissing(requested, missing []string) {
	if len(missing) == 0 {
		return
	}
	// Counted by the SAME reason the wire reports, not a blanket not_found: the
	// empty-ID case is reported as `error`, and a
	// metric that disagrees with the response is worse than no metric.
	byReason := make(map[graph.MissingReason]int, 2)
	for _, id := range missing {
		byReason[batchMissingReason(id)]++
	}
	for reason, n := range byReason {
		c.batchMissing.WithLabelValues(string(reason)).Add(float64(n))
	}
	logged := missing
	if len(logged) > maxLoggedMissingIDs {
		logged = logged[:maxLoggedMissingIDs]
	}
	// Debug, not Warn: with reads serving under lag (ADR-084), a semantic index
	// ranking an entity whose ENTITY_STATES write has not landed yet is ordinary
	// operation, and warning on it would train operators to ignore the line. The
	// COUNTER above is the alertable signal — it has a rate, which is what
	// distinguishes normal churn from IDs dropped in bulk.
	c.logger.Debug("batch entity query did not hydrate every requested ID",
		"requested", len(requested),
		"missing", len(missing),
		"reason", string(graph.MissingNotFound),
		"missing_ids", logged)
}

// maxLoggedMissingIDs bounds the ID list on the defer log. A batch is already capped
// well below this in practice; the bound exists so a pathological caller cannot turn
// one log line into a payload.
const maxLoggedMissingIDs = 20

// handleQueryPrefixNATS handles prefix-based entity listing for hierarchy queries.
//
// Wire contract:
//   - Accepts PrefixQueryRequest; omitted cursor requests the first page.
//   - Returns graph.PrefixQueryResponse {"entities":[…],"total":N,"next_cursor":"…"};
//     next_cursor is omitted when the result set is exhausted, and total is
//     always present.
//   - Keys are sorted lexicographically before cursor application. This is a
//     behaviour change from pre-pagination (where order was KV-scan order,
//     i.e. non-deterministic) but is required for cursor correctness and is
//     safe because callers have always treated the result as a set.
func (c *Component) handleQueryPrefixNATS(ctx context.Context, data []byte) ([]byte, error) {
	maxPayload, err := c.natsClient.MaxPayload()
	if err != nil {
		return nil, rejectInternal(fmt.Errorf("observe NATS max payload: %w", err))
	}
	return c.handleQueryPrefixWithMaxPayload(ctx, data, maxPayload)
}

func (c *Component) handleQueryPrefixWithMaxPayload(
	ctx context.Context,
	data []byte,
	maxPayload int64,
) ([]byte, error) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	listing, err := c.listPrefixKeys(ctx, data)
	if err != nil {
		return nil, err
	}
	keys := listing.keys

	// Slice the page — we'll potentially trim further by byte budget below.
	pageKeys := keys
	if len(pageKeys) > listing.limit {
		pageKeys = pageKeys[:listing.limit]
	}

	// Fetch full entities with bounded concurrency and cache. Missing IDs are
	// deliberately NOT reported here: pageKeys came from a live key scan moments ago,
	// so an absence is a concurrent delete rather than an unanswerable question about a
	// caller-supplied ID. The prefix contract is "what is under this prefix now", and a
	// key deleted mid-page is honestly not under it.
	entities, _, err := c.fetchEntitiesConcurrent(ctx, pageKeys, defaultMaxConcurrent)
	if err != nil {
		return nil, classifyEntityQueryError(err)
	}

	// fetchEntitiesConcurrent returns cache-hits-then-misses order, NOT sorted.
	// Re-sort by ID so the page is a deterministic sorted prefix of pageKeys and
	// the byte-trim cursor below (derived from the last RETURNED entity) is the
	// true lexicographic max. Without this, a byte-trimmed page would set a
	// cursor on an arbitrary key and skip entities on the next page.
	sort.Slice(entities, func(i, j int) bool { return entities[i].ID < entities[j].ID })

	// A count-limited page continues after the last key selected for this
	// fetch. Byte fitting may choose an earlier returned entity, in which case
	// marshalFittingPrefixPage replaces this boundary with that entity's ID.
	var followingBoundary string
	if len(keys) > len(pageKeys) && len(pageKeys) > 0 {
		followingBoundary = pageKeys[len(pageKeys)-1]
	}

	return marshalFittingPrefixPage(entities, listing.total, followingBoundary, maxPayload)
}

// prefixListing is what the prefix and prefixIDs verbs share before any stored value is read:
// the keys the request selects, sorted, each once, and past its cursor; the number of distinct
// keys listed; and the request's limit with the maximum applied.
type prefixListing struct {
	keys  []string
	total int
	limit int
}

// listPrefixKeys decodes and checks a PrefixQueryRequest and lists the keys it selects. It reads
// no stored value: an exact six-part prefix with nothing under it is only checked for existence.
func (c *Component) listPrefixKeys(ctx context.Context, data []byte) (prefixListing, error) {
	// Parse the typed request. An omitted cursor denotes the first page.
	var req graph.PrefixQueryRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return prefixListing{}, rejectInvalid(graph.ErrorCodeInvalidRequest, fmt.Errorf("invalid request: %w", err))
	}
	if req.Prefix != "" && req.Pattern != "" {
		return prefixListing{}, rejectInvalid(graph.ErrorCodeInvalidRequest,
			errors.New("invalid request: prefix and pattern are mutually exclusive"))
	}
	if req.Prefix != "" {
		if err := semtypes.ValidateEntityIDPrefix(req.Prefix); err != nil {
			return prefixListing{}, err
		}
	}
	if req.Pattern != "" {
		if err := semtypes.ValidateEntityIDPattern(req.Pattern); err != nil {
			return prefixListing{}, err
		}
	}

	// A limit of zero or less, or over the maximum, means the maximum.
	limit := req.Limit
	if limit <= 0 || limit > graph.MaxPrefixQueryLimit {
		limit = graph.MaxPrefixQueryLimit
	}

	var (
		keys []string
		err  error
	)
	if req.Pattern != "" {
		keys, err = c.entityBucket.KeysByFilter(ctx, req.Pattern)
		if err != nil {
			return prefixListing{}, rejectInternal(fmt.Errorf("failed to get keys: %w", err))
		}
	} else {
		// Build prefix for server-side filtering.
		prefixDot := req.Prefix
		if req.Prefix != "" {
			prefixDot = req.Prefix + "."
		}

		// Use server-side prefix filtering instead of loading all keys.
		keys, err = c.entityBucket.KeysByPrefix(ctx, prefixDot)
		if err != nil {
			return prefixListing{}, rejectInternal(fmt.Errorf("failed to get keys: %w", err))
		}

		// Also check exact match for full entity ID queries (6-part IDs
		// where KeysByPrefix("org.plat.dom.sys.type.inst.") finds nothing).
		if req.Prefix != "" && len(keys) == 0 {
			if _, getErr := c.entityBucket.Get(ctx, req.Prefix); getErr == nil {
				keys = []string{req.Prefix}
			} else if !natsclient.IsKVNotFoundError(getErr) {
				return prefixListing{}, rejectInternal(fmt.Errorf("failed to check exact entity key: %w", getErr))
			}
		}
	}

	// MANDATORY: sort before cursor application — cursor is meaningless
	// without a deterministic key order. Compact then drops repeated keys (nats.go does
	// not promise a listing names each key once under concurrent overwrites), so an entity is served
	// at most once and total counts distinct keys.
	sort.Strings(keys)
	keys = slices.Compact(keys)

	// The reply's total is the listing as listed, whichever page this request reads.
	total := len(keys)

	// Apply cursor: advance past keys up to and including lastKey.
	if req.Cursor != "" {
		lastKey, decodeErr := decodeCursor(req.Cursor)
		if decodeErr != nil {
			return prefixListing{}, rejectInvalid(graph.ErrorCodeInvalidRequest, fmt.Errorf("invalid cursor: %w", decodeErr))
		}
		// SearchStrings finds first index where keys[i] >= lastKey.
		idx := sort.SearchStrings(keys, lastKey)
		// Advance past any key equal to lastKey (inclusive skip).
		for idx < len(keys) && keys[idx] == lastKey {
			idx++
		}
		keys = keys[idx:]
	}
	return prefixListing{keys: keys, total: total, limit: limit}, nil
}

// handleQueryPrefixIDsNATS lists the IDs a PrefixQueryRequest selects, paged as the prefix
// verb pages, and answers a graph.PrefixIDsResponse. It reads no entity, so a damaged stored
// value is listed like any other key and is not recorded: a caller can find it and read the
// rest around it, while the prefix verb keeps refusing the whole page.
func (c *Component) handleQueryPrefixIDsNATS(ctx context.Context, data []byte) ([]byte, error) {
	maxPayload, err := c.natsClient.MaxPayload()
	if err != nil {
		return nil, rejectInternal(fmt.Errorf("observe NATS max payload: %w", err))
	}
	return c.handleQueryPrefixIDsWithMaxPayload(ctx, data, maxPayload)
}

func (c *Component) handleQueryPrefixIDsWithMaxPayload(
	ctx context.Context,
	data []byte,
	maxPayload int64,
) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	listing, err := c.listPrefixKeys(ctx, data)
	if err != nil {
		return nil, err
	}
	page := listing.keys[:min(len(listing.keys), listing.limit)]
	return marshalFittingPrefixIDsPage(page, len(page) < len(listing.keys), listing.total, maxPayload)
}

// marshalFittingPrefixIDsPage encodes the longest leading run of ids whose whole reply fits
// maxPayload. A page ending before the last of ids, or followed by keys past the limit (more),
// carries its last ID as the cursor. A longer run can fit where a shorter one does not, because
// only a page with keys after it carries a cursor, so every length is measured.
func marshalFittingPrefixIDsPage(ids []string, more bool, total int, maxPayload int64) ([]byte, error) {
	cursorAt := func(count int) string {
		if count > 0 && (count < len(ids) || more) {
			return encodeCursor(ids[count-1])
		}
		return ""
	}
	// sizeWith is the length json.Marshal gives a graph.PrefixIDsResponse whose ids encode to
	// idsBytes (brackets and commas included) and whose cursor is cursor: the fields in
	// declaration order, next_cursor left out when empty. A cursor is base64url, which JSON
	// quotes without escaping.
	sizeWith := func(idsBytes int64, cursor string) int64 {
		size := int64(len(`{"ids":`)) + idsBytes + int64(len(`,"total":`)+len(strconv.Itoa(total))+len(`}`))
		if cursor != "" {
			size += int64(len(`,"next_cursor":""`) + len(cursor))
		}
		return size
	}

	idsBytes := int64(len(`[]`))
	best, bestCursor := -1, ""
	firstReply := sizeWith(idsBytes, "")
	if len(ids) == 0 && firstReply <= maxPayload {
		best = 0
	}
	for count := 1; count <= len(ids); count++ {
		encoded, err := json.Marshal(ids[count-1])
		if err != nil {
			return nil, errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeInternal,
				fmt.Errorf("marshal prefix ID %q: %w", ids[count-1], err))
		}
		idsBytes += int64(len(encoded))
		if count > 1 {
			idsBytes++ // comma between IDs
		}
		cursor := cursorAt(count)
		size := sizeWith(idsBytes, cursor)
		if count == 1 {
			firstReply = size
		}
		if size <= maxPayload {
			best, bestCursor = count, cursor
		}
	}
	if best < 0 {
		return nil, rejectInvalidDetail(
			natsclient.ErrorCodeResponseTooLarge,
			map[string]any{
				"response_bytes": firstReply,
				"max_payload":    maxPayload,
			},
			errors.New("prefix ID listing: the first ID exceeds active NATS maximum payload"),
		)
	}

	pageIDs := ids[:best]
	if pageIDs == nil {
		pageIDs = []string{} // an empty page encodes as [], never null
	}
	encoded, err := json.Marshal(graph.PrefixIDsResponse{IDs: pageIDs, Total: total, NextCursor: bestCursor})
	if err != nil {
		return nil, errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeInternal,
			fmt.Errorf("marshal prefix ID response: %w", err))
	}
	if int64(len(encoded)) > maxPayload {
		return nil, errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeInternal,
			fmt.Errorf("prefix ID response fitting invariant: encoded %d bytes exceeds max payload %d", len(encoded), maxPayload))
	}
	return encoded, nil
}

// marshalFittingPrefixPage returns the largest sorted entity prefix whose
// complete typed response encoding fits maxPayload. Candidate measurement
// includes total and the final continuation cursor; no envelope reserve or average
// entity estimate participates in the decision. A page that cannot be encoded,
// or an encoding that breaks the fit, is an invariant failure: internal, class
// fatal, since a retry would fail the same way.
func marshalFittingPrefixPage(
	entities []graph.EntityState,
	total int,
	followingBoundary string,
	maxPayload int64,
) ([]byte, error) {
	entityBytes := make([][]byte, len(entities))
	for i := range entities {
		encoded, err := json.Marshal(entities[i])
		if err != nil {
			return nil, errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeInternal,
				fmt.Errorf("marshal prefix entity %q: %w", entities[i].ID, err))
		}
		entityBytes[i] = encoded
	}

	cursorFor := func(count int) string {
		if count > 0 && count < len(entities) {
			return encodeCursor(entities[count-1].ID)
		}
		if followingBoundary != "" {
			return encodeCursor(followingBoundary)
		}
		return ""
	}

	encodedSize := func(count int, cursor string) (int64, error) {
		size := int64(len(`{"entities":[`) + len(`],"total":`) + len(strconv.Itoa(total)) + len(`}`))
		if entities == nil {
			// A group with no keys encodes its page as null, not [].
			size += int64(len(`null`) - len(`[]`))
		}
		for i := 0; i < count; i++ {
			size += int64(len(entityBytes[i]))
			if i > 0 {
				size++ // comma between entity encodings
			}
		}
		if cursor != "" {
			encodedCursor, err := json.Marshal(cursor)
			if err != nil {
				return 0, errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeInternal,
					fmt.Errorf("marshal prefix cursor: %w", err))
			}
			size += int64(len(`,"next_cursor":`) + len(encodedCursor))
		}
		return size, nil
	}

	bestCount := -1
	bestCursor := ""
	firstCandidateBytes := int64(0)
	for count := 0; count <= len(entities); count++ {
		if count == 0 && len(entities) > 0 {
			continue
		}
		cursor := cursorFor(count)
		size, err := encodedSize(count, cursor)
		if err != nil {
			return nil, err
		}
		if count == 1 || (count == 0 && len(entities) == 0) {
			firstCandidateBytes = size
		}
		if size <= maxPayload {
			bestCount = count
			bestCursor = cursor
		}
	}

	if bestCount < 0 {
		return nil, rejectInvalidDetail(
			natsclient.ErrorCodeResponseTooLarge,
			map[string]any{
				"response_bytes": firstCandidateBytes,
				"max_payload":    maxPayload,
			},
			errors.New("prefix response entity exceeds active NATS maximum payload"),
		)
	}

	response := graph.PrefixQueryResponse{
		Entities:   entities[:bestCount],
		Total:      total,
		NextCursor: bestCursor,
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return nil, errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeInternal,
			fmt.Errorf("marshal prefix response: %w", err))
	}
	if int64(len(encoded)) > maxPayload {
		return nil, errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeInternal,
			fmt.Errorf("prefix response fitting invariant: encoded %d bytes exceeds max payload %d", len(encoded), maxPayload))
	}
	return encoded, nil
}

func classifyEntityQueryError(err error) error {
	if graph.IsStateContractError(err) {
		// Already fatal/graph_state_reset_required: aggregate errors arrive
		// classified once, naming every poisoned entity.
		return err
	}
	return rejectInternal(err)
}

// fetchEntitiesConcurrent fetches entities by IDs using bounded concurrency with cache.
// Cache hits skip KV entirely; cache misses are fetched with bounded concurrency.
//
// It returns the entities found AND the IDs whose read came back not-found, because the
// caller cannot recover the second set from the first: the entity slice is ordered
// cache-hits-then-misses, so it carries no correspondence to the requested order, and a
// caller diffing the sets would have to re-derive what this function already knew. That
// gap is the drop path — a requested ID silently absent from a shorter list.
//
// Order remains non-deterministic by contract; callers that need request order restore
// it themselves (fusionnats.Entities does, because fusion ranks by position).
func (c *Component) fetchEntitiesConcurrent(ctx context.Context, ids []string, maxConcurrent int) ([]graph.EntityState, []string, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent
	}

	// Phase 1: Check cache for all IDs, collect misses
	var cached []graph.EntityState
	var missIDs []string
	var invalid []string
	entityCache := c.entityCacheHandle()
	for _, id := range ids {
		if id == "" {
			// Reported, not skipped. The response contract is that every requested ID
			// appears exactly once across entities+missing; silently dropping the empty
			// string put it in neither and made the accounting a lie for the one input
			// a caller is most likely to send by accident.
			invalid = append(invalid, id)
			continue
		}
		if entityCache != nil {
			if entity, ok := entityCache.Get(id); ok {
				cached = append(cached, entity)
				continue
			}
		}
		missIDs = append(missIDs, id)
	}

	// Phase 2: Fetch cache misses with bounded concurrency
	if len(missIDs) == 0 {
		return cached, invalid, nil
	}

	type fetchResult struct {
		entity   graph.EntityState
		err      error
		ok       bool
		notFound bool
	}

	results := make([]fetchResult, len(missIDs))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, id := range missIDs {
		if err := ctx.Err(); err != nil {
			break
		}

		wg.Add(1)
		go func(idx int, entityID string) {
			defer wg.Done()

			// Acquire semaphore (with context cancellation)
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}

			// Check context after acquiring semaphore
			if ctx.Err() != nil {
				return
			}

			// Capture the per-key invalidation generation BEFORE the KV Get so
			// the repopulating Set below can detect an invalidate that raced our
			// read window (read-after-write coherence guard — see
			// invalidateEntityCacheEntry). This load happens-before the Get in
			// program order.
			gen0 := c.loadEntityCacheGen(entityID)

			// readEntity records poison at the failing revision, so a single
			// batch attempt inventories EVERY poisoned entity it touched,
			// and clears a stale record on a valid read (clear path (c)).
			entity, _, err := c.readEntity(ctx, entityID)
			if err != nil {
				if natsclient.IsKVNotFoundError(err) {
					// Not an error — this ID simply is not in the bucket. It is
					// REPORTED rather than dropped so the caller can tell a short
					// list from a partial one.
					results[idx].notFound = true
				} else {
					results[idx].err = err
				}
				return
			}

			// Repopulate the read-through cache, but drop the Set if this key was
			// invalidated during the read window (gen0 no longer current) — a slow
			// reader must not resurrect pre-write state past a concurrent
			// invalidate. See invalidateEntityCacheEntry for the contract.
			c.repopulateEntityCacheEntry(entityID, entity, gen0)

			results[idx] = fetchResult{entity: entity, ok: true}
		}(i, id)
	}

	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Phase 3: Merge cached + fetched results. Poison fails the WHOLE read with one
	// typed error naming every poisoned entity encountered in this attempt,
	// and never a B-only response that hides A's poison.
	//
	// A not-found ID is different in kind: it does not fail the call, it is collected
	// and REPORTED. Partiality is acceptable; invisibility is not.
	var poisoned []*graph.StateContractError
	var firstOtherErr error
	missing := invalid
	entities := make([]graph.EntityState, 0, len(cached)+len(missIDs))
	entities = append(entities, cached...)
	for i, r := range results {
		if r.err != nil {
			var contractErr *graph.StateContractError
			if errors.As(r.err, &contractErr) {
				poisoned = append(poisoned, contractErr)
			} else if firstOtherErr == nil {
				firstOtherErr = r.err
			}
			continue
		}
		if r.notFound {
			missing = append(missing, missIDs[i])
			continue
		}
		if r.ok {
			entities = append(entities, r.entity)
		}
	}
	if len(poisoned) > 0 {
		return nil, nil, aggregateEntityPoisonError(poisoned)
	}
	if firstOtherErr != nil {
		return nil, nil, firstOtherErr
	}

	return entities, missing, nil
}

// aggregateEntityPoisonError builds the single typed failure for a
// multi-entity read that encountered poisoned entities: fatal
// graph_state_reset_required naming every poisoned entity (bounded list),
// wrapping one typed cause so errors.As(*graph.StateContractError) is
// preserved across the seam. Kills the one-repair-per-round-trip discovery
// loop.
func aggregateEntityPoisonError(poisoned []*graph.StateContractError) error {
	ids := make([]string, 0, len(poisoned))
	for _, p := range poisoned {
		ids = append(ids, p.EntityID)
	}
	sort.Strings(ids)
	sample := ids
	suffix := ""
	if len(sample) > entityPoisonSampleCap {
		sample = sample[:entityPoisonSampleCap]
		suffix = fmt.Sprintf(" (+%d more)", len(ids)-entityPoisonSampleCap)
	}
	return errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,
		fmt.Errorf("read encountered %d poisoned entities: %s%s: %w",
			len(ids), strings.Join(sample, ", "), suffix, poisoned[0]))
}

// encodeCursor encodes a raw key as the prefix verb's opaque, URL-safe cursor.
// Only this package builds or reads cursors: PrefixQueryRequest's contract makes
// them opaque to callers.
func encodeCursor(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

// decodeCursor decodes a cursor back to its raw key. The empty cursor, a first
// page, decodes to the empty key without error.
func decodeCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// invalidateEntityCacheEntry drops a stale entity-query-cache entry after a
// direct entityBucket write, preserving read-after-write coherence for callers
// that query via graph.ingest.query.* (e.g. the gated-DAG executor reading the
// whole unit set right after committing a claim, or any read-after-mutate via
// the NATS mutation API). This is the single invalidation primitive for every
// write path (canonical create/reconcile/append/delete, Graphable merge, and
// hierarchy inference) so the coherence guard below is uniform.
//
// Read-after-write cache-coherence contract: a repopulating Set is dropped if
// the key was invalidated during the read window, so a slow reader cannot
// resurrect pre-write state past a concurrent invalidate. This method bumps the
// per-key invalidation generation AND drops the entry under cacheGenMu; the
// cache-miss repopulate (repopulateEntityCacheEntry) captures the generation
// before its KV Get and re-checks it under the same lock before Setting, so
// {bump+delete} and {gen-check+set} are mutually atomic. An invalidation that
// lands after a reader's KV Get is therefore guaranteed to be observed by that
// reader (it skips its Set); an invalidation that lands after a reader's Set is
// guaranteed to delete the value it wrote. No-op when the cache is disabled.
//
// Correctness precondition: entityBucket.Get is read-your-writes/monotonic
// relative to a committed write's invalidate — a Get issued after an
// invalidation is reflected in gen0 returns that write's revision or newer.
// This holds for single-server NATS (all integration/e2e testcontainers) and
// for leader reads. A clustered deployment serving stale allow_direct reads
// from a lagging replica could return a revision older than an acknowledged
// write — but such a Get already returns stale data on a cold cache, so the
// guard introduces no regression relative to the no-cache baseline.
func (c *Component) invalidateEntityCacheEntry(id string) {
	entityCache := c.entityCacheHandle()
	if entityCache == nil {
		return
	}
	c.cacheGenMu.Lock()
	defer c.cacheGenMu.Unlock()
	c.cacheGen[id]++
	entityCache.Delete(id) //nolint:errcheck
}

// entityCacheHandle reads the entity cache handle once, under mu. A caller uses the copy
// it got: release may close and clear the handle meanwhile, and a closed cache still
// serves Get, Set and Delete (cache.Cache's Close), so a use during or after Stop is
// safe. Nil when the cache is disabled or released.
func (c *Component) entityCacheHandle() cache.Cache[graph.EntityState] {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entityCache
}

// loadEntityCacheGen captures the current invalidation generation for a key.
// The cache-miss read path calls this BEFORE its KV Get so a later repopulating
// Set can detect an invalidation that raced the read window. Returns 0 for a
// never-invalidated key (map zero value), which is a valid baseline.
func (c *Component) loadEntityCacheGen(id string) uint64 {
	c.cacheGenMu.Lock()
	g := c.cacheGen[id]
	c.cacheGenMu.Unlock()
	return g
}

// repopulateEntityCacheEntry stores a freshly-read entity into the query cache
// UNLESS the key was invalidated during the read window — i.e. gen0 (captured
// before the KV Get that produced entity) no longer matches the key's current
// generation. See invalidateEntityCacheEntry for the coherence contract and the
// happens-before argument. No-op when the cache is disabled.
func (c *Component) repopulateEntityCacheEntry(id string, entity graph.EntityState, gen0 uint64) {
	entityCache := c.entityCacheHandle()
	if entityCache == nil {
		return
	}
	c.cacheGenMu.Lock()
	defer c.cacheGenMu.Unlock()
	if c.cacheGen[id] == gen0 {
		entityCache.Set(id, entity) //nolint:errcheck
	}
}
