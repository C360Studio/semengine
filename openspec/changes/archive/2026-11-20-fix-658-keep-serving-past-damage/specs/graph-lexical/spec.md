# graph-lexical Delta

## ADDED Requirements

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

## MODIFIED Requirements

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
