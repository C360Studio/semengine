# query-classification Specification

## Purpose
`graph/query` turns a natural-language question into search hints for the graph-query processor, at tier 0 with no
external provider. A chain of classifiers runs in order: keyword rules (tier 0), BM25 matching against example
questions (tier 1), and an optional caller-supplied LLM client (tier 3). This spec states what the chain returns for
each outcome, how it honours the caller's context, what results it copies, and that it imports no provider code.

## Requirements

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
context is live, the default result: tier 0, no hints, confidence 1.0.

#### Scenario: A keyword hint stops the ladder

- **WHEN** the query is "events from yesterday" and an LLM tier whose fake client counts its calls is composed
- **THEN** the result is tier 0 with a `time_range` hint and the LLM client was never called
- **AND** the test that verifies this is `TestChainKeywordHintStopsTheLadder`

#### Scenario: A product composes an example classifier

- **WHEN** a product loads two example files with `LoadDomainExamples`, builds an `EmbeddingClassifier`, composes it
  into a chain beside a `KeywordClassifier` held as a `Classifier`, and asks a question no keyword pattern matches
  but an example does
- **THEN** the result is tier 1 with that example's options and its score
- **AND** the test that verifies this is `TestEmbeddingClassifierAsAProduct`

### Requirement: A failed LLM tier gives the default result

A failure of the LLM tier under a live context SHALL yield the default result, with a nil error.

#### Scenario: The LLM tier fails under a live context

- **WHEN** no earlier tier produces a hint and the LLM client returns an error while the context is live
- **THEN** the result is the default result
- **AND** the test that verifies this is `TestChainLLMFailureGivesDefault`

### Requirement: An excluded operation binds the chain's answer

When the keyword tier found no hint and excluded an operation, `ClassifierChain.ClassifyQuery` SHALL return the
default result without asking tier 1 or tier 3.

#### Scenario: A later tier cannot restore an excluded operation

- **WHEN** an example classifier holds the examples "Count devices in error state" (intent `aggregation`, options
  `operation: count`) and "Find devices similar to TEMP-001" (intent `similarity`), a fake LLM client counts its
  calls, and the queries are "Do not count devices in error state" and "Not devices similar to TEMP-001"
- **THEN** `FindBestMatch` alone returns the unwanted example for each at or above the threshold
- **AND** the chain returns the default result for each and the LLM client was never called
- **AND** "Count devices in error state" and "Devices similar to TEMP-001" give tier 0 with `aggregation_type`
  `count` and `use_embeddings` true
- **AND** the test that verifies this is `TestChainKeepsExclusions`

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

#### Scenario: The example classifier with a nil or ended context

- **WHEN** `FindBestMatch` is called with a nil context, and again with an ended one
- **THEN** each call returns no match and a score of 0
- **AND** the test that verifies this is `TestFindBestMatchEndedContext`

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

### Requirement: A result's top-level map, Example and vector are copies

Every `Example` that `FindBestMatch` returns, its `Options` map and its `Vector`, and every
`ClassificationResult.Options` map the chain returns SHALL be copies: changing a field of the returned `Example`, a
top-level entry of either map, or an element of the `Vector` SHALL change no later result. Values nested inside an
`Options` entry are shared, not copied.

#### Scenario: A caller edits a result

- **WHEN** two classifiers are built from identical examples and receive the same sequence of queries, and after
  each tier-1 result of the first the caller deletes and adds top-level keys in its `Options`, and overwrites the
  returned `Example`'s fields and `Vector` elements, while the second's results are left untouched
- **THEN** every later result of the first equals the second's at the same step: hints, intent, vector and score
- **AND** the test that verifies this is `TestResultEditsDoNotReachClassifier`; with each copy removed in turn
  (`task mutate:check`), it fails

### Requirement: The keyword tier adds no domain predicate

`KeywordClassifier.ClassifyQuery` SHALL NOT add a `path_predicates` hint for a zone or area phrase; it SHALL keep the
phrase's path intent and start node.

#### Scenario: A zone question

- **WHEN** the query is "sensors in zone A"
- **THEN** the hints carry `path_intent` and `path_start_node` `zone-A`, and no `path_predicates`
- **AND** the test that verifies this is `TestZoneQueryAddsNoPredicate`

### Requirement: The keyword tier binds the aggregated field

The keyword tier SHALL take the aggregation field from the first word after the aggregation trigger, skipping an
optional `of` and an optional `the`; it SHALL hold no list of field names.

#### Scenario: Mean of a field

- **WHEN** the query is "Compute the arithmetic mean of pressure", and again "Return the combined total of humidity
  readings"
- **THEN** the hints are `aggregation_type` `avg` with field `pressure`, then `sum` with field `humidity`
- **AND** the test that verifies this is `TestAggregationFieldBinding`

### Requirement: The keyword tier recognises ordinary count and extremum phrases

The keyword tier SHALL select count for `the number of`, not for `a number of`, and SHALL keep count for
`total number of`. It SHALL select min for `smallest` or `lowest`, and max for `largest`, `highest` or `greatest`,
followed by one field word and a value noun (`value`, `reading`, `measurement`, `level`, singular or plural), with
that word as the field, unless `which`, `who` or `with` comes before the phrase in its clause.

#### Scenario: Ordinary phrases

- **WHEN** the queries are "Return the number of sensors", "Give the smallest temperature value", "Return the lowest
  pressure reading", "Give the largest humidity value" and "Return the highest temperature reading"
- **THEN** the hints are count; min/`temperature`; min/`pressure`; max/`humidity`; max/`temperature`
- **AND** the test that verifies this is `TestOrdinaryAggregationPhrases`

#### Scenario: Entity ranking is not a numeric extremum

- **WHEN** the query is "Which sensor has the highest temperature reading"
- **THEN** there is no `aggregation_type` hint
- **AND** the test that verifies this is `TestOrdinaryAggregationPhrases`

### Requirement: Negated, quoted and undecided operations give no hint

An operation is one of count, avg, sum, min, max, similarity and ranking, each its own. The keyword tier SHALL give
no hint for an operation whose trigger follows a negating word (`not`, `no`, `never`, `don't`, `do not`, `without`)
in the same clause, lies inside a balanced pair of straight (`"`) or curly (`“ ”`) double quotes, or is one of two
different operations joined by `or` in one clause; other hints SHALL stand. A clause is text between `;`, `,`, `.`,
`:`, `?`, `!` or the word `but`.

#### Scenario: Excluded operations

- **WHEN** the queries are "Do not count sensors; list their names", "Show pump-42 records, not devices similar to
  it", "Find documents containing the phrase \"average temperature\"" and "Maybe count devices or average pressure;
  I have not decided"
- **THEN** each result has no operation hint
- **AND** the test that verifies this is `TestKeywordExclusions`

#### Scenario: An affirmative clause after a negated one

- **WHEN** the query is "Do not count sensors; average pressure"
- **THEN** the hints are `aggregation_type` `avg` with field `pressure`
- **AND** the test that verifies this is `TestKeywordExclusions`

#### Scenario: Partial path intent is kept

- **WHEN** the query is "Show connections from that device"
- **THEN** the hints carry `path_intent` and no `path_start_node`
- **AND** the test that verifies this is `TestKeywordExclusions`

### Requirement: The keyword tier accepts any text

For any query the keyword tier SHALL return without panicking, and a query whose only triggers lie inside straight or
curly double quotes SHALL yield no operation hint.

#### Scenario: Arbitrary text

- **WHEN** `FuzzKeywordClassify` runs from its seed corpus
- **THEN** no input panics, and no input whose triggers all lie inside quotes yields an operation hint
- **AND** the test that verifies this is `FuzzKeywordClassify`

### Requirement: Classifying a query does not change the example classifier

`EmbeddingClassifier.FindBestMatch` SHALL embed the query without changing the embedder's corpus statistics, so the
score for a query SHALL NOT depend on the queries classified before it. Construction SHALL still embed the examples
as documents.

#### Scenario: Intervening queries

- **WHEN** two classifiers are built from the same examples in the same order, one classifies unrelated queries, and
  then both classify the same query
- **THEN** both return the same score and, at threshold 0, the same example
- **AND** the test that verifies this is `TestQueryEmbeddingIsReadOnly`

### Requirement: Tier 1 hints carry their Go types

`NewEmbeddingClassifier` SHALL decode each example option whose name is a `SearchOptions` JSON name into the type
tiers 0 and 3 give it (`*TimeRange`, `*SpatialBounds`, `[]string`, `int`, `bool`, `string`; `strategy` a `string`
naming one of the `SearchStrategy` values), SHALL keep other names unchanged, and SHALL return an error naming the
domain, the example and the option, and no classifier, when such a value does not decode. It SHALL keep its own
copies of the examples' options.

#### Scenario: A time range from an example file

- **WHEN** an example's options carry `"time_range": {"start": "2026-01-01T00:00:00Z", "end":
  "2026-01-02T00:00:00Z"}`, `"types": ["sensor"]`, `"limit": 5` and `"site": "north"`, and a query matches it
- **THEN** the tier-1 hints hold a `*TimeRange` with those instants, a `[]string`, an `int` 5 and the string `north`
- **AND** the test that verifies this is `TestTierOneHintsAreTyped`

#### Scenario: Every hint name is covered

- **WHEN** each `SearchOptions` JSON name is given a valid value in an example
- **THEN** each decodes to its declared type, and a wrong-typed value refuses construction
- **AND** the test that verifies this is `TestEveryHintNameIsTyped`

### Requirement: The example classifier refuses impossible hints

`NewEmbeddingClassifier` SHALL return an error naming the domain, the example and the option, and no classifier,
when an example's `strategy` is not one of the eight `SearchStrategy` values, its `time_range` has a zero start or
end or ends before it starts, its `geo_bounds` has North or South outside [-90, 90] or South above North, or its
`limit` is negative.

#### Scenario: Impossible example hints

- **WHEN** examples carry, one at a time, `strategy` `fastest`, a `time_range` ending before it starts, a
  `geo_bounds` with North 95, and `limit` -1
- **THEN** each construction returns an error naming that option, and no classifier
- **AND** the test that verifies this is `TestTierOneHintsAreTyped`

### Requirement: A question's time ranges come from one instant

The keyword tier SHALL compute every time range of a question from a single instant read once per question, in
that instant's zone.

#### Scenario: Fixed instants

- **WHEN** the ranges for "today", "yesterday", "last 3 hours", "this week" and "this month" are computed at fixed
  instants in fixed zones, including a Sunday and a daylight-saving change
- **THEN** each range equals the expected start and end exactly
- **AND** the test that verifies this is `TestTimeRangesFromOneInstant`

### Requirement: Out-of-range numbers are refused

`ClassifierChain.ClassifyQuery` SHALL return no result and an invalid-input error naming the phrase when `last N
hours` or `last N days` has N below 1 or a span that does not fit a `time.Duration`, or `within R of lat,lon` has a
latitude outside [-90, 90], a longitude outside [-180, 180] or a radius not finite; `KeywordClassifier.ClassifyQuery`
SHALL give no hint for that phrase. A box SHALL have North at most 90 and South at least -90; a box that reaches a
pole SHALL span West -180 to East 180.

#### Scenario: Overflow and bad coordinates

- **WHEN** the queries are "events in the last 3000000 hours", "within 5km of 95,10" and "within 5km of 10,500"
- **THEN** the chain returns no result and an error for which `errs.IsInvalid` is true, naming the phrase
- **AND** "within 5km of 89.9999,10" gives a result with North 90 and West -180, East 180, and no error
- **AND** the test that verifies this is `TestOutOfRangeNumbersAreRefused`
