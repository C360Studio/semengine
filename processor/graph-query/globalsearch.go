package graphquery

import (
	"context"
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

// routeGlobalSearch classifies question once with the keyword tier and chooses the strategy that
// answers it (design D1). Every refusal comes before any downstream request: two filters or an
// impossible number is invalid_request, a question only the text search could answer is
// codeTextSearchUnsupported, and a classification ended with ctx is transient internal, which the
// caller counts as an error.
func routeGlobalSearch(ctx context.Context, question string) (globalRoute, error) {
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
	return chooseRoute(hints, question)
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

// chooseRoute is design D1's table. Two filters are refused first, since the caller can fix the
// question; otherwise a strategy answers only when its hint is the question's only hint.
func chooseRoute(hints questionHints, question string) (globalRoute, error) {
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
	reference := strings.TrimSpace(question)
	if strings.ContainsFunc(reference, unicode.IsSpace) {
		return globalRoute{}, textSearchUnsupported()
	}
	return globalRoute{strategy: strategyEntity, reference: reference}, nil
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
