# rule-state Specification

## ADDED Requirements

### Requirement: A rule's stored state is kept per rule and entity

`NewStateTracker` SHALL build a `StateTracker` over a `natsclient.KVStore`. `StateTracker` SHALL store one `MatchState`
per rule ID and entity key, under the key `<rule ID>.<entity key>`.
`Set` followed by `Get` for the same pair SHALL return an equal record. `Get` for a pair with no record SHALL return an
error matching `ErrStateNotFound`.

#### Scenario: A record round-trips

- **WHEN** a record with `IsMatching`, `Iteration`, `SourceRevision` and `FieldValues` set is saved and read back
- **THEN** the record read equals the one saved

#### Scenario: No record yet

- **WHEN** `Get` is called for a rule and entity never saved
- **THEN** the error matches `ErrStateNotFound`

### Requirement: Keys are checked before the bucket

`StateTracker` SHALL refuse a rule ID that is not one literal KV key token, and a key that is not a literal KV key,
before any bucket call, with an error classified invalid. It SHALL refuse a nil context.

#### Scenario: A wildcard in the entity key

- **WHEN** `Get` or `Set` is called with an entity key holding `*` or `>`, or a rule ID holding a dot
- **THEN** it returns an error classified invalid, and no record is written

### Requirement: An entity's records are deleted together

`DeleteAllForEntity` SHALL refuse an entity ID that is not a literal KV key, classified invalid, before listing any
key. It SHALL delete every record whose entity key is the given entity ID and no other record, SHALL return nil on a
bucket with no matching record, and SHALL return an error naming each record it failed to delete.

#### Scenario: Other entities are kept

- **WHEN** records exist for rules `r1` and `r2` on entity A and for `r1` on entity B, and `DeleteAllForEntity(A)` is
  called
- **THEN** both of A's records are gone and B's remains

#### Scenario: A wildcard entity ID

- **WHEN** `DeleteAllForEntity` is called with `>` or with an ID holding `*`
- **THEN** it returns an error classified invalid, and every record remains

#### Scenario: An empty bucket

- **WHEN** `DeleteAllForEntity` is called on a bucket with no records
- **THEN** it returns nil
