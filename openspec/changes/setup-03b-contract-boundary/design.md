# Design: setup-03b-contract-boundary

## Purpose and admission

The owner's statement of purpose, as `AGENTS.md` on `main` states it:

> SemEngine is a live semantic knowledge-graph framework with durable execution. In the owner's words (2026-10-01):
> "TrustGraph and Temporal had a tiny baby — a pragmatic, Go-idiomatic, NATS-based, offline-first and edge-capable
> baby." It has two halves. The **live semantic knowledge graph**: ingest, index, query, vocabulary, provenance,
> fusion, and a tier ladder that degrades gracefully; tier 0 needs no external provider. **Durable execution**:
> workflows that survive restarts, replay, settlement, retries with known outcomes, and the rules that drive them.
> A capability is engine-owned when it belongs to either half and runs at tier 0 without an external provider; it is
> admitted by owner mandate at a named tier with a named qualifying workload, and consumer need decides order, never
> membership (epic #8, Q14). Everything outside both halves is a consumer adapter. SemEngine owns primitives and
> contracts, never a consumer's domain semantics.

This is the admission rule every row below follows: a row names its **half** (graph or durable execution; "both"
for cross-cutting seams), its tier, and its qualifying consumer. The retained-contract matrix carries a "Half" column
and a "Qualifying consumer" column for that reason. Starter consumers: semsource, semconnect, semboids, semteams
(#22).

## Sources

This design is the architect's draft for SETUP 03B (epic #8, PR #21), with the owner's rulings applied. Facts come
from three reviewed inventory passes in this change:

- `inventory.md` (sections A1–A13): the first pass; the port set at the pin. Cited as "A*n*".
- `inventory-2-scope.md`: rules, workflows, settlement, change observation, storage classes (review CHANGES
  REQUESTED, corrections applied). Cited as "scope Q*n*".
- `inventory-3-pass3.md`: the provider test, the ceiling re-measure, the rule-core seam sketch (review PASS with
  corrections, applied). Cited as "pass3 §*n*".

Rulings, all 2026-10-01, on #8 unless named:

- Q1–Q13: [comment 5929656835](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929656835) (questions
  in [5929442167](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929442167)); the word "tier" on
  [#4](https://github.com/C360Studio/semengine/issues/4#issuecomment-5929716018).
- Q14–Q18, raised in [5929815222](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929815222) and ruled
  with the owner's words in [5929902986](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929902986): Q14
  yes; Q15 in direction (tier 0 includes rules); Q16 engine-owned, with a hypothesis to verify (D13); Q17 yes ("good
  heuristic"); Q18 adopt, with the pace instruction "let's slow down a bit and ensure we have this new scope right".
  That comment held every release until the scope ruling below.
- The tier-0 scope proposal for both halves,
  [5930855353](https://github.com/C360Studio/semengine/issues/8#issuecomment-5930855353), approved as written including
  sequencing by [5930898291](https://github.com/C360Studio/semengine/issues/8#issuecomment-5930898291). It settles Q15
  (a rule core at tier 0), Q16 (`pkg/lifecycle` kept at tier 0) and Q18 (settlement) into rows, releases the holds
  5929902986 kept, and re-rules Q11. It also re-partitions D4 by "needs an external provider" and opens the
  durable-execution primitive as its own epic, [#24](https://github.com/C360Studio/semengine/issues/24).
- BM25's tier, `composition/cli`, the tier model and the extended critical list:
  [5932313950](https://github.com/C360Studio/semengine/issues/8#issuecomment-5932313950), which answers the two owner
  decisions raised by change review round 1 (H1, M1).
- The operator surface, [5931143569](https://github.com/C360Studio/semengine/issues/8#issuecomment-5931143569),
  adopting all four recommendations of the review in
  [5931117077](https://github.com/C360Studio/semengine/issues/8#issuecomment-5931117077).

SemStreams paths are at the pin `8b99efe9`. Each decision keeps its options and their costs as the record of what was
weighed.

## Context

First pass (A1): the port set at the pin is 65 packages / 126,926 non-test lines, reproduced exactly from Codex's
measurement. The 63→65 change is five entrants and three departures (A1.1): `engine`, `flowstore`, and
`internal/lifecyclejoin` no longer exist at the pin. The set reaches an agentic group of 10,264 lines through two
files (`component/dependencies.go:7`, `service/milestone_service.go:10`) and one import in graph-query
(`graphrag.go:22`), and reaches `gateway/graph-gateway` (2,995 lines with `gateway`), which SemSource composes but the
03A workload never addresses (A3.3).

Ruled scope: tier 0 is both halves, with no external provider. The rule core, entity workflows (`pkg/lifecycle`),
clustering on explicit edges, `output/websocket`, the parked-input observer and SemConnect's spatial and temporal
indexes enter. The agentic domain, the gateways and the provider clients leave. Re-measured under that scope with
SemConnect at `dff12657` and semboids at `8c03cc53`, with BM25 at tier 0 and `composition/cli` admitted
(5932313950), **tier 0 is 65 packages / 140,842 non-test lines** (D4).

## Decisions

### D1. Registration cut (#3)

Options:

- **(a) Explicit registration in the consumer's composition root.** Each consumer calls the per-package `Register`
  and `RegisterPayloads` functions it composes (A2.1 lists them; SemSource already does this for its own 14 factories
  at `run.go:301-320` and chains `RegisterPayloads` at `:289-296`). Cost: SemSource replaces two calls
  (`run.go:273,289`) with about eight component and two or three payload registrations; SemConnect builds the
  composition root PR #74 already requires. Benefit: SemEngine ships no aggregator; the compile closure of a consumer
  is exactly what it names; ADR-103's single type authority is unchanged (the registry object is still the one
  authority; only the caller moves).
- **(b) A narrowed engine-side registry** (`Register` admitting only graph components and payloads). Cost: a new
  aggregator that must choose a component set for every consumer at once; each later component widens every
  consumer's closure (the shape that produced 94- and 41-package pulls, A12.1); SemConnect, which composes no
  SemStreams component today, would import components it does not run.
- **(c) Keep importing `componentregistry`/`payloadbuiltins`.** Not available: both pull packages that will not exist
  in SemEngine (the agentic domain, `governance`, `research`, `gated-dag`; A2.1, pass3 §2.1). The rule core and
  `pkg/lifecycle` are now tier 0 (D12, D13) and are registered per package like every other component.

**Ruled (Q1, #3): (a)** — explicit per-package registration in each consumer's composition root; no aggregator, no
engine-side registry. The owner's quality bar: "simple yet detailed is always the preference but it better be solid,
idiomatic go." Measured consequence: removing the two aggregator roots drops `processor/gated-dag`, `pkg/gateddag`,
`payloadbuiltins`, `vocabulary/rulepacks` and `vocabulary/builtins` (−2,758 lines) from the closure (pass3 §2.2, C→H).
Two components SemSource composes through the aggregator become explicit decisions rather than silent imports:
`output/websocket` (composed at `run.go:1060`) and `processor/graph-clustering` (composed under `enable_clustering`,
`run.go:932`). **Q11 re-ruled (5930898291):** both are **tier 0**. `processor/graph-clustering` is clustering on
explicit edges (D4), and `output/websocket` is a transport over applied state (D14); semboids composes both
(scope Q3.2).

All four starter consumers reach an aggregator in production today: SemSource (A2.2); SemConnect at `dff12657`
(`cmd/cs-graph-backend/main.go:25,36` imports, `:110,:123` `builtins.Register()`, `:228`
`payloadbuiltins.Register(payloads)`); semboids (`cmd/semboids/main.go:163` `payloadbuiltins.Register`); semteams
(`componentregistry.Register`, `cmd/semteams/main.go:777`) (pass3 §2.4; scope Q3.1). Each moves to per-package
registration; the matrix's "Registration" row records it. Payload owners SemSource needs under (a):
`message.RegisterPayloads`, `storage/objectstore.RegisterPayloads`, plus `graph/inference.RegisterPayloads` only if
`enable_hierarchy` is set (construction already refuses otherwise, `component.go:735-741`). A consumer that composes
a workflow (semboids) adds `pkg/lifecycle.RegisterPayloads`. A SemEngine contract test (T-B8, see Invariants) forbids
any production package whose import set spans more than one component family — the machine form of "no aggregator".

**D1 adopter-path items** (pass3 §2.6). Under (a) `payloadbuiltins` does not exist in SemEngine, but the pin's error
text and doc comments send the adopter to it. Each is an item on the D1 ledger row (`adapt`):

| Site at the pin | Text | Should name |
| --- | --- | --- |
| `processor/graph-ingest/component.go:739` | boot-time refusal: "enable_hierarchy requires %s in the payload registry (register it with payloadbuiltins.Register)" | `inference.RegisterPayloads` (`graph/inference/container_entity.go:62`) |
| `graph/inference/container_entity.go:14,59` | doc comments naming `payloadbuiltins.Register` | the per-package call |
| `pkg/lifecycle/harness_entity.go:14,63` | same pattern | `lifecycle.RegisterPayloads` |
| `message/generic_json.go:26` | same pattern | `message.RegisterPayloads` |
| `storage/objectstore/stored_message.go:90` | same pattern | `objectstore.RegisterPayloads` |

The `:739` item is adopter-facing: a boot error naming a symbol that will not exist. Proving test: the graph-ingest
refusal text names a symbol that exists in the SemEngine tree (a string assertion in the factory test). SemConnect sets
`enable_hierarchy: true` (`deploy/semstreams.json:46` at `dff12657`), so it is the consumer that reads that error.

### D2. Relationship to ADR-106 (#2)

Options:

- **(a) Run alongside.** SemStreams proceeds to 1.0 under ADR-106 unchanged. SemEngine is a separate module whose
  exported surface is not ADR-106's Tier 1 by construction: 26 of the 65 are not Tier 1 and seven are `internal/`
  packages SemEngine must re-home (A7). SemSource and SemConnect leave the sister-import set when they migrate.
  Cost to SemStreams, not decided here: semsource is one of ADR-106's three canaries (ruling 6, `:90-98`); its
  departure removes one "active sister tracking a tag" from the RC-4 window.
- **(b) SemEngine supersedes.** The 39 overlapping Tier 1 packages leave SemStreams' frozen surface. Cost: a
  SemStreams-side ruling; the other seven sisters import those 39 and would be pointed at an unqualified SemEngine;
  it contradicts plan ruling 3 (frozen pin, no sync in either direction).
- **(c) Shared freeze.** SemEngine promises ADR-106 Tier 1 compatibility for the 39. Cost: SemEngine inherits a
  compatibility promise on types it is about to repair (#15–#17) and on a module path that differs; ADR-106 itself
  measures compatibility by `apidiff` against SemStreams tags.

**Ruled (Q2, #2): (a)**, recorded as: SemEngine is a fork at the pin of 39 Tier 1 and 26 non-Tier-1 packages; it
makes no compatibility promise to SemStreams' Tier 1; a SemStreams change after the pin is its own ledger row
(ruling 3). "Tier 1" here is SemStreams' *package* tier (`release/tier1-packages.txt`, the ADR-106 sister-import
surface), not a SemEngine capability level (D3). The mechanical check is adopted: a SemEngine boundary test that no
file imports `github.com/c360studio/semstreams` (I8). The 39 / 23 / 26 split was measured on the first-pass 65
(A7); under the ruled tier-0 set it changes (`graph/geo/geojson`, `output/websocket`, `processor/graph-clustering`,
`processor/rule`, `processor/rule/expression` and `vocabulary/export` are Tier-1 packages A7 counted outside the 65),
and task 2.2 re-measures it. The canary question (Q2b) is moot; the owner's direction:
"semstreams gets retired if we do this right", so SemStreams' RC window is not a SemEngine constraint.

### D3. The word for a capability level (#4)

Candidates measured against existing spellings: **tier** collides with ADR-106 tiers (`106-…md:52-57`), SemSource
`configs/tiers`, and the plan's own crosswalk; **slice** is already "extraction slice" throughout the plan and
`semengine_slices` in the 03A JSON; **profile** collides with `payloadregistry.Registration.IndexingProfile`
(ADR-103 decision 2, a per-type floor) and matches the 03A evidence's `profile: bm25|neural`
(`pinned-results.json`), which is the thing being named.

The architect proposed "profile" ("capability profile 0/1/2"). The owner, on Q3: "semstreams was tiered because we
have graceful fallback. we need to keep that concept to support offline first and edge deployments with shit comms.
problem was we never standardized what the tier names should be or represent. profile is a good name but it loses
some of the semantics around fallback."

**Ruled (#4, [comment 5929716018](https://github.com/C360Studio/semengine/issues/4#issuecomment-5929716018)):** "the
word is "tier". Tier N = a capability level with named guarantees; a deployment runs at the highest tier whose
providers are available and degrades to tier N−1 when one is lost; every tier's fallback behavior is a
retained-contract-matrix row with a proving test; tier 0 needs no external provider (offline-first, edge). The
standard, not the word, was what SemStreams lacked. Milestones and epics #9–#11 keep their names. "Profile" stays
only as the 03A evidence's run label (`profile: bm25|neural`), which names a configuration, not a level."

**Tier model (ruled, [5932313950](https://github.com/C360Studio/semengine/issues/8#issuecomment-5932313950)).** "A tier
boundary exists only where a dependency can be lost at runtime."

- **Tier 0 = no external provider:** graph foundation, rules, entity workflows, settlement, clustering on explicit
  edges, hierarchy inference, lexical BM25, change observation, operator surface.
- **Tier 1 = embedding provider present:** neural retrieval; losing the provider degrades to tier 0 with search
  intact.
- **Tier 2 = LLM provider:** the slot is defined and empty at MVP. It will hold the clustering summarizer, the
  inference review worker, the query classifier, and the agentic domain when semteams brings it.

Milestones and epics #9–#11 are **slices**, not tiers (the same ruling supersedes "keep their names" above): Slice
04A, tier-0 graph foundation (#9); Slice 04B, tier-0 lexical — BM25 completes tier 0 (#10); Slice 04C, tier-1 neural —
embedding provider (#11). Their milestones carry those titles. This ruling supersedes the approved plan's tier and
slice text (`docs/setup-plan.md`: the baseline and slice table, the capability table, the SETUP 04 slice sections and
the verification paragraph); the plan is amended in place, each passage marked "Amended 2026-10-01 per #8".

**Glossary note.** In this change, "tier N" is a SemEngine capability level as ruled above, and "slice" is an
extraction step. The crosswalk table in the matrix carries SemSource's config file names verbatim
(`tier0-statistical.json`, `tier1-semantic.json`); under the tier model they line up with SemEngine tier 0 (structural
plus BM25) and tier 1 (neural). "Tier 1" in D2 and A7 is SemStreams' *package* tier
(`release/tier1-packages.txt`), a different grammar; so is `payloadregistry.Registration.IndexingProfile` (a
registered type's floor). "profile" appears only as the quoted 03A run label (`profile: bm25|neural`) and for a
coverage profile (A13); `semengine_slices` in SemSource's JSON is not renamed.

### D4. Port set: admission or separation of each questioned package

The Q4 ruling separated sixteen packages. The scope ruling (5930898291) re-partitions them by **needs an external
provider**: the agentic domain and the gateways stay separated; the graph libraries are split by the provider test
(pass3 §1); and two edits that break `processor/rule` at compile time are withdrawn, together with the non-port of
`service/rule_pack_bind.go` (scope Q1.5).

| Package (lines) | Reach (inventory) | Options | Ruling and measured consequence |
| --- | --- | --- | --- |
| `engine`, `flowstore` | absent at the pin (A1.1) | — | **No row**: resolved by the pin; the matrix records "retired upstream by #1116". |
| `agentic` (6,005) + `internal/looptoken` (42) | `component/dependencies.go:7,56-57` (`ToolRegistryReader`); `gateway/graph-gateway/component.go:40`; `processor/rule` (`actions.go:15`, `config_validation.go:9`) | carry; adapt `component` | **Separate (agentic domain).** The independent `component` adapt (drop `Dependencies.ToolRegistry` and `ToolRegistryReader`) is **withdrawn**: it breaks `processor/rule` at `factory.go:148` (scope Q1.5). It returns as a consequence of rule-core edit E3 (D12), compile-safe once E1's registration carries the tool registry (pass3 §3.2). |
| `agentic/agentrun` (1,505) + `internal/deliverylane` (248) + `internal/agentterminal` (188) | `service/milestone_service.go:10`, constructed only by `internal/boot/run.go:573`; `processor/rule` (`actions.go:16`, `agentrun.Mint` `:1989`) | carry; adapt `service` | **Separate (agentic domain; adapt `service`)**: do not port `milestone_service.go`. SemSource's `service.RegisterAll` does not register it (`service/register.go:8-22`). Consumer impact: semteams calls `service.NewMilestoneService` (`cmd/semteams/main.go:941`; scope Q1.5). |
| `pkg/rulepack` (143) | `processor/rule/config.go:14`; `service/rule_pack_bind.go:8`; `frameworkcapabilities/rulepacks/validate.go` | carry; adapt | **Admit (carry; E5).** A rule-pack ID contract, not agentic (pass3 §2.1). The non-port of `service/rule_pack_bind.go` is **withdrawn**: it would strand `Processor.ProjectionBindings()` (`processor/rule/processor.go:443`), the only implementer of `service.ProjectionBinder` (scope Q1.4–Q1.5). |
| `vocabulary/agentic` (2,133) | `processor/graph-query/graphrag.go:1718-1720` (three label predicates); `processor/rule/actions.go:27` (`stampRunAnchors` `:731-753`) | carry; adapt | **Separate (agentic domain; adapt `graph-query`)**: drop the three agentic predicates from `labelPredicates`; the rule edge leaves with E3. Finding: entity display label has two homes — `fusion.Lens.Label` (consumer, `lens.go`) and `graphquery.labelPredicates` (framework, hard-coded). Consolidation is deferred; the agentic spellings go now. |
| `governance` (outside the 65) and `processor/agentic-*` | `processor/rule` (`actions.go:18`, `verdict_auditor.go:10`); `payloadbuiltins` | — | **Separate (agentic domain).** The rule edge leaves with E4 (D12). |
| `pkg/lifecycle` (3,739) | `component/dependencies.go:99`; `service/dependencies.go:36`; `service/component_manager.go:49,199,205,219,1218` (scope Q1.6) | carry; separate | **Admit at tier 0 (carry; Q16).** The removal of the `LifecycleManager` field and its plumbing is **withdrawn**: it breaks `processor/rule` at `factory.go:160-161` and removes the only composition path for `lifecycle_*` actions. Cost of keeping: six pass-through lines plus three import lines (`component/dependencies.go:12`, `service/component_manager.go:25`, `service/dependencies.go:13`). Correction: A1.2's `:996-1111` citation is the local `component.LifecycleComponent`, not `pkg/lifecycle` (scope Q1.6). |
| `composition` (914) + `component/flowgraph` (1,324) | `service/component_manager.go:402` boot path | carry | **Admit (carry)**: ADR-100 boot validation; the home for #16's refusal (D11); the flowgraph response is a tested contract (D15). |
| `pkg/projection/contract` (163), `internal/lifecyclecleanup` (38) | ADR-103 and every processor's one-shot lifecycle | carry | **Admit (carry)**; `internal/*` re-homed under SemEngine's `internal/`. |
| `graph/clustering` (5,516) + `processor/graph-clustering` (4,117) | `processor/graph-query` community path; semboids composes `graph-clustering` | carry; seam; defer | **Admit at tier 0** (pass3 §1). LPA (`lpa.go`) and `StatisticalSummarizer` (`summarizer.go:35-300`) are deterministic; `processor/graph-clustering` builds only `NewStatisticalSummarizer()` (`component.go:1372`) and `EnableLLM` defaults false (`:60,:419,:531`). The compile edge into `graph/llm` (`summarizer.go:12`; `LLMSummarizer` `:499-810`, `enhancement_worker.go`; `processor/graph-clustering/component.go:592,599,2268-2330,2416-2500`) is cut by the capability seam. |
| `graph/inference` (5,338) + `graph/structural` (883) | `processor/graph-ingest/component.go:19` (hierarchy, construction-guarded `:735-741`) | carry; seam; defer | **Admit at tier 0** (pass3 §1). Hierarchy (`hierarchy.go`, 540 lines, no `llm` reference) is provider-free and SemConnect turns it on (`enable_hierarchy: true`, `deploy/semstreams.json:46` at `dff12657`); the first-pass premise "not in the retained workload" is false for SemConnect. `graph/structural` imports `graph` and `pkg/errs` only. The compile edge into `graph/llm` (`config.go:12,123`; `review_worker.go:17`, nil-guarded `:386`) is cut by the seam. |
| `pkg/graphview` (1,193) | `processor/graph-query` (2 files); semboids (5 files) | carry dormant; seam | **Admit at tier 0** (pass3 §1): imports `nats.go/jetstream` only. The view layer of change observation (D14). |
| `graph/llm` (842) + `model/wire` (1,182) | `processor/graph-query/answer.go:11`, `component.go:18,434,467`; `graph/query/classifier_llm_adapter.go:9`; `graph/clustering`, `graph/inference`, `processor/graph-clustering` | carry; seam | **Behind the seam, tier 2** (provider clients; the tier-2 slot is empty at MVP). `graph/llm/openai_client.go` imports `go-openai` (`:12`); `model/wire` is an OpenAI-compatible HTTP client (`client.go:32,84,121,148`). Split shape of `graph/llm` is a design option (below). |
| `graph/embedding` (3,302) + `processor/graph-embedding` (3,253) | `graph/query/classifier_embedding.go`; `processor/graph-embedding/{component,query,readiness}.go` | carry; seam | **Admit at tier 0 (BM25 half; 5932313950, option (a) of change review round 1's H1).** The BM25 embedder (`bm25_embedder.go`, A3.2) runs in process with no provider. The package's one provider file, `graph/embedding/http_embedder.go` (220 non-test lines; the only file importing `go-openai`), moves behind the tier-1 seam as a file-level split, the same shape as `graph/clustering`'s summarizer; its only code caller is `processor/graph-embedding/component.go:931,944` (`embedding.HTTPConfig`, `embedding.NewHTTPEmbedder`). The package's doc comments that name `HTTPEmbedder` (`doc.go:19,42,75,155`, `embedder.go:36`, `worker.go:598`, `dedup.go:69,105`) move or are reworded with the split. Slice 04B completes tier 0 with BM25; Slice 04C adds the tier-1 embedding provider. The option not taken (BM25 at tier 1, set I, 62 / 134,356) stays in the delta table for the record. |
| `gateway/graph-gateway` (2,676) + `gateway` (319) | composed by SemSource (`run.go:852`), not exercised (A3.3); requires `agentic_queries` with no responder; drags `agentic`, `graph/inference` | carry; adapt (drop the agentic port and trajectory decode — a port-contract break); defer-exclude | **Separate (defer-exclude at tier 0)**: layer 4 of D5 is consumer-owned (SemSource's `mcp-gateway`, SemConnect's `cs-api`). Re-admission requires a consumer assertion through its HTTP surface. SemConnect's `cs-api` imports `gateway` only for `var _ gateway.Gateway = (*Component)(nil)` (`gateway/cs-api/component.go:198` at `dff12657`); SemStreams mounts HTTP handlers through an anonymous `RegisterHTTPHandlers` interface (`service/service_manager.go:1550-1555`), so the assertion is compile-time only and dropping it is SemConnect work. Shipping the 319-line base (pass3 set J, 65 / 141,230) is not taken. |
| `processor/rule` (15,826) + `processor/rule/expression` (1,417) | outside the 65; semboids composes it (`componentregistry/register.go:12,23`) | admit as rule core | **Admit at tier 0 as the rule core (Q15; D12)**, full lines counted; edits E1–E4. |
| `internal/maxdelivery` (309) | started only by `internal/boot/run.go:184` | port; leave | **Admit at tier 0 (Q18; D13)**: the parked-input observer; re-homed under SemEngine's `internal/`. |
| `processor/graph-index-spatial` (1,526), `processor/graph-index-temporal` (1,573), `graph/geo/geojson` (1,030), `vocabulary/export` (1,107) | SemConnect at `dff12657` (pass3 §2.1) | admit | **Admit at tier 0** (Q8; D9). |
| `composition/cli` (151) | outside the 65; imports `component`, `composition`, `config` only | admit; defer | **Admit at tier 0 (5932313950):** the no-UI documentation path needs it (D15). |
| `pkg/tlsutil` (533) + `pkg/acme` (543) | `metric/handler.go:20` | carry; adapt metric | **Carry** (metrics TLS; small). |
| `health`, `internal/logforwarderpolicy`, `internal/componentadmission` | `service` | carry | **Carry.** |
| `output/websocket` (2,220) | outside the 65; composed by SemSource (`run.go:1040-1091`) and semboids (`flock.json:207-239`) | admit; defer-exclude | **Admit at tier 0 (Q11 re-ruled; D14)** as the consumer transport over applied state. Its only input is a NATS subject (`component.NATSPort`; `output/websocket/doc.go:17,53`), so reading applied state is a port change, a `class:port-refactor` (D4a). |

**Tier-0 ceiling (Q8 re-measure; 5932313950).** Tier 0 is **65 packages / 140,842 non-test lines**: pass3 set H (64 /
140,911; the ruled roots plus SemConnect at `dff12657`, including `graph-index-spatial` and `graph-index-temporal`, and
semboids at `8c03cc53`, after cutting the agentic domain, the gateways, `graph/llm` and `model/wire`, with D1 applied)
minus the one provider file in `graph/embedding` (`http_embedder.go`, −220) plus `composition/cli` (+1 package, +151).
Measured by the writer from pass3 set H; the architect confirms at 04A's first `go list`. Commands, run in the pin
snapshot (`scratchpad/semstreams-8b99efe9/`):

```bash
# the provider files among set H's 64 packages (scratchpad/ceiling-H.txt)
for p in $(sed 's#github.com/c360studio/semstreams/##' ../ceiling-H.txt); do
  grep -l 'sashabaranov/go-openai' "$p"/*.go | grep -v _test.go
done
# → graph/embedding/http_embedder.go; model/registry.go (a comment at :347, not an import)
wc -l graph/embedding/http_embedder.go                                          # → 220
find composition/cli -maxdepth 1 -name '*.go' -not -name '*_test.go' | xargs cat | wc -l   # → 151
# 140,911 − 220 + 151 = 140,842; 64 + 1 = 65
```

The behind-the-seam set is now `graph/llm`, `model/wire` and `graph/embedding/http_embedder.go` only. The figure
assumes `graph/llm` split fork (a); fork (b) admits the 477-line provider-free part of `graph/llm`, giving 66 /
141,319 (design options, below).

| Set (pass3 §2.2; K by the writer) | Packages | Non-test lines | Δ vs 65 / 126,926 | Δ vs 67 / 129,063 |
| --- | --- | --- | --- | --- |
| A. The 65 at the pin (first pass) | 65 | 126,926 | 0 / 0 | −2 / −2,137 |
| B. Ruled roots + SemConnect + semboids, no cuts | 79 | 156,273 | +14 / +29,347 | +12 / +27,210 |
| C. B after the agentic, gateway and LLM-client cuts | 69 | 143,669 | +4 / +16,743 | +2 / +14,606 |
| H. C with D1 applied (embedding family kept) | 64 | 140,911 | −1 / +13,985 | −3 / +11,848 |
| I. H without the embedding family (BM25 at tier 1; not taken) | 62 | 134,356 | −3 / +7,430 | −5 / +5,293 |
| **K. H − `http_embedder.go` + `composition/cli` — tier 0 (ruled)** | **65** | **140,842** | **0 / +13,916** | **−2 / +11,779** |
| J. H plus the `gateway` base (not taken) | 65 | 141,230 | 0 / +14,304 | −2 / +12,167 |

Entering against the 65 (10 packages, +29,276): `processor/rule` 15,826, `processor/rule/expression` 1,417,
`processor/graph-clustering` 4,117, `output/websocket` 2,220, `processor/graph-index-temporal` 1,573,
`processor/graph-index-spatial` 1,526, `vocabulary/export` 1,107, `graph/geo/geojson` 1,030, `internal/maxdelivery`
(309 lines) and `composition/cli` 151. Leaving the 65 (10 packages, −15,140): `agentic` 6,005, `gateway/graph-gateway`
2,676, `vocabulary/agentic` 2,133, `agentic/agentrun` 1,505, `model/wire` 1,182, `graph/llm` 842, `gateway` 319,
`internal/deliverylane` 248, `internal/agentterminal` 188, `internal/looptoken` 42 (pass3 §2.2). Split out of a kept
package: `graph/embedding/http_embedder.go`, −220. The number is a reachability cut over the pin's `go list` edges, not
a `go list -deps` on a tree where the seam exists (pass3 §4); Slice 04A's first `go list -deps` replaces it.
Reproduction commands: pass3 §2.5 and the block above.

**Kept packages that still reach a cut package** (pass3 §2.4; eight packages). Each edge is a `class:port-refactor`
(D4a):

| Kept package | Edge into a cut package | Import site | What severs it |
| --- | --- | --- | --- |
| `component` | `agentic` | `component/dependencies.go:7`; `ToolRegistryReader` `:47-57`; `Dependencies.ToolRegistry` `:76` | the D4 `component` adapt, re-enabled by E3 (D12) |
| `service` | `agentic/agentrun` | `service/milestone_service.go:10` | non-port of `milestone_service.go` |
| `processor/rule` | `agentic`, `agentic/agentrun`, `governance`, `vocabulary/agentic` | `actions.go:15,16,18,27`; `config_validation.go:9`; `verdict_auditor.go:10` | E1–E4 (D12) |
| `processor/graph-query` | `graph/llm`, `vocabulary/agentic` | `answer.go:11`, `component.go:18`; `graphrag.go:22` | capability seam; label-predicate drop |
| `graph/query` | `graph/llm` | `classifier_llm_adapter.go:9` | capability seam |
| `graph/clustering` | `graph/llm` | `summarizer.go:12` | capability seam (`graph/llm` split shape, below) |
| `graph/inference` | `graph/llm` | `config.go:12`, `review_worker.go:17` | capability seam (`graph/llm` split shape, below) |
| `processor/graph-clustering` | `graph/llm` (and `model` for endpoint resolution) | `component.go:592,599`, `:2294,:2492`, `:2269` | capability seam: `startEnhancementWorker` (`:2268-2330`) and `startReviewWorker`/`resolveReviewLLMClient` (`:2416-2500`) behind the optional-capability interface |

`graph/query`'s `graph/embedding` edge (`classifier_embedding.go`), pass3 §2.4's "one remaining provider edge in H",
stays: `graph/embedding` is tier 0, and its provider half is the file split at
`processor/graph-embedding/component.go:944` (D4a).

**Ruled (Q4): separations ordered; port refactors tracked.** The agentic-domain and gateway cuts land first. The
behind-the-seam code (`graph/llm`, `model/wire`, and `graph/embedding/http_embedder.go`) is carried dormant through the
first green tier-0 extraction (Slice 04A), so the 03A attribution of that extraction is not muddied by a refactor made
blind at the pin. The capability seam (in `processor/graph-query`, `graph/query`, `graph/clustering`, `graph/inference`,
`processor/graph-clustering`, and the `http_embedder.go` split with its caller in `processor/graph-embedding`) is the
**last task of 04A**, with exit condition "tier 0 compiles without the behind-the-seam packages and files". The dormant
bridge is bounded to that one change, not deferred to Slice 04B. Q4's original set of six libraries is superseded by the
provider test: four of the six are now tier 0.

#### D4a. Port refactors are a tracked risk class

The owner, on Q4: "ensure we track this type of move as i think this is our big class of risk in this migration."
**Ruled:** restructuring ported code away from the pin's shape (a seam, a file-level adapt, a dropped field or
method) is the named risk class `class:port-refactor`. Every such move is:

- an admission-ledger row with disposition `adapt` whose `contract` field cites its retained-contract-matrix row and
  whose `proving_tests` field names the test that proves the retained behavior; and
- carried by an issue labeled `class:port-refactor`, so attribution against the 03A baseline stays queryable.

The ledger schema is closed: T-B7 (`internal/harness/contract/ledger_test.go`, `ledgerFields`) rejects any key
outside the ten fields, and `harness-boundaries` › "Admission ledger" states the same schema. The class is therefore
recorded in existing fields (matrix row in `contract`, test in `proving_tests`, `class:port-refactor` in
`known_risks`) and the ledger header documents that convention (task 2.5). A dedicated field would need a T-B7 code
change and a `harness-boundaries` spec delta; it is not part of this change. The label `class:port-refactor`
exists in the repository (created 2026-10-01, after this change's base).

Known port refactors at tier 0:

- the rule core's edits E1–E4 (D12);
- the `component` adapt (drop `Dependencies.ToolRegistry` and `ToolRegistryReader`), re-enabled by E3;
- the `service` non-port of `milestone_service.go`;
- the `graph-query` label-predicate drop;
- the capability seam in `processor/graph-query`, `graph/query`, `graph/clustering`, `graph/inference` and
  `processor/graph-clustering`;
- the D1 adopter-path text (D1);
- the `output/websocket` port change, from a NATS-subject input to applied state (D14);
- the `graph/embedding` file-level split: `http_embedder.go` behind the tier-1 seam, with its caller at
  `processor/graph-embedding/component.go:931,944` and the package doc comments that name `HTTPEmbedder` (D4);
- the re-home of `internal/maxdelivery` and the way a consumer composes it (D13).

The technical-writer files one `class:port-refactor` issue per item before any 04A change claims it (task 7.2).

The `LifecycleManager` removal and the `rule_pack_bind.go` non-port are no longer refactors (withdrawn). The
repair-before-port rows (D11), including settlement, also change code, under their own disposition; whether their
issues carry `class:port-refactor` as well is not ruled here.

### D5. Fusion and graph-tool boundary

The four layers, mapped to code at the pin:

1. **Graph substrate** (SemEngine): `graph`, `graph/readiness`, `graph/query` (classifier; its
   `classifier_llm_adapter.go:9` and `classifier_embedding.go` imports move behind the D4 seam), the graph processors
   (`graph-ingest`, `graph-index`, `graph-index-spatial`, `graph-index-temporal`, `graph-query`, `graph-clustering`),
   `storage/objectstore`, `storage/storeregistry`, `pkg/projection`, `internal/graphmutation`. `graph/embedding` and
   `processor/graph-embedding` are tier-0 substrate for BM25; the HTTP embedder is tier 1 (D4).
2. **Generic deterministic fusion** (SemEngine): `pkg/fusion` lens path — `Engine.Fuse`, facets, budget, hydrate,
   provenance, partial results (`engine_lens.go`, `engine_facets.go`, `engine_graph.go`, `hydrate.go`,
   `contract.go`), `fusionnats`, `fusionvocab`. **Keep.** The package-level `fusion.Fuse` / `SubQuery` /
   `GraphQueryClient` / `Evidence` path (`engine.go:96`, `subquery.go`, `client.go`, `evidence.go`) has zero
   consumer in SemSource or SemConnect and its named caller is `research-graph-execute` (`doc.go:7-8`): **defer
   (not ported)**.
3. **SemSource lenses and policy** (consumer): `source/fusion/lens/{code,docs}`, supersession, symbol policy. Not
   ported.
4. **Consumer-owned public adapters**: SemSource `mcp-gateway` and `code-context` HTTP; SemConnect `cs-api`.
   SemStreams' `graph-gateway` is not the SemEngine adapter (D4).

`graph.query.searchGraph`: **keep at tier 0/1 under its degraded contract** (exact or semantic fallback, unavailable
enrichment reported, `query.go:63`); **change** by the D4 seam for the LLM-answer path. With `graph/clustering` now
tier 0, `processor/graph-query` keeps no edge into a cut clustering package (pass3 §2.4); which enrichment a tier-0
deployment reports depends on whether `graph-clustering` is composed, and the 04A change that ports `graph-query`
measures it. The 03A observation `graph_search_public_query` is its proving test at each tier. `find/anchor/ask` as
cases, not a mode API: *find* = `ResolveModeSymbol`/`Prefix` + `Names` (`lens.go:86-94`, `retrieval.go`; observed
`cold_duplicate_name_anchors`); *anchor* = `Entity`/`Entities` by ID (hydration; observed `*_exact_relationship`,
`*_exact_content`); *ask* = `ResolveModeNL` and `searchGraph` (observed `neural_paraphrase`,
`graph_search_public_query`). No new mode enters the contract. Known fusion defects: SS#621 (open,
`class:unobserved-skip`) and SS#603 (open, post-v1) are matrix rows marked "reproduce at the pin before carrying as a
current defect" per the plan; neither was reproduced in 03A.

### D6. Applied-input barrier (#18)

Options: (i) a barrier primitive owned by graph-ingest (per-producer applied census, incarnation-aware); (ii) a
per-producer applied census bucket; (iii) **durable applied facts on the owning entity** — the restart-safety L4a
precedent in the pin (SS PR #1361, `c4a79fd5`: agentic-loop recovers redelivered inputs from facts stamped on the
`LoopEntity` itself, SS#1147 hierarchy item 2, no new bucket), here a per-source applied marker on the source's own
entity; (iv) **defer** with the consumer consequence recorded. Evidence: no primitive
exists (A11 row 4); SS#1147's binding hierarchy admits additional component authority "only when a named failpoint
proves the first three insufficient" and lists a generic outcome/checkpoint bucket as an anti-goal; the consumer need
(semsource#215 retired-source completion) is not in the 03A retained workload (`source_removed` markers are absent
at both pins and recorded as pre-existing, `compatibility.md:244-251`). **Ruled (Q5, #18): (iv) defer**, with two
records: the consequence (SemSource has no supported completion signal for retired sources and keeps
`applied_tail_unproven` as a declared limit), and a design constraint on D11 #15: the stream-generation identity
the #15 repair introduces is the one home a later barrier would key on — it must be a named, readable fact, not a
private encoding, so #18 does not later add a second spelling of "which stream incarnation". The ruling names
(iii), durable applied facts on the owning entity (L4a), as the recorded precedent if a barrier is ever needed,
keyed on the generation fact from #15; the consequence is recorded on #18 and semsource#215. A general barrier, if it
returns, belongs with the durable-execution primitive (#24, D13), not with graph-ingest.

### D7. Conditional reconcile at a caller-observed revision (#19)

Options: (i) add `ExpectedRevision uint64` to `projection.ReconcileMutation`, honoured by `MutationClient.Reconcile`
(zero keeps today's owner-read behavior); (ii) a separate `ReconcileAt` operation; (iii) defer. Evidence: two of two
consumers need the fence — SemConnect already sends `graph.ReconcilePredicatesRequest{ExpectedRevision:
exact.KVRevision}` over the raw subject (`graph_mutations.go:214-215`), the path `internal/graphmutation/protocol.go:2`
says applications should not take; SemSource cannot through the typed client (`mutation_client.go:191,199`). The wire
already carries the field (`graph/mutation_requests.go:29`); the owner already fences on it
(`canonical_mutations.go:349-355`). Against SS#1147's hierarchy this is item 2 (stable identity plus deterministic
reconciliation) with no new durable state: the fence is the revision the owner already keeps, so it is the cheapest
rung, not additional authority. The consumer design it serves is SemSource PR #213 (merged `3604a9ce`): consumer-private
source-lifecycle machinery that today reports `conditional_reconcile_unavailable` and leaves matching entities stale.
**Ruled (Q6, #19): (i)** — `ExpectedRevision` on `projection.ReconcileMutation`, surfaced through the typed client,
as the one typed home; SemConnect's raw `graph.mutation.>` send moves to the typed client as consumer work, recorded
on SemConnect's ledger row (closing the parallel raw-wire channel). The zero-value fallback is a predict-vs-observe
seam only if a caller forgets the field; the compensating rule is a `MutationError` kind `revision-conflict` that
names the observed and current revisions so a wrong fence is loud, never a silent re-read. Proving tests: read R /
concurrent write R+1 / reconcile at R → conflict, no clear; unchanged R → applied; both consumers' fixtures.

### D8. Commit ambiguity and unknown outcomes (#20)

Options: (i) carry `CommitUnknown` end to end plus a terminal operation-status lookup by request ID; (ii) an
owner-enforced generation fence; (iii) defer. Evidence narrows (i): the client already has `CommitUnknown` and
`unknownMutation` (`mutation_types.go:74`, `mutation_client.go:404-417`); the defect is one server conversion
(`canonical_mutations.go:360` → `rejectFromError` `mutation_runtime.go:218` → `rejectInternal` `:214-216`) that turns a
backend write error of uncertain effect into a classified error, and one client predicate (`isDefiniteFailure`,
`:419-422`) that treats every classified error as definite. **Ruled (Q7, #20): (i-narrow) only** — a repair-before-port
row spanning `processor/graph-ingest` and `pkg/projection`: the owner distinguishes rejected-before-effect (invalid,
conflict, not-found) from effect-uncertain (a new coded outcome, class transient-unknown), and the client maps only
the former to `CommitNotCommitted`. The repair has a consumer half: SemConnect bypasses `pkg/projection` and
classifies replies itself (`graph_mutations.go:113-117`; `systems_post.go:465-469` maps `ErrorCodeInternal` to a
500), so as a first-wave consumer (D9) it must map the new effect-uncertain code to its existing
`commitUnknownError` path — consumer work recorded on its row, not a framework task. **Defer** the
status-lookup-by-request-ID and the generation fence (a same-class collision with SS#1147's "generic outcome bucket"
anti-goal; the graph already offers the observable alternative: re-read the authoritative entity and compare).
Consumer consequence recorded honestly: **no consumer resolves an unknown outcome today** — SemConnect returns HTTP
503 "verify resource state before retrying" to its caller (`systems.go:1034-1042`) and SemSource (PR #213) keeps a
durable fence and refuses re-admission; after the repair both at least receive the truthful classification, and
resolution remains re-read plus the consumer's own fence. Proving test (ruled): an injected KV `Update` timeout in
the harness, and the caller observes `CommitUnknown`.

### D9. SemConnect's status for SemEngine

Options: **(a)** reference fixtures only, port set 65; **(b)** first-wave consumer, port set widened by
`graph/geo/geojson` (1,030) and `vocabulary/export` (1,107) → 67 packages / 129,063 lines (A10), both leaves
(`export` imports only `message` and `vocabulary`); **(c)** SemConnect copies the two packages. Evidence: the plan
places "generic registration and export mechanisms" in the engine (`setup-plan.md:103`); `vocabulary/cco` and `bfo`
are already in; (c) duplicates 2,137 generic lines in a consumer. **Ruled (Q8): (b)**, SemConnect is a first-wave
(tier 0) consumer, with SemConnect's 03A before/after (once PR #74 records it) entering the matrix as a second
observed column. The owner: "semconnect touches a few different parts we should get right at t0." The seams it
exercises are tier-0 matrix rows with harness proving tests: entity identity and authority (ADR-102 d5 import-lane
rule and read-only mirrors; ADR-104 unique `platform.id`), the typed mutation client, projection,
artifact/`StorageReference`, generic vocabulary/export, spatial and temporal indexes, and hierarchy inference
(`enable_hierarchy`). The federated-create expectation in semconnect#74
(`TestSetup03AFederatedDatastreamCreate`) is SemConnect's pre-ADR-102 contract; its resolution (import lane as
mirror, or mint locally and cite via `@id`) is SemConnect work, not an engine change. Its outcome is now recorded on
semconnect#74 (PR body, head `dff12657`, 2026-10-01): HTTP 201 at beta.160 and HTTP 400 `authority_foreign` at the
pin, the d5 rejection this matrix keeps. SemConnect's cutover is not an MVP acceptance gate (the plan's acceptance is
SemSource); its composition root is SemConnect-owned work PR #74 already lists.

**Q8 re-measure (measured).** At `dff12657`, SemConnect's production reach
(`cmd/cs-graph-backend`) is 26 SemStreams packages, including `processor/graph-index-spatial` and
`processor/graph-index-temporal` (pass3 §2.1). With semboids at `8c03cc53` (19 packages) and the ruled keeps, the
ceiling is the tier-0 number in D4, **65 / 140,842**, which replaces 67 / 129,063. SemConnect's ledger rows follow it.

### D10. Critical package list for the 80% coverage gate

Selection rule: packages that hold an admission gate (integrity, silent loss, context ownership, durability,
authority/readiness, metadata/content preservation). **Ruled list (Q9):** `processor/graph-ingest`,
`processor/graph-index`, `processor/graph-embedding`, `processor/graph-query`, `graph`, `graph/readiness`,
`graph/embedding`, `pkg/projection`, `internal/graphmutation`, `pkg/fusion`, `pkg/fusion/fusionnats`, `natsclient`,
`service`, `config`, `composition`, `component`, `storage/objectstore`, `storage/storeregistry`, `message`,
`payloadregistry`. Not gated at 80% (tested, not critical): `pkg/errs`, `pkg/types`, `types`, `metric`, `health`,
`model`, the vocabularies, `pkg/{buffer,cache,retry,dispatch,worker,resource,revlag,timestamp,security,platform,
tlsutil,acme}`, `fusionvocab`. Baseline coverage at the pin is **not established** (A13); the gate is new (plan
`:227-229`), so the first measurement is taken when each package lands and recorded on its PR. **Extended
([5932313950](https://github.com/C360Studio/semengine/issues/8#issuecomment-5932313950)) with the packages entering tier
0:** `processor/rule` (the rule core, with `processor/rule/expression`), `pkg/lifecycle`, `graph/clustering`,
`processor/graph-clustering`, `pkg/graphview`, `output/websocket`, `internal/maxdelivery` (the `MaxDeliver` observer),
`graph/inference`, `graph/structural`, `graph/embedding` (its BM25 half; `http_embedder.go` is tier 1; already on the Q9
list), and `composition/cli`. The gate applies to a package when it is admitted at its tier; code carried dormant is not
gated until it is admitted. The `task cover:check` targets for these packages are set by the 04A change that ports each
(no Taskfile change here). `processor/rule/expression` is gated with the rule core: D12 counts it in the core, and it
evaluates every rule condition (change review re-check). ADR-102 d5 is enforced in `processor/graph-ingest`
(`authority_gate.go`, `mutation_runtime.go` emit `authority_foreign`; change review round 1, N5), which is on the list:
the confirmation Q9 asks for.

### D11. Repair-before-port rows and the tier-0 admission gate (Slice 04A, #9)

| Row | Package(s) | Matrix row | Repair shape (design constraint, not implementation) | Gate evidence before admission |
| --- | --- | --- | --- | --- |
| #15 (SS#1442) | `processor/graph-ingest` | acknowledged writes → recovery after transport recreation | the applied-sequence guard key carries a stream-generation identity; equal generation keeps `seq <= last`; a new generation never suppresses current source state | failing-first real-NATS test: retained guard, recreated stream with lower sequences, re-ingest applies; the two 03A failures turn green on SemEngine |
| #16 (SS#1143) | `graph` (declaration), `processor/graph-ingest`, `processor/graph-query`, `composition` | RPC plane / stream-filter safety | one reserved-subject declaration (one home for the 4 port-set literal sites, A4); `composition.Analyze` refuses any stream whose subjects cover a reserved request subject; error names stream, filter, subject | failing-first real-NATS PubAck collision with an unsafe composition; refusal before readiness; SemSource's explicit list starts and answers |
| #17 (SS#1443) | `config` | config crosswalk → desired-state deletion | `syncFromKV` applies component deletion symmetrically with its Services reset (tombstone or reset-and-overlay); a deleted file-declared component does not boot | SemSource's reproduction ported into `internal/harness`; remove → restart → absent; re-add → live |
| #20 (SS#1446) | `processor/graph-ingest`, `pkg/projection` | acknowledged writes → commit classification | D8 | unit + real-NATS: injected KV timeout → `CommitUnknown`; conflict/not-found → `CommitNotCommitted` |
| #19 (SS#1445) | `pkg/projection` | update semantics → conditional reconcile | D7 | the two D7 cases |
| Settlement (Q18) | `processor/graph-ingest`, `natsclient` | Settlement → durable consumer | keep graph-ingest's order (apply → durable guard stamp → in-memory stamp → `Ack`, `keyed_ingest.go:144-229`; "Guard stamp AFTER side effects, BEFORE ack" `:208`); graph-ingest becomes the first non-agentic caller of the #759 settlement surface (`DeliveryWork`, `DeliveryDecision`, `DeliveryRetryPolicy`; `natsclient/delivery_settlement.go`, called today through `internal/deliverylane/deliverylane.go:118,152` and directly by `agentic/agentrun` and `processor/agentic-{dispatch,governance,loop,model,tools}`, all agentic); `InProgress` heartbeat for long applies (`HeartbeatDeliveryPolicy` `:139-155`; graph-ingest has 0 `InProgress` calls) (scope Q4.1–Q4.2) | failing-first: process kill mid-apply on a file stream in the harness; after restart the message is redelivered and either judged stale by the durable guard or re-applied idempotently; no input is lost and the entity state equals the state of one application. No such test exists at the pin or in the four consumers (scope Q4.4); it needs the SETUP 02 harness extension (task 7.1) |
| SS#1411 | `component` (ruled at SETUP 02) | component/service seams → owner lifecycle | one owner-lifecycle guard composed by the 7 port-set copies (A5) | lifecycle-suite over each ported component |
| SS#1415 | `config` | component/service seams → Stop authority | `Manager.Stop(ctx)` | lifecycle-suite; no timeout parameter remains |
| SS#1218 | `service`, `pkg/errs` | component/service seams → shutdown sentinel | one `ErrAlreadyStopped` | unit test, written by the developer in the 04A change that ports `service`: a stopped service and a stopped component both return the one `ErrAlreadyStopped` under `errors.Is` |
| SS#1220 | `service` | component/service seams → registry after failed start | spec statement of the mode-independent clear (option 1 or 2 of the issue) | spec scenario |
| SS#1417 | ported tests in the 8 packages with 105 entries (69 entries / 6 copies bind at tier 0, D11 recompute) | test debt | ported tests must pass `scripts/cleanup-roots-check.sh` (already a `task verify` step) | the guard |
| SS#1145/#1147 | all | restart behavior | recorded as the lifecycle-suite floor plus per-package restart promises; no new recovery subsystem | lifecycle-suite `Promise{Restart}` per component |

Each is a `repair-before-port` ledger row; none is waivable by budget or coverage. **Ruled (Q12):** the mapping is
approved. The tier-0 gate (#9) is: every row above whose package is in the tier-0 set has its gate evidence green on
SemEngine before the package is admitted. The Q12 ruling bound 68 cleanup-baseline entries and 6 `lifecycleUsed` copies,
measured for the first-pass tier-0 set. Recomputed from A5 for the ruled set (105 entries and 7 copies in the 65:
graph-gateway 32, graph-index 27, service 20, graph-embedding 13, agentrun 4, pkg/dispatch 4, objectstore 4,
pkg/lifecycle 1), with BM25 at tier 0 the gate binds **69 entries / 6 copies** (graph-gateway and agentrun out;
graph-embedding and pkg/lifecycle in). The ten packages entering under the scope ruling (D4) are not measured
(declared cost).

**Ruled (Q18):** the settlement row above is adopted as repair-before-port, with the process-replacement proving test
in the harness. **Ruled (Q13):** SemStreams PR #1437 (open, after the pin) is a ledger-row candidate for its
`processor/graph-ingest` half only, taken up with the graph-ingest repair rows (#15, #20, settlement); the row cites
the PR head SHA. Its `agentic/loop_execution_entity.go` half is separated by Q4.

### D12. Rule core (Q15)

**Ruled (Q15, 5930898291):** tier 0 includes a **rule core**: `processor/rule` minus the `publish_agent` (agentic) and
`deny`/`approve` (governance) action families, with the `lifecycle_*` actions kept. Qualifying consumer: semboids now
(7 actions: `publish` ×6, `lifecycle_transition` ×1; `configs/rules/zone-steering/*.json`); semteams later (its
`publish_agent` actions belong to the agentic tier). Full lines are counted: 15,826 + `expression` 1,417 = 17,243
(pass3 §3.1).

The seam (pass3 §3.2), each edit a `class:port-refactor` ledger-row candidate (`adapt`, `contract` = its Rules matrix
row, `proving_tests` named):

- **E1 — action-family registration in the core.** Dispatch is a closed `switch action.Type` in
  `ActionExecutor.Execute` (`actions.go:916-945`); its `default:` arm returns `unknown action type` (`:943`). E1 makes
  the `default:` arm consult a registry of `ActionFamily` values (an `Execute` and a `Validate` hook; `Validate` called
  from the per-action loop in `config_validation.go`). Core families stay compiled in: `publish`, `add_triple`,
  `remove_triple`, `update_triple`, `reconcile_predicates`, `update_kv`, `lifecycle_transition|complete|fail`.
- **E2 — `Action` keeps its JSON shape but loses the agentic types.** `ResponseFormat *agentic.ResponseFormat`
  (`actions.go:191`) and `ToolChoice *agentic.ToolChoice` (`:206`) become raw JSON with unchanged tags; the
  `publish_agent` family decodes and validates them. The package decodes without `DisallowUnknownFields`, so deleting
  either field would silently drop that key from every rule carrying it; semteams uses `tool_choice` in 31 rule JSON
  files and `response_format` in none.
- **E3 — `publish_agent` moves to the agentic tier.** The const (`:50`), its executor and stamp helpers
  (`:1445-2090`, `stampRunAnchors` `:731-753`), its config checks (`config_validation.go:346,358,396,407`), and the
  tool-registry plumbing (`actions.go:542,769`; `processor.go:82,347,687-689`; `factory.go:148`) leave the core. The
  family's dependencies (tool registry, lifecycle manager for `agentrun.Mint` `:1989`) arrive through its E1
  registration. Consequence: the D4 `component` adapt becomes compile-safe again.
- **E4 — `deny`/`approve` and `VerdictAuditor` move to the governance tier** (the ruled "minus `deny`/`approve`"):
  consts `:56,:63`, `executeDeny`/`emitVerdictAudit`/`executeApprove` (`:2101-2248`), the `VerdictAuditor` interface
  (`:497-503`), `verdict_auditor.go`, and the wiring at `processor.go:714-716`. `deny.go` stays in the core: a
  `*DenyVerdict` short-circuits the chain (`stateful_evaluator.go:427`, `cron_scheduler.go:610`), and the governance
  family returns it. Present tier-0 users of `deny`/`approve`: semboids 0, semteams 0. The architect's alternative
  (keep both in the core and retype the auditor to primitives, pass3 §3.2 E4(ii)) is recorded and not taken.
- **E5 — `pkg/rulepack` carries (no edit).** `config.go:148-177` validates `pack_id` through
  `rulepackcontract.ValidateID`; `service/rule_pack_bind.go` (ported) binds `ProjectionBindings()` at boot.

Files touched in `processor/rule`: `actions.go`, `config_validation.go`, `verdict_auditor.go`, `processor.go`,
`factory.go` (plus test splits); `actions_lifecycle.go` and `pkg/lifecycle` are untouched (pass3 §3.2). Rule JSON is
unchanged for every consumer under E1–E4 (pass3 §3.4): semboids decodes and executes 7/7 on the core; semteams decodes
119/119 and executes 70/119, the 40 `publish_agent` actions needing the agentic family registered.

Consumer debt recorded, not engine work: semteams' 9 `replace_owned` actions (5 files under
`configs/rules/dev-via-test/`) name a type that does not exist at the pin (`actions.go:33-75`; retired for
`reconcile_predicates` per `docs/operations/36-graph-foundation-breaking-cutover.md:43,97`). They already fail at fire
time on the pin; semteams owes that migration independently of the seam.

**Open (held, owner):** whether an action whose `type` is neither core nor registered is refused when the rule pack
loads or fails when the rule fires. Today it passes config validation and fails at fire time (`actions.go:943`; no
type check in `config_validation.go`). Refusing at load turns an adopter's log line into a boot error, but is a
behavior change from the pin; on the semteams corpus it would reject the five `replace_owned` files at boot even with
the agentic family registered (pass3 §3.3). Task 4.9 carries the hold.

Where the core's `ProjectionBindings` get bound in a SemEngine composition is not established: at the pin
`service.ConfigureRulePackMutations` is called only from `internal/boot/run.go:330` (scope Q1.4), which is not ported.
The 04A change that ports the rule core names the call site.

### D13. Durable execution at tier 0

**Ruled (scope proposal, approved):** the durable-execution half has a tier-0 floor now, and the primitive is its own
epic. The 03B matrix carries only the floor rows.

- **Entity workflows (`pkg/lifecycle`, Q16): keep at tier 0.** Qualifying consumers: semboids (its own `flock.boid`
  workflow: `lifecycle.NewManager` and `.Register(boidgraph.BoidWorkflow())`, `cmd/semboids/main.go:177-178`;
  `Dependencies.LifecycleManager` set at `:190`; the `predator-cull` rule's `lifecycle_transition` drives the cull
  loop, `internal/sim/lifecycle.go:114-173`) and semteams (the framework's `agent-run` workflow, agentic tier). Cost:
  six pass-through lines and three imports (D4). What it has and lacks (scope Q2.1): revision-fenced CAS on the entity
  write (`manager.go:29,296,523`; `updateRetries = 5`, `:489-545`), state as triples in `ENTITY_STATES`, and
  `Watch`/`WatchEvents` bootstrap replay; no effect fences, journal, receipts, effect retry, replay scan, or
  generation.
  **Q16's hypothesis** (5929902986: SemSource rolled its own because `pkg/lifecycle` was missing) has its inventory
  outcome: **overlapping in part**. SemSource's `sourcelifecycle` implements durable-execution concerns `pkg/lifecycle`
  does not model (journal, effect fences, receipts, retryable blockers, replay scan, seed seals, generation); its
  stated reasons were a classification decision plus pin constraints (semsource
  `openspec/changes/replay-source-removal/design.md:53-70`; scope Q2.2–Q2.4). That gap is the durable-execution
  primitive's, epic #24, not `pkg/lifecycle`'s; the Q5/Q7 deferrals stand.
- **Settlement (Q18): keep graph-ingest's order** and make it the first non-agentic `natsclient` settlement caller,
  with an `InProgress` heartbeat for long applies (D11 settlement row). Qualifying consumers: semboids (file stream
  `ENTITY`, about 200 entity messages/s at `graph_hz: 1`; scope Q3.2) and semsource.
- **Parked input after `MaxDeliver` (Q18): port the observer.** `MaxDeliver` defaults to 3 everywhere
  (`component/port_jetstream.go:130`; no starter consumer declares it). `config` already provisions the fixed
  `MAX_DELIVERY_EVENTS` stream (file, 168h, 64 MiB; `config/streams.go:188-196,289-292`) from `EnsureStreams`, which
  all four consumers call (semsource `run.go:256`, semboids `main.go:138`, semteams `main.go:705`, semconnect PR #74
  `cs-graph-backend/main.go:224`). The observer, `internal/maxdelivery` (`observer.go:1-6`), is started only by
  `internal/boot/run.go:184`, which no consumer uses (scope Q4.3). It becomes a tier-0 component. Because it is an
  `internal/` package, how a consumer composes it without `internal/boot` (an exported component, or started by
  `service`) is a design point for the 04A change that ports it.
- **Recovery per storage class** replaces I7 with three rows, one consumer each (scope Q6.4): memory stream (SemSource
  `GRAPH`, `memory`/1h/256 MiB, `run.go:990-998`: loss at broker restart, recovery is re-publication from the
  source); file stream (semboids `ENTITY`, `file`/24h/2 GiB, `flock.json:17-25`: redelivery, idempotent merge and
  durable guard, with parked input visible); no stream (SemConnect: graph-ingest is driven by the `graph.mutation.>`
  request port, `deploy/semstreams.json:33-45`; the caller holds the outcome or a commit-unknown, D8). No pinned
  framework document ties recovery to the storage class (scope Q6.3); these rows are the first statement.
- **The durable-execution primitive is epic [#24](https://github.com/C360Studio/semengine/issues/24)** (milestone "Slice
  04A: tier-0 graph foundation", sequenced after #9's first green extraction): a journal with terminal ownership, effect
  fences, typed retryable blockers and replay on boot, generalised — not invented — from the two existing
  implementations, agentic-loop's trajectory/terminal-owner/inflight machinery and SemSource's `sourcelifecycle` (scope
  Q2.2). The agentic *domain* stays separated; the mechanics underneath become engine. Qualifying consumers: semsource
  (`sourcelifecycle` migrates onto it) and semteams (`agent-run`, once migrated). The symbol-level read of
  agentic-loop's surface is that epic's precondition, not 03B's.

### D14. Change observation ("live")

None of the four starter consumers observes graph changes through an engine contract today, and graph-ingest
publishes nothing on a subject after apply (scope Q5.1–Q5.2). **Ruled (scope proposal):**

- **Primitive:** applied-state observation is a KV watch on `ENTITY_STATES` and `COMMUNITY_INDEX`. The surfaces exist
  in the port set: `graph.OpenCatalogReader` with `CatalogReader.Watch`/`WatchAll` (`graph/kvcatalog.go:272-280,294`)
  and `component.KVWatchPort` (`component/port_kv.go:5-6`); graph-index already observes `ENTITY_STATES` this way
  (`component.go:154,183,974`). semboids uses raw `kv.WatchAll` on `ENTITY_STATES` twice
  (`internal/boidgraph/probe.go:80-84`, `internal/sim/lifecycle.go:119-124`).
- **View layer:** `pkg/graphview.View[T]` (ADR-081; snapshot plus delta over one `WatchAll`,
  `pkg/graphview/doc.go:1-14`) moves out of D4's dormant group to tier 0. semboids uses it in 5 files; semteams' UI
  reaches it through `processor/agentic-dispatch`'s activity SSE (agentic tier).
- **Transports:** one change-observation primitive (the applied-state KV watch) with two transports. For consumers,
  `output/websocket`, a tier-0 output component over applied state; it is a transport, not a UI. Its only input at the
  pin is a NATS subject (`component.NATSPort`; `output/websocket/doc.go:17,53`), so reading applied state is a port
  change, a `class:port-refactor` (D4a). SemSource's websocket relays ingest *input* (`graph.ingest.>` at `run.go:1067`,
  at-most-once) — the pattern this row replaces. For operators, the service's HTTP SSE KV watch, `GET
  {prefix}kv/{bucket}/watch` (`service/message_logger_http.go:45`; handler `service/message_logger_kv_watch.go:108`), an
  operator and debug surface kept in D15. Whether that SSE route is gated to a dev or test mode at the pin was not
  verified (change review round 1, M2).

### D15. Operator surface

**Ruled (5931143569, adopting 5931117077):** "Operator surface" is a retained-contract-matrix area for the endpoints
the port set already serves at the pin; each row has keep/change/defer, an owner and a proving test. The readiness
rows belong to the durable-execution half (ADR-085/088). Metric names and the flowgraph response are tested contracts:
a dashboard definition checked in against the engine's own metric names, plus one test that fails when a name drifts
(a Q17 DX gate, D16). There is **no `semengine-ui` repository**: UI work lives in `semteams/ui` against documented
endpoints only, and a shared read-only component is extracted only when a second consumer wants the same view.
Documentation leads with the no-UI path: Mermaid from the composition CLI, Swagger at `/docs`, and the status and trace
endpoints.

Reconciliation with D14: one primitive, the applied-state KV watch, has two transports. `output/websocket` is the
consumer transport; the service's HTTP SSE KV watch (`service/message_logger_http.go:45`) is the operator transport.
Neither is a UI, and the operator surface does not widen the port set.

**Ruled ([5932313950](https://github.com/C360Studio/semengine/issues/8#issuecomment-5932313950)): `composition/cli` is
admitted at tier 0** (+1 package, +151 lines; it imports only `component`, `composition` and `config`), because the
no-UI documentation path needs it. The ruling cites `composition/cli/main.go:53-65` for the CLI verb dispatcher. Its
ledger row is task 2.13.

### D16. Exported surface (Q17)

**Ruled (Q17, [5929902986](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929902986): "good heuristic";
released by 5930898291):** SemEngine packages are internal by default and public only where a starter consumer imports
them. The measured import sets are SemSource's 25, SemConnect's 26 at `dff12657` and semboids' 19 at `8c03cc53` (pass3
§2.1). semteams' set is **unmeasured** against the pin: it imports `engine`, `flowstore` and `flowtemplate`, which are
absent there (scope Q3.1). Each ledger row's `destination` fixes the package path under that rule; for a package that
only semteams would import, the destination cannot be decided until semteams' set is measured, and the row says so. The
DX gates that go with it: a compiled example consumer in CI (the documented composition path), package-doc lint on
exported packages, and the metric-name drift test (D15). The 04A change that first exports a package adds the gates.

## Retained-contract matrix skeleton

Columns are fixed so another consumer adds a column, never a restructure: **Area · Behavior · Observed (SemSource 03A @
pin) · Observed (SemConnect @ pin, semconnect#74) · Observed (semboids `8c03cc53`) · Observed (semteams `ce22c961`) ·
Intended SemEngine · Keep/Change/Defer · Owner · Proving test · Qualifying consumer · Half · Tier**. Rows are keyed by
area and behavior. SemSource cells cite an observation name from `pinned-results.json` (A9) or "not observed".
SemConnect cells still marked `pending #74` are filled from semconnect#74's recorded qualification evidence (head
`dff12657`) by task 3.3. semboids and semteams cells cite the scope inventory; "—" means the row was not inventoried for
that consumer. Half is "graph", "durable execution", or "both" for a cross-cutting seam. Tier is the tier at which the
row's behavior is admitted; "all" for a rule that holds at every tier. A row with no qualifying consumer says why:
"framework invariant: none", "none (deferred row)", or "none; removal row".

| Area | Behavior | Observed (SemSource) | Observed (SemConnect) | Observed (semboids) | Observed (semteams) | Intended SemEngine | K/C/D | Owner | Proving test | Qualifying consumer | Half | Tier |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Tiers | 0 foundation = structural ops under `tier0-statistical.json`, embedding still composed | slices [0,1] in bm25 run | pending #74 | — | — | Slice 04A: a true no-embedder composition exists and is admitted | Change | architect | Slice 04A no-embedder boot + structural observations | semsource, semconnect | graph | 0 |
| Tiers | 0 lexical = BM25 under `tier0-statistical.json` (Slice 04B completes tier 0) | bm25 61/63 | n/a | — | — | same, scope-before-limit retained; no external provider | Keep | SemSource owner | `{cold,warm}_*_scope_before_limit`, `*_duplicate_name_anchors` | semsource | graph | 0 |
| Tiers | 1 neural = semembed under `tier1-semantic.json` (Slice 04C); losing the provider degrades to tier 0 with search intact | neural 65/67; provider identity held | n/a | — | — | same; identity pinned per `pins.json` fields | Keep | SemSource owner | `neural_paraphrase`, `provider_metrics_*`, fault supplement 6/6; the tier-1 → tier-0 fallback test | semsource | graph | 1 |
| Tiers | 2 = LLM provider: clustering summarizer, inference review worker, query classifier, agentic domain | not observed | n/a | — | — | slot defined and empty at MVP | Defer | owner | none until a package is admitted at tier 2 | none (deferred row) | both | 2 |
| Tiers | fallback ladder (#4): run at the highest tier whose providers are available; lose one, degrade to tier N−1; tier 0 needs no external provider | not observed | SemConnect has no tiers | — | — | each tier names its guarantees, and its fallback is its own row of this matrix with a proving test | Change | architect | per-tier fallback test, added by the change that admits each tier above 0 | all four | graph | all |
| Crosswalk | SemEngine tier N ↔ SemSource config file | `compatibility.md:21-30` table | SemConnect has no tiers | — | — | table carried verbatim | Keep | technical-writer | the reviewer checks the contract document's table against `compatibility.md:21-30` verbatim (contract-document change) | semsource | graph | all |
| Identity | ADR-102 segment grammar; ADR-104 minted platform authority | `governed_identity_authority`, `*_persisted_authority` | pending #74 (`entityid.Authority` bounds) | — | — | Keep | Keep | architect | same observations on SemEngine | semsource, semconnect | graph | 0 |
| Identity | ADR-102 d5: every graph boundary enforces the deployment's own authority; a foreign `org.platform` enters only on an operator-declared import lane, as a read-only mirror (O-12) (Q8) | not observed | `TestSetup03AFederatedDatastreamCreate` expects a local create under a foreign authority (pre-ADR-102 contract); #74 records HTTP 201 at beta.160, HTTP 400 `authority_foreign` at the pin | — | — | foreign create rejected with the coded error; import-lane mirror accepted; `@id` citation of a foreign identity allowed | Keep | architect | harness: those three cases (04A identity change adds them) | semconnect | graph | 0 |
| Identity | ADR-104: minted `platform.id` is unique (Q8) | not observed | pending #74 | — | — | Keep | Keep | architect | harness test of the minted `platform.id` (04A identity change adds it) | semconnect | graph | 0 |
| Graphable / vocabulary | typed vocabulary (ADR-107), CCO/BFO, export edge | datatype changes attributed | `vocabulary/export` JSON-LD, `csapi` registration | — | — | `vocabulary/export` admitted (D9) | Keep | architect | SemConnect `register_test.go`; export round-trip | semsource, semconnect | graph | 0 |
| Statement metadata | source, time, confidence, correlation preserved through ingest/mutation/storage/query | `*_provenance` | pending #74 | — | — | Keep | Keep | architect | provenance observations; a metadata round-trip property | semsource, semconnect | graph | 0 |
| Content | exact bytes via `StorageReference`; unresolved instance excludes body, never degrades | `*_exact_content`, `*_doc_exact_passage_content` | immutable artifact bytes + `StorageReference` | — | — | Keep | Keep | architect | content observations; SemConnect `schema_artifacts_test.go` | semsource, semconnect | graph | 0 |
| Mutation | typed create/reconcile/delete under revision fence | `edited_*`, `recreated_*` | root-only create/reconcile/delete with exact revisions, absent target | — | — | Keep + D7 | Change | architect | SemConnect `beta160_structural_integration_test.go`; D7 cases | semsource, semconnect | graph | 0 |
| Mutation | conditional reconcile at caller-observed revision (#19) | not available via typed client | raw-wire today | — | — | typed `ExpectedRevision` | Change | owner #19 | D7 | semsource, semconnect | graph | 0 |
| Mutation | commit ambiguity preserved (#20) | not induced | own classifier; `ErrorCodeInternal` → not-committed | — | — | `CommitUnknown` end to end; lookup deferred | Change | owner #20 | D8 | semsource, semconnect | graph | 0 |
| Deletion | watched-file deletion retains stale history; recreation restores identity | `deleted_*`, `recreated_same_identity_not_stale` | n/a | — | — | Keep | Keep | SemSource owner | same | semsource | graph | 0 |
| Deletion | desired-source removal: file-declared component stays out of next boot (#17) | removal supplement 9/10; workaround | n/a | — | — | framework-correct | Change | owner #17 | D11 #17 | semsource | graph | 0 |
| Deletion | retired-source completion signal (#18) | `source_removed` markers absent, pre-existing | n/a | — | — | deferred; consequence recorded | Defer | owner #18 | consumer's `applied_tail_unproven` stays a declared limit | semsource | graph | 0 |
| Deletion | physical purge (semsource#210) | not in workload | n/a | — | — | not admitted | Defer | SemSource owner | — | none (deferred row) | graph | — (not admitted) |
| Query | exact entity, byName, prefix, status | `initial_*`, `cold/warm_exact_absence` | spatial queries (`graph/geo/geojson`) | — | — | Keep | Keep | architect | same; SemConnect spatial case | semsource, semconnect | graph | 0 |
| Query | spatial and temporal indexes | n/a | composed at `dff12657` (`graph-index-spatial`, `graph-index-temporal`) | — | — | admitted at tier 0 (D4, D9) | Keep | architect | SemConnect spatial and temporal cases | semconnect | graph | 0 |
| Query | hierarchy inference (`enable_hierarchy`) | unset | `enable_hierarchy: true` (`deploy/semstreams.json:46`) | — | — | tier 0, provider-free (`graph/inference` hierarchy) | Keep | architect | harness: container entities and edges minted from the ingested ID prefix; D1 refusal text | semconnect | graph | 0 |
| Query | clustering on explicit edges | composed only under `enable_clustering` (`run.go:932`); not observed | n/a | composes `graph-clustering` | — | tier 0: LPA + statistical summarizer; LLM summarizer behind the seam (D4) | Keep | architect | harness: communities written to `COMMUNITY_INDEX` with `EnableLLM` false | semboids | graph | 0 |
| Query | `searchGraph` degraded contract | `graph_search_public_query` | n/a | — | — | keep; LLM-answer path behind the seam (D4, D5) | Change | architect | same observation at tiers 0/1/2 | semsource | graph | 0, 1 (LLM answer at 2) |
| Fusion | `Engine.Fuse` lens path: resolve, expand, hydrate, rank, budget, provenance, partial results | fusion HTTP observations | n/a | — | — | Keep | Keep | architect | 03A fusion observations; #621 reproduce-first row | semsource | graph | 0 (lens path; NL resolve at its provider's tier) |
| Fusion | package-level `fusion.Fuse`/SubQuery | no consumer | no consumer | — | — | not ported | Defer | architect | T-B8 import guard | none (deferred row) | graph | — (not ported) |
| Fusion | impact facet names (SS#603) | not observed | n/a | — | — | reproduce first | Defer | architect | — | none (deferred row) | graph | — (deferred) |
| Readiness | honest readiness; miss is absence only when ready (ADR-066/084) | `ingestion_ready`; fault supplement "error is not absence" | `index-readiness` reader | — | — | Keep | Keep | architect | same | semsource, semconnect | graph | 0 |
| Durability | recovery after stream recreation (#15) | **fails** ×2 at both pins: `broker_restart_reingested_exact_relationship`, `broker_restart_reingested_exact_content` (A6) | pending | — | — | repaired | Change | owner #15 | D11 #15 | semsource | graph | 0 |
| Durability | capacity: OSH 32,720 under 256 MiB; historical 77,802 open | guarded pass | n/a | — | — | tier-gated workload evidence, not transport success | Defer | SemSource owner | capacity probe at the admitted tier | semsource | graph | the admitted tier |
| Recovery | memory stream: a broker restart loses accepted-but-unapplied messages; recovery is re-publication from a source that still exists (replaces I7) | `memory_transport_not_durable`, `accepted_unindexed_before_broker_restart`, `authority_survives_broker_restart_before_reingest`, `broker_restart_reingested` (`GRAPH` is `memory`, `run.go:990-998`) | n/a | n/a | — | recorded as the contract for a memory stream: acknowledged = accepted by transport; durable = KV authority + content; recovery = re-publication | Keep (as stated) | owner | same observations | semsource | graph | 0 |
| Recovery | file stream: unacked messages are redelivered after a process or broker restart; recovery is redelivery + idempotent merge + durable guard, with parked input visible (replaces I7) | n/a (memory stream) | n/a | `ENTITY` is `file`/24h/2 GiB (`flock.json:17-25`); no process-kill test anywhere (scope Q4.4) | — | stated and proven for a file stream | Change | owner (Q18) | the settlement process-kill test (D11) plus the parked-input test; needs the SETUP 02 harness extension (task 7.1) | semboids | graph | 0 |
| Recovery | no stream: graph-ingest driven by request/reply; the caller holds the outcome or a commit-unknown (replaces I7) | n/a | `graph.mutation.>` request port, no ingest stream (`deploy/semstreams.json:33-45`) | n/a | — | stated as the contract; D8 classification | Keep + D8 | owner #20 | D8 injected KV `Update` timeout | semconnect | graph | 0 |
| Settlement | durable-consumer settlement order and heartbeat (Q18) | apply → durable guard → ack (`keyed_ingest.go:144-229`) on a memory stream | n/a (no stream) | file stream at about 200 entity msgs/s; settlement symbols 0 | settlement symbols 0 | order kept; graph-ingest calls `natsclient` settlement (#759); `InProgress` heartbeat for long applies | Change | owner (Q18) | harness: process kill mid-apply on a file stream (D11); needs the SETUP 02 harness extension (task 7.1) | semboids, semsource | durable execution | 0 |
| Parked input | message parked after `MaxDeliver` (default 3) is visible | `MAX_DELIVERY_EVENTS` provisioned by `EnsureStreams` (`run.go:256`); no observer | provisioned (`cs-graph-backend/main.go:224`); no observer | provisioned (`main.go:138`); no observer | provisioned (`main.go:705`); no observer | `internal/maxdelivery` observer ported as a tier-0 component reading `MAX_DELIVERY_EVENTS` | Change | owner (Q18) | harness: a message that exhausts `MaxDeliver` produces a visible parked occurrence (record and metric) | all four | durable execution | 0 |
| Rules | action dispatch is open to registered families; core families enumerated (E1) | no rule processor composed | no rule processor composed | 7 actions, all core (`publish` ×6, `lifecycle_transition` ×1) | 119 actions; 70 core | closed switch (`actions.go:916-945`) gains a family registry | Change | architect | a fake family registered on a test executor is dispatched and validated; a core-only pack loads and fires with no family registered (semboids corpus) | semboids | durable execution | 0 |
| Rules | an action type that is neither core nor registered (E1) | n/a | n/a | none | 40 `publish_agent` without the family; 9 `replace_owned` (not a type at the pin) | refused at load or fails at fire time — owner ruling pending (D12) | pending | owner (#8) | named by the ruling: a definition with an unregistered type is rejected at load with the type named, or fails at fire time as at the pin | semboids, semteams | durable execution | 0 |
| Rules | rule JSON decodes unchanged for every consumer corpus (E2) | n/a | n/a | 7/7 | 119/119 (`tool_choice` in 31 files; `response_format` in 0) | `response_format`/`tool_choice` held as raw JSON in the core; the family decodes them | Change | architect | golden round-trip of the semteams (119) and semboids (7) corpora through the core decoder; property: core decode then family decode of `tool_choice` equals direct `agentic.ToolChoice` decode | semboids, semteams | durable execution | 0 |
| Rules | the core compiles with no agentic import; `publish_agent` is a registered family (E3) | n/a | n/a | 0 `publish_agent` | 40 `publish_agent` | `processor/rule` imports no `agentic*`, `vocabulary/agentic` or `governance` | Change | architect | import guard: `go list -deps ./processor/rule` ∩ {`agentic*`, `vocabulary/agentic`, `governance`} = ∅; semteams 40/40 dispatched with the family registered | semboids | durable execution | 0 |
| Rules | `deny` is terminal: a `*DenyVerdict` stops the chain and is never retried (E4) | n/a | n/a | 0 `deny`/`approve` | 0 `deny`/`approve` | `deny.go` stays in the core; `deny`/`approve` executors and `VerdictAuditor` move to the governance tier | Change | architect | `deny_integration_test.go` and the deny arm of `stateful_evaluator_test.go` move with the family; a core test of the short-circuit on a `*DenyVerdict` (the 04A rule-core change adds it) | none; removal row | durable execution | 0 |
| Rules | rule-pack IDs validated by one contract package; projection bindings bound at boot (E5) | n/a | n/a | — | `pkg/rulepack` 0 imports | `pkg/rulepack` and `service/rule_pack_bind.go` carried | Keep | architect | pack-ID validation tests; a rule pack's projection contracts are bound before Start (the 04A rule-core change adds it) | semboids | durable execution | 0 |
| Rules | retired action type `replace_owned` | n/a | n/a | none | 9 actions in 5 files fail at fire time on the pin | consumer migrates to `reconcile_predicates` | Defer (consumer) | semteams owner | semteams corpus with no `replace_owned` | semteams | durable execution | 0 |
| Entity workflows | named-instance phases on graph entities, rule-driven transitions, restart resume through the entity (`pkg/lifecycle`, Q16) | not used | not used | `flock.boid` workflow; `predator-cull` → `lifecycle_transition` → cull watcher | framework `agent-run` workflow (8 `lifecycle_transition` actions) | kept at tier 0; `Dependencies.LifecycleManager` kept | Keep | architect | harness: semboids' cull loop — a `lifecycle_transition` sets the phase and a watcher observes it; state survives a restart through `ENTITY_STATES`; needs the SETUP 02 harness extension (task 7.1) | semboids | durable execution | 0 |
| Change observation | applied-state observation: KV watch on `ENTITY_STATES`/`COMMUNITY_INDEX` as the primitive | no graph watch (relays ingest input) | no graph watch | raw `kv.WatchAll` ×2 | through `agentic-dispatch` SSE | `CatalogReader.Watch`/`WatchAll` and `component.KVWatchPort` as the documented primitive | Keep | architect | harness: an applied mutation is observed by a watch on `ENTITY_STATES`; a watcher started after the write receives the current value | semboids | graph | 0 |
| Change observation | view layer: `pkg/graphview` snapshot + delta | n/a | n/a | 5 files (`internal/api/graphstream.go`, `graphstream_views.go`, `service.go`, …) | through `agentic-dispatch` | tier 0 (out of D4's dormant group) | Keep | architect | snapshot-then-delta consistency over `ENTITY_STATES` | semboids | graph | 0 |
| Change observation | `output/websocket` as a transport over applied state, not a UI and not a relay of ingest input | composed over `graph.ingest.>` (`run.go:1040-1091`) | not composed | composed (`flock.json:207-239`) | not composed | tier-0 output over applied state (D14); port change from a NATS-subject input (`output/websocket/doc.go:17,53`), a `class:port-refactor` | Change | architect | harness: a websocket client receives applied entity changes (test to be written by the developer in the 04A change that ports `output/websocket`) | semboids | graph | 0 |
| Operator surface | health, liveness, readiness (`/health`, `/healthz`, `/readyz`; `service/service_manager.go:1707-1709`) | not established | not established | — | — | served as at the pin; readiness ties to ADR-085/088 | Keep | architect | harness: `/readyz` reports not-ready before the composed components are ready and ready after; `/healthz` live | framework invariant: none (which endpoints each consumer exposes was not checked, 5931117077) | durable execution | 0 |
| Operator surface | service list and health (`/services`, `/services/health`; `:1712-1713`) | not established | not established | — | — | served as at the pin | Keep | architect | harness: the composed services are listed with their health | framework invariant: none (which endpoints each consumer exposes was not checked, 5931117077) | both | 0 |
| Operator surface | OpenAPI document and Swagger UI (`/openapi.json`, `/docs`; `:1568-1572`) | not established | not established | — | — | served as at the pin; the docs' no-UI path links `/docs` | Keep | architect | `/openapi.json` parses and lists the mounted routes | framework invariant: none (which endpoints each consumer exposes was not checked, 5931117077) | both | 0 |
| Operator surface | component health, list, types, status, config (`service/component_manager_http.go:68-73`) | not established | not established | — | — | served as at the pin | Keep | architect | harness: status of a composed `graph-ingest` | framework invariant: none (which endpoints each consumer exposes was not checked, 5931117077) | both | 0 |
| Operator surface | flowgraph projection: `flowgraph`, `validate`, `paths` (`component_manager_http.go:76-78`) | not established | not established | — | — | a tested contract (D15) | Keep | architect | golden test of the flowgraph response for a fixture composition | framework invariant: none (which endpoints each consumer exposes was not checked, 5931117077) | both | 0 |
| Operator surface | message trace by ID and KV query/watch (`service/message_logger_http.go:34-45`) | `service.RegisterAll` registers the message logger (A2.2) | not established | — | — | served as at the pin; the SSE KV watch (`message_logger_http.go:45`) is the operator transport of the change-observation primitive (D14) | Keep | architect | harness: trace by ID returns the logged entries | framework invariant: none (which endpoints each consumer exposes was not checked, 5931117077) | both | 0 |
| Operator surface | storage observability report (`service/storage_observability_http.go:133`) | not established | not established | — | — | served as at the pin | Keep | architect | harness: the report covers the composed stores | framework invariant: none (which endpoints each consumer exposes was not checked, 5931117077) | both | 0 |
| Operator surface | Prometheus handler and the metric names it exports (`metric/handler.go:124`) | not established | not established | `cmd/sweep` scrapes `:9090` for ingest lag, latency and write amplification | — | metric names are a tested contract; a dashboard definition is checked in against them (D15, D16) | Change | architect | the dashboard definition's metric names are all exported; a drift test fails when one is not | semboids | both | 0 |
| Operator surface | composition rendered as Mermaid; CLI verb dispatcher (`composition/mermaid.go:12`, `composition/cli/main.go:53-65`) | not established | not established | — | — | the documented no-UI view; `composition/cli` admitted at tier 0 (D15) | Keep | technical-writer | golden Mermaid for a fixture composition (developer, in the 04A change that ports `composition`); the technical-writer runs the documented no-UI commands in the contract-document change and records their output | framework invariant: none (which endpoints each consumer exposes was not checked, 5931117077) | both | 0 |
| Transport | reserved RPC subjects vs stream filters (#16) | `rpc_wildcard_collision_reproduced` | pending | — | — | refusal at boot | Change | owner #16 | D11 #16 | semsource | graph | 0 |
| Component/service seams | declared ports, typed ops, one-shot lifecycle, failed-Start cleanup (ADR-094/095/096/100) | application restart observations; join tests | pending | — | — | Keep; `Dependencies` narrowed by the E3 consequence (D4, D12); `LifecycleManager` kept | Change | architect | lifecycle-suite per component | all four | both | 0 |
| Component/service seams | `config.Manager.Stop(timeout)` (SS#1415) | n/a | n/a | — | — | `Stop(ctx)` | Change | owner | D11 | framework invariant: none | both | 0 |
| Component/service seams | two `ErrAlreadyStopped` (SS#1218) | n/a | n/a | — | — | one | Change | owner | D11 | framework invariant: none | both | 0 |
| Registration | aggregator imports | both imported | `payloadbuiltins` and `vocabulary/builtins` imported in production at `dff12657` (`cmd/cs-graph-backend/main.go:25,36,110,123,228`) | `payloadbuiltins.Register` (`cmd/semboids/main.go:163`) | `componentregistry.Register` (`cmd/semteams/main.go:777`) | explicit per-package (D1); the D1 adopter-path text names per-package calls | Change | owner #3 | T-B8; consumer boots; the graph-ingest refusal-text assertion (D1) | all four | both | 0 |
| Provider/model identity | image, build, model, artifact, ONNX, dims, prefixes held constant | `pins.json` | n/a | — | — | same fields required per tier-1 run | Keep | SemSource owner | pins check in the qualification runner | semsource | graph | 1 |
| Context/stop/join | contexts enter as arguments; no retained context; bounded terminal cleanup | 0 retained fields; 34 roots (A8) | — | — | — | each root triaged in the ledger | Keep/Change per root | developer | `TestNoRetainedContext`, cleanup-roots guard | framework invariant: none | both | 0 |
| Level name | "tier" vs "profile" | 03A uses `profile` for runs (a configuration label) | — | — | — | "tier" (ruled on #4, D3); `profile` stays only as the 03A run label | Keep | owner #4 | the reviewer searches the contract document for `profile` outside the quoted run label (contract-document change) | framework invariant: none | both | all |
| Boundary | no SemEngine file imports `github.com/c360studio/semstreams` (Q2) | n/a | n/a | — | — | fork at the pin; no Tier-1 compatibility promise | Keep | architect | SemEngine boundary test (I8) | framework invariant: none | both | all |

## Premises (each with its measurement)

1. The first-pass port set at the pin is 65 / 126,926 — A1, `go list` + line count, identical to Codex; reproduced
   as pass3 set A.
2. `engine` and `flowstore` do not exist at the pin — A1.1, `ls` and `git log --diff-filter=D`.
3. `agentic` is reached in the tier-0 set only through `component/dependencies.go:7` and `processor/rule`
   (`actions.go:15`, `config_validation.go:9`); `agentrun` through `service/milestone_service.go:10` and
   `processor/rule/actions.go:16` — pass3 §2.4.
4. SemSource's retained workload does not exercise graph-gateway — A3.3, `grep gateway test/setup03a` → one config line.
5. The two Fuse entry points are distinct and only the lens path has a consumer — A3.1.
6. SemConnect already sends `ExpectedRevision` by raw wire — A6 #19, `graph_mutations.go:214-215`.
7. `CommitUnknown` exists at the pin and is defeated by the server-side conversion — A6 #20.
8. Tier 0 is 65 / 140,842 under the ruled scope — pass3 §2.2 set H (reviewed: "all deltas recompute") less
   `graph/embedding/http_embedder.go` (220) plus `composition/cli` (151), measured by the writer (D4).
9. Of the six first-pass "higher-tier" libraries, four are provider-free at runtime and two are provider clients;
   in `graph/embedding` one file, `http_embedder.go`, imports `go-openai` — pass3 §1, D4.
10. Two D4 edits break `processor/rule` at compile time (`factory.go:148`, `:160-161`) and the `rule_pack_bind.go`
    non-port strands `ProjectionBindings()` — scope Q1.5.
11. graph-ingest acknowledges only after the durable write on every path that intends one — scope Q4.1.
12. `MAX_DELIVERY_EVENTS` is provisioned by `config.EnsureStreams` in all four consumers; the observer is not composed
    by any — scope Q4.3 (corrected after review).
13. No coverage baseline exists at the pin — A13.

## Invariants and their spec homes (new spec deltas this change must add)

- **I1 (registration):** no SemEngine production package imports more than one component family's factories; the
  consumer's composition root is the only aggregator. Home: `harness-boundaries` spec, new requirement; guard T-B8.
- **I2 (reserved subjects):** for every stream config admitted at boot, no subject filter covers a reserved
  request subject; violation is a boot refusal naming stream, filter, subject. Home: new capability spec
  `graph-transport-boundary` (#16).
- **I3 (generation-aware replay guard):** within one stream generation, a redelivered sequence ≤ last applied is
  stale; across generations, the comparison never suppresses current source state. Home: new capability spec
  `graph-ingest-recovery` (#15).
- **I4 (deletion durability):** a component deleted from desired state is absent from the next boot's concrete set
  regardless of its declaring source. Home: new capability spec `config-desired-state` (#17).
- **I5 (commit classification):** `CommitNotCommitted` is returned only for rejections proven before any storage
  effect; every other failure is `CommitUnknown`. Home: new capability spec `projection-mutation` (#20).
- **I6 (conditional reconcile):** a reconcile carrying revision R applies only if R is current; otherwise it returns
  `revision-conflict` with both revisions and makes no change. Home: `projection-mutation` (#19).
- **I7 (recovery per storage class; replaces the single acknowledged-write contract):** accepted ≠ durable;
  durable = KV authority + content. On a memory stream, recovery after broker loss is re-publication from the source;
  on a file stream, recovery is redelivery, idempotent merge and the durable guard, with exhausted input parked
  visibly; with no stream, the caller holds the outcome or a commit-unknown. Home: `graph-ingest-recovery`.
- **I8 (fork boundary, Q2):** no SemEngine file imports `github.com/c360studio/semstreams` or any path under it; this
  is ADR-106's sister-import freeze enforced from SemEngine's side. Home: `harness-boundaries` spec, the same
  modified requirement as I1; guard: a SemEngine boundary test that lands with the first 04A change carrying the
  `harness-boundaries` delta, so the spec never lags the test (task 2.6).
- **I9 (settlement, Q18):** graph-ingest acknowledges an input only after its effect and its durable guard stamp are
  committed; a process replaced between apply and acknowledgement leaves the input redelivered, never lost, and the
  entity state equal to the state of one application. Home: `graph-ingest-recovery`.
- **I10 (rule core, Q15):** the rule core imports no agentic or governance package; an action family outside the core
  enters only by registration. Home: drafted by the 04A change that ports the rule core.

## Adopter seam findings (the gaps are the design work)

From A12: seams 3 (stream subjects), 4 (projection client), and 5 (desired-config removal) are at "found out
nowhere" or "next boot" today; D11 #16, D7/D8, and D11 #17 are their fixes, each converting a predicted value into an
observed one (boot refusal; owner-side fence and classification; owner-side tombstone). Seam 1 (registration) moves
from compile-time-silent to compile-time-explicit under D1, and the D1 adopter-path items stop the one boot error that
would name a missing symbol. Seam 2 (Lens SPI) and seam 6 (durability) stay as documented contracts with their
invariants in spec (I7). Two new seams enter with the scope ruling: the rule core's action-type refusal (D12; held) and
the operator surface's metric names (D15; a drift test).

## Design options (not holds)

The architect's forks that no ruling decides. Each is settled by the 04A change that performs the edit, with its
reviewer; none blocks a task here.

- **E1 registration shape** (pass3 §3.2): (a) a package-level `rule.RegisterActionFamily(name, factory)` called from
  the consumer's composition root — the D1 shape, at the cost of process-global state in a library, a pattern ADR-103
  moved away from for payload types; (c) the agentic tier wraps the rule factory and registers its family on the
  processor instance with its dependencies captured in the closure — no global state and no new field on
  `component.Dependencies`, at the cost of one more exported method on `Processor`. Option (b), an `ActionFamilies`
  field on `component.Dependencies`, re-creates the shape D4 removes and is listed only to be visible. E3's
  compile-safety of the `component` adapt holds under (c) directly and under (a) only if registration carries the tool
  registry.
- **E2 field shape** (pass3 §3.2): (a) `json.RawMessage` per field — minimal; the core still names two agentic keys;
  (b) a core `Extensions map[string]json.RawMessage` collecting every undeclared key — family-agnostic, but needs a
  custom `Action.UnmarshalJSON` and changes today's silent drop of unknown keys into retention.
- **`graph/llm` split shape** (pass3 §1.2): (a) the whole package behind the seam, so `graph/clustering` and
  `graph/inference` lose their `LLMSummarizer`/`EnhancementWorker` and review-worker LLM field to the provider tier (two
  files out of each of two tier-0 libraries); (b) split `openai_client.go` (365 lines) into the provider tier and admit
  the 477-line provider-free contract at tier 0, so the three importers compile unchanged and tier 0 holds an interface
  whose present consumers are nil-guarded optional fields (+477, −365 lines in the closure; tier 0 would be 66 /
  141,319).

## Open questions

One question is open, and it holds one task. Owner, on #8.

- **Unknown action types** (D12): refuse an action whose type is neither core nor registered at rule-pack load, or
  keep the pin's fire-time failure. Holds task 4.9.

Ruled since the step-back review and released (each task's first line names its ruling): Q14 (admission by owner
mandate, yes), Q15 (rule core at tier 0, D12), Q16 (`pkg/lifecycle` kept, D13), Q17 (exported surface, D16), Q18
(settlement, parked input and recovery rows, D11 and D13), and the Q8 re-measure (D4 and D9). Ruled by 5932313950 and
released: BM25 at tier 0 (tasks 1.7, 2.3, 2.11), `composition/cli` admitted (task 2.13), the tier model and slices (D3),
and the extended critical list (D10).

## Declared costs

- The capability seam (D4) is still the single largest adapt, now spread over five packages (`processor/graph-query`,
  `graph/query`, `graph/clustering`, `graph/inference`, `processor/graph-clustering`) plus the `http_embedder.go` split.
  Q4 orders it last in 04A, so the first green tier-0 extraction carries the behind-the-seam packages dormant through
  lint, vuln and coverage gates until the exit condition holds.
- Port refactors (D4a) are the owner's named largest risk class. Their tracking rests on existing ledger fields and
  an issue label, not on a machine-checked ledger field; a reviewer, not T-B7, catches a missing matrix citation. The
  rule core adds four of them (E1–E4).
- Tier 0 grows by 13,916 lines over the first-pass 65 and counts the rule core at its full 17,243 lines, although E3
  and E4 move part of it out (pass3 §3.1 sizes the parts). The 65 / 140,842 figure is a reachability cut plus a
  file-level subtraction by the writer, not a `go list -deps` on a seamed tree; the architect confirms it at Slice 04A's
  first `go list`.
- The Q12 gate figures are recomputed from A5 (69 / 6); the ten entering packages are not measured.
- The critical list grows by eleven packages (ten named in the ruling plus `processor/rule/expression` with the rule
  core; `graph/embedding` was already on it) with no coverage baseline at the pin.
- The process-kill, broker-restart and failpoint proving tests need harness capability SETUP 02 does not have
  (`natsfixture` has `Stop` only); the extension is the first task of the first 04A change (task 7.1).
- SemConnect's second matrix column is partly filled; the cells marked `pending #74` wait on task 3.3, which reads
  semconnect#74's recorded qualification evidence. That PR is a draft whose qualification is itself open on the
  federated-create assertion, so its evidence can still change.
- Operator-surface rows record "not established" for which endpoints each consumer exposes; the review that the
  ruling adopts did not check it.
- `output/websocket` needs a port change to read applied state (D14). How a consumer composes the
  `internal/maxdelivery` observer is not established (D13).
- Coverage baseline is unknown; the 80% gate may fail on first port of a critical package and that is a finding, not
  a reason to drop the package from the list.
- SemStreams PR #1437 (open, after the pin) changes two port-set packages, one of them `processor/graph-ingest`,
  which now carries four repair rows; under ruling 3 it enters only as its own ledger row with source commit and
  proving test. Q13 ruled it a candidate for the graph-ingest half only (D11).

## Spec deltas drafted for the SETUP 04A changes

This change sets `skip_specs: true`: `openspec/specs/` is current truth verified against code, and no code lands here.
The 04A extraction change for each package carries the delta below as its `specs/<capability>/spec.md`.

### graph-transport-boundary (new; #16)

```markdown
## ADDED Requirements

### Requirement: Reserved request subjects have one declaration
The graph request/reply subject families served by graph-ingest, graph-index, graph-embedding, and graph-query
SHALL be declared once, in the `graph` package, and every server and client SHALL reference that declaration.

#### Scenario: A literal spelling is added
- **WHEN** a non-test file outside the declaration spells a reserved subject as a string literal
- **THEN** the contract test fails naming the file

### Requirement: Stream filters never cover a reserved subject
Composition analysis SHALL refuse to start when any configured stream's subject filter covers a reserved request
subject, with an error naming the stream, the filter, and the subject.

#### Scenario: Wildcard ingest stream
- **WHEN** a stream declares `graph.ingest.>` and graph-ingest serves `graph.ingest.query.entity`
- **THEN** startup fails before readiness with the stream, filter, and subject in the error

#### Scenario: Explicit subjects
- **WHEN** a stream declares exactly `graph.ingest.entity`, `graph.ingest.batch`, `graph.ingest.manifest`,
  `graph.ingest.status`, `graph.ingest.predicates`
- **THEN** startup succeeds and entity, batch, prefix, and suffix queries return correct answers
```

### graph-ingest-recovery (new; #15, settlement, and recovery per storage class)

```markdown
## ADDED Requirements

### Requirement: Acknowledged is not durable
A transport acknowledgement SHALL mean accepted by the transport; durability SHALL mean the entity authority and
content are committed to their stores.

#### Scenario: Memory transport lost at broker restart
- **WHEN** an accepted but unindexed message is lost with a memory stream at broker restart
- **THEN** the authority and content committed before the restart survive, the lost message is not reported as
  applied, and re-publication from the source applies it

### Requirement: Recovery on a file stream is redelivery
On a file-backed input stream, an input that was not acknowledged SHALL be redelivered after a process or broker
restart and applied at most once by the idempotent merge and the durable guard; an input that exhausts its delivery
limit SHALL be recorded as parked and visible.

#### Scenario: Process killed between apply and acknowledgement
- **WHEN** graph-ingest is killed after applying an input and before acknowledging it, and is restarted
- **THEN** the input is redelivered, the entity state equals the state of one application, and no input is lost

#### Scenario: Delivery limit exhausted
- **WHEN** an input fails on every delivery up to the consumer's delivery limit
- **THEN** a parked occurrence naming the stream and consumer is visible to the operator

### Requirement: Settlement order
graph-ingest SHALL acknowledge an input only after its effect and its durable guard stamp are committed, and SHALL
signal progress on an input whose application outlasts the acknowledgement wait.

#### Scenario: Long apply
- **WHEN** applying one input takes longer than the consumer's acknowledgement wait
- **THEN** the input is not redelivered while the apply is still progressing

### Requirement: Replay protection is generation-aware
The applied-sequence guard SHALL key on the stream generation; within one generation a sequence not newer than the
last applied SHALL be stale; across generations the guard SHALL never suppress current source state.

#### Scenario: Stream recreated with lower sequences
- **WHEN** the stream is recreated and re-ingestion publishes the current source at a sequence lower than the
  retained applied sequence
- **THEN** the current relationship and content are applied and queries return them

#### Scenario: Redelivery within one generation
- **WHEN** a message already applied in the current generation is redelivered
- **THEN** it is acknowledged without changing current state
```

### config-desired-state (new; #17)

```markdown
## ADDED Requirements

### Requirement: Deleted components stay deleted across boot
A component removed from desired state SHALL be absent from the next boot's concrete component set regardless of
whether the file configuration declared it.

#### Scenario: File-declared component deleted then restart
- **WHEN** a file-declared component is deleted through the desired-state API and the process restarts with the same
  file and retained bucket
- **THEN** the component is not created and not started

#### Scenario: Re-add after deletion
- **WHEN** the same component name is written to desired state again and the process restarts
- **THEN** the component is created and started
```

### projection-mutation (new; #19, #20)

```markdown
## ADDED Requirements

### Requirement: Conditional reconcile at a caller-observed revision
`ReconcileMutation` SHALL accept an expected revision; when set, the owner SHALL apply the mutation only if that
revision is current and otherwise return a revision-conflict error naming the expected and current revisions and
making no change.

#### Scenario: Concurrent update between read and reconcile
- **WHEN** the caller read revision R, another writer committed R+1, and the caller reconciles at R
- **THEN** the result is revision-conflict and the entity is unchanged at R+1

#### Scenario: Unchanged revision
- **WHEN** the caller reconciles at the current revision R
- **THEN** the mutation is applied and the receipt reports verified with the new revision

### Requirement: Commit ambiguity is preserved
The owner SHALL classify a failure as not-committed only when it is proven to precede any storage effect (invalid
request, revision conflict, entity not found); every other failure SHALL be reported as commit-unknown through the
canonical server and the public client.

#### Scenario: Backend write timeout
- **WHEN** the authority store update times out or its acknowledgement is lost
- **THEN** the receipt reports commit-unknown, never not-committed

#### Scenario: Revision conflict
- **WHEN** the authority store rejects the update for a revision mismatch
- **THEN** the receipt reports not-committed with the conflict classification
```

### harness-boundaries (modified; T-B8)

```markdown
## MODIFIED Requirements

### Requirement: Import graph
No non-test Go file outside `internal/harness/` SHALL import `internal/harness/...`, `testcontainers-go`, `testing`,
or `gopkg.in/yaml.v3`. No production package SHALL import the component factories of more than one component family;
the consumer's composition root is the only place factories and payload registrations are aggregated. No Go file in
the module SHALL import `github.com/c360studio/semstreams` or any path under it.

#### Scenario: Production package imports the fixture
- **WHEN** a non-test file outside internal/harness imports internal/harness/natsfixture
- **THEN** the contract test fails naming the file

#### Scenario: An aggregator package appears
- **WHEN** a production package imports `Register` from two or more component packages
- **THEN** the contract test fails naming the package

#### Scenario: A file imports SemStreams
- **WHEN** any Go file in the module, test or non-test, imports `github.com/c360studio/semstreams/...`
- **THEN** the contract test fails naming the file and the import
```
