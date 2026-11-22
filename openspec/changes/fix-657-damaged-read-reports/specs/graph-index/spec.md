# graph-index Delta

## MODIFIED Requirements

### Requirement: Malformed index rows are skipped and declared

A query that lists a stored incoming, alias, predicate or name index row that is not its index's layout SHALL leave
that row out, answer from the well-formed rows, log a warning naming the index, count the row in
`semengine_graph_index_query_rows_skipped_total` under that index's `index_type` label, and count it in the reply's
`data.unreadable_records`. Every reply of incoming, alias, predicate, predicateList, predicateStats, predicateCompound
and byName SHALL carry `data.unreadable_records`, 0 when none was skipped. A row deleted after the listing SHALL NOT be
counted.

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

### Requirement: A relationship read of a damaged entity is refused

`outgoing` for a damaged entity SHALL be refused fatal `graph_state_reset_required` with the refusal the follower
recorded, as graph-ingest's entity read refuses it, with `detail.entity_ids` holding that entity. `incoming` for it
SHALL answer, since its rows are the other entities' facts.

#### Scenario: Outgoing and incoming of B

- **WHEN** B is damaged and A holds a relationship to B
- **THEN** `outgoing` on B is refused fatal `graph_state_reset_required` with `entity_ids` [B], and `incoming` on B
  answers A's edge (`TestDamagedEntityLeftOutAndNamed`)

## ADDED Requirements

### Requirement: The value filter refuses a damaged entity it reads

When `predicate` with a `value` reads a stored entity value `graph.UnmarshalEntityState` refuses, graph-index SHALL
refuse the request fatal `graph_state_reset_required` with `detail.entity_ids` naming each damaged entity it read,
sorted. Any other read failure SHALL stay class transient, code `internal`. An entity graph-index has already recorded
damaged has no membership row, so the filter does not read it, and the reply counts it in `damaged_entities`.

#### Scenario: A membership row for a damaged entity

- **WHEN** B is recorded damaged (a reply counts `damaged_entities` 1) and a `PREDICATE_INDEX` membership row for B
  under predicate P is then stored
- **THEN** `predicate` P with a value is refused fatal `graph_state_reset_required` with `entity_ids` [B]
