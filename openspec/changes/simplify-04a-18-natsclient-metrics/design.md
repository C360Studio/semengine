# Design: simplify-04a-18-natsclient-metrics

This design depends on `INVENTORY PASS` for `inventory.md`; premises cite its sections. It ports nothing from the
pin, so no pin probe applies. Its statements on how nats.go reports a refusal, and on an empty filtered watch, are
proved by tasks 2.1 and 2.3. Those tests drive the pinned nats.go v1.54.0: a scripted server for refusals, and a real
server for the empty watch.

## D1. A server refusal is counted (#169)

Premises:

- `handleError` only logs (§ 1, `client.go:1867`).
- nats.go hands both refusals to the async handler, typed and with no subscription (§ 1).
- A publish refusal carries the same type; telling them apart needs the text, which ruling N forbids (ADR-112 §3).
- The existing error counter's Help reads "Total number of JetStream operation errors", labelled `operation`
  (`jetstream_metrics.go:124-126`). A server refusal on a core connection is neither a JetStream error nor an
  operation.

Change (option B, recommended): `newJetStreamMetrics` also registers a counter vector,
`semengine_nats_server_refusals_total`, through `metric.RegisterOrGet`, with label `reason` and Help "Refusals the
NATS server reported on the client's connection: a publish or subscribe permission, or the subscription limit".
When metrics are configured, `handleError` adds one with reason `permission_violation` for
`errors.Is(err, nats.ErrPermissionViolation)` and `max_subscriptions_exceeded` for
`nats.ErrMaxSubscriptionsExceeded`, and then logs. The existing error counter and its Help are unchanged.

`Subscribe` and `SubscribeForRequests` are unchanged (owner ruling on #169, 2026-10-12). The counter may rise after
the call returns, which the ruling accepts. Without metrics, only the log line is written.

If the owner picks option A (the existing counter): the two values become `operation` values on
`semengine_jetstream_operation_errors_total`. Its Help widens to "Total number of JetStream operation errors and
NATS server refusals". The spec delta's series name and label change to match. A changed Help on that key conflicts
with nothing, since only `newJetStreamMetrics` registers it.

## D2. One metrics owner per registry; no ownership mechanism (#75)

Premises:

- The defect needs two clients on one registry; no production caller does that (§ 1).
- The same holds for two graph-ingest instances: no measured config runs two (§ 1).
- Within one client, setting `stream_state` to 0 means "this client cannot read the stream", and that is accurate.

Change: these places say it in plain words:

- natsclient: the `WithMetrics` doc comment and the README;
- graph-ingest: the comment at `processor/graph-ingest/component.go:723-725`;
- the ledger: natsclient `:990` and `:1088-1091`, graph-ingest `:2279`.

What they say: a registry carries one natsclient client's JetStream series, and one graph-ingest instance's series. A
second one on the same registry shares the collectors (design D9; graph-ingest design D4), and which one's write
stands is not promised. No measured configuration runs two. #75 closes with this, so none of these lines cites it
any more. No new issue is filed: no trigger was found. `TestJetStreamMetricsShareCanonicalCollectorsAcrossOwners`
stays.

## D3. One publish path

`publishToStream` becomes the one path, returning `(*jetstream.PubAck, error)`.

- `PublishToStream` and `PublishToStreamWithMsgID` return its error unchanged.
- `PublishToStreamWithAck` behaves as today:
  - `ErrNotConnected` and a `JetStream()` error come back bare and are not counted;
  - only a failure of the publish itself is counted as `publish_to_stream` and wrapped as transient.
- The status, JetStream and trace steps and the message build are written once. `publishToStreamAsync` uses the same
  message builder, and keeps its `ctx.Err()` check before it enqueues.
- The `AutoCreate` doc line is removed.

There is no change in behaviour.

## D4. One key listing

`KeysByFilter`'s body becomes an unexported function over a reader that offers `WatchFiltered`.

- `KVStore.KeysByFilter` applies the store's timeout, then calls it.
- `FilteredKeys` checks for a nil context, then calls it. Its parameter becomes
  `interface{ WatchFiltered(ctx, []string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) }`.
- The following go:
  - `collectFilteredKeys`;
  - the `ErrKeyNotFound` mapping;
  - the `ErrNoKeysFound` branch (`kv.go:632`).

  A source read says nats.go returns neither error from `WatchFiltered` (§ 1). Task 2.3's real-server row is the
  probe: an empty match must give nil and no error without those branches.
- The error prefix becomes `kv keys by filter`. `errors.Is` still matches the cause.

## D5. Text follows the code

- ADR-112 §2: a refused SUB is also counted when metrics are configured (#169).
- README `:102`, `doc.go`, `WithMetrics` (D2).
- Ledger:
  - natsclient row: contract gains D1's collector and D3 and D4's changed surface; `:990`, `:1011` and
    `:1088-1091` are reworded (D1, D2);
  - graph-ingest row: `:2279` (D2).

## Options

| Item | Options | Chosen |
| --- | --- | --- |
| #169 | A. two values on the JetStream error counter, Help widened; B. a new refusal counter; C. return an error (needs text or sync subs; ruled out) | B (Q1) |
| #75 | A. declare one owner per registry; B. refuse a second client (reverses D9; `Close` must unregister); C. client label (D9 says none); D. reference counts | A (Q2) |
| Publish | A. one path, errors per call as today; B. also unify errors and metric; C. leave | A |
| Keys | A. `FilteredKeys` on `KeysByFilter`'s code; B. remove `FilteredKeys`; C. leave both, record why | A (Q3) |

## Tests

- D1, new, in `subscribe_registration_test.go`:
  - `connectScripted` takes extra `ClientOption`s, applied after its defaults. The new tests pass `WithMetrics` and a
    `WithLogger` whose handler reads the refusal counter inside `Handle` when the `NATS error` record arrives.
  - The tests run outside a synctest bubble, since they need no fake clock, so `ownProcessBubble` (#156) does not
    apply. They do not sleep. Each one:
    1. waits on `server.pinged`;
    2. writes the `-ERR` line;
    3. calls `answerPings()`;
    4. checks that `Subscribe` returned the subscription and nil, and that the count read inside `Handle` had risen
       by one.
  - Rows:
    - `-ERR 'Permissions Violation for Subscription to "x"'` gives `permission_violation`;
    - `-ERR 'maximum subscriptions exceeded'` gives `max_subscriptions_exceeded`;
    - a publish refusal gives `permission_violation`.
  - They fail on base. `client_async_error_test.go` keeps its log rows.
- D3, new: `PublishToStreamWithAck` to a subject no stream captures counts `publish_to_stream` and is transient;
  `PublishToStream` stays unclassified. On a client that is not connected, `PublishToStreamWithAck` returns an error
  equal to `ErrNotConnected`, and the count is unchanged. These pass on base and after the change. Mutant: the
  metric call removed, through `task mutate:check`.
- D4:
  - `TestKVStoreFilteredCancellationFinishesNativeDelivery` and `_WatcherSnapshotAndStopErrors` run over both entry
    points. The `FilteredKeys` rows fail on base.
  - New integration row: a real bucket with no key matching the filter gives nil and no error through both entry
    points.
  - `TestCollectFilteredKeys_*` (3) go with `collectFilteredKeys`, and `TestFilteredKeysAcceptsMinimalReader` is
    rewritten for `WatchFiltered`.
  - The integration tests at `kv_key_contract_integration_test.go:384` and `:689` keep passing.
- Generated checks: none. Each change has one input and no stated law over sequences; examples suffice.

## Exports (#190)

| Symbol | Kind | Evidence | Disposition |
| --- | --- | --- | --- |
| `WithMetrics` | framework API | ledger contract `:839`; semboids `main.go:327`; `integration_test.go:284` | kept; doc line |
| `PublishToStream`, `PublishToStreamWithMsgID` | framework API | ADR-112 §2; semsource `publisher.go:406`, `component.go:549` | kept |
| `PublishToStreamWithAck` | framework API | ledger contract; semteams `teamhint/command.go:97`; `stream_integration_test.go:701` | kept |
| `KVStore.KeysByFilter` | plumbing | `processor/graph-ingest/query.go:270` | kept |
| `FilteredKeys` | framework API | ledger contract; semteams `derivation.go:914`; `kv_key_contract_integration_test.go:384` | kept, parameter changed (Q3) |

The new refusal counter (D1, B) is for the #169 ruling; its contract line is the spec requirement this change adds. No
Go export is added or removed.

## Open issues and merge order

- Retired: #75 (D2, task 3.2), #169 (D1, task 3.1) and #222 (tasks 3.1-3.4). Every other issue in inventory § 3
  stays, as another package's work.
- #289 merges first: it changes `processor/graph-ingest/component.go:745-753`, beside the comment D2 rewords. 6d then
  merges `origin/main` and rewords `:723-725` against #289's readiness gauges.
- #292 does not overlap and may merge in either order.

## Owner questions

1. When the server refuses a subscription, the call keeps returning success and the refusal is counted, as ruled on
   2026-10-12. Should that count go on a new counter for server refusals (recommended), or as two new values on the
   existing JetStream error counter, whose description would then widen? And since the library reports a refused
   publish and a refused subscription the same way, is it acceptable that the count includes both?
2. Two NATS clients, or two graph-ingest instances, sharing one metrics registry overwrite each other's gauges. No
   program or config measured runs that. Should we write "one per registry" in the comments, README and ledger and
   close the metrics issue, instead of making a second client on the same registry fail to start?
3. Should the free function that lists keys by pattern run on the method's code, which refuses a half-finished list?
   That means readers passed to it must offer a filtered watch. The alternatives are to delete the free function,
   which semteams uses, or to keep both and write down why.
