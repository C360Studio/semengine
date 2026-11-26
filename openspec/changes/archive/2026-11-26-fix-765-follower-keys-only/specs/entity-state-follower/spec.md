# entity-state-follower Delta

## MODIFIED Requirements

### Requirement: An entry carries the delivered value

`Entry.Value` SHALL be the value the watch delivered for `Entry.Revision`, and nil for a delete or purge marker. It
SHALL be valid only for the duration of `Deliver`; a plug-in that keeps it copies it. A plug-in that re-reads current
truth at apply time SHALL NOT apply `Value` (ADR-108 D4); a plug-in that applies `Value` SHALL ignore a delivery at or
below the revision it last applied for that key. A plug-in that sets the `Config` keys-only field SHALL receive every
entry with `Value` nil and `Bytes` 0, while `Key`, `Revision` and `Deleted` stay as they are without it, and SHALL read
the entity itself.

#### Scenario: Two writes to one key

- **WHEN** a key is written twice and both revisions are delivered
- **THEN** each entry's `Value` is the bytes written at its revision (`TestEntryCarriesDeliveredValue`)

#### Scenario: Keys only

- **WHEN** the keys-only field is set, and a key is written, deleted and purged
- **THEN** each entry has `Value` nil and `Bytes` 0, the delete and the purge arrive with `Deleted` true, revisions
  ascend, and the initial build completes (`TestKeysOnlyEntriesCarryNoValue`)
