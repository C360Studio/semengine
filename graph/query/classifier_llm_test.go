package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDomainExamples returns a small set of domain examples for use in tests.
func testDomainExamples() []*DomainExamples {
	return []*DomainExamples{
		{
			Domain:  "iot",
			Version: "1.0",
			Examples: []Example{
				{Query: "Show sensor SENS-001 status", Intent: "entity_lookup"},
				{Query: "What sensors were active yesterday?", Intent: "temporal_filter"},
			},
		},
	}
}

// mockLLMClient is a test implementation of LLMClient.
type mockLLMClient struct {
	response string
	err      error
	// capturedPrompt allows tests to inspect what prompt was sent.
	capturedPrompt string
}

func (m *mockLLMClient) ClassifyQuery(_ context.Context, prompt string) (string, error) {
	m.capturedPrompt = prompt
	return m.response, m.err
}

// validLLMResponse returns a well-formed JSON response for use in tests.
func validLLMResponse(strategy, query string) string {
	return fmt.Sprintf(`{"strategy":%q,"query":%q}`, strategy, query)
}

// TestNewLLMClassifier tests constructor with various inputs.
func TestNewLLMClassifier(t *testing.T) {
	t.Run("with client and domains", func(t *testing.T) {
		client := &mockLLMClient{}
		domains := testDomainExamples()
		c := NewLLMClassifier(client, domains)
		require.NotNil(t, c)
	})

	t.Run("with client and nil domains", func(t *testing.T) {
		client := &mockLLMClient{}
		c := NewLLMClassifier(client, nil)
		require.NotNil(t, c)
	})

	t.Run("with client and empty domains", func(t *testing.T) {
		client := &mockLLMClient{}
		c := NewLLMClassifier(client, []*DomainExamples{})
		require.NotNil(t, c)
	})
}

// TestLLMClassifier_ValidResponse tests the happy path: valid JSON from the LLM.
func TestLLMClassifier_ValidResponse(t *testing.T) {
	tests := []struct {
		name           string
		llmJSON        string
		originalQuery  string
		validateResult func(t *testing.T, result *ClassificationResult)
	}{
		{
			name:          "graphrag strategy",
			llmJSON:       `{"strategy":"graphrag","query":"active sensors"}`,
			originalQuery: "show me active sensors",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 3, result.Tier)
				assert.Equal(t, 0.9, result.Confidence)
				assert.Equal(t, "graphrag", result.Options["strategy"])
				assert.Equal(t, "active sensors", result.Options["query"])
			},
		},
		{
			name:          "semantic strategy with use_embeddings",
			llmJSON:       `{"strategy":"semantic","query":"sensors similar to temp-001","use_embeddings":true}`,
			originalQuery: "find sensors similar to temp-001",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 3, result.Tier)
				assert.Equal(t, "semantic", result.Options["strategy"])
				assert.Equal(t, true, result.Options["use_embeddings"])
				assert.Equal(t, "similarity", result.Intent)
			},
		},
		{
			name:          "temporal strategy with time_range",
			llmJSON:       `{"strategy":"temporal_graphrag","query":"sensor readings","time_range":{"start":"2026-03-01T00:00:00Z","end":"2026-03-07T23:59:59Z"}}`,
			originalQuery: "show sensor readings from last week",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 3, result.Tier)
				assert.Equal(t, "temporal_graphrag", result.Options["strategy"])
				require.NotNil(t, result.Options["time_range"])
				tr, ok := result.Options["time_range"].(*TimeRange)
				require.True(t, ok, "time_range should be *TimeRange")
				assert.Equal(t, "2026-03-01T00:00:00Z", tr.Start.UTC().Format(time.RFC3339))
			},
		},
		{
			name:          "pathrag strategy",
			llmJSON:       `{"strategy":"pathrag","query":"gateway connections","predicates":["connected_to"]}`,
			originalQuery: "what is connected to gateway-main",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 3, result.Tier)
				assert.Equal(t, "pathrag", result.Options["strategy"])
				assert.Equal(t, []string{"connected_to"}, result.Options["predicates"])
			},
		},
		{
			name:          "exact strategy with types filter",
			llmJSON:       `{"strategy":"exact","types":["sensor","device"],"limit":50}`,
			originalQuery: "list all sensors and devices",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 3, result.Tier)
				assert.Equal(t, "exact", result.Options["strategy"])
				assert.Equal(t, []string{"sensor", "device"}, result.Options["types"])
				assert.Equal(t, 50, result.Options["limit"])
			},
		},
		{
			name: "unknown strategy is silently ignored",
			// Unknown strategies are dropped and options reflect a default graphrag query.
			llmJSON:       `{"strategy":"future_unknown_strategy","query":"clean query"}`,
			originalQuery: "raw query",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, 3, result.Tier)
				// strategy key should not be present when invalid
				_, hasStrategy := result.Options["strategy"]
				assert.False(t, hasStrategy, "invalid strategy should not be in options map")
			},
		},
		{
			name: "empty llm query field preserves original",
			// When LLM returns empty "query", original query is preserved.
			llmJSON:       `{"strategy":"graphrag","query":""}`,
			originalQuery: "original user query",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				// Original query preserved when LLM returns empty
				assert.Equal(t, "original user query", result.Options["query"])
			},
		},
		{
			name:          "whitespace-only llm query field preserves original",
			llmJSON:       `{"strategy":"graphrag","query":"   "}`,
			originalQuery: "original user query",
			validateResult: func(t *testing.T, result *ClassificationResult) {
				require.NotNil(t, result)
				assert.Equal(t, "original user query", result.Options["query"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockLLMClient{response: tt.llmJSON}
			c := NewLLMClassifier(client, nil)
			ctx := t.Context()

			result, err := c.ClassifyQuery(ctx, tt.originalQuery)

			require.NoError(t, err)
			tt.validateResult(t, result)
		})
	}
}

// TestLLMClassifier_InvalidJSON tests that unparseable LLM responses return an error.
func TestLLMClassifier_InvalidJSON(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{
			name:     "completely invalid json",
			response: "I classified your query as temporal.",
		},
		{
			name:     "truncated json",
			response: `{"strategy":"graphrag","query":`,
		},
		{
			name:     "empty response",
			response: "",
		},
		{
			name:     "json array instead of object",
			response: `["graphrag","temporal"]`,
		},
		{
			name:     "markdown-wrapped json",
			response: "```json\n{\"strategy\":\"graphrag\"}\n```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockLLMClient{response: tt.response}
			c := NewLLMClassifier(client, nil)
			ctx := t.Context()

			result, err := c.ClassifyQuery(ctx, "any query")

			// Must return an error — chain relies on this to fall back gracefully.
			assert.Nil(t, result, "result should be nil on parse error")
			assert.Error(t, err, "should return an error for invalid JSON")
		})
	}
}

// TestLLMClassifier_ClientError tests that LLM call failures are propagated as errors.
func TestLLMClassifier_ClientError(t *testing.T) {
	tests := []struct {
		name        string
		clientError error
	}{
		{
			name:        "network error",
			clientError: errors.New("connection refused"),
		},
		{
			name:        "rate limit error",
			clientError: errors.New("rate limit exceeded: retry after 60s"),
		},
		{
			name:        "context cancelled",
			clientError: context.Canceled,
		},
		{
			name:        "context deadline exceeded",
			clientError: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockLLMClient{err: tt.clientError}
			c := NewLLMClassifier(client, nil)
			ctx := t.Context()

			result, err := c.ClassifyQuery(ctx, "any query")

			assert.Nil(t, result, "result should be nil on client error")
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.clientError), "error should wrap the client error")
		})
	}
}

// TestLLMClassifier_PromptIncludesDomainExamples tests that domain examples appear in the prompt.
func TestLLMClassifier_PromptIncludesDomainExamples(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain:  "iot",
			Version: "1.0",
			Examples: []Example{
				{Query: "Show sensor SENS-001 status", Intent: "entity_lookup"},
				{Query: "What sensors were active yesterday?", Intent: "temporal_filter"},
				{Query: "Find sensors similar to TEMP-001", Intent: "similarity"},
			},
		},
	}

	client := &mockLLMClient{response: validLLMResponse("graphrag", "sensor status")}
	c := NewLLMClassifier(client, domains)
	ctx := t.Context()

	_, err := c.ClassifyQuery(ctx, "what is sensor-42 status")
	require.NoError(t, err)

	prompt := client.capturedPrompt

	// Prompt should contain representative domain examples.
	// collectFewShotExamples picks at most 2 per domain with distinct intents.
	assert.Contains(t, prompt, "Show sensor SENS-001 status", "first example query should appear")
	assert.Contains(t, prompt, "entity_lookup", "first example intent should appear")
	assert.Contains(t, prompt, "What sensors were active yesterday?", "second example query should appear")
	assert.Contains(t, prompt, "temporal_filter", "second example intent should appear")

	// Third example has intent "similarity" — collected if distinct (it is)
	// But maxPerDomain = 2 so only 2 are included.
	// Just verify the prompt structure contains the Examples section header.
	assert.Contains(t, prompt, "## Examples", "examples section header should appear")
}

// TestLLMClassifier_PromptWithoutDomainExamples tests prompt construction without examples.
func TestLLMClassifier_PromptWithoutDomainExamples(t *testing.T) {
	tests := []struct {
		name    string
		domains []*DomainExamples
	}{
		{name: "nil domains", domains: nil},
		{name: "empty domains slice", domains: []*DomainExamples{}},
		{name: "domains with nil entries", domains: []*DomainExamples{nil}},
		{name: "domains with empty examples", domains: []*DomainExamples{{Domain: "test", Examples: []Example{}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockLLMClient{response: validLLMResponse("graphrag", "clean query")}
			c := NewLLMClassifier(client, tt.domains)
			ctx := t.Context()

			result, err := c.ClassifyQuery(ctx, "some query")

			// Should succeed — examples section simply not included.
			require.NoError(t, err)
			require.NotNil(t, result)

			prompt := client.capturedPrompt

			// Should still include the mandatory structural sections.
			assert.Contains(t, prompt, "## Output Format")
			assert.Contains(t, prompt, "## Valid Strategies")
			assert.Contains(t, prompt, "## Query to Classify")

			// Examples section should be absent when no examples available.
			assert.NotContains(t, prompt, "## Examples")
		})
	}
}

// TestLLMClassifier_PromptContainsQuery tests that the user query is always in the prompt.
func TestLLMClassifier_PromptContainsQuery(t *testing.T) {
	queries := []string{
		"show me all active sensors",
		"find devices connected to gateway-main",
		"what happened yesterday in zone-A",
		"sensors similar to temp-001",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			client := &mockLLMClient{response: validLLMResponse("graphrag", query)}
			c := NewLLMClassifier(client, nil)
			ctx := t.Context()

			_, err := c.ClassifyQuery(ctx, query)
			require.NoError(t, err)

			assert.Contains(t, client.capturedPrompt, query, "user query must appear in the prompt")
		})
	}
}

// TestLLMClassifier_PromptContainsStrategies verifies all valid strategies are listed in the prompt.
func TestLLMClassifier_PromptContainsStrategies(t *testing.T) {
	client := &mockLLMClient{response: validLLMResponse("graphrag", "query")}
	c := NewLLMClassifier(client, nil)
	ctx := t.Context()

	_, err := c.ClassifyQuery(ctx, "any query")
	require.NoError(t, err)

	prompt := client.capturedPrompt

	strategies := []string{
		string(StrategyGraphRAG),
		string(StrategyGeoGraphRAG),
		string(StrategyTemporalGraphRAG),
		string(StrategyHybridGraphRAG),
		string(StrategyPathRAG),
		string(StrategySemantic),
		string(StrategyExact),
	}

	for _, s := range strategies {
		assert.Contains(t, prompt, s, "strategy %q should appear in prompt", s)
	}
}

// TestLLMClassifier_TierIs3 verifies that LLMClassifier always returns Tier=3.
func TestLLMClassifier_TierIs3(t *testing.T) {
	client := &mockLLMClient{response: validLLMResponse("graphrag", "result")}
	c := NewLLMClassifier(client, nil)
	ctx := t.Context()

	result, err := c.ClassifyQuery(ctx, "any query")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 3, result.Tier, "LLMClassifier must always return Tier=3")
}

// TestLLMClassifier_ConfidenceIs0Point9 verifies the fixed confidence score.
func TestLLMClassifier_ConfidenceIs0Point9(t *testing.T) {
	client := &mockLLMClient{response: validLLMResponse("graphrag", "query")}
	c := NewLLMClassifier(client, nil)
	ctx := t.Context()

	result, err := c.ClassifyQuery(ctx, "any query")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0.9, result.Confidence)
}

// TestLLMClassifier_TimeRangeMalformed tests that malformed time_range values are ignored.
//
// The LLM may return invalid RFC3339 strings. The classifier should silently drop the
// time_range rather than erroring out, keeping the result usable.
func TestLLMClassifier_TimeRangeMalformed(t *testing.T) {
	tests := []struct {
		name    string
		llmJSON string
	}{
		{
			name:    "non-rfc3339 format",
			llmJSON: `{"strategy":"temporal_graphrag","time_range":{"start":"March 1 2026","end":"March 7 2026"}}`,
		},
		{
			name:    "missing end",
			llmJSON: `{"strategy":"temporal_graphrag","time_range":{"start":"2026-03-01T00:00:00Z","end":""}}`,
		},
		{
			name:    "both empty strings",
			llmJSON: `{"strategy":"temporal_graphrag","time_range":{"start":"","end":""}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockLLMClient{response: tt.llmJSON}
			c := NewLLMClassifier(client, nil)
			ctx := t.Context()

			result, err := c.ClassifyQuery(ctx, "events last week")

			// Must not error — malformed time_range is silently dropped.
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Nil(t, result.Options["time_range"], "malformed time_range should be absent from options")
		})
	}
}

// TestLLMClassifier_IntentInference tests the intent label derived from SearchOptions.
func TestLLMClassifier_IntentInference(t *testing.T) {
	tests := []struct {
		name           string
		llmJSON        string
		expectedIntent string
	}{
		{
			name:           "use_embeddings -> similarity",
			llmJSON:        `{"strategy":"semantic","use_embeddings":true}`,
			expectedIntent: "similarity",
		},
		{
			name:           "temporal range -> temporal_filter",
			llmJSON:        `{"strategy":"temporal_graphrag","time_range":{"start":"2026-03-01T00:00:00Z","end":"2026-03-07T23:59:59Z"}}`,
			expectedIntent: "temporal_filter",
		},
		{
			name:           "types filter -> entity_lookup",
			llmJSON:        `{"strategy":"exact","types":["sensor"]}`,
			expectedIntent: "entity_lookup",
		},
		{
			name:           "predicates filter -> entity_lookup",
			llmJSON:        `{"strategy":"graphrag","predicates":["connected_to"]}`,
			expectedIntent: "entity_lookup",
		},
		{
			name:           "plain query -> entity_lookup",
			llmJSON:        `{"strategy":"graphrag","query":"sensor status"}`,
			expectedIntent: "entity_lookup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockLLMClient{response: tt.llmJSON}
			c := NewLLMClassifier(client, nil)
			ctx := t.Context()

			result, err := c.ClassifyQuery(ctx, "test query")

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedIntent, result.Intent)
		})
	}
}

// TestLLMClassifier_MultipleDomains tests that examples from multiple domains are included.
func TestLLMClassifier_MultipleDomains(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "iot",
			Examples: []Example{
				{Query: "Show sensor status", Intent: "entity_lookup"},
			},
		},
		{
			Domain: "logistics",
			Examples: []Example{
				{Query: "Where is shipment ABC?", Intent: "entity_lookup"},
				{Query: "Show delayed deliveries", Intent: "temporal_filter"},
			},
		},
	}

	client := &mockLLMClient{response: validLLMResponse("graphrag", "query")}
	c := NewLLMClassifier(client, domains)
	ctx := t.Context()

	_, err := c.ClassifyQuery(ctx, "any query")
	require.NoError(t, err)

	prompt := client.capturedPrompt

	// Should include examples from both domains.
	assert.Contains(t, prompt, "Show sensor status")
	assert.Contains(t, prompt, "Where is shipment ABC?")
}

// TestLLMClassifier_FewShotMaxPerDomain tests that at most 2 examples are selected per domain.
func TestLLMClassifier_FewShotMaxPerDomain(t *testing.T) {
	// Domain has 5 examples with distinct intents. Expect only 2 to be selected.
	domains := []*DomainExamples{
		{
			Domain: "iot",
			Examples: []Example{
				{Query: "Query A", Intent: "intent_a"},
				{Query: "Query B", Intent: "intent_b"},
				{Query: "Query C", Intent: "intent_c"},
				{Query: "Query D", Intent: "intent_d"},
				{Query: "Query E", Intent: "intent_e"},
			},
		},
	}

	client := &mockLLMClient{response: validLLMResponse("graphrag", "q")}
	c := NewLLMClassifier(client, domains)

	examples := c.collectFewShotExamples()

	// Exactly 2 should be selected (maxPerDomain = 2).
	assert.Len(t, examples, 2)
	// The two selected should have distinct intents.
	assert.NotEqual(t, examples[0].Intent, examples[1].Intent)
}

// TestLLMClassifier_FewShotDedupByIntent tests that duplicate intents are not selected.
func TestLLMClassifier_FewShotDedupByIntent(t *testing.T) {
	// All examples have the same intent — only the first should be selected.
	domains := []*DomainExamples{
		{
			Domain: "iot",
			Examples: []Example{
				{Query: "Query A", Intent: "entity_lookup"},
				{Query: "Query B", Intent: "entity_lookup"},
				{Query: "Query C", Intent: "entity_lookup"},
			},
		},
	}

	c := NewLLMClassifier(nil, domains)
	examples := c.collectFewShotExamples()

	// Only one should be picked (first distinct intent).
	assert.Len(t, examples, 1)
	assert.Equal(t, "Query A", examples[0].Query)
}

// TestParseLLMResponse_AllFields tests that all SearchOptions fields are parsed correctly.
func TestParseLLMResponse_AllFields(t *testing.T) {
	raw := `{
		"strategy": "hybrid_graphrag",
		"query": "active sensors in zone-A last week",
		"predicates": ["located_in", "reports_to"],
		"types": ["sensor", "gateway"],
		"use_embeddings": true,
		"time_range": {"start": "2026-03-01T00:00:00Z", "end": "2026-03-07T23:59:59Z"},
		"limit": 25
	}`

	opts, err := parseLLMResponse(raw, "original")
	require.NoError(t, err)
	require.NotNil(t, opts)

	assert.Equal(t, StrategyHybridGraphRAG, opts.Strategy)
	assert.Equal(t, "active sensors in zone-A last week", opts.Query)
	assert.Equal(t, []string{"located_in", "reports_to"}, opts.Predicates)
	assert.Equal(t, []string{"sensor", "gateway"}, opts.Types)
	assert.True(t, opts.UseEmbeddings)
	require.NotNil(t, opts.TimeRange)
	assert.Equal(t, 25, opts.Limit)
}

// TestParseLLMResponse_MinimalFields tests that a minimal valid response is accepted.
func TestParseLLMResponse_MinimalFields(t *testing.T) {
	raw := `{"strategy":"graphrag"}`

	opts, err := parseLLMResponse(raw, "original query")
	require.NoError(t, err)
	require.NotNil(t, opts)

	// Only strategy set; query preserved from original.
	assert.Equal(t, StrategyGraphRAG, opts.Strategy)
	assert.Equal(t, "original query", opts.Query)
	assert.Nil(t, opts.TimeRange)
	assert.Empty(t, opts.Predicates)
	assert.Empty(t, opts.Types)
	assert.False(t, opts.UseEmbeddings)
	assert.Zero(t, opts.Limit)
}

// TestIsValidStrategy covers all strategy constants and one invalid value.
func TestIsValidStrategy(t *testing.T) {
	valid := []SearchStrategy{
		StrategyGraphRAG,
		StrategyGeoGraphRAG,
		StrategyTemporalGraphRAG,
		StrategyHybridGraphRAG,
		StrategyPathRAG,
		StrategySemantic,
		StrategyExact,
	}

	for _, s := range valid {
		assert.True(t, isValidStrategy(s), "strategy %q should be valid", s)
	}

	assert.False(t, isValidStrategy("unknown_strategy"))
	assert.False(t, isValidStrategy(""))
}

// TestLLMOptionsToMap_EmptyOptions tests nil and zero-value options.
func TestLLMOptionsToMap_EmptyOptions(t *testing.T) {
	t.Run("nil options", func(t *testing.T) {
		m := llmOptionsToMap(nil)
		assert.NotNil(t, m)
		assert.Empty(t, m)
	})

	t.Run("zero value options", func(t *testing.T) {
		opts := &SearchOptions{}
		m := llmOptionsToMap(opts)
		assert.NotNil(t, m)
		// No fields set — map should be empty.
		assert.Empty(t, m)
	})
}

// TestLLMClassifier_ContextCancellationPropagated tests that context cancellation
// from the LLM call is correctly wrapped and returned.
func TestLLMClassifier_ContextCancellationPropagated(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	// Client that respects context cancellation.
	client := &mockLLMClient{err: ctx.Err()}
	cancel() // Cancel before the call.

	c := NewLLMClassifier(client, nil)

	// Force the error to be context.Canceled.
	client.err = context.Canceled

	result, err := c.ClassifyQuery(ctx, "query")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

// TestLLMClassifier_ExamplesOptionsIncluded tests that example Options hints appear in prompt.
func TestLLMClassifier_ExamplesOptionsIncluded(t *testing.T) {
	domains := []*DomainExamples{
		{
			Domain: "iot",
			Examples: []Example{
				{
					Query:   "Find sensors similar to TEMP-001",
					Intent:  "similarity",
					Options: map[string]any{"reference_entity": true},
				},
				{
					Query:   "List sensors by zone",
					Intent:  "spatial_relationship",
					Options: map[string]any{"group_by": "zone"},
				},
			},
		},
	}

	client := &mockLLMClient{response: validLLMResponse("semantic", "sensors")}
	c := NewLLMClassifier(client, domains)
	ctx := t.Context()

	_, err := c.ClassifyQuery(ctx, "sensors like temp-001")
	require.NoError(t, err)

	prompt := client.capturedPrompt

	// Options JSON should appear in the prompt for the selected examples.
	assert.True(t,
		strings.Contains(prompt, "reference_entity") || strings.Contains(prompt, "group_by"),
		"at least one example's options should appear in the prompt",
	)
}
