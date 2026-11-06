package graphquery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/query"
	"github.com/c360studio/semengine/pkg/errs"
)

// The refusal texts global search sends, spelled here as the oracle rather than read from the code.
const (
	textTierRefusal = "this question needs the text search, which this build of graph-query does not have"
	pathAndTime     = "invalid request: the question names two filters, a path from pump-42 and a time range; ask with one"
)

// refusalOfRoute returns err as a classified refusal of class and code, failing the test otherwise.
func refusalOfRoute(t *testing.T, question string, err error, class errs.ErrorClass, code string) *errs.ClassifiedError {
	t.Helper()
	var refusal *errs.ClassifiedError
	if !errors.As(err, &refusal) {
		t.Fatalf("route(%q) error = %v, want a classified refusal", question, err)
	}
	if refusal.Class != class || refusal.Code != code {
		t.Fatalf("route(%q) refused %s %q (%v), want %s %q", question, refusal.Class, refusal.Code, err, class, code)
	}
	return refusal
}

// TestGlobalSearchRoutesByHints proves design D1's table through the keyword classifier, one row per
// table line and D1's five mixed-hint questions. It carries the pin's strategy reports
// (slice_e_contract_test.go:21-192 at 8b99efe9: each answer names the strategy that answered), renamed:
// pathrag is path, entity_lookup is entity. The options each question gets are in
// notes/probes/step4-pin-probe.log and step4-tree-probe.log.
func TestGlobalSearchRoutesByHints(t *testing.T) {
	answered := []struct {
		question  string
		strategy  string
		reference string
	}{
		{"what is connected to sensor-001", "path", "sensor-001"},
		// The tree's keyword tier reads a zone phrase as a path start and no predicate (PR #475); the
		// start names no entity here, which step 3.2's resolver sends to the text tier.
		{"the area of the plant", "path", "area-of"},
		{"what changed in the last 2 hours", "temporal", ""},
		{"what is within 5km of 40.7,-74.0", "spatial", ""},
		{"sensor-001", "entity", "sensor-001"},
		{"  acme.ops.robotics.gcs.drone.001\t", "entity", "acme.ops.robotics.gcs.drone.001"},
		// One token and no hint is an entity reference; one naming no entity is the resolver's to refuse.
		{"forklift", "entity", "forklift"},
	}
	for _, tc := range answered {
		route, err := routeGlobalSearch(t.Context(), tc.question)
		if err != nil {
			t.Errorf("route(%q) error = %v, want strategy %s", tc.question, err, tc.strategy)
			continue
		}
		if route.strategy != tc.strategy || route.reference != tc.reference {
			t.Errorf("route(%q) = %s %q, want %s %q", tc.question, route.strategy, route.reference, tc.strategy, tc.reference)
		}
		switch tc.strategy {
		case "temporal":
			if route.timeRange == nil || route.timeRange.End.Sub(route.timeRange.Start) != 2*time.Hour {
				t.Errorf("route(%q) time range = %+v, want two hours", tc.question, route.timeRange)
			}
		case "spatial":
			// The box is the classifier's; the route must carry it around the question's point.
			if a := route.area; a == nil || a.South >= 40.7 || a.North <= 40.7 || a.West >= -74.0 || a.East <= -74.0 {
				t.Errorf("route(%q) area = %+v, want a box around 40.7,-74.0", tc.question, route.area)
			}
		default:
			if route.timeRange != nil || route.area != nil {
				t.Errorf("route(%q) carries a time range %+v or area %+v", tc.question, route.timeRange, route.area)
			}
		}
	}

	refused := []struct {
		question string
		code     string
	}{
		// No hint and several tokens.
		{"show me sensor-001", "text_search_unsupported"},
		// Path words with no start.
		{"show the path", "text_search_unsupported"},
		// Path words with no start beside a time range: not two filters, and the time range is not
		// the only hint.
		{"what is related to it in the last 2 hours", "text_search_unsupported"},
		{"how many pumps are there", "text_search_unsupported"},
		{"top 5 sensors", "text_search_unsupported"},
		{"find sensors similar to pump-42", "text_search_unsupported"},
		// D1's five mixed-hint rows: a strategy that answered past a count, a ranking limit or a
		// similarity hint would answer a different question.
		{"how many sensors are connected to pump-42", "text_search_unsupported"},
		{"top 5 sensors connected to pump-42", "text_search_unsupported"},
		{"find sensors similar to pump-42 in the last 2 hours", "text_search_unsupported"},
		{"how many pumps changed in the last 2 hours", "text_search_unsupported"},
		{"how many pumps connected to pump-42 in the last 2 hours", "invalid_request"},
		// Two filters: the table's row checked first.
		{"what is connected to pump-42 in the last 2 hours", "invalid_request"},
	}
	for _, tc := range refused {
		route, err := routeGlobalSearch(t.Context(), tc.question)
		if err == nil {
			t.Errorf("route(%q) = %s %q, want refused %s", tc.question, route.strategy, route.reference, tc.code)
			continue
		}
		refusal := refusalOfRoute(t, tc.question, err, errs.ErrorInvalid, tc.code)
		if tc.code == "text_search_unsupported" && err.Error() != textTierRefusal {
			t.Errorf("route(%q) refusal = %q, want %q", tc.question, err.Error(), textTierRefusal)
		}
		if refusal.Detail != nil {
			t.Errorf("route(%q) refusal detail = %v, want none", tc.question, refusal.Detail)
		}
	}
}

// TestGlobalSearchRefusesTwoFilters proves that two or three of path, time range and area are refused
// invalid_request naming each, whatever other hints the question holds (design D1, spec "Two filters").
func TestGlobalSearchRefusesTwoFilters(t *testing.T) {
	cases := []struct {
		question string
		want     string
	}{
		{"what is connected to pump-42 in the last 2 hours", pathAndTime},
		{"how many pumps connected to pump-42 in the last 2 hours", pathAndTime},
		{
			"what changed in the last 2 hours within 5km of 40.7,-74.0",
			"invalid request: the question names two filters, a time range and an area; ask with one",
		},
		{
			"what is connected to pump-42 within 5km of 40.7,-74.0",
			"invalid request: the question names two filters, a path from pump-42 and an area; ask with one",
		},
		{
			"what is connected to pump-42 in the last 2 hours within 5km of 40.7,-74.0",
			"invalid request: the question names three filters, a path from pump-42, a time range and an area; ask with one",
		},
	}
	for _, tc := range cases {
		_, err := routeGlobalSearch(t.Context(), tc.question)
		refusalOfRoute(t, tc.question, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if err.Error() != tc.want {
			t.Errorf("route(%q) refusal = %q, want %q", tc.question, err.Error(), tc.want)
		}
	}
}

// TestGlobalSearchRefusesImpossibleNumbers proves the classifier's refusal of an out-of-range number
// reaches the caller as invalid_request naming the phrase (owner, #479; design D1).
func TestGlobalSearchRefusesImpossibleNumbers(t *testing.T) {
	cases := []struct {
		question string
		phrase   string
	}{
		{"what changed in the last 0 hours", `"last 0 hours": the number must be at least 1`},
		{"what is within 5km of 200,-74.0", `"within 5km of 200,-74.0": the latitude must be in [-90, 90]`},
		// An impossible number wins over two filters: the caller fixes the number first.
		{"what is connected to pump-42 in the last 0 days", `"last 0 days": the number must be at least 1`},
	}
	for _, tc := range cases {
		_, err := routeGlobalSearch(t.Context(), tc.question)
		refusalOfRoute(t, tc.question, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if !strings.HasPrefix(err.Error(), "invalid request: ") || !strings.HasSuffix(err.Error(), tc.phrase) {
			t.Errorf("route(%q) refusal = %q, want invalid request ending %q", tc.question, err.Error(), tc.phrase)
		}
	}
}

// TestGlobalSearchEndedClassification proves that a classification with no result and no error, which
// the chain returns once the request's context has ended (graph/query/classifier_chain.go:91-93), is
// refused transient internal, never read as a question with no hints (design D1).
func TestGlobalSearchEndedClassification(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// "sensor-001" would route to entity under a live context.
	route, err := routeGlobalSearch(ctx, "sensor-001")
	if err == nil {
		t.Fatalf("route under an ended context = %s %q, want refused", route.strategy, route.reference)
	}
	refusalOfRoute(t, "sensor-001", err, errs.ErrorTransient, graph.ErrorCodeInternal)
	const want = "internal error: the question's classification ended with the request: context canceled"
	if err.Error() != want {
		t.Errorf("refusal = %q, want %q", err.Error(), want)
	}
}

// TestGlobalSearchRefusesHintsItCannotRead proves the typed read fails closed (design D1): a hint
// global search does not know, or one of another type, is refused, never read as absent. The keyword
// tier sends neither; another tier's options would.
func TestGlobalSearchRefusesHintsItCannotRead(t *testing.T) {
	cases := []struct {
		options map[string]any
		want    string
	}{
		{map[string]any{"strategy": "semantic"}, `classification holds the hint "strategy", which global search does not read`},
		{map[string]any{"time_range": map[string]any{"start": "2026-11-06T00:00:00Z"}}, `classification hint "time_range" holds map[string]interface {}`},
		{map[string]any{"time_range": (*query.TimeRange)(nil)}, `classification hint "time_range" holds *query.TimeRange`},
		{map[string]any{"geo_bounds": query.SpatialBounds{}}, `classification hint "geo_bounds" holds query.SpatialBounds`},
		{map[string]any{"path_intent": "true"}, `classification hint "path_intent" holds string`},
		{map[string]any{"path_intent": true, "path_start_node": 42}, `classification hint "path_start_node" holds int`},
	}
	for _, tc := range cases {
		_, err := readHints(tc.options)
		if err == nil || err.Error() != tc.want {
			t.Errorf("readHints(%v) error = %v, want %q", tc.options, err, tc.want)
		}
	}
}

// TestGlobalSearchDecodesStrictly proves design D5: query and min_revision decode; a blank query, any
// other key (the pin's level, max_communities, summarize_threshold, include_*) by name, and bytes after
// the request are refused.
func TestGlobalSearchDecodesStrictly(t *testing.T) {
	request, err := decodeGlobalSearchRequest([]byte(`{"query":"what is connected to sensor-001","min_revision":7}`))
	if err != nil {
		t.Fatalf("decode = %v", err)
	}
	if request.Query != "what is connected to sensor-001" || request.MinRevision != 7 {
		t.Errorf("decode = %+v", request)
	}

	refused := []struct {
		data string
		want string
	}{
		{`{"query":"sensor-001","level":1}`, `json: unknown field "level"`},
		{`{"query":"sensor-001","max_communities":5}`, `json: unknown field "max_communities"`},
		{`{"query":"sensor-001","summarize_threshold":50}`, `json: unknown field "summarize_threshold"`},
		{`{"query":"sensor-001","include_summaries":true}`, `json: unknown field "include_summaries"`},
		{`{"query":"sensor-001","include_relationships":true}`, `json: unknown field "include_relationships"`},
		{`{"query":"sensor-001","include_sources":true}`, `json: unknown field "include_sources"`},
		{`{"query":" \t\n"}`, "query is blank"},
		{`{}`, "query is blank"},
		{`{"query":"sensor-001"}x`, "bytes after the request"},
	}
	for _, tc := range refused {
		_, err := decodeGlobalSearchRequest([]byte(tc.data))
		if err == nil || err.Error() != tc.want {
			t.Errorf("decode(%s) error = %v, want %q", tc.data, err, tc.want)
		}
	}
}

// FuzzDecodeGlobalSearchRequest checks that an accepted request holds a query that is not blank, and
// that routing any accepted query never panics and answers a strategy or one classified refusal.
func FuzzDecodeGlobalSearchRequest(f *testing.F) {
	for _, seed := range []string{
		`{"query":"what is connected to sensor-001","min_revision":7}`,
		`{"query":"sensor-001"}`,
		`{"query":"what changed in the last 2 hours within 5km of 40.7,-74.0"}`,
		`{"query":"what changed in the last 0 hours"}`,
		`{"query":"how many pumps connected to pump-42 in the last 2 hours"}`,
		`{"query":" "}`,
		`{"query":"a","level":1}`,
		`{"query":"a"}x`,
		``,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		request, err := decodeGlobalSearchRequest(data)
		if err != nil {
			return
		}
		if strings.TrimSpace(request.Query) == "" {
			t.Fatalf("accepted blank query %q", request.Query)
		}
		route, err := routeGlobalSearch(t.Context(), request.Query)
		if err != nil {
			var refusal *errs.ClassifiedError
			if !errors.As(err, &refusal) || refusal.Code == "" {
				t.Fatalf("route(%q) error %v is not a coded refusal", request.Query, err)
			}
			return
		}
		switch route.strategy {
		case "entity":
			if route.reference == "" || strings.ContainsFunc(route.reference, unicode.IsSpace) {
				t.Errorf("route(%q) entity reference %q", request.Query, route.reference)
			}
		case "path":
			if route.reference == "" {
				t.Errorf("route(%q) path with no start", request.Query)
			}
		case "temporal":
			if route.timeRange == nil {
				t.Errorf("route(%q) temporal with no range", request.Query)
			}
		case "spatial":
			if route.area == nil {
				t.Errorf("route(%q) spatial with no area", request.Query)
			}
		default:
			t.Errorf("route(%q) strategy %q", request.Query, route.strategy)
		}
	})
}
