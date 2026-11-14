package graphquery

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	pathAndTime = "invalid request: the question names two filters, a path from pump-42 and a time range; ask with one"
	// unsentWordSearch begins the refusal of a word search a component that was never started cannot
	// send: a question that reaches it was routed to text.
	unsentWordSearch = "request to graph.lexical.query.search: "
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

// askUnstarted sends request to the globalSearch handler of a component that was never started: it
// holds no connection, so a downstream request cannot be made, and no community view.
func askUnstarted(t *testing.T, request string) (*Component, []byte, error) {
	t.Helper()
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	reply, err := c.queryHandlers()["globalSearch"](t.Context(), []byte(request))
	return c, reply, err
}

// TestGlobalSearchRoutesByHints proves the route table through the keyword classifier, one row per
// table line, with ordinary-word questions. Each row names the strategy that answers it.
func TestGlobalSearchRoutesByHints(t *testing.T) {
	answered := []struct {
		question   string
		strategy   string
		reference  string
		unhonoured []string
	}{
		{"what is connected to sensor-001", "path", "sensor-001", nil},
		// The keyword tier reads a zone phrase as a path start and no predicate; the start names no
		// entity here, which the resolver answers entity_not_found.
		{"the area of the plant", "path", "area-of", nil},
		{"what changed in the last 2 hours", "temporal", "", nil},
		{"what is within 5km of 40.7,-74.0", "spatial", "", nil},
		{"sensor-001", "entity", "sensor-001", nil},
		{"  acme.ops.robotics.gcs.drone.001\t", "entity", "acme.ops.robotics.gcs.drone.001", nil},
		// One token is an entity reference, read before any hint; one naming no entity is searched
		// as text by the resolver's miss. The keyword tier would read these words as a time range,
		// a path, a count or a ranking.
		{"forklift", "entity", "forklift", nil},
		{"acme.plant.today.sensor.temp.001", "entity", "acme.plant.today.sensor.temp.001", nil},
		{"acme.plant.zone-a.sensor.temp.001", "entity", "acme.plant.zone-a.sensor.temp.001", nil},
		{"acme.plant.link.sensor.temp.001", "entity", "acme.plant.link.sensor.temp.001", nil},
		{"acme.plant.most.sensor.temp.001", "entity", "acme.plant.most.sensor.temp.001", nil},
		{"connector-7", "entity", "connector-7", nil},
		{"max-pump", "entity", "max-pump", nil},
		{"total-flow", "entity", "total-flow", nil},
		// No hint and several tokens.
		{"forklift hydraulic leak", "text", "", nil},
		{"show me sensor-001", "text", "", nil},
		// Path words with no start, a similarity or a ranking alone: text, each hint named.
		{"show the path", "text", "", []string{"path"}},
		{"http connection pool", "text", "", []string{"path"}},
		{"file path handling", "text", "", []string{"path"}},
		{"top 5 sensors", "text", "", []string{"ranking"}},
		{"find sensors similar to pump-42", "text", "", []string{"similarity"}},
		{"most similar pumps", "text", "", []string{"similarity", "ranking"}},
		{"top 5 sensors related to things", "text", "", []string{"path", "ranking"}},
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
		if !slices.Equal(route.unhonoured, tc.unhonoured) {
			t.Errorf("route(%q) unhonoured = %q, want %q", tc.question, route.unhonoured, tc.unhonoured)
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

	// A count or other aggregation is refused, with or without other hints: a list of matches over
	// sampled pages would invite the reader to count a sample.
	refused := []struct {
		question string
		want     string
	}{
		{"max retries", "invalid request: the question asks for an aggregation (max of retries), which global search does not compute; ask without it"},
		{"total memory usage", "invalid request: the question asks for an aggregation (sum of memory), which global search does not compute; ask without it"},
		{"how many pumps are there", "invalid request: the question asks for an aggregation (count), which global search does not compute; ask without it"},
		{"how many sensors are connected to pump-42", "invalid request: the question asks for an aggregation (count), which global search does not compute; ask without it"},
		{"how many pumps changed in the last 2 hours", "invalid request: the question asks for an aggregation (count), which global search does not compute; ask without it"},
		// Two filters: the table's row checked first.
		{"how many pumps connected to pump-42 in the last 2 hours", pathAndTime},
		{"what is connected to pump-42 in the last 2 hours", pathAndTime},
	}
	for _, tc := range refused {
		route, err := routeGlobalSearch(t.Context(), tc.question)
		if err == nil {
			t.Errorf("route(%q) = %s %q, want refused %q", tc.question, route.strategy, route.reference, tc.want)
			continue
		}
		refusal := refusalOfRoute(t, tc.question, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if err.Error() != tc.want {
			t.Errorf("route(%q) refusal = %q, want %q", tc.question, err.Error(), tc.want)
		}
		if refusal.Detail != nil {
			t.Errorf("route(%q) refusal detail = %v, want none", tc.question, refusal.Detail)
		}
	}

	// Through the handler (spec, "A question this build cannot answer"): a count is one input and no
	// error, and asks nothing downstream. A text question reaches the word search, which a component
	// that holds no connection cannot send: its own refusal, one error.
	for _, tc := range []struct {
		question, code, want string
		errors               int
	}{
		{"max retries", graph.ErrorCodeInvalidRequest, refused[0].want, 0},
		{"http connection pool", graph.ErrorCodeInternal, unsentWordSearch, 1},
		{"forklift hydraulic leak", graph.ErrorCodeInternal, unsentWordSearch, 1},
	} {
		request := fmt.Sprintf(`{"query":%q}`, tc.question)
		c, reply, err := askUnstarted(t, request)
		if err == nil {
			t.Errorf("globalSearch(%q) = %q, want refused %s", tc.question, reply, tc.code)
			continue
		}
		var refusal *errs.ClassifiedError
		if !errors.As(err, &refusal) || refusal.Code != tc.code || !strings.HasPrefix(err.Error(), tc.want) {
			t.Errorf("globalSearch(%q) refused %v, want %s %q", tc.question, err, tc.code, tc.want)
		}
		// The handler counts before it returns.
		if flow := c.DataFlow(); flow.Messages != 1 || flow.Bytes != int64(len(request)) {
			t.Errorf("globalSearch(%q) DataFlow = %+v, want one message of %d bytes", tc.question, flow, len(request))
		}
		if got := c.Health().ErrorCount; got != tc.errors {
			t.Errorf("globalSearch(%q) ErrorCount = %d, want %d", tc.question, got, tc.errors)
		}
	}
}

// TestGlobalSearchHintBesideAFilter proves that a text-routed hint (path words with no start, a
// similarity or a ranking) beside a filter, or a start carrying path predicates, is refused
// invalid_request naming both, never answered by the text tier with the filter dropped nor by the
// filter's strategy with the hint dropped. The handler asks nothing downstream: the component holds
// no connection, and the refusal is not the text tier's index_not_ready.
func TestGlobalSearchHintBesideAFilter(t *testing.T) {
	cases := []struct {
		question string
		want     string
	}{
		{"top 5 sensors connected to pump-42", "a ranking beside a path from pump-42"},
		{"find sensors similar to pump-42 in the last 2 hours", "a similarity beside a time range"},
		{"what is related to it in the last 2 hours", "path words with no start beside a time range"},
		{"top 5 sensors in the last 2 hours", "a ranking beside a time range"},
		{"sensors similar to pump-42 within 5km of 40.7,-74.0", "a similarity beside an area"},
	}
	for _, tc := range cases {
		want := "invalid request: the question names " + tc.want + ", which no strategy answers together; ask with one"
		_, err := routeGlobalSearch(t.Context(), tc.question)
		refusalOfRoute(t, tc.question, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if err.Error() != want {
			t.Errorf("route(%q) refusal = %q, want %q", tc.question, err.Error(), want)
		}
		c, _, err := askUnstarted(t, fmt.Sprintf(`{"query":%q}`, tc.question))
		refusalOfRoute(t, tc.question, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
		if got := c.Health().ErrorCount; got != 0 {
			t.Errorf("globalSearch(%q) ErrorCount = %d, want 0", tc.question, got)
		}
	}

	// The keyword tier sends no path predicates; another tier's options would. A start carrying them
	// would walk every predicate, dropping theirs; with no start they are path words.
	start := questionHints{path: true, pathStart: "pump-42", pathPredicates: true}
	_, err := chooseRoute(start)
	refusalOfRoute(t, "path predicates", err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	const want = "invalid request: the question names path predicates beside a path from pump-42, which no strategy answers together; ask with one"
	if err.Error() != want {
		t.Errorf("chooseRoute(%+v) refusal = %q, want %q", start, err.Error(), want)
	}
	for _, hints := range []questionHints{{path: true, pathPredicates: true}, {pathPredicates: true}} {
		route, err := chooseRoute(hints)
		if err != nil || route.strategy != "text" || !slices.Equal(route.unhonoured, []string{"path"}) {
			t.Errorf("chooseRoute(%+v) = %+v, %v, want text naming path", hints, route, err)
		}
	}
}

// TestGlobalSearchRefusesTwoFilters proves that two or three of path, time range and area are
// refused invalid_request naming each, whatever other hints the question holds (spec "Two
// filters").
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

// TestGlobalSearchRefusesImpossibleNumbers proves the classifier's refusal of an out-of-range
// number reaches the caller as invalid_request naming the phrase.
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

// TestGlobalSearchEndedClassification proves that a classification with no result and no error,
// which the chain returns once the request's context has ended
// (graph/query/classifier_chain.go:91-93), is refused transient internal, never read as a question
// with no hints.
func TestGlobalSearchEndedClassification(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// The question would route to path under a live context; a one-word question is not classified.
	const question = "what is connected to sensor-001"
	route, err := routeGlobalSearch(ctx, question)
	if err == nil {
		t.Fatalf("route under an ended context = %s %q, want refused", route.strategy, route.reference)
	}
	refusalOfRoute(t, question, err, errs.ErrorTransient, graph.ErrorCodeInternal)
	const want = "internal error: the question's classification ended with the request: context canceled"
	if err.Error() != want {
		t.Errorf("refusal = %q, want %q", err.Error(), want)
	}

	// Through the handler, as natsclient calls it once the handler's budget has ended its context: one
	// input and one error, and nothing is asked downstream, since the component holds no connection.
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	const request = `{"query":"what is connected to sensor-001"}`
	reply, err := c.queryHandlers()["globalSearch"](ctx, []byte(request))
	if err == nil {
		t.Fatalf("globalSearch under an ended context = %q, want refused", reply)
	}
	refusalOfRoute(t, question, err, errs.ErrorTransient, graph.ErrorCodeInternal)
	if err.Error() != want {
		t.Errorf("handler refusal = %q, want %q", err.Error(), want)
	}
	// The handler counts before it returns.
	if flow := c.DataFlow(); flow.Messages != 1 || flow.Bytes != int64(len(request)) {
		t.Errorf("DataFlow = %+v, want one message of %d bytes", flow, len(request))
	}
	if got := c.Health().ErrorCount; got != 1 {
		t.Errorf("ErrorCount = %d, want 1", got)
	}
}

// TestGlobalSearchRefusesHintsItCannotRead proves the typed read fails closed: a hint global search
// does not know, or one of another type, is refused, never read as absent. The keyword tier sends
// neither; another tier's options would.
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
		{map[string]any{"aggregation_type": 1}, `classification hint "aggregation_type" holds int`},
		{map[string]any{"aggregation_type": "max", "aggregation_field": true}, `classification hint "aggregation_field" holds bool`},
		{map[string]any{"use_embeddings": "true"}, `classification hint "use_embeddings" holds string`},
		{map[string]any{"ranking_intent": 1}, `classification hint "ranking_intent" holds int`},
		{map[string]any{"ranking_intent": true, "limit": "5"}, `classification hint "limit" holds string`},
		{map[string]any{"path_predicates": "link.to.entity"}, `classification hint "path_predicates" holds string`},
	}
	for _, tc := range cases {
		_, err := readHints(tc.options)
		if err == nil || err.Error() != tc.want {
			t.Errorf("readHints(%v) error = %v, want %q", tc.options, err, tc.want)
		}
	}
}

// TestGlobalSearchRequestKeys proves the request keys: query, min_revision and level (0 to 2,
// omitted 0) decode; a blank query, a level outside 0 to 2, any other key (such as max_communities,
// summarize_threshold, include_*) by name, and bytes after the request are refused.
func TestGlobalSearchRequestKeys(t *testing.T) {
	for _, tc := range []struct {
		data         string
		level, limit int
	}{
		{`{"query":"what is connected to sensor-001","min_revision":7}`, 0, 0},
		{`{"query":"what is connected to sensor-001","min_revision":7,"level":0}`, 0, 0},
		{`{"query":"what is connected to sensor-001","min_revision":7,"level":2,"limit":1}`, 2, 1},
		{`{"query":"what is connected to sensor-001","min_revision":7,"limit":100}`, 0, 100},
	} {
		request, err := decodeGlobalSearchRequest([]byte(tc.data))
		if err != nil {
			t.Fatalf("decode(%s) = %v", tc.data, err)
		}
		if request.Query != "what is connected to sensor-001" || request.MinRevision != 7 || request.Level != tc.level || request.Limit != tc.limit {
			t.Errorf("decode(%s) = %+v, want level %d, limit %d", tc.data, request, tc.level, tc.limit)
		}
	}

	refused := []struct {
		data string
		want string
	}{
		{`{"query":"sensor-001","level":3}`, "level: clustering.LevelPrefix: check level failed: level 3 is not 0, 1 or 2"},
		{`{"query":"sensor-001","level":-1}`, "level: clustering.LevelPrefix: check level failed: level -1 is not 0, 1 or 2"},
		{`{"query":"sensor-001","max_communities":5}`, `json: unknown field "max_communities"`},
		{`{"query":"sensor-001","summarize_threshold":50}`, `json: unknown field "summarize_threshold"`},
		{`{"query":"sensor-001","include_summaries":true}`, `json: unknown field "include_summaries"`},
		{`{"query":"sensor-001","include_relationships":true}`, `json: unknown field "include_relationships"`},
		{`{"query":"sensor-001","include_sources":true}`, `json: unknown field "include_sources"`},
		{`{"query":"sensor-001","limit":101}`, "limit 101 is not 0 or 1 to 100"},
		{`{"query":"sensor-001","limit":-1}`, "limit -1 is not 0 or 1 to 100"},
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
		`{"query":"a","level":3}`,
		`{"query":"forklift hydraulic leak"}`,
		`{"query":"top 5 sensors connected to pump-42"}`,
		`{"query":"a"}x`,
		``,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		request, err := decodeGlobalSearchRequest(data)
		if err == nil && (request.Limit < 0 || request.Limit > 100) {
			t.Fatalf("accepted limit %d", request.Limit)
		}
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
		case "text":
			for _, hint := range route.unhonoured {
				if hint != "path" && hint != "similarity" && hint != "ranking" {
					t.Errorf("route(%q) unhonoured hint %q", request.Query, hint)
				}
			}
		default:
			t.Errorf("route(%q) strategy %q", request.Query, route.strategy)
		}
	})
}
