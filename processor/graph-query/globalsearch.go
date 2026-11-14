package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/query"
	"github.com/c360studio/semengine/pkg/errs"
)

// Global search's strategies: the names a reply's strategy field carries.
const (
	strategyEntity   = "entity"
	strategyPath     = "path"
	strategyTemporal = "temporal"
	strategySpatial  = "spatial"
	strategyText     = "text"
)

// The hints the text strategy answers past, named in its reply's unhonoured_hints in this order.
const (
	hintPath       = "path"
	hintSimilarity = "similarity"
	hintRanking    = "ranking"
)

// The path strategy's walk: a fixed depth and node limit, both directions, every predicate, since
// the keyword tier names none.
const (
	globalPathDepth = 3
	globalPathNodes = 100
)

// globalIndexLimit is the most results the temporal and spatial strategies ask for.
const globalIndexLimit = 100

// globalSearchSubjects are the verbs global search reads: the alias and entity verbs through the
// resolver, the index's edge verbs for the walk, the time-range and bounds verbs, and the word
// search and batch verbs for the text strategy.
type globalSearchSubjects struct {
	alias, entity, outgoing, incoming, temporal, spatial, lexical, batch string
}

// globalSearch answers a question with the one strategy its hints choose. A request is one input
// however many requests it makes, and at most one error: ask and brokenReply count theirs, the text
// strategy counts an answer past unreadable records or a reply it cannot fit, and a classification
// that could not be read counts here. A caller's invalid_request, a responder's refusal and a named
// start's entity_not_found count none. verb is the verb's name in graph.QueryVerbs, which the text
// strategy's truncation counter and Warn carry.
func (c *Component) globalSearch(verb string, subjects globalSearchSubjects) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		request, err := decodeGlobalSearchRequest(data)
		if err != nil {
			return nil, invalidRequest(err)
		}
		route, err := routeGlobalSearch(ctx, request.Query)
		if err != nil {
			var refusal *errs.ClassifiedError
			if errors.As(err, &refusal) && refusal.Code == graph.ErrorCodeInternal {
				c.lifecycle.CountError()
			}
			return nil, err
		}
		switch route.strategy {
		case strategyEntity:
			return c.globalEntity(ctx, verb, subjects, route.reference, request)
		case strategyPath:
			return c.globalPath(ctx, subjects, route.reference, request.MinRevision)
		case strategyText:
			return c.globalText(ctx, verb, subjects, request, route.unhonoured)
		case strategyTemporal:
			return c.globalTemporal(ctx, subjects.temporal, *route.timeRange, request.MinRevision)
		case strategySpatial:
			return c.globalSpatial(ctx, subjects.spatial, *route.area, request.MinRevision)
		default:
			c.lifecycle.CountError()
			return nil, internalError(fmt.Errorf("the question was routed to %q, which global search does not answer", route.strategy))
		}
	}
}

// resolveGlobal reads a question's reference through the shared resolver, asking the alias verb
// with the request's min_revision. A reference that names no entity returns the ID "", for the
// caller to decide: a one-word reference is searched as text, a named start is entity_not_found.
func (c *Component) resolveGlobal(ctx context.Context, subjects globalSearchSubjects, reference string, minRevision uint64) (string, uint64, error) {
	aliasRequest, err := json.Marshal(struct {
		Alias string `json:"alias"`
		graph.MinRevisionField
	}{reference, graph.MinRevisionField{MinRevision: minRevision}})
	if err != nil {
		return "", 0, internalError(err)
	}
	return c.resolveReference(ctx, subjects.alias, aliasRequest, reference)
}

// globalEntity answers with the entity verb's reply bytes for the resolved reference, under the
// alias reply's revision. A reference that names no entity, or an entity_not_found from the entity
// verb (a valid ID with no record, or a holder since deleted), is searched as text; any other
// refusal reaches the caller.
func (c *Component) globalEntity(ctx context.Context, verb string, subjects globalSearchSubjects, reference string, request graph.GlobalSearchRequest) ([]byte, error) {
	id, revision, err := c.resolveGlobal(ctx, subjects, reference, request.MinRevision)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return c.globalText(ctx, verb, subjects, request, nil)
	}
	entity, err := c.readEntity(ctx, subjects.entity, id)
	if isEntityNotFound(err) {
		return c.globalText(ctx, verb, subjects, request, nil)
	}
	if err != nil {
		return nil, err
	}
	if len(entity) == 0 {
		// Entity is omitted when empty, so an empty reply would answer with no entity at all.
		return nil, c.brokenReply(subjects.entity, errors.New("is empty"))
	}
	return envelope(c, subjects.entity, revision, graph.GlobalSearchData{Strategy: strategyEntity, Entity: json.RawMessage(entity)})
}

// globalPath answers with the path search's walk from the resolved start, as pathSearch answers it
// for the same request, under the walk's revision. A start that names no entity is
// entity_not_found, as entityByAlias answers it, and a start with no record is the entity verb's
// refusal as it came: a text search would drop the walk.
func (c *Component) globalPath(ctx context.Context, subjects globalSearchSubjects, reference string, minRevision uint64) ([]byte, error) {
	id, _, err := c.resolveGlobal(ctx, subjects, reference, minRevision)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, entityNotFound(reference)
	}
	if err := c.readPathStart(ctx, subjects.entity, id); err != nil {
		return nil, err
	}
	walk := graph.PathSearchRequest{
		StartEntity: id, Direction: directionBoth, MaxDepth: globalPathDepth, MaxNodes: globalPathNodes,
		MinRevisionField: graph.MinRevisionField{MinRevision: minRevision},
	}
	found, revision, err := walkPaths(ctx, walk, c.indexHop(subjects.outgoing, subjects.incoming, minRevision))
	if err != nil {
		return nil, err
	}
	return envelope(c, subjects.outgoing, revision, graph.GlobalSearchData{Strategy: strategyPath, Path: &found})
}

// globalTemporal answers with what the time-range verb returns for the question's range, truncated
// included, under that reply's revision; a reply missing results or truncated is refused, one
// error. Until a responder serves the verb, ask refuses transient internal naming it and counts the
// error.
func (c *Component) globalTemporal(ctx context.Context, subject string, span query.TimeRange, minRevision uint64) ([]byte, error) {
	request, err := json.Marshal(graph.TemporalRangeRequest{
		StartTime: span.Start, EndTime: span.End, Limit: globalIndexLimit,
		MinRevisionField: graph.MinRevisionField{MinRevision: minRevision},
	})
	if err != nil {
		return nil, internalError(err)
	}
	raw, err := c.ask(ctx, subject, request)
	if err != nil {
		return nil, err
	}
	reply, err := decodeIndexReply[graph.TemporalData](c, subject, raw)
	if err != nil {
		return nil, err
	}
	if reply.Data.Results == nil {
		// Results absent would read as nothing changed.
		return nil, c.brokenReply(subject, errors.New("holds no results"))
	}
	if truncatedAbsent(raw) {
		return nil, c.brokenReply(subject, errors.New("holds no truncated"))
	}
	return envelope(c, subject, reply.IndexedRevision, graph.GlobalSearchData{Strategy: strategyTemporal, Temporal: &reply.Data})
}

// globalSpatial answers with what the bounds verb returns for the question's area, truncated
// included, under that reply's revision; a reply missing results or truncated is refused, one
// error. Until a responder serves the verb, ask refuses transient internal naming it and counts the
// error.
func (c *Component) globalSpatial(ctx context.Context, subject string, area query.SpatialBounds, minRevision uint64) ([]byte, error) {
	request, err := json.Marshal(graph.SpatialBoundsRequest{
		North: area.North, South: area.South, East: area.East, West: area.West, Limit: globalIndexLimit,
		MinRevisionField: graph.MinRevisionField{MinRevision: minRevision},
	})
	if err != nil {
		return nil, internalError(err)
	}
	raw, err := c.ask(ctx, subject, request)
	if err != nil {
		return nil, err
	}
	reply, err := decodeIndexReply[graph.SpatialData](c, subject, raw)
	if err != nil {
		return nil, err
	}
	if reply.Data.Results == nil {
		// Results absent would read as nothing there.
		return nil, c.brokenReply(subject, errors.New("holds no results"))
	}
	if truncatedAbsent(raw) {
		return nil, c.brokenReply(subject, errors.New("holds no truncated"))
	}
	return envelope(c, subject, reply.IndexedRevision, graph.GlobalSearchData{Strategy: strategySpatial, Spatial: &reply.Data})
}

// truncatedAbsent reports whether a time-range or bounds reply's data holds no truncated, or a null
// one, which the bool reads as false: a cut-off answer would look complete.
func truncatedAbsent(raw []byte) bool {
	var keys struct {
		Data struct {
			Truncated *bool `json:"truncated"`
		} `json:"data"`
	}
	return json.Unmarshal(raw, &keys) != nil || keys.Data.Truncated == nil
}

// isEntityNotFound reports whether err is the entity verb's entity_not_found.
func isEntityNotFound(err error) bool {
	var refusal *errs.ClassifiedError
	return errors.As(err, &refusal) && refusal.Code == graph.ErrorCodeEntityNotFound
}

// globalRoute is the strategy that answers a question and what that strategy reads: the reference
// for entity and path (the question, or the path's start), the time range for temporal, the area
// for spatial, and for text the hints it answers past, in hintPath, hintSimilarity, hintRanking order.
type globalRoute struct {
	strategy   string
	reference  string
	timeRange  *query.TimeRange
	area       *query.SpatialBounds
	unhonoured []string
}

// questionHints is the typed read of a keyword classification's Options.
type questionHints struct {
	path           bool // path words, with or without a start
	pathStart      string
	pathPredicates bool
	timeRange      *query.TimeRange
	area           *query.SpatialBounds
	// aggregation is a count or other aggregation's operation (count, sum, avg, min or max), and
	// aggregationField the field it reads, if the classifier found one.
	aggregation, aggregationField string
	similarity                    bool
	ranking                       bool // a ranking or its limit
}

// decodeGlobalSearchRequest decodes strictly, refusing any key but query, level, limit and
// min_revision by name, a blank query, a level outside 0 to 2 and a limit other than 0 or 1 to
// graph.MaxSimilarityLimit. LevelPrefix owns which levels exist.
func decodeGlobalSearchRequest(data []byte) (graph.GlobalSearchRequest, error) {
	var request graph.GlobalSearchRequest
	if err := decodeStrict(data, &request); err != nil {
		return request, err
	}
	if strings.TrimSpace(request.Query) == "" {
		return request, errors.New("query is blank")
	}
	if _, err := clustering.LevelPrefix(request.Level); err != nil {
		return request, fmt.Errorf("level: %w", err)
	}
	if err := checkSearchLimit(request.Limit); err != nil {
		return request, err
	}
	return request, nil
}

// routeGlobalSearch chooses the strategy that answers question. A one-word question is an entity
// reference, read before any hint: the keyword patterns find trigger words inside IDs and aliases
// (today, zone-a, link, max, most). Any other question is classified once with the keyword tier.
// Every refusal comes before any downstream request: two filters, an impossible number, an
// aggregation or a text-routed hint beside a filter is invalid_request, and a classification ended
// with ctx is transient internal, which the caller counts as an error.
func routeGlobalSearch(ctx context.Context, question string) (globalRoute, error) {
	if reference := strings.TrimSpace(question); !strings.ContainsFunc(reference, unicode.IsSpace) {
		return globalRoute{strategy: strategyEntity, reference: reference}, nil
	}
	result, err := query.NewClassifierChain(query.NewKeywordClassifier(), nil).ClassifyQuery(ctx, question)
	if err != nil {
		if errs.IsInvalid(err) {
			return globalRoute{}, invalidRequest(err)
		}
		return globalRoute{}, internalError(err)
	}
	if result == nil {
		// The chain answers nothing only once ctx has ended (graph/query/classifier_chain.go:91-93):
		// never read as a question with no hints.
		return globalRoute{}, internalError(fmt.Errorf("the question's classification ended with the request: %w", ctx.Err()))
	}
	hints, err := readHints(result.Options)
	if err != nil {
		return globalRoute{}, internalError(err)
	}
	return chooseRoute(hints)
}

// readHints reads the keyword tier's Options (graph/query/classifier_chain.go:182-222) into their
// types. A key it does not know, or a value of another type, is refused rather than dropped: a hint
// read as absent would answer a different question.
func readHints(options map[string]any) (questionHints, error) {
	var hints questionHints
	for key, value := range options {
		ok := true
		switch key {
		case "path_intent":
			hints.path, ok = value.(bool)
		case "path_start_node":
			hints.pathStart, ok = value.(string)
		case "time_range":
			hints.timeRange, ok = value.(*query.TimeRange)
			ok = ok && hints.timeRange != nil
		case "geo_bounds":
			hints.area, ok = value.(*query.SpatialBounds)
			ok = ok && hints.area != nil
		case "aggregation_type":
			hints.aggregation, ok = value.(string)
		case "aggregation_field":
			hints.aggregationField, ok = value.(string)
		case "use_embeddings":
			hints.similarity, ok = value.(bool)
		case "ranking_intent":
			var ranking bool
			ranking, ok = value.(bool)
			hints.ranking = hints.ranking || ranking
		case "limit":
			_, ok = value.(int)
			hints.ranking = true
		case "path_predicates":
			var predicates []string
			predicates, ok = value.([]string)
			hints.pathPredicates = len(predicates) > 0
		default:
			return hints, fmt.Errorf("classification holds the hint %q, which global search does not read", key)
		}
		if !ok {
			return hints, fmt.Errorf("classification hint %q holds %T", key, value)
		}
	}
	if hints.pathStart != "" {
		hints.path = true
	}
	return hints, nil
}

// chooseRoute chooses the route for a question of more than one word. Two filters are refused
// first, since the caller can fix the question, then an aggregation, which no strategy computes. A
// filter (a path start, a time range or an area) answers only alone: beside a text-routed hint
// (path words with no start, a similarity or a ranking), or a start beside path predicates, it is
// refused naming both, since either strategy would drop the other. The rest is the text strategy's,
// each text-routed hint named.
func chooseRoute(hints questionHints) (globalRoute, error) {
	var filters []string
	if hints.pathStart != "" {
		filters = append(filters, "a path from "+hints.pathStart)
	}
	if hints.timeRange != nil {
		filters = append(filters, "a time range")
	}
	if hints.area != nil {
		filters = append(filters, "an area")
	}
	if len(filters) > 1 {
		return globalRoute{}, invalidRequest(errors.New(twoFilters(filters)))
	}
	if hints.aggregation != "" {
		operation := hints.aggregation
		if hints.aggregationField != "" {
			operation += " of " + hints.aggregationField
		}
		return globalRoute{}, invalidRequest(fmt.Errorf("the question asks for an aggregation (%s), which global search does not compute; ask without it", operation))
	}

	// textHints are the reply's names, named the refusal's words for the same hints.
	var textHints, named []string
	if (hints.path || hints.pathPredicates) && hints.pathStart == "" {
		textHints, named = append(textHints, hintPath), append(named, "path words with no start")
	}
	if hints.similarity {
		textHints, named = append(textHints, hintSimilarity), append(named, "a similarity")
	}
	if hints.ranking {
		textHints, named = append(textHints, hintRanking), append(named, "a ranking")
	}
	if hints.pathPredicates && hints.pathStart != "" {
		named = append(named, "path predicates")
	}
	if len(named) > 0 && len(filters) == 1 {
		return globalRoute{}, invalidRequest(fmt.Errorf("the question names %s beside %s, which no strategy answers together; ask with one", strings.Join(named, " and "), filters[0]))
	}

	switch {
	case hints.pathStart != "":
		return globalRoute{strategy: strategyPath, reference: hints.pathStart}, nil
	case hints.timeRange != nil:
		return globalRoute{strategy: strategyTemporal, timeRange: hints.timeRange}, nil
	case hints.area != nil:
		return globalRoute{strategy: strategySpatial, area: hints.area}, nil
	}
	return globalRoute{strategy: strategyText, unhonoured: textHints}, nil
}

// twoFilters names the filters a question holds together: "two filters, a path from x and a time
// range".
func twoFilters(filters []string) string {
	count := "two"
	if len(filters) == 3 {
		count = "three"
	}
	last := len(filters) - 1
	return fmt.Sprintf("the question names %s filters, %s and %s; ask with one", count, strings.Join(filters[:last], ", "), filters[last])
}
