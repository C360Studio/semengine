# SETUP 03B — pass 3: provider test, ceiling re-measure, rule-core seam sketch

Copied into this change on 2026-10-01 from the architect's reviewed scratch file `pass3.md` (with its
changelog). Edits are mechanical only: prose rewrapped and long headings shortened to the line limits; no
fact changed. Paths under `scratchpad/` name the architect's read-only working trees.

base: semengine worktree `48b4616` (PR #21) · SemStreams pin `8b99efe9` (snapshot `scratchpad/semstreams-8b99efe9/`) ·
semconnect PR #74 head `dff12657` (read via `git show`; the clone's HEAD is `d0d06e0`, the commit is present as an
object;
its `go.mod:6` pins `semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` = the pin) · semboids `8c03cc53`
(`go.mod:6` beta.160) · semteams `ce22c961`. Scope: #8 comment 5930855353 as ruled by 5930898291 (2026-10-01).
Architect, read-only; facts are `path:line` in the pin unless prefixed. Lines are non-test `.go` lines per package
directory (`find <pkg> -maxdepth 1 -name '*.go' -not -name '*_test.go' | xargs cat | wc -l`), the same measure as 65 /
126,926.

## Summary

**Item 1 — provider test.** Four of six are provider-free at runtime; two are provider clients.

| Package | Lines | Provider reach at the pin | Verdict |
| --- | --- | --- | --- |
| `graph/clustering` | 5,516 | compile edge to `graph/llm` from `summarizer.go:12`, used only by `LLMSummarizer` (`:499-810`) and `EnhancementWorker` (`enhancement_worker.go`, refuses a nil LLM summarizer `:107-109`, calls `w.llm.SummarizeCommunity` `:417`). LPA (`lpa.go`) and `StatisticalSummarizer` (`summarizer.go:35-300`, "Always available, no external dependencies" `doc.go:84-88`) are deterministic. `processor/graph-clustering` builds only `NewStatisticalSummarizer()` (`component.go:1372`); `EnableLLM` defaults false (`:60,:419,:531`); the worker starts only under `EnableLLM` (`:1073`) after an endpoint probe (`:2278-2285`) | **tier 0** (runtime); the compile edge is a two-file seam (see Item 1.1) |
| `graph/llm` | 842 | `openai_client.go` (365 lines) imports `go-openai` (`:12`), `model` (`:9`), `model/wire` (`:10`); the other seven files (477 lines) define `Client` (`client.go:19-21` — one method, `ChatCompletion`), `ChatRequest/ChatResponse` (`:31,:47`), `Config`, `ContentFetcher`, prompt templates — no provider import | **behind the seam** (provider client). Fork: splitting `openai_client.go` out leaves a provider-free interface package that `graph/clustering`, `graph/inference`, `graph/query` could keep importing (Item 1.1) |
| `graph/inference` | 5,338 | `graph/llm` imported at `config.go:12` (field `ReviewConfig.LLM llm.Config` `:123`) and `review_worker.go:17` (`LLMClient llm.Client // optional` `:69`; nil-guarded `:386`; the only call `ChatCompletion` `:430`; nil path falls to human review or auto-reject `:395-405`). `net/http` is **server-side** (`http_handlers.go:33-310`, review API), not a client. `hierarchy.go` (540 lines) has no `llm` reference | **tier 0** for hierarchy; compile edge is two files (seam) |
| `graph/structural` | 883 | imports `graph`, `pkg/errs` only (`go list`); files `kcore.go`, `pivot.go`, `types.go` | **tier 0** |
| `pkg/graphview` | 1,193 | imports `nats.go/jetstream` only; no semstreams import; generic `View`/`Delta`/`Subscription` over KV (`view.go:24`, `delta.go:41,59,80`, `subscription.go:17`) | **tier 0** |
| `model/wire` | 1,182 | OpenAI-compatible HTTP client: `client.go:32` (`HTTPClient *http.Client`), `:84,:121` (chat completions POST), `:148` (embeddings POST), `:177-182` (`http.NewRequestWithContext`). Importers: `graph/llm/openai_client.go:10`, `processor/agentic-model/*` (9 files), `model/wire/responses` | **behind the seam** (provider HTTP client) |

`enable_hierarchy: true` (semconnect `deploy/semstreams.json:46`) turns on `initHierarchyInference`
(`processor/graph-ingest/component.go:1417-1448`; doc comment `:1416`):
`inference.NewHierarchyInference(entityManagerAdapter,
tripleAdderAdapter, HierarchyConfig{CreateTypeEdges, CreateSystemEdges, CreateDomainEdges, CreateTypeSiblings,
Org, Platform})` — container entities and edges minted deterministically from the ingested entity's ID prefix
(`hierarchy.go:290-520`), authority-gated per ADR-102. Construction requires the container payload type registered
(`component.go:735-741`; `Config.EnableHierarchy` default false `:343,:408`). `processor/graph-ingest` has **0**
`llm`/`LLM` references (grep). **Provider-free.**

**Item 2 — ceiling under the ruled scope.** 65 / 126,926 reproduced. Pre-seam closure of the ruled roots plus
semconnect (`dff12657`) and semboids production imports, no cuts: **79 / 156,273**. After cutting the agentic domain,
`gateway*`, `graph/llm` + `model/wire`, and applying ruled D1 (no aggregator roots): **64 packages / 140,911 lines**
(−1 / +13,985 vs 65 / 126,926; −3 / +11,848 vs 67 / 129,063). Variants: aggregators left as roots 69 / 143,669;
embedding family also out 62 / 134,356; `gateway` base kept for semconnect's cs-api 65 / 141,230. Eight kept packages in
H
carry an edge into a cut package (ten in C, which still had the two aggregators; table in 2.4); two consumer production
imports (`payloadbuiltins`,
`vocabulary/builtins`) contradict the matrix's "none in production" cell for SemConnect.

**Item 3 — rule-core seam.** Four edits to `processor/rule` (E1 registry, E2 `Action` fields, E3 `publish_agent`
family out, E4 `deny`/`approve` + `VerdictAuditor`) plus one consequence (the withdrawn D4 `component` adapt becomes
compile-safe again). **No consumer's rule JSON changes**: semboids' 7 actions (publish ×6, lifecycle_transition ×1)
and semteams' 119 actions (add_triple 50, publish_agent 40, replace_owned 9, lifecycle_transition 8,
reconcile_predicates 6, remove_triple 4, publish 2) all still decode, provided E2 keeps `response_format`/`tool_choice`
as raw JSON rather than deleting them (semteams uses `tool_choice` in 31 rule JSON files; `response_format` in none —
E2 rests on the pin schema and the silent-drop property, not on consumer use). On a tier-0 core without the agentic
family registered, semteams' 40 `publish_agent` actions fail at **fire time** today (`actions.go:943` `unknown action
type`), not at load (no action-type check in `config_validation.go`); whether E1 moves that refusal to load time is a
fork (3.3). Independently of E1–E4, semteams' 9 `replace_owned` actions (5 files under `configs/rules/dev-via-test/`)
name a type that does not exist at the pin (`actions.go:33-75`; retired for `reconcile_predicates` per
`docs/operations/36-graph-foundation-breaking-cutover.md:43,97`) and already fail at fire time on the pin — semteams
owes that migration regardless of the seam.

---

## 1. Provider test (facts)

### 1.0 Imports at the pin

`go list -mod=mod -f '{{join .Imports "\n"}}' ./<pkg>` (run in the snapshot), third-party/provider-relevant and
semstreams imports:

| Package | Third-party of note | semstreams imports |
| --- | --- | --- |
| `graph/clustering` | `nats.go`, `jetstream` | `graph`, `graph/llm`, `message`, `metric`, `natsclient`, `pkg/errs`, `pkg/types` |
| `graph/llm` | `github.com/sashabaranov/go-openai` | `graph`, `model`, `model/wire`, `pkg/errs` |
| `graph/inference` | `nats.go`, `jetstream`, `net/http` | `graph`, `graph/llm`, `graph/structural`, `internal/graphmutation`, `message`, `metric`, `natsclient`, `payloadregistry`, `pkg/errs`, `pkg/types`, `vocabulary` |
| `graph/structural` | — | `graph`, `pkg/errs` |
| `pkg/graphview` | `jetstream` | — |
| `model/wire` | `net/http` | — |
| `graph/embedding` (in the 65; for 2.4) | `nats.go`, `jetstream`, `go-openai` | `model`, `pkg/errs`, `storage` |
| `processor/graph-clustering` (kept) | `net/http` | `component`, `graph`, `graph/clustering`, `graph/inference`, `graph/llm`, `graph/readiness`, `graph/structural`, `internal/graphmutation`, `internal/lifecyclecleanup`, `metric`, `model`, `natsclient`, `pkg/errs`, `pkg/resource`, `pkg/types`, `vocabulary` |

`model` (in the 65) imports `net/http` and `x/net/http2` for `NewHTTPClient` (`model/httpclient.go:105-106`), the
transport handed to OpenAI-compatible clients; it is a registry + HTTP transport factory, not itself a provider call.

### 1.1 Call sites that reach a provider client

- `graph/clustering`: `summarizer.go:501` (`Client llm.Client`), `:511,:529` (`llm.ContentFetcher`), `:625`
  (`llm.CommunityPrompt.Render`), `:638` (`s.Client.ChatCompletion`); `enhancement_worker.go:417`
  (`w.llm.SummarizeCommunity`), `:434` (`w.llm.Client.Model()`). Nothing in `lpa.go` or the statistical summarizer
  reaches `llm`. Composition: `processor/graph-clustering/component.go:1372-1387` wires `NewStatisticalSummarizer`
  into `NewLPADetector(...).WithSummarizer(...)`; the LLM path is `startEnhancementWorker` (`:2268`) →
  `llm.NewOpenAIClient` (`:2294`) → `clustering.NewLLMSummarizer` (`:2301`) → `clustering.NewEnhancementWorker`
  (`:2318`), entered only when `c.config.EnableLLM` (`:1073`). Compile seam = `summarizer.go:499-810`
  (`LLMSummarizer`, `LLMSummarizerConfig`, options, `domainGroupBuilder`) + `enhancement_worker.go` (549 lines).
- `graph/inference`: `review_worker.go:430` (`w.llmClient.ChatCompletion`) under `if w.llmClient != nil` (`:386`);
  `config.go:123` is a config-type dependency only. The review worker is constructed by
  `processor/graph-clustering/component.go:2416-2452` (`startReviewWorker`, `LLMClient: reviewClient` `:2439`),
  itself gated by `EnableAnomalyDetection` (`:76`, `:1062`, `:1082`); `resolveReviewLLMClient` falls back to the
  community-summary client (`:2486-2500`). Compile seam = `review_worker.go` (654 lines) + the `LLM llm.Config`
  field at `config.go:123`.
- `graph/query` (in the 65, kept): `classifier_llm_adapter.go:9` imports `graph/llm` (already D4's seam) and
  `classifier_embedding.go` imports `graph/embedding`.
- `processor/graph-query` (kept): `answer.go:11`, `component.go:18` import `graph/llm`; `component.go:434,:467`
  call `llm.NewOpenAIClient` (already D4's seam); `graphrag.go:22` imports `vocabulary/agentic` (D4's
  "three agentic label predicates" drop).
- Provider-client constructors repo-wide (non-test): `llm.NewOpenAIClient` at
  `processor/graph-query/component.go:434,467`,
  `processor/graph-clustering/component.go:2294,2492`, `processor/agentic-loop/component.go:474`,
  `processor/research-graph-{route,assess,classify}/component.go`. None in `graph/clustering`, `graph/inference`,
  `graph/structural`, `pkg/graphview`, or `processor/graph-ingest`.

### 1.2 Fork on `graph/llm` (facts for the owner, no ruling)

`graph/llm` is one package with two halves: a provider-free contract (`client.go` `Client` interface, `ChatRequest`,
`ChatResponse`; `config.go`; `prompts.go`, `prompt_data.go`, `prompt_types.go`; `content_fetcher.go`; 477 lines) and
one provider implementation (`openai_client.go`, 365 lines, the only file importing `go-openai`, `model`, `model/wire`).
(a) Treat the whole package as behind the seam (the ruled text): `graph/clustering` and `graph/inference` must then
drop their `llm` imports — i.e., `LLMSummarizer`/`EnhancementWorker` and `ReviewWorker`'s LLM field move to the tier
that owns the provider, which is a `class:port-refactor` on two tier-0 libraries. (b) Split `openai_client.go` out
of `graph/llm` (into the provider tier) and admit the 477-line contract at tier 0: `graph/clustering`,
`graph/inference`, `graph/query` compile unchanged; tier 0 holds an interface with no implementation, which the
contract's category 4 ("consumer at birth") would ask a present consumer for — at the pin the interface's tier-0
consumers are the nil-guarded optional fields (`review_worker.go:69`, `enhancement_worker.go:88`). Cost of (a): two
files out of each of two libraries plus their tests; cost of (b): one file move, and `graph/llm` enters the tier-0
closure (+477 lines, −365). Neither changes consumer-facing JSON.

## 2. Ceiling re-measure

### 2.1 Roots

- R28 (reproduces 65 / 126,926; inventory A1): `component config gateway/graph-gateway graph message metric model
  natsclient payloadregistry pkg/buffer pkg/errs pkg/fusion pkg/fusion/fusionnats pkg/fusion/fusionvocab
  pkg/projection pkg/retry pkg/types processor/graph-embedding processor/graph-index processor/graph-ingest
  processor/graph-query service storage storage/objectstore storage/storeregistry types vocabulary vocabulary/cco`.
- Ruled keeps (added as roots): `processor/rule pkg/lifecycle processor/graph-clustering pkg/graphview
  output/websocket internal/maxdelivery processor/graph-index-spatial processor/graph-index-temporal`.
- semconnect `dff12657`, production reach of `cmd/cs-graph-backend/main.go` (internal reach: `gateway/cs-api`
  via `main.go:18`, `message/oms`, `parser/sensorml`, `pkg/swecommon`, `vocabulary/csapi`, `vocabulary/sosa`), 26
  semstreams packages: `component composition config gateway graph graph/geo/geojson message metric natsclient
  payloadbuiltins payloadregistry pkg/errs pkg/projection pkg/projection/contract pkg/types processor/graph-index
  processor/graph-index-spatial processor/graph-index-temporal processor/graph-ingest processor/graph-query service
  storage/objectstore types vocabulary vocabulary/builtins vocabulary/export`. (`cmd/cs-api-server` reaches 13, a
  subset plus nothing new.)
- semboids `8c03cc53`, production reach of `cmd/semboids` + `cmd/sweep` (internal: `componentregistry`,
  `internal/{api,boidgraph,flock,sim,zone}`), 19: `component config graph message metric natsclient
  output/websocket payloadbuiltins payloadregistry pkg/graphview pkg/lifecycle pkg/projection
  processor/graph-clustering processor/graph-index processor/graph-ingest processor/rule service types vocabulary`.
- Cuts: **D** (agentic domain) = `agentic agentic/agentrun governance vocabulary/agentic processor/agentic-*`;
  **G** = `gateway gateway/graph-gateway`; **P6** (Item 1) = `graph/llm model/wire`. `pkg/rulepack` is **not** cut:
  its non-test importers are `processor/rule/config.go:14`, `service/rule_pack_bind.go`,
  `frameworkcapabilities/rulepacks/validate.go` — a rule-pack contract (143 lines), not agentic.
- D1 (ruled, #3): the engine ships no aggregator, so `payloadbuiltins` (imports `agentic governance graph/inference
  message payloadregistry pkg/lifecycle processor/gated-dag storage/objectstore`; `register.go:44-52`) and
  `vocabulary/builtins` (imports `vocabulary/agentic`, `vocabulary/rulepacks`) are removed from the root set; the
  per-package registrations they stand for (`graph/inference.RegisterPayloads`, `pkg/lifecycle.RegisterPayloads`,
  `message`, `storage/objectstore`) are already in the closure.

### 2.2 Numbers

| Set | Packages | Non-test lines | Δ vs 65 / 126,926 | Δ vs 67 / 129,063 |
| --- | --- | --- | --- | --- |
| A. R28 at the pin (reproduced) | 65 | 126,926 | 0 / 0 | −2 / −2,137 |
| B. Ruled roots + semconnect + semboids, **no cuts** (pre-seam closure; `go list -deps` verified, 2.5) | 79 | 156,273 | +14 / +29,347 | +12 / +27,210 |
| C. B after cuts D ∪ G ∪ P6, aggregators still roots | 69 | 143,669 | +4 / +16,743 | +2 / +14,606 |
| **H. C with D1 applied (no aggregator roots) — the tier-0 production set under the ruled scope** | **64** | **140,911** | **−1 / +13,985** | **−3 / +11,848** |
| I. H minus `graph/embedding` + `processor/graph-embedding` (if "no external provider" is applied to the embedding family) | 62 | 134,356 | −3 / +7,430 | −5 / +5,293 |
| J. H plus `gateway` base (semconnect cs-api's interface assertion, 2.4) | 65 | 141,230 | 0 / +14,304 | −2 / +12,167 |

H minus the 65 (9 packages, +29,125): `processor/rule` 15,826, `processor/rule/expression` 1,417,
`processor/graph-clustering` 4,117, `output/websocket` 2,220, `processor/graph-index-temporal` 1,573,
`processor/graph-index-spatial` 1,526, `vocabulary/export` 1,107, `graph/geo/geojson` 1,030, `internal/maxdelivery` 309.

The 65 minus H (10 packages, −15,140): `agentic` 6,005, `gateway/graph-gateway` 2,676, `vocabulary/agentic` 2,133,
`agentic/agentrun` 1,505, `model/wire` 1,182, `graph/llm` 842, `gateway` 319, `internal/deliverylane` 248,
`internal/agentterminal` 188, `internal/looptoken` 42 (the last three leave because only `agentic*` reached them).

Dropped from C to H by D1 (−2,758): `processor/gated-dag` 2,367, `pkg/gateddag` 221, `payloadbuiltins` 93,
`vocabulary/rulepacks` 62, `vocabulary/builtins` 15 — all reached only through the two aggregators.

### 2.3 The 64 (H), with lines

```text
component 5585 · component/flowgraph 1324 · composition 914 · config 5193 · graph 3293 · graph/clustering 5516 ·
graph/embedding 3302 · graph/geo/geojson 1030 · graph/inference 5338 · graph/query 1551 · graph/readiness 1016 ·
graph/structural 883 · health 625 · internal/componentadmission 8 · internal/graphmutation 264 ·
internal/lifecyclecleanup 38 · internal/logforwarderpolicy 105 · internal/maxdelivery 309 · message 2186 ·
metric 1218 · model 1397 · natsclient 12377 · output/websocket 2220 · payloadregistry 508 · pkg/acme 543 ·
pkg/buffer 1235 · pkg/cache 2449 · pkg/dispatch 1122 · pkg/errs 904 · pkg/fusion 2789 · pkg/fusion/fusionnats 658 ·
pkg/fusion/fusionvocab 51 · pkg/graphview 1193 · pkg/lifecycle 3739 · pkg/platform 43 · pkg/projection 694 ·
pkg/projection/contract 163 · pkg/resource 398 · pkg/retry 262 · pkg/revlag 213 · pkg/rulepack 143 ·
pkg/security 384 · pkg/timestamp 366 · pkg/tlsutil 533 · pkg/types 751 · pkg/worker 707 ·
processor/graph-clustering 4117 · processor/graph-embedding 3253 · processor/graph-index 4527 ·
processor/graph-index-spatial 1526 · processor/graph-index-temporal 1573 · processor/graph-ingest 5870 ·
processor/graph-query 6745 · processor/rule 15826 · processor/rule/expression 1417 · service 11819 · storage 356 ·
storage/objectstore 3342 · storage/storeregistry 115 · types 188 · vocabulary 2673 · vocabulary/bfo 402 ·
vocabulary/cco 515 · vocabulary/export 1107
```

(File: `scratchpad/ceiling-H.txt`; all variants: `scratchpad/ceiling-sets.json`.)

### 2.4 Kept packages that reach a cut package (agentic, gateway, provider)

One line each.

| Kept package | Edge | Import site | What severs it |
| --- | --- | --- | --- |
| `component` | → `agentic` | `component/dependencies.go:7`; `ToolRegistryReader` `:47-57` (`agentic.ToolCall`, `agentic.ToolResult`, `agentic.ToolDefinition`); field `Dependencies.ToolRegistry` `:76` | the D4 `component` adapt, withdrawn because of `processor/rule`; re-enabled by E3 (3.2) |
| `service` | → `agentic/agentrun` | `service/milestone_service.go:10` (`agentrun.StartConfig` `:23,:50,:64`) | D4 non-port of `milestone_service.go` (stands; semteams `cmd/semteams/main.go:941` impact already in scope-inventory Q1.5) |
| `processor/rule` | → `agentic`, `agentic/agentrun`, `governance`, `vocabulary/agentic` | `actions.go:15,16,18,27`; `config_validation.go:9`; `verdict_auditor.go:10` | E1–E4 (section 3) |
| `processor/graph-query` | → `graph/llm`, `vocabulary/agentic` | `answer.go:11`, `component.go:18`; `graphrag.go:22` | D4 capability seam + label-predicate drop (unchanged) |
| `graph/query` | → `graph/llm` | `classifier_llm_adapter.go:9` | D4 capability seam (unchanged) |
| `graph/clustering` | → `graph/llm` | `summarizer.go:12` | Item 1.2 fork (a) or (b) |
| `graph/inference` | → `graph/llm` | `config.go:12`, `review_worker.go:17` | Item 1.2 fork (a) or (b) |
| `processor/graph-clustering` | → `graph/llm` (and `model` for endpoint resolution) | `component.go:592,599` (`llm.Client` fields), `:2294,:2492` (`llm.NewOpenAIClient`), `:2269` (`model.ResolveEndpointWithConfig`) | the same capability seam shape as `processor/graph-query`: `startEnhancementWorker` (`:2268-2330`) and `startReviewWorker`/`resolveReviewLLMClient` (`:2416-2500`) behind the optional-capability interface |
| `graph/query`, `processor/graph-embedding` | → `graph/embedding` (imports `go-openai`) | `graph/query/classifier_embedding.go`; `processor/graph-embedding/{component,query,readiness}.go` | **not cut by the ruled text** — the 65 keeps the embedding family (SemSource composes it). Under "tier 0 = no external provider" this is the one remaining provider edge in H (variant I removes it: −2 / −6,555) |
| semconnect `gateway/cs-api` (consumer) | → `gateway` (cut by G) | `gateway/cs-api/component.go:15` at `dff12657`; the sole use is `var _ gateway.Gateway = (*Component)(nil)` `:198`. Compile-time only: SemStreams mounts HTTP handlers through an anonymous `interface{ RegisterHTTPHandlers(prefix string, mux *http.ServeMux) }` (`service/service_manager.go:1550-1555`), never through `gateway.Gateway` | consumer drops the assertion, or SemEngine ships the 319-line `gateway` base (imports `component`, `pkg/errs`, `service`; variant J) — owner call |
| semconnect `cmd/cs-graph-backend`, semboids `cmd/semboids` (consumers) | → `payloadbuiltins`, `vocabulary/builtins` (→ `agentic`, `governance`, `gated-dag`, `vocabulary/agentic`) | semconnect `main.go:25,36` imports; `:110,:123` `builtins.Register()`; `:228` `payloadbuiltins.Register(payloads)`. semboids `cmd/semboids/main.go:163` `payloadbuiltins.Register(payloadReg)` | ruled D1 (a): per-package registration in the consumer's composition root. **Matrix correction:** the "Registration · aggregator imports" row's SemConnect cell reads "none in production"; at `dff12657` both aggregators are imported in production |

### 2.5 Commands (reproduction)

```text
# pin snapshot: edges for every package in the module (180 packages)
cd scratchpad/semstreams-8b99efe9
go list -mod=mod -f '{{.ImportPath}} {{join .Imports " "}}' ./... > ../pin-edges.txt

# per-package imports (Item 1)
go list -mod=mod -f '{{join .Imports "\n"}}' ./graph/clustering   # likewise graph/llm graph/inference graph/structural pkg/graphview model/wire graph/embedding processor/graph-clustering

# B, pre-seam closure of the ruled roots + consumer imports (79; diff-identical to the script's closure)
go list -mod=mod -deps ./component ./config ./graph ./message ./metric ./model ./natsclient ./payloadregistry \
  ./pkg/buffer ./pkg/errs ./pkg/fusion ./pkg/fusion/fusionnats ./pkg/fusion/fusionvocab ./pkg/projection ./pkg/retry \
  ./pkg/types ./processor/graph-embedding ./processor/graph-index ./processor/graph-ingest ./processor/graph-query \
  ./service ./storage ./storage/objectstore ./storage/storeregistry ./types ./vocabulary ./vocabulary/cco \
  ./processor/rule ./pkg/lifecycle ./processor/graph-clustering ./pkg/graphview ./output/websocket \
  ./internal/maxdelivery ./processor/graph-index-spatial ./processor/graph-index-temporal \
  ./composition ./gateway ./graph/geo/geojson ./payloadbuiltins ./pkg/projection/contract ./vocabulary/builtins \
  ./vocabulary/export | grep 'c360studio/semstreams' | sort -u | wc -l        → 79

# C/H/I/J cannot be produced by go list at the pin (the seam does not exist there): they are reachability over
# pin-edges.txt with the cut packages removed as nodes — scratchpad/ceiling.py (sets A–E; its final json.dump()
# writes ceiling-sets.json, the one scratch file the script writes) and the inline variant block (F–J) recorded in
# this pass (writes ceiling-H.txt); stdout captured in scratchpad/ceiling-out.txt.

# consumer production reach (git show at the commit; no checkout): scratchpad/reach.py
python3 reach.py semconnect dff12657 github.com/c360studio/semconnect cmd/cs-graph-backend   → 26 pkgs (sc74-reach-backend.txt)
python3 reach.py semboids   HEAD      github.com/c360studio/semboids   cmd/semboids,cmd/sweep → 19 pkgs (semboids-reach.txt)
```

### 2.6 D1 adopter-path items (ledger-row candidates under the D1 adapt)

Under ruled D1 (a) `payloadbuiltins` does not exist in SemEngine, but the pin's error text and doc comments send the
adopter to it:

| Site | Text at the pin | Should name |
| --- | --- | --- |
| `processor/graph-ingest/component.go:739` | `enable_hierarchy requires %s in the payload registry (register it with payloadbuiltins.Register)` — a **boot-time refusal** the adopter reads | `inference.RegisterPayloads` (`graph/inference/container_entity.go:62`) |
| `graph/inference/container_entity.go:14,59` | "Registered by RegisterPayloads (via payloadbuiltins.Register)…"; "Called from payloadbuiltins.Register at process bootstrap" | the per-package call |
| `pkg/lifecycle/harness_entity.go:14,63` | same pattern | `lifecycle.RegisterPayloads` |
| `message/generic_json.go:26` | "Called from payloadbuiltins.Register at…" | `message.RegisterPayloads` |
| `storage/objectstore/stored_message.go:90` | "Called from payloadbuiltins.Register at process…" | `objectstore.RegisterPayloads` |

The `:739` item is adopter-facing (rank: boot error naming a symbol that will not exist); the rest are doc comments.
All six belong on the D1 ledger row (`adapt`); proving test: the graph-ingest refusal text names a symbol that exists
in the SemEngine tree (a string assertion in the factory test).

## 3. Rule-core seam sketch

### 3.1 Where `processor/rule` reaches separated packages (scope §1)

From scope-inventory §1, re-pinned.

| Package | Import | Symbols and sites (non-test) | Family |
| --- | --- | --- | --- |
| `agentic` | `actions.go:15`, `config_validation.go:9` | `Action.ResponseFormat *agentic.ResponseFormat` `:191`, `Action.ToolChoice *agentic.ToolChoice` `:206`; `TryChainExecutionEntityID` `:751`; `ToolDefinition` `:1445`; `TaskMessage` `:1488,1546,1576,1601,1628`; metadata keys and `IsKnownFilesystemPolicy` in `publishAgentOnce` `:1757-2090`; validation `config_validation.go:358` (`LineageTriplePredicate`), `:407` (`IsKnownFilesystemPolicy`) | `publish_agent` |
| `agentic/agentrun` | `actions.go:16` | `agentrun.Mint(ctx, e.lifecycle, …)` `:1989` | `publish_agent` (run-scope "new") |
| `vocabulary/agentic` | `actions.go:27` | `stampRunAnchors` `:731-753` (`agvocab.LoopRun`, `LoopRunEntityID` `:750,:752`); called only from `publishAgentOnce` `:2009` | `publish_agent` |
| `governance` | `actions.go:18`, `verdict_auditor.go:10` | `VerdictAuditor.EmitVerdict(ctx, governance.VerdictEvent)` `:497-503`; `executeDeny` `:2101-2111`, `emitVerdictAudit` `:2119-2150`, `executeApprove` `:2164-2248` (`DecisionDeny`/`DecisionApprove`); `streamVerdictAuditor` (`verdict_auditor.go:20-80`, `VerdictSubject`, `PublishToStream`) | `deny` / `approve` |
| `pkg/rulepack` | `config.go:14` | `PackIDCharset`, `MaxPackIDBytes` `:149-151`; `ValidateID` `:177` | rule-pack ID validation (core; stays) |
| `component.ToolRegistryReader` (→ `agentic`) | `actions.go:542,769`; `processor.go:82,347,687-689`; `factory.go:148` | `toolRegistry` field, `SetToolRegistry`, factory plumbing from `deps.ToolRegistry` | `publish_agent` (tool-name resolution `resolveToolNames` `:1445`) |

Dispatch is a closed `switch action.Type` in `ActionExecutor.Execute` (`actions.go:916-945`; `default:` →
`fmt.Errorf("unknown action type: %s", …)` `:943`). Action types are string consts (`:33-75`). The `Action` struct
(`:89-444`) is one flat struct for every family, decoded by plain `encoding/json` (no `UnmarshalJSON` on `Action` or
`Definition`; `config_validation.go:524` `json.Unmarshal(raw, &actions)`; no `DisallowUnknownFields` anywhere in the
package) — unknown keys are silently dropped today. Config validation (`config_validation.go`) checks fields per type
(`:346,:382,:396,:407`, `:438`) but never rejects an unknown `type`. Deny short-circuit is core chain semantics:
`DenyVerdict`/`ErrDenyVerdict` (`deny.go:29-48`) are consulted by `stateful_evaluator.go:427` and
`cron_scheduler.go:610`.

Sizes now (ruled "count its full lines"): `processor/rule` 15,826 + `expression` 1,417 = **17,243**. The three files
that shrink: `actions.go` 2,339 (publish_agent `:1445-2090` ≈ 646 lines incl. helpers, `stampRunAnchors` `:731-753`,
deny/approve/audit `:2101-2248` ≈ 148; agentic-typed fields `:179-206`), `config_validation.go` 658 (`:346-409`
publish_agent checks ≈ 60), `verdict_auditor.go` 80 (whole file). `deny.go` (48) stays.

### 3.2 Edits (each a `class:port-refactor` ledger-row candidate)

Each: disposition `adapt`, `contract` = matrix row, `proving_tests` named.

**E1 — action-family registration in the core.** `actions.go:916-945`: the `default:` arm consults a registry
`map[string]ActionFamily` on `ActionExecutor` before returning `unknown action type`. New core symbols: `type
ActionFamily interface { Execute(ctx context.Context, action Action, ec *ExecutionContext) error; Validate(def
*Definition, label string, i int, action Action) error }` and a registration method (fork below); the family
`Validate` hook is called from the per-action loop in `config_validation.go` (where `:346-409` sit today) so a family
can keep its config-time checks. Core families stay compiled in: `publish`, `add_triple`, `remove_triple`,
`update_triple`, `reconcile_predicates`, `update_kv`, `lifecycle_transition|complete|fail`.
*Fork (registration shape):* (a) a package-level `rule.RegisterActionFamily(name, factory)` called from the
consumer's composition root (D1 (a) shape: the root names what it composes; cost: process-global state in a library,
a pattern ADR-103 moved away from for payload types); (c) the agentic tier wraps the rule factory — the consumer
registers component type `rule` from either `processor/rule.CreateComponent` (core) or
`<agentic-tier>.CreateRuleComponent`, which calls the core factory then `processor.RegisterActionFamily(...)` on the
instance with its own dependencies (tool registry, lifecycle manager) captured in the closure — no global state, no
new field on `component.Dependencies`; cost: the factory becomes part of the exported surface (`Processor` gains one
method). Option (b), a typed `ActionFamilies` field on `component.Dependencies`, re-creates the shape D4 is removing
(`Dependencies.ToolRegistry`) under a new name and is listed only to be visible.
*Compile consequence:* none for consumers — semboids' 7 and semteams' 70 core actions dispatch exactly as today
(semteams' 9 `replace_owned` actions hit the `default:` arm on the pin today and keep doing so under E1).
*Behavior fork (see 3.3):* whether an unregistered `type` is refused at config load (new) or at fire time (today).
*Matrix row:* Rules · "action dispatch is open to registered families; core families enumerated"; *proving tests:*
a fake family registered on a test executor is dispatched and validated; a rule pack containing only core types
loads and fires with no family registered (semboids corpus: `configs/rules/zone-steering/*.json`).

**E2 — `Action` keeps its JSON shape but loses the agentic types.** `actions.go:191` `ResponseFormat
*agentic.ResponseFormat` → `json.RawMessage` (tag `response_format,omitempty` unchanged); `:206` `ToolChoice
*agentic.ToolChoice` → `json.RawMessage` (`tool_choice,omitempty`). The `publish_agent` family decodes them into
`agentic.ResponseFormat`/`agentic.ToolChoice` and runs their `Validate()` (`agentic/types.go:19,:90`) in its `Validate`
hook. The twelve plain publish_agent-only fields (`Role :141, Model :144, Prompt :148, Tools :160, ActionAllowlist
:177, RelatedLoops :229, FilesystemPolicy :248, ScratchPaths :257, RunScope :272, WorkflowSlug :275, WorkflowStep
:278, LoopMaxIterations :368`) decode without `agentic` and can stay as-is.
*Fork:* (a) RawMessage per field — minimal, the core still names two agentic keys; (b) a core `Extensions
map[string]json.RawMessage` collecting every key the core does not declare — family-agnostic, but needs a custom
`Action.UnmarshalJSON` (none exists) and changes the silent-drop behavior for genuinely unknown keys (today dropped;
under (b) retained and visible). Either way **no consumer's rule JSON changes**: the keys, their names and `omitempty`
are identical. The case for retaining both keys is the pin schema plus the decoder's silent-drop property: both are
declared at the pin (`actions.go:191,206`) and the package decodes without `DisallowUnknownFields`, so deleting a field
would drop that key from every rule that carries it with no error. Present consumer use: `tool_choice` in 31 semteams
rule JSON files (`grep -rl tool_choice --include='*.json' semteams/configs/rules`); `response_format` in 0 (the only
hit is prose, `configs/rules/autoresearch/README.md`) — so for `response_format` the retention argument is schema
fidelity alone, not an observed consumer.
*Matrix row:* Rules · "rule JSON decodes unchanged for every consumer corpus"; *proving tests:* golden round-trip of
the semteams corpus (119 actions) and semboids corpus (7) through the core decoder; property: core decode →
family decode of `tool_choice` equals direct `agentic.ToolChoice` decode.

**E3 — `publish_agent` family moves to the agentic tier.** Out of `actions.go`: const `ActionTypePublishAgent`
`:50`; `resolveToolNames :1445`, `stampRelatedLoops :1488`, `isReservedTaskMetadataKey :1523` (only caller `:1548`),
`stampAuthorMetadata :1546`, `stampFilesystemPolicy :1576`, `stampPerSpawnLLMKnobs :1601`, `stampLoopMaxIterations
:1628`, `diagnoseForEachResolutionFailure :1650` (only caller `:1727`), `executePublishAgent :1683`,
`publishAgentOnce :1757-2090`, `stampRunAnchors :731-753`; fields `toolRegistry :542` + `SetToolRegistry :769`;
imports `:15,16,27`. Out of `config_validation.go`: `:346` (related_loops only on publish_agent), `:358`, `:396`
(run_scope), `:407`; import `:9`. Out of `processor.go`: `toolRegistry :82`, `SetToolRegistry :347`, the setter
probe `:687-689`. Out of `factory.go`: `:148`. The family's dependencies (tool registry, `LifecycleManager` for
`agentrun.Mint` `:1989`, deployment authority) arrive through its registration (E1). The core `LifecycleManager`
interface keeps `Get`/`Create` (`:521-530`, documented as the mint path) only if the family needs them through the
core interface; otherwise they move too — one line each.
*Compile consequence:* `processor/rule` no longer imports `agentic`, `agentic/agentrun`, `vocabulary/agentic`, or
`component.ToolRegistryReader`; the withdrawn D4 `component` adapt (drop `Dependencies.ToolRegistry`
`component/dependencies.go:76` and `ToolRegistryReader` `:47-57`, plus `service/dependencies.go:34`,
`service/component_manager.go:47,197,203,1216`) becomes compile-safe again, **provided** E1's registration carries
the tool registry (option (c) does; option (a) needs the registry passed at registration). semboids: unaffected
(0 publish_agent). semteams: 40 `publish_agent` actions require the agentic family registered; on a core without it
they fail at fire time today (`:943`) or at load under 3.3(ii); its 9 `replace_owned` actions fail at fire time on the
pin with or without the family (not a type at the pin) until semteams migrates them to `reconcile_predicates`.
`agentic-loop`, `agentic-dispatch`, `agentic-tools`
also consume `ToolRegistryReader` (`processor/agentic-{dispatch,loop,tools}/*.go`) — they travel with the agentic
tier, not with this edit.
*Matrix row:* Rules · "core compiles with no agentic import; publish_agent is a registered family"; *proving tests:*
import guard (T-B8 shape) `go list -deps ./processor/rule` ∩ {`agentic*`, `vocabulary/agentic`, `governance`} = ∅;
the eleven existing publish_agent test files move with the family (`action_id_test.go`,
`action_failure_metrics_test.go`,
`action_maxiterations_test.go`, `actions_foreign_firing_reason_test.go`, `actions_run_scope_integration_test.go`,
`actions_test.go` (split), `config_validation_test.go` (split), `cron_scheduler_test.go` (split),
`example_fan_out_integration_test.go`, `user_response_subject_reservation_test.go` (split),
`research_graph_pipeline_integration_test.go`); semteams corpus: 40/40 dispatched with the family registered.

**E4 — `deny`/`approve` and `VerdictAuditor`.** Two shapes, both schema-neutral:
(i) *family out* (the ruled text): consts `:56,:63`; `executeDeny :2101`, `emitVerdictAudit :2119`, `executeApprove
:2164-2248`; `VerdictAuditor` `:497-503`, field `:546`, `SetVerdictAuditor :782-788`; `verdict_auditor.go` whole;
`processor.go:714-716` wiring; imports `actions.go:18`, `verdict_auditor.go:10`. `deny.go` stays (the chain's
short-circuit at `stateful_evaluator.go:427`, `cron_scheduler.go:610`); the governance family returns the core's
`*DenyVerdict`. Core then has **no** verdict action; present tier-0 consumers of deny/approve: semteams 0, semboids 0
(corpus counts), so category 4 ("consumer at birth") is satisfied by removal.
(ii) *keep deny/approve in core, retype the auditor*: `VerdictAuditor.EmitVerdict(ctx, decision, ruleID, reason
string, ec *ExecutionContext) error` — primitives, no `governance` type; `verdict_auditor.go` (the
`governance.VerdictEvent` builder + `VerdictSubject` + stream publish) moves to the governance tier as the
implementation; `emitVerdictAudit` shrinks to the nil-check and call. Core keeps two action types whose only tier-0
users are SemStreams' own gated flows; `approve` also publishes a `GenericJSON` verdict to `action.Subject`
(`:2194-2232`) with no governance import — that half is already core-shaped.
*Compile consequence:* none for semboids/semteams either way. *Matrix rows:* Rules · "deny is terminal: a
`*DenyVerdict` stops the chain and is never retried" (core, both shapes); Rules · "verdict audit is optional and
never flips a verdict" (family or auditor). *Proving tests:* `deny_integration_test.go`, `stateful_evaluator_test.go`
(deny arm) stay in core under (ii), move under (i); `verdict_auditor_test.go` moves under both.

**E5 — `pkg/rulepack` carries (no edit).** `config.go:148-177` validates `pack_id` via `rulepackcontract.ValidateID`;
`service/rule_pack_bind.go` (ruled: port) imports it too. Ledger row disposition `carry`; `contract` = Rules ·
"rule-pack IDs validated by one contract package".

Edit count: **4** (E1–E4) + 1 re-enabled D4 adapt (`component`) as a consequence of E3; E5 is a carry row.
Files touched in `processor/rule`: `actions.go`, `config_validation.go`, `verdict_auditor.go`, `processor.go`,
`factory.go` (+ test splits); `pkg/lifecycle` untouched; `actions_lifecycle.go` untouched (`requireLifecycleManager`
`:166-168` already returns a typed error when no manager is wired).

### 3.3 Behavior fork surfaced by E1 (for the owner, no ruling)

Today an action whose `type` is unknown passes config validation and fails only when the rule fires
(`actions.go:943`; no check in `config_validation.go`). With families registered per deployment this becomes
operator-visible: a semteams pack loaded on a tier-0 core would fire-fail 40 times rather than refuse to load.
(i) Keep fire-time refusal — behavior-identical to the pin; the adopter learns at "log line" rank. (ii) Refuse at
load when `type` is neither core nor registered — a boot/typed error ("compile error > boot error > … > log line",
architect contract, adopter seam Q3), but a behavior change from the pin that needs its own matrix row and proving
test ("a definition with an unregistered action type is rejected at load with the type named"). Measured
consequence of (ii) on the semteams corpus at `ce22c961`: the 5 files carrying `replace_owned`
(`configs/rules/dev-via-test/{02c-plan-retry-stamp,02d-plan-retry-driver,07a-cbg-approved-to-coordinator,
07c-cbg-retry-stamp,07d-cbg-retry-driver}.json`) would be rejected at boot **even with the agentic family
registered**, because `replace_owned` is not a type at the pin; under (i) those 9 actions keep failing at fire time
as they do on the pin today.

### 3.4 Consumer decode/execute table after E1–E4

| Consumer | Actions | Decode on tier-0 core | Execute on tier-0 core | Needs registered family |
| --- | --- | --- | --- | --- |
| semboids `8c03cc53` | publish 6, lifecycle_transition 1 | 7/7 | 7/7 (lifecycle needs `SetLifecycleManager`, as today) | none |
| semteams `ce22c961` | add_triple 50, publish_agent 40, replace_owned 9, lifecycle_transition 8, reconcile_predicates 6, remove_triple 4, publish 2 (119) | 119/119 (E2 keeps `tool_choice` ×31 files; `response_format` ×0) | 70/119 | `publish_agent` (agentic tier) for 40; `replace_owned` ×9 is **not an action type at the pin** (`actions.go:33-75`; retired per `docs/operations/36-graph-foundation-breaking-cutover.md:43,97`) — fails at fire time on the pin today; semteams owes the `replace_owned` → `reconcile_predicates` migration independently of the seam |
| semconnect `dff12657`, semsource `3604a9ce` | compose no rule processor (scope-inventory Q3; semconnect `deploy/semstreams.json` has no `rule` component) | — | — | — |

SemStreams' rule JSON schema: unchanged for every consumer under E1–E4 (keys, names, `omitempty` preserved; E2 (b)
would additionally retain unknown keys instead of dropping them).

## 4. Not established (and what would establish it)

- The post-seam closure (H) is a graph cut computed over `go list` edges, not a `go list -deps` on a tree where the
  seam exists; the first green tier-0 extraction's `go list -deps` on SemEngine is the measurement that replaces it.
- ~~Whether semconnect's `_ gateway.Gateway` assertion is load-bearing beyond compile time~~ — closed by review:
  the runtime mount point is the anonymous `RegisterHTTPHandlers` interface at `service/service_manager.go:1550-1555`,
  so the assertion is compile-time conformance only (2.4).
- semboids' and semteams' trees are at beta.160 / `ce22c961`, not at the pin; their import strings were read, not
  resolved through `go list`, so a package renamed between beta.160 and the pin would be missed (none of the 19/26
  names is absent from `pin-edges.txt`).
- Which `publish_agent` tests in the shared files (`actions_test.go`, `config_validation_test.go`,
  `cron_scheduler_test.go`, `user_response_subject_reservation_test.go`) split to the family: a per-test enumeration
  (`grep -n 'func Test' + body scan`) owned by the 04A change that performs E3.
- Whether `graph/embedding` + `processor/graph-embedding` (go-openai, 6,555 lines) stay in tier 0 is not in the
  ruled text; variant I records the number if the "no external provider" rule is applied to them.

## Changelog

### 2026-10-01 — review round 1: PASS with corrections → applied

Review: `scratchpad/03b/pass3-review.md`.

- **C1 (HIGH)** semteams census: 119 actions, not 110 — 9 `replace_owned` in 5 `configs/rules/dev-via-test/` files
  were missed. `replace_owned` is not an action type at the pin (`actions.go:33-75`; retired per
  `docs/operations/36-graph-foundation-breaking-cutover.md:43,97`), so those 9 fail at fire time on the pin
  regardless of E1–E4. Fixed: Summary (Item 3), E1 compile-consequence line, E2 proving-test corpus size, E3 semteams
  line, §3.3 (refusal-at-load would reject the 5 files at boot even with the agentic family registered), §3.4 row
  (119/119 decode, 70/119 execute). Added: semteams owes the `replace_owned` → `reconcile_predicates` migration the
  pin requires, independently of the seam.
- **C2 (MEDIUM)** `response_format` is in 0 semteams rule JSON files (the one hit was `autoresearch/README.md`);
  `tool_choice` is in 31 (the 32nd hit was README prose). E2 restated on the pin schema (`actions.go:191,206`) and
  the silent-drop property (no `DisallowUnknownFields`) alone; consumer use recorded as 31 / 0. Summary and §3.4
  updated.
- **C3 (MEDIUM)** Summary: eight kept packages in H carry an edge into a cut package (ten was set C's count, which
  still had `payloadbuiltins` and `vocabulary/builtins` as roots).
- **Addition** §2.4 gateway row and §4: SemStreams mounts HTTP handlers through an anonymous
  `RegisterHTTPHandlers` interface (`service/service_manager.go:1550-1555`), not `gateway.Gateway`; semconnect's
  assertion at `gateway/cs-api/component.go:198` is compile-time only. The §4 open item is closed.
- **Addition** new §2.6: D1 adopter-path items — the boot refusal at `processor/graph-ingest/component.go:739` and
  doc comments at `graph/inference/container_entity.go:14,59`, `pkg/lifecycle/harness_entity.go:14,63`,
  `message/generic_json.go:26`, `storage/objectstore/stored_message.go:90` name `payloadbuiltins.Register`, which
  will not exist under D1; listed as D1 ledger-row items with the per-package `RegisterPayloads` each should name
  (`graph/inference/container_entity.go:62` for the refusal).
- **Nits** `initHierarchyInference` cited `:1417-1448` (doc comment `:1416`); `executeApprove` cited `:2101-2248`
  (deny/approve/audit ≈ 148 lines); §2.5 marks `ceiling.py` as writing one scratch file (`ceiling-sets.json` via its
  final `json.dump`) and the inline variant block as writing `ceiling-H.txt`.
- Not changed: every number in §2.2 (reviewer: "no re-measurement needed"); the provider verdicts in §1; the edit
  count (4 + 1 consequence) in §3.
