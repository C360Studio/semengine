# lexical-search Delta

## ADDED Requirements

### Requirement: The tokenizer splits text into words

`lexical.Tokens` SHALL split text into runs of Unicode letters and digits, split each run at every lower-to-upper
step and before the last capital of a run of capitals followed by lower case, lower-case each part and keep
one-letter parts. The joined form of a split identifier SHALL NOT be held. Index and query SHALL use this tokenizer
alone.

#### Scenario: Words from code and prose

- **WHEN** the text is `withRetry HTTPServer graph.query.status docker compose down -v pump-7 Services`
- **THEN** the words are `retry http server graph query status docker compose down v pump 7 service` (`with` is a
  stopword)
- **AND** the test that verifies this is `TestTokens`

#### Scenario: A lower-case query for a split identifier

- **WHEN** an entity holds `withRetry` and the queries are `withRetry`, `retry` and `withretry`
- **THEN** the first two return the entity and the third returns nothing
- **AND** the test that verifies this is `TestJoinedIdentifierIsNotHeld`

#### Scenario: Any text

- **WHEN** generated text is tokenized, and its words are joined with spaces and tokenized again
- **THEN** no call panics, every word is lower case, at most 32 bytes and free of separators, and the second result
  equals the first
- **AND** the test that verifies this is `FuzzTokens`

### Requirement: A word over 32 bytes is dropped

`lexical.Tokens` SHALL drop a part longer than 32 bytes after splitting.

#### Scenario: Long runs

- **WHEN** the text holds `TestComponentManagerStopAfterProviderFailureDoesNotWaitForUnlaunchedSupervisor` and a
  40-character lower-case hex commit SHA
- **THEN** the identifier gives its words and the SHA gives none
- **AND** the test that verifies this is `TestTokens`

### Requirement: Stopwords are dropped

`lexical.Tokens` SHALL drop the package's 30 stopwords: the, a, an, and, or, but, in, on, at, to, for, of, with, by,
from, as, is, was, are, be, has, have, had, do, does, did, will, would, could and should.

#### Scenario: A stopword in an identifier

- **WHEN** the text is `withRetry not found`
- **THEN** the words are `retry not found`
- **AND** the test that verifies this is `TestTokens`

### Requirement: Plurals fold by five ordered rules

`lexical.Tokens` SHALL fold each word `w` by the first rule that applies: (1) over 4 bytes, ending in `es` after `ss`,
`x`, `ch` or `sh`: drop the `es`; (2) over 3 bytes, ending in `e` after them: drop the `e`; (3) over 4 bytes, ending
in `ies` but not `eies` or `aies`: replace `ies` with `y`; (4) over 3 bytes, ending in `es` but not `aes`, `ees` or
`oes`: drop the `s`; (5) over 3 bytes, ending in `s` but not `us` or `ss`: drop the `s`. Folding twice SHALL equal
folding once.

#### Scenario: Plurals on both sides of the rules

- **WHEN** each of `service services`, `query queries`, `class classes`, `match matches`, `box boxes`,
  `hash hashes`, `cache caches`, `index indexes`, `process processes` and `address addresses` is tokenized, and so is
  each of `bus buses`, `status statuses` and `cookie cookies`
- **THEN** each pair in the first list gives one word, and each pair in the second gives two different words
- **AND** the test that verifies this is `TestPluralRules`, and `TestPropFoldIsIdempotent` checks folding twice on
  generated words

#### Scenario: A singular finds its plural

- **WHEN** an entity holds `services` and `processes`, and the queries are `service` and `process`
- **THEN** each query returns the entity
- **AND** the test that verifies this is `TestSingularFindsPlural`

### Requirement: An entity is written whole, replaced and deleted

`Index.Put` SHALL replace everything an entity held with the words of its `Name` and `Body`, refuse an entity ID that
`semtypes.ValidateEntityID` refuses, and remove the entity when the document has no words. `Index.Delete` SHALL
remove an entity and SHALL do nothing for one the index does not hold.

#### Scenario: Replace, empty and delete

- **WHEN** an entity is written with `alpha`, written again with `beta`, written with an empty document, then
  written with `gamma` and deleted
- **THEN** after each step a search for each word returns the entity exactly when its last write held that word,
  and `Len` counts it only while it holds a word
- **AND** the test that verifies this is `TestPutReplacesAndDeletes`

### Requirement: A search ranks by field-weighted BM25

A search SHALL score an entity as the sum, over the query's words it holds, of BM25 with Lucene's IDF, `k1` 1.2 and
`b` 0.75, where term frequency is `w` times the frequency in `Name` plus that in `Body`, length is `w` times the
length of `Name` plus that of `Body`, and `w` is the name weight the design records.

#### Scenario: A name word outranks a body word

- **WHEN** two entities of equal length hold the query word once, one in `Name` and one in `Body`
- **THEN** the first ranks above the second
- **AND** the test that verifies this is `TestNameOutranksBody`

#### Scenario: Against a reference model

- **WHEN** a generated history of writes and deletes runs against the index and against a test-owned naive BM25F over
  a map of documents
- **THEN** every generated query returns the model's entities, in the model's order wherever two scores differ by more
  than 1e-9, with the model's scores within that tolerance
- **AND** the test that verifies this is `TestPropIndexMatchesModel`

### Requirement: Statistics are read when the search runs

The entity count, each word's entity count and the average length SHALL be read from the index when a search runs,
never stored from earlier writes.

#### Scenario: Two histories that end in the same entities

- **WHEN** a generated set of entities is written to one index in one order and to another in a different order,
  with extra entities written and deleted on the way
- **THEN** every generated query returns equal results from both, scores included
- **AND** the test that verifies this is `TestPropRankingIgnoresHistory`

### Requirement: Hits are ordered and counted before the limit

A search SHALL return the entities scoring above 0, ordered by score and then by entity ID, cut to the limit, and
`Matched` SHALL be the number of matches before the limit.

#### Scenario: Scores worked by hand

- **WHEN** an index holds five entities whose scores for a two-word query were computed by hand
- **THEN** the search returns them in that order, with those scores and `Matched` 5, and with limit 2 it returns the
  first two with `Matched` 5
- **AND** the test that verifies this is `TestSearchRanksByBM25`

### Requirement: A word no entity holds matches nothing

A search SHALL return no hit for a query whose words no held entity holds.

#### Scenario: Made-up words

- **WHEN** a generated query holds only words absent from a generated corpus
- **THEN** the search returns no hit and `Matched` 0
- **AND** the test that verifies this is `TestPropAbsentWordsMatchNothing`

### Requirement: A query holds words, quoted phrases and excluded words

`lexical.ParseQuery` SHALL read plain words, phrases in double quotes, and words or phrases preceded by `-` at the
start of a term outside quotes, all tokenized as documents are. A `-` inside quotes SHALL be text, and an unclosed
quote SHALL end at the end of the text.

#### Scenario: The three forms

- **WHEN** the query is `retry -test "docker compose down -v"`
- **THEN** it parses to one word, one excluded word and one phrase of four words
- **AND** the test that verifies this is `TestParseQuery`, with `FuzzParseQuery` showing no input panics

### Requirement: A query with nothing to search for is refused

A phrase or excluded term left with no word after tokenizing SHALL be ignored. A query left with no plain word and no
phrase SHALL be refused with an error that `errors.Is` matches to `ErrEmptyQuery`, and one with more than
`MaxQueryWords` (32) words with `ErrQueryTooLong`.

#### Scenario: Forms and refusals

- **WHEN** the queries are `retry "to be" -the`, `the of and`, `"to be"`, `-test`, `""` and 33 distinct words
- **THEN** the first parses to the word `retry` alone, the next four are refused with `ErrEmptyQuery`, and the last
  is refused with `ErrQueryTooLong`
- **AND** the test that verifies this is `TestParseQuery`

### Requirement: A quoted phrase matches only its words in order

A phrase SHALL match an entity only where its words, after stopwords are removed, stand next to each other in that
order in the entity's `Name` or `Body`.

#### Scenario: The words apart and together

- **WHEN** one entity holds `docker compose down -v` and another holds the same words apart, and the query is the
  phrase in quotes
- **THEN** only the first is returned
- **AND** the test that verifies this is `TestPhraseNeedsAdjacentWords`

### Requirement: An excluded word removes its entities

A query term preceded by `-` SHALL remove from the answer every entity that holds its words, adjacent, in any field.

#### Scenario: A file name in the body

- **WHEN** two entities hold `retry`, one of them also holds `retry_test.go`, and the query is `retry -test`
- **THEN** only the other entity is returned
- **AND** the test that verifies this is `TestExcludedWordRemovesEntities`

### Requirement: Scope applies before the limit and leaves scores alone

A search with scope prefixes SHALL return the best entities whose IDs `graph.MatchesAnyIDPrefix` accepts, up to the
limit, with the scores an unscoped search gives them. It SHALL refuse a malformed prefix or a limit below 1.

#### Scenario: Better matches outside the scope

- **WHEN** entities outside the scope outscore every entity inside it, and the limit is smaller than the matches
  inside
- **THEN** the answer holds only entities inside the scope, the best of them, with their unscoped scores, and
  `Matched` counts the matches inside
- **AND** the test that verifies this is `TestScopeBeforeLimit`

### Requirement: A search during a write sees it whole or not at all

`Index` SHALL be safe for concurrent use, and a search running during a `Put` or `Delete` SHALL return the result it
would return before that write or after it, never a mix.

#### Scenario: One entity switching between two documents

- **WHEN** one goroutine writes an entity alternately with two documents while others search, under the race detector
- **THEN** every result equals one of the two results worked out beforehand on fresh indexes
- **AND** the test that verifies this is `TestSearchDuringWrites`

### Requirement: The scorecard holds its floor

The package's frozen known-item corpus SHALL have the right entity in the first ten at or above the floor the design
records, and every behaviour question SHALL pass.

#### Scenario: The fixed questions

- **WHEN** the scorecard's questions are asked of an index holding its corpus
- **THEN** R@10 is at or above the floor, and each behaviour question's expected entities are present or absent as it
  says
- **AND** the tests that verify this are `TestScorecardHoldsItsFloor` and `TestScorecardBehaviour`

### Requirement: A product can use the package as documented

The exported surface SHALL be usable from outside the package as the package README shows.

#### Scenario: The README's example

- **WHEN** an example in package `lexical_test` writes two entities and searches
- **THEN** its output matches the README
- **AND** the test that verifies this is `ExampleIndex`

### Requirement: The package imports no NATS client

`go list -deps ./graph/lexical` SHALL list no `natsclient` and no path under `github.com/nats-io`.

#### Scenario: The package's dependencies

- **WHEN** the contract test lists the package's dependencies, and a fixture module whose package reaches NATS
- **THEN** the real package passes and the fixture fails, naming the path
- **AND** the tests that verify this are `TestLexicalImportsNoNATS` and `TestLexicalImportsNoNATSSensitivity`
