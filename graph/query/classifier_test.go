package query

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeywordClassifier_Aggregation verifies that aggregation intents are extracted from queries.
func TestKeywordClassifier_Aggregation(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		wantAggType   string
		wantAggField  string
		wantNoAggType bool // true when we expect NO aggregation (e.g. ranking-only)
	}{
		{
			name:         "how many — count intent",
			query:        "How many sensors are active?",
			wantAggType:  AggregationCount,
			wantAggField: "",
		},
		{
			name:         "total number of — count intent",
			query:        "Total number of entities",
			wantAggType:  AggregationCount,
			wantAggField: "",
		},
		{
			name:         "count keyword — count intent",
			query:        "Count all drones in zone-alpha",
			wantAggType:  AggregationCount,
			wantAggField: "",
		},
		{
			name:         "average — avg intent with field",
			query:        "What is the average temperature?",
			wantAggType:  AggregationAvg,
			wantAggField: "temperature",
		},
		{
			name:         "avg abbreviation",
			query:        "avg response time for requests",
			wantAggType:  AggregationAvg,
			wantAggField: "response",
		},
		{
			name:         "mean keyword",
			query:        "Show me the mean pressure reading",
			wantAggType:  AggregationAvg,
			wantAggField: "pressure",
		},
		{
			name:         "sum keyword",
			query:        "sum all readings",
			wantAggType:  AggregationSum,
			wantAggField: "all",
		},
		{
			name:         "total — sum intent (no number-of suffix)",
			query:        "total payload weight",
			wantAggType:  AggregationSum,
			wantAggField: "payload",
		},
		{
			name:         "minimum keyword",
			query:        "minimum battery level across drones",
			wantAggType:  AggregationMin,
			wantAggField: "battery",
		},
		{
			name:         "min abbreviation",
			query:        "min response time for api calls",
			wantAggType:  AggregationMin,
			wantAggField: "response",
		},
		{
			name:         "maximum keyword",
			query:        "maximum temperature in zone-alpha",
			wantAggType:  AggregationMax,
			wantAggField: "temperature",
		},
		{
			name:         "max abbreviation",
			query:        "max speed recorded today",
			wantAggType:  AggregationMax,
			wantAggField: "speed",
		},
		// "total number of" must resolve to count, not sum.
		{
			name:         "total number of binds to count not sum",
			query:        "what is the total number of active sensors?",
			wantAggType:  AggregationCount,
			wantAggField: "",
		},
		// Ranking queries carry no aggregation type.
		{
			name:          "top N — ranking only, no aggregation type",
			query:         "Top 5 sensors by reading",
			wantNoAggType: true,
		},
		{
			name:          "most — ranking only, no aggregation type",
			query:         "most active drones",
			wantNoAggType: true,
		},
	}

	k := NewKeywordClassifier()
	ctx := t.Context()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := k.ClassifyQuery(ctx, tt.query)
			require.NotNil(t, opts)

			if tt.wantNoAggType {
				assert.Empty(t, opts.AggregationType, "expected no AggregationType for ranking-only query")
			} else {
				assert.Equal(t, tt.wantAggType, opts.AggregationType)
				assert.Equal(t, tt.wantAggField, opts.AggregationField)
			}
		})
	}
}

// TestKeywordClassifier_Ranking verifies that ranking intents are extracted from queries.
func TestKeywordClassifier_Ranking(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantRanking bool
		wantLimit   int
	}{
		{
			name:        "top N — explicit limit",
			query:       "Top 5 sensors by reading",
			wantRanking: true,
			wantLimit:   5,
		},
		{
			name:        "top N — different number",
			query:       "top 10 fastest drones",
			wantRanking: true,
			wantLimit:   10,
		},
		{
			name:        "bottom N — explicit limit",
			query:       "bottom 3 performing nodes",
			wantRanking: true,
			wantLimit:   3,
		},
		{
			name:        "most — no explicit limit",
			query:       "most active devices in zone-B",
			wantRanking: true,
			wantLimit:   0,
		},
		{
			name:        "least — no explicit limit",
			query:       "least responsive nodes",
			wantRanking: true,
			wantLimit:   0,
		},
		{
			name:        "no ranking in regular query",
			query:       "Show all sensors in zone-alpha",
			wantRanking: false,
			wantLimit:   0,
		},
	}

	k := NewKeywordClassifier()
	ctx := t.Context()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := k.ClassifyQuery(ctx, tt.query)
			require.NotNil(t, opts)

			assert.Equal(t, tt.wantRanking, opts.RankingIntent)
			if tt.wantLimit > 0 {
				assert.Equal(t, tt.wantLimit, opts.Limit)
			}
		})
	}
}

// TestKeywordClassifier_AggregationWithTemporal verifies that aggregation and temporal
// intents can coexist in a single query.
func TestKeywordClassifier_AggregationWithTemporal(t *testing.T) {
	k := NewKeywordClassifier()
	ctx := t.Context()

	before := time.Now()
	opts := k.ClassifyQuery(ctx, "How many sensors were active yesterday?")
	after := time.Now()
	require.NotNil(t, opts)

	assert.Equal(t, AggregationCount, opts.AggregationType, "should detect count aggregation")
	assertCalendarDay(t, opts.TimeRange, before, after, -1)
}

// TestKeywordClassifier_AggregationWithSpatial verifies that aggregation and spatial
// intents can coexist.
func TestKeywordClassifier_AggregationWithSpatial(t *testing.T) {
	k := NewKeywordClassifier()
	ctx := t.Context()

	opts := k.ClassifyQuery(ctx, "Count all drones in zone-alpha")
	require.NotNil(t, opts)

	assert.Equal(t, AggregationCount, opts.AggregationType, "should detect count aggregation")
	assert.True(t, opts.PathIntent, "should detect spatial zone intent")
}

// TestKeywordClassifier_ExistingPatternsUnaffected ensures that the addition of
// aggregation patterns does not break any pre-existing keyword extraction.
func TestKeywordClassifier_ExistingPatternsUnaffected(t *testing.T) {
	k := NewKeywordClassifier()
	ctx := t.Context()

	t.Run("temporal — yesterday", func(t *testing.T) {
		before := time.Now()
		opts := k.ClassifyQuery(ctx, "events from yesterday")
		after := time.Now()
		require.NotNil(t, opts)
		assertCalendarDay(t, opts.TimeRange, before, after, -1)
		assert.Empty(t, opts.AggregationType)
	})

	t.Run("temporal — last N days", func(t *testing.T) {
		before := time.Now()
		opts := k.ClassifyQuery(ctx, "data from last 7 days")
		after := time.Now()
		require.NotNil(t, opts)
		assertSpanEndingNow(t, opts.TimeRange, before, after, 7*24*time.Hour)
		assert.Empty(t, opts.AggregationType)
	})

	t.Run("similarity intent", func(t *testing.T) {
		opts := k.ClassifyQuery(ctx, "find items similar to sensor-001")
		require.NotNil(t, opts)
		assert.True(t, opts.UseEmbeddings)
		assert.Empty(t, opts.AggregationType)
	})

	t.Run("path intent — connected to", func(t *testing.T) {
		opts := k.ClassifyQuery(ctx, "devices connected to sensor-001")
		require.NotNil(t, opts)
		assert.True(t, opts.PathIntent)
		assert.Equal(t, "sensor-001", opts.PathStartNode)
		assert.Empty(t, opts.AggregationType)
	})

	t.Run("zone intent", func(t *testing.T) {
		opts := k.ClassifyQuery(ctx, "show items in zone-A")
		require.NotNil(t, opts)
		assert.True(t, opts.PathIntent)
		assert.Equal(t, "zone-A", opts.PathStartNode)
		assert.Empty(t, opts.AggregationType)
	})

	t.Run("spatial — within radius", func(t *testing.T) {
		opts := k.ClassifyQuery(ctx, "devices within 5km of 40.7128,-74.0060")
		require.NotNil(t, opts)
		assert.NotNil(t, opts.GeoBounds)
		assert.Empty(t, opts.AggregationType)
	})
}

// TestKeywordClassifier_EmptyQuery confirms that an empty query returns non-nil SearchOptions
// with no aggregation fields set.
func TestKeywordClassifier_EmptyQuery(t *testing.T) {
	k := NewKeywordClassifier()
	ctx := t.Context()

	opts := k.ClassifyQuery(ctx, "")
	require.NotNil(t, opts)
	assert.Empty(t, opts.AggregationType)
	assert.Empty(t, opts.AggregationField)
	assert.False(t, opts.RankingIntent)
}
