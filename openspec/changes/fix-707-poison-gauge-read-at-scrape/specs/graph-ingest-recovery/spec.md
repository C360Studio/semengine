# graph-ingest-recovery Delta

## MODIFIED Requirements

### Requirement: graph-ingest reports its poisoned entities

While any entry is recorded, `Health()` SHALL name the count and the first 10 IDs in sorted order with their reasons,
the gauge `semengine_graph_ingest_poisoned_entities` SHALL equal the count, and the debug status SHALL list every
entry in sorted order. A new entry SHALL invalidate that entity's query-cache entry.

#### Scenario: Eleven poisoned entities

- **WHEN** 11 entities are recorded
- **THEN** Health names 11, the first 10 IDs in sorted order with reasons and 1 more, and the debug status lists all
  11 in sorted order (`TestHealthNamesTenPoisonedEntities`)

#### Scenario: Detection invalidates the cached entry

- **WHEN** an entity is cached and its stored value is then poisoned outside graph-ingest and detected
- **THEN** the new entry deletes the entity's query-cache entry (`TestDetectionInvalidatesCachedEntry`)

#### Scenario: Concurrent records and clears

- **WHEN** reports and clears of distinct keys run concurrently and all have returned
- **THEN** the gauge equals the number of keys left recorded: the record's count is exact after concurrent changes
  (`TestConcurrentReportsAndClears`) and the gauge reads that count (`TestPoisonGaugeReadsTheRecord`)

#### Scenario: The gauge reads the record

- **WHEN** entries are reported to and cleared from the record directly, with no gauge call
- **THEN** after each change the scraped gauge equals the number of entries recorded: 1, 2, 1, 0
  (`TestPoisonGaugeReadsTheRecord`)
