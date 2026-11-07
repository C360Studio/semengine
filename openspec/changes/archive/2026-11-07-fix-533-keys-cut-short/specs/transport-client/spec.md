# Delta: transport-client

## RENAMED Requirements

- FROM: `### Requirement: Both filtered key listings complete the same way`
- TO: `### Requirement: The key listings complete the same way`
- FROM: `### Requirement: The two filtered listings differ in their time bound`
- TO: `### Requirement: The key listings differ in their time bound`

## MODIFIED Requirements

### Requirement: The key listings complete the same way

`KVStore.KeysByFilter`, `natsclient.FilteredKeys`, and `KVStore.Keys` with the filter `>` SHALL list keys through one
implementation over the reader's `WatchFiltered`. They SHALL return every key in the watcher's initial snapshot, and
nil with no error when it holds none. `KVStore.Keys` SHALL return its keys sorted, each once. Changed: `KVStore.Keys`
read through nats.go's `Keys`, which returned the keys gathered so far when its watch closed early (#533).

#### Scenario: No key matches

- **WHEN** no key in a bucket on a NATS server matches the filter, or `KVStore.Keys` lists an empty bucket
- **THEN** the call returns nil and no error

#### Scenario: A repeated key is listed once

- **WHEN** the watcher's initial snapshot delivers a key more than once, out of key order
- **THEN** `KVStore.Keys` returns each key once, in sorted order

### Requirement: A filtered listing cut short returns no keys

When the context ends, or the watcher's update channel closes before the snapshot is complete, they SHALL return no keys
and an error, matching the context's error when it ended.

#### Scenario: Cancelled during the snapshot

- **WHEN** a listing's context is cancelled after the first key arrives and before the snapshot completes
- **THEN** the call returns no keys and an error matching `context.Canceled`, and the watcher's update channel has
  closed

#### Scenario: The watcher closes before the snapshot completes

- **WHEN** the watcher's update channel closes before the end-of-snapshot marker, with the context live
- **THEN** the call returns no keys and an error

### Requirement: A filtered listing stops its watcher before returning

Before returning, they SHALL stop the watcher and wait at most 5 s, past the call's context, for its update channel to
close. Once the snapshot is complete and the channel has closed, an error from Stop SHALL NOT fail the listing, as
nats.go `Keys` ignores it. Changed: such an error failed the listing (owner, 2026-11-07, #533).

#### Scenario: The watcher is stopped

- **WHEN** a listing returns
- **THEN** the watcher is stopped, and the call waited at most 5 s, past its context, for the update channel to close

#### Scenario: Stop fails after a complete snapshot

- **WHEN** the snapshot completes, Stop returns an error such as a denied consumer delete, and the channel closes
- **THEN** the listing returns its keys and no error

### Requirement: The key listings differ in their time bound

`KeysByFilter` and `Keys` SHALL also apply the store's operation timeout; `FilteredKeys` SHALL be bounded by its
caller's context. Changed: `FilteredKeys` read through nats.go's `ListKeysFiltered`, took a reader offering that
method, and returned the keys gathered so far when the lister's channel closed with the context live (#222).

#### Scenario: The bound of each call

- **WHEN** the listing does not complete in time
- **THEN** `KeysByFilter` and `Keys` end at the store's operation timeout or their caller's context, and
  `FilteredKeys` ends only with its caller's context
