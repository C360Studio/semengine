# Inventory: Slice 04A slicing (epic #9, claim PR #47)

Inventory only (architect contract step 2). No options, no recommendation, no target state, no tasks.

- base: SemEngine `38b184b29226c51177f63fb7676c761b4f94d47d` (worktree `claude/setup-04a-foundation`; `git diff --stat
  9286055..38b184b` is empty, so the tree equals `main` `9286055e`)
- SemStreams pin: `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read from the tarball
  `gh api repos/C360Studio/semstreams/tarball/8b99efe9…` extracted to `scratchpad/semstreams-pin/` (4,679 files, the
  count the 03B inventory recorded for `git ls-tree -r 8b99efe9`). Cited below as `path:line` at the pin.
- SemSource: `main` `e4febc0d2e23bf33811edc20b9a4159cf5275dc4` (2026-10-01, post-#223; `go.mod:8` pins
  `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`); 03A evidence branch `75a17f7d6297cf…` (cited where named);
  draft PR #222 head `533b62a74eae2ec93776cc316f69d9b55bf91cea`.
- SemConnect: PR #74 head `dff1265708910ef411ab6f85b16fd9e94663e997` (unchanged since 03B pass3).
- Pending-merge SemEngine PRs read as adjacent claims: #39 head `74db5cc9` (adds `docs/testing.md`, amends the
  architect contract), #44 head `6c568460` (OpenSpec change `flake-defense`).
- Scratch outputs (all under `scratchpad/04a/`): `pin-edges.txt` (180 packages, `go list -mod=mod -f
  '{{.ImportPath}} {{join .Imports " "}}' ./...`), `tier0-set.txt` (65), `tier0-lines.txt`, `inset-edges.json`,
  `dag.json`, `dag.py`, `deps16.txt` (the `go list -deps` of §2), `tier0-test-imports.txt`, `revive-tier0.txt`,
  `dependency-results.json` (03A),
  `semsource-run.go` (`cmd/semsource/run.go` @ `e4febc0d`), `semsource-222-design.md`, `semconnect-74-refcases.md`,
  `semconnect-backend-main.go` (@ `dff12657`), `semsource-qual-test.go` (@ `75a17f7d`).

## §0. Question and repositories read

**Question.** How is the tier-0 port set (65 packages / 140,842 non-test lines at the pin, 03B D4) cut into Slice
04A OpenSpec changes, in what order, and what surface does the first change touch? This file supplies the facts a
slicing design needs; it does not cut.

**Repositories read**, per `docs/inventory-scope.md:13-27` (rule 1: named before reading; none added):

| Repository | Read for | At |
| --- | --- | --- |
| semengine | harness surface, specs, ledger, gates, open claims | `38b184b`; PRs #39, #44 at their heads |
| semstreams | code facts only, from the pin snapshot | `8b99efe9` |
| semsource | composition root and import set; `tier0-statistical.json`; 03A closure JSON; the SemSource owner's tier-0 plan | `e4febc0d`, `75a17f7d`, PR #222 `533b62a7` |
| semconnect | PR #74 qualification evidence and backend composition root | `dff12657` |
| semboids, semteams | not re-read; symbol-level facts cited from 03B `inventory-2-scope.md` Q3 (`8c03cc53`, `ce22c961`) | — |

Not read: semembed, seminstruct, semdev, semstreams-ui, semmem, any repository marked "none".

## §1. Dependency structure of the tier-0 set at the pin

### §1.1 Measure

- Edge list: `go list -mod=mod -f '{{.ImportPath}} {{join .Imports " "}}' ./...` in the snapshot → 180 lines
  (`pin-edges.txt`), the same 180 pass3 §2.5 reports.
- Set: pass3 §2.3's 64 (set H) plus `composition/cli` (D4) = 65 (`tier0-set.txt`). Lines: `find <pkg> -maxdepth 1
  -name '*.go' -not -name '*_test.go' | xargs cat | wc -l` per package (`tier0-lines.txt`), the 03B measure.
- The induced subgraph (`inset-edges.json`) has **no cycle** (Tarjan over the 65 → `cycles []`; Go forbids import
  cycles, so "strongly connected" reduces to "tightly coupled", measured below as closure overlap).

### §1.2 Leaves, roots, levels

**14 leaves** (no in-set import): `graph/geo/geojson` 1,030 · `internal/componentadmission` 8 ·
`internal/lifecyclecleanup` 38 · `internal/logforwarderpolicy` 105 · `model` 1,397 · `pkg/graphview` 1,193 ·
`pkg/platform` 43 · `pkg/resource` 398 · `pkg/retry` 262 · `pkg/revlag` 213 · `pkg/security` 384 · `pkg/timestamp`
366 · `storage` 356 · `vocabulary/bfo` 402. (`pkg/graphview` imports only `nats.go/jetstream`; `model` imports
`net/http`, `x/net/http2`.)

**16 roots** (imported by no in-set package): the seven graph processors (`processor/graph-{ingest,index,
index-spatial,index-temporal,query,embedding,clustering}`), `processor/rule`, `output/websocket`, `storage/objectstore`,
`service`, `internal/maxdelivery`, `composition/cli`, and three libraries consumers import directly:
`pkg/fusion/fusionnats`, `pkg/fusion/fusionvocab`, `vocabulary/export`. The briefing's root list (`processor/*`,
`output/websocket`, `composition/cli`) is a subset; `service`, `storage/objectstore`, `internal/maxdelivery` and the
three libraries are roots too.

**Levels** (longest path from a leaf; level 0 = leaf) with lines, in-set deps, in-set importers, and transitive
closure (packages / lines, `http_embedder.go` not yet subtracted):

```text
lvl package                          lines deps imp  closure
 0  (14 leaves above)
 1  pkg/errs                           904   1  36    2 / 1,166
 1  storage/storeregistry              115   1   3    2 / 471
 1  vocabulary                       2,673   1  13    2 / 2,716
 1  vocabulary/cco                     515   1   1    2 / 917
 2  graph/embedding                  3,302   3   2    5 / 6,221
 2  pkg/acme                           543   1   1    3 / 1,709
 2  pkg/types                          751   1  17    3 / 1,917
 2  types                              188   1   6    3 / 1,354
 3  graph/query                      1,551   1   1    6 / 7,772
 3  pkg/projection/contract            163   2   2    6 / 4,796
 3  pkg/tlsutil                        533   3   2    5 / 2,626
 4  metric                           1,218   3  21    6 / 3,844
 4  payloadregistry                    508   4   7    7 / 5,304
 5  message                          2,186   5  15    9 / 7,856
 5  pkg/buffer                       1,235   2   1    7 / 5,079
 5  pkg/cache                        2,449   2   7    7 / 6,293
 5  pkg/worker                         707   1   1    7 / 4,551
 6  natsclient                      12,377   5  23    9 / 19,068
 6  vocabulary/export                1,107   2   0   10 / 8,963
 7  graph                            3,293   5  21   17 / 29,051
 7  internal/maxdelivery               309   3   0   10 / 19,377
 7  pkg/dispatch                     1,122   3   1   11 / 20,897
 8  graph/clustering                 5,516   6   2   18 / 34,567
 8  graph/readiness                  1,016   3   6   18 / 30,067
 8  graph/structural                   883   2   2   18 / 29,934
 8  internal/graphmutation             264   3   7   18 / 29,315
 8  pkg/fusion                       2,789   5   2   19 / 32,196
 8  processor/rule/expression        1,417   3   1   18 / 30,468
 9  graph/inference                  5,338  10   2   20 / 35,536
 9  pkg/fusion/fusionnats              658   5   0   21 / 33,870
 9  pkg/fusion/fusionvocab              51   3   0   22 / 33,164
 9  pkg/projection                     694   7   3   19 / 30,009
10  pkg/lifecycle                    3,739   9   3   20 / 33,748
11  component                        5,585  12  16   26 / 41,397
12  component/flowgraph              1,324   3   1   27 / 42,721
12  config                           5,193   9   5   27 / 46,590
12  health                             625   1   1   27 / 42,022
12  output/websocket                 2,220   8   0   29 / 44,890
12  processor/graph-clustering       4,117  15   0   32 / 58,305
12  processor/graph-embedding        3,253  15   0   31 / 49,219
12  processor/graph-index            4,527  12   0   30 / 47,191
12  processor/graph-index-spatial    1,526   8   0   29 / 43,991
12  processor/graph-index-temporal   1,573   8   0   28 / 43,008
12  processor/graph-ingest           5,870  16   0   33 / 56,371
12  processor/graph-query            6,745  13   0   32 / 59,742
12  storage/objectstore              3,342  11   0   28 / 44,777
13  composition                        914   4   2   29 / 48,828
13  pkg/rulepack                       143   3   2   28 / 46,733
14  composition/cli                    151   3   0   30 / 48,979
14  processor/rule                  15,826  19   0   32 / 65,030
14  service                         11,819  20   0   34 / 61,558
```

Full edge list per package: `inset-edges.json`. The edges a slicing design most depends on:

- `component` → `pkg/lifecycle` (`component/dependencies.go:12` import; field `Dependencies.LifecycleManager
  *lifecycle.Manager` `:99`), and `pkg/lifecycle` → `pkg/projection` → `internal/graphmutation` → `graph` →
  `natsclient`. Consequence: every processor's closure contains `pkg/lifecycle` (3,739), `pkg/projection`,
  `internal/graphmutation` and `graph`; the smallest closure that holds `component` is 26 packages / 41,397 lines. D4
  kept this field ("the removal of the `LifecycleManager` field … is withdrawn"); the edge is therefore load-bearing
  for any cut that ports `component` before `pkg/lifecycle`.
- `graph` → `natsclient` (`graph` imports `message natsclient pkg/errs pkg/types vocabulary`): `graph` cannot land
  without `natsclient` (12,377). closure(`graph`) = 17 / 29,051; the plan's "graph/ alone 9 / 23K" (`setup-plan.md:56`)
  is a beta.161 directory count, not a compile closure.
- `config` → `component` and `config` → `graph`, `model`; `composition` → `component`, `component/flowgraph`,
  `config`, `types`; `service` → `composition`, `config`, `health`, `pkg/rulepack`, `pkg/lifecycle`, `pkg/projection`.
  So `config`, `composition`, `service` sit above `component`, not beside it.
- `pkg/rulepack` (143 lines) → `config`, `natsclient`, `types` (`pkg/rulepack/identity.go:11-13`; `ValidateConfig(cfg
  *config.Config)` `:62`, `ValidateRuntimeUpdate(…types.ComponentConfig…)` `:91`). 03B D4 describes it as "a rule-pack
  ID contract"; at the pin it is also a config validator at level 13 whose closure is 28 / 46,733. `processor/rule`
  (`config.go:14`) and `service` (`rule_pack_bind.go:8`) both already import `config`, so it adds no closure to them.
- `graph/query` → `graph/embedding` only (in set); `graph/embedding` → `model`, `pkg/errs`, `storage` (not `graph`).
  `processor/graph-query` → `graph/query`. Consequence: the BM25 library `graph/embedding` (3,082 lines after the
  `http_embedder.go` split) enters 04A through `graph-query`; only the `processor/graph-embedding` component (3,253) is
  04B's.
- `processor/graph-query` → `graph/clustering` (5,516), `pkg/graphview`; `processor/graph-ingest` → `graph/inference`
  (5,338), `graph/structural`.
- `health` → `component` only; `component/flowgraph` → `component`, `internal/graphmutation`, `pkg/errs`.
- `internal/maxdelivery` → `metric`, `natsclient`, `pkg/errs`; its only exported symbol is `Start(ctx, natsClient,
  metricsRegistry, logger)` (`internal/maxdelivery/observer.go:224`); started only by `internal/boot/run.go:184`.
- `composition/cli` → `component`, `composition`, `config`; `Dispatch(args, registry *component.Registry, …)`
  (`composition/cli/main.go:53`) needs the consumer's registrations (D1).

### §1.3 Out-of-set edges (what a change must sever or carry dormant)

Reproduced in the snapshot (`grep -n` for the import lines):

| Kept package | Cut package | Import site at `8b99efe9` | 03B disposition |
| --- | --- | --- | --- |
| `component` | `agentic` | `component/dependencies.go:7`; field `ToolRegistry ToolRegistryReader` `:76` | #29, re-enabled by E3 |
| `service` | `agentic/agentrun` | `service/milestone_service.go:10` | #30 non-port |
| `processor/rule` | `agentic`, `agentic/agentrun`, `governance`, `vocabulary/agentic` | `actions.go:15,16,18,27`; `config_validation.go:9`; `verdict_auditor.go:10` | #25–#28 (E1–E4) |
| `processor/graph-query` | `graph/llm`, `vocabulary/agentic` | `answer.go:11`, `component.go:18`; `graphrag.go:22` | #32 seam, #31 |
| `graph/query` | `graph/llm` | `classifier_llm_adapter.go:9` | #32 |
| `graph/clustering` | `graph/llm` | `summarizer.go:12` | #32 (split-shape fork) |
| `graph/inference` | `graph/llm` | `config.go:12`, `review_worker.go:17` | #32 |
| `processor/graph-clustering` | `graph/llm` | `component.go:22` (import; uses `:592,599,2294,2492` per pass3) | #32 |

Eight packages, as pass3 §2.4. No other tier-0 package imports a package outside the 65 (`dag.py` `out_of_set`).

### §1.4 Closures of named root sets (ceiling measure: `graph/embedding` at 3,082)

| Root set | Packages / lines | Adds over the row above |
| --- | --- | --- |
| closure(`natsclient`) | 9 / 19,068 | — |
| closure(`message`) | 9 / 7,856 | — |
| closure(`natsclient`) ∪ closure(`message`) | 16 / 25,758 | `message metric natsclient payloadregistry pkg/{acme,cache,errs,platform,projection/contract,resource,retry,security,timestamp,tlsutil,types} vocabulary` |
| closure(`graph`) | 17 / 29,051 | +`graph` |
| closure(`component`) | 26 / 41,397 | +`component internal/componentadmission internal/graphmutation model pkg/lifecycle pkg/projection storage storage/storeregistry types` (9 / 12,346) |
| closure(`config`) | 27 / 46,590 | +`config` |
| closure(`composition`) | 29 / 48,828 | +`component/flowgraph composition` |
| closure(`service`) | 34 / 61,558 | +`component/flowgraph composition config health internal/lifecyclecleanup internal/logforwarderpolicy pkg/rulepack service` (8 / 20,161 over `component`) |
| closure(`processor/graph-ingest`) | 33 / 56,371 | over `component`: `graph/inference graph/readiness graph/structural internal/lifecyclecleanup pkg/dispatch pkg/worker processor/graph-ingest` |
| graph-ingest + graph-index | 35 / 61,111 | |
| ingest + index + query + objectstore | 41 / 82,540 | |
| … + `service` | 48 / 102,663 | `service` adds `component/flowgraph composition config health internal/logforwarderpolicy pkg/rulepack service` |
| closure(`processor/rule`) | 32 / 65,030 | over the 48 above: only `processor/rule`, `processor/rule/expression` |
| closure(`output/websocket`) | 29 / 44,890 | over `component`: `internal/lifecyclecleanup output/websocket pkg/buffer` |
| closure(`internal/maxdelivery`) | 10 / 19,377 | |
| closure(`processor/graph-embedding`) | 31 / 48,999 | over the 48: only itself |
| closure(`processor/graph-clustering`) | 32 / 58,305 | over the 48: only itself |

Increments of each root over closure(`service`): graph-ingest 6 / 14,936; graph-index 3 / 5,756; graph-query 5 /
18,087 (`graph/clustering graph/embedding graph/query pkg/graphview processor/graph-query`); objectstore 1 / 3,342;
spatial 2 / 2,556; temporal 1 / 1,573; clustering 5 / 16,870; embedding 4 / 7,564; rule 3 / 18,259; websocket 2 /
3,455; maxdelivery 1 / 309; cli 1 / 151; fusionnats 3 / 4,463; fusionvocab 4 / 3,757; export 1 / 1,107; graphview 1 /
1,193.

Partition of the 65 by the closures above (unordered; a property of the DAG, not a slice sequence):
closure(`natsclient`) ∪ closure(`message`) 16 / 25,758; `graph` 1 / 3,293; closure(`component`) minus closure(`graph`)
9 / 12,346; closure(`service`) minus closure(`component`) 8 / 20,161; the remaining 31 packages (processors, graph
libraries, fusion, the consumer-facing libraries) 79,284; sum 65 / 140,842. The DAG levels in §1.2 already state the
edge-direction property: every in-set edge goes from a higher level to a lower one.

Consumer compositions (symbol-level roots, §5) and what each leaves outside:

| Composition | Packages / lines | Outside it |
| --- | --- | --- |
| SemSource as composed at `e4febc0d` (ingest, index, **embedding**, query, objectstore, service, fusionnats, fusionvocab, websocket; `graph-gateway` is cut by D4, `graph-clustering` is opt-in and off by default) | 56 / 113,786 | `composition/cli graph/geo/geojson internal/maxdelivery processor/graph-clustering processor/graph-index-{spatial,temporal} processor/rule{,/expression} vocabulary/export` |
| SemSource with `enable_clustering` (`run.go:947-952`) | 57 / 117,903 | the row above minus `processor/graph-clustering` |
| SemConnect (ingest, index, spatial, temporal, query, objectstore, service, composition, export, geojson) | 52 / 107,899 | `composition/cli internal/maxdelivery output/websocket pkg/buffer pkg/fusion{,/fusionnats,/fusionvocab} processor/graph-clustering processor/graph-embedding processor/rule{,/expression} vocabulary/{bfo,cco}` |
| semboids (ingest, index, clustering, rule, websocket, graphview, lifecycle, projection, service) | 49 / 112,758 | `composition/cli graph/embedding graph/geo/geojson graph/query internal/maxdelivery pkg/fusion* processor/graph-embedding processor/graph-index-{spatial,temporal} processor/graph-query storage/objectstore vocabulary/{bfo,cco,export}` |
| Union of the three | 63 / 140,382 | `composition/cli` (owner mandate, D15), `internal/maxdelivery` (D13; composed by no consumer) |

SemSource composes `graph-embedding` unconditionally (`run.go:812-817`, default `embedder_type` `"bm25"` `:724`; its
owner's seam table: "BM25 and graph-embedding always enabled", PR #222 `design.md:27`). A no-embedder SemSource
composition does not exist at `e4febc0d`; it is the composition #9 asks the SemSource owner to build, and
`processor/graph-embedding` is placed in Slice 04B by ruling (5932313950), not by consumer reach. Without
`processor/graph-embedding` the SemSource row would be 55 / 110,533 — a hypothetical, recorded only so the 04B
subtraction is visible.

Intersection of all 16 root closures: `pkg/errs`, `pkg/retry` (1,166 lines) — the only packages every root needs.

### §1.5 Per-package test volume and test-side reach

Ported tests are part of each cut.

- Tier-0 test files / lines at the pin: **774 files / 218,454 lines** (`ls <pkg>/*_test.go | wc -l`; `cat … | wc -l`),
  against 140,842 production lines. Largest: `processor/rule` 107 / 32,114; `natsclient` 78 / 21,656; `service` 65 /
  18,653; `graph-index` 40 / 13,936; `graph-ingest` 56 / 13,928; `graph-query` 29 / 10,028.
- Tests carrying `//go:build integration`: 165 files in 24 packages.
- Non-tier-0 SemStreams packages that tier-0 tests import (`go list -f '{{join .TestImports}} {{join
  .XTestImports}}'`): `internal/semantictest` ← 13 packages (`graph graph/clustering graph/inference message pkg/fusion
  pkg/lifecycle processor/graph-clustering processor/graph-index processor/graph-ingest processor/graph-query
  processor/rule processor/rule/expression service`; 67 non-test lines, imports `pkg/types`, `vocabulary`);
  `graph/llm` ← 5; `componentregistry` ← 3 (`composition/shipped_configs_test.go`, `composition/cli/main_test.go`,
  `service/message_logger_census_test.go`); `frameworkcapabilities/graphresearch` ← 3; `agentic` ← 3 (`graph-ingest`,
  `rule`, `service`); `payloadbuiltins` ← 3; `cmd/e2e-semstreams/mission`, `examples/processors/{document,iot_sensor}`
  ← 2 (`composition`, `service`); `frameworkadapters/otel` ← 2; `agentic/research` ← 2; `governance`,
  `processor/agentic-tools`, `vocabulary/agentic`, `agentic/agentrun` ← 1 each. 03A's "closure including test-only
  dependencies" is 114 packages / 232,126 lines (`dependency-results.json` `pinned.port_all_package_tests`).
- Third-party test imports: `stretchr/testify` in **438 test files across 40 packages**; `nats-io/nats-server/v2`
  (embedded broker) in 9 files (`internal/maxdelivery/runtime_integration_test.go:285,320,338` incl. an authorized
  server and a three-node cluster; `natsclient/client_connect_test.go:104`; `lifecycle_owner_test.go` in
  `graph-clustering:52`, `graph-embedding:58`, `graph-index-spatial:36`, `graph-index-temporal:35`, `graph-query:51`;
  `graph-index/failed_start_subscription_test.go:264`; `service/stream_override_expiry_test.go:154`);
  `pgregory.net/rapid` 5 files / 5 packages; `testcontainers-go` 3 files (natsclient); `gojsonschema` 1. SemEngine's
  `go.mod` has testify only as `// indirect` (`go.mod:67`), no nats-server, no rapid.
- The pin's own NATS test fixture, `natsclient.NewTestClient(t, opts…)` (`natsclient/test_client.go:854`; options
  `:440-522` incl. `WithFileStorage` `:512`, `WithBucketPrefix` `:522`), is called **278 times** in tier-0 tests across
  21 packages: natsclient's own tests 69, and 209 in the other 20 (config 41, graph-index 26, graph-ingest 21, rule 20,
  websocket 18, clustering 18, service 18, embedding 12, graph 11, lifecycle 7, component 3, spatial 3, temporal 3,
  objectstore 2, graph/embedding 1, maxdelivery 1, pkg/dispatch 1, fusionnats 1, graphview 1, graph-query 1).
  `natsclient.StartTestNATS` 0; fixed broker addresses in tier-0 tests 0.

### §1.6 Production third-party imports the tier-0 set carries (per importing package)

`nats.go` (9) and `nats.go/jetstream` (22); `prometheus/client_golang` (22) + `collectors`, `promhttp` (metric) +
`client_model` (service); `google/uuid` (graph/inference, message, pkg/lifecycle, objectstore); `gorilla/websocket`
(output/websocket only); `robfig/cron/v3` (processor/rule only); `go-acme/lego/v4` ×6 paths (pkg/acme only);
`x/net/http2` (model); `x/sync/errgroup` (pkg/fusion), `x/sync/singleflight` (graph/embedding); `sashabaranov/go-openai`
(graph/embedding — `http_embedder.go` only, leaves with #35); `testcontainers-go` ×2 + `docker/go-connections/nat`
(natsclient — `test_client.go` only); `stretchr/testify` ×2 (component — `lifecycle_test_suite.go` only). Pin
`go.mod`: nats.go `v1.52.0`; SemEngine `go.mod:11` has `v1.54.0` (a behavioral-pin difference the plan names,
`setup-plan.md:235-236`).

## §2. Ceiling confirmation (#9 item 2)

```text
cd scratchpad/semstreams-pin
sum of tier0-lines.txt (65 packages)                       → 141,062   (= pass3 set H 140,911 + composition/cli 151)
wc -l graph/embedding/http_embedder.go                     → 220
141,062 − 220                                              → 140,842   (= D4)
for p in <65>: grep -l sashabaranov/go-openai $p/*.go | grep -v _test
                                                           → graph/embedding/http_embedder.go; model/registry.go
grep -n go-openai model/registry.go                        → :347 a comment, not an import
go list -f '{{join .Imports "\n"}}' ./composition/cli | grep c360studio
                                                           → component, composition, config
```

**Result: 65 packages / 140,842 lines, delta 0 against D4.** Compiler-side cross-check at the pin:

```text
go list -mod=mod -deps <the 16 roots of §1.2> | grep c360studio/semstreams | sort -u | wc -l   → 74   (deps16.txt)
comm -23 deps16.txt tier0-set.txt → agentic agentic/agentrun governance graph/llm internal/agentterminal
                                     internal/deliverylane internal/looptoken model/wire vocabulary/agentic   (9)
comm -13 deps16.txt tier0-set.txt → (nothing: no tier-0 package is missing from the compiler's closure)
```

74 = 65 + exactly the nine behind-the-seam packages reached through the eight §1.3 edges. The 140,842 figure
remains a reachability cut (pass3 §4) only in that those nine are subtracted by ruling rather than by a seam that
exists at the pin; a `go list -deps` on a SemEngine tree with the seam severed as ruled would return the 65 (the
`graph/llm` split fork (b) would add 477 lines, D4 "design options").

Cross-check against 03A: `dependency-results.json` `pinned.port_production` = 65 / 126,926 with 28 roots; the
set difference against the ruled set is exactly D4's ten-out (`agentic agentic/agentrun gateway gateway/graph-gateway
graph/llm internal/{agentterminal,deliverylane,looptoken} model/wire vocabulary/agentic`) and ten-in
(`composition/cli graph/geo/geojson internal/maxdelivery output/websocket processor/graph-clustering
processor/graph-index-{spatial,temporal} processor/rule{,/expression} vocabulary/export`).

## §3. Harness surface in this repository (what the first change touches)

### §3.1 Exported symbols (`go doc -all`, base `38b184b`)

- `internal/harness/natsfixture` (1,501 non-test lines + 1,270 test; 2,771 total): `New(t testing.TB) *Fixture`
  (`fixture.go:72`); `(*Fixture) Start(ctx) error` `:88`, `Stop(ctx) error` (`stop.go`), `URL() string`, `JetStream()
  jetstream.JetStream`, `Name(base) string` (`names.go`), `CreateStream(ctx, name, subjects…) (jetstream.Stream, error)`,
  `CreateKeyValue(ctx, bucket) (jetstream.KeyValue, error)`, `Consume(ctx, stream, base, handler) (jetstream.Consumer,
  error)`; `CheckName(name) error`; `type Phase string` with consts `PhaseImage … PhaseConsume` (`errors.go:18-30`);
  `type Error{Attempt, Phase, ContainerID, ParentErr, Cause, Cleanup}` (`errors.go:35-44`) with `Unwrap() []error`;
  `ErrAlreadyUsed`, `ErrNotAdmitted`. The `Fixture` struct (`fixture.go:43-66`) holds `container
  testcontainers.Container`, `containerID`, `url`, `nc *nats.Conn`, `js`, `consumers`, `streams`, `buckets`, `calls`,
  `rec` (`:67`); all unexported. The Docker/NATS operations are an unexported `deps` struct of function hooks (`deps.go:27-38`:
  `start host mappedPort connect jsReady createStream createKV terminate drain logs absent`), the fixture's own
  fault-injection seam, driven from `fixture_integration_test.go`, `admission_test.go` and `fixture_test.go`. The
  injection helper is `failAfter[T](real, err)` ("failpoints wrap a real dependency: fail returns an error after the
  real call, so the resource the call creates exists; block runs the real call, then parks until released",
  `fixture_integration_test.go:166-173`), used by S1-1 `TestS1_1PartialStartupCleanup` (`:176-201`). The same test
  file shells out to the `docker` CLI (`exec.Command("docker", "inspect", …)` `:37`; `docker ps -aq --filter label=`
  `:154`). **No `Restart`, `Pause`, `Kill` or container accessor on `Fixture`**; the only container-terminating path is
  `deps.terminate` (`fixture.go:364`, `stop.go:125`). Search, as run on the base:

  ```text
  git grep -n -i -E 'Restart\(|container\.(Stop|Start)|StopContainer|SIGKILL|os\.Process' -- internal/harness scripts Taskfile.yml
  natsfixture/admission.go:72          (comment: a SIGKILLed runner never runs its EXIT trap)
  natsfixture/admission_test.go:125    (comment, same fact)
  natsfixture/fixture_integration_test.go:509  func TestS1_7Restart(t *testing.T)   (the owner's own second Start, refused)
  runner/runner_test.go:398,614,697,780        syscall.Kill(…, syscall.SIGKILL)
  ```

  The `terminate` calls do not match that pattern; they were found by `grep -n terminate`.
- `internal/harness/lifecycletest` (337 + 405 test): `Owner interface{ Start(ctx) error; Stop(ctx) error; Observer }`
  (`lifecycletest.go:36-40`); `Observer{ Observe() Observation }` `:43`; `Observation{Unresolved []string; Calls
  map[string]int}` `:49`; `Promise{Restart bool}` `:55`; `Factory func() Owner` `:62`; `Run(t, factory, promise)` `:68`;
  seven `Check*` functions (`CheckNilContextsRefused`, `CheckPreCancelledStartRefused`, `CheckStopBeforeStartSafe`,
  `CheckControlledStopUnderLiveStartAuthority`, `CheckAbortStopPreservesCause`, `CheckRepeatedStopIsNoOp`,
  `CheckSecondStartRefusedOrRestartCycle`). `Observe()` is required at compile time (`lifecycle-suite/spec.md:9-11`).
  The pin's component contract is `LifecycleComponent{Discoverable; Initialize() error; Start(ctx) error; Stop(ctx)
  error}` (`component/lifecycle.go:63-68`) — no `Observe`; every ported owner needs an adapter to be driven by `Run`.
  Failpoints exist only as the in-package test double: `type failpoint string` with nine values
  (`refowner_test.go:14-26`), exercised by `TestEachFailpointTripsExactlyItsCheck` (made deterministic by PR #43,
  merged `d303c51`, closing #40). No exported failpoint or injection surface: `git grep -n -i failpoint --
  internal/harness` → `lifecycletest.go:5` (a comment), 20 lines in `refowner_test.go` (the type, nine consts, the
  double, `TestEachFailpointTripsExactlyItsCheck`), and `natsfixture/fixture_integration_test.go:166` (the `failAfter`
  comment above).
- `internal/harness/probe` (160 + 190 test): `Await[T](ctx, observe, done) (T, error)`; `Callback{Block(ctx),
  Entered(), Joined(), Release(), ContextErrAtRelease()}`, `NewCallback()`; `ObservedContext`, `Observe(parent)`,
  `ObserveAndHold(parent, proceed)`.
- `internal/harness/contract` (test-only; `doc.go` 7 lines): T-B1 `TestImportGraph` (`imports_test.go:16`;
  `forbiddenInProduction` `:52-60` = `internal/harness`, `testcontainers-go`, `testing`, `gopkg.in/yaml.v3`), T-B2
  `TestNoRetainedContext` (`context_test.go:22`), `TestOneImagePin` (`imagepin_test.go:13`), `TestNoBroadDockerCleanup`
  and `TestSemEngineAssignedNames` (`docker_test.go:18,72`), `TestNoFixedAddressesInTests` + `TestLintTestPortsPasses`
  (`addresses_test.go:21,48`), `TestCleanupRootsCheckSensitivity` (`cleanuproots_test.go:18`),
  `TestCoverCheckSensitivity` + `TestTreeStateSeesUntrackedContent` (`cover_test.go:37,86`), T-B7 `TestAdmissionLedger`
  - `TestAdmissionLedgerSchemaSensitivity` (`ledger_test.go:17,63`; `ledgerFields` closes the ten-field schema, D4a).
  Each guard has a paired `…Sensitivity` test. **Absent** (the claimed gap, #9 item 9 / I8 / T-B8): `git grep -n
  'c360studio/semstreams' -- '*.go'` → nothing; `git grep -n -i -E 'aggregator|component famil|T-B8' -- '*.go'` →
  nothing.
- `internal/harness/runner` (tests of `scripts/test-integration.sh`; `runner_test.go` 829 lines).
- A subprocess host exists, test-only, in `internal/harness/runner/runner_test.go`: `(h *harness) command(ctx, args…)
  *exec.Cmd` builds `exec.CommandContext(ctx, "bash", …/scripts/test-integration.sh, …)` (`:175-176`); it signals
  SIGTERM (`cmd.Process.Signal(syscall.SIGTERM)` `:378,615,698`), kills (`cmd.Process.Kill()` `:371,585,711,793`),
  runs a child in its own process group and signals the group (`SysProcAttr{Setpgid: true}` `:639`,
  `syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)` `:641`), probes liveness (`pidAlive` `:269`), and SIGKILLs
  surviving grandchildren (`:398,614,697,780`). It hosts the runner script, not an engine process. Search, as run:

  ```text
  git grep -n 'os/exec' -- internal/harness
  contract/addresses_test.go:5   contract/cleanuproots_test.go:4   contract/cover_test.go:5   contract/repo_test.go:7
  natsfixture/admission_test.go:7   natsfixture/fixture_integration_test.go:11   runner/runner_test.go:9
  ```

  (7 files; the four contract tests drive `bash`/`git`, the two natsfixture tests drive `docker` and the runner,
  `runner_test.go` is the process host.)

### §3.2 Gates and scripts (`Taskfile.yml`, `scripts/`)

- `task verify` = `scripts/verify.sh:10-11` steps, cheapest first: `spec:check docs:check fmt:check tidy:check
  cleanup-roots:check build vet lint vuln ledger:check test:unit test:integration cover:check`; fails if tracked files
  change (`:33-35`). `AGENTS.md:43-44` lists `spec:check docs:check fmt:check tidy:check build vet lint vuln
  test:unit` and omits `cleanup-roots:check`, `ledger:check`, `test:integration`, `cover:check`: the root file is
  stale against the script. CI runs `task doctor` then `task verify` in job `verify` (`.github/workflows/ci.yml:46,50`),
  with
  `required` needing it (`:67-70`).
- `build`, `vet`, `lint`, `test:unit` go through `scripts/gopkgs.sh` (`Taskfile.yml:41,46,51,74`), which runs `go list
  ./...` first and states "0 package(s) … nothing to run" on an empty module; today 5 packages.
- `task lint` = pinned revive 1.15.0 over `./...` incl. test files (`Taskfile.yml:51`; `revive.toml` has no
  test exclusion), `function-length [80, 0]` (`revive.toml:36-37`), 25 rules, warnings fail (`:7-8`); then
  `scripts/lint-test-ports.sh` and its fixture test (carried byte-identical, ledger rows).
- `scripts/cleanup-roots-check.sh:15-16`: `git grep -n --untracked -E '\.(Stop|Close|Terminate)\(.*context\.(Background|
  TODO)\(\)' -- '*_test.go'`, zero baseline, every hit fails.
- `scripts/cover-check.sh`: `threshold=80` (`:14`), `base=…/internal/harness` (`:15`), targets hard-coded as three
  `check <pkg> <profile> <lane>` lines (`:73-75`: lifecycletest unit, probe unit, natsfixture integration); the
  integration profile is the last `.evidence/last-run` whose `runner.env` says `go_test_status=0` (`:24-30`). Adding a
  critical package (#9 item 7) means editing `base` and the `check` list.
- `scripts/test-integration.sh`: `go test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m`
  (`integration-test-runner/spec.md:76-77`), host lock `/tmp/semstreams-integration.lock` shared with SemStreams, process
  group ownership, session-label leak check. `natsfixture.Start` refuses outside it (`ErrNotAdmitted`).
- `task ledger:check` = `go test -run '^TestAdmissionLedger' ./internal/harness/contract/` (`Taskfile.yml:64`).
- `task spec:check` = `openspec validate --all --strict`; `task spec:queue` reads holds (`scripts/openspec-queue.sh:80-85`:
  an unchecked task line containing `hold`/`blocked`/`blocking` is `BLOCKED`).
- `docs/testing.md` is **not on this base** (`ls docs/testing.md` → No such file); PR #39 adds it (200 lines at
  `74db5cc9`). It documents the three levels, the helper packages, the structural guards, the "show the test can fail"
  procedure, fuzz/property guidance ("No property-testing library is in `go.mod` today", `:149-150`), and the
  concurrency/cleanup rules.

### §3.3 What the ruled gates measure against the tier-0 set at the pin

These are the first change's collisions.

- **T-B1** (production files importing `testing`/`testcontainers`/`yaml.v3`): 4 files in the set:
  `component/lifecycle_test_suite.go:11`, `composition/assert.go:4`, `natsclient/test_client.go:10,16,17`,
  `payloadregistry/testing.go:4` (grep over the 65, non-test). Two already have ledger rows (`admission-ledger.yaml:25,
  73`, both `adapt`, destinations in `internal/harness/`); `composition/assert.go` and `payloadregistry/testing.go` have
  none.
- **T-B2** (retained context): `grep -nE '^\s+\w+\s+context\.Context\s*(//.*)?$'` over tier-0 non-test files → 0
  (a grep, not the type-checked scan; embedded/alias/container shapes are unmeasured here).
- **Context roots** (`context.Background()`/`TODO()` in non-test files): 34 — natsclient 9, component 5, service 3,
  pkg/worker 2, pkg/errs 2, pkg/buffer 2, metric 2, graph/query 2, config 2, output/websocket 1, objectstore 1,
  pkg/dispatch 1, pkg/cache 1, model 1; **0 in `processor/rule`, `pkg/lifecycle`, `graph/clustering`,
  `graph/inference`, `processor/graph-*`, `internal/maxdelivery`** (A8's 34 had graph-gateway 1 in, websocket 1 out).
- **cleanup-roots guard** (SemEngine's zero-baseline pattern over the pin's `*_test.go`): **274 hits in 15
  packages** — service 92, graph-index 42, output/websocket 30, graph-clustering 22, graph-embedding 18, pkg/dispatch
  15, rule 10, spatial 10, temporal 10, objectstore 9, natsclient 6, graph-ingest 4, maxdelivery 4, lifecycle 1,
  graph-query 1 (sample: `service/base_lifecycle_test.go:19` `require.NoError(t, s.Stop(context.Background()))`).
- **Cleanup-baseline entries, D11's unit** (#9 item 8 asks in this unit): the pin's
  `test/testinfra/cleanup_baseline.json` (234 entries, `identity` keyed `<file>|<test>|…`), counted by the entry's
  package: **117 in the 65** — graph-index 27, output/websocket 22, service 20, graph-clustering 16, graph-embedding 13,
  spatial 5, temporal 5, pkg/dispatch 4, objectstore 4, pkg/lifecycle 1. The ten entering packages carry **48**
  (websocket 22, clustering 16, spatial 5, temporal 5; rule, expression, export, geojson, maxdelivery, cli 0); the rest
  is D11's 69 exactly. The 274 and the 117 are two instruments over the same tests (SemEngine's one-line grep vs the
  pin's classified baseline) and are not comparable to each other.
- **`lifecycleUsed` copies** (SS#1411): **12 in the ruled set** — `output/websocket/websocket.go:157`,
  `processor/graph-clustering/component.go:646`, `graph-embedding/component.go:298`, `graph-index-spatial/component.go:
  187`, `graph-index-temporal/component.go:196`, `graph-ingest/component.go:520`, `graph-query/component.go:177`,
  `processor/rule/cron_scheduler.go:54`, `processor/rule/processor.go:102`, `service/component_manager.go:102`,
  `service/message_logger.go:234`, `storage/objectstore/component.go:49`. D11 bound 6 (first-pass set, entering
  packages unmeasured); the ten entering packages add 6. This answers #9 item 8 for `lifecycleUsed`.
- **Start/Stop owners** in tier-0 roots: 23 types (22 once `MilestoneService` is not ported) — the seven processor
  `Component`s, `rule.Processor`, `rule.CronScheduler`, `rule.ConfigManager` (`kv_config_integration.go:117,166`),
  `websocket.Output`, `objectstore.Component`, ten in `service` (`BaseService`, `HeartbeatService`, `ComponentManager`,
  `LogForwarder`, `MetricsForwarder`, `MessageLogger`, `MilestoneService` [not ported], `Metrics`, `Manager`,
  `StorageObservabilityService`), and `config.Manager` with `Stop(timeout time.Duration)` (`config/manager.go:478`; the
  SS#1415 row). These are the owners the lifecycle suite and #38's failed-start check run against.
- **revive** (SemEngine's `revive.toml`, file-list mode per package over the pin; package mode found no files in a
  foreign module): 70 findings, **all in test files**, 0 in production: `context-as-argument` 65, `function-length` 2
  (`graph-query/component_integration_test.go:30` 97 statements; `rule/entity_watcher_integration_test.go:288` 81),
  `empty-block` 2, `unused-parameter` 1; by package graph-index 26, natsclient 13, config 10, graph-ingest 7,
  objectstore 6, rule 3, maxdelivery 3, graph-query 1, fusionnats 1. (284 `package-comments` findings are a
  file-list-mode artifact: every tier-0 package has a `// Package` comment.)
- **Metric names** (D15 drift test scope): 213 distinct `Name: "…"` literals in tier-0 non-test files (a grep upper
  bound; rule 28, metric 19, objectstore 17, graph-ingest 17, graph-embedding 17, graph-query 15, websocket 15, …).
- **Package comments** (D16 package-doc gate; `revive` `package-comments` is already on, `revive.toml:22`): every
  tier-0 package has one.

### §3.4 Rulings that bind the first change's harness work

- **#38** (owner, comment 5934015068): failed-start honesty is part of the lifecycle suite's standard run, designed
  against a real component in 04A; scope starts at "`Start` returns an error and holds nothing"; no readiness accessor
  on `Owner` before a ported component has a readiness surface. Open design questions on the issue: how a factory
  injects a dependency failure without the suite knowing the owner's dependencies; suite check vs per-package
  obligation.
- **#40 / #43**: the flake was in the test double (`refowner_test.go`), fixed on `main` at `d303c51`; routed to #42
  (flake defense, PR #44 in flight): `finalize` not joining the `stopReturnsNilWithWorkerRunning` worker, a hand-written
  failpoint list, a real 2 s per matrix iteration.
- **PR #39** (pending): architect-contract additions — surface audit per ported package, "guidance returns with the
  package", the adoption sweep for establishing changes, and the intent-check table (applied in §7.3).
- **PR #44** (pending): `openspec/changes/flake-defense/` (inventory 1,175 lines, proposal, tasks) — an in-flight
  OpenSpec change on the harness; the first 04A change shares `openspec/changes/` and the harness with it.

## §4. Records the first change must extend

### §4.1 `docs/admission-ledger.yaml` (12 entries, base `38b184b`)

Header `:1-23`: ten fields, four dispositions, port-refactor convention (`:20-23`). Entries: `natsclient/test_client.go`
(adapt → natsfixture), `natsclient/test_options.go` (defer-exclude), `internal/lifecyclecleanup/lifecyclecleanup.go`
(adapt → natsfixture unexported helper; `known_risks:` "production home deferred until a production consumer exists
(04A)", `:68`), `component/lifecycle_test_suite.go` (adapt → lifecycletest), `processor/graph-query/
lifecycle_owner_test.go` (adapt → probe), `scripts/run-integration-tests.sh` (adapt), `test/contract/
context_ownership_contract_test.go` (adapt), `test/testinfra/integration_runner_contract_test.go` (defer-exclude),
`scripts/lint-test-ports.sh` + fixture (carry), `testutil/nats.go` (defer-exclude), `test/testinfra/
cleanup_guard_test.go` (defer-exclude; "revisit at 04A", `:187-188`). All at `source_sha
5457b3458936f668b71d2fea061f67f8d7d01e67` (the audit snapshot, not the pin); `source_path` is unique per entry
(T-B7), so a 04A row for `natsclient` or `component` as a package would be a second row keyed by a different path
(`natsclient` vs `natsclient/test_client.go`). Three tier-0 sources already have rows whose destination is the
harness, and `internal/lifecyclecleanup` is both a harness helper (row) and a tier-0 package (D4 carry) — two homes
for one unit. Package rows: none (#9 item 1: 65 + separated packages).

### §4.2 Spec deltas drafted in 03B `design.md:869-1023`

`graph-transport-boundary` (new, #16; 2 requirements, 3 scenarios), `graph-ingest-recovery` (new; #15, settlement,
recovery per storage class; 4 requirements, 6 scenarios), `config-desired-state` (new, #17; 1 requirement, 2
scenarios), `projection-mutation` (new; #19, #20; 2 requirements, 4 scenarios), `harness-boundaries` (MODIFIED
"Import graph": T-B8 aggregator rule + I8 no-SemStreams-import; 3 scenarios). I10 (rule core imports) is "drafted by
the 04A change that ports the rule core" (`design.md:791-792`), not drafted. `openspec/specs/` holds only the four
SETUP 02 capabilities (`harness-boundaries` 122 lines, `integration-test-runner` 100, `lifecycle-suite` 43,
`nats-fixture` 98); no graph capability spec exists; `openspec/changes/` has no active change on this base
(`.gitkeep` only; PR #44 adds `flake-defense`).

### §4.3 Repair rows and port refactors

- D11 rows (`design.md:478-491`): #15, #16, #17, #20, #19, settlement, SS#1411, SS#1415, SS#1218, SS#1220, SS#1417,
  SS#1145/#1147. Issues #15, #16, #17 are open, `status:blocked`, milestone Slice 04A. #18, #19, #20 are open
  decision issues (milestone "Setup: foundation through contract").
- Port refactors #25–#36, all open, label `class:port-refactor`, milestone Slice 04A: E1–E4 (#25–#28), `component`
  (#29), `service` (#30), `graph-query` (#31), capability seam (#32), D1 adopter path (#33; refusal text at
  `processor/graph-ingest/component.go:739` confirmed in the snapshot), `output/websocket` (#34), `graph/embedding`
  split (#35), `internal/maxdelivery` (#36).
- Epic #24 (durable-execution primitive) open, `status:blocked`, milestone Slice 04A, "after #9's first green
  extraction" (D13).
- `#9` items 1–9 (body, "Carried to 04A"): ledger rows; ceiling confirmation (§2 here); proving tests per row;
  SemConnect observed column (§5.2); DX gates; harness extension as first task of the first change; coverage targets
  (§3.2 `cover-check.sh` shape); unmeasured debt figures (§3.3: 48 baseline entries for the ten entering packages, 117
  in the 65; 12 `lifecycleUsed`; 274 by SemEngine's own guard); spec deltas
  with the I8 test landing alongside the `harness-boundaries` delta.

### §4.4 Where the pin composes what no consumer composes

These are D13's open composition points.

`internal/boot/run.go` at the pin: `maxdelivery.Start(runtimeCtx, natsClient, metricsRegistry, logger)` `:184`;
`lifecycle.NewManager(natsClient, logger)` `:220`; `agentrun.Register(svcDeps.LifecycleManager)` `:299`;
`service.ConfigureRulePackMutations(manager)` `:330`; `registerRuleConfigService(…)` `:339`; `service.RegisterAll`
`:492`; `service.NewMilestoneService` `:573`; `payloadbuiltins.Register(reg)` `:744`. None of the four consumers uses
`internal/boot` (scope Q1.4, Q4.3).

## §5. Adopter seams reached from outside this repository

### §5.1 SemSource (first-wave consumer; the integration branch the plan requires)

**State.** `main` `e4febc0d` pins the SemStreams pin. PR #223 (merged 2026-10-01 at `e4febc0d`) **removed**
`internal/sourcelifecycle` and `internal/sourceintent` (`gh api …/contents/internal/sourcelifecycle?ref=e4febc0d` →
404; same for `sourceintent`). PR #223's body, verbatim: "removes 3,426 net production Go lines"; "The private
journal, coordinator, publication receipts, seed proofs, effect fences and replay" are the subsystem it removes;
replies "explicitly report `projection_status:"unavailable"`"; the lifecycle status endpoint "returns HTTP 410,
`SOURCE_LIFECYCLE_UNAVAILABLE`"; "SemEngine #18–20 must be reconsidered as separate framework contract questions, not
prerequisites for this removed design". Draft PR #222 (`codex/semengine-tier0-qualification`, head `533b62a7`,
planning only; 0 conversation comments, 0 reviews) is the SemSource owner's tier-0 qualification plan. From **PR #222's
body** (`gh pr view 222 --json body`), verbatim: "The private journal, coordinator, receipts, fences and replay have
been removed"; "The private journal, replay, publication receipts and effect fences must not be ported. SemEngine
\#18–#20 remain separately justified contract decisions"; "Qualification uses SemEngine exclusively on this separate
branch; mainline remains on SemStreams until full semembed-backed 04C passes"; implementation waits for "an exact
usable admitted Tier 0 source commit under SemEngine #9". From its `design.md` @ `533b62a7`: "Port consumer bindings
and tests exclusively to admitted SemEngine contracts. The qualification binary must import no SemStreams packages,
not merely avoid starting them" (`:142-144`).

**What SemSource composes from the port set** (`cmd/semsource/run.go` @ `e4febc0d`, 1,313 lines):
imports `component config(semconfig) metric natsclient payloadregistry service types` + the two aggregators
`componentregistry`, `payloadbuiltins` (`:47-55`); `componentregistry.Register(registry)` `:295`;
`payloadbuiltins.Register(reg)` `:311` then its own `graph.RegisterPayloads` `:314`, `sourcemanifest.RegisterPayloads`
`:317`; `semconfig.NewConfigManager` `:156`, `NewStreamsManager(...).EnsureStreams` `:261-262`;
`service.NewServiceRegistry` / `RegisterAll` / `NewServiceManager` `:371-376`. Components: `graph-ingest` `:759`
(request port type `semstreams.graph.mutation` `:788`), `graph-index` `:800`, `graph-embedding` `:812` with
`"embedder_type": embedderType` `:817` (default `"bm25"`, `:724`), `graph-query` `:842`, `graph-gateway` `:867`
(requesters `graph.query` `:858,884`, `agentic.query` `:900`), `objectstore` `:917`, `graph-clustering` only under
`enableClustering` `:947-952`, `websocket` `:1075` with input `JetStreamPort{StreamName: "GRAPH", Subjects:
["graph.ingest.>"]}` `:1058`, `delivery_mode: at-most-once` `:1069`. GRAPH stream `Storage: "memory"`, 256 MiB, 1h,
`DiscardNew` `:984-1003`.

**Production import set** at `e4febc0d` (`grep -rhoE` over non-test `.go`, 25 distinct — unchanged from D16's 25):
component 45 files, message 28, vocabulary 20, natsclient 19, storage 14, types 7, pkg/fusion 7, pkg/projection 6,
graph 6, config 6, pkg/types 5, metric 5, storage/objectstore 4, pkg/retry 4, pkg/errs 3, payloadregistry 3,
storage/storeregistry 2, vocabulary/cco 1, service 1, pkg/fusion/fusionvocab 1, pkg/fusion/fusionnats 1, pkg/buffer 1
(`internal/entitypub/publisher.go`: `buffer.Buffer`, `WithOverflowPolicy`, `WithMetrics`), payloadbuiltins 1, model 1
(`config/config.go`: `model.Registry`, `model.Capability{Embedding,Summarization,QueryClassification,
IntentClassification,AnswerSynthesis,AnomalyReview,CommunitySummary}`, `model.EndpointConfig`), componentregistry 1.
Test-only additions: `graph/readiness`, `processor/graph-{index,ingest,query}`.

**No-embedder baseline.** `configs/tiers/tier0-statistical.json` @ `e4febc0d` (12 lines): `"graph": {"embedder_type":
"bm25", "index_workers": 4, "coalesce_ms": 200}`; no provider. Under it SemSource still composes `graph-embedding`
(BM25). The SemSource owner's seam table (`PR #222 design.md:25-36`): composition "BM25 and graph-embedding always
enabled"; graph config "bm25/http; no disable selector"; validation "HTTP requires embedding capability"; registries
"full framework registrations"; gateway "graph/index and agentic requester declarations"; fusion "requires NATS, body
resolver and readiness"; query verbs "all code/doc verbs exposed in every profile"; removal "disabled envelope defeats
file overlay"; corpus selector "accepts only bm25/neural"; corpus Stop "joins child without asserting wait error". And
`:38-40`: "No current `embedder_type: none` or foundation-profile flag is claimed. Omitting a provider leaves the BM25
embedding component composed. Turning off an optional component also cannot remove imports pulled in by registries."

**Framework-side readiness fact** bearing on the "no-embedder readiness" seam: `pkg/fusion/fusionnats/client.go:139`
builds its watcher on `readiness.KeyGraphIndex` only; `graph/readiness/watcher.go:52-62` declares `KeyGraphIndex`,
`KeyGraphEmbedding`, `KeyGraphIngest`, `KeyRule`. Fusion does not wait on the embedding key; the aggregate wait PR #222
names is in SemSource's own oracle (`design.md:106-110`).

**Open consumer asks on the 04A contract** (PR #222 `design.md:44-60, 85-104`): a public exact document/passage
retrieval path without NL (`*_doc_exact_passage_content` today goes through `doc-context/context` with a phrase query,
`:100-104`); a readiness contract that works without an embedding component; placement of the SemConnect cases and
the dogfood workload; #15's two broker-reingestion assertions "mandatory 04A … never skipped" (`:97-98`, `:118-120`);
`Enabled:false` retained until #17's repair is proved (`:128-131`).

**Subprocess and broker-restart shape SemSource already owns** (`test/setup03a/qualification_test.go` @ `75a17f7d`):
`exec.Command(h.binary, "run", …)` `:404`; `(h *harness) start()` `:396`, `stop(force bool)` `:428` (signal `:438`,
`Process.Kill` `:446`); `docker logs` / `docker rm -f` `:468-473`; `SIGSTOP` `:829`; `docker restart --time 5` `:880`.
PR #222 names it "checked process exit evidence" to reuse or adapt (`design.md:133-135`).

**The adopter-seam questions** (contract §"adopter seam inventory"), answered as a SemSource developer who has not
opened the SemEngine tree:

1. *Must know:* which per-package `Register`/`RegisterPayloads` calls replace the two aggregators (D1; today the error
   text at `graph-ingest/component.go:739` names `payloadbuiltins.Register`); that `graph-embedding` must be omitted
   for a no-embedder composition and that nothing refuses `embedder_type` without it; that readiness has one key per
   producer and which one fusion waits on; that GRAPH on a memory stream loses accepted-but-unapplied messages at
   broker restart (I7); the reserved request subjects (#16); that `Reconcile` fences on its own read unless
   `ExpectedRevision` is sent (#19); that a classified error can hide an uncertain commit (#20) until repaired.
2. *If they do nothing:* they import both aggregators (and their closure, 94 and 41 packages at the pin; A12.1), keep
   `graph-embedding` composed with BM25, and read "not-committed" for an effect-uncertain write. None is a compile
   error today.
3. *Found out at:* compile time only once `payloadbuiltins` does not exist in SemEngine (D1 makes the aggregator
   absence a compile error; the `:739` text is still a boot error naming a missing symbol until #33); the rest at
   log/next-boot/nowhere (A12 seams 3–6; D11/D7/D8 are the ruled fixes).
4. *Should know:* the gap list is 03B's "Adopter seam findings" (`design.md:794-802`) plus the two new facts above (the
   no-embedder composition has no refusal/selector, and the exact-document path has no non-NL public operation).

### §5.2 SemConnect (first-wave consumer; PR #74)

PR #74 head `dff12657` (draft, "qualification blocked"): three reference cases pass at both pins (`TestSetup03AReference
{SensorML,TypedMutations,ImmutableArtifact}`, `gateway/cs-api/setup03a_reference{,_integration}_test.go`), the
federated-create case fails at the target with HTTP 400 `authority_foreign` (the ADR-102 d5 fence D9 keeps);
"combined framework closure is 63→67 production packages and 99→113 including retained-package tests"; "the cases
use fresh **embedded NATS**, real graph-ingest and ObjectStore" (`semengine-reference-cases.md:28`). The external OGC
suite is 137/0/0 at both pins (`:87`). Composition root `cmd/cs-graph-backend/main.go` @ `dff12657` (352 lines)
already registers **per package**: `graphingest.Register, graphindex.Register, graphspatial.Register,
graphtemporal.Register, graphquery.Register, objectstore.Register` (`:126-127`) — the D1 shape — while still calling
`builtins.Register()` (`:110,:123`, `vocabulary/builtins`) and `payloadbuiltins.Register(payloads)` (`:228`), and
importing `composition` directly (`:20`), `config` (`:21`), `graph` (`:22`), `pkg/types` (`:27`), `types` (`:35`).
The matrix cells marked `pending #74` (D9, `design.md:682,684,686,704,731`) can be filled from the PR body and
`qualification.md`; nothing newer than `dff12657` exists (PR `updatedAt` 2026-10-01T10:29Z).

### §5.3 semboids and semteams

Not re-read (scope rule 3: one inventory per question). Cited from 03B scope Q3: semboids `8c03cc53` composes
`graph-ingest`, `graph-index`, `graph-clustering`, `processor/rule`, `output/websocket`, uses `pkg/lifecycle`
(`cmd/semboids/main.go:177-178,190`) and `pkg/graphview` (5 files), `payloadbuiltins.Register` (`main.go:163`), file
stream `ENTITY` 24h/2 GiB (`flock.json:17-25`); semteams `ce22c961` imports `engine`, `flowstore`, `flowtemplate`
(absent at the pin) and reaches rules through `componentregistry.Register` (`main.go:777`).

## §6. Same-class collision table

Triggered by the harness extension (#9 item 6), which would introduce three test-time runtime-coordination
primitives. The semantic job is named first; owners are listed wherever the job is already done.

| Dimension | A. Failpoint injection into a component under test | B. Subprocess host (process kill / replacement) | C. Broker restart / loss under a test |
| --- | --- | --- | --- |
| Semantic class | make a dependency or step fail at a chosen point, observably, from outside the component | run the engine (or one component) as a separate OS process and kill/restart it while the broker persists | stop and restart the NATS server a test owns, keeping or discarding its state |
| Owners (SemEngine) | `lifecycletest` failpoint double (`refowner_test.go:14-26`, in-package, test-only); `natsfixture.deps` hooks (`deps.go:27-38`, unexported, Docker/NATS ops) with the `failAfter[T]`/block wrappers that fail or park a real dependency after its real call (`fixture_integration_test.go:166-173`; S1-1 `:176-201`) | `runner/runner_test.go` harness: `exec.CommandContext` of the runner script (`:175-176`), SIGTERM (`:378,615,698`), `Process.Kill` (`:371,585,711,793`), own process group + group SIGINT (`:639-641`), `pidAlive` (`:269`), grandchild SIGKILL (`:398,614,697,780`); test-only, hosts the runner, not an engine | none on `Fixture`: `deps.terminate` only (`fixture.go:364`, `stop.go:125`); `TestS1_7Restart` (`fixture_integration_test.go:509`) is the owner's own second Start, not the broker's; the same file already drives the `docker` CLI (`inspect` `:37`, `ps` `:154`), the mechanism a `docker restart` would use |
| Owners (pin, port set) | **KV fault injection at the `jetstream.KeyValue` interface**: `natsclient.KVStore{bucket jetstream.KeyValue; …}` (`natsclient/kv.go:48-52`), `func (c *Client) NewKVStore(bucket jetstream.KeyValue, opts…) *KVStore` (`:55`), `KVStore.Update` → `kv.bucket.Update` (`:232-236`); graph-ingest's own double `type mockKVBucket` with `putFunc createFunc getFunc deleteFunc listFilteredFunc watchAllFactory` hooks (`processor/graph-ingest/component_test.go:28-36`; `Update` `:122`), injected by in-package field assignment `component.entityBucket = natsClient.NewKVStore(mockBucket)` (`component_test.go:1030`), `c.entityBucket = deps.NATSClient.NewKVStore(newMockKVBucket())` (`factory_registry_test.go:49`), `component.entityBucket = component.natsClient.NewKVStore(entityBucket, …)` (`keyed_ingest_test.go:315`; guard bucket `:188,263`), `merge_entity_bench_test.go:119`; `NewKVStore(` appears in tier-0 test files of 6 packages (files: natsclient 7, graph-index 8, graph-ingest 5, rule 2, config 1, service 1). Unexported func fields in graph-ingest: `waitForStreamInput`, `consumeStream` (`component.go:497-498`), `statusNowFn` `:558`, `repopulateHook` `:589` ("inject a concurrent invalidation … Production is nil", `:588`; `query.go:734`); `component/lifecycle_test_suite.go` `ErrorInjectingComponent` (dropped by the SETUP 02 ledger row, `admission-ledger.yaml:84`) | `test/e2e/harness/processbarrier` (agentic-only, `processbarrier.go:1-6`); no process kill in any port-set test (scope Q4.4) | `natsclient/test_client.go` has no restart (`grep -n -i -E 'func \(.*\) (Restart\|Kill\|Pause)' natsclient/test_client.go` → none); 9 test files run an embedded `nats-server/v2` (§1.5), which can be stopped in-process |
| Owners (consumers) | — | SemSource `qualification_test.go:396-473` (`exec.Command`, signal, `Process.Kill`) | SemSource `qualification_test.go:829,880` (`SIGSTOP` app, `docker restart --time 5`); SemConnect cases use embedded NATS (`semengine-reference-cases.md:28`) |
| Catalogs | the nine failpoint names (`refowner_test.go:17-26`); `Observation.Calls` keys (`lifecycletest.go:49-52`); `natsfixture.Phase` names the phase each `failAfter` targets (S1-1 iterates them); `mockKVBucket`'s six hook fields (`component_test.go:31-36`) | the runner's `runner.env` keys and exit codes (`integration-test-runner/spec.md:36-39`) | `natsfixture.Phase` consts (`errors.go:22-30`): no restart phase |
| Status | `Observation{Unresolved, Calls}`; `natsfixture.Error{Phase,…}`; a `mockKVBucket` hook returns the error the test chose (no status of its own) | SemSource: exit status, signal observed (`:438-446`); runner tests: exit status 143 on TERM, `log_incomplete`, `int_ignored_on_entry` (`integration-test-runner/spec.md:35-39`) | fixture `rec` evidence record (`fixture.go:67`; `evidence.go`) |
| Lifecycle | failpoint double: per-check fresh owner (`Factory`); `mockKVBucket` lives for one test and is assigned before `Start` or in place of `initStorage` | SemSource: `start`/`stop(force)`; runner spec and `runner_test.go`: process group, TERM, KILL after grace, reap, leak check (`integration-test-runner/spec.md:32-34`) | fixture Stop order: consumers → streams/buckets → drain → terminate → observe absent (`nats-fixture/spec.md:60-62`) |
| Ownership | the test owns the double; the pin's hooks and the `mockKVBucket` assignment are in-package (unexported field), so only graph-ingest's own tests can inject today | `runner_test.go` owns the processes it starts (TERM, KILL, group signal, grandchild reap); nothing owns a child engine process | one fixture = one container (`docs/testing.md@74db5cc9:178`); `Start` after `Start` refused (`ErrAlreadyUsed`) |
| Readers | `Run`; `TestEachFailpointTripsExactlyItsCheck`; S1-1; graph-ingest unit tests through `KVStore` | `runner_test.go` (R1–R4, M4) | fixture owner tests S1-1..S1-9 |
| Writers | the double; `failAfter`; `mockKVBucket` hooks | SemSource harness; `runner_test.go` harness | SemSource harness; `deps.terminate` |
| Recovery | n/a | SemSource `h.start()` re-exec with the same config and `--nats-url` | SemSource: `docker restart` keeps the container's volume; the fixture creates no volumes (`nats-fixture/spec.md:45-46`), so a terminated fixture container has no state to recover |
| Unknowns | how a factory injects a dependency failure without the suite knowing the owner's dependencies (#38). **Measured, not unknown:** graph-ingest's KV `Update` timeout (D8 proving test) is injectable with no code seam — `entityBucket` is a `*natsclient.KVStore` (`component.go:506`) over the `jetstream.KeyValue` interface, built in production from `graph.EnsureCatalogBucket` (`:1193-1198`) and replaced in tests by assigning `NewKVStore(mockKVBucket)` to the unexported field (four sites above). What remains open is only reach: the field is unexported, so the assignment works from graph-ingest's own test package and not from `internal/harness` | whether the engine has a binary to exec (none exists; `docs/repository-map.md:14`) or the host runs a `go test`-built helper process | whether JetStream state must survive the restart (file-stream rows of D11/I7 need it; the fixture's streams are created with its own bounds, `nats-fixture/spec.md:45`) and whether that needs a volume the fixture does not create |

Two further same-class notes for the slicing design (not primitives the harness adds, but collisions a cut meets):

- **Two NATS test fixtures in one tree.** `natsfixture` (SemEngine) and `natsclient.NewTestClient` (pin, 278 call
  sites in tier-0 tests across 21 packages, 69 of them natsclient's own,
  §1.5) own the same job: one disposable real NATS per test.
  The ledger row for
  `natsclient/test_client.go` records "reimplemented … no natsclient.Client" (`admission-ledger.yaml:30-31`) and the
  production import of `testing`/`testcontainers` as a T-B1 violation; the row does not say what happens to its 278
  callers.
- **Two Graphable/triple fixture homes.** `internal/semantictest` (pin, 13 tier-0 test importers) and the plan's
  helper-kit item "valid Graphable, triple, metadata, and content-reference fixtures" (`setup-plan.md:285-287`), which
  SETUP 02 deferred as a non-goal ("types absent; 03B semantics", `setup-02 proposal.md:47`). No SemEngine home exists
  yet (`git grep -n -i graphable -- internal` → nothing).

## §7. Premises as measurable claims (each with its measurement)

### §7.1 Structure and ceiling

1. Tier 0 at the pin is 65 / 140,842 — §2 commands; delta 0 vs D4.
2. The induced import graph has no cycles — `dag.py` Tarjan → `cycles []`.
3. Every processor's closure contains `pkg/lifecycle` — `component → pkg/lifecycle` edge (`component/dependencies.go:12,
   99`); closure(`component`) = 26 / 41,397.
4. `graph` cannot be ported without `natsclient` — `inset-edges.json` `graph: [message natsclient pkg/errs pkg/types
   vocabulary]`.
5. `processor/rule`'s closure is inside the SemSource-minimal composition's closure plus itself — closure(rule) −
   closure(ingest+index+query+objectstore+service) = {`processor/rule`, `processor/rule/expression`}.
6. `graph/embedding` (BM25 library) is reached in 04A through `graph/query` — edges `processor/graph-query → graph/query
   → graph/embedding`.
7. Eight tier-0 packages import a cut package, at the sites in §1.3 — grep over the snapshot.
8. Two packages are reached by no consumer composition: `composition/cli`, `internal/maxdelivery` — union closure
   63 / 140,382 (§1.4). `processor/graph-embedding` is composed by SemSource (`run.go:812-817`) and is in Slice 04B by
   ruling, not by absence of a consumer.
9. The 03A `pinned.port_production` set differs from the ruled set by exactly D4's ten-out/ten-in —
   `dependency-results.json` set difference (§2).

### §7.2 Harness and gates

<!-- markdownlint-disable MD029 -->
<!-- premise numbers continue from §7.1 so that citations ("premise 13") stay stable -->

10. No SemEngine Go file imports `github.com/c360studio/semstreams`, and no test guards it — `git grep -n
    'c360studio/semstreams' -- '*.go'` → nothing (I8 is a claimed gap, confirmed).
11. No aggregator/family guard exists — `git grep -n -i -E 'aggregator|component famil|T-B8' -- '*.go'` → nothing.
12. `natsfixture.Fixture` has no restart, pause, kill or container accessor — §3.1 search output; `TestS1_7Restart` is
    the owner's second Start, and the `docker` CLI is driven from `fixture_integration_test.go:37,154`.
13. The harness has a test-only subprocess host for the runner script (`runner/runner_test.go:175-176,269,371-398,
    585-641,697-793`) and none for an engine process — `git grep -n 'os/exec' -- internal/harness` → 7 files (§3.1).
14. `lifecycletest.Owner` requires `Observe()`; the pin's `LifecycleComponent` does not have it —
    `lifecycletest.go:36-45`; `component/lifecycle.go:63-68`.
15. Four tier-0 production files violate T-B1 — §3.3 grep.
16. 274 tier-0 test lines hit the cleanup-roots guard; 12 `lifecycleUsed` copies; 70 revive findings, all in tests —
    §3.3 commands.
17. 438 tier-0 test files import testify; SemEngine lists testify only as indirect — grep; `go.mod:67`.
18. `cover-check.sh` targets are three hard-coded lines with `base=internal/harness` — `scripts/cover-check.sh:15,73-75`.
19. `docs/testing.md` is absent on this base and present at PR #39's head — `ls`; `gh api …?ref=74db5cc9`.

<!-- markdownlint-enable MD029 -->

### §7.3 Intent check

PR #39 architect-contract addition, applied because slicing orders capabilities.

| Capability named in `AGENTS.md` "What this is for" | Status | Ruling |
| --- | --- | --- |
| ingest, index, query | admitted, tier 0 | D4; #8 comment 5930898291 |
| vocabulary (incl. `vocabulary/export`, CCO/BFO) | admitted, tier 0 | D4, D9 (Q8) |
| provenance / statement metadata | admitted, tier 0 | matrix "Statement metadata" row, Keep |
| fusion (`Engine.Fuse` lens path) | admitted, tier 0 | D5; package-level `fusion.Fuse` deferred (D5) |
| tier ladder / graceful degradation | admitted (contract element) | D3, #4 comment 5929716018 |
| workflows that survive restarts (`pkg/lifecycle`) | admitted, tier 0 | D13 (Q16) |
| replay, retries with known outcomes (the durable-execution primitive) | deferred to epic #24 | D13; #24 open, blocked on #9 |
| settlement | admitted, tier 0 (repair-before-port row) | D11/D13 (Q18) |
| rules (rule core) | admitted, tier 0 | D12 (Q15); unknown action types refused at load, comment 5932719893 |
| BM25 lexical | admitted, tier 0, slice 04B | comment 5932313950 |
| change observation, operator surface, `composition/cli` | admitted, tier 0 | D14, D15, comment 5931143569 / 5932313950 |
| neural retrieval | admitted, tier 1, slice 04C | D3 |
| LLM features, agentic domain, gateways | tier-2 slot (empty) / separated / defer-exclude | D4 (Q4), proposal "Non-goals" |

No capability is deferred or excluded without a ruling.

### §7.4 Facts that bear on 03B decisions (reported plainly)

- **D13 / epic #24 rests on an implementation SemSource has removed.** D13: the primitive is "generalised … from the two
  existing implementations, agentic-loop's … and SemSource's `sourcelifecycle` (scope Q2.2)", qualifying consumer
  "semsource (`sourcelifecycle` migrates onto it)". At `e4febc0d` (SemSource PR #223, merged 2026-10-01)
  `internal/sourcelifecycle` and `internal/sourceintent` do not exist. PR #223's body: "SemEngine #18–20 must be
  reconsidered as separate framework contract questions, not prerequisites for this removed design". PR #222's body:
  "The private journal, replay, publication receipts and effect fences must not be ported. SemEngine #18–#20 remain
  separately justified contract decisions". D6's recorded consequence ("keeps
  `applied_tail_unproven` as a declared limit") and D8's ("SemSource (PR #213) keeps a durable fence and refuses
  re-admission") describe the pre-#223 consumer; PR #223's body states replies now report
  `projection_status:"unavailable"` and the lifecycle status endpoint returns HTTP 410. Epic #24's body (lines 11, 16)
  still names `sourcelifecycle` as a source implementation and the migrating consumer.
- **D11's "69 entries / 6 copies" holds for the packages it measured and extends to 117 / 12 on the ruled set.** In
  D11's own unit (`cleanup_baseline.json` entries) the 65 carry 117 = 69 + 48 from the ten entering packages;
  `lifecycleUsed` copies are 12 = 6 + 6 from the entering packages (03B declared those ten unmeasured). SemEngine's
  `cleanup-roots-check.sh` is a different instrument and reads 274 hits over the same tests; the two numbers do not
  contradict each other or D11.
- **`pkg/rulepack` is not a leaf contract package.** D4 and pass3 §2.1 describe it as "a rule-pack ID contract (143
  lines)"; at the pin it imports `config`, `natsclient`, `types` (`identity.go:11-13`) and validates `*config.Config`
  and `types.ComponentConfig` (`:62,:91`). Its closure is 28 packages. (No closure consequence for `rule`/`service`,
  which import `config` anyway; a consequence for any cut that wanted `pkg/rulepack` below `config`.)
- **D16's consumer import sets are confirmed for SemSource** (25 at `e4febc0d`), and SemConnect's root imports
  `composition` directly (`main.go:20`) — a package D16's rule would therefore export.
- **The plan's "graph/ alone 9 / 23K"** (`setup-plan.md:56`) is not a compile closure: closure(`graph`) is 17 /
  29,051 because `graph` imports `natsclient`.
- **`processor/graph-clustering`'s `graph/llm` import line is `component.go:22`**, not among the `:592,599,2294,2492`
  use sites pass3 §2.4 lists (those are uses; the import is `:22`). A line-citation nit, no change of fact.

## §8. Not measured, and what would measure it

- `go list -deps` on a SemEngine tree with the seam severed: not possible until packages land. At the pin it
  returns 74 = 65 + 9 behind-the-seam (§2); the remaining gap is only that the subtraction is by ruling, not by a seam.
- The type-checked T-B2 scan over the pin (embedded/alias/generic-holder contexts): needs the pin's packages loaded by
  SemEngine's `go/packages`-based checker in one module; only the grep form was run (0 hits).
- Which of the 278 `NewTestClient` call sites and 438 testify files each named root set carries, per test file: a
  per-file table is derivable from `tier0-test-imports.txt` and `grep -c`; not tabulated here.
- The exact 03A-observation-to-public-operation mapping for `*_doc_exact_passage_content` without NL: the SemSource
  owner states no such public path exists today (PR #222 `design.md:100-104`); whether `graph.query.entity` plus
  objectstore hydration satisfies it is a design question, not measured.
- (Answered in §6 column A, no longer unmeasured: KV fault injection exists at the pin as `mockKVBucket` assigned
  through `NewKVStore(jetstream.KeyValue)`; what is unmeasured is only whether a harness outside graph-ingest's package
  can reach the unexported field.)
- Whether JetStream file-store state can survive a `natsfixture` container restart: the fixture creates no volumes; a
  `docker restart` of a container keeps its writable layer, which would need a run to confirm.
- semboids and semteams were not re-read at a newer commit (scope rule 3); their 03B facts are cited as of `8c03cc53`
  / `ce22c961`.
- Coverage per tier-0 package at the pin (D10 baseline): not run (A13 stands).
- The 284 revive `package-comments` findings are attributed to file-list mode; a package-mode run inside a module that
  resolves the pin's dependencies would confirm 0.

## Correction pass (after inventory review `inventory-review.md`, verdict BLOCKING)

Inventory-only; structure unchanged. Pre-correction copy: `inventory.md.pre-correction`.

1. (BLOCKING) §6 column A, §3.1, §8: added the pin's KV fault-injection owner — `natsclient.KVStore` over the
   `jetstream.KeyValue` interface (`kv.go:48-55,232-236`) and graph-ingest's `mockKVBucket` (`component_test.go:28-36,
   122`) assigned at four sites — and natsfixture's `failAfter` wrapper (`fixture_integration_test.go:166-173`); the D8
   injection question is recorded as answered (no code seam; reach from outside the package is the only open part);
   the failpoint search output is restated as the grep returns it (`fixture_integration_test.go:166` included).
2. (BLOCKING) §6 column B, §3.1, premises 12–13: `runner/runner_test.go` recorded as a test-only subprocess host
   (`exec.CommandContext`, SIGTERM, `Process.Kill`, `Setpgid` + group SIGINT, `pidAlive`, grandchild SIGKILL); both
   search outputs pasted as returned (7 `os/exec` files; `TestS1_7Restart` and the four SIGKILL lines); the `docker`
   CLI calls in `fixture_integration_test.go:37,154` added to column C.
3. (HIGH) §1.4, premise 8: the "SemSource no-embedder" row replaced by SemSource as composed at `e4febc0d` with
   `graph-embedding` (56 / 113,786; 57 / 117,903 with clustering); union of the three consumers 63 / 140,382 leaving
   `composition/cli` and `internal/maxdelivery`; the 55 / 110,533 figure kept only as a labelled hypothetical;
   premise 8 restated (`processor/graph-embedding` is composed by SemSource and placed in 04B by ruling).
4. (MEDIUM) §1.4: heading retitled to "closures of named root sets"; the S1–S5 numbering removed; the partition kept
   as an unordered measurement; edge direction stated as the DAG-level property it is.
5. (MEDIUM) §5.1, §7.4: the three quotes re-cited to PR #222's body (0 comments, 0 reviews on that PR; the body is
   their only home), PR #223's body quoted only for what it contains, `design.md:142-144` quoted verbatim, and
   "SemEngine #18–#20 remain separately justified contract decisions" added.
6. (MEDIUM) §3.3, §4.3, §7.4: #9 item 8 answered in D11's unit — 48 `cleanup_baseline.json` entries for the ten
   entering packages, 117 in the 65 (= 69 + 48), per package; 274 guard hits kept as a separate instrument and corrected
   to 15 packages; the §7.4 bullet rephrased from "contradiction" to "extends D11; different instruments".
7. (MEDIUM) §1.5, §6: `NewTestClient(` restated as 278 call sites in 21 packages, with natsclient's own 69 stated
   separately (the earlier 209 / 20 excluded them without saying so).
8. (NIT) §3.3: Start/Stop owners 23 types (22 ported), ten in `service`.
9. (NIT) §3.1: natsfixture 1,501 non-test lines (2,771 total, 1,270 test).
10. (NIT) §2, §8: `go list -mod=mod -deps` over the 16 roots at the pin added — 74 = 65 + exactly the nine
    behind-the-seam packages, no tier-0 package missing (`deps16.txt`); "cannot be run" dropped.
11. (NIT) §3.2: `AGENTS.md:43-44`'s `task verify` list recorded as stale against `scripts/verify.sh:10-11`.
