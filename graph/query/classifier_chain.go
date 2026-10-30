package query

import (
	"context"
)

// ClassificationResult contains the classification output from the tiered classifier chain.
type ClassificationResult struct {
	Tier       int            // Which tier produced the result (0=keyword, 1=example classifier, 3=LLM)
	Intent     string         // Classified intent (from an example or the LLM, empty for keyword)
	Options    map[string]any // SearchOptions hints (from keyword, example or LLM)
	Confidence float64        // Confidence score (1.0 for keywords, similarity for an example, 0.9 for the LLM)
}

// ClassifierChain orchestrates tiered query classification.
//
// Routing logic:
//   - T0 (keyword): Always try first - a keyword hint stops the ladder
//   - An operation the keyword tier excluded, with no keyword hint: the default result
//   - T1 (example classifier): Only if no keyword hint AND one is composed
//   - T3 (LLM): Only if no earlier tier produced a hint AND one is composed
//   - Default: T0 with no hints and confidence 1.0, while the context is live
//
// Thread-safe for concurrent ClassifyQuery calls.
type ClassifierChain struct {
	keyword   *KeywordClassifier   // T0 - always available (pattern matching)
	embedding *EmbeddingClassifier // T1 - example classifier (optional)
	llm       *LLMClassifier       // T3 - LLM-based classification (optional)
}

// NewClassifierChain creates a classifier chain with keyword and optional embedding/LLM classifiers.
//
// All parameters may be nil. Chain will route queries through available tiers.
func NewClassifierChain(keyword *KeywordClassifier, embedding *EmbeddingClassifier, llmClassifiers ...*LLMClassifier) *ClassifierChain {
	var llm *LLMClassifier
	if len(llmClassifiers) > 0 {
		llm = llmClassifiers[0]
	}
	return &ClassifierChain{
		keyword:   keyword,
		embedding: embedding,
		llm:       llm,
	}
}

// ClassifyQuery classifies a query through the tier chain and returns the answer of the first
// tier that produces a hint:
//   - T0: the keyword hints (temporal, spatial, similarity, path, zone, aggregation, ranking),
//     confidence 1.0
//   - T1: a copy of the matching example's options and its score, when the example classifier
//     is composed and the match reaches its threshold
//   - T3: the LLM classifier's result, when it is composed and succeeds
//   - Default: T0 with no hints and confidence 1.0
//
// The keyword tier's outcome decides whether the later tiers are asked (#95):
//
//	Keyword outcome                    Chain result
//	any hint                           tier 0, those hints
//	no hint, nothing excluded          tiers 1, 3, then the default result
//	no hint, some operation excluded   the default result; tiers 1 and 3 are not asked
//
// An exclusion (a negated, quoted or undecided operation; KeywordClassifier.ClassifyQuery
// states the rules) binds the final decision whichever later tiers are composed. A later tier
// answers in the caller's own words, which no framework rule can read, so the chain does not ask
// it. The cost: a question whose only operation is excluded gets no tier 1 or tier 3 answer.
//
// It returns nil when the chain is nil, when ctx is nil or has ended before it starts, and when
// ctx has ended by the time a tier after the keyword tier has produced nothing: an ended context
// never gets the default result.
//
// A failed LLM tier under a live context is a declared degrade to the default result: the chain
// answers as if no LLM were composed, and the error is not reported. A caller that must tell the
// two apart calls LLMClassifier.ClassifyQuery itself.
//
// Each result's Options map is the caller's own; values nested inside an entry may be shared.
func (c *ClassifierChain) ClassifyQuery(ctx context.Context, query string) *ClassificationResult {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return nil
	}

	// T0: Try keyword classifier first (if available)
	if c.keyword != nil {
		opts, excluded := c.keyword.classify(query)
		if hasExplicitIntent(opts) {
			return &ClassificationResult{
				Tier:       0,
				Intent:     "", // Keyword tier doesn't set Intent (uses SearchOptions fields)
				Options:    searchOptionsToMap(opts),
				Confidence: 1.0,
			}
		}
		if excluded {
			return noHintResult()
		}
	}

	// T1: Try the example classifier if available and no keyword match. FindBestMatch returns
	// a copy, so its Options map is the result's own.
	if c.embedding != nil {
		match, score := c.embedding.FindBestMatch(ctx, query)
		if match != nil && score >= c.embedding.Threshold() {
			return &ClassificationResult{
				Tier:       1,
				Intent:     match.Intent,
				Options:    match.Options,
				Confidence: score,
			}
		}
	}

	// T3: Try LLM classifier if available and no keyword/example match
	if c.llm != nil {
		result, err := c.llm.ClassifyQuery(ctx, query)
		if err == nil && result != nil {
			return result
		}
		// A failure under a live context degrades to the default result (declared above).
	}

	// A context that ended during a later tier gets no answer, not the default result.
	if ctx.Err() != nil {
		return nil
	}

	return noHintResult()
}

// noHintResult is the default result: tier 0, no hints, confidence 1.0.
func noHintResult() *ClassificationResult {
	return &ClassificationResult{
		Tier:       0,
		Intent:     "",
		Options:    make(map[string]any),
		Confidence: 1.0,
	}
}

// hasExplicitIntent checks if SearchOptions has any explicit filters or intents.
//
// Returns true if any of these are set:
//   - TimeRange (temporal filter)
//   - GeoBounds (spatial filter)
//   - UseEmbeddings (similarity intent)
//   - PathIntent (path/zone intent)
//   - AggregationType (aggregation intent)
//   - RankingIntent (ranking intent)
func hasExplicitIntent(opts *SearchOptions) bool {
	if opts == nil {
		return false
	}

	return opts.TimeRange != nil ||
		opts.GeoBounds != nil ||
		opts.UseEmbeddings ||
		opts.PathIntent ||
		opts.AggregationType != "" ||
		opts.RankingIntent
}

// searchOptionsToMap converts SearchOptions to map[string]any for ClassificationResult.
//
// Only includes non-nil/non-zero fields relevant to classification.
func searchOptionsToMap(opts *SearchOptions) map[string]any {
	if opts == nil {
		return make(map[string]any)
	}

	result := make(map[string]any)

	// Add present fields
	if opts.TimeRange != nil {
		result["time_range"] = opts.TimeRange
	}
	if opts.GeoBounds != nil {
		result["geo_bounds"] = opts.GeoBounds
	}
	if opts.UseEmbeddings {
		result["use_embeddings"] = opts.UseEmbeddings
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
	if opts.AggregationType != "" {
		result["aggregation_type"] = opts.AggregationType
		if opts.AggregationField != "" {
			result["aggregation_field"] = opts.AggregationField
		}
	}
	if opts.RankingIntent {
		result["ranking_intent"] = opts.RankingIntent
		if opts.Limit > 0 {
			result["limit"] = opts.Limit
		}
	}

	return result
}
