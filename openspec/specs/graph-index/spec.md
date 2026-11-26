# graph-index Specification

## Purpose
graph-index (`processor/graph-index`) keeps the outgoing, incoming, alias, predicate and name indexes of the graph in
step with `ENTITY_STATES`, and answers the eight `graph.index.query.*` verbs, refusing while it is not ready (archived
change `2026-10-27-setup-04a-04-graph-roots-graph-index`).

## Requirements

### Requirement: Configuration is strict

graph-index's factory SHALL refuse an unknown configuration key, naming it. The keys SHALL be `ports`, `workers`
(0 means the default, 1; otherwise 1 to 16), `coalesce_ms`, `startup_attempts` and `startup_interval_ms`; each SHALL
change behaviour.
A configuration with no `ports` key SHALL take the default ports; a `ports` block that is present SHALL be used as
written, never merged with the defaults (ruling K, #91 comment 6062681355, applied to graph-index by PR #414 comment
6431777824).

#### Scenario: A configuration carrying batch_size

- **WHEN** graph-index is constructed from a configuration with the key `batch_size`
- **THEN** construction fails with an error naming `batch_size`
- **AND** the test that verifies this is `TestUnknownConfigKeyIsRefused`

#### Scenario: No ports key

- **WHEN** graph-index is constructed from a configuration with no `ports` key
- **THEN** construction succeeds with the `entity_watch` input and the four index outputs
- **AND** the test that verifies this is `TestConfigWithoutPortsTakesDefaultPorts`

#### Scenario: A ports block with an empty inputs list

- **WHEN** graph-index is constructed from a `ports` block whose `inputs` list is empty
- **THEN** construction fails with an invalid-configuration error naming the missing input port, and the default input
  is not filled in
- **AND** the test that verifies this is `TestConfigPresentPortsUsedAsWritten`

#### Scenario: workers 0

- **WHEN** graph-index is constructed with `"workers": 0`
- **THEN** construction succeeds and the index runs one lane

### Requirement: Inputs name the one source graph-index reads

Every input port of graph-index SHALL be a KV watch of every `ENTITY_STATES` key with no history, the one source it
reads. Construction and port declaration SHALL refuse any other input with an invalid-configuration error naming the
port (#424).

#### Scenario: An input graph-index does not read

- **WHEN** graph-index is constructed, or its ports declared, with an input that watches another bucket, filters keys,
  asks for history, or is not a KV watch
- **THEN** both fail with an invalid-configuration error naming the input port
- **AND** the test that verifies this is `TestConfigRefusesInputsTheFollowerDoesNotRead`

### Requirement: Startup-replay queries get a retryable refusal

Each of graph-index's eight verbs SHALL answer class transient, code `index_not_ready`, while its initial build is not
applied or while index writes it requires are unresolved. A stored damaged entity SHALL NOT refuse a verb, except
`outgoing` for that entity. Once the build is applied and no write is unresolved, the same request SHALL succeed,
whatever the lag.

#### Scenario: A request during the replay

- **WHEN** a request on any of the eight verbs arrives before the initial build is applied
- **THEN** it fails with class transient and code `index_not_ready`, and the same request succeeds after the build
- **AND** the test that verifies this is `TestQueryDuringStartupReplayIsIndexNotReady`

### Requirement: Health reads named stuck states only

graph-index's `Healthy` SHALL be false exactly while it is not running or a stuck state holds: a damaged entity
stored, a required index write unresolved, or the feed lost. `ErrorCount` SHALL never lower it.

#### Scenario: A failed write and its repair

- **WHEN** an entity's required writes fail and are later repaired, and errors were counted
- **THEN** `Healthy` is false while the failure holds and true after the repair
- **AND** the test that verifies this is `TestHealthIgnoresErrorCountAndTracksStuckStates`

### Requirement: DataFlow counts entity reads

graph-index's follower SHALL watch `ENTITY_STATES` keys only (`follower.Config` keys-only field), and graph-index's
`DataFlow` SHALL count one input per entity it reads from `ENTITY_STATES` to apply it, with the byte length of the
value it read, and SHALL not count index writes. Writes the coalescer folds into one apply SHALL count as one read; a
delete, whose read finds no entity, SHALL count none. `watch_events_total` SHALL still count each delivered entry by
kind.

#### Scenario: One entity with three relationships

- **WHEN** one entity with three relationships is written once
- **THEN** `DataFlow().Messages` rises by one

#### Scenario: Coalesced updates are each counted

- **GIVEN** `coalesce_ms` above zero
- **WHEN** one entity is written three times with values of different sizes, each write read before the next
- **THEN** `DataFlow().Messages` rises by three and `DataFlow().Bytes` by the sum of the three values' lengths
- **AND** the test that verifies this is `TestDataFlowCountsEachEntityRead`

#### Scenario: A replay counts the stored bytes

- **WHEN** graph-index starts over stored entities, one written twice, and the replay is applied
- **THEN** `DataFlow().Messages` is the number of stored entities and `DataFlow().Bytes` the summed lengths of the
  values stored last
- **AND** the test that verifies this is `TestDataFlowCountsReplayedBytes`

### Requirement: One Start per instance

A second `Start` on a started graph-index SHALL return `errs.ErrAlreadyStarted` and change nothing; ending the context
given to `Start` after it returned SHALL stop nothing.

#### Scenario: Start twice

- **WHEN** `Start` is called on a running graph-index
- **THEN** it returns `errs.ErrAlreadyStarted` and the component keeps running
- **AND** the test that verifies this is `TestSecondStartIsRefused`

### Requirement: Indexes implement replacement

For outgoing, incoming, alias, predicate (exact, value, list, stats, compound) and name reads, a change of an entity's
statements from A to B and then to none SHALL leave each read answering B and then nothing for that entity, and
deleting the entity SHALL leave each read answering nothing for it (ADR-114).

#### Scenario: A then B then none

- **WHEN** an entity's statements go from A to B to none
- **THEN** each read listed answers A, then B, then nothing for that entity
- **AND** the tests that verify this are `TestQueryReadsImplementReplacement` and `TestAliasReadImplementsReplacement`

#### Scenario: A deleted entity's alias

- **WHEN** an entity carrying alias X is deleted and graph-index has indexed the delete
- **THEN** an `alias` request for X answers no ID, and every other read answers nothing for the entity
- **AND** the tests that verify this are `TestAliasReadImplementsReplacement` and
  `TestIndexReplacementThroughEveryIndex`

#### Scenario: The longest alias key

- **WHEN** an entity with a 256-byte ID carries an alias under a 194-byte alias predicate, and is then deleted
- **THEN** an `alias` request answers that entity, and after the delete answers no ID
- **AND** the test that verifies this is `TestMaximumLengthIDsAndPredicatesThroughPublicReads`

#### Scenario: One holder of a shared alias is deleted

- **WHEN** entities E1 and E2 both carry alias X and E2 is deleted
- **THEN** an `alias` request for X answers E1
- **AND** the test that verifies this is `TestSharedAliasIsRefusedAsAmbiguous`

### Requirement: Malformed index rows are skipped and declared

A query that lists a stored incoming, alias, predicate or name index row that is not its index's layout SHALL leave that
row out, answer from the well-formed rows, log a warning naming the index, count the row in
`semengine_graph_index_query_rows_skipped_total` under that index's `index_type` label, and count it in the reply's
`data.unreadable_records`, which every row-listing verb's reply carries, 0 when none was skipped. A row deleted after
the listing SHALL NOT be counted.

#### Scenario: Malformed rows in three index buckets

- **WHEN** a malformed key is stored in `INCOMING_INDEX`, `PREDICATE_INDEX` and `NAME_INDEX`, and a name row holds a
  value that is not JSON
- **THEN** incoming, predicate, predicateList and byName answer only the well-formed rows, a warning names each index,
  the counter reads incoming 1, predicate 2 (listed by predicate and by predicateList) and name 2, and the replies'
  `data.unreadable_records` are incoming 1, predicate 1, predicateList 1 and byName 2
- **AND** the test that verifies this is `TestMalformedIndexRowsAreSkippedLoggedAndCounted`

#### Scenario: A malformed alias row

- **WHEN** a key under an alias's hash prefix in `ALIAS_INDEX` is not `hash.entity6.hex(predicate)`
- **THEN** the `alias` read answers from the well-formed rows with `data.unreadable_records` 1, a warning names the
  alias index, and the counter reads alias 1
- **AND** the test that verifies this is `TestMalformedIndexRowsAreSkippedLoggedAndCounted`

#### Scenario: Every row-listing verb carries the count

- **WHEN** incoming, alias, predicate, predicateList, predicateStats, predicateCompound and byName each answer a request
  that skipped no row
- **THEN** each reply's `data.unreadable_records` is 0
- **AND** the test that verifies this is `TestMalformedIndexRowsAreSkippedLoggedAndCounted`

### Requirement: Queries are refused while the feed is lost

While graph-index's follower reports its `ENTITY_STATES` feed lost, each of the eight verbs SHALL answer class
transient, code `index_not_ready`; `Healthy` SHALL be false and the envelope `degraded` until the process restarts.

#### Scenario: The bucket is deleted under a running graph-index

- **WHEN** `ENTITY_STATES` is deleted while graph-index runs
- **THEN** within two activity checks every verb answers `index_not_ready` and `Healthy` is false
- **AND** the test that verifies this is `TestFollowerDeclaresLostFeed`

### Requirement: Stop writes accepted work

Index work a lane has started when `Stop` begins SHALL be written whole before `Stop` returns nil. Index work the
keyed pool accepted but no lane has started SHALL be dropped: no read, no write, no failure mark, no revision
completed, and no log line, from the moment `Stop` begins, even while it waits on query handlers. The next start's
replay re-derives what was dropped. `Stop` waits on the lanes for at most one entity's writes per lane.

#### Scenario: Accepted work at Stop

- **WHEN** `Stop` is called while one entity is being written and later entries wait in the lane's queue
- **THEN** the entity in flight has its index rows stored when `Stop` returns nil, the queued entries have none, no
  entity is counted failed, `ErrorCount` is unchanged, no revision past the one in flight is completed, and no
  warning is logged
- **AND** the tests that verify this are `TestStopDropsQueuedWork` and, with `Stop` held in a query drain,
  `TestStopDropsQueuedWorkFromItsStart`

### Requirement: Every declared alias predicate is indexed

graph-index SHALL index every string value of every predicate `vocabulary.DiscoverAliasPredicates` returns when
graph-index starts, `core.identity.alias` among them, whatever bytes the value holds; a value empty after trimming is
not an alias (ADR-114).

#### Scenario: The framework alias predicate is declared

- **WHEN** `vocabulary.DiscoverAliasPredicates` is called with the built-in registrations in place
- **THEN** it contains `core.identity.alias`
- **AND** the test that verifies this is `TestCoreIdentityAliasIsDeclared`

#### Scenario: An alias that is not a KV key

- **WHEN** an entity carries alias `two words`
- **THEN** an `alias` request for `two words` answers that entity, and the entity's other rows are written
- **AND** the test that verifies this is `TestKVUnsafeAliasIsIndexed`

### Requirement: An alias resolves to its one current holder

An `alias` request SHALL match an alias trimmed and case-folded as NAME matches a name, and SHALL answer no ID when no
live entity carries it, that entity's ID when exactly one does (under any number of predicates), and a refusal of
class invalid with code `alias_ambiguous` naming the alias when two or more do (ADR-114). An alias empty after
trimming SHALL be refused with code `invalid_request`; unlike a name, a whitespace-only alias is refused.

#### Scenario: A shared alias is refused, then resolves

- **WHEN** entities E1 and E2 both carry alias X, and later E2 drops X
- **THEN** an `alias` request for X is first refused with class invalid, code `alias_ambiguous` and a message
  containing X, and after graph-index indexes E2's change answers E1
- **AND** the test that verifies this is `TestSharedAliasIsRefusedAsAmbiguous`

#### Scenario: Case and space fold

- **WHEN** an entity carries alias `SN-1` and the request names ` sn-1 `
- **THEN** the reply answers that entity
- **AND** the test that verifies this is `TestAliasMatchFoldsCase`

### Requirement: An unmet `min_revision` is refused

Each of graph-index's eight verbs SHALL accept `min_revision` (`graph.MinRevisionField`; zero means none) and, before
its first bucket read, compare it with the revision the reply would carry. At or below that revision the verb SHALL
answer as usual. Above it, the verb SHALL refuse with class transient, code `revision_not_reached`, and detail
`indexed_revision` naming that revision. No reply SHALL wait for the revision.

#### Scenario: The boundary

- **WHEN** a follower at revision R receives a request on any of the eight verbs with `min_revision` R, then with R+1
- **THEN** the first is answered with `indexed_revision` at or above R
- **AND** the second is refused with `revision_not_reached` and detail R (`TestMinRevisionBoundary`)

#### Scenario: A strict decoder accepts the field

- **WHEN** a `predicateList` request carries `min_revision`
- **THEN** it is not refused as an unknown field (`TestMinRevisionBoundary`)

### Requirement: `index_not_ready` keeps its meaning

A request SHALL be validated first, then refused `index_not_ready` if the index is not ready, and only then compared
with `min_revision`. An unmet `min_revision` SHALL NOT be reported as `index_not_ready` (ADR-109 Decision 4).

#### Scenario: Not ready and behind

- **WHEN** a request carries a `min_revision` above the follower's revision while the follower withholds readiness
- **THEN** it is refused `index_not_ready`, not `revision_not_reached` (`TestMinRevisionBoundary`)

### Requirement: An entity that cannot be planned is held as failed

When graph-index cannot build an entity's index rows from a stored value it accepted, it SHALL mark the entity failed,
counting it once in `ErrorCount` and in `semengine_graph_index_write_failures_total`, and hold readiness until the
entity is rewritten or deleted. The revision still completes, as after a failed write (ADR-066 §1); the failed set,
not a pending revision, withholds readiness. Every value `graph.UnmarshalEntityState` accepts SHALL be plannable.

#### Scenario: Accepted values plan

- **WHEN** generated entity states at the byte bounds of entity IDs, predicates, aliases and names pass
  `graph.UnmarshalEntityState`
- **THEN** each builds an index plan, with the seed and the completed checks recorded
  (`TestContractValidStateAlwaysPlans`)

### Requirement: A damaged entity is left out and counted

When the stored value graph-index reads for an entity is refused by `graph.UnmarshalEntityState`, or its key is not a
literal entity ID, graph-index SHALL report it to the follower, remove every row it built from the entity's earlier
value, and complete the revision. Every reply SHALL carry `damaged_entities`, the number of damaged entities recorded
when it answered. A valid value or a delete for the key SHALL be applied as for any entity, with no restart.

#### Scenario: Damage stored at start

- **WHEN** A is valid with a name, an alias and a relationship, and B is stored damaged before graph-index starts
- **THEN** after the first build `byName`, `alias` and `outgoing` answer A with `damaged_entities` 1, and `Health`
  names B (`TestDamagedEntityLeftOutAndNamed`)

#### Scenario: A valid entity overwritten with damage

- **WHEN** B is indexed and then overwritten with bytes that do not decode
- **THEN** B's name, alias and relationship rows are gone, the replies count 1, and the revision completes
  (`TestDamagedEntityLeftOutAndNamed`)

#### Scenario: Repair without restart

- **WHEN** B is then written with a valid value
- **THEN** B's rows answer, replies carry no `damaged_entities`, and `Healthy` is true (`TestRepairedEntityReturns`)

### Requirement: A relationship read of a damaged entity is refused

`outgoing` for a damaged entity SHALL be refused fatal `graph_state_reset_required` with the refusal the follower
recorded, as graph-ingest's entity read refuses it, with `detail.entity_ids` holding that entity. `incoming` for it
SHALL answer, since its rows are the other entities' facts.

#### Scenario: Outgoing and incoming of B

- **WHEN** B is damaged and A holds a relationship to B
- **THEN** `outgoing` on B is refused fatal `graph_state_reset_required` with `entity_ids` [B], and `incoming` on B
  answers A's edge (`TestDamagedEntityLeftOutAndNamed`)

### Requirement: The value filter refuses a damaged entity it reads

When `predicate` with a `value` reads a stored entity value `graph.UnmarshalEntityState` refuses, graph-index SHALL
refuse the request fatal `graph_state_reset_required` with `detail.entity_ids` naming each damaged entity it read,
sorted. Any other read failure SHALL stay class transient, code `internal`. An entity graph-index has already recorded
damaged has no membership row, so the filter does not read it, and the reply counts it in `damaged_entities`.

#### Scenario: A membership row for a damaged entity

- **WHEN** B is recorded damaged (a reply counts `damaged_entities` 1) and a `PREDICATE_INDEX` membership row for B
  under predicate P is then stored
- **THEN** `predicate` P with a value is refused fatal `graph_state_reset_required` with `entity_ids` [B]
- **AND** the test that verifies this is `TestValueFilterRefusesDamagedEntityItReads`
