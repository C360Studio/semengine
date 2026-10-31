package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEmbeddingClassifier tests constructor with various inputs
func TestNewEmbeddingClassifier(t *testing.T) {
	tests := []struct {
		name      string
		domains   []*DomainExamples
		threshold float64
		validate  func(t *testing.T, c *EmbeddingClassifier)
	}{
		{
			name: "valid single domain",
			domains: []*DomainExamples{
				{
					Domain:  "logistics",
					Version: "1.0",
					Examples: []Example{
						{Query: "Where is shipment ABC-123?", Intent: "entity_lookup"},
						{Query: "Show delayed deliveries", Intent: "temporal_filter"},
					},
				},
			},
			threshold: 0.75,
			validate: func(t *testing.T, c *EmbeddingClassifier) {
				assert.NotNil(t, c)
				assert.Equal(t, 0.75, c.Threshold())

				// Should have 2 examples loaded with BM25 vectors
				// (can't access examples directly due to private field, but constructor should work)
			},
		},
		{
			name: "multiple domains",
			domains: []*DomainExamples{
				{
					Domain: "logistics",
					Examples: []Example{
						{Query: "Where is my package?", Intent: "entity_lookup"},
					},
				},
				{
					Domain: "iot",
					Examples: []Example{
						{Query: "Show temperature readings", Intent: "sensor_query"},
						{Query: "Find sensors above threshold", Intent: "threshold_filter"},
					},
				},
			},
			threshold: 0.8,
			validate: func(t *testing.T, c *EmbeddingClassifier) {
				assert.NotNil(t, c)
				assert.Equal(t, 0.8, c.Threshold())
				// Should have 3 total examples (1 + 2)
			},
		},
		{
			name:      "empty domains slice",
			domains:   []*DomainExamples{},
			threshold: 0.75,
			validate: func(t *testing.T, c *EmbeddingClassifier) {
				assert.NotNil(t, c)
				assert.Equal(t, 0.75, c.Threshold())
				// Should have 0 examples but not panic
			},
		},
		{
			name:      "nil domains slice",
			domains:   nil,
			threshold: 0.75,
			validate: func(t *testing.T, c *EmbeddingClassifier) {
				assert.NotNil(t, c)
				assert.Equal(t, 0.75, c.Threshold())
				// Should handle nil gracefully
			},
		},
		{
			name: "domain with empty examples",
			domains: []*DomainExamples{
				{
					Domain:   "test",
					Examples: []Example{},
				},
			},
			threshold: 0.5,
			validate: func(t *testing.T, c *EmbeddingClassifier) {
				assert.NotNil(t, c)
				assert.Equal(t, 0.5, c.Threshold())
			},
		},
		{
			name: "zero threshold",
			domains: []*DomainExamples{
				{
					Domain: "test",
					Examples: []Example{
						{Query: "test query", Intent: "test"},
					},
				},
			},
			threshold: 0.0,
			validate: func(t *testing.T, c *EmbeddingClassifier) {
				assert.NotNil(t, c)
				assert.Equal(t, 0.0, c.Threshold())
				// Zero threshold means all matches are valid
			},
		},
		{
			name: "high threshold",
			domains: []*DomainExamples{
				{
					Domain: "test",
					Examples: []Example{
						{Query: "test", Intent: "test"},
					},
				},
			},
			threshold: 0.99,
			validate: func(t *testing.T, c *EmbeddingClassifier) {
				assert.NotNil(t, c)
				assert.Equal(t, 0.99, c.Threshold())
				// High threshold requires very close match
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustNewEmbeddingClassifier(t, tt.domains, tt.threshold)
			tt.validate(t, c)
		})
	}
}

// TestEmbeddingClassifier_BM25Initialization verifies BM25 vectors are created at startup
func TestEmbeddingClassifier_BM25Initialization(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "test",
			Examples: []Example{
				{Query: "test query one", Intent: "intent1"},
				{Query: "test query two", Intent: "intent2"},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.5)
	require.NotNil(t, c)

	// After construction, FindBestMatch should work using BM25 vectors
	// This verifies that BM25 embeddings were generated during initialization
	ctx := t.Context()
	match, score := c.FindBestMatch(ctx, "test query one")

	// Should find exact match with high score
	assert.NotNil(t, match)
	assert.Greater(t, score, 0.5)
	assert.Equal(t, "intent1", match.Intent)
}

// TestFindBestMatch tests finding the best matching example
func TestFindBestMatch(t *testing.T) {
	// Create classifier with sample examples
	domains := []*DomainExamples{
		{
			Domain: "logistics",
			Examples: []Example{
				{
					Query:   "Where is shipment ABC-123?",
					Intent:  "entity_lookup",
					Options: map[string]any{"entity_pattern": "shipment"},
				},
				{
					Query:   "Show delayed deliveries this week",
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

	tests := []struct {
		name           string
		query          string
		threshold      float64
		expectMatch    bool
		expectIntent   string
		expectMinScore float64
		expectNilScore bool
	}{
		{
			name:           "exact match",
			query:          "Where is shipment ABC-123?",
			threshold:      0.5,
			expectMatch:    true,
			expectIntent:   "entity_lookup",
			expectMinScore: 0.9, // Exact match should have very high score
		},
		{
			name:           "similar query",
			query:          "Where is my shipment?",
			threshold:      0.4,
			expectMatch:    true,
			expectIntent:   "entity_lookup",
			expectMinScore: 0.4,
		},
		{
			name:           "query below threshold",
			query:          "completely unrelated query about weather",
			threshold:      0.8,
			expectMatch:    false,
			expectNilScore: true,
		},
		{
			name:           "empty query",
			query:          "",
			threshold:      0.5,
			expectMatch:    false,
			expectNilScore: true,
		},
		{
			name:           "whitespace only query",
			query:          "   ",
			threshold:      0.5,
			expectMatch:    false,
			expectNilScore: true,
		},
		{
			name:           "partial keyword match",
			query:          "Show deliveries",
			threshold:      0.3,
			expectMatch:    true,
			expectIntent:   "temporal_filter",
			expectMinScore: 0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustNewEmbeddingClassifier(t, domains, tt.threshold)
			ctx := t.Context()

			match, score := c.FindBestMatch(ctx, tt.query)

			if tt.expectMatch {
				assert.NotNil(t, match, "Expected to find a match")
				assert.Equal(t, tt.expectIntent, match.Intent)
				assert.GreaterOrEqual(t, score, tt.expectMinScore)
				assert.GreaterOrEqual(t, score, tt.threshold)
			} else {
				if tt.expectNilScore {
					// For empty queries and unmatched queries
					assert.Nil(t, match)
					assert.Equal(t, 0.0, score)
				} else {
					// Score exists but below threshold
					if match != nil {
						assert.Less(t, score, tt.threshold)
					}
				}
			}
		})
	}
}

// TestFindBestMatch_ContextCancellation tests context cancellation handling
func TestFindBestMatch_ContextCancellation(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "test",
			Examples: []Example{
				{Query: "test query", Intent: "test"},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.5)

	// Create cancelled context
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	match, score := c.FindBestMatch(ctx, "test query")

	// Should return early with nil result
	assert.Nil(t, match)
	assert.Equal(t, 0.0, score)
}

// TestFindBestMatch_ContextTimeout tests context timeout handling
func TestFindBestMatch_ContextTimeout(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "test",
			Examples: []Example{
				{Query: "test query", Intent: "test"},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.5)

	// A deadline already in the past: the context has ended before the call,
	// with no wait (the pin slept past a 1ns timeout).
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	match, score := c.FindBestMatch(ctx, "test query")

	// Should handle timeout gracefully
	assert.Nil(t, match)
	assert.Equal(t, 0.0, score)
}

// TestFindBestMatch_NilClassifier tests defensive nil handling
func TestFindBestMatch_NilClassifier(t *testing.T) {
	var c *EmbeddingClassifier
	ctx := t.Context()

	// Should not panic on nil classifier
	match, score := c.FindBestMatch(ctx, "test query")

	assert.Nil(t, match)
	assert.Equal(t, 0.0, score)
}

// TestFindBestMatch_MultipleMatches tests selecting the best match when multiple examples match
func TestFindBestMatch_MultipleMatches(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "logistics",
			Examples: []Example{
				{
					Query:  "Where is shipment ABC?",
					Intent: "exact_match",
				},
				{
					Query:  "Where is package?",
					Intent: "partial_match",
				},
				{
					Query:  "unrelated query about temperature",
					Intent: "no_match",
				},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.3)
	ctx := t.Context()

	// Query that's closer to first example
	match, score := c.FindBestMatch(ctx, "Where is shipment ABC?")

	// Should return the best match (highest similarity)
	assert.NotNil(t, match)
	assert.Equal(t, "exact_match", match.Intent)
	assert.Greater(t, score, 0.8) // Exact match should be very high
}

// TestFindBestMatch_PreservesOptions tests that matched example options are preserved: a
// caller's own name unchanged and a known name as its typed value. The pin's version gave
// time_range the word "week", which construction now refuses (#478, design D4).
func TestFindBestMatch_PreservesOptions(t *testing.T) {
	week := &TimeRange{
		Start: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
	}
	domains := []*DomainExamples{
		{
			Domain: "logistics",
			Examples: []Example{
				{
					Query:  "Show delayed deliveries",
					Intent: "temporal_filter",
					Options: map[string]any{
						"status_filter": "delayed",
						"time_range":    map[string]any{"start": "2026-03-02T00:00:00Z", "end": "2026-03-09T00:00:00Z"},
						"limit":         100,
					},
				},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.3)
	ctx := t.Context()

	match, score := c.FindBestMatch(ctx, "Show delayed deliveries")

	require.NotNil(t, match)
	assert.Greater(t, score, 0.5)

	// Verify options are preserved
	assert.Equal(t, "delayed", match.Options["status_filter"])
	assert.Equal(t, week, match.Options["time_range"])
	assert.Equal(t, 100, match.Options["limit"])
}

// TestThreshold tests threshold getter
func TestThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
	}{
		{name: "low threshold", threshold: 0.3},
		{name: "medium threshold", threshold: 0.5},
		{name: "high threshold", threshold: 0.9},
		{name: "zero threshold", threshold: 0.0},
		{name: "one threshold", threshold: 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustNewEmbeddingClassifier(t, nil, tt.threshold)
			assert.Equal(t, tt.threshold, c.Threshold())
		})
	}
}

// TestEmbeddingClassifier_UnicodeQueries tests handling of non-ASCII characters
func TestEmbeddingClassifier_UnicodeQueries(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "international",
			Examples: []Example{
				{Query: "Où est mon colis?", Intent: "french_query"},
				{Query: "运输在哪里？", Intent: "chinese_query"},
				{Query: "Show deliveries 🚚📦", Intent: "emoji_query"},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.5)
	ctx := t.Context()

	// Should handle unicode without panic
	match, _ := c.FindBestMatch(ctx, "Où est mon colis?")
	assert.NotNil(t, match)
	assert.Equal(t, "french_query", match.Intent)
}

// TestEmbeddingClassifier_LongQueries tests handling of very long queries
func TestEmbeddingClassifier_LongQueries(t *testing.T) {
	// Create a very long query (1000 characters)
	longQuery := ""
	for i := 0; i < 100; i++ {
		longQuery += "show delayed deliveries "
	}

	domains := []*DomainExamples{
		{
			Domain: "test",
			Examples: []Example{
				{Query: "show delayed deliveries", Intent: "test"},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.3)
	ctx := t.Context()

	// Should handle long query without panic
	match, score := c.FindBestMatch(ctx, longQuery)

	// Should still find a match (repeated keywords)
	assert.NotNil(t, match)
	assert.Greater(t, score, 0.0)
}

// TestEmbeddingClassifier_SpecialCharacters tests handling of special characters
func TestEmbeddingClassifier_SpecialCharacters(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "test",
			Examples: []Example{
				{Query: "Find item #ABC-123 @location", Intent: "special_chars"},
				{Query: "Query with \"quotes\" and 'apostrophes'", Intent: "quotes"},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.3)
	ctx := t.Context()

	// Should handle special characters
	match, _ := c.FindBestMatch(ctx, "Find item #ABC-123 @location")
	assert.NotNil(t, match)
	assert.Equal(t, "special_chars", match.Intent)
}

// TestEmbeddingClassifier_EmptyExamples tests classifier with no examples
func TestEmbeddingClassifier_EmptyExamples(t *testing.T) {
	c := mustNewEmbeddingClassifier(t, []*DomainExamples{}, 0.5)
	ctx := t.Context()

	match, score := c.FindBestMatch(ctx, "any query")

	// Should return nil for empty classifier
	assert.Nil(t, match)
	assert.Equal(t, 0.0, score)
}

// TestFindBestMatch_AllBelowThreshold tests when all examples are below threshold
func TestFindBestMatch_AllBelowThreshold(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "test",
			Examples: []Example{
				{Query: "completely unrelated example one", Intent: "intent1"},
				{Query: "completely unrelated example two", Intent: "intent2"},
			},
		},
	}

	c := mustNewEmbeddingClassifier(t, domains, 0.95) // Very high threshold
	ctx := t.Context()

	// Query that won't match well
	match, score := c.FindBestMatch(ctx, "different topic entirely")

	// Should return nil when all scores are below threshold
	assert.Nil(t, match)
	assert.Less(t, score, 0.95)
}

// mustNewEmbeddingClassifier builds a classifier under the test's context and
// fails the test if construction fails.
func mustNewEmbeddingClassifier(t *testing.T, domains []*DomainExamples, threshold float64) *EmbeddingClassifier {
	t.Helper()
	c, err := NewEmbeddingClassifier(t.Context(), domains, threshold)
	require.NoError(t, err)
	require.NotNil(t, c)
	return c
}
