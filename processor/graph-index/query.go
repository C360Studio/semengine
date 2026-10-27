package graphindex

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/vocabulary"
)

// queryTimeout bounds each handler; predicateList, which scans every membership, gets
// listTimeout. Both are the pin's fixed values (design D6).
const (
	queryTimeout = 5 * time.Second
	listTimeout  = 10 * time.Second
)

// nameQueryMaxHydration bounds how many NAME_INDEX memberships a byName query reads one by
// one before it refuses with resource_exhausted; the caller narrows the query.
const nameQueryMaxHydration = 2000

// queryHandlers maps each verb graph-index answers, by its name in graph.QueryVerbs, to its
// handler.
func (c *Component) queryHandlers() map[string]func(context.Context, []byte) ([]byte, error) {
	return map[string]func(context.Context, []byte) ([]byte, error){
		"outgoing":          c.handleQueryOutgoing,
		"incoming":          c.handleQueryIncoming,
		"alias":             c.handleQueryAlias,
		"predicate":         c.handleQueryPredicate,
		"predicateList":     c.handleQueryPredicateList,
		"predicateStats":    c.handleQueryPredicateStats,
		"predicateCompound": c.handleQueryPredicateCompound,
		"byName":            c.handleQueryByName,
	}
}

// subscribeQueries subscribes to the subject of every verb the table in graph declares for
// graph-index, and to nothing else (design D6). A declared verb with no handler here fails
// Start, as graph-ingest's does. Each subscription is recorded as it is made, so a Start
// that fails part way drains the ones already made.
func (c *Component) subscribeQueries(ctx context.Context) error {
	handlers := c.queryHandlers()
	var subjects []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphIndex {
			continue
		}
		handler, ok := handlers[verb.Name]
		if !ok {
			return fmt.Errorf("query verb %q (%s) is declared for graph-index but has no handler", verb.Name, verb.Subject)
		}
		sub, err := c.natsClient.SubscribeForRequests(ctx, verb.Subject, handler)
		if err != nil {
			return errs.Wrap(err, "Component", "Start", "subscribe "+verb.Name+" query")
		}
		c.subscriptions = append(c.subscriptions, sub)
		subjects = append(subjects, verb.Subject)
	}
	c.logger.Info("query handlers registered", slog.Any("subjects", subjects))
	return nil
}

// queryRevision refuses the query while the follower withholds readiness (QueryReady,
// design D5) and otherwise returns the revision the reply carries. It is read here, before
// the handler's first bucket read, so every revision at or below it is reflected in what
// the handler then reads (design D6).
func (c *Component) queryRevision(ctx context.Context) (uint64, error) {
	f := c.follower.Load()
	if err := f.QueryReady(ctx); err != nil {
		return 0, err
	}
	return f.Indexed(), nil
}

// reply is a successful reply: data in the graph.QueryResponse envelope, producer
// graph-index, carrying revision.
func reply[T any](revision uint64, data T) ([]byte, error) {
	response, err := graph.NewQueryResponse(graph.ResponderGraphIndex, revision, data)
	if err != nil {
		return nil, internalError(err)
	}
	return json.Marshal(response)
}

func invalidRequest(err error) error {
	return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, fmt.Errorf("invalid request: %w", err))
}

func internalError(err error) error {
	return errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeInternal, fmt.Errorf("internal error: %w", err))
}

// entityRequest is the request of the outgoing and incoming verbs.
type entityRequest struct {
	EntityID string `json:"entity_id"`
}

// decodeEntityRequest decodes and validates an outgoing or incoming request.
func decodeEntityRequest(data []byte) (string, error) {
	var req entityRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return "", invalidRequest(err)
	}
	if req.EntityID == "" {
		return "", invalidRequest(errors.New("empty entity_id"))
	}
	if err := validateEntityLiteralKey(req.EntityID); err != nil {
		return "", invalidRequest(fmt.Errorf("entity_id: %w", err))
	}
	return req.EntityID, nil
}

// handleQueryOutgoing answers an entity's outgoing relationships: its OUTGOING_INDEX row,
// or none.
func (c *Component) handleQueryOutgoing(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	entityID, err := decodeEntityRequest(data)
	if err != nil {
		return nil, err
	}
	revision, err := c.queryRevision(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := c.outgoingBucket.Get(ctx, entityID)
	if natsclient.IsKVNotFoundError(err) {
		return reply(revision, graph.OutgoingRelationshipsData{Relationships: []graph.OutgoingEntry{}})
	}
	if err != nil {
		return nil, internalError(err)
	}
	var entries []graph.OutgoingEntry
	if err := json.Unmarshal(entry.Value, &entries); err != nil {
		return nil, internalError(err)
	}
	return reply(revision, graph.OutgoingRelationshipsData{Relationships: entries})
}

// handleQueryIncoming answers the edges that point at an entity, read from the
// INCOMING_INDEX keys under target.>, sorted by source and predicate and deduplicated.
func (c *Component) handleQueryIncoming(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	entityID, err := decodeEntityRequest(data)
	if err != nil {
		return nil, err
	}
	prefix := entityID + "."
	if err := validateKVPrefix(prefix); err != nil {
		return nil, invalidRequest(err)
	}
	revision, err := c.queryRevision(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := c.incomingBucket.KeysByPrefix(ctx, prefix)
	if err != nil {
		return nil, internalError(err)
	}
	entries := make([]graph.IncomingEntry, 0, len(keys))
	for _, key := range keys {
		entry, ok := incomingEntryFromKey(key, prefix)
		if !ok {
			c.logger.Debug("incoming query: skipping malformed key", slog.String("key", key))
			continue
		}
		entries = append(entries, entry)
	}
	// Sort and deduplicate the whole set: a filtered listing may repeat a key, and storage
	// order depends on write timing.
	slices.SortFunc(entries, func(a, b graph.IncomingEntry) int {
		return cmp.Or(strings.Compare(a.FromEntityID, b.FromEntityID), strings.Compare(a.Predicate, b.Predicate))
	})
	entries = slices.Compact(entries)
	return reply(revision, graph.IncomingRelationshipsData{Relationships: entries})
}

// incomingEntryFromKey reads an INCOMING_INDEX key, target.source6.hex(predicate), whose
// target prefix is prefix. A key that is not that shape, or holds an invalid source ID or a
// noncanonical predicate, is refused.
func incomingEntryFromKey(key, prefix string) (graph.IncomingEntry, bool) {
	suffix, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return graph.IncomingEntry{}, false
	}
	sourceID, predicate, ok := entityAndPredicate(suffix)
	if !ok {
		return graph.IncomingEntry{}, false
	}
	return graph.IncomingEntry{FromEntityID: sourceID, Predicate: predicate}, true
}

// entityAndPredicate reads entity6.hex(predicate), the tail of an INCOMING_INDEX and a
// NAME_INDEX key: exactly seven tokens, a valid entity ID and a canonical predicate.
func entityAndPredicate(suffix string) (entityID, predicate string, ok bool) {
	parts := strings.Split(suffix, ".")
	if len(parts) != 7 {
		return "", "", false
	}
	entityID = strings.Join(parts[:6], ".")
	predicate, decoded := graph.DecodePredicateToken(parts[6])
	if !decoded || predicate == "" {
		return "", "", false
	}
	if err := semtypes.ValidateEntityID(entityID); err != nil {
		return "", "", false
	}
	if _, err := vocabulary.ParsePredicate(predicate); err != nil {
		return "", "", false
	}
	return entityID, predicate, true
}

// handleQueryAlias resolves an alias through ALIAS_INDEX. While #415 is open an alias that
// was dropped or changed, or whose entity was deleted, may still resolve (design D12).
func (c *Component) handleQueryAlias(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, invalidRequest(err)
	}
	if req.Alias == "" {
		return nil, invalidRequest(errors.New("empty alias"))
	}
	if err := natsclient.ValidateKVLiteralKey(req.Alias); err != nil {
		return nil, invalidRequest(err)
	}
	revision, err := c.queryRevision(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := c.aliasBucket.Get(ctx, req.Alias)
	if natsclient.IsKVNotFoundError(err) {
		return reply(revision, graph.AliasData{})
	}
	if err != nil {
		return nil, internalError(err)
	}
	canonicalID := string(entry.Value)
	return reply(revision, graph.AliasData{CanonicalID: &canonicalID})
}

// handleQueryPredicate answers the entities carrying a predicate, sorted; with a value,
// only those whose current state holds that predicate with that value; then the limit.
func (c *Component) handleQueryPredicate(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var req struct {
		Predicate string  `json:"predicate"`
		Value     *string `json:"value,omitempty"`
		Limit     int     `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, invalidRequest(err)
	}
	if _, err := predicateExactFilter(req.Predicate); err != nil {
		return nil, invalidPredicateQuery(err)
	}
	revision, err := c.queryRevision(ctx)
	if err != nil {
		return nil, err
	}
	entities, err := c.predicateEntities(ctx, req.Predicate)
	if err != nil {
		return nil, internalError(err)
	}
	if req.Value != nil {
		entities, err = c.entitiesWithValue(ctx, entities, req.Predicate, *req.Value, req.Limit)
		if err != nil {
			return nil, internalError(err)
		}
	}
	return reply(revision, graph.PredicateData{Entities: limited(entities, req.Limit)})
}

// limited returns the first limit values; a limit of zero or less keeps them all.
func limited(values []string, limit int) []string {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

// predicateEntities lists the entities with a PREDICATE_INDEX membership for predicate,
// sorted and deduplicated.
func (c *Component) predicateEntities(ctx context.Context, predicate string) ([]string, error) {
	filter, err := predicateExactFilter(predicate)
	if err != nil {
		return nil, err
	}
	keys, err := c.predicateBucket.KeysByFilter(ctx, filter)
	if err != nil {
		return nil, err
	}
	entities := make([]string, 0, len(keys))
	for _, key := range keys {
		stored, entityID, ok := predicateMembershipFromKey(key)
		if !ok || stored != predicate {
			continue
		}
		entities = append(entities, entityID)
	}
	slices.Sort(entities)
	return slices.Compact(entities), nil
}

// entitiesWithValue keeps, in order, the entities whose current ENTITY_STATES value holds
// predicate with value, stopping once limit are found (zero or less: no limit). An entity
// deleted since the membership was read is left out; any other read failure fails the
// whole query rather than answer part of it.
func (c *Component) entitiesWithValue(ctx context.Context, entityIDs []string, predicate, value string, limit int) ([]string, error) {
	var matched []string
	for _, entityID := range entityIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry, err := c.entities.Get(ctx, entityID)
		if natsclient.IsKVNotFoundError(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("value filter: read entity %s: %w", entityID, err)
		}
		var state graph.EntityState
		if err := graph.UnmarshalEntityState(entityID, entry.Value(), &state); err != nil {
			return nil, err
		}
		for _, triple := range state.Triples {
			if triple.Predicate == predicate && objectString(triple.Object) == value {
				matched = append(matched, entityID)
				break
			}
		}
		if limit > 0 && len(matched) >= limit {
			break
		}
	}
	return matched, nil
}

// objectString is a triple object as the value filter compares it: an integral JSON number
// without a trailing decimal point ("85", not "85.0").
func objectString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// handleQueryPredicateList answers every predicate with its entity count, in lexical
// order, or only those in one domain or domain.category namespace.
func (c *Component) handleQueryPredicateList(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	var req graph.PredicateListQuery
	if len(data) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			return nil, invalidRequest(err)
		}
	}
	filter := ""
	if req.Namespace != "" {
		var err error
		if filter, err = predicateNamespaceFilter(strings.TrimSuffix(req.Namespace, ".")); err != nil {
			return nil, invalidPredicateQuery(err)
		}
	}
	revision, err := c.queryRevision(ctx)
	if err != nil {
		return nil, err
	}
	var keys []string
	if filter == "" {
		keys, err = c.predicateBucket.Keys(ctx)
	} else {
		keys, err = c.predicateBucket.KeysByFilter(ctx, filter)
	}
	if err != nil {
		return nil, internalError(err)
	}
	predicates := predicateSummaries(keys)
	return reply(revision, graph.PredicateListData{Predicates: predicates, Total: len(predicates)})
}

// handleQueryPredicateStats answers a predicate's entity count and the first sample_limit
// of its entities in order (default 10).
func (c *Component) handleQueryPredicateStats(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var req struct {
		Predicate   string `json:"predicate"`
		SampleLimit int    `json:"sample_limit"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, invalidRequest(err)
	}
	if _, err := predicateExactFilter(req.Predicate); err != nil {
		return nil, invalidPredicateQuery(err)
	}
	if req.SampleLimit <= 0 {
		req.SampleLimit = 10
	}
	revision, err := c.queryRevision(ctx)
	if err != nil {
		return nil, err
	}
	entities, err := c.predicateEntities(ctx, req.Predicate)
	if err != nil {
		return nil, internalError(err)
	}
	return reply(revision, graph.PredicateStatsData{
		Predicate:      req.Predicate,
		EntityCount:    len(entities),
		SampleEntities: limited(entities, req.SampleLimit),
	})
}

// handleQueryPredicateCompound answers the intersection (AND) or union (OR) of the entity
// sets of several predicates, sorted, then the limit; matched counts the whole set.
func (c *Component) handleQueryPredicateCompound(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var req graph.CompoundPredicateQuery
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, invalidRequest(err)
	}
	if len(req.Predicates) == 0 {
		return nil, invalidRequest(errors.New("empty predicates"))
	}
	for _, predicate := range req.Predicates {
		if _, err := predicateExactFilter(predicate); err != nil {
			return nil, invalidPredicateQuery(err)
		}
	}
	if req.Operator != "AND" && req.Operator != "OR" {
		return nil, invalidRequest(errors.New("operator must be AND or OR"))
	}
	revision, err := c.queryRevision(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, predicate := range req.Predicates {
		entities, err := c.predicateEntities(ctx, predicate)
		if err != nil {
			return nil, internalError(err)
		}
		for _, entityID := range entities {
			counts[entityID]++
		}
	}
	// A predicate named twice counts twice, so AND keeps an entity only when it is in every
	// listed set: the intersection.
	entities := make([]string, 0, len(counts))
	for entityID, n := range counts {
		if req.Operator == "OR" || n == len(req.Predicates) {
			entities = append(entities, entityID)
		}
	}
	slices.Sort(entities)
	return reply(revision, graph.CompoundPredicateData{
		Entities: limited(entities, req.Limit),
		Operator: req.Operator,
		Matched:  len(entities),
	})
}

// predicateExactFilter is the PREDICATE_INDEX filter for one canonical predicate's
// memberships: predicate3.*.*.*.*.*.*.
func predicateExactFilter(predicate string) (string, error) {
	if _, err := vocabulary.ParsePredicate(predicate); err != nil {
		return "", err
	}
	filter := predicate + "." + wildcardPositions(6)
	if err := natsclient.ValidateKVWildcardFilter(filter); err != nil {
		return "", err
	}
	return filter, nil
}

// predicateNamespaceFilter is the PREDICATE_INDEX filter for a one-token (domain) or
// two-token (domain.category) namespace: nine tokens either way.
func predicateNamespaceFilter(namespace string) (string, error) {
	if _, err := vocabulary.ParsePredicateNamespace(namespace); err != nil {
		return "", err
	}
	var filter string
	switch strings.Count(namespace, ".") {
	case 0:
		filter = namespace + ".*.*." + wildcardPositions(6)
	case 1:
		filter = namespace + ".*." + wildcardPositions(6)
	default:
		return "", errors.New("predicate namespace must contain one or two tokens")
	}
	if err := natsclient.ValidateKVWildcardFilter(filter); err != nil {
		return "", err
	}
	return filter, nil
}

// predicateMembershipFromKey reads a PREDICATE_INDEX key, predicate3.entity6.
func predicateMembershipFromKey(key string) (predicate, entityID string, ok bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 9 {
		return "", "", false
	}
	predicate = strings.Join(parts[:3], ".")
	entityID = strings.Join(parts[3:], ".")
	if _, err := vocabulary.ParsePredicate(predicate); err != nil {
		return "", "", false
	}
	if err := semtypes.ValidateEntityID(entityID); err != nil {
		return "", "", false
	}
	return predicate, entityID, true
}

// predicateSummaries counts the distinct entities of each predicate in keys, in lexical
// order of predicate.
func predicateSummaries(keys []string) []graph.PredicateSummary {
	entities := make(map[string]map[string]struct{})
	for _, key := range keys {
		predicate, entityID, ok := predicateMembershipFromKey(key)
		if !ok {
			continue
		}
		if entities[predicate] == nil {
			entities[predicate] = make(map[string]struct{})
		}
		entities[predicate][entityID] = struct{}{}
	}
	summaries := make([]graph.PredicateSummary, 0, len(entities))
	for predicate, members := range entities {
		summaries = append(summaries, graph.PredicateSummary{Predicate: predicate, EntityCount: len(members)})
	}
	slices.SortFunc(summaries, func(a, b graph.PredicateSummary) int { return strings.Compare(a.Predicate, b.Predicate) })
	return summaries
}

func invalidPredicateQuery(err error) error {
	return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest,
		fmt.Errorf("invalid predicate query: %w", err))
}

// handleQueryByName answers the entities carrying a name, case-insensitively, ranked:
// exact-case first, then label-predicate salience, then entity ID, one match per entity,
// then the limit. An unknown name is an empty answer, not an error.
func (c *Component) handleQueryByName(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var req struct {
		Name  string `json:"name"`
		Limit int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, invalidRequest(err)
	}
	if req.Name == "" {
		return nil, invalidRequest(errors.New("empty name"))
	}
	prefix := nameIndexKey(req.Name) + "."
	if err := validateKVPrefix(prefix); err != nil {
		return nil, invalidRequest(err)
	}
	revision, err := c.queryRevision(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := c.nameBucket.KeysByPrefix(ctx, prefix)
	if err != nil {
		return nil, internalError(err)
	}
	// Deduplicate before the budget, so a repeated key cannot exhaust it.
	slices.Sort(keys)
	keys = slices.Compact(keys)
	if len(keys) > nameQueryMaxHydration {
		return nil, errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeResourceExhausted,
			fmt.Errorf("name lookup exceeds read budget: %d memberships > %d", len(keys), nameQueryMaxHydration))
	}
	items := make([]graph.NameIndexItem, 0, len(keys))
	for _, key := range keys {
		entityID, predicate, ok := entityAndPredicate(strings.TrimPrefix(key, prefix))
		if !ok {
			c.logger.Debug("name query: skipping malformed key", slog.String("key", key))
			continue
		}
		entry, err := c.nameBucket.Get(ctx, key)
		if natsclient.IsKVNotFoundError(err) {
			continue // deleted since the listing
		}
		if err != nil {
			return nil, internalError(err)
		}
		var v nameCompositeValue
		if err := json.Unmarshal(entry.Value, &v); err != nil {
			c.logger.Debug("name query: skipping malformed value", slog.String("key", key), slog.Any("error", err))
			continue
		}
		items = append(items, graph.NameIndexItem{EntityID: entityID, Name: v.Name, Predicate: predicate, Priority: v.Priority})
	}
	return reply(revision, graph.NameData{Matches: rankNameMatches(items, req.Name, req.Limit)})
}

// nameRank pairs a match with its label-predicate priority for ordering.
type nameRank struct {
	match    graph.NameMatch
	priority int
}

// nameRankCompare is the order of name matches: exact case first, then lower priority
// (higher salience), then entity ID.
func nameRankCompare(a, b nameRank) int {
	if a.match.ExactCase != b.match.ExactCase {
		if a.match.ExactCase {
			return -1
		}
		return 1
	}
	if a.priority != b.priority {
		return a.priority - b.priority
	}
	return strings.Compare(a.match.EntityID, b.match.EntityID)
}

// rankNameMatches keeps each entity's best-ranked item, orders them by nameRankCompare and
// applies limit (zero or less: no limit).
func rankNameMatches(items []graph.NameIndexItem, query string, limit int) []graph.NameMatch {
	best := make(map[string]nameRank)
	for _, it := range items {
		candidate := nameRank{
			match: graph.NameMatch{
				EntityID: it.EntityID, MatchedName: it.Name, Predicate: it.Predicate, ExactCase: it.Name == query,
			},
			priority: it.Priority,
		}
		if current, seen := best[it.EntityID]; !seen || nameRankCompare(candidate, current) < 0 {
			best[it.EntityID] = candidate
		}
	}
	ranked := make([]nameRank, 0, len(best))
	for _, r := range best {
		ranked = append(ranked, r)
	}
	slices.SortFunc(ranked, nameRankCompare)
	matches := make([]graph.NameMatch, 0, len(ranked))
	for _, r := range ranked {
		matches = append(matches, r.match)
	}
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// validateKVPrefix refuses a key prefix whose listing filter, prefix + ">", is not a valid
// NATS KV filter.
func validateKVPrefix(prefix string) error {
	return natsclient.ValidateKVWildcardFilter(prefix + ">")
}
