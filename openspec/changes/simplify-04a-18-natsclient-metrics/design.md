# Design: simplify-04a-18-natsclient-metrics

This design depends on `INVENTORY PASS` for `inventory.md`; premises cite its sections. It ports nothing from the
pin, so no pin probe applies. Its one statement on how nats.go reports a refusal is proved by task 2.1's
scripted-server tests, which drive the pinned nats.go v1.54.0.

## D1. A server refusal is counted (#169)

Premises:

- `handleError` only logs (§ 1, `client.go:1867`).
- nats.go hands both refusals to the async handler, typed and with no subscription (§ 1).
- A publish refusal carries the same type; telling them apart needs the text, which ruling N forbids (ADR-112 §3).

Change: when metrics are configured, `handleError` adds one to the JetStream error counter, with operation
`permission_violation` for `errors.Is(err, nats.ErrPermissionViolation)` and `max_subscriptions_exceeded` for
`nats.ErrMaxSubscriptionsExceeded`, before it logs. `Subscribe` and `SubscribeForRequests` are unchanged (owner
ruling on #169, 2026-10-12). The counter may rise after the call returns, which the ruling accepts. Without metrics,
only the log line is written. This follows `late_delivery_refused` (§ 5).

## D2. One client per registry; no ownership mechanism (#75)

Premise: the defect needs two clients on one registry, and no production caller does that (§ 1). Within one client,
setting `stream_state` to 0 means "this client cannot read the stream", and that is accurate.

Change: the `WithMetrics` doc comment, the README and the ledger say it. A registry carries one client's JetStream
series. A second client on it shares them (design D9), and which client's write stands is not promised. #75 closes
with this. `TestJetStreamMetricsShareCanonicalCollectorsAcrossOwners` stays.

## D3. One publish path

`publishToStream` becomes the one path, returning `(*jetstream.PubAck, error)`.

- `PublishToStream` and `PublishToStreamWithMsgID` return its error unchanged.
- `PublishToStreamWithAck` counts `publish_to_stream` and wraps the error as transient, as today.
- The status, JetStream and trace steps and the message build are written once. `publishToStreamAsync` uses the same
  message builder, and keeps its `ctx.Err()` check before it enqueues.
- The `AutoCreate` doc line is removed.

There is no change in behaviour.

## D4. One key listing

`KeysByFilter`'s body becomes an unexported function over a reader that offers `WatchFiltered`.

- `KVStore.KeysByFilter` applies the store's timeout, then calls it.
- `FilteredKeys` checks for a nil context, then calls it. Its parameter becomes
  `interface{ WatchFiltered(ctx, []string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) }`.
- `collectFilteredKeys` and the `ErrKeyNotFound` mapping go (§ 1: a source read says nats.go returns neither).
- The error prefix becomes `kv keys by filter`. `errors.Is` still matches the cause.

## D5. Text follows the code

- ADR-112 §2: a refused SUB is also counted when metrics are configured (#169).
- README `:102`, `doc.go`, `WithMetrics` (D2).
- Ledger natsclient row:
  - contract: D3 and D4's changed surface;
  - known_risks: #75 becomes the declared one-registry line;
  - `:1011`: the refusal is counted.

## Options

| Item | Options | Chosen |
| --- | --- | --- |
| #169 | A. counter on the error counter; B. a new collector; C. return an error (needs text or sync subs; ruled out) | A (Q1) |
| #75 | A. declare one per registry; B. refuse a second client (reverses D9; `Close` must unregister); C. client label (D9 says none); D. reference counts | A (Q2) |
| Publish | A. one path, errors per call as today; B. also unify errors and metric; C. leave | A |
| Keys | A. `FilteredKeys` on `KeysByFilter`'s code; B. remove `FilteredKeys`; C. leave both, record why | A (Q3) |

## Tests

- D1, new, in `subscribe_registration_test.go`, where a scripted server drives the pinned nats.go:
  - the server answers the SUB with `-ERR 'Permissions Violation for Subscription to "x"'` before the PONG.
    `Subscribe` returns the subscription and nil, and by the time the "NATS error" record is written,
    `permission_violation` has risen by one;
  - the same for `-ERR 'maximum subscriptions exceeded'`;
  - a publish refusal also counts.

  These fail on base. `client_async_error_test.go` keeps its log rows.
- D3:
  - new: `PublishToStreamWithAck` to a subject no stream captures counts `publish_to_stream` and is transient;
    `PublishToStream` stays unclassified. Passes on base and after the change;
  - mutant: the metric call removed, through `task mutate:check`.
- D4: `TestKVStoreFilteredCancellationFinishesNativeDelivery` and `_WatcherSnapshotAndStopErrors` run over both entry
  points. The `FilteredKeys` rows fail on base. `TestCollectFilteredKeys_*` (3) go with `collectFilteredKeys`, and
  `TestFilteredKeysAcceptsMinimalReader` is rewritten for `WatchFiltered`. The integration tests at
  `kv_key_contract_integration_test.go:384` and `:689` keep passing.
- Generated checks: none. Each change has one input and no stated law over sequences; examples suffice.

## Exports (#190)

| Symbol | Kind | Evidence | Disposition |
| --- | --- | --- | --- |
| `WithMetrics` | framework API | ledger contract `:839`; semconnect, semboids; `integration_test.go:284` | kept; doc line |
| `PublishToStream`, `PublishToStreamWithMsgID` | framework API | ADR-112 §2; semsource `publisher.go:346` | kept |
| `PublishToStreamWithAck` | framework API | ledger contract; semteams `teamhint/command.go:97`; `stream_integration_test.go:701` | kept |
| `KVStore.KeysByFilter` | plumbing | `processor/graph-ingest/query.go:270` | kept |
| `FilteredKeys` | framework API | ledger contract; semteams `derivation.go:914`; `kv_key_contract_integration_test.go:384` | kept, parameter changed (Q3) |

The new label values are for the #169 ruling. No export is added or removed.

## Open issues and merge order

The change retires #75 (D2, task 3.2), #169 (D1, task 3.1) and #222 (tasks 3.1-3.4). Every other issue in inventory
§ 3 stays, as another package's work. #292 does not overlap and may merge in either order. Merge `origin/main` into
the claim branch before task 5.1.

## Owner questions

1. Follow the #169 ruling (nil return plus counter), counting publish and subscribe permission refusals alike?
2. Close #75 by declaring one client per registry, with no mechanism?
3. `FilteredKeys` takes a `WatchFiltered` reader and runs on `KeysByFilter`'s code, with option B or C as the
   alternative?
