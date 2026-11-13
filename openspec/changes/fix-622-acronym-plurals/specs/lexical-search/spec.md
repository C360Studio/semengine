# lexical-search Delta

## MODIFIED Requirements

### Requirement: The tokenizer splits text into words

`lexical.Tokens` SHALL split text into runs of Unicode letters and digits. A run that ends in a lower-case `s`
right after two or more capitals SHALL drop that `s`: it is the acronym's plural. Each run SHALL then split at
every step from a lower-case letter to a capital and before the last capital of a capital run followed by lower
case (a capital is a letter that lower-casing changes). It SHALL lower-case each part, split it again at any
non-letter, non-digit, and keep one-letter words, but not the joined form of a split identifier. Index and query
SHALL use it alone.

#### Scenario: Words from code and prose

- **WHEN** the text is `withRetry HTTPServer graph.query.status docker compose down -v pump-7 Services`
- **THEN** the words are `retry http server graph query status docker compose down v pump 7 service` (`with` is a
  stopword)
- **AND** the test that verifies this is `TestTokens`

#### Scenario: Acronym plurals

- **WHEN** the texts are `IDs`, `URLs`, `UUIDs`, `CPUs` and `EntityIDsByType`
- **THEN** the words are `id`, `url`, `uuid`, `cpu`, and `entity i ds type` (an `s` inside a run is not dropped)
- **AND** the test that verifies this is `TestTokens`

#### Scenario: An acronym meets its plural in either case

- **WHEN** an index holds one entity for each of `ID`, `id`, `IDs` and `ids`, and likewise for URL, API, PR, ADR
  and UUID, and each form is the query
- **THEN** each query returns exactly the four entities of its acronym
- **AND** the test that verifies this is `TestAcronymPluralsMeetTheirSingular`

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
`aes`, `ees` or `oes`: drop the `s`; (5) over 2 bytes, ending in `s` but not `us` or `ss`: drop the `s`. Folding
twice SHALL equal folding once.

#### Scenario: Plurals on both sides of the rules

- **WHEN** each of `service services`, `query queries`, `class classes`, `match matches`, `box boxes`,
  `hash hashes`, `cache caches`, `index indexes`, `process processes`, `address addresses`, `id ids` and `pr prs` is
  tokenized, and so is each of `bus buses`, `status statuses` and `cookie cookies`
- **THEN** each pair in the first list gives one word, and each pair in the second gives two different words
- **AND** the test that verifies this is `TestPluralRules`, and `TestPropFoldIsIdempotent` checks folding twice on
  generated words

#### Scenario: A singular finds its plural

- **WHEN** an entity holds `services` and `processes`, and the queries are `service` and `process`
- **THEN** each query returns the entity
- **AND** the test that verifies this is `TestSingularFindsPlural`
