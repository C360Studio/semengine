# fusion Delta

## MODIFIED Requirements

### Requirement: An NL answer says where its seeds came from

`ProvenanceForMode` SHALL map `nl` to `lexical`, `symbol` and `prefix` to `deterministic`, and any other mode to
`embedding`. A response SHALL carry its `Resolution`'s `Provenance` when the resolve set one, and its lens's mode's
provenance otherwise, a failed resolve included. An NL `Resolve` in `fusionnats` SHALL set `embedding` when at
least one result in the reply carries a `vector_rank`, and `lexical` otherwise. An NL `Resolve` in `fusionnats`
SHALL copy the reply's `merge` (`honoured`, `detail`, `reason`, `code`) into `Resolution.Merge`, and leave it nil when
the reply has none. Every response built after an NL resolve answered SHALL carry that `Merge` as `merge`; a symbol
or prefix resolve, a failed resolve, and a resolve with no `Merge` SHALL leave `merge` out.

#### Scenario: Each mode's provenance

- **WHEN** `ProvenanceForMode` is called with `nl`, `symbol`, `prefix` and an unknown mode
- **THEN** it returns `lexical`, `deterministic`, `deterministic` and `embedding` (`TestProvenanceForMode`)

#### Scenario: A merged answer

- **WHEN** the merged search reply holds a result with a `vector_rank`
- **THEN** the `Resolution` and the response carry `embedding` (`TestMergedResolveProvenance`)

#### Scenario: Words alone

- **WHEN** no result in the reply carries a `vector_rank`: the reply has no `merge`, a `merge` not honoured, or an
  honoured `merge` whose embedding list was empty or fell below the limit
- **THEN** the `Resolution` and the response carry `lexical` (`TestMergedResolveProvenance`)

#### Scenario: A client that sets none

- **WHEN** a `RetrievalClient` returns a `Resolution` with no `Provenance`
- **THEN** the response carries its mode's provenance (`TestResolutionProvenanceWins`)

#### Scenario: A merge that failed

- **WHEN** the merged search reply holds a `merge` not honoured, with its `detail`, `reason` and `code`
- **THEN** the `Resolution` carries `Merge` with `honoured` false and the three strings, and the response, with
  nodes, a miss or an all-seeds-unhydrated deferral, carries them as `merge` (`TestMergedResolveMerge`,
  `TestNLAnswerCarriesMerge`)

#### Scenario: No merge answer

- **WHEN** the merged search reply has no `merge`, or the lens picks symbol mode
- **THEN** the response has no `merge` key (`TestMergedResolveMerge`, `TestNLAnswerCarriesMerge`)
