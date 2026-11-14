package graphquery

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The text tier's own score, which global search ranks a group page by.

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

// searchTermsOf is the text tier's terms of query, which the test needs to be accepted.
func searchTermsOf(t *testing.T, query string) []string {
	t.Helper()
	terms, err := checkedSearchTerms(query)
	if err != nil {
		t.Fatalf("checkedSearchTerms(%q) = %v", query, err)
	}
	return terms
}

// TestTextTierScoresAsThePin checks the text score on fixed entities and queries against literal
// scores, and each against the 0.3 threshold. A case that holds a deliberate choice of this verb's
// rule says so.
func TestTextTierScoresAsThePin(t *testing.T) {
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
			kept := tc.score >= 0.3 // minTextScore, written as a literal so the test does not read the value it checks
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

func TestTextTierTermsAreDistinctAndBounded(t *testing.T) {
	if got, want := searchTermsOf(t, "Alpha  alpha\tBETA\ngamma ALPHA"), []string{"alpha", "beta", "gamma"}; !slices.Equal(got, want) {
		t.Errorf("terms = %q, want %q", got, want)
	}

	var atLimit []string
	for i := 1; i <= 32; i++ {
		atLimit = append(atLimit, fmt.Sprintf("t%02d", i))
	}
	thirtyTwo := strings.Join(atLimit, " ")
	if got := searchTermsOf(t, thirtyTwo+" T32 t01"); len(got) != 32 {
		t.Errorf("32 distinct terms with repeats gave %d terms, want 32", len(got))
	}
	for _, query := range []string{thirtyTwo + " t33", "", " \t\n"} {
		if terms, err := checkedSearchTerms(query); err == nil || !strings.Contains(err.Error(), "query") {
			t.Errorf("checkedSearchTerms(%q) = %q, %v; want an error naming query", query, terms, err)
		}
	}

	// A query of about 1 MB, every term distinct: the split stops at the first term over the bound.
	var long strings.Builder
	for i := 0; long.Len() < 1<<20; i++ {
		fmt.Fprintf(&long, "t%d ", i)
	}
	if _, err := checkedSearchTerms(long.String()); err == nil || !strings.Contains(err.Error(), "query") {
		t.Errorf("a 1 MB query: %v, want an error naming query", err)
	}
	if got := len(searchTerms(long.String())); got != 33 {
		t.Errorf("searchTerms of a 1 MB query returned %d terms, want 33", got)
	}
}

func TestTextTierOrderIsDeterministic(t *testing.T) {
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
