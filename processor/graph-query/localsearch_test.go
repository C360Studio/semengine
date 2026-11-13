package graphquery

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/c360studio/semengine/graph"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

func rawEntities(entities ...string) []json.RawMessage {
	raws := make([]json.RawMessage, 0, len(entities))
	for _, entity := range entities {
		raws = append(raws, json.RawMessage(entity))
	}
	return raws
}

// rawStrings is each match's bytes, in order.
func rawStrings(matches []rankedEntity) []string {
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, string(match.raw))
	}
	return out
}

// searchTermsOf decodes a request holding query, so a test's terms come through the verb's own decoding.
func searchTermsOf(t *testing.T, query string) []string {
	t.Helper()
	request, err := json.Marshal(graph.LocalSearchRequest{EntityID: "acme.ops.src.docs.note.n00", Query: query})
	if err != nil {
		t.Fatal(err)
	}
	_, terms, err := decodeLocalSearchRequest(request)
	if err != nil {
		t.Fatalf("decodeLocalSearchRequest(%s) = %v", request, err)
	}
	return terms
}

// TestLocalSearchScoresAsThePin checks the text score on fixed entities and queries against literal
// scores, and each against the 0.3 threshold. A case that holds a deliberate choice of this verb's
// rule says so.
func TestLocalSearchScoresAsThePin(t *testing.T) {
	const (
		drone          = `{"id":"a.b.c.d.drone.001","triples":[]}`
		sensorLocation = `{"id":"a.b.c.d.sensor.001","triples":[{"predicate":"sensor.identity.location","object":"warehouse"}]}`
		droneBattery   = `{"id":"a.b.c.d.drone.001","triples":[{"predicate":"drone.measurement.battery","object":85.0}]}`
		droneActive    = `{"id":"a.b.c.d.drone.001","triples":[{"predicate":"drone.state.status","object":"active"}]}`
		droneSurvey    = `{"id":"a.b.c.d.drone.001","triples":[{"predicate":"drone.assignment.mission","object":"survey"}]}`
		droneACTIVE    = `{"id":"a.b.c.d.drone.001","triples":[{"predicate":"drone.state.status","object":"ACTIVE"}]}`
		note           = `{"id":"acme.ops.src.docs.note.n00","triples":[{"subject":"x","predicate":"dc.terms.title","object":"alpha"}]}`
	)
	cases := []struct {
		name   string
		entity string
		query  string
		score  float64
	}{
		{"type match scores highest", drone, "drone", 1.0},
		{"triple value match scores moderate", sensorLocation, "warehouse", 0.5},
		{"no match scores zero", droneBattery, "warehouse logistics", 0},
		// Only string values are searched.
		{"a number is not text", droneBattery, "85", 0},
		{"partial match proportional", droneActive, "drone operations", 0.5},
		{"multiple matches accumulate", droneSurvey, "drone survey", 0.75},
		// 1.0 for the type and 0.5 for the value, over two terms.
		{"case insensitive", droneACTIVE, "DRONE active", 0.75},
		{"probe TWO_TERMS_ONE_TEXT", note, "alpha zzz", 0.25},
		// A repeated term counts once, so the query scores as "alpha zzz" and is dropped.
		{"probe DUP_TERMS", note, "alpha alpha zzz", 0.25},
		{"probe TYPE_TERM", note, "note", 1},
		{"probe PRED_TERM: a predicate name matches", note, "title", 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			terms := searchTermsOf(t, tc.query)
			entity, err := decodeSearchEntity(json.RawMessage(tc.entity))
			if err != nil {
				t.Fatal(err)
			}
			if got := entity.score(terms); got != tc.score {
				t.Errorf("score(%s, %q) = %v, want %v", tc.entity, tc.query, got, tc.score)
			}
			matches, _, err := rankEntities(rawEntities(tc.entity), terms, nil)
			if err != nil {
				t.Fatal(err)
			}
			kept := tc.score >= 0.3 // minLocalSearchScore, written as a literal so a changed constant fails here
			if got := len(matches) == 1; got != kept {
				t.Errorf("kept = %v at score %v, want %v", got, tc.score, kept)
			}
		})
	}

	t.Run("FilterEntitiesByQuery_DropsLowScore", func(t *testing.T) {
		page := rawEntities(
			`{"id":"a.b.c.d.drone.001","triples":[{"predicate":"drone.assignment.mission","object":"survey"}]}`,
			`{"id":"a.b.c.d.sensor.002","triples":[{"predicate":"sensor.identity.location","object":"hangar"}]}`,
			`{"id":"a.b.c.d.worker.003","triples":[{"predicate":"worker.identity.role","object":"logistics"}]}`,
		)
		matches, scanned, err := rankEntities(page, searchTermsOf(t, "drone survey"), nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{`{"id":"a.b.c.d.drone.001","triples":[{"predicate":"drone.assignment.mission","object":"survey"}]}`}
		if got := rawStrings(matches); !slices.Equal(got, want) || scanned != 3 {
			t.Errorf("rankEntities = %q, %d; want %q, 3", got, scanned, want)
		}
	})

	// This verb's threshold is the fixed 0.3, at which the sensor scores 0.25 and is dropped.
	// "temperature sensor" keeps both, the type match first.
	t.Run("FilterEntitiesByQuery_SortsByScore", func(t *testing.T) {
		const (
			sensor = `{"id":"a.b.c.d.sensor.001","triples":[{"predicate":"sensor.identity.type","object":"temperature"}]}`
			drone  = `{"id":"a.b.c.d.drone.002","triples":[{"predicate":"drone.equipment.sensor","object":"temperature"}]}`
		)
		for query, want := range map[string][]string{
			"drone temperature":  {drone},
			"temperature sensor": {sensor, drone},
		} {
			matches, _, err := rankEntities(rawEntities(sensor, drone), searchTermsOf(t, query), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := rawStrings(matches); !slices.Equal(got, want) {
				t.Errorf("%q: rankEntities = %q, want %q", query, got, want)
			}
		}
	})
}

func TestLocalSearchTermsAreDistinctAndBounded(t *testing.T) {
	if got, want := searchTermsOf(t, "Alpha  alpha\tBETA\ngamma ALPHA"), []string{"alpha", "beta", "gamma"}; !slices.Equal(got, want) {
		t.Errorf("terms = %q, want %q", got, want)
	}

	sixteen := "t01 t02 t03 t04 t05 t06 t07 t08 t09 t10 t11 t12 t13 t14 t15 t16"
	if got := searchTermsOf(t, sixteen+" T16 t01"); len(got) != 16 {
		t.Errorf("16 distinct terms with repeats gave %d terms, want 16", len(got))
	}
	for _, query := range []string{sixteen + " t17", "", " \t\n"} {
		request := `{"entity_id":"acme.ops.src.docs.note.n00","query":"` + strings.ReplaceAll(strings.ReplaceAll(query, "\t", `\t`), "\n", `\n`) + `"}`
		if _, terms, err := decodeLocalSearchRequest([]byte(request)); err == nil || !strings.Contains(err.Error(), "query") {
			t.Errorf("decodeLocalSearchRequest(%s) = %q, %v; want an error naming query", request, terms, err)
		}
	}

	// A query of about 1 MB, every term distinct: the split stops at the first term over the bound.
	var long strings.Builder
	for i := 0; long.Len() < 1<<20; i++ {
		fmt.Fprintf(&long, "t%d ", i)
	}
	request, err := json.Marshal(graph.LocalSearchRequest{EntityID: "acme.ops.src.docs.note.n00", Query: long.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeLocalSearchRequest(request); err == nil || !strings.Contains(err.Error(), "query") {
		t.Errorf("a 1 MB query: %v, want an error naming query", err)
	}
	if got := len(searchTerms(long.String())); got != 17 {
		t.Errorf("searchTerms of a 1 MB query returned %d terms, want 17", got)
	}
}

func TestLocalSearchRequestDecoding(t *testing.T) {
	request, terms, err := decodeLocalSearchRequest([]byte(`{"entity_id":"acme.ops.src.docs.note.n00","query":"Alpha","level":2,"min_revision":7}`))
	want := graph.LocalSearchRequest{EntityID: "acme.ops.src.docs.note.n00", Query: "Alpha", Level: 2, MinRevisionField: graph.MinRevisionField{MinRevision: 7}}
	if err != nil || request != want || !slices.Equal(terms, []string{"alpha"}) {
		t.Errorf("decode = %+v, %q, %v; want %+v, [alpha], nil", request, terms, err, want)
	}
	if request, _, err := decodeLocalSearchRequest([]byte(`{"entity_id":"acme.ops.src.docs.note.n00","query":"a"}`)); err != nil || request.Level != 0 {
		t.Errorf("an omitted level = %d, %v; want 0, nil", request.Level, err)
	}

	for request, field := range map[string]string{
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","limit":3}`: "limit",
		`{"entity_id":"acme.ops.src.docs.note","query":"a"}`:               "entity_id",
		`{"query":"a"}`: "entity_id",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","level":3}`:  "level",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","level":-1}`: "level",
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a"} {}`:         "after",
		`not json`: "invalid",
	} {
		if _, _, err := decodeLocalSearchRequest([]byte(request)); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("decodeLocalSearchRequest(%s) = %v, want an error naming %q", request, err, field)
		}
	}
}

func FuzzDecodeLocalSearchRequest(f *testing.F) {
	for _, seed := range []string{
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"Alpha beta ALPHA","level":1,"min_revision":7}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"t01 t02 t03 t04 t05 t06 t07 t08 t09 t10 t11 t12 t13 t14 t15 t16"}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"t01 t02 t03 t04 t05 t06 t07 t08 t09 t10 t11 t12 t13 t14 t15 t16 t17"}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"İSTANBUL straße"}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":" "}`,
		`{"entity_id":"acme.ops.src.docs.note","query":"a"}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","level":3}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a","extra":1}`,
		`{"entity_id":"acme.ops.src.docs.note.n00","query":"a"}x`,
		``,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		request, terms, err := decodeLocalSearchRequest(data)
		if err != nil {
			return
		}
		if err := semtypes.ValidateEntityID(request.EntityID); err != nil {
			t.Errorf("accepted entity_id %q: %v", request.EntityID, err)
		}
		if request.Level < 0 || request.Level > 2 {
			t.Errorf("accepted level %d", request.Level)
		}
		if len(terms) < 1 || len(terms) > graph.MaxSearchTerms {
			t.Errorf("accepted %d terms", len(terms))
		}
		lowered := strings.ToLower(request.Query)
		seen := make(map[string]bool)
		for _, term := range terms {
			if term == "" || strings.ContainsFunc(term, unicode.IsSpace) || !strings.Contains(lowered, term) || seen[term] {
				t.Errorf("term %q of %q is empty, holds white space, is not in the lowered query or repeats", term, request.Query)
			}
			seen[term] = true
		}
	})
}

func TestLocalSearchOrderIsDeterministic(t *testing.T) {
	const (
		d = `{"id":"acme.ops.src.docs.note.d","triples":[{"predicate":"dc.terms.title","object":"alpha"}]}`
		b = `{"id":"acme.ops.src.docs.note.b","triples":[{"predicate":"dc.terms.title","object":"alpha"}]}`
		a = `{"id":"acme.ops.src.docs.alpha.a","triples":[]}`
		c = `{"id":"acme.ops.src.docs.note.c","triples":[{"predicate":"dc.terms.title","object":"alpha"}]}`
	)
	// a scores 1.0 by its type; b, c and d tie at 0.5, read out of ID order, and b repeats.
	matches, scanned, err := rankEntities(rawEntities(d, b, a, c, b), searchTermsOf(t, "alpha"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := rawStrings(matches), []string{a, b, c, d}; !slices.Equal(got, want) || scanned != 4 {
		t.Errorf("rankEntities = %q, %d; want %q, 4", got, scanned, want)
	}

	if _, _, err := rankEntities(rawEntities(`{"id":"acme.ops.src.docs.note.e","triples":{}}`), []string{"alpha"}, nil); err == nil {
		t.Error("rankEntities read an entity whose triples are not a list")
	}
}

func TestLocalSearchAnswerTemplate(t *testing.T) {
	const summary = "Community of 7000 entities including 7000 notes. Key themes: alpha, beta, gamma, delta, epsilon."
	data := graph.LocalSearchData{
		CommunityID:      "acme.ops.src.docs",
		MemberCount:      7000,
		CommunitySummary: summary,
		Keywords:         []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"},
		Entities:         rawEntities(`{}`, `{}`, `{}`),
		EntitiesScanned:  40,
	}
	const found = "Found 3 matching entities among 40 read from acme.ops.src.docs."
	if got, want := localSearchAnswer(data), found+" "+summary; got != want {
		t.Errorf("answer = %q\nwant     %q", got, want)
	}

	data.CommunitySummary = ""
	if got, want := localSearchAnswer(data), found+" It has 7000 members. Key themes: alpha, beta, gamma, delta, epsilon."; got != want {
		t.Errorf("answer without a summary = %q, want %q", got, want)
	}
	data.Keywords = nil
	if got, want := localSearchAnswer(data), found+" It has 7000 members."; got != want {
		t.Errorf("answer with neither = %q, want %q", got, want)
	}
}

func TestLocalSearchFitKeepsTheBestThatFit(t *testing.T) {
	data := graph.LocalSearchData{
		CommunityID:     "acme.ops.src.docs",
		MemberCount:     3,
		Entities:        rawEntities(`{"id":"a"}`, `{"id":"b"}`, `{"id":"c"}`),
		EntitiesScanned: 3,
	}
	encode := func(d graph.LocalSearchData) ([]byte, error) { return json.Marshal(d) }
	const (
		all = `{"community_id":"acme.ops.src.docs","level":0,"member_count":3,"entities":[{"id":"a"},{"id":"b"},{"id":"c"}],"entities_scanned":3,"entities_truncated":false,"answer":"Found 3 matching entities among 3 read from acme.ops.src.docs. It has 3 members."}`
		two = `{"community_id":"acme.ops.src.docs","level":0,"member_count":3,"entities":[{"id":"a"},{"id":"b"}],"entities_scanned":3,"entities_truncated":true,"answer":"Found 2 matching entities among 3 read from acme.ops.src.docs. It has 3 members."}`
		one = `{"community_id":"acme.ops.src.docs","level":0,"member_count":3,"entities":[{"id":"a"}],"entities_scanned":3,"entities_truncated":true,"answer":"Found 1 matching entities among 3 read from acme.ops.src.docs. It has 3 members."}`
	)
	for limit, want := range map[int]string{len(all): all, len(two): two, len(two) - 1: one} {
		got, truncated, err := fitLocalSearch(data, limit, encode)
		if err != nil || string(got) != want || truncated != (limit < len(all)) {
			t.Errorf("fit at %d = %s, %t, %v\nwant %s, %t", limit, got, truncated, err, want, limit < len(all))
		}
	}
	data.EntitiesTruncated = true // the page had a continuation
	if got, truncated, err := fitLocalSearch(data, len(all), encode); err != nil || !truncated || !strings.Contains(string(got), `"entities_truncated":true`) {
		t.Errorf("fit of a truncated page = %s, %t, %v; want it still truncated", got, truncated, err)
	}
	if got, _, err := fitLocalSearch(data, 10, encode); err == nil {
		t.Errorf("fit at 10 bytes = %s, want an error", got)
	}
}
