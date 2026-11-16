# graph-lexical Delta

## MODIFIED Requirements

### Requirement: A revision completes only once its text is in the index

graph-lexical SHALL report a revision complete only after the index holds it and, for an entity with an offloaded
body, after that body is indexed or has reached a state below. A body read SHALL be indexed only if, when it
returns, the key's latest revision still names the reference it read and that body is not yet indexed; it SHALL be
joined to that revision's inline text. Otherwise it SHALL be dropped, and the revision it was read for SHALL be
complete. A revision whose `StorageRef` and inline text equal those the index holds for the key SHALL leave the index
unchanged and ask for no body read, and a revision that keeps a reference whose read is queued or in flight SHALL ask
for no read of its own: a body is read again only when its reference or the entity's inline text changes, or at
start. References are equal only as a whole `StorageReference` (instance, key, content type and size). A reference
SHALL name fixed content: a producer that puts new content SHALL give it a new reference, since content replaced
behind an unchanged reference is not read again until one of those changes.

#### Scenario: A body read that returns after a newer delivery

- **WHEN** a body read for revision R is in flight and a delivery of the same key at R+1, naming a different
  reference, is applied before it returns
- **THEN** the index holds R+1's text and never R's body, and the follower's floor passes R
  (`TestStaleBodyReadDropped`)

#### Scenario: Non-text values change and the reference stays

- **WHEN** an entity's offloaded body is indexed and revisions follow that change only values that are not text,
  keeping the same reference, some arriving while the first read is still out
- **THEN** a search for a body-only word finds the entity after each of them once the first read has returned, no
  further body read is asked for, and each revision completes (`TestPropApplyMatchesModel`)

#### Scenario: Any history of deliveries, deletes and body reads

- **WHEN** a generated history interleaves deliveries that keep or change a key's reference, deletes, body outcomes
  and late read returns over a few keys
- **THEN** after each step every live key holds its latest revision's inline text, plus the body of that
  revision's reference only if a read of that reference succeeded since the key last named another reference or
  changed its inline text, as a reference model says, and every delivered revision has completed once its outcome
  is in (`TestPropApplyMatchesModel`, seed and checks recorded)

### Requirement: Each body state is retried and reported by its cause

`queued` SHALL mean not yet read. `unread` (any read failure but a missing object, or its store stalled) SHALL be
retried by every repair pass and reported as the failure `body_unread`, so the readiness key reads degraded and
`Health` names it.
`missing` (the store answers `storage.ErrObjectNotFound`) SHALL not be retried until the entity's reference or
inline text changes, or the component restarts, and is not a failure. `no store` (no store registered under the
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
