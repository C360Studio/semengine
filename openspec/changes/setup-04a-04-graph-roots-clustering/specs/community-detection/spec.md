# community-detection

## ADDED Requirements

### Requirement: Detection partitions entities over explicit edges only

`clustering.LPADetector.DetectCommunities(ctx)` SHALL assign every entity the provider lists to exactly one community
using only the edges and weights its `graph.Provider` returns, SHALL synthesize no edge from entity-ID positions or
embedding similarity, and SHALL write one level, level 0, returned as a map with the single key 0.

#### Scenario: Two disjoint groups

- **WHEN** the provider serves two cliques with no edge between them
- **THEN** detection returns two level-0 communities, each holding one clique, and no community at any other level
- **AND** the tests that verify this are `TestDetectDisjointCliques` and `TestOneLevel`

#### Scenario: Entities sharing an ID prefix but no edge

- **WHEN** two entities share every entity-ID position but the type instance, and the provider returns no edge
  between them
- **THEN** each is in its own community
- **AND** the test that verifies this is `TestNoVirtualEdges`

### Requirement: Detection refuses missing inputs

`DetectCommunities` SHALL return a typed error and write nothing when its context, provider or storage is nil.

#### Scenario: Nil context

- **WHEN** `DetectCommunities` is called with a nil context
- **THEN** it returns an error and calls neither the provider nor the storage
- **AND** the test that verifies this is `TestDetectRefusesNilContext`

### Requirement: Detection is reproducible given a fixed edge set

Repeated detection over an unchanged entity and edge set SHALL produce the same partition regardless of the order
in which the provider lists entities or neighbours: the shuffle is seeded per run and a vote tie goes to the
lexicographically smallest label.

#### Scenario: Shuffled provider order

- **WHEN** detection runs twice over the same seeded random graph, the second time with the provider's entity and
  neighbour order shuffled
- **THEN** both runs return identical communities with identical members
- **AND** the test that verifies this is `TestDetectionMatchesModel`, which records its seed and run count

### Requirement: The community index is rebuilt non-destructively

A detection run SHALL save every community before it removes anything, SHALL call `Prune` once with the complete
partition, and SHALL NOT clear the store first. Any save failure SHALL make the run return an error without calling
`Prune`. A `Prune` failure SHALL be logged and SHALL NOT fail the run.

#### Scenario: A save fails

- **WHEN** one community's save fails
- **THEN** `DetectCommunities` returns an error, `Prune` is not called, and no prior key is deleted
- **AND** the test that verifies this is `TestSaveFailureDoesNotPrune`

#### Scenario: Prune fails

- **WHEN** every save succeeds and `Prune` fails
- **THEN** `DetectCommunities` returns the partition with no error
- **AND** the test that verifies this is `TestPruneFailureIsNotFatal`

### Requirement: Storage fails closed

`NATSCommunityStorage.GetAllCommunities` SHALL return an error naming the key when an entry cannot be read or
decoded, and every storage method SHALL return the context's error when its context ends, never a nil error with a
partial result. An empty bucket SHALL return no communities and no error.

#### Scenario: A corrupt entry

- **WHEN** `COMMUNITY_INDEX` holds a community key whose value is not a community record
- **THEN** `GetAllCommunities` returns an error naming that key
- **AND** the test that verifies this is `TestGetAllCommunitiesFailsOnCorruptEntry`

#### Scenario: Cancelled during prune

- **WHEN** the context ends while `Prune` lists or deletes keys
- **THEN** `Prune` returns the context's error
- **AND** the test that verifies this is `TestPruneReportsCancel`

### Requirement: A community summary is a function of its members

`StatisticalSummarizer.SummarizeCommunity` SHALL return the same keywords, representative entities and summary text,
in the same order, for the same member entities, breaking every score tie by term or entity ID.

#### Scenario: Tied scores

- **WHEN** a community's entities give two terms the same score and the summary is computed 200 times
- **THEN** every result is identical
- **AND** the test that verifies this is `TestSummaryDeterministic`
