package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
)

// LLMClient is the minimal interface for sending a classification prompt to an LLM backend.
//
// The returned string must be valid JSON matching llmResponse. Implementations are
// responsible for retries, timeouts, and backend-specific authentication.
type LLMClient interface {
	// ClassifyQuery sends a structured classification prompt to the LLM and returns
	// the raw JSON response string.
	ClassifyQuery(ctx context.Context, prompt string) (string, error)
}

// llmResponse is the expected JSON structure returned by the LLM.
//
// Fields map directly to SearchOptions so parsing is straightforward. All fields
// are optional — the LLM omits fields that don't apply to the query.
type llmResponse struct {
	Strategy      string    `json:"strategy"`
	Query         string    `json:"query"`
	Predicates    []string  `json:"predicates"`
	Types         []string  `json:"types"`
	KeyTerms      []string  `json:"key_terms"`
	UseEmbeddings bool      `json:"use_embeddings"`
	TimeRange     *llmRange `json:"time_range"`
	Limit         int       `json:"limit"`
}

// llmRange carries temporal bounds from the LLM response.
//
// Times are expected in RFC3339 format. The LLM is instructed to use this format
// in the system prompt.
type llmRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// LLMClassifier classifies queries by asking an LLM to return structured SearchOptions.
//
// It is the T3 tier in the ClassifierChain — called only when T0 (keyword) and T1
// (example classifier) tiers produce no confident match. When the LLM call fails or returns
// unparseable JSON, LLMClassifier returns an error so the chain can fall back
// gracefully rather than silently returning a wrong classification.
type LLMClassifier struct {
	client  LLMClient
	domains []*DomainExamples // Optional few-shot examples per domain
}

// NewLLMClassifier creates an LLMClassifier with an LLM backend and optional domain examples.
//
// domains may be nil or empty; when provided, a few representative examples are
// included in the prompt as few-shot context to improve classification accuracy.
func NewLLMClassifier(client LLMClient, domains []*DomainExamples) *LLMClassifier {
	return &LLMClassifier{
		client:  client,
		domains: domains,
	}
}

// ClassifyQuery classifies a natural language query using an LLM and returns a
// ClassificationResult at Tier 3.
//
// Returns an error if:
//   - ctx is nil (an invalid-input error, before the client is called)
//   - The LLM call fails (network, auth, quota)
//   - The LLM response is not valid JSON
//   - Context is cancelled before the call completes
func (c *LLMClassifier) ClassifyQuery(ctx context.Context, query string) (*ClassificationResult, error) {
	if ctx == nil {
		return nil, errs.WrapInvalid(errors.New("nil context"), "LLMClassifier", "ClassifyQuery", "missing context")
	}
	prompt := c.buildPrompt(query)

	raw, err := c.client.ClassifyQuery(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm classify query: %w", err)
	}

	opts, err := parseLLMResponse(raw, query)
	if err != nil {
		return nil, fmt.Errorf("llm response parse: %w", err)
	}

	return &ClassificationResult{
		Tier:       3,
		Intent:     inferIntentFromOptions(opts),
		Options:    llmOptionsToMap(opts),
		Confidence: 0.9, // LLM classifications are treated as high-confidence
	}, nil
}

// buildPrompt constructs the system + user prompt sent to the LLM.
//
// The prompt includes:
//   - A clear instruction to return only JSON
//   - The full list of valid strategies with descriptions
//   - The full list of valid intent types
//   - Few-shot examples drawn from loaded domain files (if any)
//   - The query to classify
func (c *LLMClassifier) buildPrompt(query string) string {
	var b strings.Builder

	b.WriteString("You are a query classifier for a knowledge graph search engine. ")
	b.WriteString("Analyze the user query and return a JSON object describing how to execute the search.\n\n")

	b.WriteString("## Output Format\n")
	b.WriteString("Return ONLY a valid JSON object with these fields (omit fields that do not apply):\n")
	b.WriteString("{\n")
	b.WriteString(`  "strategy": "<one of the valid strategies below>",` + "\n")
	b.WriteString(`  "query": "<cleaned or reformulated query text>",` + "\n")
	b.WriteString(`  "predicates": ["<predicate filter>", ...],` + "\n")
	b.WriteString(`  "types": ["<entity type filter>", ...],` + "\n")
	b.WriteString(`  "key_terms": ["<important search terms extracted from the query>"],` + "\n")
	b.WriteString(`  "use_embeddings": <true|false>,` + "\n")
	b.WriteString(`  "time_range": {"start": "<RFC3339>", "end": "<RFC3339>"},` + "\n")
	b.WriteString(`  "limit": <integer result count>` + "\n")
	b.WriteString("}\n\n")

	b.WriteString("## Valid Strategies\n")
	b.WriteString("- graphrag: General graph traversal + text search (default)\n")
	b.WriteString("- geo_graphrag: Graph traversal with geographic bounds filter\n")
	b.WriteString("- temporal_graphrag: Graph traversal with time range filter\n")
	b.WriteString("- hybrid_graphrag: Multiple filter types combined (geo + temporal, etc.)\n")
	b.WriteString("- pathrag: Find paths between entities (use when query asks about connections or relationships)\n")
	b.WriteString("- semantic: Pure vector similarity search (use when query asks for similarity)\n")
	b.WriteString("- exact: Exact match on entity attributes (use when query has precise filters but no text)\n\n")

	b.WriteString("## Valid Intent Types (for your reasoning, not returned in JSON)\n")
	b.WriteString("entity_lookup, temporal_filter, spatial_relationship, similarity, path, aggregation, metric_query, relationship\n\n")

	// Few-shot examples from domain files
	if examples := c.collectFewShotExamples(); len(examples) > 0 {
		b.WriteString("## Examples\n")
		for _, ex := range examples {
			b.WriteString(fmt.Sprintf("Query: %q\n", ex.Query))
			b.WriteString(fmt.Sprintf("Intent: %s\n", ex.Intent))
			if len(ex.Options) > 0 {
				if raw, err := json.Marshal(ex.Options); err == nil {
					b.WriteString(fmt.Sprintf("Hints: %s\n", raw))
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Query to Classify\n")
	b.WriteString(query)
	b.WriteString("\n\n")
	b.WriteString("Return ONLY the JSON object. No explanation, no markdown, no code fences.")

	return b.String()
}

// collectFewShotExamples gathers a representative sample of examples across all loaded domains.
//
// At most 2 examples per domain are selected to keep the prompt compact while still
// providing meaningful few-shot context. The selection favors variety of intent types.
func (c *LLMClassifier) collectFewShotExamples() []Example {
	const maxPerDomain = 2

	var selected []Example
	for _, domain := range c.domains {
		if domain == nil || len(domain.Examples) == 0 {
			continue
		}

		seen := make(map[string]bool)
		count := 0
		for _, ex := range domain.Examples {
			if count >= maxPerDomain {
				break
			}
			// Prefer diverse intents
			if !seen[ex.Intent] {
				seen[ex.Intent] = true
				selected = append(selected, ex)
				count++
			}
		}
	}

	return selected
}

// parseLLMResponse parses the raw JSON string returned by the LLM into SearchOptions.
//
// The original query is always preserved in the returned SearchOptions regardless of
// what the LLM returned in the "query" field, so callers can rely on Query being set.
func parseLLMResponse(raw, originalQuery string) (*SearchOptions, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("llm returned empty response")
	}

	var resp llmResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("invalid json from llm: %w", err)
	}

	opts := &SearchOptions{
		Query:         originalQuery,
		Predicates:    resp.Predicates,
		Types:         resp.Types,
		KeyTerms:      resp.KeyTerms,
		UseEmbeddings: resp.UseEmbeddings,
		Limit:         resp.Limit,
	}

	// Apply strategy if valid
	if strategy := SearchStrategy(resp.Strategy); isValidStrategy(strategy) {
		opts.Strategy = strategy
	}

	// Override query text only when LLM provides a non-empty reformulation
	if strings.TrimSpace(resp.Query) != "" {
		opts.Query = strings.TrimSpace(resp.Query)
	}

	// Parse temporal range — silently ignore parse failures to be lenient
	if resp.TimeRange != nil {
		start, errS := time.Parse(time.RFC3339, resp.TimeRange.Start)
		end, errE := time.Parse(time.RFC3339, resp.TimeRange.End)
		if errS == nil && errE == nil && !start.IsZero() && !end.IsZero() {
			opts.TimeRange = &TimeRange{Start: start, End: end}
		}
	}

	return opts, nil
}

// isValidStrategy returns true if the strategy is one of the defined SearchStrategy constants.
func isValidStrategy(s SearchStrategy) bool {
	switch s {
	case StrategyGraphRAG,
		StrategyGeoGraphRAG,
		StrategyTemporalGraphRAG,
		StrategyHybridGraphRAG,
		StrategyPathRAG,
		StrategySemantic,
		StrategyExact,
		StrategyAggregation:
		return true
	}
	return false
}

// inferIntentFromOptions derives a human-readable intent label from the SearchOptions
// produced by the LLM response. This mirrors the vocabulary used in domain example files.
func inferIntentFromOptions(opts *SearchOptions) string {
	if opts == nil {
		return ""
	}

	switch {
	case opts.PathIntent:
		return "path"
	case opts.UseEmbeddings:
		return "similarity"
	case opts.TimeRange != nil && opts.GeoBounds != nil:
		return "hybrid_filter"
	case opts.TimeRange != nil:
		return "temporal_filter"
	case opts.GeoBounds != nil:
		return "spatial_relationship"
	case len(opts.Types) > 0 || len(opts.Predicates) > 0:
		return "entity_lookup"
	default:
		return "entity_lookup"
	}
}

// llmOptionsToMap converts SearchOptions into the map[string]any form used by ClassificationResult.
//
// Only non-zero/non-nil fields are included, matching the convention established by
// searchOptionsToMap in classifier_chain.go.
func llmOptionsToMap(opts *SearchOptions) map[string]any {
	if opts == nil {
		return make(map[string]any)
	}

	result := make(map[string]any)

	if opts.Strategy != "" {
		result["strategy"] = string(opts.Strategy)
	}
	if opts.Query != "" {
		result["query"] = opts.Query
	}
	if len(opts.Predicates) > 0 {
		result["predicates"] = opts.Predicates
	}
	if len(opts.Types) > 0 {
		result["types"] = opts.Types
	}
	if len(opts.KeyTerms) > 0 {
		result["key_terms"] = opts.KeyTerms
	}
	if opts.UseEmbeddings {
		result["use_embeddings"] = opts.UseEmbeddings
	}
	if opts.TimeRange != nil {
		result["time_range"] = opts.TimeRange
	}
	if opts.Limit > 0 {
		result["limit"] = opts.Limit
	}
	if opts.PathIntent {
		result["path_intent"] = opts.PathIntent
		if opts.PathStartNode != "" {
			result["path_start_node"] = opts.PathStartNode
		}
	}
	if len(opts.PathPredicates) > 0 {
		result["path_predicates"] = opts.PathPredicates
	}

	return result
}
