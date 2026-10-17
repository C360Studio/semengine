# Inventory: simplify-04a-15-graph-readiness

base: `a163c61153e4f23c751d267d72932cd74d8e3a04` (origin/main; the claim commit `c0221ef` is empty; draft PR #289)

Question: where `graph/readiness` keeps test hooks in production code, spells one fact twice, or turns bad wiring
into a silent no-op, and what each export is for. Scope: #135 (body findings 1–5 and its Tests paragraph; comment
items a–h; the "WaitForFirst has no test" item), plus two items other issues route here: #132 item b's remainder
(`AllDeferReasons`, routed by 8c's design D6) and #132 item i (the envelope's round-trip tests). Owner intent: epic #223
comment 6216623583 ("**8e — `graph/readiness`**: #135"), comment 6232804491 (8e ahead of 8d), and the epic body's
PR 8 line ("remove test hooks and duplicate reading types in `graph/readiness` (#135)"). Not in scope: `published_at`
(#183, PR 10), `IsKVTombstone`'s home (#133 item a, 8d), graph-ingest's own test seams `statusInterval`/`statusNowFn`
(`processor/graph-ingest/component.go:479-482`, #139, PR 10).

This change brings in no package from the pin: `graph/readiness` was ported by change 2
(`docs/admission-ledger.yaml:1979` — `- source_path: graph/readiness`, disposition `adapt`), so the pin probe runs
are not owed. #135's line numbers are at PR #93 head `7bf1eb8` / `88d0d3b`; every pin below is at the base commit.
The package holds 1,071 non-test lines in five files (`wc -l graph/readiness/*.go` without `_test.go`); unit coverage
91.1% (`go test -cover ./graph/readiness/`, base), floor 80% (`docs/repository-map.md:47`).

Repositories read (`docs/inventory-scope.md`): this tree; the SemStreams pin `8b99efe9` through `git show` /
`git grep 8b99efe9` on the local object store (the pin tree, not semstreams main); semsource and semconnect working
trees, for symbol-level readers only.

## 1. The claimed gaps (re-measured)

### Finding 1: two models of one consumer reading — CONFIRMED

- `graph/readiness/readiness_gate.go:103` — `type StatusReading struct {` (`Status`, `Fresh`; `:105`, `:108`).
- `graph/readiness/watcher.go:124` — `type Reading struct {` (`Status`, `Raw`, `Fresh`, `Known`, `Age`, `Err`;
  `:126-148`).
- `graph/readiness/readiness_gate.go:101` — `// Reading maps onto it directly; an IN-PROCESS producer that computes`.
- The pin's watcher reader converts one into the other: pin `processor/graph-clustering/component.go:1629-1630` —
  `proceed, reason := graph.EvaluateReadinessGate(` / `graph.StatusReading{Status: reading.Status, Fresh: reading.Fresh})`.
- No production caller of the gate or the Watcher in this tree (`git grep -nw 'EvaluateReadinessGate\|NewWatcher' --
  '*.go' ':!graph/readiness/*'`: two comment hits in `graph/index_status.go:95`, `:120`, no code).

### Finding 2: test-only hooks in the production Watcher — CONFIRMED

- `graph/readiness/watcher.go:161` — `rebindDelay time.Duration`; `:162` — `now         func() time.Time`.
- `:184-186` — `applied chan struct{}` ("a test-only synchronization hook"); `:211-233` — `withClock`,
  `withRebindDelay`, `withAppliedHook`; `:375` — `defer w.signalApplied()`; `:458-468` — `func (w *Watcher)
  signalApplied() {`.
- Their only setters are tests: `graph/readiness/watcher_test.go:143-145` (`withClock(h.clock.now)`,
  `withRebindDelay(time.Millisecond)`, `withAppliedHook(h.applied)`), `:325`, `:540`.
- The test file already runs a `synctest` bubble: `watcher_test.go:531` — `synctest.Test(t, func(t *testing.T) {`.
  Inside a bubble `time.Now` and `time.After` read the bubble's clock, so production's `now: time.Now`
  (`watcher.go:244`) already gives a test the clock it needs.
- Wall-clock guards the hooks force: `watcher_test.go:172`, `:177`, `:331`, `:338`, `:363`, `:505`, `:510` —
  `case <-time.After(5 * time.Second):`.
- The no-sleep check matches only the text `time.Sleep`; "a wait built from `time.After` or a timer [is] outside its
  scope by construction" (`openspec/specs/harness-boundaries/spec.md:226-230`). Precedent:
  `internal/dispatch/shutdown_test.go:54` — `<-time.After(time.Hour)` inside a bubble.
- With the hooks gone, `Option` carries two options: `watcher.go:194` `WithHeartbeat` (ignores `d <= 0`, `:196`)
  and `:203` `WithLogger` (ignores nil, `:205`). `FreshnessWindow` defaults a non-positive heartbeat a second time
  (`:110-112`).

### Finding 3: two spellings of one producer name, plus a hand-kept list — CONFIRMED

- `graph/readiness/gauges.go:18` — `type ProducerNames struct {` (`Service`, `Subsystem`).
- `Service` is only the service half of the registry's private key: `gauges.go:109` — `service := g.names.Service`;
  `metric/registry.go:38` — ``key := fmt.Sprintf("%s.%s", serviceName, metricName)``. The exposed name uses
  `Subsystem` only (`gauges.go:62`, `:68`, `:74`, `:80`, `:86`).
- One caller: `processor/graph-ingest/component.go:748-750` — `c.readinessGauges = readiness.NewGauges(` /
  `readiness.ProducerNames{Service: metricService, Subsystem: "graph_ingest"},` / `)`.
- `gauges.go:96` — `func (g *Gauges) MetricNames() []string {`; its only reader is
  `graph/readiness/gauges_test.go:31` (`TestGauges_MetricNamesArePinned`, `:19`). `gauges_test.go:52`
  (`TestGauges_FullyQualifiedNamesPreserveTheWireContract`) already pins the exposed names from the live registry.

### Finding 4: a nil Publisher is a silent no-op heartbeat — CONFIRMED

- `graph/readiness/publisher.go:68-73` — `func NewPublisher(bucket StatusWriter, key string) *Publisher {` / `if
  bucket == nil || key == "" {` / `return nil`.
- `:97-98` — `if p == nil || p.bucket == nil {` / `return nil` (Publish); `:117-118` the same in `Key`.
- Pinned by `graph/readiness/publisher_test.go:157` (`TestNewPublisher_RejectsIncompleteWiring`: asserts a nil
  publisher and a nil error from its `Publish`).
- graph-ingest's second guard: `processor/graph-ingest/readiness.go:328` — `if c.statusPublisher == nil {`. It cannot
  fire: the status loop starts after `createStatusBucket` (`component.go:1006`, loop at `:1022`), and the publisher is
  cleared only after the loop is joined (`component.go:1138-1143` join, `:1098` clear, only on a clean cleanup).
- `readiness.go:346-349` says a provisioning failure is fatal to Start; `NewPublisher` returning nil is not one.

### Finding 5: comments that are history — CONFIRMED in part, re-measured

- Comment lines / file lines: `index_status.go` 100/149, `readiness_gate.go` 122/174, `watcher.go` 194/468,
  `publisher.go` 52/121, `gauges.go` 50/159 (`grep -cE '^\s*//'`).
- **Changed since #135 was filed:** ADR-066, -083, -084, -085, -088 now resolve (`docs/adr/066-…`, `083-…`, `084-…`,
  `085-…`, `088-…`, ported without code for #157; ledger rows `docs/admission-ledger.yaml:550-609`).
- Still unresolved here: `gh#474` (`gauges.go:76`, `readiness_gate.go:67`, `:125`), `gh#590` (`readiness_gate.go:8`,
  `:44`; `watcher.go:11`, `:122`), `gh#712` (`index_status.go:27`), `gh#742` (`:38`) — SemStreams issues; ADR-055
  (`watcher.go:47`, not ported); `processor/graph-index/watermark.go:69-80` (`index_status.go:51`, `:110`, not
  ported); "framework-owned-bucket-guards F3" (`watcher.go:52`) and "the graph-index-readiness spec" (`gauges.go:53`),
  SemStreams specs; `pkg/fusion.IndexStatus` (`watcher.go:128`, not ported).
- Statements the code contradicts (surface audit (c)):
  - `watcher.go:98-101` — "rather than importing natsclient, so this package stays a leaf", and the package comment
    `:25-26`; `publisher.go:12` imports `natsclient`.
  - `publisher.go:103` — `// Unreachable (all-scalar struct)`; `graph/index_status.go:79` — `FailedReasons
    map[string]uint64`.
  - `watcher.go:1-5`, `:7-13` describe consumers gating on the watched key with "no per-decision NATS round-trip";
    ADR-109 Decision 3 (`docs/adr/109-ready-after-boot.md:131-141`): "`GRAPH_STATUS` is information, not a gate …
    Nothing in SemEngine waits on the bucket".
  - `watcher.go:261-262` — "The caller's own escape hatch (e.g. allow_ungated_reads)"; ADR-109's table drops it
    (`:192`, "Dropped: no consumer reads a sign before it asks").
  - `readiness_gate.go:142-144` — "graph-index maintains it in latchBootstrap"; graph-index is not ported, and
    graph-ingest's latch leaves with #183.

### Comment items on #135 (state at base)

- **a** settled: the gate and Watcher have pin readers (#135 comment 3); `ComputeIndexStatus`, `WithRevisionGauges`
  are gone (`git grep -n 'ComputeIndexStatus\|WithRevisionGauges\|GaugeOption' -- '*.go'`: comments only).
- **b**, **d** settled by removal (same search).
- **c** open: `readiness_gate.go:145` — `func EvaluateReadinessGate(reading StatusReading) (proceed bool, reason
  DeferReason) {`; every `return` pairs `true` only with `DeferNone` (`:148`, `:154`, `:157`, `:160`, `:162`).
- **e** open: `publisher.go:44-53` (`EnsureBucket`: a nil check and an error prefix over
  `kvcatalog.EnsureCatalogBucket`, `graph/kvcatalog/kvcatalog.go:236`); the nil check repeats
  `natsclient/kvspec.go:249` — `if c == nil {`. graph-ingest already calls `kvcatalog.EnsureCatalogBucket` directly
  for its other two buckets (`component.go:1172`, `:1199`). `publisher.go:61` `timeout` is only ever
  `publishTimeout` (`:72`).
- **f** open: `watcher.go:54` — `BucketGraphStatus = graph.BucketGraphStatus`, held by `bucket_alias_test.go:16`.
- **g** open: `watcher.go:133` `Raw []byte`; pin reader `pkg/fusion/fusionnats/client.go:227` — `if err :=
  json.Unmarshal(reading.Raw, &resp); err != nil {`. ADR-109's table assigns fusion's copy to "fusion's port (#135
  (g))" (`docs/adr/109-ready-after-boot.md:193`).
- **h**: the two names are not dead (#135 comment 3); the comment half is finding 5 above.
- **WaitForFirst** (`watcher.go:318`): no test (`git grep -n WaitForFirst -- '*_test.go'`: none); pin reader
  `pkg/fusion/fusionnats/client.go:168`.

### Routed here by #132 (8c, PR #286)

- **#132 item b remainder:** `readiness_gate.go:89` — `var AllDeferReasons = []DeferReason{` is an exported mutable
  slice; 8c design D6: "`readiness.AllDeferReasons` stays with 8e (#135)". Pin reader
  `processor/graph-clustering/metrics.go:92` — `var deferReasons = graph.AllDeferReasons`.
- **#132 item i:** the envelope's JSON round-trip tests live in `readiness`: `index_status_test.go:20`, `:49`, `:103`
  and `backlog_status_test.go:282`; a fifth sits in `graph/write_rules_test.go:241`.

## 2. Every current spelling of the facts touched

| Fact | Spellings at base |
| --- | --- |
| A consumer's reading of a producer | `StatusReading` (`readiness_gate.go:103`), `Reading` (`watcher.go:124`) |
| Whether to proceed | `proceed bool` and `reason == DeferNone` (`readiness_gate.go:145`) |
| A producer's metric name | `ProducerNames.Service` + `.Subsystem` (`gauges.go:18-23`); `MetricNames()` list (`:96-98`) and the five `RegisterOrGet` names (`:110-126`) |
| The status bucket's name | `graph.BucketGraphStatus` and `readiness.BucketGraphStatus` (`watcher.go:54`) |
| Ensuring a catalog bucket | `kvcatalog.EnsureCatalogBucket` (`kvcatalog.go:236`), `readiness.EnsureBucket` (`publisher.go:44`) |
| The heartbeat default | `NewWatcher` (`watcher.go:242`), `WithHeartbeat` guard (`:196`), `FreshnessWindow` (`:110-112`) |
| The watcher clock / rebind delay | `w.now` (`:162`), `w.rebindDelay` (`:161`) and `defaultRebindDelay` (`:95`) |
| The closed defer set | the const block (`readiness_gate.go:49-73`) and `AllDeferReasons` (`:89-94`) |

## 3. Adjacent claims

### Open pull requests

Read with `gh pr list --state open`; full file lists via `gh api --paginate …/pulls/<n>/files`.

- **#286 (8c, `graph`), draft, 72 files at the API listing.** Its branch head `8c15e6a` (2026-10-17 06:28) adds commits
  `ac7a7f2` (task 4.1, D6) and `dcdd011` (task 4.2) that edit **six files here**: `graph/readiness/gauges.go`
  (`:136`, `:144-145`, `graph.IndexStates()` and `string(s)`), `readiness_gate.go` (imports `:3`; comments `:59`,
  `:77`, `:121`; `recognizedState` `:165-174` takes `graph.IndexState`, uses `slices.Contains`), `gauges_test.go`,
  `readiness_gate_test.go`, `backlog_status_test.go:21`, and `graph/index_status.go` (`IndexState` type). It also
  edits `processor/graph-ingest/component.go` at `:867` and `:1294-1300`, and the `graph` ledger row
  (`docs/admission-ledger.yaml:1948-1990`, ending one row above `graph/readiness` at `:1979` on base). The owner's
  2026-10-17 note says "8c leaves `graph/readiness` alone"; at `8c15e6a` that is no longer so (mechanical type edits).
- **#288 (6b, `natsclient`), draft, 20 files:** all under `natsclient/` and its own OpenSpec change. No overlap
  (`natsclient/kvspec.go` is not in it).
- Dependabot #242–#248: `go.mod`/`go.sum`, `package*.json`, one workflow. No overlap.

Lines of `processor/graph-ingest/component.go` this change would touch: `:748-750` (the `NewGauges` call) only. #286's
hunks are at `:867` and `:1294`. `processor/graph-ingest/readiness.go` (`:16-17`, `:327-344`, `:350-360`) and
`readiness_integration_test.go` (`:50`, `:431`) are in no open PR.

### Open issues naming the package

Read with `gh issue list --state open --search "readiness" --limit 200`; every hit read.

| Issue | Names | Disposition |
| --- | --- | --- |
| #135 | the package | this change |
| #132 | `readiness.AllDeferReasons` (item b), readiness tests (item i) | 8c routes both here; #132 closes with #286 |
| #110 | `graph/readiness.Publisher.Publish`, `readiness/watcher.go:120-128` (fusion's copy) | change 5 (epic body) |
| #183 | `graph/readiness/publisher_test.go:206`, `watcher.go:18` (`published_at`) | PR 10 (epic body) |
| #133 | `watcher.go:377` (item a, `IsKVTombstone`) | 8d |
| #103 | `graph/readiness_gate.go:32-35` (read-your-writes comment, now `graph/readiness/readiness_gate.go`) | change 4 |
| #161 | the package, as an example | the harness check |
| #223 | the package (PR 8 line) | epic; `Addresses #223` |

Names the word, not the package: #7, #9, #16, #38, #108, #109, #139.

### Specs, ledger, ADRs

- No capability spec covers the gate, the Watcher or the publisher (`git grep -ln -i readiness -- openspec/specs`:
  `graph-transport-boundary/spec.md:94` — the envelope's `published_at` and legacy fields only;
  `lifecycle-suite/spec.md:26`, unrelated). `background-work/spec.md:63` covers `Run` under `synctest`.
- Ledger contract (`docs/admission-ledger.yaml:1987-1994`): "Publisher, gauges, Watcher, EvaluateReadinessGate,
  StatusReading, and ComputeBacklogStatus as at the pin, with these adaptations: (1)–(5)". `known_risks`
  (`:1998-2003`) names #135 items 1, 2, (c), (g) as open.

## 4. Exports and their readers

Readers: this tree's production code; the pin (`git grep 8b99efe9`, production files outside `graph/readiness`); and
named consumers at symbol level. Admitted, not yet ported: graph-index, graph-embedding, fusion/fusionnats, rule,
graph-clustering (`docs/tier1-cross-check.md:62-65`). `gateway/graph-gateway` is a consumer adapter (`:93`); its pin
readers are `readiness.NewSet`/`Dump`/`WithLogger` for `Set`, which change 2 dropped (ledger `known_risks`).

| Export | Here (production) | Pin (admitted, not ported) | Consumers |
| --- | --- | --- | --- |
| `BucketGraphStatus` (alias) | graph-ingest `readiness.go:340`, `:357` | 6 packages | semconnect `conformance/cmd/index-readiness/main.go:31`; semsource test |
| `KeyGraphIngest` | `readiness.go:359` | — | — |
| `KeyGraphIndex`, `KeyGraphEmbedding`, `KeyRule` | none | graph-index, -embedding, rule, clustering, fusionnats | semconnect `main.go:35`; semsource test |
| `DefaultHeartbeat` | `readiness.go:17` | 4 packages | — |
| `FreshnessMultiplier`, `FreshnessWindow` | none (own package `watcher.go:329`; graph-ingest comment `:16`) | comments only | — |
| `BucketSource` | none | fusionnats `client.go:132` | — |
| `Watcher`, `NewWatcher`, `Run`, `Read`, `Reading` | none | clustering `:1509`, `:1524`; fusionnats `:139` | — |
| `WaitForFirst` | none | fusionnats `:168` | — |
| `Reading.Raw` | none | fusionnats `:227` | — |
| `Option`, `WithHeartbeat`, `WithLogger` | none | clustering `:1510-1511`, `:1525-1526` | — |
| `StatusWriter` | `NewPublisher`'s parameter; graph-ingest passes `jetstream.KeyValue` | 4 producers | semsource test |
| `EnsureBucket` | `readiness.go:351` | graph-index, -embedding, rule | semsource test |
| `Publisher`, `NewPublisher`, `Publish`, `Key` | `component.go:457`; `readiness.go:331`, `:341`, `:359` | 4 producers | semsource test |
| `ProducerNames`, `Gauges`, `NewGauges`, `Register`, `Set`, `RecordPublishFailure` | `component.go:458`, `:748-751`; `readiness.go:108`, `:337` | 4 producers | — |
| `MetricNames` | none (test only) | none | — |
| `DeferReason`, `DeferNone`, `DeferStatusUnknown` | none | clustering, fusion, graph-index | — |
| `DeferHardStop`, `DeferUnrecognizedState`, `DeferBootstrapIncomplete` | none | gate's own returns; graph-index `query.go:169` comment | — |
| `AllDeferReasons` | none | clustering `metrics.go:92` | — |
| `StatusReading`, `EvaluateReadinessGate` | none | clustering, fusion `engine_lens.go`, `retrieval.go`, graph-index `query.go` | — |
| `BacklogStatusInputs`, `ComputeBacklogStatus` | `readiness.go:154` | graph-ingest | — |

## 5. The problem shapes and their closest instances

- Refuse bad wiring at construction, return an error: `internal/graphmutation/client.go:74-80` — `func
  NewClient(requester requester, timeout time.Duration) (*Client, error) {`; 8c's `NewExactEntityReader` (#286 D2).
  Adopted for `NewPublisher`.
- A closed set behind a function that returns a copy: 8c's `graph.IndexStates()` (`8c15e6a:graph/index_status.go:157`).
  Adopted for the defer reasons.
- Time control in tests through the `synctest` bubble, no production hook: `internal/dispatch/shutdown_test.go:54`;
  `watcher_test.go:529-585`. Adopted.
- No primitive is established; no adoption sweep is owed.

## 6. The pin's decision records

Searched: every `docs/adr` file at `8b99efe9` for `graph/readiness`, `ComputeIndexStatus`, `EvaluateReadinessGate`,
`GRAPH_STATUS`, `readiness.Watcher`, `ComputeBacklogStatus`; hits 066, 082, 083, 085, 088, 091, 094; read by title:
084. SemEngine's own: ADR-109.

| ADR | Decides (for this package) |
| --- | --- |
| 066 | `ready` is coverage; the readiness gauges and their names (`:89-92`) |
| 082 | bounded-lag gating for periodic consumers; its unit superseded by 083, its freshness parameter by 084 |
| 083 | readiness published as KV state, one key per producer (D1); freshness by consumer-local arrival, 3 heartbeats (D2); staleness reported (D3); one evaluator (D4) |
| 084 | readiness licenses health, never absence; `bootstrap_complete` (D2); hard stops bind consumers (D7) |
| 085 | gate on health: four conditions in order (D1); four defer reasons (D4) |
| 088 | per-producer keys; no mandatory list; backlog producers (graph-ingest) |
| 091, 094 | name `GRAPH_STATUS` in passing (graph-ingest, rule heartbeat); decide nothing in this package |
| 109 (here) | `GRAPH_STATUS` is information, not a gate (D3); the gate's one SemEngine reader is graph-index in process; ADR-083 D2 dropped from the contract; fusion's copy is fusion's port |

## 7. Collision table, intent check, adopter seams

- Collision table: not triggered. The change adds no durable, communication or coordination primitive.
- Intent check: not triggered. No boundary, tier, exclusion or deferral moves.
- Adopter seams (who reaches this from outside): later ports (graph-index, graph-embedding, rule, fusion,
  graph-clustering), semconnect's conformance tool, a semsource test that plays graph-index, and anyone reading the KV
  value or the gauges. The KV value and the five exposed metric names are the wire; every item in § 1 keeps them as
  they are. What an adopter must know today: (1) convert `Reading` into `StatusReading` before the gate; (2) check
  `proceed` and `reason` together; (3) a nil `*Publisher` publishes nothing and says nothing — found out nowhere;
  (4) state the producer name twice, differing in punctuation, or every metric is renamed silently.
