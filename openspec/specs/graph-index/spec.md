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
applied or while index writes it requires are unresolved, and SHALL answer class fatal, code
`graph_state_reset_required`, once reset is latched. Once the build is applied and no write is unresolved, the same
request SHALL succeed, whatever the lag.

#### Scenario: A request during the replay

- **WHEN** a request on any of the eight verbs arrives before the initial build is applied
- **THEN** it fails with class transient and code `index_not_ready`, and the same request succeeds after the build
- **AND** the test that verifies this is `TestQueryDuringStartupReplayIsIndexNotReady`

### Requirement: Health reads named stuck states only

graph-index's `Healthy` SHALL be false exactly while it is not running or a stuck state holds: reset latched, a
required index write unresolved, or the feed lost. `ErrorCount` SHALL never lower it.

#### Scenario: A failed write and its repair

- **WHEN** an entity's required writes fail and are later repaired, and errors were counted
- **THEN** `Healthy` is false while the failure holds and true after the repair
- **AND** the test that verifies this is `TestHealthIgnoresErrorCountAndTracksStuckStates`

### Requirement: DataFlow counts deliveries

graph-index's `DataFlow` SHALL count one input per delivered `ENTITY_STATES` entry, update or delete, with the
delivered value's byte length, counted before coalescing, and SHALL not count index writes. The apply SHALL still read
the key's current authoritative state.

#### Scenario: One entity with three relationships

- **WHEN** one entity with three relationships is written once
- **THEN** `DataFlow().Messages` rises by one

#### Scenario: Coalesced updates are each counted

- **GIVEN** `coalesce_ms` above zero
- **WHEN** graph-index receives three delivered updates of one entity, with values of different sizes, inside one
  coalescing window
- **THEN** `DataFlow().Messages` rises by three and `DataFlow().Bytes` by the sum of the three delivered values' lengths
- **AND** the test that verifies this is `TestDataFlowCountsEachDeliveryBeforeCoalescing`

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

A query that lists a stored incoming, alias, predicate or name index row that is not its index's layout SHALL leave
that row out, answer from the well-formed rows, log a warning naming the index, and count the row in
`semengine_graph_index_query_rows_skipped_total` under that index's `index_type` label. A row deleted after the
listing SHALL NOT be counted.

#### Scenario: Malformed rows in three index buckets

- **WHEN** a malformed key is stored in `INCOMING_INDEX`, `PREDICATE_INDEX` and `NAME_INDEX`, and a name row holds a
  value that is not JSON
- **THEN** incoming, predicate, predicateList and byName answer only the well-formed rows, a warning names each index,
  and the counter reads incoming 1, predicate 2 (listed by predicate and by predicateList) and name 2
- **AND** the test that verifies this is `TestMalformedIndexRowsAreSkippedLoggedAndCounted`

#### Scenario: A malformed alias row

- **WHEN** a key under an alias's hash prefix in `ALIAS_INDEX` is not `hash.entity6.hex(predicate)`
- **THEN** the `alias` read answers from the well-formed rows, a warning names the alias index, and the counter reads
  alias 1
- **AND** the test that verifies this is `TestMalformedIndexRowsAreSkippedLoggedAndCounted`

### Requirement: Queries are refused while the feed is lost

While graph-index's follower reports its `ENTITY_STATES` feed lost, each of the eight verbs SHALL answer class
transient, code `index_not_ready`; `Healthy` SHALL be false and the envelope `degraded` until the process restarts.

#### Scenario: The bucket is deleted under a running graph-index

- **WHEN** `ENTITY_STATES` is deleted while graph-index runs
- **THEN** within two activity checks every verb answers `index_not_ready` and `Healthy` is false
- **AND** the test that verifies this is `TestFollowerDeclaresLostFeed`

### Requirement: Stop writes accepted work

Index work the keyed pool accepted before `Stop` SHALL be written before `Stop` returns nil, without counting an error.

#### Scenario: Accepted work at Stop

- **WHEN** `Stop` is called with entries accepted by the index lanes
- **THEN** each entry's index rows are stored when `Stop` returns nil, and `ErrorCount` is unchanged
- **AND** the test that verifies this is `TestStopWritesAcceptedWork`

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
