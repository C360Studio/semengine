# transport-client

## ADDED Requirements

### Requirement: A refusal the server reports is counted

When the client was built with `WithMetrics`, each refusal nats.go reports to the client's async error handler as
`nats.ErrPermissionViolation` SHALL add one to the JetStream error counter
(`semengine_jetstream_operation_errors_total`) with operation `permission_violation`. Each reported as
`nats.ErrMaxSubscriptionsExceeded` SHALL add one with operation `max_subscriptions_exceeded`. The count SHALL be made
before the client's `NATS error` log line for that refusal is written, and SHALL be decided with `errors.Is`, never by
the error's text, so a refused publish and a refused SUB count alike. `Subscribe` and `SubscribeForRequests` SHALL
still return the subscription and nil for a SUB the server refused (owner ruling on #169, 2026-10-12; changed: at the
pin and before this change a refusal was only logged). This requirement does not say how soon after the call returns
the counter rises. A client built without metrics SHALL write the log line only.

#### Scenario: A refused subscription is counted

- **WHEN** a client with metrics subscribes and the server answers the SUB with a permissions violation before the
  PONG
- **THEN** the call returns the subscription and nil, and once the refusal's `NATS error` record is written the
  `permission_violation` count has risen by one

#### Scenario: A subscription over the server's limit is counted

- **WHEN** a client with metrics subscribes and the server answers the SUB with "maximum subscriptions exceeded"
- **THEN** the call returns the subscription and nil, and the `max_subscriptions_exceeded` count rises by one

#### Scenario: A refused publish is counted the same way

- **WHEN** the server answers a publish from a client with metrics with a permissions violation
- **THEN** the `permission_violation` count rises by one

### Requirement: Both filtered key listings complete the same way

`KVStore.KeysByFilter` and `natsclient.FilteredKeys` SHALL list keys through one implementation over the reader's
`WatchFiltered`. They SHALL return every key in the watcher's initial snapshot, and nil with no error when it holds
none. When the context ends, or the watcher's update channel closes before the snapshot is complete, they SHALL
return no keys and an error, matching the context's error when it ended. Before returning, they SHALL stop the watcher
and wait at most 5 s, past the call's context, for its update channel to close. `KeysByFilter` also applies the
store's operation timeout; `FilteredKeys` is bounded by its caller's context. Changed: `FilteredKeys` read through
nats.go's `ListKeysFiltered`, took a reader offering that method, and returned the keys gathered so far when the
lister's channel closed with the context live (#222).

#### Scenario: Cancelled during the snapshot

- **WHEN** either call's context is cancelled after the first key arrives and before the snapshot completes
- **THEN** the call returns no keys and an error matching `context.Canceled`, and the watcher's update channel has
  closed

#### Scenario: The watcher closes before the snapshot completes

- **WHEN** the watcher's update channel closes before the end-of-snapshot marker, with the context live
- **THEN** either call returns no keys and an error

#### Scenario: No key matches

- **WHEN** no key in the bucket matches the filter
- **THEN** either call returns nil and no error
