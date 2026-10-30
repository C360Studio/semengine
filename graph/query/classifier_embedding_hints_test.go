package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQueryEmbeddingIsReadOnly proves that classifying a query leaves the example classifier
// as it was (#96, design D3): the score for a question does not depend on the questions asked
// before it. The oracle is a second classifier built from the same examples that is asked
// nothing else.
func TestQueryEmbeddingIsReadOnly(t *testing.T) {
	domains := []*DomainExamples{{
		Domain: "iot",
		Examples: []Example{
			{Query: "Show temperature readings for every sensor", Intent: "sensor_query"},
			{Query: "List pressure alarms raised by pumps", Intent: "alarm_query"},
			{Query: "Where is shipment ABC-123?", Intent: "entity_lookup"},
			{Query: "Open doors in building seven", Intent: "door_query"},
		},
	}}
	const question = "temperature alarms for pumps"
	// Traffic whose words overlap the question unevenly, so folding it into the corpus
	// statistics would move the question's word weights.
	traffic := []string{
		"temperature temperature temperature",
		"temperature of the boiler",
		"temperature trend for sensors",
		"hourly temperature report",
		"temperature alarms",
	}

	quiet := mustNewEmbeddingClassifier(t, domains, 0)
	busy := mustNewEmbeddingClassifier(t, domains, 0)

	// Before any traffic, the two classifiers agree.
	beforeMatch, beforeScore := busy.FindBestMatch(t.Context(), question)
	require.NotNil(t, beforeMatch)

	for range 4 {
		for _, q := range traffic {
			busy.FindBestMatch(t.Context(), q)
		}
	}

	gotMatch, gotScore := busy.FindBestMatch(t.Context(), question)
	wantMatch, wantScore := quiet.FindBestMatch(t.Context(), question)

	require.NotNil(t, gotMatch)
	require.NotNil(t, wantMatch)
	assert.Equal(t, wantScore, gotScore, "intervening queries changed the score")
	assert.Equal(t, wantMatch.Query, gotMatch.Query, "intervening queries changed the match")
	assert.Equal(t, beforeScore, gotScore, "the same classifier scored the same question differently")
	assert.Equal(t, beforeMatch.Query, gotMatch.Query)

	t.Run("construction still learns", func(t *testing.T) {
		// The same last example after two different corpora. If construction did not fold the
		// examples into the statistics, every word would weigh the same and both classifiers
		// would score the question alike.
		const last = "temperature alarms raised by pumps"
		shared := []Example{
			{Query: "Where is shipment ABC-123?", Intent: "entity_lookup"},
			{Query: "Open doors in building seven", Intent: "door_query"},
			{Query: "Battery level of drone four", Intent: "battery_query"},
			{Query: "Delivery route for truck nine", Intent: "route_query"},
		}
		withAlarms := append(append([]Example{}, shared...),
			Example{Query: "pump alarms in hall two", Intent: "alarm_query"},
			Example{Query: last, Intent: "target"})
		withoutAlarms := append(append([]Example{}, shared...),
			Example{Query: "shipment doors in hall two", Intent: "other"},
			Example{Query: last, Intent: "target"})

		a := mustNewEmbeddingClassifier(t, []*DomainExamples{{Domain: "a", Examples: withAlarms}}, 0)
		b := mustNewEmbeddingClassifier(t, []*DomainExamples{{Domain: "b", Examples: withoutAlarms}}, 0)

		matchA, scoreA := a.FindBestMatch(t.Context(), question)
		matchB, scoreB := b.FindBestMatch(t.Context(), question)
		require.NotNil(t, matchA)
		require.NotNil(t, matchB)
		assert.Equal(t, "target", matchA.Intent)
		assert.Equal(t, "target", matchB.Intent)
		assert.NotEqual(t, scoreA, scoreB, "the examples did not shape the word weights")
	})
}

// hintFile writes one domain example file holding a single example with the given options
// JSON and loads it the way a product does.
func hintFile(t *testing.T, domain, query, optionsJSON string) *DomainExamples {
	t.Helper()
	body := `{"domain":` + quoteJSON(t, domain) + `,"version":"1.0","examples":[` +
		`{"query":` + quoteJSON(t, query) + `,"intent":"hinted","options":` + optionsJSON + `}]}`
	path := filepath.Join(t.TempDir(), domain+".json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	d, err := LoadDomainExamples(path)
	require.NoError(t, err)
	return d
}

func quoteJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	require.NoError(t, err)
	return string(b)
}

// TestTierOneHintsAreTyped proves that tier 1 gives a known hint the Go type tiers 0 and 3
// give it, keeps a caller's own names unchanged, keeps its own copies, and refuses an
// impossible hint at construction (#478, design D4).
func TestTierOneHintsAreTyped(t *testing.T) {
	const question = "Show sensor readings for the north site"

	t.Run("a time range from an example file", func(t *testing.T) {
		d := hintFile(t, "iot", question, `{
			"time_range": {"start": "2026-01-01T00:00:00Z", "end": "2026-01-02T00:00:00Z"},
			"types": ["sensor"], "limit": 5, "site": "north"}`)
		c, err := NewEmbeddingClassifier(t.Context(), []*DomainExamples{d}, 0.3)
		require.NoError(t, err)

		got, err := NewClassifierChain(nil, c).ClassifyQuery(t.Context(), question)
		require.NoError(t, err)

		require.NotNil(t, got)
		require.Equal(t, 1, got.Tier)
		want := map[string]any{
			"time_range": &TimeRange{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			"types": []string{"sensor"},
			"limit": 5,
			"site":  "north",
		}
		require.Len(t, got.Options, len(want))
		tr, ok := got.Options["time_range"].(*TimeRange)
		require.True(t, ok, "time_range is %T", got.Options["time_range"])
		assert.True(t, tr.Start.Equal(want["time_range"].(*TimeRange).Start))
		assert.True(t, tr.End.Equal(want["time_range"].(*TimeRange).End))
		assert.Equal(t, want["types"], got.Options["types"])
		assert.Equal(t, want["limit"], got.Options["limit"])
		assert.Equal(t, want["site"], got.Options["site"])
	})

	t.Run("the classifier keeps its own copies", func(t *testing.T) {
		labels := []any{"a", "b"}
		domains := []*DomainExamples{{Domain: "iot", Examples: []Example{{
			Query:   question,
			Intent:  "hinted",
			Options: map[string]any{"types": []any{"sensor"}, "labels": labels},
		}}}}
		c, err := NewEmbeddingClassifier(t.Context(), domains, 0.3)
		require.NoError(t, err)

		// The caller edits its own example after construction.
		domains[0].Examples[0].Options["types"] = []any{"pump"}
		domains[0].Examples[0].Options["added"] = true
		delete(domains[0].Examples[0].Options, "labels")

		match, _ := c.FindBestMatch(t.Context(), question)
		require.NotNil(t, match)
		assert.Equal(t, map[string]any{"types": []string{"sensor"}, "labels": []any{"a", "b"}}, match.Options)
	})

	refusals := []struct {
		name    string
		option  string
		options string
	}{
		{"strategy outside the eight", "strategy", `{"strategy": "fastest"}`},
		{"empty strategy", "strategy", `{"strategy": ""}`},
		{"time range ending before it starts", "time_range",
			`{"time_range": {"start": "2026-01-02T00:00:00Z", "end": "2026-01-01T00:00:00Z"}}`},
		{"time range with no end", "time_range", `{"time_range": {"start": "2026-01-01T00:00:00Z"}}`},
		{"time range with no start", "time_range", `{"time_range": {"end": "2026-01-01T00:00:00Z"}}`},
		{"null time range", "time_range", `{"time_range": null}`},
		{"time range as a word", "time_range", `{"time_range": "week"}`},
		{"geo bounds with North 95", "geo_bounds", `{"geo_bounds": {"north": 95, "south": 10, "east": 20, "west": 0}}`},
		{"geo bounds with South -91", "geo_bounds", `{"geo_bounds": {"north": 10, "south": -91, "east": 20, "west": 0}}`},
		{"geo bounds with South above North", "geo_bounds", `{"geo_bounds": {"north": 10, "south": 20, "east": 20, "west": 0}}`},
		{"null geo bounds", "geo_bounds", `{"geo_bounds": null}`},
		{"negative limit", "limit", `{"limit": -1}`},
		{"fractional limit", "limit", `{"limit": 2.5}`},
		{"types as a string", "types", `{"types": "sensor"}`},
		{"null limit", "limit", `{"limit": null}`},
		{"null types", "types", `{"types": null}`},
		{"null strategy", "strategy", `{"strategy": null}`},
		{"null use_embeddings", "use_embeddings", `{"use_embeddings": null}`},
	}
	for _, tt := range refusals {
		t.Run("refuses "+tt.name, func(t *testing.T) {
			const domain = "plant-floor"
			const example = "Show pump readings"
			d := hintFile(t, domain, example, tt.options)

			c, err := NewEmbeddingClassifier(t.Context(), []*DomainExamples{d}, 0.3)

			require.Error(t, err)
			assert.Nil(t, c)
			for _, name := range []string{domain, example, tt.option} {
				assert.Contains(t, err.Error(), name)
			}
		})
	}

	// An example built in code can hold a typed nil or a raw JSON null; each encodes as JSON null
	// and is refused under a known name like the file's null. A caller's own name passes through.
	typedNulls := []struct {
		name   string
		option string
		value  any
	}{
		{"nil *int limit", "limit", (*int)(nil)},
		{"nil []string types", "types", []string(nil)},
		{"raw null use_embeddings", "use_embeddings", json.RawMessage("null")},
		{"nil *TimeRange time_range", "time_range", (*TimeRange)(nil)},
	}
	for _, tt := range typedNulls {
		t.Run("refuses "+tt.name, func(t *testing.T) {
			d := &DomainExamples{Domain: "plant-floor", Version: "1.0", Examples: []Example{
				{Query: "Show pump readings", Intent: "hinted", Options: map[string]any{tt.option: tt.value}},
			}}

			c, err := NewEmbeddingClassifier(t.Context(), []*DomainExamples{d}, 0.3)

			require.Error(t, err)
			assert.Nil(t, c)
			for _, name := range []string{"plant-floor", "Show pump readings", tt.option} {
				assert.Contains(t, err.Error(), name)
			}
		})
	}
	t.Run("a caller's own null passes through", func(t *testing.T) {
		d := &DomainExamples{Domain: "plant-floor", Version: "1.0", Examples: []Example{
			{Query: question, Intent: "hinted", Options: map[string]any{"site": json.RawMessage("null")}},
		}}
		c, err := NewEmbeddingClassifier(t.Context(), []*DomainExamples{d}, 0.3)
		require.NoError(t, err)
		match, _ := c.FindBestMatch(t.Context(), question)
		require.NotNil(t, match)
		assert.Equal(t, json.RawMessage("null"), match.Options["site"])
	})

	t.Run("pole and antimeridian boxes are accepted", func(t *testing.T) {
		d := hintFile(t, "geo", question,
			`{"geo_bounds": {"north": 90, "south": -90, "east": 190, "west": -180}, "limit": 0}`)
		c, err := NewEmbeddingClassifier(t.Context(), []*DomainExamples{d}, 0.3)
		require.NoError(t, err)
		match, _ := c.FindBestMatch(t.Context(), question)
		require.NotNil(t, match)
		assert.Equal(t, &SpatialBounds{North: 90, South: -90, East: 190, West: -180}, match.Options["geo_bounds"])
		assert.Equal(t, 0, match.Options["limit"])
	})
}

// TestEveryHintNameIsTyped walks SearchOptions' JSON names by reflection, so a field added
// without its decoding fails here. Each name, given a valid value in an example file, comes
// back from tier 1 as the field's own Go type (strategy as a string); given a value of the
// wrong JSON kind, it refuses construction.
func TestEveryHintNameIsTyped(t *testing.T) {
	// A valid JSON value for each name and the Go value tier 1 must give for it. The oracle is
	// written by hand from the field's declared type.
	valid := map[string]struct {
		json string
		want any
	}{
		"query": {`"pump faults"`, "pump faults"},
		"geo_bounds": {`{"north": 10, "south": -10, "east": 20, "west": -20}`,
			&SpatialBounds{North: 10, South: -10, East: 20, West: -20}},
		"time_range": {`{"start": "2026-03-01T00:00:00Z", "end": "2026-03-02T00:00:00Z"}`,
			&TimeRange{Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)}},
		"predicates":        {`["iot.sensor.reading"]`, []string{"iot.sensor.reading"}},
		"types":             {`["sensor", "pump"]`, []string{"sensor", "pump"}},
		"key_terms":         {`["fault"]`, []string{"fault"}},
		"use_embeddings":    {`true`, true},
		"strategy":          {`"pathrag"`, "pathrag"},
		"limit":             {`7`, 7},
		"path_intent":       {`true`, true},
		"path_start_node":   {`"pump-42"`, "pump-42"},
		"path_predicates":   {`["iot.pump.feeds"]`, []string{"iot.pump.feeds"}},
		"aggregation_type":  {`"avg"`, "avg"},
		"aggregation_field": {`"pressure"`, "pressure"},
		"ranking_intent":    {`true`, true},
	}

	const question = "Show pump faults on the north line"
	fields := reflect.VisibleFields(reflect.TypeFor[SearchOptions]())
	require.NotEmpty(t, fields)
	for _, field := range fields {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		require.NotEmpty(t, name, "field %s has no JSON name", field.Name)

		t.Run(name, func(t *testing.T) {
			v, ok := valid[name]
			require.True(t, ok, "SearchOptions.%s (%q) has no valid value here: add one", field.Name, name)

			wantType := field.Type
			if name == "strategy" {
				wantType = reflect.TypeFor[string]()
			}
			require.Equal(t, wantType, reflect.TypeOf(v.want), "the oracle for %q has the wrong type", name)

			d := hintFile(t, "hints", question, `{"`+name+`": `+v.json+`}`)
			c, err := NewEmbeddingClassifier(t.Context(), []*DomainExamples{d}, 0.3)
			require.NoError(t, err)
			match, _ := c.FindBestMatch(t.Context(), question)
			require.NotNil(t, match)
			got := match.Options[name]
			assert.Equal(t, wantType, reflect.TypeOf(got), "%q decoded to %T", name, got)
			assert.Equal(t, v.want, got)

			// A value of the wrong JSON kind for this field.
			wrong := `"not-a-` + name + `"`
			if field.Type.Kind() == reflect.String {
				wrong = `42`
			}
			bad := hintFile(t, "hints", question, `{"`+name+`": `+wrong+`}`)
			c, err = NewEmbeddingClassifier(t.Context(), []*DomainExamples{bad}, 0.3)
			require.Error(t, err, "%q accepted %s", name, wrong)
			assert.Nil(t, c)
			assert.Contains(t, err.Error(), name)
		})
	}
}
