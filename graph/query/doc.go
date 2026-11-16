// Package query classifies a natural-language question into search hints: the time range,
// area, path start, aggregation or ranking it asks for. It reads and stores no graph data.
//
// A ClassifierChain asks its two tiers in order and answers from the first that produces a hint:
//
//   - Tier 0, KeywordClassifier: regular expressions over the question. Always available.
//   - Tier 3, LLMClassifier: asks a language model through an LLMClient the caller supplies,
//     with optional few-shot examples from domain example files (LoadDomainExamples).
//     Optional; this package holds no LLM client.
//
// No code produces tiers 1 or 2; the numbers are the values callers read from
// ClassificationResult.Tier.
//
// When no composed tier produces a hint, the chain answers the default result: tier 0, no
// hints, confidence 1.0. A nil or ended context gets no answer (nil), never the default
// result. ClassifierChain.ClassifyQuery documents each outcome, including a failed LLM tier.
//
// A question holding an impossible number ("last 0 hours", a latitude of 95) is refused:
// ClassifierChain.ClassifyQuery returns no result and an invalid-input error naming the phrase,
// whichever tiers are composed. With the keyword tier composed, a question naming an entity
// reference and a zone or area phrase for another place ("sensor-001 in zone-b"), or two zone or
// area phrases for different places ("zone a in zone b"), is refused the same way, naming both places. "Today" and the other calendar ranges follow the server's zone
// (time.Local).
//
// Hints travel as ClassificationResult.Options, a map keyed by the SearchOptions JSON names
// (time_range, path_intent, path_start_node, ...). Each result's map is the caller's own copy.
package query
