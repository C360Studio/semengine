package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewClassifierChain_Constructor tests chain construction with various inputs
func TestNewClassifierChain_Constructor(t *testing.T) {
	tests := []struct {
		name      string
		keyword   *KeywordClassifier
		embedding *EmbeddingClassifier
		validate  func(t *testing.T, chain *ClassifierChain)
	}{
		{
			name:      "both classifiers provided",
			keyword:   NewKeywordClassifier(),
			embedding: createTestEmbeddingClassifier(t),
			validate: func(t *testing.T, chain *ClassifierChain) {
				assert.NotNil(t, chain)
				// Should have both classifiers available
			},
		},
		{
			name:      "keyword only (no embedding)",
			keyword:   NewKeywordClassifier(),
			embedding: nil,
			validate: func(t *testing.T, chain *ClassifierChain) {
				assert.NotNil(t, chain)
				// Should work with only T0 available
			},
		},
		{
			name:      "nil keyword (unusual but should handle)",
			keyword:   nil,
			embedding: createTestEmbeddingClassifier(t),
			validate: func(t *testing.T, chain *ClassifierChain) {
				assert.NotNil(t, chain)
				// Should still work with only T1/T2 available
			},
		},
		{
			name:      "both nil (edge case)",
			keyword:   nil,
			embedding: nil,
			validate: func(t *testing.T, chain *ClassifierChain) {
				assert.NotNil(t, chain)
				// Should not panic, but may return defaults
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := NewClassifierChain(tt.keyword, tt.embedding)
			tt.validate(t, chain)
		})
	}
}

// TestClassifierChain_T0_KeywordMatch tests that keyword patterns bypass embedding
func TestClassifierChain_T0_KeywordMatch(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectTier     int
		expectIntent   string
		validateResult func(t *testing.T, result *ClassificationResult)
	}{
		{
			name:         "yesterday temporal keyword",
			query:        "events from yesterday",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				// Should have TimeRange set
				assert.NotNil(t, result.Options["time_range"])
			},
		},
		{
			name:         "today temporal keyword",
			query:        "show data from today",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				assert.NotNil(t, result.Options["time_range"])
			},
		},
		{
			name:         "last week temporal keyword",
			query:        "deliveries from last week",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				assert.NotNil(t, result.Options["time_range"])
			},
		},
		{
			name:         "similarity keyword",
			query:        "find items similar to ABC-123",
			expectTier:   0,
			expectIntent: "",
			validateResult: func(t *testing.T, result *ClassificationResult) {
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
			validateResult: func(t *testing.T, result *ClassificationResult) {
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
			validateResult: func(t *testing.T, result *ClassificationResult) {
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
			validateResult: func(t *testing.T, result *ClassificationResult) {
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
			embedding := createTestEmbeddingClassifier(t)
			chain := NewClassifierChain(keyword, embedding)

			ctx := t.Context()
			result := chain.ClassifyQuery(ctx, tt.query)

			tt.validateResult(t, result)
		})
	}
}

// TestClassifierChain_T1T2_EmbeddingMatch tests embedding tier when no keywords match
func TestClassifierChain_T1T2_EmbeddingMatch(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		validateResult func(t *testing.T, result *ClassificationResult)
	}{
		{
			name:  "query matches example - no keywords",
			query: "Where is shipment ABC-123?", // Exact match in test examples
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should use embedding tier (T1 or T2)
				assert.True(t, result.Tier == 1 || result.Tier == 2)
				assert.Equal(t, "entity_lookup", result.Intent)
				// Confidence should be from similarity score, not 1.0
				assert.Greater(t, result.Confidence, 0.5)
				// Should have options from matched example
				assert.NotNil(t, result.Options)
			},
		},
		{
			name:  "similar query - no exact keyword",
			query: "Where is my package?", // Similar to "Where is shipment..."
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should try embedding tier
				// May match or fall back to default depending on threshold
				assert.NotNil(t, result)
			},
		},
		{
			name:  "delayed deliveries query",
			query: "Show delayed deliveries this week", // Has "this week" but also matches example
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// "this week" is a keyword, should hit T0
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyword := NewKeywordClassifier()
			embedding := createTestEmbeddingClassifier(t)
			chain := NewClassifierChain(keyword, embedding)

			ctx := t.Context()
			result := chain.ClassifyQuery(ctx, tt.query)

			tt.validateResult(t, result)
		})
	}
}

// TestClassifierChain_NoEmbedding_KeywordOnly tests T0-only chain
func TestClassifierChain_NoEmbedding_KeywordOnly(t *testing.T) {
	keyword := NewKeywordClassifier()
	chain := NewClassifierChain(keyword, nil) // No embedding classifier

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
			result := chain.ClassifyQuery(ctx, tt.query)
			tt.validateResult(t, result)
		})
	}
}

// TestClassifierChain_DefaultResult tests fallback when no tier matches
func TestClassifierChain_DefaultResult(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		threshold      float64
		validateResult func(t *testing.T, result *ClassificationResult)
	}{
		{
			name:      "no keywords, no embedding match",
			query:     "completely unrelated topic about weather patterns",
			threshold: 0.9, // High threshold to prevent matching
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should return default (T0 result with no filters)
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
				assert.Equal(t, "", result.Intent)
			},
		},
		{
			name:      "empty query",
			query:     "",
			threshold: 0.5,
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should return default for empty query
				assert.Equal(t, 0, result.Tier)
				assert.Equal(t, 1.0, result.Confidence)
			},
		},
		{
			name:      "whitespace only query",
			query:     "   ",
			threshold: 0.5,
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
			embedding := createTestEmbeddingClassifierWithThreshold(t, tt.threshold)
			chain := NewClassifierChain(keyword, embedding)

			ctx := t.Context()
			result := chain.ClassifyQuery(ctx, tt.query)

			tt.validateResult(t, result)
		})
	}
}

// TestClassifierChain_ContextCancellation: a context that has already ended,
// by cancel or by deadline, yields nil (design D4). The pin accepted either nil
// or the default result here and waited out a 1ns timeout with a sleep; the
// deadline below is already in the past, so no wait is needed.
func TestClassifierChain_ContextCancellation(t *testing.T) {
	keyword := NewKeywordClassifier()
	embedding := createTestEmbeddingClassifier(t)
	chain := NewClassifierChain(keyword, embedding)

	t.Run("cancelled context before call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		assert.Nil(t, chain.ClassifyQuery(ctx, "test query"))
	})

	t.Run("context deadline already passed", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()

		assert.Nil(t, chain.ClassifyQuery(ctx, "test query"))
	})
}

// TestClassifierChain_NilChain tests nil chain handling
func TestClassifierChain_NilChain(t *testing.T) {
	var chain *ClassifierChain
	ctx := t.Context()

	// Should not panic on nil chain
	result := chain.ClassifyQuery(ctx, "test query")

	// Should return nil for nil chain
	assert.Nil(t, result)
}

// TestClassifierChain_TierPriority tests that T0 takes precedence over T1/T2
func TestClassifierChain_TierPriority(t *testing.T) {
	// This test verifies the tier routing logic:
	// If a keyword matches (T0), it should NOT try embedding (T1/T2)

	keyword := NewKeywordClassifier()
	embedding := createTestEmbeddingClassifier(t)
	chain := NewClassifierChain(keyword, embedding)

	ctx := t.Context()

	// Query that has BOTH keyword AND might match embedding example
	// "yesterday" is a keyword, and the query might be in examples
	result := chain.ClassifyQuery(ctx, "show deliveries from yesterday")

	require.NotNil(t, result)
	// Should use T0 (keyword) and NOT check embedding
	assert.Equal(t, 0, result.Tier)
	assert.Equal(t, 1.0, result.Confidence)
	// Should have temporal filter from keyword
	assert.NotNil(t, result.Options["time_range"])
}

// TestClassifierChain_ResultStructure tests ClassificationResult structure
func TestClassifierChain_ResultStructure(t *testing.T) {
	keyword := NewKeywordClassifier()
	embedding := createTestEmbeddingClassifier(t)
	chain := NewClassifierChain(keyword, embedding)

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
				// Intent is for embedding matches
			},
		},
		{
			name:  "T1/T2 result has correct structure",
			query: "Where is shipment ABC-123?", // Should match example
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Should be T1 or T2
				assert.True(t, result.Tier == 1 || result.Tier == 2)
				assert.NotEqual(t, "", result.Intent) // Should have intent from example
				assert.Greater(t, result.Confidence, 0.0)
				assert.NotNil(t, result.Options)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			result := chain.ClassifyQuery(ctx, tt.query)
			tt.validateResult(t, result)
		})
	}
}

// TestClassifierChain_OptionsPopulation tests that Options are populated correctly
func TestClassifierChain_OptionsPopulation(t *testing.T) {
	keyword := NewKeywordClassifier()
	embedding := createTestEmbeddingClassifier(t)
	chain := NewClassifierChain(keyword, embedding)

	ctx := t.Context()

	t.Run("keyword options", func(t *testing.T) {
		result := chain.ClassifyQuery(ctx, "find similar items")

		require.NotNil(t, result)
		assert.Equal(t, 0, result.Tier)
		// Should have use_embeddings from similarity keyword
		useEmb, ok := result.Options["use_embeddings"].(bool)
		assert.True(t, ok)
		assert.True(t, useEmb)
	})

	t.Run("embedding example options", func(t *testing.T) {
		result := chain.ClassifyQuery(ctx, "Where is shipment ABC-123?")

		require.NotNil(t, result)
		if result.Tier > 0 {
			// Should have options from matched example
			assert.NotNil(t, result.Options)
			// Specific options depend on example definition
		}
	})
}

// TestClassifierChain_ConcurrentAccess tests thread safety
func TestClassifierChain_ConcurrentAccess(t *testing.T) {
	keyword := NewKeywordClassifier()
	embedding := createTestEmbeddingClassifier(t)
	chain := NewClassifierChain(keyword, embedding)

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
				result := chain.ClassifyQuery(ctx, query)
				// Should not panic and should return valid result
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
	embedding := createTestEmbeddingClassifier(t)
	chain := NewClassifierChain(keyword, embedding)

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
			result := chain.ClassifyQuery(ctx, tt.query)
			assert.NotNil(t, result)
		})
	}
}

// Helper functions

// createTestEmbeddingClassifier creates a classifier with test examples
func createTestEmbeddingClassifier(t *testing.T) *EmbeddingClassifier {
	t.Helper()
	return createTestEmbeddingClassifierWithThreshold(t, 0.5)
}

// createTestEmbeddingClassifierWithThreshold creates a classifier with specific threshold
func createTestEmbeddingClassifierWithThreshold(t *testing.T, threshold float64) *EmbeddingClassifier {
	t.Helper()
	domains := []*DomainExamples{
		{
			Domain:  "logistics",
			Version: "1.0",
			Examples: []Example{
				{
					Query:   "Where is shipment ABC-123?",
					Intent:  "entity_lookup",
					Options: map[string]any{"entity_pattern": "shipment"},
				},
				{
					Query:   "Show delayed deliveries",
					Intent:  "temporal_filter",
					Options: map[string]any{"status_filter": "delayed"},
				},
				{
					Query:   "Find packages near location",
					Intent:  "spatial_filter",
					Options: map[string]any{"spatial": true},
				},
			},
		},
	}

	return mustNewEmbeddingClassifier(t, domains, threshold)
}
