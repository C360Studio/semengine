package fusionnats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/fusion"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// requester is the one NATS operation the client needs: a classified
// request/reply. *natsclient.Client satisfies it. It is local so the production
// path imports no NATS client and the unit tests can fake the wire.
type requester interface {
	RequestClassified(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error)
}

// verbs are the rows of graph.QueryVerbs() the client uses: graph-query's, and
// graph-lexical's search for an NL resolve.
type verbs struct {
	entity, batch, prefixIDs, byName, relationships, wordSearch graph.QueryVerb
}

// lookupVerbs reads the client's rows from table, refusing one that is missing
// and naming it, so a table that drops a verb fails construction rather than a
// later call.
func lookupVerbs(table []graph.QueryVerb) (verbs, error) {
	var v verbs
	for _, want := range []struct {
		responder, name string
		row             *graph.QueryVerb
	}{
		{graph.ResponderGraphQuery, "entity", &v.entity},
		{graph.ResponderGraphQuery, "batch", &v.batch},
		{graph.ResponderGraphQuery, "prefixIDs", &v.prefixIDs},
		{graph.ResponderGraphQuery, "byName", &v.byName},
		{graph.ResponderGraphQuery, "relationships", &v.relationships},
		{graph.ResponderGraphLexical, "search", &v.wordSearch},
	} {
		found := false
		for _, verb := range table {
			if verb.Responder == want.responder && verb.Name == want.name {
				*want.row = verb
				found = true
				break
			}
		}
		if !found {
			return verbs{}, fmt.Errorf("fusionnats: the verb table has no %s verb %q", want.responder, want.name)
		}
	}
	return v, nil
}

// Client is the NATS implementation of fusion.RetrievalClient. It holds no
// goroutine, watch or lock, so every method is safe to call from any goroutine.
// Build it with New.
type Client struct {
	nats    requester
	timeout time.Duration
	verbs   verbs
}

// Compile-time assertion that Client satisfies the engine's retrieval surface.
var _ fusion.RetrievalClient = (*Client)(nil)

// New builds a retrieval client over nats, sending each request with
// timeout. It refuses a nil requester and a negative timeout; a zero timeout is
// passed to the requester, which applies its default (natsclient's
// DefaultRequestTimeout). A typed-nil pointer is not detected.
func New(nats requester, timeout time.Duration) (*Client, error) {
	if nats == nil {
		return nil, errors.New("fusionnats: requester is required")
	}
	// natsclient replaces only a zero timeout with its default; a negative one
	// would give every request an already expired deadline.
	if timeout < 0 {
		return nil, fmt.Errorf("fusionnats: timeout must not be negative, got %s", timeout)
	}
	v, err := lookupVerbs(graph.QueryVerbs())
	if err != nil {
		return nil, err
	}
	return &Client{nats: nats, timeout: timeout, verbs: v}, nil
}

// Resolve maps a query to seeds by mode, most relevant first. An unknown mode is
// an error, never a default (ResolveMode is an open string enum).
func (c *Client) Resolve(ctx context.Context, q fusion.ResolveQuery) (fusion.Resolution, error) {
	switch q.Mode {
	case fusion.ResolveModeSymbol:
		return c.resolveByName(ctx, q.Query, q.Limit)
	case fusion.ResolveModePrefix:
		return c.resolvePrefix(ctx, q.Query, q.Limit)
	case fusion.ResolveModeNL:
		return c.resolveWords(ctx, q.Query, q.Scope, q.Limit)
	default:
		return fusion.Resolution{}, fmt.Errorf("fusionnats: unsupported resolve mode %q", q.Mode)
	}
}

// resolveByName resolves a symbol through byName. That reply ranks by order and
// carries no score, so the seeds carry none.
func (c *Client) resolveByName(ctx context.Context, query string, limit int) (fusion.Resolution, error) {
	matches, err := c.byNameMatches(ctx, query, limit)
	if err != nil {
		return fusion.Resolution{}, err
	}
	seeds := make([]fusion.Seed, 0, len(matches))
	for _, m := range matches {
		seeds = append(seeds, fusion.Seed{ID: m.EntityID})
	}
	return fusion.Resolution{Seeds: seeds}, nil
}

// resolvePrefix resolves an ID prefix through prefixIDs, taking only the first
// page: seeds are bounded by limit, not exhaustive. The listing reads no stored
// value, so a damaged entity under the prefix becomes a seed and is reported
// when it is hydrated, rather than failing the resolve. An enumeration carries
// no score, so the seeds carry none.
func (c *Client) resolvePrefix(ctx context.Context, query string, limit int) (fusion.Resolution, error) {
	if err := validatePrefix(query); err != nil {
		return fusion.Resolution{}, err
	}
	raw, err := c.request(ctx, c.verbs.prefixIDs, graph.PrefixQueryRequest{Prefix: query, Limit: limit})
	if err != nil {
		return fusion.Resolution{}, err
	}
	resp, err := decodeReply[graph.PrefixIDsResponse](c.verbs.prefixIDs, raw)
	if err != nil {
		return fusion.Resolution{}, err
	}
	if err := graph.ValidateDecodedEntityIDs(resp.IDs); err != nil {
		return fusion.Resolution{}, fmt.Errorf("fusionnats: validate prefix: %w", err)
	}
	seeds := make([]fusion.Seed, 0, len(resp.IDs))
	for _, id := range resp.IDs {
		seeds = append(seeds, fusion.Seed{ID: id})
	}
	return fusion.Resolution{Seeds: seeds}, nil
}

// resolveWords resolves a natural-language query through graph-lexical's word
// search. The query passes unchanged, so the word index's query rules apply and
// its refusals are returned as it sent them. A non-empty scope keeps only
// entities under its ID prefixes (ADR-071); an empty one is left out of the
// request. Seeds keep the reply's order and carry each hit's score as the
// mode's own score. A request nothing answers, graph-lexical not composed
// included, is a transient internal error naming the subject.
func (c *Client) resolveWords(ctx context.Context, query string, scope []string, limit int) (fusion.Resolution, error) {
	if err := validateScope(scope); err != nil {
		return fusion.Resolution{}, err
	}
	verb := c.verbs.wordSearch
	raw, err := c.request(ctx, verb, graph.LexicalSearchRequest{Query: query, Limit: limit, Scope: scope})
	if err != nil {
		var refusal *errs.ClassifiedError
		if errors.As(err, &refusal) {
			return fusion.Resolution{}, err
		}
		return fusion.Resolution{}, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal,
			fmt.Errorf("fusionnats: request to %s: %w", verb.Subject, err))
	}
	resp, err := decodeReply[graph.LexicalSearchData](verb, raw)
	if err != nil {
		return fusion.Resolution{}, err
	}
	seeds := make([]fusion.Seed, 0, len(resp.Results))
	ids := make([]string, 0, len(resp.Results))
	for _, hit := range resp.Results {
		seeds = append(seeds, fusion.Seed{ID: hit.EntityID, Similarity: hit.Score, HasSimilarity: true})
		ids = append(ids, hit.EntityID)
	}
	if err := graph.ValidateDecodedEntityIDs(ids); err != nil {
		return fusion.Resolution{}, fmt.Errorf("fusionnats: validate word search: %w", err)
	}
	return fusion.Resolution{Seeds: seeds}, nil
}

func validatePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	return semtypes.ValidateEntityIDPrefix(prefix)
}

func validateScope(scope []string) error {
	for _, prefix := range scope {
		// An explicit empty member is the documented match-all scope.
		if prefix == "" {
			continue
		}
		if err := semtypes.ValidateEntityIDPrefix(prefix); err != nil {
			return err
		}
	}
	return nil
}

// entityReply is the entity verb's reply with the stored value left raw, so it
// is decoded once, by graph.UnmarshalEntityState. Entity shadows the embedded
// field; encoding/json takes the shallower one and the embedded kv_revision.
type entityReply struct {
	graph.ExactEntity
	Entity json.RawMessage `json:"entity"`
}

// Entity returns the entity id, or (nil, nil) when the read finds none. The
// entity verb refuses a missing entity with code entity_not_found; that one code
// is read as absence, and every other error is returned, so the engine never
// reads a backend fault as a not-found. A stored value the stored-value rule
// refuses, the wrong entity included, is returned as that refusal.
func (c *Client) Entity(ctx context.Context, id string) (*fusion.Entity, error) {
	raw, err := c.request(ctx, c.verbs.entity, graph.EntityRequest{ID: id})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return c.decodeEntity(id, raw)
}

// decodeEntity decodes and checks an entity reply for id. Its refusals are the
// client's own, so a caller never reads them as the responder's.
func (c *Client) decodeEntity(id string, raw []byte) (*fusion.Entity, error) {
	reply, err := decodeReply[entityReply](c.verbs.entity, raw)
	if err != nil {
		return nil, err
	}
	if len(reply.Entity) == 0 || string(reply.Entity) == "null" {
		return nil, fmt.Errorf("fusionnats: entity %q reply has no entity", id)
	}
	if reply.KVRevision == 0 {
		return nil, fmt.Errorf("fusionnats: entity %q reply has zero KV revision", id)
	}
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(id, reply.Entity, &state); err != nil {
		return nil, err
	}
	return &fusion.Entity{ID: state.ID, Triples: state.Triples}, nil
}

// Entities reads the entities ids names through batch, then reconciles the
// reply with the request: every distinct requested ID is accounted for once, in
// Entities in request order or in Unhydrated with a reason. Zero IDs send no
// request. A non-nil error is a backend failure, which the engine tells apart
// from an entity that is simply absent.
//
// The batch's responder answers cache hits first, and the engine ranks by
// position, so request order is restored here, where it is still known; and an
// ID the reply lists nowhere is unhydrated unknown, never not_found
// (reconcileHydration).
//
// The batch's responder refuses the whole batch with code
// graph_state_reset_required when one stored entity is poisoned, naming it only
// in text. On that refusal the IDs are read one at a time (entitiesAlone), so a
// poisoned seed is listed unhydrated error rather than failing every answer it
// ranks in (ADR-109). A refusal from the client's own check of a reply carries
// the same code; it is returned and never starts that read. A nil context is
// refused, even for zero IDs.
func (c *Client) Entities(ctx context.Context, ids []string) (fusion.Hydration, error) {
	if ctx == nil {
		return fusion.Hydration{}, errors.New("fusionnats: nil context")
	}
	if len(ids) == 0 {
		return fusion.Hydration{}, nil
	}
	raw, err := c.request(ctx, c.verbs.batch, graph.EntityBatchRequest{IDs: ids})
	if err != nil {
		// Only the responder's refusal, before any decode, means poison here.
		if graph.IsStateContractError(err) {
			return c.entitiesAlone(ctx, ids)
		}
		return fusion.Hydration{}, err
	}
	resp, err := decodeReply[graph.EntityBatchResponse](c.verbs.batch, raw)
	if err != nil {
		return fusion.Hydration{}, err
	}
	if err := graph.ValidateDecodedEntityStates(resp.Entities); err != nil {
		return fusion.Hydration{}, fmt.Errorf("fusionnats: validate batch: %w", err)
	}
	return reconcileHydration(ids, resp), nil
}

// entitiesAlone reads each distinct ID in ids with its own entity request, in
// request order, after the batch was refused for poison. An ID the responder
// refuses with graph_state_reset_required is unhydrated error; one not found is
// unhydrated not_found. An empty ID is unhydrated error with no request, the
// reason the batch gives it. Any other error, the client's own refusal of a
// reply and an ended context included, is returned. It costs one request per
// distinct ID, and only while poison is present. The context is checked before
// each request and after each refused one.
func (c *Client) entitiesAlone(ctx context.Context, ids []string) (fusion.Hydration, error) {
	out := fusion.Hydration{Entities: make([]*fusion.Entity, 0, len(ids))}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if id == "" {
			out.Unhydrated = append(out.Unhydrated, fusion.Unhydrated{Handle: id, Reason: fusion.UnhydratedError})
			continue
		}
		// Stop between requests once the caller has given up.
		if err := ctx.Err(); err != nil {
			return fusion.Hydration{}, err
		}
		raw, err := c.request(ctx, c.verbs.entity, graph.EntityRequest{ID: id})
		if err != nil {
			// A refusal that arrives after the caller gave up is not listed: the
			// ended context is the answer.
			if cerr := ctx.Err(); cerr != nil {
				return fusion.Hydration{}, cerr
			}
			switch {
			case isNotFound(err):
				out.Unhydrated = append(out.Unhydrated, fusion.Unhydrated{Handle: id, Reason: fusion.UnhydratedNotFound})
				continue
			case graph.IsStateContractError(err):
				out.Unhydrated = append(out.Unhydrated, fusion.Unhydrated{Handle: id, Reason: fusion.UnhydratedError})
				continue
			default:
				return fusion.Hydration{}, err
			}
		}
		ent, err := c.decodeEntity(id, raw)
		if err != nil {
			return fusion.Hydration{}, err
		}
		out.Entities = append(out.Entities, ent)
	}
	return out, nil
}

// unhydratedReason maps a reply's missing reason into fusion's closed set. The
// reply is untrusted input, so a reason outside the set, or none, is unknown
// rather than cast through to a consumer's exhaustive switch.
func unhydratedReason(r graph.MissingReason) fusion.UnhydratedReason {
	switch r {
	case graph.MissingNotFound:
		return fusion.UnhydratedNotFound
	case graph.MissingError:
		return fusion.UnhydratedError
	default:
		return fusion.UnhydratedUnknown
	}
}

// reconcileHydration turns a batch reply into a total account of the requested
// IDs, in request order (ADR-084). It works by ID set, never by count: a
// duplicate request is one entity, reported once; an ID the reply both returns
// and lists missing is hydrated, since the entity is held; an ID in neither list
// is unknown, since nobody observed it missing; an entity nobody asked for is
// dropped.
func reconcileHydration(requested []string, resp graph.EntityBatchResponse) fusion.Hydration {
	hydrated := make(map[string]*fusion.Entity, len(resp.Entities))
	for i := range resp.Entities {
		hydrated[resp.Entities[i].ID] = &fusion.Entity{
			ID: resp.Entities[i].ID, Triples: resp.Entities[i].Triples,
		}
	}
	reported := make(map[string]fusion.UnhydratedReason, len(resp.Missing))
	for _, m := range resp.Missing {
		reported[m.ID] = unhydratedReason(m.Reason)
	}

	out := fusion.Hydration{Entities: make([]*fusion.Entity, 0, len(resp.Entities))}
	seen := make(map[string]bool, len(requested))
	for _, id := range requested {
		if seen[id] {
			continue
		}
		seen[id] = true
		if ent, ok := hydrated[id]; ok {
			out.Entities = append(out.Entities, ent)
			continue
		}
		reason, ok := reported[id]
		if !ok {
			reason = fusion.UnhydratedUnknown
		}
		out.Unhydrated = append(out.Unhydrated, fusion.Unhydrated{Handle: id, Reason: reason})
	}
	return out
}

// relationshipsRequest is the relationships verb's request; graph declares no
// type for it.
type relationshipsRequest struct {
	EntityID  string `json:"entity_id"`
	Direction string `json:"direction"`
}

// Neighbors returns the edges of id in dir along predicates through
// relationships. The verb answers every edge in the direction, so this keeps
// only the requested predicates; no predicates means every edge.
func (c *Client) Neighbors(ctx context.Context, id string, predicates []string, dir fusion.Direction) ([]fusion.Edge, error) {
	raw, err := c.request(ctx, c.verbs.relationships, relationshipsRequest{EntityID: id, Direction: directionString(dir)})
	if err != nil {
		return nil, err
	}
	rels, err := decodeReply[[]graph.RelationshipEdge](c.verbs.relationships, raw)
	if err != nil {
		return nil, err
	}
	endpointIDs := make([]string, 0, len(rels)*2)
	for _, r := range rels {
		endpointIDs = append(endpointIDs, r.FromEntityID, r.ToEntityID)
	}
	if err := graph.ValidateDecodedEntityIDs(endpointIDs); err != nil {
		return nil, fmt.Errorf("fusionnats: validate relationships: %w", err)
	}
	want := predicateSet(predicates)
	edges := make([]fusion.Edge, 0, len(rels))
	for _, r := range rels {
		if want != nil && !want[r.Predicate] {
			continue
		}
		// The target is the other end: outgoing points to to_entity_id,
		// incoming comes from from_entity_id.
		target := r.ToEntityID
		if dir == fusion.Incoming {
			target = r.FromEntityID
		}
		edges = append(edges, fusion.Edge{Predicate: r.Predicate, Target: target})
	}
	return edges, nil
}

// Names suggests display names near query for a miss's did_you_mean, through
// byName. Entities may share a name, so names are deduplicated in rank order and
// capped at limit.
func (c *Client) Names(ctx context.Context, query string, limit int) ([]string, error) {
	matches, err := c.byNameMatches(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		if m.MatchedName == "" || seen[m.MatchedName] {
			continue
		}
		seen[m.MatchedName] = true
		names = append(names, m.MatchedName)
		if limit > 0 && len(names) >= limit {
			break
		}
	}
	return names, nil
}

// byNameRequest is the byName verb's request; graph declares no type for it.
type byNameRequest struct {
	Name  string `json:"name"`
	Limit int    `json:"limit"`
}

// byNameMatches asks byName and returns the ranked matches, for Resolve
// (symbol), which reads the IDs, and Names, which reads the names.
func (c *Client) byNameMatches(ctx context.Context, query string, limit int) ([]graph.NameMatch, error) {
	raw, err := c.request(ctx, c.verbs.byName, byNameRequest{Name: query, Limit: limit})
	if err != nil {
		return nil, err
	}
	resp, err := decodeReply[graph.NameData](c.verbs.byName, raw)
	if err != nil {
		return nil, err
	}
	entityIDs := make([]string, 0, len(resp.Matches))
	for _, match := range resp.Matches {
		entityIDs = append(entityIDs, match.EntityID)
	}
	if err := graph.ValidateDecodedEntityIDs(entityIDs); err != nil {
		return nil, fmt.Errorf("fusionnats: validate byName: %w", err)
	}
	return resp.Matches, nil
}

// request marshals req and sends it as a classified request on verb's subject.
func (c *Client) request(ctx context.Context, verb graph.QueryVerb, req any) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("fusionnats: marshal %s request: %w", verb.Name, err)
	}
	return c.nats.RequestClassified(ctx, verb.Subject, body, c.timeout)
}

// decodeReply decodes raw as verb's row declares: an enveloped row's reply is a
// graph.QueryResponse whose Data is T, refused when it names no producer (which
// graph.NewQueryResponse never builds, so a bare body or "{}" is refused); a
// bare row's reply is T, unknown keys ignored.
func decodeReply[T any](verb graph.QueryVerb, raw []byte) (T, error) {
	var zero T
	if !verb.Enveloped {
		var body T
		if err := json.Unmarshal(raw, &body); err != nil {
			return zero, fmt.Errorf("fusionnats: decode %s reply: %w", verb.Name, err)
		}
		return body, nil
	}
	var reply graph.QueryResponse[T]
	if err := json.Unmarshal(raw, &reply); err != nil {
		return zero, fmt.Errorf("fusionnats: decode %s reply: %w", verb.Name, err)
	}
	if reply.Producer == "" {
		return zero, fmt.Errorf("fusionnats: %s reply names no producer", verb.Name)
	}
	return reply.Data, nil
}

// directionString maps a fusion.Direction to the relationships verb's value.
func directionString(dir fusion.Direction) string {
	if dir == fusion.Incoming {
		return "incoming"
	}
	return "outgoing"
}

// predicateSet builds a lookup set from predicates, or nil, meaning no filter,
// when none are given.
func predicateSet(predicates []string) map[string]bool {
	if len(predicates) == 0 {
		return nil
	}
	set := make(map[string]bool, len(predicates))
	for _, p := range predicates {
		set[p] = true
	}
	return set
}

// isNotFound reports whether err carries the code entity_not_found, the one
// error Entity reads as absence.
func isNotFound(err error) bool {
	var ce *errs.ClassifiedError
	return errors.As(err, &ce) && ce.Code == graph.ErrorCodeEntityNotFound
}
