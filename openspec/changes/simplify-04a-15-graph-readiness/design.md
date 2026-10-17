# Design: simplify-04a-15-graph-readiness

This design depends on `INVENTORY PASS` for `inventory.md`; its spec delta is `specs/graph-readiness/spec.md`, which
names `graph.IndexStates()` from #286. Premises cite the inventory's sections; line numbers are at the base
commit. No ADR decision is changed or dropped (inventory § 6): the gate's four conditions and their order (ADR-085 D1),
the four defer reasons (D4), one evaluator (ADR-083 D4), arrival-time freshness over three heartbeats (ADR-083 D2, kept
in code though ADR-109 drops it from the contract), and the exposed metric names (ADR-066) stay as they are. The KV
value is unchanged.

## D1. One reading, one answer (#135 finding 1, item c; #132 item b)

- `EvaluateReadinessGate(r Reading) DeferReason`. `StatusReading` goes. A caller proceeds when the result is
  `DeferNone`. A Watcher reader writes `EvaluateReadinessGate(w.Read())`, so status and freshness come from one read;
  an in-process producer (graph-index, ADR-109 D3) writes `Reading{Status: s, Fresh: true}`.
- The zero `Reading` answers `DeferStatusUnknown`, as the zero `StatusReading` does today.
- `DeferReasons() []DeferReason` returns a new slice of the four reasons; `AllDeferReasons` goes. This mirrors 8c's
  `graph.IndexStates()`.
- Options: (a) chosen: the gate takes the Watcher's type. (b) `(status, fresh bool)`: one type fewer to name, but a
  caller can pair a status from one read with freshness from another. (c) Keep both types: the pin's reader copies
  one into the other (inventory § 1). (d) Do nothing: the two types become API when change 4 or 5 reads them.
- Tests: `readiness_gate_test.go` keeps its tables with one result column; `TestReadingZeroValueFailsClosed`;
  `TestDeferReasonsReturnsACopy` (writing to the result does not change the next call). Sensitivity: `task
  mutate:check` with the `!r.Fresh` check removed is detected.

## D2. No test hooks in the Watcher (#135 finding 2, WaitForFirst)

- `now`, `rebindDelay` (field), `applied`, `signalApplied`, `withClock`, `withRebindDelay` and `withAppliedHook` go.
  `Read` and `apply` call `time.Now()`; the rebind wait is the `rebindDelay` constant (one second, as today).
- `NewWatcher(src BucketSource, key string, heartbeat time.Duration, logger *slog.Logger) *Watcher`. `Option`,
  `WithHeartbeat` and `WithLogger` go. `Run` refuses, before any I/O, a nil context, a nil source, an empty key (as
  today), a heartbeat of zero or less, and a nil logger. Nothing defaults: callers pass `DefaultHeartbeat`.
- `FreshnessMultiplier` and `FreshnessWindow` become unexported: no reader outside the package here or at the pin
  (inventory § 4). The second heartbeat default inside `FreshnessWindow` goes with the first.
- Observable: `Read`, `WaitForFirst` and `Run` behave as today, except `Run`'s two new refusals.
- Tests: `watcher_test.go` runs each test in a `synctest.Test` bubble, with every fake channel made inside it. A
  delivery is a send on the fake watch followed by `synctest.Wait()`; time moves with `<-time.After(d)`, which reads
  the bubble's clock (no `time.Sleep`; `harness-boundaries` "No sleeps in tests"). The 5-second wall-clock guards go.
  New: `TestWaitForFirst` (nil once an envelope is decoded, a stale-on-arrival one included; the context's error when
  none arrives; a tombstone or undecodable value does not release it); `TestWatcher_RunValidatesWiring` gains
  the heartbeat and logger rows. No real NATS runs in a bubble (#156 does not apply). Sensitivity: a mutant with a
  freshness multiplier of 4 is detected by `TestWatcher_FreshnessExpiresAtThreeHeartbeats`; one without the
  `firstOnce.Do` close is detected by `TestWaitForFirst`.
- The comments on how a nats.go watch delivers (`watcher.go:340-341`, `:360-361`) are carried text and stay word for
  word (ADR-112; Bounded port scope). This design makes no new library claim.

## D3. A publisher that refuses bad wiring (#135 finding 4, item e)

- `NewPublisher(bucket StatusWriter, key string) (*Publisher, error)` refuses a nil bucket and an empty key. A
  typed-nil pointer is not detected, as in `graphmutation.NewClient` (inventory § 5).
- `Publish` and `Key` lose their nil-receiver checks. `Publish` uses `publishTimeout`; the `timeout` field goes.
- `EnsureBucket` goes (owner question 1). graph-ingest's `createStatusBucket` calls
  `kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketGraphStatus)`, as it does for its two other buckets,
  and returns `NewPublisher`'s error wrapped like the provisioning error. `publishReadinessStatus` loses its nil guard,
  which cannot fire (inventory § 1, finding 4).
- Observable: a producer wired with a nil bucket or an empty key fails `Start` naming the cause. Today it publishes
  nothing, counts no failure, and consumers read the key as absent.
- Tests: `TestNewPublisherRefusesIncompleteWiring` replaces the base test that pins the no-op;
  `TestIntegration_ReadinessEnvelope_PublishedAndCatchesUp` (graph-ingest) still passes. Sensitivity: a mutant that
  accepts an empty key is detected.

## D4. One producer name (#135 finding 3)

- `NewGauges(subsystem string) *Gauges`. `Register` keys each collector under the subsystem. `ProducerNames`,
  `MetricNames` and `TestGauges_MetricNamesArePinned` go. graph-ingest: `readiness.NewGauges("graph_ingest")`
  (`component.go:748-750`, the only lines of that file this change touches).
- The exposed names do not change. The registry's private key changes from `graph-ingest.readiness` to
  `graph_ingest.readiness`; nothing outside `metric` reads it.
- Test: `TestGauges_FullyQualifiedNamesPreserveTheWireContract` also asserts that a `graph_ingest` set exposes exactly
  its five families. It fails if a name, the namespace or the subsystem changes.

## D5. One bucket name (#135 item f)

`readiness.BucketGraphStatus` goes (owner question 2); graph-ingest and its integration test use
`graph.BucketGraphStatus`; `bucket_alias_test.go` goes. If kept, it stays with its test, and this section is dropped.

## D6. Text follows the code (#135 finding 5, item h; #132 item i)

- The package comment states ADR-109 Decision 3: the bucket is information, the gate has one in-process reader, and
  the Watcher is kept for the ports that read it.
- Statements the code contradicts go (inventory § 1, finding 5): the "leaf" claim, `allow_ungated_reads`,
  "all-scalar struct", `latchBootstrap`. `gh#NNN` and unported paths are prefixed "SemStreams" or "pin". History is cut
  to the reason a rule exists.
- Item i: the four round-trip tests of `graph.IndexStatusResponse` and `graph/write_rules_test.go:241` become one table
  test in `graph/index_status_test.go`, which #286 adds.
- The `graph/readiness` ledger row: contract adaptations (6)–(9) for D1–D5; `known_risks` drops #135; proving tests
  drop `bucket_alias_test.go`.

## Exports (#190)

Kinds: **Plumbing** has a production caller here, or at the pin in an admitted package not yet ported (6a ruling 2).
**API** has a named consumer reading it (`docs/inventory-scope.md`) and a contract line.

| Symbols | Action | Kind | Caller or contract line; test |
| --- | --- | --- | --- |
| `KeyGraphIngest`, `DefaultHeartbeat`, `StatusWriter`, `Publisher` (+`Publish`, `Key`), `NewPublisher` | keep; D3 signature | Plumbing | graph-ingest `readiness.go:17`, `:331`, `:341`, `:359`; ledger contract |
| `Gauges` (+`Register`, `Set`, `RecordPublishFailure`), `NewGauges` | keep; D4 signature | Plumbing | `component.go:748-751`, `readiness.go:108`, `:337` |
| `BacklogStatusInputs`, `ComputeBacklogStatus` | keep | Plumbing | `readiness.go:154` |
| `KeyGraphIndex`, `KeyGraphEmbedding`, `KeyRule` | keep | Plumbing (pin) and API | pin producers; semconnect `main.go:35` reads `KeyGraphIndex` |
| `Watcher` (+`Run`, `Read`, `WaitForFirst`), `NewWatcher`, `Reading`, `BucketSource` | keep; D2 signature | Plumbing (pin) | clustering, fusionnats (inventory § 4); ledger contract |
| `Reading.Raw` | keep | Plumbing (pin) | fusionnats `client.go:227`; goes with fusion's port (#110, ADR-109) |
| `EvaluateReadinessGate`, `DeferReason` + 5 values | keep; D1 signature | Plumbing (pin) | clustering, fusion, graph-index; ledger contract |
| `DeferReasons` | new, replaces `AllDeferReasons` (D1) | Plumbing (pin) | clustering `metrics.go:92` |
| `StatusReading` | remove (D1) | — | epic #223 PR 8 line |
| `Option`, `WithHeartbeat`, `WithLogger`, `EnsureBucket` | remove (D2, D3); owner question 1 | — | their callers move to constructor arguments or `kvcatalog` |
| `BucketGraphStatus` (alias) | remove (D5); owner question 2 | was API | semconnect `main.go:31`; `graph.BucketGraphStatus` remains |
| `ProducerNames`, `MetricNames` | remove (D4) | — | one caller, which moves; test-only reader |
| `FreshnessMultiplier`, `FreshnessWindow` | unexport (D2) | — | no reader outside the package, here or at the pin |

## Open issues and merge order

- Retired: #135 (tasks 2.1–4.2). Items it hands on: (g) is recorded on #110 (change 5).
- Also done here, closed elsewhere: #132 items b (rest) and i (8c routes them here; #132 closes with #286).
- Stay: #110 (change 5), #183 (PR 10: `published_at` in `Publish`), #133 (8d: `IsKVTombstone`, `watcher.go:377`),
  #103 (change 4; its cite of the read-your-writes comment moves), #161 (the harness check).
- Merge order: #286 first. It is in final review and edits six files here (inventory § 3). Batch 1 touches only
  `watcher.go` and `watcher_test.go`, which #286 does not, so it can start on the base. Batches 2 and 3 start after
  this branch merges `origin/main` with #286 in it. #288 has no overlap. 8d and PR 10 come after this change and
  inherit D2 and D3.

## Owner questions

1. Later ports (graph-index, graph-embedding, rule, fusion, graph-clustering) call four things this design reshapes:
   the watcher's option functions, `EnsureBucket`, the list of defer reasons, and the gate's two-value answer. Each
   job stays, under one spelling: constructor arguments, the catalog's own bucket call, a function that returns a
   copy, one answer. Each port then changes one line, and the compiler points to it. The 6a ruling and 8c's answer
   kept names that nothing here reads. These names are read at the pin, and only their spelling changes.
   **Recommended: reshape them now.** Keeping them leaves the option framework, a third layer over bucket creation,
   a mutable list, and two values for one fact.
2. semconnect's conformance tool reads `readiness.BucketGraphStatus`, an alias of `graph.BucketGraphStatus` that a
   test keeps equal. **Recommended: remove the alias.** semconnect switches the import when it adopts SemEngine.
   Keeping it keeps two names for one bucket.

FYI, no ruling needed: `FreshnessMultiplier` and `FreshnessWindow` become private. Nothing outside the package reads
them, here or at the pin, and ADR-109 dropped freshness from the contract. The owner's note of 2026-10-17 said 8c
leaves `graph/readiness` alone. Since #286's commits of 2026-10-17 it no longer does, hence the merge order above.
