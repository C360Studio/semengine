# Proposal: setup-04a-02-ingest-kernel

## Why

Slice 04A ports SemEngine's tier-0 set from the frozen SemStreams pin `8b99efe9` as a chain of seven OpenSpec changes
(foundation design, accepted on #9). Change 1 (`setup-04a-01-floor`, PR #48) ported the transport and message floor
and extended the test harness. This is change 2 (issue #91): it ports the packages graph-ingest needs that change 1
did not, so that **graph-ingest runs as a component under the lifecycle suite, with no boot path**. It is also where
the graph-ingest repair rows are proven, because their packages (`processor/graph-ingest`, `pkg/projection`) are
admitted together here: replay protection across stream generations (#15), the reserved request subjects' one
declaration (#16, its first half), conditional reconcile (#19), commit ambiguity (#20), settlement order (Q18), and the
payload fence from SemStreams PR #1437 (Q13). When it merges, the durable-execution epic #24 is unblocked (foundation
ruling (d)).

Terms: a **service** is an owner the engine starts and stops with a `Start` that can fail, run through the lifecycle
suite; **background work** is a goroutine that outlives the call that started it in code that is not a service; a
**guard** is graph-ingest's record of the last stream sequence applied per entity, used to drop redeliveries; a
**stream generation** is one lifetime of a JetStream stream, which restarts its sequences when the stream is recreated.

## What Changes

- Port 17 packages (30,613 non-test lines at the pin) with their 154 test files, plus `graph/llm` and `model/wire`
  carried unchanged and unused until the capability seam (change 7). Seventeen keep their pin paths; `pkg/worker` and
  `pkg/dispatch` move to `internal/worker` and `internal/dispatch` (no consumer imports them). `component` drops its
  agentic tool-registry field (#29). `component/lifecycle_test_suite.go` is not ported (its replacement is
  `internal/harness/lifecycletest`).
- Repair the ported tests to the harness rules: the pin's test NATS client becomes `natsfixture` (43 sites, a small
  helper per package); 28 sleeps, 6 skipped tests, 11 fixed broker addresses and 20 unbounded cleanups are repaired;
  eight test files that import packages outside the set are adapted (`payloadbuiltins` → per-package registration,
  agentic sample payloads → test-registered types).
- graph-ingest runs under `lifecycletest.Run` through a test-side adapter, with a failing factory, and a failed start
  whose own cleanup fails leaves what it holds for the next `Stop`.
- Seven helpers that run background work take the standing three shapes (`Run(ctx)`, `Shutdown(ctx)`): the worker
  pool, the two dispatchers, the readiness watcher and set, the review worker, and three channel-returning watches. The
  pool's second `Stop` after a timed-out one no longer panics.
- Metrics register through `metric.RegisterOrGet`, under the `semengine` namespace (after #92), on the registry the
  component is given, never on Prometheus' process-global registry. Metrics nothing writes are removed.
- Dead surface (157 candidates, §2 of `inventory.md`) is removed where nothing reads it; the rest is kept with its
  reason. graph-ingest refuses unknown configuration keys.
- Repair rows #15, #16 (declaration), #19, #20, settlement, Q13, #33, #29 and SS#1411, each with a failing-first test.
- The deployment-authority fields in `graph/inference.HierarchyConfig` go; the authority reaches the hierarchy
  inference as the `deps.Platform` carrier.
- Guidance returns with the packages: the SemStreams contract sections on semantic identity and graph, payload
  registry, and state ownership and component wiring; the skills `entity-or-bucket`, `kv-or-stream`, `new-payload`,
  `query-pattern`.
- Ledger: 19 package rows; the `internal/lifecyclecleanup` file row reconciled; `cover:check` gains ten targets.

## Capabilities

- `graph-ingest-recovery` (ADDED): accepted is not durable; recovery on a file stream is redelivery; settlement
  order; generation-aware replay protection; a payload that fails or panics on the Graphable lane is poison, not a
  redelivery loop.
- `projection-mutation` (ADDED): conditional reconcile at a caller-observed revision; commit ambiguity preserved.
- `graph-transport-boundary` (ADDED): the reserved request subjects have one declaration. The stream-filter refusal
  is change 3's.
- `component-registration` (ADDED): each component package registers itself; no aggregator; refusals name the
  per-package call; `component` reaches no agentic package.
- `lifecycle-suite` (MODIFIED): the Observe adapter contract covers a ported component and a failed start whose
  cleanup fails.
- `harness-boundaries` (MODIFIED): the image-pin check reads Go files that can start a container; the
  deployment-authority and public-signature requirements gain the exceptions owner questions B and C decide.
- `metric-registry` (ADDED requirement): nothing registers on Prometheus' process-global registry.

## Impact

- New Go code: 19 packages; `go.mod` gains `go-openai` (dormant) and `golang.org/x/net`.
- Metric names change from `semstreams_*` to `semengine_*` for graph-ingest, readiness and the component; no
  consumer pins SemEngine yet.
- `docs/admission-ledger.yaml`, `scripts/cover-check.sh`, the developer and reviewer contracts, four skills and
  `AGENTS.md` rows change.
- PR #92 merges first. Owner questions A (framing only), B, C and D are in `design.md`.
- Out of scope: the composition refusal of overlapping stream filters (#16, change 3); the parked-delivery scenario
  (`internal/maxdelivery`, change 7); `service`, `config`, `composition` (change 3).
