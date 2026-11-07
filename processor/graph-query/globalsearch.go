package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/query"
	"github.com/c360studio/semengine/pkg/errs"
)

// Global search's strategies (design D1, D4): the names a reply's strategy field carries.
const (
	strategyEntity   = "entity"
	strategyPath     = "path"
	strategyTemporal = "temporal"
	strategySpatial  = "spatial"
)

// codeTextSearchUnsupported refuses a question only the text search could answer, which this build of
// graph-query does not have (design D2). Its class is invalid: asking again unchanged will not help.
// It is unexported, since no consumer should build on it: step 5 brings the text search and removes it.
const codeTextSearchUnsupported = "text_search_unsupported"

// The path strategy's walk (design D4): the pin's depth and node limit, both directions, every
// predicate, since the keyword tier names none.
const (
	globalPathDepth = 3
	globalPathNodes = 100
)

// globalIndexLimit is the most results the temporal and spatial strategies ask for (design D4): the
// pin's.
const globalIndexLimit = 100

// globalSearchSubjects are the verbs global search reads: the alias and entity verbs through the
// resolver, the index's edge verbs for the walk, and the time-range and bounds verbs.
type globalSearchSubjects struct {
	alias, entity, outgoing, incoming, temporal, spatial string
}

// globalSearch answers a question with the one strategy its hints choose (design D1, D4). A request
// is one input however many requests it makes, and at most one error: ask and brokenReply count
// theirs, and a classification that could not be read counts here. A caller's invalid_request, a
// responder's refusal and codeTextSearchUnsupported count none.
func (c *Component) globalSearch(subjects globalSearchSubjects) queryHandler {
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
			return c.globalEntity(ctx, subjects, route.reference, request.MinRevision)
		case strategyPath:
			return c.globalPath(ctx, subjects, route.reference, request.MinRevision)
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

// resolveGlobal reads a question's reference through the shared resolver, asking the alias verb with
// the request's min_revision. A reference that names no entity is codeTextSearchUnsupported: it is no
// answer to the question, which the text search would read as words (design D3).
func (c *Component) resolveGlobal(ctx context.Context, subjects globalSearchSubjects, reference string, minRevision uint64) (string, uint64, error) {
	aliasRequest, err := json.Marshal(struct {
		Alias string `json:"alias"`
		graph.MinRevisionField
	}{reference, graph.MinRevisionField{MinRevision: minRevision}})
	if err != nil {
		return "", 0, internalError(err)
	}
	id, revision, err := c.resolveReference(ctx, subjects.alias, aliasRequest, reference)
	if err != nil {
		return "", 0, err
	}
	if id == "" {
		return "", 0, textSearchUnsupported()
	}
	return id, revision, nil
}

// globalEntity answers with the entity verb's reply bytes for the resolved reference, under the alias
// reply's revision. An entity_not_found from the entity verb (a valid ID with no record, or a holder
// since deleted) falls through as resolveGlobal's miss does; any other refusal reaches the caller.
func (c *Component) globalEntity(ctx context.Context, subjects globalSearchSubjects, reference string, minRevision uint64) ([]byte, error) {
	id, revision, err := c.resolveGlobal(ctx, subjects, reference, minRevision)
	if err != nil {
		return nil, err
	}
	entity, err := c.readEntity(ctx, subjects.entity, id)
	if err != nil {
		return nil, fallThroughNotFound(err)
	}
	if len(entity) == 0 {
		// Entity is omitted when empty, so an empty reply would answer with no entity at all.
		return nil, c.brokenReply(subjects.entity, errors.New("is empty"))
	}
	return envelope(c, subjects.entity, revision, graph.GlobalSearchData{Strategy: strategyEntity, Entity: json.RawMessage(entity)})
}

// globalPath answers with step 2's walk from the resolved start, as pathSearch answers it for the
// same request, under the walk's revision. A start with no record falls through as globalEntity's
// does.
func (c *Component) globalPath(ctx context.Context, subjects globalSearchSubjects, reference string, minRevision uint64) ([]byte, error) {
	id, _, err := c.resolveGlobal(ctx, subjects, reference, minRevision)
	if err != nil {
		return nil, err
	}
	if err := c.readPathStart(ctx, subjects.entity, id); err != nil {
		return nil, fallThroughNotFound(err)
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
// included, under that reply's revision (design D4); a reply missing results or truncated is
// refused, one error. Until a responder serves the verb (change 7), ask refuses transient internal
// naming it and counts the error.
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
// included, under that reply's revision (design D4); a reply missing results or truncated is
// refused, one error. Until a responder serves the verb (change 7), ask refuses transient internal
// naming it and counts the error.
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

// fallThroughNotFound turns the entity verb's entity_not_found into codeTextSearchUnsupported and
// returns any other error unchanged.
func fallThroughNotFound(err error) error {
	var refusal *errs.ClassifiedError
	if errors.As(err, &refusal) && refusal.Code == graph.ErrorCodeEntityNotFound {
		return textSearchUnsupported()
	}
	return err
}

// globalRoute is the strategy that answers a question and what that strategy reads: the reference
// for entity and path (the question, or the path's start), the time range for temporal and the area
// for spatial.
type globalRoute struct {
	strategy  string
	reference string
	timeRange *query.TimeRange
	area      *query.SpatialBounds
}

// questionHints is the typed read of a keyword classification's Options (design D1).
type questionHints struct {
	path      bool // path words, with or without a start
	pathStart string
	timeRange *query.TimeRange
	area      *query.SpatialBounds
	// unhonoured is true when the question holds a hint no strategy honours: a count or other
	// aggregation, a ranking or its limit, a similarity, or a path predicate.
	unhonoured bool
}

// decodeGlobalSearchRequest decodes strictly, refusing any key but query and min_revision by name
// (design D5), and a blank query.
func decodeGlobalSearchRequest(data []byte) (graph.GlobalSearchRequest, error) {
	var request graph.GlobalSearchRequest
	if err := decodeStrict(data, &request); err != nil {
		return request, err
	}
	if strings.TrimSpace(request.Query) == "" {
		return request, errors.New("query is blank")
	}
	return request, nil
}

// routeGlobalSearch chooses the strategy that answers question (design D1). A one-word question is an
// entity reference, read before any hint: the keyword patterns find trigger words inside IDs and
// aliases (today, zone-a, link, max, most). Any other question is classified once with the keyword
// tier. Every refusal comes before any downstream request: two filters or an impossible number is
// invalid_request, a question only the text search could answer is codeTextSearchUnsupported, and a
// classification ended with ctx is transient internal, which the caller counts as an error.
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
		case "aggregation_type", "aggregation_field", "ranking_intent", "limit", "use_embeddings", "path_predicates":
			hints.unhonoured = true
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

// chooseRoute is design D1's table for a question of more than one word. Two filters are refused
// first, since the caller can fix the question; otherwise a strategy answers only when its hint is the
// question's only hint, and a question with no hint is the text search's.
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

	count := len(filters)
	if hints.path && hints.pathStart == "" {
		count++
	}
	switch {
	case hints.unhonoured || count > 1:
		return globalRoute{}, textSearchUnsupported()
	case hints.pathStart != "":
		return globalRoute{strategy: strategyPath, reference: hints.pathStart}, nil
	case hints.path:
		// Path words with no start: nothing to walk from.
		return globalRoute{}, textSearchUnsupported()
	case hints.timeRange != nil:
		return globalRoute{strategy: strategyTemporal, timeRange: hints.timeRange}, nil
	case hints.area != nil:
		return globalRoute{strategy: strategySpatial, area: hints.area}, nil
	}
	return globalRoute{}, textSearchUnsupported()
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

func textSearchUnsupported() error {
	return errs.ClassifiedCode(errs.ErrorInvalid, codeTextSearchUnsupported,
		errors.New("this question needs the text search, which this build of graph-query does not have"))
}
