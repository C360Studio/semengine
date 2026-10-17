# Proposal: simplify-04a-15-graph-readiness

## Why

This is PR 8e of epic #223 (owner, 2026-10-16, #223 comment 6216623583: "**8e — `graph/readiness`**: #135"; taken
ahead of 8d on 2026-10-17, comment 6232804491). The epic's PR 8 line: "remove test hooks and duplicate reading types in
`graph/readiness` (#135)".

`graph/readiness` has two types for one consumer reading, and the gate answers with two values that say one thing.
The production Watcher carries six test-only hooks, although its tests already run in a `synctest` bubble. The gauges
take a producer name spelled twice, plus a hand-kept list of metric names. A publisher built with incomplete wiring is
nil, and a nil publisher publishes nothing and reports nothing. Its comments describe a gate that ADR-109 retired and
cite paths and issues that exist only in SemStreams.

## What Changes

- **One reading, one answer.** `EvaluateReadinessGate(Reading) DeferReason`; `StatusReading` goes. `DeferReasons()`
  returns a copy and replaces `AllDeferReasons`.
- **No hooks in the Watcher.** The clock, rebind and applied hooks go; tests use the `synctest` bubble's clock.
  `NewWatcher(src, key, heartbeat, logger)` replaces the options, and `Run` refuses a non-positive heartbeat or a nil
  logger. The freshness helpers become private. `WaitForFirst` gets a test.
- **A publisher that refuses bad wiring.** `NewPublisher` returns an error for a nil bucket or an empty key; no
  nil-receiver no-op. graph-ingest calls `kvcatalog.EnsureCatalogBucket` directly; `EnsureBucket` goes.
- **One producer name.** `NewGauges(subsystem)`; `ProducerNames` and `MetricNames` go. Exposed metric names unchanged.
- **One bucket name.** `readiness.BucketGraphStatus` goes; `graph.BucketGraphStatus` is the name.
- **Text.** The package comment states ADR-109 Decision 3; false statements go; SemStreams-only citations say so. The
  envelope's round-trip tests move beside the type (#132 item i). The ledger row is updated.

## Capabilities

- Added: `graph-readiness` (the gate's order and zero value; the publisher's and the Watcher's wiring refusals).

## Impact

- Files: `graph/readiness` (all files); `processor/graph-ingest/readiness.go` (`:16-17`, `:327-360`),
  `component.go:748-750`, `readiness_integration_test.go:50`, `:431`; `graph/index_status_test.go`,
  `graph/write_rules_test.go` (test move); `docs/admission-ledger.yaml` (`graph/readiness` row).
- No wire change: the KV value and the five exposed metric names are unchanged. The registry's private key for the
  readiness gauges changes from `graph-ingest.*` to `graph_ingest.*`.
- Behaviour change: graph-ingest's `Start` fails if the publisher's wiring is incomplete, instead of running without a
  heartbeat; `Watcher.Run` refuses a non-positive heartbeat or a nil logger.
- API change for later ports and consumers, caught by the compiler: the gate's signature, `NewWatcher`, `NewPublisher`,
  `NewGauges`; removed `StatusReading`, `Option`/`WithHeartbeat`/`WithLogger`, `EnsureBucket`, `ProducerNames`,
  `MetricNames`, `AllDeferReasons`, `readiness.BucketGraphStatus`.
- Closes #135. Addresses #223.
