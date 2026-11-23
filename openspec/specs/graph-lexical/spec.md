# graph-lexical Specification

## Purpose
The `processor/graph-lexical` component keeps tier 0's word index (`graph/lexical`) current. It follows
`ENTITY_STATES`, rebuilds the index in memory on every start, reads bodies kept in an object store on one path the
follower never waits for, and answers `graph.lexical.query.search` once the rebuild has caught up. This capability
states how deliveries and body reads are applied, when a revision counts as indexed, how unread bodies are reported
by cause, and the shape of a search reply.

## Requirements

### Requirement: The index follows ENTITY_STATES and is rebuilt on every start

graph-lexical SHALL follow `ENTITY_STATES` through `graph/follower` under the readiness key `graph-lexical`, apply
each delivered entity's inline text to its in-memory `lexical.Index` in delivery order, and ignore a delivery at or
below the revision the index holds for that key. After a start it SHALL hold exactly the live entities with text,
whatever was written or deleted before.

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

### Requirement: A revision completes only once its text is in the index

graph-lexical SHALL report a revision complete only after the index holds it and, for an entity with an offloaded
body, after that body is indexed or has reached a state below. A body read SHALL be indexed only if, when it
returns, the key's latest revision still names the reference it read and that body still awaits a read (queued,
unread or no store); it SHALL be joined to that revision's inline text. Otherwise it SHALL be dropped, and the
revision it was read for SHALL be complete.

#### Scenario: A body read that returns after a newer delivery

- **WHEN** a body read for revision R is in flight and a delivery of the same key at R+1, naming a different
  reference, is applied before it returns
- **THEN** the index holds R+1's text and never R's body, and the follower's floor passes R
  (`TestStaleBodyReadDropped`)

#### Scenario: Any history of deliveries, deletes and body reads

- **WHEN** a generated history interleaves deliveries that keep or change a key's reference, deletes, body outcomes
  and late read returns over a few keys
- **THEN** after each step every live key holds its latest revision's inline text, plus the body of that
  revision's reference only if a read of that reference succeeded since the key last named another reference or
  changed its inline text, as a reference model says, and every delivered revision has completed once its outcome
  is in (`TestPropApplyMatchesModel`, seed and checks recorded)

### Requirement: A body is read again only when its reference or inline text changes

A revision with the `StorageRef` and inline text the index holds SHALL change nothing and ask for no read; one keeping
a reference whose read a delivery asked for is still out SHALL ask for none of its own (a repair pass's read is not
counted). References are equal only as a whole `StorageReference` (instance, key, content type, size). A reference
SHALL name fixed content: content replaced behind one is read again only when the reference or inline text
changes, or at start.

#### Scenario: Non-text values change and the reference stays

- **WHEN** an entity's offloaded body is indexed and revisions follow that change only values that are not text,
  keeping the same reference, some arriving while the first read is still out
- **THEN** a search for a body-only word finds the entity after each of them once the first read has returned, no
  further body read is asked for, and each revision completes (`TestPropApplyMatchesModel`)

### Requirement: The search verb answers only once the index has caught up

`graph.lexical.query.search` SHALL validate the request first (strict decode, limit, scope prefixes, query), then
refuse transient `index_not_ready` before the initial build is applied or while the feed is lost, and only then
compare `min_revision`: above the revision the reply would carry it SHALL refuse transient `revision_not_reached` with
that revision as detail. A stored damaged entity SHALL NOT refuse a search.

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
what it reflects, holding `graph.LexicalSearchData`: `results`, at most `limit` hits by score then entity ID, each
`entity_id` and `score`; `matched`, every match in scope before the limit; `excluded`, the excluded words, omitted
when none; and `bodies_not_indexed`, how many entities in the whole index have an offloaded body not yet indexed.

#### Scenario: The limit's range

- **WHEN** `limit` is 0, 1 to 100, or any other value
- **THEN** 0 means 10, 1 to 100 is used as given, and any other value is refused `invalid_request`
  (`TestSearchLimitRange`)

#### Scenario: A refused query

- **WHEN** `lexical.ParseQuery` refuses the query, here `pump*`
- **THEN** it is refused `invalid_request`, the message naming the rule, here reserved syntax
  (`TestSearchRefusesParseErrors`)

#### Scenario: Excluded words are declared

- **WHEN** the query is `go test -race`
- **THEN** the reply's `excluded` is `["race"]` (`TestSearchDeclaresExcluded`)

### Requirement: A search is scoped by entity-ID prefix before the limit

A scope SHALL hold entity-ID prefixes, an empty list meaning every entity, and SHALL be applied before the limit.

#### Scenario: Better matches outside the scope

- **WHEN** a scoped search with limit 1 runs while better matches lie outside the scope
- **THEN** the hit is the best inside the scope and `matched` counts every match inside it
  (`TestSearchScopeLimitMatched`)

### Requirement: An offloaded body is read on one path with a bound

For an entity whose `StorageRef` names a store, graph-lexical SHALL read the body through `graph/entitytext` on one
body path that the follower never waits for, and index it after the inline text. A body not in the index SHALL be in
one of four states, `queued`, `unread`, `missing` or `no store`, each counted in `bodies_not_indexed` and in a gauge
by state, none withholding the verb.

#### Scenario: A body cut at the cap

- **WHEN** an entity's body is longer than `max_text_len`
- **THEN** its words past the cap are not found, its name's words are, the cut gauge counts it, and the first cut
  since start is logged once at warning level, naming the entity and the cap (`TestBodyCutAtCap`)

### Requirement: Each body state is retried and reported by its cause

`queued` SHALL mean not yet read. `unread` (any read failure but a missing object, or its store stalled) SHALL be
retried by every repair pass and reported as the failure `body_unread` (readiness degraded, named in `Health`).
`missing` (`storage.ErrObjectNotFound`) SHALL be retried only when the reference or inline text changes, or at
restart, and is not a failure. `no store` (no store registered under the
instance) SHALL be looked up again on every repair pass and is not a failure.

#### Scenario: A store that fails, then recovers

- **WHEN** an entity's body cannot be read at start and its store later serves it
- **THEN** a search for a body-only word misses it, `bodies_not_indexed` is 1 and the key reads degraded, and after
  the next repair pass it is found, `bodies_not_indexed` is 0 and the key reads ready
  (`TestUnreadBodyCountedThenRepaired`)

#### Scenario: A body that is gone, and a store that is not registered

- **WHEN** one entity's body is missing from its store and another names an instance no store is registered under
- **THEN** both are counted, `missing` and `no store`, the key reads ready and `Health` is healthy
  (`TestMissingAndUnresolvedBodies`)

### Requirement: A store that stalls is skipped until the next repair pass

After any read on a store fails, other than with a missing object, graph-lexical SHALL not read that store again
until the next repair pass, and its queued entities SHALL complete as `unread`, whatever error the store returned
(the NATS object store returns `nats: timeout` at its deadline, not a context error). Repair passes SHALL not
overlap.

#### Scenario: Many entities on a store that blocks

- **WHEN** 10,000 entities have bodies on a store whose reads block until their context ends, then return an error
  that wraps no context error
- **THEN** the initial build completes within one read deadline of the inline replay, every one of them counted
  `unread` (`TestBlockingStoreBoundsFirstBuild`, under `testing/synctest`)

### Requirement: Configuration is strict and tier 0

graph-lexical SHALL refuse at construction an unknown config key, naming it, SHALL take `text_suffixes` and
`max_text_len` with graph-embedding's meaning (`graph/entitytext`), `max_text_len` defaulting to 16,000 characters,
and SHALL import no model or provider module.

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

### Requirement: A damaged entity is left out and counted

When a delivered value is refused by `graph.UnmarshalEntityState`, graph-lexical SHALL report it to the follower,
remove the entity's words from the index, and complete the revision. A search reply SHALL carry `damaged_entities`,
the damaged entities within the request's `scope`, all when it has none. A valid value or a delete for the key SHALL
be applied as for any entity, with no restart.

#### Scenario: Damage stored at start

- **WHEN** A and B both hold the word `pump`, and B is stored damaged before the component starts
- **THEN** after the build a search for `pump` answers A alone with `damaged_entities` 1, and a search scoped to a
  prefix B is not under counts 0 (`TestSearchPastDamagedEntity`)

#### Scenario: Overwritten, then repaired

- **WHEN** B is indexed, overwritten with bytes that do not decode, then written with a valid value
- **THEN** B is not a hit and the count is 1 after the overwrite, and B is a hit with no count after the repair
  (`TestSearchPastDamagedEntity`)
