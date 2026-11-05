# community-grouping

## ADDED Requirements

### Requirement: A record names the latest entity write it reflects

A group record SHALL carry `entity_revision`: the highest revision among the `Upsert` and `Delete` calls the
maintainer applied for entities whose IDs parse into the group, held or not; a removed group takes its mark with it.
A tick SHALL write every group whose mark rose. With the caller feeding ascending revisions across keys, which it
SHALL, every `ENTITY_STATES` write at or below `entity_revision` SHALL be reflected in the record.

#### Scenario: Writes to a group

- **WHEN** a group receives an upsert, an upsert with the same words, a delete of a member, a delete of an entity the
  maintainer never held, and a refused value, each at a higher revision and followed by a tick
- **THEN** `entity_revision` takes each of those revisions in turn
- **AND** the refused value is reflected as contributing nothing to the record's counts; `Tick` names it to the
  operator, and a reader reads the entity itself from its page
- **AND** the test that verifies this is `TestRecordNamesItsLatestWrite`

#### Scenario: Obsolete deliveries and a recreated group

- **WHEN** a member receives a delivery at or below its last-applied revision, and later every member is deleted and
  one entity at revision R recreates the group
- **THEN** the obsolete delivery leaves `entity_revision` unchanged, and the recreated group's record carries R
- **AND** the test that verifies this is `TestRecordNamesItsLatestWrite`

#### Scenario: Nothing applied

- **WHEN** a tick runs with nothing applied since the last, and then after a same-content upsert
- **THEN** the first adds no revision to `COMMUNITY_INDEX` and the second writes each of the entity's groups once
- **AND** the test that verifies this is `TestUnchangedRecordAddsNoRevision`

### Requirement: A reader finds and checks a record as storage does

`clustering.GroupKey` SHALL return the key of an entity's group at a level and SHALL refuse a level outside 0 to 2 and
an ID the canonical parser refuses. `clustering.DecodeCommunity` SHALL decode the value at a key and SHALL refuse one
that is not JSON, whose level and ID do not give that key, or whose `entity_revision` is 0. `GetAllCommunities` SHALL
check each entry through it.

#### Scenario: A key and a record

- **WHEN** a product asks the key of an entity's level-1 group and decodes the value stored there, and a value with a
  wrong level or revision 0
- **THEN** it gets `1.{org}.{platform}.{system}` and the record, and each wrong value is refused naming the key
- **AND** the tests that verify this are `TestGroupKey` and `TestDecodeCommunity`

## MODIFIED Requirements

### Requirement: Applying a change depends only on the latest revision

The maintainer SHALL keep each present entity's last-applied revision and contribution; an `Upsert` or `Delete` at
or below that revision SHALL change nothing; a newer `Upsert` SHALL replace the contribution, and a `Delete` SHALL
remove it. The published records after any history SHALL equal those computed from the final entity values alone,
apart from `entity_revision`, which is computed from the deliveries applied. The caller SHALL pass, with each
`Upsert`, the revision of the exact value decoded, in increasing order per key.

#### Scenario: Two histories, one final state

- **WHEN** two histories interleave entities in different orders, keep each entity's own revision order, and end with
  the same entity values; obsolete deliveries are generated only for entities still present
- **THEN** the published records are byte-identical apart from `entity_revision`, which those histories need not
  agree on, since only an ascending feed fixes it
- **AND** the tests that verify this are `TestHistoryOrderIrrelevant` and `TestMaintainerMatchesModel`

#### Scenario: An obsolete delivery

- **WHEN** an entity applied at revision 9 receives an upsert or delete at revision 7
- **THEN** its contribution is unchanged
- **AND** the test that verifies this is `TestObsoleteDeliveryIgnored`

### Requirement: A group record is bounded

A group record SHALL hold its level, ID, member count, `entity_revision`, at most `MaxKeywords` types, at most
`MaxKeywords` keywords, a summary, and at most `MaxRepEntities` representative IDs, SHALL NOT hold the member list, and
its encoding SHALL be at most `clustering.MaxRecordBytes` whatever the members' values. A word SHALL be a run of 3 to
`MaxWordBytes` ASCII `a-z` and `0-9` characters; a longer run SHALL NOT be a word and SHALL NOT be truncated into one.

#### Scenario: A large group

- **WHEN** a group has more members than fit in one NATS message, `MaxKeywords` types whose type positions are
  246 bytes, `MaxRepEntities` 256-byte representative IDs and `MaxKeywords` 32-byte keywords
- **THEN** its record holds no member IDs beyond its representatives and encodes to at most `MaxRecordBytes`
- **AND** the test that verifies this is `TestRecordIsBounded`, which fails if an eleventh type is kept

#### Scenario: Long literals

- **WHEN** members' string values hold ten 120 KB alphanumeric literals
- **THEN** none is a keyword and the encoded record is at most `MaxRecordBytes`
- **AND** the test that verifies this is `TestLongLiteralIsNotAWord`
