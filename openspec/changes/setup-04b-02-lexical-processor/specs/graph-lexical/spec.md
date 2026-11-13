# graph-lexical Delta

## ADDED Requirements

### Requirement: The index follows ENTITY_STATES and is rebuilt on every start

graph-lexical SHALL follow `ENTITY_STATES` through `graph/follower` under the readiness key `graph-lexical`, apply
each delivered entity to its in-memory `lexical.Index` in delivery order, and ignore a delivery at or below the
revision it last applied for that key. It SHALL report a revision complete only after the index holds it. After a
start it SHALL hold exactly the live entities with text, whatever was written or deleted before.

#### Scenario: A restart rebuilds the index

- **WHEN** entities are written, one is edited and one deleted, and the component is stopped and started again
- **THEN** a search answers only the live entities whose current words match, with the same hits, order and scores
  as before the restart (`TestRestartRebuildsSameAnswers`)

#### Scenario: An older delivery after a newer one

- **WHEN** a key's revision R is applied and a delivery of the same key at a revision at or below R arrives
- **THEN** the index keeps R's words (`TestOlderDeliveryIgnored`)

#### Scenario: A deleted entity

- **WHEN** an entity is deleted and its tombstone's revision is indexed
- **THEN** no search returns it (`TestDeleteRemovesEntity`)

### Requirement: The search verb answers only once the index has caught up

`graph.lexical.query.search` SHALL validate the request first (strict decode, limit, scope prefixes, query), then
refuse transient `index_not_ready` before the initial build is applied or while the feed is lost, fatal
`graph_state_reset_required` after a latched reset, and only then compare `min_revision`: above the revision the
reply would carry it SHALL refuse transient `revision_not_reached` with that revision as detail.

#### Scenario: During the replay and after

- **WHEN** a request arrives before the initial build is applied, and again after
- **THEN** the first is refused transient `index_not_ready` and the second is answered
  (`TestSearchWithheldUntilBuilt`)

#### Scenario: A caller waits for its own write

- **WHEN** a follower at revision R gets `min_revision` R, then R+1
- **THEN** the first is answered with `indexed_revision` at or above R and the second refused `revision_not_reached`
  with detail R (`TestSearchMinRevisionBoundary`)

### Requirement: A search reply is scoped, limited, ranked and counted

A reply SHALL be a `graph.QueryResponse` from producer `graph-lexical` whose `indexed_revision` is a lower bound on
what it reflects, holding `graph.LexicalSearchData`: `results`, at most `limit` hits ordered by score then entity
ID, each `entity_id` and `score`; `matched`, every match under the scope before the limit; and `unread_bodies`. A
scope SHALL hold entity-ID prefixes, an empty list meaning every entity.

#### Scenario: The limit's range

- **WHEN** `limit` is 0, 1 to 100, or any other value
- **THEN** 0 means 10, 1 to 100 is used as given, and any other value is refused `invalid_request`
  (`TestSearchLimitRange`)

#### Scenario: Better matches outside the scope

- **WHEN** a scoped search with limit 1 runs while better matches lie outside the scope
- **THEN** the hit is the best inside the scope and `matched` counts every match inside it
  (`TestSearchScopeLimitMatched`)

#### Scenario: A refused query

- **WHEN** `lexical.ParseQuery` refuses the query, here `pump*`
- **THEN** it is refused `invalid_request`, the message naming the rule, here reserved syntax
  (`TestSearchRefusesParseErrors`)

### Requirement: An offloaded body is read with a bound, and an unread one is declared

For an entity whose `StorageRef` names a registered store, graph-lexical SHALL index the body read through
`graph/entitytext` after its inline text. A body not read (no store registered, a read error, or the read's
deadline) SHALL leave the entity indexed on its inline text, counted in `unread_bodies` and in the readiness
envelope under `body_unread`, without withholding the verb, and SHALL be read again on a later repair pass until it
reads or the entity changes.

#### Scenario: A store that fails, then recovers

- **WHEN** an entity's body cannot be read at start and its store later serves it
- **THEN** a search for a body-only word misses it with `unread_bodies` 1, and finds it with `unread_bodies` 0 after
  the next repair pass (`TestUnreadBodyCountedThenRepaired`)

#### Scenario: A store that blocks until its context ends

- **WHEN** a store's read blocks until its context ends
- **THEN** the initial build still completes and the entity is counted in `unread_bodies` (`TestHungStoreBounded`)

### Requirement: Configuration is strict and tier 0

graph-lexical SHALL refuse at construction an unknown config key, naming it, SHALL take `text_suffixes` and
`max_text_len` with graph-embedding's meaning (`graph/entitytext`), and SHALL import no model or provider module.

#### Scenario: Unknown key and imports

- **WHEN** the config holds `workers`
- **THEN** construction fails naming `workers` (`TestConfigRefusesUnknownKey`), and
  `TestGraphLexicalImportsNoProviderModule` passes

### Requirement: The index's size is visible

graph-lexical SHALL report the number of entities and of words its index holds as gauges on the injected metrics
registry.

#### Scenario: After writes and a delete

- **WHEN** three entities are written and one deleted
- **THEN** the entity gauge reads 2 and the word gauge the words the two hold (`TestIndexGauges`)
