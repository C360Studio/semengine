# Proposal: setup-04a-02-ingest-kernel

## Why

Slice 04A ports SemEngine's tier-0 set from the frozen SemStreams pin `8b99efe9` as a chain of seven OpenSpec changes
(foundation design, accepted on #9). Change 1 (`setup-04a-01-floor`, PR #48) ported the transport and message floor
and extended the test harness. This is change 2 (issue #91): it ports the packages graph-ingest needs that change 1
did not, so that **graph-ingest runs as a component under the lifecycle suite, with no boot path**. It is also where
the graph-ingest repair rows are proven, because their packages (`processor/graph-ingest`, `pkg/projection`) are
admitted together here: replay protection across stream generations (#15), the reserved request subjects' one
declaration (#16, its first half), conditional reconcile (#19), commit ambiguity (#20), settlement order, the payload
fence from SemStreams PR #1437, and one owner-lifecycle guard (SemStreams issue #1411). When it merges, the
durable-execution epic #24 is unblocked.

Terms: a **service** is an owner the engine starts and stops with a `Start` that can fail, run through the lifecycle
suite; **background work** is a goroutine that outlives the call that started it in code that is not a service; a
**guard record** is graph-ingest's record of the last stream sequence applied per entity, used to drop redeliveries;
a **stream generation** is one lifetime of a JetStream stream, which restarts its sequences when the stream is
recreated; the **owner-lifecycle state** is a component's one-shot `Start`/`Stop` bookkeeping.

## What Changes

- Port 17 packages (16 if the owner leaves out `pkg/worker`, question E) with their tests, plus `graph/llm` and
  `model/wire` carried unchanged and unused until the capability seam (change 7). `pkg/dispatch` moves to
  `internal/dispatch` (no consumer imports it). `component` drops its agentic tool-registry field (#29).
  `component/lifecycle_test_suite.go` is not ported (its replacement is `internal/harness/lifecycletest`).
- Repair the ported tests to the harness rules: the pin's test NATS client becomes one `natsfixture` helper that takes
  an open function (43 sites); sleeps, 6 skipped tests, 11 fixed broker addresses and 20 unbounded cleanups are
  repaired; eight test files that import packages outside the set are adapted.
- graph-ingest runs under `lifecycletest.Run` through a test-side adapter, with a failing factory. A failed start whose
  own cleanup fails keeps the pin's behavior, recorded as a known risk and pinned by a test until #77 is ruled.
- One owner-lifecycle guard, `internal/lifecycleguard`, composed by graph-ingest; the 11 later copies adopt it when
  ported.
- Helpers that run background work take the standing shapes (`Run(ctx)`, `Shutdown(ctx)`): the keyed dispatch pool,
  the readiness watcher, the review worker, and the lifecycle manager's two watches.
- Metrics register through `metric.RegisterOrGet`, under the `semengine` namespace (after #92), on the registry the
  component is given, never on Prometheus' process-global registry. The configuration bucket, the mutation interface
  type and the alert digest domain are renamed to `semengine` under #69.
- Dead surface, read by type (186 candidates, `inventory.md` §2), is removed where nothing reads it, including the
  bounded dispatcher and the readiness set; the rest is kept with its reason. graph-ingest refuses unknown
  configuration keys.
- Repair rows #15, #16 (declaration), #19, #20, settlement, the PR #1437 payload fence, #33, #29 and SemStreams #1411,
  each with a failing-first test.
- The deployment-authority fields in `graph/inference.HierarchyConfig` go; the authority reaches the hierarchy
  inference as the `deps.Platform` carrier.
- Guidance returns with the packages: the SemStreams contract sections on semantic identity and graph, payload
  registry, and state ownership and component wiring; the skills `entity-or-bucket`, `kv-or-stream`, `new-payload`,
  `query-pattern`.
- Ledger: package rows; the `internal/lifecyclecleanup` file row reconciled; `cover:check` gains its targets at the one
  80% floor.

## Capabilities

- `graph-ingest-recovery` (ADDED): accepted is not durable; recovery on a file stream is redelivery; settlement
  order; generation-aware replay protection; a payload that fails or panics on the Graphable lane is poison, not a
  redelivery loop.
- `projection-mutation` (ADDED): conditional reconcile at a caller-observed revision; commit ambiguity preserved.
- `graph-transport-boundary` (ADDED): the reserved request subjects have one declaration. The stream-filter refusal
  is change 3's.
- `component-registration` (ADDED): each component package registers itself; no aggregator; refusals name the
  per-package call; `component` reaches no agentic package; unknown configuration keys are refused.
- `harness-boundaries` (MODIFIED): in Go files the image-pin check matches a tag that starts with a digit or a
  variable, or a digest; the deployment-authority and public-signature requirements gain the exceptions owner
  questions B and C decide.
- `nats-fixture` (ADDED requirement): a connected value for a package's tests, with bounded cleanup.
- `metric-registry` (ADDED requirement): nothing registers on Prometheus' process-global registry.
- `lifecycle-suite`: unchanged. Its current "Observe adapter contract" already binds graph-ingest; the failed-rollback
  branch waits for #77.

## Impact

- New Go code: 18 or 19 packages and two new ones (`internal/lifecycleguard`, the `natsfixture` helper); `go.mod`
  gains `go-openai` (dormant) and `golang.org/x/net`.
- Metric names change from `semstreams_*` to `semengine_*` for graph-ingest, readiness and the keyed pool; three wire
  and storage names change; the consumers who spell them edit them when they adopt SemEngine.
- `docs/admission-ledger.yaml` (also changed by #92, which merges first), `scripts/cover-check.sh`, the developer and
  reviewer contracts, four skills and `AGENTS.md` rows change.
- Owner questions A (framing only, on #77), B, C, E and F are in `design.md`.
- Out of scope: the composition refusal of overlapping stream filters (#16, change 3); the parked-delivery scenario
  (`internal/maxdelivery`, change 7); `service`, `config`, `composition` (change 3).
