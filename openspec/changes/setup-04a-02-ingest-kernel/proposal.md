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

- Port 15 of the 17 packages foundation D2 assigns to this change, with their tests, and from `graph/llm` only
  `client.go` (the `Client` interface, `ChatRequest` and `ChatResponse`), the part `graph/inference` reads (owner
  ruling B, #91 comment 6035477895). Not ported: `pkg/worker` (ruling E, #91 comment 6035429806),
  `internal/componentadmission` (ruling C, comment 6035477895), the rest of `graph/llm`, `model/wire` and `go-openai`
  (they wait for change 7). `pkg/dispatch` moves to `internal/dispatch` (no consumer imports it) and declares its own
  "stopped" error. `component` drops its agentic tool-registry field (#29). `component/lifecycle_test_suite.go` is not
  ported (its replacement is `internal/harness/lifecycletest`).
- `component.Registry`'s `CreateComponent`, `SealComposition` and `Snapshots` lose the internal access-token parameter
  and become plain public methods whose doc comments direct callers to the component manager (ruling C).
- Repair the ported tests to the harness rules: the pin's test NATS client becomes one `natsfixture` helper that takes
  an open function (43 sites); sleeps, 6 skipped tests, 11 fixed broker addresses and 20 unbounded cleanups are
  repaired; eight test files that import packages outside the set are adapted.
- graph-ingest runs under `lifecycletest.Run` through a test-side adapter, with a failing factory. A failed start whose
  own cleanup fails keeps the pin's behavior, pinned by a test; #77's ruling on it (2026-10-07) is applied by task 1.9.
- One owner-lifecycle guard, `internal/lifecycleguard`, composed by graph-ingest; the 11 later copies adopt it when
  ported.
- Helpers that run background work take the standing shapes (`Run(ctx)`, `Shutdown(ctx)`): the keyed dispatch pool,
  the readiness watcher, the review worker, and the lifecycle manager's two watches.
- Metrics register through `metric.RegisterOrGet`, under the `semengine` namespace (after #92), on the registry the
  component is given, never on Prometheus' process-global registry. The configuration bucket, the mutation interface
  type and the alert digest domain are renamed to `semengine` under #69.
- Dead surface, read by type (186 candidates, `inventory.md` §2), is removed where nothing reads it, including the
  bounded dispatcher and the readiness set; the rest is kept with its reason. graph-ingest refuses unknown
  configuration keys; `graph/inference` drops six configuration fields nothing reads, the `review.llm` key among them.
- Repair rows #15, #16 (declaration), #19, #20, settlement, the PR #1437 payload fence, #33, #29 and SemStreams #1411,
  each with a failing-first test.
- The deployment-authority fields in `graph/inference.HierarchyConfig` go; the authority reaches the hierarchy
  inference as the `deps.Platform` carrier.
- Guidance returns with the packages: the SemStreams contract sections on semantic identity and graph, payload
  registry, and state ownership and component wiring; the skills `entity-or-bucket`, `kv-or-stream`, `new-payload`,
  `query-pattern`.
- Ledger: package rows; the `internal/lifecyclecleanup` file row reconciled; `cover:check` gains its targets at the one
  80% floor. `graph/inference` is recorded at 47.9% and joins the gate in change 7 (ruling F).

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
  variable, or a digest. The deployment-authority and public-signature requirements are unchanged: rulings B and C
  remove the two exceptions round 2 asked for.
- `nats-fixture` (ADDED requirement): a connected value for a package's tests, with bounded cleanup.
- `metric-registry` (ADDED requirement): nothing registers on Prometheus' process-global registry.
- `lifecycle-suite`: unchanged in this revision. Its current "Observe adapter contract" already binds graph-ingest.
  The #38 exception #77 granted for ported components is written by task 1.9.

## Impact

- New Go code: 16 package directories from the pin (15 whole, and `graph/llm` as one file), one new package
  (`internal/lifecycleguard`) and a new `natsfixture` helper; `go.mod` gains `golang.org/x/net`.
- Metric names change from `semstreams_*` to `semengine_*` for graph-ingest, readiness and the keyed pool; three wire
  and storage names change; the consumers who spell them edit them when they adopt SemEngine.
- A consumer that calls `Registry.CreateComponent`, `SealComposition` or `Snapshots` directly is no longer stopped by
  the compiler; that it uses the component manager instead is review only (ruling C).
- `docs/admission-ledger.yaml` (also changed by #92, which merges first), `scripts/cover-check.sh`, the developer and
  reviewer contracts, four skills and `AGENTS.md` rows change.
- Owner questions B, C, E and F are ruled (`design.md`, "Ruled"). Question A was framing for #77, which the owner ruled
  on 2026-10-07 (#77 comment 6035317931); PR #93 closes #77 (comment 6035358884).
- Out of scope: the composition refusal of overlapping stream filters (#16, change 3); the parked-delivery scenario
  (`internal/maxdelivery`, change 7); `service`, `config`, `composition` (change 3).
