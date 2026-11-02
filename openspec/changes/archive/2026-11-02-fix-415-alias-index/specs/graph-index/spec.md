# graph-index

## MODIFIED Requirements

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

## ADDED Requirements

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
