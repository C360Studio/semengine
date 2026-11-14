package graphquery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/query"
	"github.com/c360studio/semengine/pkg/errs"
)

// unsentWordSearch begins the refusal of a word search a component that was never started cannot
// send: a question that reaches it was searched, not refused by its hints. Spelled here as the
// oracle rather than read from the code.
const unsentWordSearch = "request to graph.lexical.query.search: "

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

// notHonoured is a hint the route lists not honoured, as the test spells it: kind, detail, reason.
type notHonoured struct{ hint, detail, reason string }

// The reasons a hint global search never honours is not honoured, spelled here as the oracle.
const (
	noStart       = "the question names no start to walk from"
	noTime        = "global search does not filter by time"
	noPlace       = "global search does not filter by place"
	noAggregation = "global search computes no aggregation"
	noSimilarity  = "global search matches words, not meaning"
	noRanking     = "global search orders matches by how well their words match"
)

// requireHints holds that got lists exactly want, in order, each not honoured with no code.
func requireHints(t *testing.T, question string, got []graph.GlobalSearchHint, want []notHonoured) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("route(%q) hints = %+v, want %+v", question, got, want)
		return
	}
	for i, hint := range got {
		w := want[i]
		if hint.Hint != w.hint || hint.Honoured || hint.Reason != w.reason || hint.Code != "" || (w.detail != "*" && hint.Detail != w.detail) {
			t.Errorf("route(%q) hint %d = %+v, want %+v not honoured", question, i, hint, w)
		}
	}
}

// TestGlobalSearchListsHints proves what classifying a question gives, through the keyword
// classifier: the reference to verify (a one-word question's entity, a path's start) and every other
// hint listed not honoured with its reason, in a fixed order. No trigger word refuses: two filters,
// an aggregation or a hint beside a filter are each listed. Detail "*" is a time range or an area,
// whose text the test reads apart.
func TestGlobalSearchListsHints(t *testing.T) {
	cases := []struct {
		question  string
		strategy  string
		reference string
		hints     []notHonoured
	}{
		{"what is connected to sensor-001", "path", "sensor-001", nil},
		// The keyword classifier reads a zone phrase as a path start; the start is verified, not trusted.
		{"the area of the plant", "path", "area-of", nil},
		{"sensor-001", "entity", "sensor-001", nil},
		{"  acme.ops.robotics.gcs.drone.001\t", "entity", "acme.ops.robotics.gcs.drone.001", nil},
		// One token is an entity reference and is not classified, so words the keyword classifier
		// would read as a time range, a path, a count or a ranking are no hint.
		{"forklift", "entity", "forklift", nil},
		{"acme.plant.today.sensor.temp.001", "entity", "acme.plant.today.sensor.temp.001", nil},
		{"acme.plant.zone-a.sensor.temp.001", "entity", "acme.plant.zone-a.sensor.temp.001", nil},
		{"max-pump", "entity", "max-pump", nil},
		{"total-flow", "entity", "total-flow", nil},
		{"forklift hydraulic leak", "", "", nil},
		{"show me sensor-001", "", "", nil},
		{"http connection pool", "", "", []notHonoured{{"path", "", noStart}}},
		{"top 5 sensors", "", "", []notHonoured{{"ranking", "", noRanking}}},
		{"find sensors similar to pump-42", "", "", []notHonoured{{"similarity", "", noSimilarity}}},
		{"most similar pumps", "", "", []notHonoured{{"similarity", "", noSimilarity}, {"ranking", "", noRanking}}},
		{"what changed in the last 2 hours", "", "", []notHonoured{{"time_range", "*", noTime}}},
		{"what is within 5km of 40.7,-74.0", "", "", []notHonoured{{"area", "*", noPlace}}},
		{"max retries", "", "", []notHonoured{{"aggregation", "max of retries", noAggregation}}},
		{"how many pumps are there", "", "", []notHonoured{{"aggregation", "count", noAggregation}}},
		// A count of a path: the start is verified and the count is listed.
		{"how many sensors are connected to pump-42", "path", "pump-42", []notHonoured{{"aggregation", "count", noAggregation}}},
		// Two filters, and a hint beside a filter: the path is walked and each other hint is listed.
		{"what is connected to pump-42 in the last 2 hours", "path", "pump-42", []notHonoured{{"time_range", "*", noTime}}},
		{"top 5 sensors connected to pump-42", "path", "pump-42", []notHonoured{{"ranking", "", noRanking}}},
		{"what changed in the last 2 hours within 5km of 40.7,-74.0", "", "", []notHonoured{{"time_range", "*", noTime}, {"area", "*", noPlace}}},
		{"what is related to it in the last 2 hours", "", "", []notHonoured{{"path", "", noStart}, {"time_range", "*", noTime}}},
	}
	for _, tc := range cases {
		route, err := routeGlobalSearch(t.Context(), tc.question)
		if err != nil {
			t.Errorf("route(%q) error = %v, want hints", tc.question, err)
			continue
		}
		if route.strategy != tc.strategy || route.reference != tc.reference {
			t.Errorf("route(%q) = %q %q, want %q %q", tc.question, route.strategy, route.reference, tc.strategy, tc.reference)
		}
		requireHints(t, tc.question, route.hints, tc.hints)
		for _, hint := range route.hints {
			switch hint.Hint {
			case "time_range":
				// The classifier reads the clock; the range is the last two hours, written as two times.
				start, end, ok := strings.Cut(hint.Detail, " to ")
				from, errStart := time.Parse(time.RFC3339, start)
				to, errEnd := time.Parse(time.RFC3339, end)
				if !ok || errStart != nil || errEnd != nil || to.Sub(from) != 2*time.Hour {
					t.Errorf("route(%q) time range detail = %q, want two hours", tc.question, hint.Detail)
				}
			case "area":
				// The box is the classifier's; the detail must carry it around the question's point.
				var north, south, east, west float64
				if _, err := fmt.Sscanf(hint.Detail, "north %g, south %g, east %g, west %g", &north, &south, &east, &west); err != nil ||
					south >= 40.7 || north <= 40.7 || west >= -74.0 || east <= -74.0 {
					t.Errorf("route(%q) area detail = %q, want a box around 40.7,-74.0", tc.question, hint.Detail)
				}
			}
		}
	}

	// Another tier's options can name path predicates, which the walk would drop: the start is then
	// not verified, and the path is listed not honoured. With no start they are path words.
	for _, tc := range []struct {
		hints questionHints
		want  notHonoured
	}{
		{questionHints{path: true, pathStart: "pump-42", pathPredicates: true}, notHonoured{"path", "pump-42", "a walk follows every predicate, not only the question's"}},
		{questionHints{pathPredicates: true}, notHonoured{"path", "", noStart}},
	} {
		route := listHints(tc.hints)
		if route.strategy != "" || route.reference != "" {
			t.Errorf("listHints(%+v) verifies %q %q, want nothing", tc.hints, route.strategy, route.reference)
		}
		requireHints(t, fmt.Sprintf("%+v", tc.hints), route.hints, []notHonoured{tc.want})
	}

	// Through the handler: every question reaches the word search, which a component holding no
	// connection cannot send, its own refusal and one error; a count question reaches it like any other.
	for _, question := range []string{"max retries", "http connection pool", "what is connected to pump-42 in the last 2 hours"} {
		request := fmt.Sprintf(`{"query":%q}`, question)
		c, reply, err := askUnstarted(t, request)
		if err == nil {
			t.Errorf("globalSearch(%q) = %q, want the unsent word search's refusal", question, reply)
			continue
		}
		var refusal *errs.ClassifiedError
		if !errors.As(err, &refusal) || refusal.Code != graph.ErrorCodeInternal || !strings.HasPrefix(err.Error(), unsentWordSearch) {
			t.Errorf("globalSearch(%q) refused %v, want internal %q", question, err, unsentWordSearch)
		}
		// The handler counts before it returns.
		if flow := c.DataFlow(); flow.Messages != 1 || flow.Bytes != int64(len(request)) {
			t.Errorf("globalSearch(%q) DataFlow = %+v, want one message of %d bytes", question, flow, len(request))
		}
		if got := c.Health().ErrorCount; got != 1 {
			t.Errorf("globalSearch(%q) ErrorCount = %d, want 1", question, got)
		}
	}
}

// TestClassifierRefusalIsAHint proves that the classifier's refusal of a question (an impossible
// number, two places) is a not-honoured classification hint carrying the refusal's text, never a
// refusal of the question and never a walk from one of the places: the question is still searched.
func TestClassifierRefusalIsAHint(t *testing.T) {
	for _, tc := range []struct{ question, says string }{
		{"what changed in the last 0 hours", `"last 0 hours": the number must be at least 1`},
		{"what is within 5km of 200,-74.0", `"within 5km of 200,-74.0": the latitude must be in [-90, 90]`},
		{"what is connected to pump-42 in the last 0 days", `"last 0 days": the number must be at least 1`},
		{"what is connected to sensor-001 near zone-b", "the question names two places, sensor-001 and zone-b; ask about one"},
		{"what is connected to acme.ops.robotics.gcs.drone.001 in zone b", "the question names two places, acme.ops.robotics.gcs.drone.001 and zone-b; ask about one"},
	} {
		route, err := routeGlobalSearch(t.Context(), tc.question)
		if err != nil {
			t.Errorf("route(%q) error = %v, want a classification hint", tc.question, err)
			continue
		}
		if route.strategy != "" || route.reference != "" {
			t.Errorf("route(%q) verifies %q %q, want nothing", tc.question, route.strategy, route.reference)
		}
		if len(route.hints) != 1 {
			t.Errorf("route(%q) hints = %+v, want one classification hint", tc.question, route.hints)
			continue
		}
		hint := route.hints[0]
		const reason = "the question classifier refused the question; it is searched for its words"
		if hint.Hint != "classification" || hint.Honoured || hint.Reason != reason || hint.Code != graph.ErrorCodeInvalidRequest ||
			!strings.HasSuffix(hint.Detail, tc.says) {
			t.Errorf("route(%q) hint = %+v, want classification not honoured, invalid_request, detail ending %q", tc.question, hint, tc.says)
		}
		c, _, err := askUnstarted(t, fmt.Sprintf(`{"query":%q}`, tc.question))
		if err == nil || !strings.HasPrefix(err.Error(), unsentWordSearch) {
			t.Errorf("globalSearch(%q) error = %v, want the unsent word search's refusal", tc.question, err)
		}
		if got := c.Health().ErrorCount; got != 1 {
			t.Errorf("globalSearch(%q) ErrorCount = %d, want the word search's 1", tc.question, got)
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

// FuzzDecodeGlobalSearchRequest checks that an accepted request holds a query that is not blank and
// a limit of 0 to 100, and that routing any accepted query never panics and, under a live context,
// never refuses: it names a reference only beside its kind, and lists only known hints, each not
// honoured with a reason.
func FuzzDecodeGlobalSearchRequest(f *testing.F) {
	for _, seed := range []string{
		`{"query":"what is connected to sensor-001","min_revision":7}`,
		`{"query":"sensor-001"}`,
		`{"query":"what changed in the last 2 hours within 5km of 40.7,-74.0"}`,
		`{"query":"what changed in the last 0 hours"}`,
		`{"query":"how many pumps connected to pump-42 in the last 2 hours"}`,
		`{"query":"what is connected to sensor-001 near zone-b"}`,
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
	known := map[string]bool{"path": true, "time_range": true, "area": true, "aggregation": true, "similarity": true, "ranking": true, "classification": true}
	f.Fuzz(func(t *testing.T, data []byte) {
		request, err := decodeGlobalSearchRequest(data)
		if err != nil {
			return
		}
		if request.Limit < 0 || request.Limit > 100 {
			t.Fatalf("accepted limit %d", request.Limit)
		}
		if strings.TrimSpace(request.Query) == "" {
			t.Fatalf("accepted blank query %q", request.Query)
		}
		route, err := routeGlobalSearch(t.Context(), request.Query)
		if err != nil {
			t.Fatalf("route(%q) refused %v under a live context", request.Query, err)
		}
		switch route.strategy {
		case "entity":
			if route.reference == "" || strings.ContainsFunc(route.reference, unicode.IsSpace) || len(route.hints) != 0 {
				t.Errorf("route(%q) entity %q beside %+v", request.Query, route.reference, route.hints)
			}
		case "path":
			if route.reference == "" {
				t.Errorf("route(%q) path with no start", request.Query)
			}
		case "":
			if route.reference != "" {
				t.Errorf("route(%q) reference %q with no kind", request.Query, route.reference)
			}
		default:
			t.Errorf("route(%q) kind %q", request.Query, route.strategy)
		}
		for _, hint := range route.hints {
			if !known[hint.Hint] || hint.Honoured || hint.Reason == "" {
				t.Errorf("route(%q) hint %+v", request.Query, hint)
			}
		}
	})
}
