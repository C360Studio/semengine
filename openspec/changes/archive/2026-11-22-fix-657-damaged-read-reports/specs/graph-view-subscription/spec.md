# graph-view-subscription Delta

## MODIFIED Requirements

### Requirement: An enumeration counts what it cannot serve

`View.ListCounted(keep)` SHALL return the entries whose keys `keep` accepts, sorted by key, and the number of poisoned
keys `keep` accepts (every key when `keep` is nil), both copied under one hold of the projection, and SHALL honour the
readiness rules `List` honours. `keep` SHALL run after the hold is released.

#### Scenario: Poison counted, then healed

- **WHEN** one key `keep` accepts holds a value its decode refuses, and later a valid value
- **THEN** `ListCounted` first returns the other entries and 1, then every entry and 0 (`TestListCountedCountsPoison`)

#### Scenario: Before the initial replay

- **WHEN** `ListCounted` is called before the view has caught up
- **THEN** it returns `ErrNotReady` (`TestListCountedCountsPoison`)
