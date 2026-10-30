# query-classification

## MODIFIED Requirements

### Requirement: The chain answers from the first tier that produces a hint

`ClassifierChain.ClassifyQuery` SHALL return the keyword tier's hints (tier 0, confidence 1.0) when the keyword
classifier finds any; otherwise the example classifier's match at or above its threshold (tier 1, the match's score)
when one is composed; otherwise the LLM tier's result (tier 3) when one is composed and succeeds; otherwise, while the
context is live, the default result: tier 0, no hints, confidence 1.0. A failure of the LLM tier under a live context
SHALL yield the default result. When the keyword tier found no hint and excluded an operation, a tier 1 or tier 3
answer that requests that operation SHALL NOT be used, and the chain SHALL go on to the next tier or the default
result. An answer requests aggregation when it has an `aggregation_type` hint or `strategy` `aggregation`, similarity
when `use_embeddings` is true or `strategy` is `semantic`, and ranking when `ranking_intent` is true.

#### Scenario: A keyword hint stops the ladder

- **WHEN** the query is "events from yesterday" and an LLM tier whose fake client counts its calls is composed
- **THEN** the result is tier 0 with a `time_range` hint and the LLM client was never called
- **AND** the test that verifies this is `TestChainKeywordHintStopsTheLadder`

#### Scenario: The LLM tier fails under a live context

- **WHEN** no earlier tier produces a hint and the LLM client returns an error while the context is live
- **THEN** the result is the default result
- **AND** the test that verifies this is `TestChainLLMFailureGivesDefault`

#### Scenario: A product composes an example classifier

- **WHEN** a product loads two example files with `LoadDomainExamples`, builds an `EmbeddingClassifier`, composes it
  into a chain beside a `KeywordClassifier` held as a `Classifier`, and asks a question no keyword pattern matches
  but an example does
- **THEN** the result is tier 1 with that example's options and its score
- **AND** the test that verifies this is `TestEmbeddingClassifierAsAProduct`

#### Scenario: A later tier cannot restore an excluded operation

- **WHEN** the query is "Do not count sensors; list their names", an example classifier holds an example whose
  options carry `aggregation_type` `count` and match that query, and a fake LLM client answers `strategy`
  `aggregation`
- **THEN** the result is the default result
- **AND** the test that verifies this is `TestChainKeepsExclusions`

#### Scenario: A fallback that requests no excluded operation still answers

- **WHEN** the query is "Do not count sensors; list their names" and the only later answer is an example whose
  options carry `strategy` `pathrag`
- **THEN** the result is tier 1 with that example's options
- **AND** the test that verifies this is `TestChainKeepsExclusions`

## ADDED Requirements

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

The keyword tier SHALL give no hint for an aggregation, similarity or ranking whose trigger follows a negating
word (`not`, `no`, `never`, `don't`, `do not`, `without`) in the same clause, lies inside a balanced pair of double
quotes, or is one of two different operations joined by `or` in one clause; other hints SHALL stand. A clause is
text between `;`, `,`, `.`, `:`, `?`, `!` or the word `but`.

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

For any query the keyword tier SHALL return without panicking, and a query whose only triggers lie inside double
quotes SHALL yield no operation hint.

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

### Requirement: A question's time ranges come from one instant

The keyword tier SHALL compute every time range of a question from a single instant read once per question, in
that instant's zone.

#### Scenario: Fixed instants

- **WHEN** the ranges for "today", "yesterday", "last 3 hours", "this week" and "this month" are computed at fixed
  instants in fixed zones, including a Sunday and a daylight-saving change
- **THEN** each range equals the expected start and end exactly
- **AND** the test that verifies this is `TestTimeRangesFromOneInstant`

### Requirement: Out-of-range numbers give no hint

The keyword tier SHALL give no `time_range` hint for `last N hours` or `last N days` when N is below 1 or the span
does not fit a `time.Duration`, and no `geo_bounds` hint when the latitude is outside [-90, 90], the longitude
outside [-180, 180] or the radius not finite. A box SHALL have North at most 90 and South at least -90; a box that
reaches a pole SHALL span West -180 to East 180.

#### Scenario: Overflow and bad coordinates

- **WHEN** the queries are "events in the last 3000000 hours", "within 5km of 95,10" and "within 5km of 10,500"
- **THEN** none has the hint its phrase would give
- **AND** "within 5km of 89.9999,10" gives North 90 and West -180, East 180
- **AND** the test that verifies this is `TestOutOfRangeNumbersGiveNoHint`
