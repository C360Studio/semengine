# fusion Delta

## ADDED Requirements

### Requirement: An NL answer reports what the word index left out

Every response built after a natural-language `Resolve` answered SHALL carry that reply's `bodies_not_indexed` and
`excluded`, each omitted when 0 or empty. Responses after a symbol or prefix resolve SHALL omit both.

#### Scenario: Entities found by their inline text only

- **WHEN** an NL resolve returns seeds with `bodies_not_indexed` 2 and `excluded` `["race"]`
- **THEN** the response carries both beside its nodes (`TestNLAnswerCarriesWordIndexCounts`)

#### Scenario: A miss with excluded words

- **WHEN** an NL resolve returns no seeds with `excluded` `["race"]`
- **THEN** the miss response carries `excluded` `["race"]` (`TestNLAnswerCarriesWordIndexCounts`)

#### Scenario: A symbol resolve

- **WHEN** the lens picks symbol mode
- **THEN** the response has neither key (`TestSymbolAnswerOmitsCounts`)

### Requirement: An NL answer says the word index found its seeds

`ProvenanceForMode` SHALL map `nl` to `lexical`, `symbol` and `prefix` to `deterministic`, and any other mode to
`embedding`, and every response SHALL carry the provenance of its lens's mode.

#### Scenario: Each mode's provenance

- **WHEN** `ProvenanceForMode` is called with `nl`, `symbol`, `prefix` and an unknown mode
- **THEN** it returns `lexical`, `deterministic`, `deterministic` and `embedding` (`TestProvenanceForMode`)

### Requirement: An NL resolve asks the word index

An NL `Resolve` SHALL send one `graph.LexicalSearchRequest` holding the query, the limit and the scope (left out when
empty) to graph-lexical's `search` row of `graph.QueryVerbs()`, and SHALL return the hits as seeds in the reply's
order, each with its score as the mode's own score.

The question SHALL pass unchanged, so the word index's query rules apply, and its refusals SHALL reach the caller
with their class, code and detail.

#### Scenario: Seeds from the word index

- **WHEN** an NL resolve is answered with three hits
- **THEN** the one request went to `graph.lexical.query.search` with the query, limit and scope, and the seeds are
  the three hits in order with their scores (`TestNLResolveAsksTheWordIndex`)

#### Scenario: A question the word index refuses

- **WHEN** an NL question holds a syntax the word index refuses, such as `name:retry`
- **THEN** `Fuse` returns the word index's refusal, class invalid, code `invalid_request`
  (`TestNLWordIndexRefusalReturned`)

#### Scenario: The word index is not ready

- **WHEN** the word index refuses an NL resolve transient `index_not_ready`
- **THEN** `Fuse` answers `deferred` with `defer_reason` `index_not_ready` (`TestNLIndexNotReadyDefers`)

#### Scenario: No word index composed

- **WHEN** nothing serves `graph.lexical.query.search`
- **THEN** an NL `Fuse` returns a transient `internal` error naming that subject (`TestNLResolveWithoutWordIndex`)

## MODIFIED Requirements

### Requirement: Subjects come from the verb table

Each request SHALL go to the subject of its row in `graph.QueryVerbs()`: graph-lexical's `search` row for an NL
resolve and graph-query's rows for every other call. When the table lacks a row the client uses, construction SHALL
be refused with an error naming the verb.

#### Scenario: Subjects from the table

- **WHEN** each `RetrievalClient` method is called on a recording requester
- **THEN** each request's subject is its row's subject (`TestSubjectsComeFromVerbTable`)

#### Scenario: A missing row

- **WHEN** the verb lookup is given a table without the `batch` row, or without graph-lexical's `search` row
- **THEN** it refuses, naming the missing verb (`TestVerbLookupNamesMissingRow`)

### Requirement: A word search passes on what it could not read

An NL `Resolve` SHALL copy the word index reply's `bodies_not_indexed` and `excluded` into the `Resolution`; a symbol
or prefix `Resolve` SHALL leave both empty.

#### Scenario: Word-search counts

- **WHEN** the word index reply reports 2 bodies not indexed and the excluded word `race`
- **THEN** the `Resolution` carries 2 and `["race"]`, and a symbol or prefix resolve carries 0 and none
  (`TestWordIndexCountsReachResolution`)

## REMOVED Requirements

### Requirement: A word-search answer reports what it could not read

**Reason**: The embedding cache's counts are gone with NL resolve on the word index (design D2, D3). Replaced by "An
NL answer reports what the word index left out".
