package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewClassifierChain_Constructor: a chain builds with or without its keyword tier.
func TestNewClassifierChain_Constructor(t *testing.T) {
	for name, keyword := range map[string]*KeywordClassifier{
		"keyword classifier": NewKeywordClassifier(),
		"nil keyword":        nil,
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotNil(t, NewClassifierChain(keyword))
		})
	}
}

// TestClassifierChain_T0_KeywordMatch tests that keyword patterns answer at tier 0
func TestClassifierChain_T0_KeywordMatch(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectTier     int
		expectIntent   string
		validateResult func(t *testing.T, result *ClassificationResult, before, after time.Time)
	}{
		{
			name:         "yesterday temporal keyword",
			query:        "events from yesterday",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult, before, after time.Time) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				assertCalendarDay(t, result.Options["time_range"], before, after, -1)
			},
		},
		{
			name:         "today temporal keyword",
			query:        "show data from today",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult, before, after time.Time) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				assertCalendarDay(t, result.Options["time_range"], before, after, 0)
			},
		},
		{
			name:         "last week temporal keyword",
			query:        "deliveries from last week",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult, before, after time.Time) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				assertSpanEndingNow(t, result.Options["time_range"], before, after, 7*24*time.Hour)
			},
		},
		{
			name:         "similarity keyword",
			query:        "find items similar to ABC-123",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult, _, _ time.Time) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				// Should have UseEmbeddings set
				useEmb, ok := result.Options["use_embeddings"].(bool)
				assert.True(t, ok)
				assert.True(t, useEmb)
			},
		},
		{
			name:         "path keyword - connected to",
			query:        "devices connected to sensor-001",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult, _, _ time.Time) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				// Should have PathIntent and PathStartNode
				pathIntent, ok := result.Options["path_intent"].(bool)
				assert.True(t, ok)
				assert.True(t, pathIntent)
				assert.Equal(t, "sensor-001", result.Options["path_start_node"])
			},
		},
		{
			name:         "zone keyword",
			query:        "show items in zone-A",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult, _, _ time.Time) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				// Should have path intent with zone as start node
				pathIntent, ok := result.Options["path_intent"].(bool)
				assert.True(t, ok)
				assert.True(t, pathIntent)
				assert.Equal(t, "zone-A", result.Options["path_start_node"])
			},
		},
		{
			name:         "spatial keyword - within",
			query:        "devices within 5km of 40.7128,-74.0060",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult, _, _ time.Time) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				// Should have GeoBounds set
				assert.NotNil(t, result.Options["geo_bounds"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyword := NewKeywordClassifier()
			chain := NewClassifierChain(keyword)

			ctx := t.Context()
			before := time.Now()
			result, err := chain.ClassifyQuery(ctx, tt.query)
			after := time.Now()
			require.NoError(t, err)

			tt.validateResult(t, result, before, after)
		})
	}
}

// TestClassifierChain_KeywordOnly tests T0-only chain
func TestClassifierChain_KeywordOnly(t *testing.T) {
	keyword := NewKeywordClassifier()
	chain := NewClassifierChain(keyword)

	tests := []struct {
		name           string
		query          string
		validateResult func(t *testing.T, result *ClassificationResult)
	}{
		{
			name:  "keyword match - should work",
			query: "events from yesterday",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
			},
		},
		{
			name:  "no keyword match - should return default",
			query: "generic query without keywords",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should return default result (T0 with no options set)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			result, err := chain.ClassifyQuery(ctx, tt.query)
			require.NoError(t, err)
			tt.validateResult(t, result)
		})
	}
}

// TestClassifierChain_DefaultResult tests fallback when no tier matches
func TestClassifierChain_DefaultResult(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		validateResult func(t *testing.T, result *ClassificationResult)
	}{
		{
			name:  "no keywords",
			query: "completely unrelated topic about weather patterns",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should return default (T0 result with no filters)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				assert.Equal(t, "", result.Intent)
			},
		},
		{
			name:  "empty query",
			query: "",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should return default for empty query
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
			},
		},
		{
			name:  "whitespace only query",
			query: "   ",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should return default for whitespace
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyword := NewKeywordClassifier()
			chain := NewClassifierChain(keyword)

			ctx := t.Context()
			result, err := chain.ClassifyQuery(ctx, tt.query)
			require.NoError(t, err)

			tt.validateResult(t, result)
		})
	}
}

// TestClassifierChain_ContextCancellation: a context that has already ended,
// by cancel or by deadline, yields nil, never the default result. The deadline
// below is already in the past, so no wait is needed.
func TestClassifierChain_ContextCancellation(t *testing.T) {
	keyword := NewKeywordClassifier()
	chain := NewClassifierChain(keyword)

	t.Run("cancelled context before call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		result, err := chain.ClassifyQuery(ctx, "test query")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("context deadline already passed", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()

		result, err := chain.ClassifyQuery(ctx, "test query")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

// TestClassifierChain_NilChain tests nil chain handling
func TestClassifierChain_NilChain(t *testing.T) {
	var chain *ClassifierChain
	ctx := t.Context()

	// Should not panic on nil chain
	result, err := chain.ClassifyQuery(ctx, "test query")
	require.NoError(t, err)

	// Should return nil for nil chain
	assert.Nil(t, result)
}

// TestClassifierChain_TierPriority tests that a keyword hint answers at tier 0
func TestClassifierChain_TierPriority(t *testing.T) {
	keyword := NewKeywordClassifier()
	chain := NewClassifierChain(keyword)

	ctx := t.Context()

	before := time.Now()
	result, err := chain.ClassifyQuery(ctx, "show deliveries from yesterday")
	after := time.Now()
	require.NoError(t, err)

	require.NotNil(t, result)
	// Should use T0 (keyword)
	assert.Equal(t, 0, result.Tier)
	assert.Equal(t, 1.0, result.Confidence)
	// Should have temporal filter from keyword
	assertCalendarDay(t, result.Options["time_range"], before, after, -1)
}

// TestClassifierChain_ResultStructure tests ClassificationResult structure
func TestClassifierChain_ResultStructure(t *testing.T) {
	keyword := NewKeywordClassifier()
	chain := NewClassifierChain(keyword)

	tests := []struct {
		name           string
		query          string
		validateResult func(t *testing.T, result *ClassificationResult)
	}{
		{
			name:  "T0 result has correct structure",
			query: "events from yesterday",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				assert.NotNil(t, result.Options)
				// Tier 0 doesn't set Intent (uses SearchOptions fields instead)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			result, err := chain.ClassifyQuery(ctx, tt.query)
			require.NoError(t, err)
			tt.validateResult(t, result)
		})
	}
}

// TestClassifierChain_OptionsPopulation tests that Options are populated correctly
func TestClassifierChain_OptionsPopulation(t *testing.T) {
	keyword := NewKeywordClassifier()
	chain := NewClassifierChain(keyword)

	ctx := t.Context()

	t.Run("keyword options", func(t *testing.T) {
		result, err := chain.ClassifyQuery(ctx, "find similar items")
		require.NoError(t, err)

		require.NotNil(t, result)
		assert.Equal(t, 0, result.Tier)
		// Should have use_embeddings from similarity keyword
		useEmb, ok := result.Options["use_embeddings"].(bool)
		assert.True(t, ok)
		assert.True(t, useEmb)
	})
}

// TestClassifierChain_ConcurrentAccess tests thread safety
func TestClassifierChain_ConcurrentAccess(t *testing.T) {
	keyword := NewKeywordClassifier()
	chain := NewClassifierChain(keyword)

	ctx := t.Context()

	// Run many concurrent classifications
	const numGoroutines = 10
	const numIterations = 20

	queries := []string{
		"events from yesterday",
		"Where is shipment ABC-123?",
		"show delayed deliveries",
		"find similar items",
		"devices in zone-A",
	}

	doneCh := make(chan bool, numGoroutines)

	for range numGoroutines {
		go func() {
			for j := 0; j < numIterations; j++ {
				query := queries[j%len(queries)]
				result, err := chain.ClassifyQuery(ctx, query)
				// Should not panic and should return valid result. assert, not
				// require: FailNow must not run off the test goroutine.
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
			doneCh <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-doneCh
	}
}

// TestClassifierChain_UnicodeQueries tests handling of non-ASCII characters
func TestClassifierChain_UnicodeQueries(t *testing.T) {
	keyword := NewKeywordClassifier()
	chain := NewClassifierChain(keyword)

	ctx := t.Context()

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "french text",
			query: "Où est mon colis de hier?", // "hier" = yesterday
		},
		{
			name:  "chinese text",
			query: "显示昨天的事件",
		},
		{
			name:  "emoji in query",
			query: "show deliveries 🚚📦 from yesterday",
		},
		{
			name:  "mixed scripts",
			query: "show yesterday データ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic on unicode
			result, err := chain.ClassifyQuery(ctx, tt.query)
			require.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}
