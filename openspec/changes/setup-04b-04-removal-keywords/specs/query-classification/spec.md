# query-classification Delta

## ADDED Requirements

### Requirement: An ended context gives no classification

`ClassifierChain.ClassifyQuery` SHALL return `nil` when the chain is nil, when the context is nil or has ended before
it starts, and when the context has ended by the time the LLM tier has produced nothing; it SHALL never return the
default result for an ended context.

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

### Requirement: The chain answers from the keyword tier, then the LLM tier

`ClassifierChain.ClassifyQuery` SHALL return the keyword tier's hints (tier 0, confidence 1.0) when the keyword
classifier finds any; otherwise the LLM tier's result (tier 3) when one is composed and succeeds; otherwise, while the
context is live, the default result: tier 0, no hints, confidence 1.0.

#### Scenario: A keyword hint stops the ladder

- **WHEN** the query is "events from yesterday" and an LLM tier whose fake client counts its calls is composed
- **THEN** the result is tier 0 with a `time_range` hint and the LLM client was never called
- **AND** the test that verifies this is `TestChainKeywordHintStopsTheLadder`

#### Scenario: The LLM tier answers what the keywords do not

- **WHEN** a product composes a `KeywordClassifier` and an LLM tier whose fake client returns a `types` hint, and asks
  a question no keyword pattern matches
- **THEN** the result is tier 3 with that hint, and with no LLM tier composed it is the default result
- **AND** the test that verifies this is `TestChainAnswersFromFirstTier`

## MODIFIED Requirements

### Requirement: Tier 0 classification imports no provider code

`graph/query` SHALL build without any external provider: `go list -deps ./graph/query` SHALL name neither
`github.com/sashabaranov/go-openai` nor the module's `model` package or anything under it, and SHALL NOT name
`graph/embedding`.

#### Scenario: The package's dependencies

- **WHEN** `go list -deps ./graph/query` runs on the tree
- **THEN** no listed path is `go-openai`, `model` or under either
- **AND** the test that verifies this is `TestQueryImportsNoProviderModule`

#### Scenario: No embedding code

- **WHEN** `go list -deps ./graph/query` runs on the tree
- **THEN** `graph/embedding` is not listed
- **AND** the test that verifies this is `TestQueryImportsNoEmbedding`

### Requirement: An excluded operation binds the chain's answer

When the keyword tier found no hint and excluded an operation, `ClassifierChain.ClassifyQuery` SHALL return the
default result without asking the LLM tier.

#### Scenario: A later tier cannot restore an excluded operation

- **WHEN** a fake LLM client that would answer an aggregation and a similarity counts its calls, and the queries are
  "Do not count devices in error state" and "Not devices similar to TEMP-001"
- **THEN** the chain returns the default result for each and the LLM client was never called
- **AND** "Count devices in error state" and "Devices similar to TEMP-001" give tier 0 with `aggregation_type`
  `count` and `use_embeddings` true
- **AND** the test that verifies this is `TestChainKeepsExclusions`

## REMOVED Requirements

### Requirement: The example classifier builds under the caller's context

**Reason**: The example classifier is deleted (design D1); nothing called it. The chain's context rules stay in "An
ended context gives no classification".

### Requirement: A result's top-level map, Example and vector are copies

**Reason**: Only the example classifier returned an `Example` and a vector, and kept the maps it handed out (design
D1). The keyword and LLM tiers build each result's `Options` map new.

### Requirement: Classifying a query does not change the example classifier

**Reason**: The example classifier and the hashed embedder whose statistics it guarded are deleted (design D1, D5).

### Requirement: Tier 1 hints carry their Go types

**Reason**: Tier 1 is gone (design D1); the keyword and LLM tiers give hints their Go types already.

### Requirement: The example classifier refuses impossible hints

**Reason**: The example classifier is deleted (design D1).

### Requirement: The chain answers from the first tier that produces a hint

**Reason**: The example classifier (tier 1) is deleted (design D1). Replaced by "The chain answers from the keyword
tier, then the LLM tier".

### Requirement: An ended context yields no classification

**Reason**: `FindBestMatch` is deleted with the example classifier (design D1). Replaced by "An ended context gives no
classification".
