# fusion Delta

## RENAMED Requirements

- FROM: `### Requirement: An NL answer says the word index found its seeds`
- TO: `### Requirement: An NL answer says where its seeds came from`

## MODIFIED Requirements

### Requirement: Subjects come from the verb table

Each request SHALL go to the subject of its row in `graph.QueryVerbs()`: graph-query's `mergedSearch` row for an NL
resolve and graph-query's rows for every other call. When the table lacks a row the client uses, construction SHALL
be refused with an error naming the verb.

#### Scenario: Subjects from the table

- **WHEN** each `RetrievalClient` method is called on a recording requester
- **THEN** each request's subject is its row's subject (`TestSubjectsComeFromVerbTable`)

#### Scenario: A missing row

- **WHEN** the verb lookup is given a table without the `batch` row, or without graph-query's `mergedSearch` row
- **THEN** it refuses, naming the missing verb (`TestVerbLookupNamesMissingRow`)

### Requirement: A word search passes on what it could not read

An NL `Resolve` SHALL copy the merged search reply's `bodies_not_indexed` and `excluded`, the word index's counts,
into the `Resolution`; a symbol or prefix `Resolve` SHALL leave both empty.

#### Scenario: Word-search counts

- **WHEN** the merged search reply reports 2 bodies not indexed and the excluded word `race`
- **THEN** the `Resolution` carries 2 and `["race"]`, and a symbol or prefix resolve carries 0 and none
  (`TestWordIndexCountsReachResolution`)

### Requirement: An NL answer says where its seeds came from

`ProvenanceForMode` SHALL map `nl` to `lexical`, `symbol` and `prefix` to `deterministic`, and any other mode to
`embedding`. A response SHALL carry its `Resolution`'s `Provenance` when the resolve set one, and its lens's mode's
provenance otherwise, a failed resolve included. An NL `Resolve` in `fusionnats` SHALL set `embedding` when the
reply's `merge` is present and honoured, and `lexical` otherwise.

#### Scenario: Each mode's provenance

- **WHEN** `ProvenanceForMode` is called with `nl`, `symbol`, `prefix` and an unknown mode
- **THEN** it returns `lexical`, `deterministic`, `deterministic` and `embedding` (`TestProvenanceForMode`)

#### Scenario: A merged answer

- **WHEN** the merged search reply's `merge` is honoured
- **THEN** the `Resolution` and the response carry `embedding` (`TestMergedResolveProvenance`)

#### Scenario: Words alone

- **WHEN** the reply has no `merge`, or a `merge` not honoured
- **THEN** the `Resolution` and the response carry `lexical` (`TestMergedResolveProvenance`)

#### Scenario: A client that sets none

- **WHEN** a `RetrievalClient` returns a `Resolution` with no `Provenance`
- **THEN** the response carries its mode's provenance (`TestResolutionProvenanceWins`)

## REMOVED Requirements

### Requirement: An NL resolve asks the word index

**Reason**: An NL resolve no longer asks graph-lexical's word search; it asks graph-query's merged search, so the
scenario "Seeds from the word index" is no longer true. Its other three scenarios are kept, adjusted for the
graph-query hop, in the ADDED requirement "An NL resolve asks the merged search".

**Migration**: none in this repository; consumers see the subject change described in `pkg/fusion/MIGRATION.md`.

## ADDED Requirements

### Requirement: An NL resolve asks the merged search

An NL `Resolve` SHALL send one `graph.LexicalSearchRequest` holding the query, the limit and the scope (left out when
empty) to graph-query's `mergedSearch` row of `graph.QueryVerbs()`, and SHALL return the reply's results as seeds in
order, each with its `score` as the mode's own score. The question SHALL pass unchanged, and a refusal SHALL reach
the caller with its class, code and detail.

#### Scenario: Seeds from the merged search

- **WHEN** an NL resolve is answered with three results
- **THEN** the one request went to `graph.query.mergedSearch` with the query, limit and scope, and the seeds are the
  three results in order with their scores: the word index's score, or the merged score when the reply merged
  (`TestNLResolveAsksTheMergedSearch`)

#### Scenario: A question the word index refuses

- **WHEN** an NL question holds a syntax the word index refuses, such as `name:retry`
- **THEN** `Fuse` returns the word index's refusal, class invalid, code `invalid_request`
  (`TestNLWordIndexRefusalReturned`)

#### Scenario: The word index is not ready

- **WHEN** the merged search refuses an NL resolve transient `index_not_ready` (at tier 0, or at tier 1 when the
  embedding search failed too)
- **THEN** `Fuse` answers `deferred` with `defer_reason` `index_not_ready` (`TestNLIndexNotReadyDefers`)

#### Scenario: Meaning alone while the word index builds

- **WHEN** the reply holds the embedding list with `merge` honoured and detail `index_not_ready`
- **THEN** `Fuse` answers those entities as nodes, not deferred, with provenance `embedding` and no
  `bodies_not_indexed` or `excluded` (`TestNLMeaningAloneWhileWordIndexBuilds`)

#### Scenario: No word index composed

- **WHEN** graph-query runs and nothing serves `graph.lexical.query.search`
- **THEN** an NL `Fuse` returns a transient `internal` error naming that subject (`TestNLResolveWithoutWordIndex`)

#### Scenario: Words and meaning over real responders

- **WHEN** graph-query runs with `embedding_enabled` true, graph-lexical runs, and the embedding search answers an
  entity no word matches
- **THEN** `Fuse` answers that entity among its nodes with provenance `embedding` (`TestNLMergedOverRealResponders`)
