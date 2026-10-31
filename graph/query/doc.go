// Package query classifies a natural-language question into search hints: the time range,
// area, path start, aggregation or ranking it asks for. It reads and stores no graph data.
//
// A ClassifierChain asks its tiers in order and answers from the first that produces a hint:
//
//   - Tier 0, KeywordClassifier: regular expressions over the question. Always available.
//   - Tier 1, EmbeddingClassifier: the most similar example from domain example files
//     (LoadDomainExamples), compared as BM25 vectors in the process. Optional.
//   - Tier 3, LLMClassifier: asks a language model through an LLMClient the caller supplies.
//     Optional; this package holds no LLM client. No code produces tier 2.
//
// When no composed tier produces a hint, the chain answers the default result: tier 0, no
// hints, confidence 1.0. A nil or ended context gets no answer (nil), never the default
// result. ClassifierChain.ClassifyQuery documents each outcome, including a failed LLM tier.
//
// A question holding an impossible number ("last 0 hours", a latitude of 95) is refused:
// ClassifierChain.ClassifyQuery returns no result and an invalid-input error naming the phrase,
// whichever tiers are composed. "Today" and the other calendar ranges follow the server's zone
// (time.Local).
//
// Hints travel as ClassificationResult.Options, a map keyed by the SearchOptions JSON names
// (time_range, path_intent, path_start_node, ...). Each result's map is the caller's own copy.
package query
