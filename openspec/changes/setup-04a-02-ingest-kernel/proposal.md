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

Before the graph packages are fixed in place, the owner ordered a pre-port design audit of them (PR #93 comment
6036316289). Its change-2 part is folded in here: three rulings (#97, #98, #99) and seven port-refactors that fix
debt at the port instead of carrying it (#100–#104, #106's pattern, #111 item 1). The owner's rulings A–F on the
review of that fold (#91 comment 6037287957) settle the open choices it raised.

Terms: a **service** is an owner the engine starts and stops with a `Start` that can fail, run through the lifecycle
suite; **background work** is a goroutine that outlives the call that started it in code that is not a service; a
**guard record** is graph-ingest's record of the last stream sequence applied per entity, used to drop redeliveries;
a **stream generation** is one lifetime of a JetStream stream, which restarts its sequences when the stream is
recreated; the **owner-lifecycle state** is a component's one-shot `Start`/`Stop` bookkeeping; a **write lane** is
the path a change takes into graph-ingest's entity bucket (a stream message, a mutation request, or a call inside
graph-ingest); a **statement** is one stored triple with its source, time, confidence and context.

## What Changes

- Port 14 of the 17 packages foundation D2 assigns to this change, with their tests, one of them, `graph/inference`, as
  its hierarchy slice (`hierarchy.go`, `container_entity.go` and the `TripleAdder` interface; #97). Not ported:
  `pkg/worker` (ruling E), `internal/componentadmission` (ruling C), `graph/structural` (to change 7 with its only
  readers, ruling E, which amends foundation D2 and the 80%-coverage critical list), and nothing of `graph/llm`,
  `model/wire` or `go-openai` (no package ported here reads them). `pkg/dispatch` moves to `internal/dispatch` and
  declares its own "stopped" error. `component/lifecycle_test_suite.go` is not ported (its replacement is
  `internal/harness/lifecycletest`).
- **One write seam** (#100): every change to a stored entity goes through one write path, with one identity rule per
  write mode (create, replace, conditional replace, append, delete), the same on every lane; `EntityState.Version`
  goes, and the key-value revision is the only fence. A replace is keyed on (subject, predicate, source): each source
  replaces only its own values, so a predicate may hold several sources' values, and the single-value reads pick the
  latest (ruling A). A reconcile names its source.
- **Statement metadata** (#98): every stored statement has a source and a time, taken from the message envelope on the
  stream lane, never from the clock by default; a statement's time orders a replace, and an older arrival is not
  applied and is counted; confidence and context never order a write; a reconcile is fenced by the caller's revision,
  not by time (ruling C). Statements graph-ingest derives (the indexing profile, hierarchy edges) carry the
  triggering message's time. The typed mutation client stops reading the clock.
- **One stored revision** (#99): `ENTITY_STATES` keeps one revision per key, written down as a constraint.
- **The `graph` root** (#101) holds the data model and wire types and imports no NATS package: the bucket catalog
  moves to a new `graph/kvcatalog`, the readiness computation to `graph/readiness`, and `events.go` (a second mutation
  grammar with no subscriber) is not ported; the catalog loses the `TOOL_CALL_OUTCOMES` row.
- **`component` reaches no graph package** (#102): `Dependencies.LifecycleManager` goes, as #29's tool registry does
  (ruling D);
  `ModelRegistry` and `StoreRegistry` stay, since neither reaches a graph package.
- **One reply envelope** (#103): `graph.QueryResponse` carries `indexed_revision` and `producer`, requests may carry
  `min_revision`, and the content-sniffing `UnwrapQueryResponse` is not ported; producers follow in change 4.
- **`ENTITY_SUFFIX_INDEX` and the suffix verb are not ported** (#104).
- **`graph.ingest.query.*` is declared once, by its responder** (#106's pattern): a verb table in `graph`, from which
  graph-ingest subscribes and callers take subjects.
- **Hierarchy birth and the guard record fail closed** (#111 item 1): on the lanes that infer hierarchy, an entity is
  born with its hierarchy statements or not at all; a mutation-lane create gets none, as at the pin (ruling B); a
  guard record that cannot be decoded is refused, not read as first seen.
- **The readiness envelope** (#110's change-2 part, ruling F): `IndexStatusResponse` loses `phase`, `revision` and
  `last_synced` and gains `published_at`, set by the publisher on every write.
- `component.Registry`'s `CreateComponent`, `SealComposition` and `Snapshots` lose the internal access-token parameter
  and become plain public methods whose doc comments direct callers to the component manager (ruling C).
- Repair the ported tests to the harness rules: the pin's test NATS client becomes one `natsfixture` helper that takes
  an open function (43 sites); sleeps, 6 skipped tests, 11 fixed broker addresses and 20 unbounded cleanups are
  repaired; eight test files that import packages outside the set are adapted.
- graph-ingest runs under `lifecycletest.Run` through a test-side adapter, with a failing factory.
- The failed-start rollback helper becomes public as `pkg/lifecyclecleanup.RollbackFailedStart`, same name and
  five-second budget, now refusing a nil callback (#77 ruling, comment 6035317931). A ported component whose failed
  `Start` cleanup also fails may keep what it could not release, reporting both errors and retrying in `Stop`, proven
  by its own test (the #38 exception for ported components). The `natsfixture` copy stays, each home naming the
  other.
- One owner-lifecycle guard, `internal/lifecycleguard`, composed by graph-ingest; the 11 later copies adopt it when
  ported.
- Helpers that run background work take the standing shapes (`Run(ctx)`, `Shutdown(ctx)`): the keyed dispatch pool,
  the readiness watcher, and the lifecycle manager's two watches.
- Metrics register through `metric.RegisterOrGet`, under the `semengine` namespace (after #92), on the registry the
  component is given, never on Prometheus' process-global registry. The configuration bucket and the mutation
  interface type are renamed to `semengine` under #69.
- Dead surface, read by type (`inventory.md` §2 and §9), is removed where nothing reads it; the rest is kept with its
  reason. graph-ingest refuses unknown configuration keys.
- Repair rows #15, #16 (declaration), #19, #20, settlement, the PR #1437 payload fence, #33, #29 and SemStreams #1411,
  each with a failing-first test; each audit port-refactor with its own.
- The deployment-authority fields in `graph/inference.HierarchyConfig` go; the authority reaches the hierarchy
  inference as the `deps.Platform` carrier.
- Guidance returns with the packages: the SemStreams contract sections on semantic identity and graph, payload
  registry, and state ownership and component wiring; the skills `entity-or-bucket`, `kv-or-stream`, `new-payload`,
  `query-pattern`.
- Ledger: package rows; the `internal/lifecyclecleanup` package and file rows reconciled; `cover:check` gains ten
  targets at the one 80% floor, `graph/inference` among them (the slice measures 82.8% at the pin; ruling F re-read).

## Capabilities

- `graph-entity-writes` (ADDED): one rule per write mode on every lane, a replace keyed by source; a single-value read
  that picks the same statement every time; statement metadata required; time orders a replace; the revision is the
  only fence; one stored revision per entity; birth with hierarchy fails closed.
- `graph-ingest-recovery` (ADDED): accepted is not durable; recovery on a file stream is redelivery; settlement
  order; generation-aware replay protection, refusing a guard record it cannot decode; a payload that fails or panics
  on the Graphable lane is poison, not a redelivery loop.
- `projection-mutation` (ADDED): conditional reconcile at a caller-observed revision, from one named source; commit
  ambiguity preserved; the typed client never reads the clock.
- `graph-transport-boundary` (ADDED): the reserved request subjects have one declaration, owned by their responder,
  which serves exactly its declared verbs; the `graph` root imports no transport; one reply envelope for the graph query
  family; the readiness envelope carries its publish time and no legacy fields. The stream-filter refusal is change 3's.
- `component-registration` (ADDED): each component package registers itself; no aggregator; refusals name the
  per-package call; `component` reaches no agentic or graph package; unknown configuration keys are refused.
- `harness-boundaries` (MODIFIED): in Go files the image-pin check matches a tag that starts with a digit or a
  variable, or a digest.
- `nats-fixture` (ADDED requirement): a connected value for a package's tests, with bounded cleanup.
- `metric-registry` (ADDED requirement): nothing registers on Prometheus' process-global registry.
- `lifecycle-suite` (MODIFIED "Portable floor"; ADDED "Failed start whose own cleanup fails" and "Failed-start
  rollback helper"): the #38 exception for ported components, and the public helper's contract. Its current "Observe
  adapter contract" already binds graph-ingest.

## Impact

- New Go code: 14 package directories from the pin (`graph/inference` as a slice), two new packages
  (`internal/lifecycleguard`, and `graph/kvcatalog` split from `graph`), a new `natsfixture` helper, and
  `internal/lifecyclecleanup` moved to the public `pkg/lifecyclecleanup`; `go.mod` gains `golang.org/x/net`.
- Metric names change from `semstreams_*` to `semengine_*` for graph-ingest, readiness and the keyed pool; two wire and
  storage names change; the consumers who spell them edit them when they adopt SemEngine.
- The write path changes behavior at port: a stream arrival replaces only its own source's statements of a predicate,
  where at the pin it replaced every source's; a statement without source or time is refused on every lane, as is an
  empty create on the mutation or in-process lane; an older stream arrival no longer overwrites its source's
  statements; stored values lose `version`. Mutation-lane births keep the pin's no-hierarchy behavior.
- Consumers that adopt SemEngine edit imports for what leaves the `graph` root, semboids' sim takes the lifecycle
  manager through its constructor, semteams acquires `TOOL_CALL_OUTCOMES` itself, and a raw-wire mutation caller
  stamps `source` and `timestamp` and names a reconcile's source; each finds out from a compile error or a typed
  refusal. semsource's reads of the readiness envelope's `revision` and `last_synced` go empty with no error; it
  changes on adoption (ruling F).
- Later changes inherit `class:port-refactor` items: rule (change 6), `service` (change 3), graph-query (change 4),
  `fusionnats` (change 5).
- A consumer that calls `Registry.CreateComponent`, `SealComposition` or `Snapshots` directly is no longer stopped by
  the compiler; that it uses the component manager instead is review only (ruling C).
- `docs/admission-ledger.yaml`, `scripts/cover-check.sh`, the developer and reviewer contracts, four skills and
  `AGENTS.md` rows change.
- Every owner question is ruled (`design.md`, "Ruled"): B, C, E and F on #91; A on #77 (comment 6035317931); #97,
  #98 and #99 on their issues; A–F on the audit's review (#91 comment 6037287957). PR #93 closes #91, #77 and
  #97–#104.
- SemTeams' components change one import path, `internal/lifecyclecleanup` to `pkg/lifecyclecleanup`, and their
  proving case compiles against it from this change on.
- Out of scope: the composition refusal of overlapping stream filters (#16, change 3); the parked-delivery scenario
  (`internal/maxdelivery`, change 7); `service`, `config`, `composition` (change 3); #105, #107–#109, #110 beyond its
  envelope fields, and #111 items 2–4 (changes 4, 5 and 7), which this change leaves open.
