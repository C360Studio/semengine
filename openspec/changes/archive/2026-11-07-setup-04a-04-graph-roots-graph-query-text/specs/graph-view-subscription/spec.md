# graph-view-subscription

## ADDED Requirements

### Requirement: An enumeration counts what it cannot serve

`View.ListCounted(prefix)` SHALL return, under one hold of the projection, the entries `List(prefix, 0)` returns,
sorted by key, and the number of poisoned keys with that prefix, and SHALL honour the readiness rules `List` honours.

#### Scenario: Poison counted, then healed

- **WHEN** one key under the prefix holds a value its decode refuses, and later a valid value
- **THEN** `ListCounted` first returns the other entries and 1, then every entry and 0 (`TestListCountedCountsPoison`)

#### Scenario: Before the initial replay

- **WHEN** `ListCounted` is called before the view has caught up
- **THEN** it returns `ErrNotReady` (`TestListCountedCountsPoison`)
