# Inventory: change 5 PR C, `setup-04a-05-fusion`

Packages: `pkg/fusion` (lens path), `pkg/fusion/fusionvocab`.

base: 431dfc4d5115341fa1644c8f2fdcc62e47ee4cfd (claim commit on origin/main 67a86509125cddef37d6d04c649fdec7f8659c5f)
pin: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128 (the copy at `/tmp/c5pin/src` PR A's inventory extracted, local only)

Question: what must PR C carry, change and drop to port the lens path of `pkg/fusion` and `fusionvocab`, after the
owner's ruling that fusion reads "not ready" from the reply (PR #550 comment 6716972354, item 4)? Repositories read
(`docs/inventory-scope.md`): this tree, the pin copy, semsource (symbol level, local checkout at `8737c7ea`, `git grep`
only) and semvocab (local checkout at `3faba58b`, `go.mod` and `catalog/cco` only). Labels: **measured** (command run
here), **read** (code read). PR A's inventory (`archive/2026-11-08-...-graph-embedding/inventory.md`) § 1, § 7 and the
pkg/fusion row of § 8 are the starting hypotheses, re-checked here.

## 0. Pin probes

Already run by the owner (PR #550 comment 6716972354) over `./pkg/fusion/...`, which includes `fusionvocab`: `go test
-count=1 -race` and `-count=5 -cpu=1 -shuffle=on`, rc=0 (PR A inventory § 0). No new run is needed: the package has no
`//go:build integration` file and no lifecycle owner, so there is no lifecycle suite to run. Test-text rules,
**measured** on the pin copy for the carried files: `time.Sleep` 0 (the one, `engine_test.go`, is the `Fuse` path, not
carried); `t.Parallel` 1 (`metrics_internal_test.go`, rewritten under D5; the other 5 are `fusionnats`, PR D); `Skip`
0; build tags none. Not reproduced by running: SemStreams #621 (unlabelled caps) and #603 (impact names); the caps are
**read** below (F8, F11), and the pin test `engine_facets_test.go:243-255` (`TestPaths_MaxPathsCap`, unit, passed in
the probe) asserts exactly 10 paths with no flag.

## 1. Sizes (measured, `wc -l`, pin)

| Part | Prod | Test |
| --- | --- | --- |
| pkg/fusion, all | 2,789 | 3,723 |
| not carried: `engine.go` 332, `subquery.go` 119, `evidence.go` 64, `client.go` 29; `engine_test.go` 332, `subquery_test.go` 85 | 544 | 417 |
| lens path (this PR) | 2,245 | 3,306 |
| pkg/fusion/fusionvocab | 51 | 52 |
| pkg/fusion/lensregistry (not carried) | 89 | 114 |

Pin imports, **measured** (`grep -h '"github.com/c360studio'`): lens path `graph message metric pkg/errs storage`;
tests add `internal/semantictest` (in `engine_graph_test.go`; this tree has no `internal/semantictest`, `ls internal/`);
`fusionvocab` `pkg/fusion vocabulary vocabulary/cco`, its test also `vocabulary/bfo`. All in the tree except
`vocabulary/cco`, `vocabulary/bfo` (`defer-exclude`, `docs/admission-ledger.yaml:2698-2728`, #234) and `semantictest`.

## 2. Category 1, claimed gaps

- "fusionvocab imports `vocabulary/cco` for two IRI strings" (`docs/admission-ledger.yaml:2723-2725`): **false**.
  `pin:pkg/fusion/fusionvocab/signals.go:40` — `return float64(len(cco.Parents(classIRI)))`, a transitive-ancestor
  walk (`pin:vocabulary/cco/hierarchy.go:87`). Tree: no ancestor function in `vocabulary` (PR A inventory § 2);
  semvocab has one, `semvocab:catalog/cco/hierarchy.go:64` — `func Parents(iri string) []string {`, but semvocab's
  `go.mod` requires `github.com/c360studio/semengine v0.1.0-alpha.2` (**measured**), so an import back is a module
  cycle.
- "fusion decodes `graph.IndexStatusResponse`" (#110 item 2, rewritten 2026-10-12): superseded by ruling 4
  (2026-11-08): fusion reads no status, so there is nothing to decode.
- "`pkg/fusion` calls `graph/embedding`" (`docs/admission-ledger.yaml:3767`): **false** at the pin; no pin fusion file
  imports it (import list above).
- `StoreResolver` assertion "deferred until pkg/fusion is ported" (`docs/admission-ledger.yaml:2271-2272`): open;
  `storage/storeregistry/storeregistry.go:97` — `func (r *Registry) Store(instance string) (storage.Store, bool) {`.

## 3. Category 2, spellings of the modelled facts

- Readiness record: `pin:pkg/fusion/contract.go:99` — `type IndexStatus struct {` beside
  `graph/index_status.go:30` — `type IndexStatusResponse struct {`; state mirror `pin:contract.go:73` —
  `type IndexState string` beside `graph/index_status.go:141`; its projection `pin:contract.go:159` —
  `func (s IndexStatus) readinessEnvelope() graph.IndexStatusResponse {`. #110 item 2.
- The gate: `pin:engine_lens.go:147` — `if proceed, reason := graph.EvaluateReadinessGate(`; tree home
  `graph/readiness/readiness_gate.go:117` — `func EvaluateReadinessGate(r Reading) DeferReason {`, whose one reader is
  graph-index in process (ADR-109 D3).
- "Not ready" from a reply: `pin:engine_lens.go:293-296` (`isIndexNotReady`, `errors.As` and `ce.Code ==
  graph.ErrorCodeIndexNotReady`); tree `graph/mutation_responses.go:87` — `ErrorCodeIndexNotReady =
  errs.CodeIndexNotReady`.
- Revision of an answer: `pin:engine_graph.go:205` reads it from the status; the tree's per-reply home is
  `graph/query_response.go:21` — ``IndexedRevision uint64 `json:"indexed_revision"` ``; `entity`, `batch`, `prefix` on
  `graph.query.*` are bare (`graph/query_verbs.go:95-100`).
- Register-or-get: `pin:metrics.go:46` — `if err := reg.Register(vec); err != nil {` beside
  `metric/registry.go:29` — `func RegisterOrGet[C prometheus.Collector](r *MetricsRegistry, serviceName, metricName
  string, candidate C) (C, error) {`.
- Missing-reason set: `pin:retrieval.go:97` ("mirrors graph.MissingReason value-for-value") beside
  `graph/query_batch_types.go:22`, which has `not_found` and `error` only (the ledger's `graph` row removed
  `MissingUnknown`); fusion's `UnhydratedUnknown` stays fusion's own.
- Search counts: `graph/query_search_types.go:58-59` (`unreadable_records`, `failed_records`); pin `Seed`
  (`pin:retrieval.go:60`) has no home for them.
- Ontology class predicate: `pin:engine_lens.go:56` — `const ontologyClassPredicate = "entity.ontology.class"`;
  `git grep -n 'entity.ontology.class' -- '*.go'` in the tree: 0 hits; semsource's own constant is its twin (pin
  comment `:54-55`).

## 4. Category 3, adjacent claims

- Open PRs (**measured**, `gh pr list --state open --json number,title,changedFiles,files`): #565 (this claim, 0
  files); #560 draft, 11 files, all under `internal/smoke/` and `openspec/changes/test-557-composed-stack-smoke/`
  (capability `composed-stack`). No shared file or capability with this change.
- Ledger: `vocabulary/cco` (`:2714-2728`), `storage/storeregistry` (`:2262-2278`), `graph` known_risks
  (`:2133-2137`, "pkg/fusion carries its IndexStatus copy"), `graph/readiness` (`:2190`, "Reading.Raw stays for
  fusion's port"), `graph/embedding` (`:3767`), ADR-071 (`:4413-4427`).
- Rulings: 03B D5 (`archive/2026-10-01-setup-03b-contract-boundary/design.md:347-352`, lens path keep, `Fuse` path
  defer); 03B critical coverage list names `pkg/fusion`, not `fusionvocab` (`:459-463`); split.md row C; ruling 4.
- Specs: no current spec names fusion (`grep -rn -i fusion openspec/specs/`: 0 hits).
- Stale comments naming fusion (**measured**, `git grep -n -i fusion -- '*.go'`): `graph/index_status.go:143-144`,
  `graph/readiness/watcher.go:23`, `:251`, `storage/storeregistry/storeregistry.go:22-25`, `:94-95`,
  `component/dependencies.go:54`, `processor/graph-ingest/query.go:481` (fusionnats, PR D),
  `vocabulary/registry.go:260`, `:285`.

## 5. Category 4, who it is for

Ported framework API with a consumer caller (semsource, **measured** `git grep`): `fusion.NewEngine` with
`.WithSignals(fusionvocab.New()).WithMetrics(c.metrics)` (`semsource:processor/code-context/component.go:212`, `:218`),
`fusion.NewBodyResolver(c.storeRegistry)` (`:232`), `fusion.MapStoreResolver` (`:241`), `fusion.RetrievalClient`
(`:82`, `exact_seed.go:25`), `ResolveMode*` (`source/fusion/lens/code/code.go:35-43`), and the in-memory fake
`source/fusion/fusiontest/memgraph.go:89` — `func (m *MemGraph) Status(context.Context) (fusion.IndexStatus, error)`.
No production read of `Response.Index`, `Deferred`, `ViewRevision` or `IndexStatus` fields (same grep: 0 hits outside
`fusiontest`). New surface this design adds: `Option`, `Resolution`, `Response.UnreadableRecords`,
`Response.FailedRecords`, `Node.RelationsTruncated`, `Response.PathsTruncated`, `fusionvocab.New(parents)`; each is
framework API whose contract line is a requirement of the new `fusion` spec, called by SemSource's code-context.

## 6. Category 5, problem shapes

- Classified refusal read from a reply: `processor/graph-query/query.go:32` forwards and relays the responder's class
  and code; fusion adopts the same reading (D1). No pre-ask gate exists in the tree outside graph-index's own
  in-process check (ADR-109 D3).
- Partial result labelled on the answer: `graph/query_search_types.go:58-59`, `pin:engine_facets.go:20`
  (`Path.Truncated`), `:30` (`Impact.Truncated`). Adopted for relations and the paths list (D4).
- Options at construction with a returned refusal: `graph/readiness` `NewWatcher(src, key, heartbeat, logger)` (ledger
  `:2169-2170`) and `internal/dispatch` `NewKeyedPool(ctx, cfg)` returning registration refusals (`:2249-2250`).
  Adopted (D5).
- Metric registration: `metric.RegisterOrGet`, as `internal/dispatch` and `graph/readiness` gauges do (ledger
  `:2166`, `:2249`). Adopted.

## 7. Category 6, pin ADRs

**Measured**: `grep -l -e pkg/fusion -e fusionvocab -e RankSignals -e RetrievalClient -e BodyResolver -e StoreResolver
-e Lens` over the pin's `docs/adr`, then the titles read (ADR-082 and ADR-075 name "fusion" in prose only).

| ADR | Decides, for these packages | In tree |
| --- | --- | --- |
| 062 deterministic graph fusion | the engine, Lens SPI, `lensregistry`, hydration by handle, increments 1-7 (5: ontology ranking via `vocabulary/bfo,cco` `Parents()`) | no |
| 063 store substrate and resolver | `StoreResolver`, registry satisfies it (`:282-283`) | yes |
| 066 honest readiness | fusion's mirror (`:338-344`), `Fuse` gates on `Ready` once per request (`:394`) | yes |
| 071 domain-scoped NL retrieval | `Request.Scope` to resolve | yes |
| 082 bounded-lag readiness | exact consumers incl. "fusion honesty envelope" MUST gate on `Ready` (`:65`) | no |
| 083 readiness as distributed state | fusionnats' watch (PR D) | yes |
| 084 readiness licenses health | D3 no absence, D4 `index_not_ready`, D6 omission reported, D7 hard stops bind envelope consumers | yes |
| 085 gate on health | fusion as a gate reader | yes |
| 075 framework package admission | `:76` fusion framework-owned | no; not binding on code |
| 109 (SemEngine) | D1 readiness rides the reply, D3 no consumer reads a sign first | yes |

## 8. Shape sweep (read on the pin copy, `pin:pkg/fusion/`)

| # | Shape | Pin | Disposition |
| --- | --- | --- | --- |
| F1 | second spelling | `contract.go:99` — `type IndexStatus struct {` | required (#110 item 2; ruling 4) |
| F2 | second spelling | `contract.go:73` — `type IndexState string` | required (same) |
| F3 | pre-ask gate | `engine_lens.go:124` — `status, err := e.graph.Status(ctx)` | required (ruling 4; ADR-109 D1, D3) |
| F4 | value from the removed read | `engine_graph.go:205` — `status, err := e.graph.Status(ctx)` | required with F3 (D2) |
| F5 | global registry | `metrics.go:41` — `var reg prometheus.Registerer = prometheus.DefaultRegisterer` | required (AGENTS.md § 1, metric-registry) |
| F6 | second spelling | `metrics.go:46` — `if err := reg.Register(vec); err != nil {` | required (`metric.RegisterOrGet` rule) |
| F7 | mutation after build | `engine_lens.go:102` — `func (e *Engine) WithMetrics(registry *metric.MetricsRegistry) *Engine {` | required with F6 (no place for a refusal) |
| F8 | silent cap | `engine_lens.go:495` — `if role == "" \|\| len(rels[role]) >= maxRelationsPerNode {` | required (declared-degrade rule; SS#621) |
| F9 | silent fault | `engine_lens.go:491` — `return nil` (after a non-readiness `Neighbors` error) | required (same) |
| F10 | silent fault | `engine_lens.go:508` — `if err != nil \|\| te == nil {` | required (same) |
| F11 | silent cap | `engine_facets.go:117` — `if len(out) >= maxPaths {` | required (same; SS#621) |
| F12 | advertised-absent | `engine_facets.go:202` — `return id` (an entity ID in a path of names, against `:14-15`) | required (Surface audit (c)) |
| F13 | silent fault | `engine_facets.go:91` — `if p := e.pathOf(ctx, dep, lens); p != "" {` | required (declared-degrade rule) |
| F14 | silent fault | `engine_graph.go:232` — `if err != nil \|\| counterpart == nil {` | required for the error case |
| F15 | silent skip | `engine_graph.go:281` — `continue // an unserializable object cannot be projected verbatim` | optional: no trigger, objects come from decoded JSON |
| F16 | swallowed error | `engine_lens.go:341` — `names, _ := e.graph.Names(ctx, query, 5)` | optional: suggestions; empty claims nothing |
| F17 | consumer predicate in engine | `engine_lens.go:56` — `const ontologyClassPredicate = "entity.ontology.class"` | optional: moving it to `Lens` changes the SPI for no failure |
| F18 | out-of-tree import | `fusionvocab/signals.go:40` — `return float64(len(cco.Parents(classIRI)))` | required (#234; D6) |
| F19 | stale doc | `retrieval.go:97` — `// mirrors graph.MissingReason value-for-value (pinned by a test) — fusion keeps its own` | required (Surface audit (c)) |
| F20 | deferred path | `engine.go:96` — `func Fuse(ctx context.Context, gq GraphQueryClient, ...` | not carried (03B D5) |
| F21 | TODO | `engine_lens.go:408` — `// TODO(when lens Engine is wired): distinct not_configured reason` | optional |
| F22 | label | `engine_lens.go:332` — `Provenance:      ProvenanceDeterministic,` on every deferral | optional |
| F23 | two not-ready policies | relations defer (`engine_lens.go:488`), paths/impact/graph truncate (`engine_facets.go:188-193`) | optional: both are declared |

Shape 3 (one implementation): `RankSignals` (`rank_signals.go:13`), implemented only by `fusionvocab.Signals` at the
pin; retained, it keeps `pkg/fusion` free of `vocabulary`. Shape 5 (error text): **measured** `grep -n
'err.Error()\|Contains(err'` over the production files, 0 hits. Context: **measured** `grep -n
'context.Background\|context.TODO\|WithoutCancel\|go func'`, 0 hits; no goroutine, no retained context.

## 9. Open issues naming a carried package

**Measured**, `gh issue list --state open --search <term> --limit 200` for `fusion`, `fusionvocab`, `lensregistry`,
`RankSignals`, `StoreResolver`, `cco`. Kept: #110 (item 2 is this PR's; the issue stays for item 3, temporal's port;
item 1 is done, `graph/index_status.go:26-27`). Names the word, not these packages: #557 (tag after PR D), #195
(`fusionnats`, PR D), #109, #9. SemStreams #621 and #603 are the plan's reproduce-first rows
(`docs/setup-plan.md:508-509`); #621 is D4's, #603 (impact names) stays deferred (03B D5 matrix row).

## 10. Adopter seam

Who: SemSource's code-context author and its fusion lens author.

1. Must know: `NewEngine` takes options and returns an error; `fusionvocab.New` needs semvocab's `cco.Parents`;
   `RetrievalClient` has no `Status` and `Resolve` returns a `Resolution`; the wire has no `index` (read `deferred`);
   `view_revision` is gone (under G2).
2. Do nothing: every item but the wire fails to compile (`fusiontest.MemGraph`, `code-context`). The wire change is
   silent for an agent reading `index.ready` from code-context's JSON.
3. Find out: compile errors; for the wire, `contract_version` "2" and `MIGRATION.md`.
4. Should know: nothing beyond passing the parent function; the gap is the wire's `index` block, closed by the version
   bump.

Collision table: not triggered (no durable, communication or coordination primitive). Intent check: not triggered
(03B D5 and split.md set the boundary; this design moves none).
