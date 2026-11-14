package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/query"
	"github.com/c360studio/semengine/pkg/errs"
)

// The kinds of hint a global search reply lists, in GlobalSearchHint.Hint.
const (
	hintEntity         = "entity"
	hintPath           = "path"
	hintTimeRange      = "time_range"
	hintArea           = "area"
	hintAggregation    = "aggregation"
	hintSimilarity     = "similarity"
	hintRanking        = "ranking"
	hintClassification = "classification"
)

// Why a hint global search never honours is not honoured. Each is said in the reply, so a caller
// knows the matches answer the question's words and not that part of it.
const (
	reasonNoStart        = "the question names no start to walk from"
	reasonPathPredicates = "a walk follows every predicate, not only the question's"
	reasonTimeRange      = "global search does not filter by time"
	reasonArea           = "global search does not filter by place"
	reasonAggregation    = "global search computes no aggregation"
	reasonSimilarity     = "global search matches words, not meaning"
	reasonRanking        = "global search orders matches by how well their words match"
	reasonClassification = "the question classifier refused the question; it is searched for its words"
)

// A verified start's walk: a fixed depth and node limit, both directions, every predicate, since
// the keyword classifier names none.
const (
	globalPathDepth = 3
	globalPathNodes = 100
)

// globalSearchSubjects are the verbs global search reads: the word search and batch verbs for its
// matches, the alias and entity verbs through the resolver, and the index's edge verbs for a walk.
type globalSearchSubjects struct {
	lexical, batch, alias, entity, outgoing, incoming string
}

// globalSearch answers every question that decodes with a word search (globalText), and lists what
// the question asked for beyond its words as hints. No hint refuses the question: a hint the
// framework cannot verify is reported not honoured, with its reason. The question is classified
// before any request, with no request of its own; a classification that cannot be read refuses
// transient internal and counts one error. verb is the verb's name in graph.QueryVerbs, which the
// truncation counter and the Warn carry.
func (c *Component) globalSearch(verb string, subjects globalSearchSubjects) queryHandler {
	return func(ctx context.Context, data []byte) ([]byte, error) {
		c.lifecycle.CountInput(len(data))
		request, err := decodeGlobalSearchRequest(data)
		if err != nil {
			return nil, invalidRequest(err)
		}
		route, err := routeGlobalSearch(ctx, request.Query)
		if err != nil {
			c.lifecycle.CountError()
			return nil, err
		}
		return c.globalText(ctx, verb, subjects, request, route)
	}
}

// verifiedHint is what verifying a route's reference found: the hint it settles, first in the
// reply's list, and the entity's bytes or the walk when the hint is honoured.
type verifiedHint struct {
	hint   graph.GlobalSearchHint
	entity json.RawMessage
	path   *graph.PathSearchData
}

// verify reads route's reference through the shared resolver, asking the alias verb with
// minRevision: for an entity hint the entity verb's bytes, for a path hint the walk from it, as
// pathSearch walks for the same start. A reference that names no entity, and any refusal or failure
// of a read, leaves the hint not honoured, carrying the refusal's code and message, and never
// refuses the reply: the word search has answered. A failure that is graph-query's own (no responder,
// a reply it cannot use) still counts its error where it is found. A route with no reference has
// nothing to verify.
func (c *Component) verify(ctx context.Context, subjects globalSearchSubjects, route globalRoute, minRevision uint64) (verifiedHint, bool) {
	if route.strategy == "" {
		return verifiedHint{}, false
	}
	hint := graph.GlobalSearchHint{Hint: route.strategy, Detail: route.reference}
	notHonoured := func(err error) (verifiedHint, bool) {
		refusal := &errs.ClassifiedError{Code: graph.ErrorCodeInternal}
		errors.As(err, &refusal)
		hint.Reason, hint.Code = err.Error(), refusal.Code
		return verifiedHint{hint: hint}, true
	}
	id, err := c.resolveGlobal(ctx, subjects, route.reference, minRevision)
	if err != nil {
		return notHonoured(err)
	}
	if id == "" {
		return notHonoured(entityNotFound(route.reference))
	}
	if route.strategy == hintEntity {
		entity, err := c.readEntity(ctx, subjects.entity, id)
		if err == nil && !json.Valid(entity) {
			// The bytes go into the reply as they came, so bytes that are not JSON would break it.
			err = c.brokenReply(subjects.entity, errors.New("is not JSON"))
		}
		if err != nil {
			return notHonoured(err)
		}
		hint.Honoured = true
		return verifiedHint{hint: hint, entity: entity}, true
	}
	if err := c.readPathStart(ctx, subjects.entity, id); err != nil {
		return notHonoured(err)
	}
	walk := graph.PathSearchRequest{
		StartEntity: id, Direction: directionBoth, MaxDepth: globalPathDepth, MaxNodes: globalPathNodes,
		MinRevisionField: graph.MinRevisionField{MinRevision: minRevision},
	}
	found, _, err := walkPaths(ctx, walk, c.indexHop(subjects.outgoing, subjects.incoming, minRevision))
	if err != nil {
		return notHonoured(err)
	}
	hint.Honoured = true
	return verifiedHint{hint: hint, path: &found}, true
}

// resolveGlobal reads a reference through the shared resolver, asking the alias verb with
// minRevision. A reference that names no entity returns the ID "".
func (c *Component) resolveGlobal(ctx context.Context, subjects globalSearchSubjects, reference string, minRevision uint64) (string, error) {
	aliasRequest, err := json.Marshal(struct {
		Alias string `json:"alias"`
		graph.MinRevisionField
	}{reference, graph.MinRevisionField{MinRevision: minRevision}})
	if err != nil {
		return "", internalError(err)
	}
	id, _, err := c.resolveReference(ctx, subjects.alias, aliasRequest, reference)
	return id, err
}

// globalRoute is what classifying a question found: the reference to verify and the kind of hint it
// is, strategy "entity" for a one-word question or "path" for a walk's start, "" when there is none;
// and the hints no read can verify, each not honoured, in the order the reply lists them.
type globalRoute struct {
	strategy  string
	reference string
	hints     []graph.GlobalSearchHint
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

// routeGlobalSearch classifies question. A one-word question is an entity reference, and is not
// classified: the keyword patterns find trigger words inside IDs and aliases (today, zone-a, link,
// max, most). Any other question is classified once with the keyword classifier, and a refusal of
// class invalid (two places, an impossible number) is a not-honoured classification hint carrying
// its text. Only a classification that cannot be read is refused: one that ended with ctx, a failure
// of another class, or hints of a shape global search does not know, each transient internal.
func routeGlobalSearch(ctx context.Context, question string) (globalRoute, error) {
	if reference := strings.TrimSpace(question); !strings.ContainsFunc(reference, unicode.IsSpace) {
		return globalRoute{strategy: hintEntity, reference: reference}, nil
	}
	result, err := query.NewClassifierChain(query.NewKeywordClassifier(), nil).ClassifyQuery(ctx, question)
	if err != nil {
		if errs.IsInvalid(err) {
			return globalRoute{hints: []graph.GlobalSearchHint{{
				Hint: hintClassification, Detail: err.Error(), Reason: reasonClassification, Code: graph.ErrorCodeInvalidRequest,
			}}}, nil
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
	return listHints(hints), nil
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

// listHints turns a classification's hints into the route: a path's start is the reference to
// verify, unless the question also names path predicates, which a walk would drop; every other hint
// is listed not honoured, since global search has no verified answer for it.
func listHints(hints questionHints) globalRoute {
	var route globalRoute
	notHonoured := func(hint, detail, reason string) {
		route.hints = append(route.hints, graph.GlobalSearchHint{Hint: hint, Detail: detail, Reason: reason})
	}
	switch {
	case hints.pathStart != "" && hints.pathPredicates:
		notHonoured(hintPath, hints.pathStart, reasonPathPredicates)
	case hints.pathStart != "":
		route.strategy, route.reference = hintPath, hints.pathStart
	case hints.path || hints.pathPredicates:
		notHonoured(hintPath, "", reasonNoStart)
	}
	if span := hints.timeRange; span != nil {
		notHonoured(hintTimeRange, span.Start.Format(time.RFC3339)+" to "+span.End.Format(time.RFC3339), reasonTimeRange)
	}
	if area := hints.area; area != nil {
		notHonoured(hintArea, fmt.Sprintf("north %g, south %g, east %g, west %g", area.North, area.South, area.East, area.West), reasonArea)
	}
	if hints.aggregation != "" {
		operation := hints.aggregation
		if hints.aggregationField != "" {
			operation += " of " + hints.aggregationField
		}
		notHonoured(hintAggregation, operation, reasonAggregation)
	}
	if hints.similarity {
		notHonoured(hintSimilarity, "", reasonSimilarity)
	}
	if hints.ranking {
		notHonoured(hintRanking, "", reasonRanking)
	}
	return route
}
