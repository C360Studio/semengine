# query-classification

## ADDED Requirements

### Requirement: Tier 0 classification imports no provider code

`graph/query` SHALL build without any external provider: `go list -deps ./graph/query` SHALL name neither
`github.com/sashabaranov/go-openai` nor the module's `model` package or anything under it.

#### Scenario: The package's dependencies

- **WHEN** `go list -deps ./graph/query` runs on the tree
- **THEN** no listed path is `go-openai`, `model` or under either
- **AND** the test that verifies this is `TestQueryImportsNoProviderModule`

### Requirement: The chain answers from the first tier that produces a hint

`ClassifierChain.ClassifyQuery` SHALL return the keyword tier's hints (tier 0, confidence 1.0) when the keyword
classifier finds any; otherwise the example classifier's match at or above its threshold (tier 1, the match's score)
when one is composed; otherwise the LLM tier's result (tier 3) when one is composed and succeeds; otherwise, while the
context is live, the default result: tier 0, no hints, confidence 1.0. A failure of the LLM tier under a live context
SHALL yield the default result.

#### Scenario: A keyword hint stops the ladder

- **WHEN** the query is "events from yesterday" and an LLM tier whose fake client counts its calls is composed
- **THEN** the result is tier 0 with a `time_range` hint and the LLM client was never called
- **AND** the test that verifies this is `TestChainKeywordHintStopsTheLadder`

#### Scenario: The LLM tier fails under a live context

- **WHEN** no earlier tier produces a hint and the LLM client returns an error while the context is live
- **THEN** the result is the default result
- **AND** the test that verifies this is `TestChainEndedContextReturnsNil`

### Requirement: An ended context yields no classification

`ClassifierChain.ClassifyQuery` SHALL return `nil` when the chain is nil, when the context is nil or has ended before
it starts, and when the context has ended by the time a tier after the keyword tier has produced nothing; it SHALL
never return the default result for an ended context. `EmbeddingClassifier.FindBestMatch` SHALL return no match and a
score of 0 for a nil or ended context.

#### Scenario: A nil context

- **WHEN** `ClassifyQuery` is called on the chain with a nil context
- **THEN** the chain returns `nil`
- **AND** the test that verifies this is `TestChainEndedContextReturnsNil`

#### Scenario: Ended at entry

- **WHEN** `ClassifyQuery` is called with an ended context
- **THEN** it returns `nil`
- **AND** the test that verifies this is `TestChainEndedContextReturnsNil`

#### Scenario: Ended during the LLM tier

- **WHEN** the LLM client ends the context and returns its error
- **THEN** `ClassifyQuery` returns `nil`, not the default result
- **AND** the test that verifies this is `TestChainEndedContextReturnsNil`

### Requirement: The LLM classifier refuses a nil context

`LLMClassifier.ClassifyQuery` SHALL refuse a nil context with an invalid-input error before calling its client.

#### Scenario: A nil context reaches the LLM classifier

- **WHEN** `LLMClassifier.ClassifyQuery` is called with a nil context
- **THEN** it returns an invalid-input error without calling its client
- **AND** the test that verifies this is `TestLLMClassifierRefusesNilContext`

### Requirement: The example classifier builds under the caller's context

`NewEmbeddingClassifier` SHALL embed its examples under the context it is given and SHALL return an error and no
classifier when that context is nil or has ended or any example cannot be embedded. No production code in the package
SHALL create a context root.

#### Scenario: An ended context at construction

- **WHEN** `NewEmbeddingClassifier` is called with an ended context and one domain of examples
- **THEN** it returns a nil classifier and a non-nil error
- **AND** the test that verifies this is `TestNewEmbeddingClassifierRefusesEndedContext`

### Requirement: A result shares no state with the classifier

Every `Example` that `FindBestMatch` returns and every `ClassificationResult.Options` map the chain returns SHALL be a
copy: changing its top-level entries, its `Options` map or its `Vector` SHALL change no later result.

#### Scenario: A caller edits a result

- **WHEN** a caller deletes and adds keys in a tier-1 result's `Options` and overwrites the returned `Example`'s
  `Vector`, then classifies the same query again
- **THEN** the second result has the original hints and the original score
- **AND** the test that verifies this is `TestResultEditsDoNotReachClassifier`

### Requirement: The keyword tier adds no domain predicate

`KeywordClassifier.ClassifyQuery` SHALL NOT add a `path_predicates` hint for a zone or area phrase; it SHALL keep the
phrase's path intent and start node.

#### Scenario: A zone question

- **WHEN** the query is "sensors in zone A"
- **THEN** the hints carry `path_intent` and `path_start_node` `zone-A`, and no `path_predicates`
- **AND** the test that verifies this is `TestZoneQueryAddsNoPredicate`
