# lexical-search Delta

## ADDED Requirements

### Requirement: A search ranks by BM25 over the entities the index holds

`graph/lexical` SHALL score an entity for a query as the sum, over the query's words the entity holds, of the BM25
term score, with the entity count, each word's entity count and the average length read from the index at the time
of the search. It SHALL return the entities scoring above 0, best first, ties by entity ID.

#### Scenario: A small corpus with scores worked by hand

- **WHEN** an index holds five entities whose BM25 scores for a two-word query were computed by hand
- **THEN** the search returns them in that order with those scores
- **AND** the test that verifies this is `TestSearchRanksByBM25`

### Requirement: A word no entity holds matches nothing

A search whose words are held by no entity SHALL return no entity.

#### Scenario: Made-up words

- **WHEN** a generated query holds only words absent from a generated corpus
- **THEN** the search returns no entity
- **AND** the test that verifies this is `TestAbsentWordsMatchNothing`

### Requirement: A ranking depends only on the entities held

Two indexes that hold the same entities SHALL answer every search alike, whatever writes, replacements and deletions
led there and in whatever order.

#### Scenario: Two histories that end in the same entities

- **WHEN** a generated set of entities is written to one index in one order and to another in a different order with
  extra entities written and deleted on the way
- **THEN** every generated query returns the same entities with the same scores from both
- **AND** the test that verifies this is `TestRankingIgnoresHistory`

### Requirement: A singular finds its plural

A word and its regular plural SHALL be one term, in entities and in queries.

#### Scenario: Plurals of words ending in e

- **WHEN** an entity holds "services" and the query is "service", and likewise for "actions" and "action"
- **THEN** the entity is returned
- **AND** the test that verifies this is `TestSingularFindsPlural`

### Requirement: A quoted phrase matches only its words in order

A query's quoted phrase SHALL match an entity only where the phrase's words stand next to each other in that order.

#### Scenario: The words apart and together

- **WHEN** one entity holds "docker compose down -v" and another holds the same words apart, and the query is the
  phrase in quotes
- **THEN** only the first is returned
- **AND** the test that verifies this is `TestPhraseNeedsAdjacentWords`

### Requirement: An excluded word removes its entities

A query word marked with a leading `-` SHALL remove every entity that holds it from the answer.

#### Scenario: A match that also holds the excluded word

- **WHEN** two entities hold "retry" and one of them also holds "test", and the query is "retry -test"
- **THEN** only the entity without "test" is returned
- **AND** the test that verifies this is `TestExcludedWordRemovesEntities`

### Requirement: Scope applies before the limit

A search limited to entity-ID prefixes SHALL return the best entities under those prefixes, up to the limit.

#### Scenario: Better matches outside the scope

- **WHEN** the entities outside the scope outscore every entity inside it and the limit is smaller than the matches
  inside
- **THEN** the answer holds only entities inside the scope, the best of them
- **AND** the test that verifies this is `TestScopeBeforeLimit`

### Requirement: The scorecard holds its floor

The package's fixed corpus and questions SHALL be answered with the right entity in the first ten at or above the
floor the design records.

#### Scenario: The fixed questions

- **WHEN** the scorecard's questions are asked of an index holding its corpus
- **THEN** the share answered in the first ten is at or above the floor
- **AND** the test that verifies this is `TestScorecardHoldsItsFloor`

### Requirement: The package imports no NATS client

`go list -deps ./graph/lexical` SHALL name no NATS module.

#### Scenario: The package's dependencies

- **WHEN** `go list -deps ./graph/lexical` runs on the tree
- **THEN** no listed path is under `github.com/nats-io`
- **AND** the test that verifies this is `TestLexicalImportsNoNATS`
