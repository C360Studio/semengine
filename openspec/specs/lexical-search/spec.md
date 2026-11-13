# lexical-search Specification

## Purpose
The `graph/lexical` package is tier 0's word index: an in-memory index over each entity's name and body text that
ranks matches with BM25F, needs no external provider, and imports no NATS client. This capability states how text
becomes words, how a query is read (plain words, quoted phrases, `-` exclusions, reserved syntax), how matches are
scoped and ranked, and the scorecard floor the ranking must hold. The `processor/graph-lexical` component fills an index from live data.

## Requirements

### Requirement: The tokenizer splits text into words

`lexical.Tokens` SHALL split text into runs of Unicode letters and digits, then split each run at every step from a
lower-case letter to a capital and before the last capital of a capital run followed by lower case (a capital is a
letter that lower-casing changes). It SHALL lower-case each part, split it again at any non-letter, non-digit, and
keep one-letter words, but not the joined form of a split identifier. Index and query SHALL use it alone.

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
- **THEN** no call panics, every word equals its lower-casing, is at most 32 bytes and holds only letters and digits,
  and the second result equals the first
- **AND** the test that verifies this is `FuzzTokens`

### Requirement: Plurals fold by five ordered rules

`lexical.Tokens` SHALL fold each lower-cased word `w` by the first rule that applies: (1) over 4 bytes, ending in `es`
after `ss`, `x`, `ch` or `sh`: drop the `es`; (2) over 3 bytes, ending in `e` after them: drop the `e`; (3) over 4
bytes, ending in `ies` but not `eies` or `aies`: replace `ies` with `y`; (4) over 3 bytes, ending in `es` but not
`aes`, `ees` or `oes`: drop the `s`; (5) over 3 bytes, ending in `s` but not `us` or `ss`: drop the `s`. Folding
twice SHALL equal folding once.

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

### Requirement: Stopwords and long words are dropped after folding

`lexical.Tokens` SHALL drop a word when the word or its fold is one of the package's 30 stopwords (the, a, an, and,
or, but, in, on, at, to, for, of, with, by, from, as, is, was, are, be, has, have, had, do, does, did, will, would,
could, should), or when its fold is over 32 bytes. Otherwise it SHALL keep the fold.

#### Scenario: Stopwords and their plurals

- **WHEN** the texts are `withRetry not found` and `wills does haves ands ares thes fors withs`
- **THEN** the first gives `retry not found` and the second gives no word
- **AND** the test that verifies this is `TestTokens`

#### Scenario: Long runs

- **WHEN** the text holds `TestComponentManagerStopAfterProviderFailureDoesNotWaitForUnlaunchedSupervisor` and a
  40-character lower-case hex commit SHA
- **THEN** the identifier gives `test component manager stop after provider failure not wait unlaunched supervisor`
  and the SHA gives no word
- **AND** the test that verifies this is `TestTokens`

### Requirement: An entity is written whole, replaced and deleted

`Index.Put` SHALL replace everything an entity held with the words of its `Name` and `Body`, refuse an entity ID that
`semtypes.ValidateEntityID` refuses with an error matching `ErrInvalidEntityID`, and remove the entity when the
document has no words. `Index.Delete` SHALL remove an entity and SHALL do nothing for one the index does not hold.

#### Scenario: Replace, empty and delete

- **WHEN** an entity is written with `alpha`, written again with `beta`, written with an empty document, then
  written with `gamma` and deleted, and an entity with ID `not an id` is written
- **THEN** after each step a search for each word returns the entity exactly when its last write held that word,
  `Len` counts it only while it holds a word, and the bad ID is refused with `ErrInvalidEntityID`
- **AND** the test that verifies this is `TestPutReplacesAndDeletes`

### Requirement: A search ranks by field-weighted BM25

A search SHALL score an entity as the sum, over the distinct words of the query's plain words and phrases that it
holds, of BM25 with Lucene's IDF, `k1` 1.2 and `b` 0.75. Term frequency SHALL be `NameWeight` times the count in
`Name` plus the count in `Body`, and length `NameWeight` times the words held from `Name` plus those from `Body`.

#### Scenario: A name word outranks a body word

- **WHEN** two entities of equal length hold the query word once, one in `Name` and one in `Body`
- **THEN** the first ranks above the second
- **AND** the test that verifies this is `TestNameOutranksBody`

### Requirement: Statistics are read when the search runs

The entity count, each word's entity count and the average of the weighted length over held entities SHALL be read
from the index when a search runs, never kept from earlier writes.

#### Scenario: Two histories that end in the same entities

- **WHEN** a generated set of entities is written to one index in one order and to another in a different order,
  with extra entities written and deleted on the way
- **THEN** every generated query returns equal results from both, scores included
- **AND** the test that verifies this is `TestPropRankingIgnoresHistory`

### Requirement: Hits are ordered and counted before the limit

A search SHALL return the entities it matches, ordered by score and then by entity ID, cut to the limit, and
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

`lexical.ParseQuery` SHALL read text in double quotes as a phrase, a term starting with a single `-` followed by a
letter, digit or quote as excluded, and the rest as plain words, all tokenized as documents are. Quoted text SHALL be
literal: no rule but tokenizing applies inside quotes. An unclosed quote SHALL end at the end of the text.

#### Scenario: The three forms

- **WHEN** the queries are `retry -test "docker compose down -v"`, `go test -race`, `go test "-race"` and
  `deploy --force`
- **THEN** the first parses to one word, one excluded word and one phrase of four words; the second to the words
  `go test` and the excluded word `race`; the third to the words `go test` and the phrase `race`; and the fourth to
  the words `deploy force`
- **AND** the test that verifies this is `TestParseQuery`, with `FuzzParseQuery` showing no input panics

### Requirement: A query with nothing to search for is refused

A phrase or excluded term left with no word after tokenizing SHALL be ignored. A query left with no plain word and no
phrase SHALL be refused with an error matching `ErrEmptyQuery`, and one with more than `graph.MaxSearchTerms`
distinct words, counting plain, phrase and excluded words after tokenizing, with an error matching `ErrQueryTooLong`.

#### Scenario: Forms and refusals

- **WHEN** the queries are `retry "to be" -the`, `the of and`, `"to be"`, `-test`, `""` and 17 distinct words
- **THEN** the first parses to the word `retry` alone, the next four are refused with `ErrEmptyQuery`, and the last
  is refused with `ErrQueryTooLong`
- **AND** the test that verifies this is `TestParseQuery`

### Requirement: Syntax kept for later query forms is refused

`ParseQuery` SHALL refuse, with an error matching `ErrReservedSyntax`, a term outside quotes that starts with `name:`
or `body:` followed by more text, or that ends in a letter or digit followed by `*` or `~`. Every other character
outside letters and digits, a lone `*` or `~` among them, SHALL separate words, as in documents.

#### Scenario: Reserved and ordinary punctuation

- **WHEN** the queries are `name:retry`, `retr*`, `retry~`, `void*`, `pkg.Func() name: retry`, `SELECT * FROM users`,
  `java.util.*`, `**bold**`, `cd ~` and `"name:retry void*"`
- **THEN** the first four are refused with `ErrReservedSyntax`; the next five parse to the words `pkg func name
  retry`, `select user`, `java util`, `bold` and `cd`; and the last parses to the phrase `name retry void`
- **AND** the test that verifies this is `TestParseQueryReservesForms`

### Requirement: Phrases and exclusions filter, words rank

A search SHALL match an entity only when it holds every phrase of the query, holds no excluded term, and holds at
least one plain word or phrase word. A query whose phrase no entity holds SHALL return no hit and `Matched` 0.

#### Scenario: A phrase beside a plain word

- **WHEN** entity A holds `retry` and `docker compose`, B holds `retry` and `docker` and `compose` apart, C holds
  `docker compose` alone, and the query is `retry "docker compose"`
- **THEN** the answer is A and then C, and B is not returned
- **AND** the test that verifies this is `TestCombinedQuery`

#### Scenario: A phrase no entity holds

- **WHEN** entities hold `retry` and the query is `retry "no such phrase"`
- **THEN** the search returns no hit, `Matched` 0 and no error
- **AND** the test that verifies this is `TestUnmatchedPhraseFindsNothing`

#### Scenario: Against a reference model

- **WHEN** a generated history of writes and deletes runs against the index and against a test-owned naive BM25F over
  a map of documents, with generated queries of plain words, up to two phrases and up to two excluded terms
- **THEN** every query returns the model's entities, in the model's order wherever two scores differ by more than
  1e-9, with the model's scores within that tolerance
- **AND** the test that verifies this is `TestPropIndexMatchesModel`

### Requirement: A quoted phrase matches only its words in order within one field

A phrase SHALL match an entity only where its words, after tokenizing, stand next to each other in that order within
`Name` or within `Body`.

#### Scenario: The words apart and together

- **WHEN** one entity holds `docker compose down -v` and another holds the same words apart, and the query is the
  phrase in quotes
- **THEN** only the first is returned
- **AND** the test that verifies this is `TestPhraseNeedsAdjacentWords`

#### Scenario: Across the field boundary

- **WHEN** an entity's `Name` ends in `docker` and its `Body` starts with `compose`, and the query is `"docker compose"`
- **THEN** the entity is not returned
- **AND** the test that verifies this is `TestPhraseStaysInOneField`

### Requirement: An excluded word removes its entities

A query term preceded by `-` SHALL remove from the answer every entity that holds its words, adjacent, in any field.

#### Scenario: A file name in the body

- **WHEN** two entities hold `retry`, one of them also holds `retry_test.go`, and the query is `retry -test`
- **THEN** only the other entity is returned
- **AND** the test that verifies this is `TestExcludedWordRemovesEntities`

### Requirement: Scope applies before the limit and leaves scores alone

A search with scope prefixes SHALL return the best entities whose IDs `graph.MatchesAnyIDPrefix` accepts, up to the
limit, with the scores an unscoped search gives them. It SHALL refuse a malformed prefix with an error matching
`ErrInvalidScope` and a limit below 1 with one matching `ErrInvalidLimit`.

#### Scenario: Better matches outside the scope

- **WHEN** entities outside the scope outscore every entity inside it, and the limit is smaller than the matches
  inside
- **THEN** the answer holds only entities inside the scope, the best of them, with their unscoped scores, and
  `Matched` counts the matches inside; a scope of `c360.` and a limit of 0 are refused with those errors
- **AND** the test that verifies this is `TestScopeBeforeLimit`

### Requirement: A search during a write sees it whole or not at all

`Index` SHALL be safe for concurrent use, and a search running during a `Put` or `Delete` SHALL return the result it
would return before that write or after it, never a mix.

#### Scenario: One entity switching between two documents

- **WHEN** one goroutine writes an entity alternately with two documents while others search, under the race detector
- **THEN** every result equals one of the two results worked out beforehand on fresh indexes
- **AND** the test that verifies this is `TestSearchDuringWrites`

### Requirement: The scorecard holds its floor

The package's frozen known-item corpus SHALL have the right entity in the first ten at or above the floor set in
`TestScorecardHoldsItsFloor`, whose value design D8 records, and every behaviour question SHALL pass.

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
