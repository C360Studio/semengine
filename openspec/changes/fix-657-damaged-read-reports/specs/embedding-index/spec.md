# embedding-index Delta

## MODIFIED Requirements

### Requirement: The vector cache scores and counts one moment

`VectorCache.FindSimilar` SHALL score only the generated vectors whose entity IDs `keep` accepts, sort by
`ScoredEntityLess` before applying the limit, and return no entities for a limit of 0 or less. It SHALL return, read at
the same moment as the vectors it scored, the number of records it could not decode whose keys `keep` accepts (every
such record when `keep` is nil). Such a record SHALL NOT be scored and SHALL NOT withdraw the cache; a later valid write
of that key SHALL be scored and no longer counted.

#### Scenario: An unreadable record heals

- **WHEN** one record fails to decode, and later a valid generated record is written under the same key
- **THEN** the first `FindSimilar` scores the other vectors and counts 1, and after the write it scores that key and
  counts 0
- **AND** the test that verifies this is `TestVectorCachePoisonCountedAndHeals`

#### Scenario: Order and limit

- **WHEN** several cached vectors tie on similarity across the limit, and when the limit is 0 or negative
- **THEN** the result is ordered by similarity then entity ID, and a non-positive limit returns no entities
- **AND** the test that verifies this is `TestFindSimilarOrderAndLimit`

#### Scenario: One moment

- **WHEN** a delete and an undecodable write are applied after `FindSimilar` read the view and before it returns
- **THEN** its answer and count do not include them, and the next call does
- **AND** the test that verifies this is `TestFindSimilarReadsOneMoment`

#### Scenario: Only generated vectors under `keep`

- **WHEN** `keep` accepts some entity IDs and a pending or failed record exists under an accepted ID
- **THEN** only the generated vectors of accepted IDs are scored, and an accepted lower score beats a rejected higher
  one at limit 1
- **AND** the test that verifies this is `TestFindSimilarFromCache_ScopeFilter`

#### Scenario: The count follows keep

- **WHEN** two records fail to decode, one under a key `keep` accepts and one outside it
- **THEN** `FindSimilar` with that `keep` counts 1, and with a nil `keep` counts 2
- **AND** the test that verifies this is `TestFindSimilarCountFollowsKeep`
