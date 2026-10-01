# setup-03b-contract-boundary

## Why

SemEngine has a harness and no ported package (`docs/repository-map.md`; `openspec/specs/` holds four harness
capabilities and no graph capability). SETUP 03B (epic #8) must approve the retained contract, the boundary, and the
port set before SETUP 04A extracts anything. SemEngine is a live semantic knowledge graph with durable execution, two
halves admitted by owner mandate at a named tier with a named qualifying workload (`AGENTS.md`; #8 Q14). The first
inventory pass measured 65 packages / 126,926 non-test lines at the frozen pin `8b99efe9` (`inventory.md` A1). Under the
ruled tier-0 scope for both halves, with four starter consumers (semsource, semconnect, semboids, semteams), tier 0 is
62 packages / 134,356 lines if BM25 is tier 1, or about 64 / 140,911 less one file if BM25 is tier 0
(`inventory-3-pass3.md` §2.2); the owner decides which (H1). Three reproduced substrate defects (#15, #16, #17), three
contract requests (#18, #19, #20), graph-ingest's settlement under process replacement, and the operator surface had no
SemEngine contract. The owner ruled the questions this change raised on 2026-10-01 except three, which hold tasks; this
change records the rulings and turns them into rows and tasks.

## What Changes

- `design.md` opens with the owner's statement of purpose and the two-halves admission rule; every row of the
  retained-contract matrix names its half (graph or durable execution) and its qualifying consumer.
- The retained-contract matrix, complete against SemSource's 03A baseline at the pin, with observed columns for
  SemConnect (PR semconnect#74), semboids and semteams, so another consumer adds a column, not a restructure.
- The registration cut: explicit per-package registration in each consumer's composition root; no SemEngine
  aggregator; the pin's error text and doc comments that name `payloadbuiltins.Register` become D1 ledger items (D1).
- The port set, re-partitioned by "needs an external provider" (D4): the agentic domain and the gateways separated;
  `graph/clustering`, `processor/graph-clustering`, `graph/inference` (hierarchy), `graph/structural` and
  `pkg/graphview` at tier 0; `graph/llm` and `model/wire` behind the capability seam, which is the last task of 04A.
  The embedding family (BM25 and an OpenAI client) is tier 0 or tier 1 by the owner's decision (H1); the ceiling
  replacing 67 / 129,063 follows it.
- A rule core at tier 0 (D12): `processor/rule` minus the `publish_agent` and `deny`/`approve` families, by four
  `class:port-refactor` edits (E1–E4); `pkg/rulepack` and `service/rule_pack_bind.go` carried.
- The durable-execution floor at tier 0 (D13): entity workflows (`pkg/lifecycle` kept), settlement (graph-ingest's
  order kept, the first non-agentic `natsclient` settlement caller, an `InProgress` heartbeat), the parked-input
  observer, and recovery stated per storage class (memory stream, file stream, no stream) in place of I7. The
  primitive itself is epic #24.
- Change observation (D14): the applied-state KV watch as the primitive, `pkg/graphview` as the view layer, and
  `output/websocket` as a tier-0 consumer transport over applied state (a port change).
- The operator surface (D15): health, service, OpenAPI, component, flowgraph, message-trace, storage, metrics and
  composition-Mermaid rows; the service's SSE KV watch as the operator transport of change observation; metric names and
  the flowgraph response as tested contracts; no `semengine-ui`.
- The exported surface (D16): internal by default, public only where a starter consumer imports, with three DX gates.
- Port refactors (seams, file-level adapts, dropped fields) are tracked as the risk class `class:port-refactor`:
  each is an `adapt` ledger row citing its matrix row and proving test (D4a).
- The capability-level word is "tier", with the fallback ladder as a contract element (D3, #4).
- A SemEngine boundary test that no file imports `github.com/c360studio/semstreams` (D2, I8), placed in the first
  04A change that carries the `harness-boundaries` delta.
- The fusion and graph-tool boundary: four layers mapped to code; `Engine.Fuse` lens path kept, package-level
  `fusion.Fuse` deferred, `searchGraph` kept under its degraded contract with the LLM-answer path behind the seam;
  find/anchor/ask recorded as cases (D5).
- Repair-before-port rows for #15, #16, #17, #19, #20, settlement, and the ruling-4 lifecycle debt (SS#1411, #1415,
  #1218, #1220, #1417, #1145/#1147), each with its tier-0 (#9) gate evidence (D11).
- The critical package list for the 80% coverage gate (D10).
- `docs/admission-ledger.yaml` seeded with one row per package in the tier-0 set and one per separated package,
  dispositions per D4.
- Spec deltas for the new contract invariants are **drafted in `design.md`** and carried by the SETUP 04A extraction
  changes, because `openspec/specs/` is current truth verified against code; this change sets `skip_specs: true`.

## Owner rulings

All on 2026-10-01:

- Q1–Q13: [#8 comment 5929656835](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929656835)
  (questions in [comment 5929442167](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929442167)),
  copied to issues #2, #3, #18, #19, and #20. The word "tier" was ruled on
  [#4](https://github.com/C360Studio/semengine/issues/4#issuecomment-5929716018).
- Q14–Q18: questions in [comment 5929815222](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929815222),
  ruled with the owner's words in [comment
  5929902986](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929902986).
- The Q14–Q18 releases and the Q11 re-ruling: the tier-0 scope proposal for both halves, [comment
  5930855353](https://github.com/C360Studio/semengine/issues/8#issuecomment-5930855353), approved as written including
  sequencing by [comment 5930898291](https://github.com/C360Studio/semengine/issues/8#issuecomment-5930898291).
- The operator surface: [comment 5931143569](https://github.com/C360Studio/semengine/issues/8#issuecomment-5931143569),
  adopting the review in [comment 5931117077](https://github.com/C360Studio/semengine/issues/8#issuecomment-5931117077).

Each decision in `design.md` states its ruling. Three questions are open, each holding tasks: BM25 at tier 0 or tier 1
(H1, D4; tasks 1.7, 2.3, 2.11); admitting `composition/cli` at tier 0 or deferring the CLI row (M1, D15; task 2.13); and
whether the rule core refuses an unknown action type at load or fails at fire time (D12; task 4.9). The architect's
unruled forks (E1 registration shape, E2 field shape, `graph/llm` split shape) are design options for the 04A changes,
not holds.

## Non-goals

Extraction of any package; SemStreams patches (frozen pin, ruling 3); the applied-input barrier primitive (#18,
deferred with consequence recorded); a terminal operation-status lookup (#20, deferred); the durable-execution
primitive (epic #24, after #9's first green extraction); the agentic domain (`agentic`, `agentic/agentrun`,
`processor/agentic-*`, `governance`, `vocabulary/agentic`); a `semengine-ui` repository; SemSource mainline cutover;
SemConnect cutover; physical purge (semsource#210); generation enrichment admission.

## Capabilities

### New Capabilities

None in this change (`skip_specs: true`). Drafted for SETUP 04A: `graph-transport-boundary` (#16),
`graph-ingest-recovery` (#15, settlement, recovery per storage class), `config-desired-state` (#17),
`projection-mutation` (#19, #20).

### Modified Capabilities

None in this change. Drafted for SETUP 04A: `harness-boundaries` gains the aggregator-free import requirement (T-B8)
and the no-SemStreams-import requirement (I8).

## Impact

`openspec/changes/setup-03b-contract-boundary/{proposal,design,tasks}.md` and its three inventory passes
(`inventory.md`, `inventory-2-scope.md`, `inventory-3-pass3.md`); `docs/admission-ledger.yaml` (new rows and a header
note on port refactors); `docs/repository-map.md` (boundary and port set recorded); `docs/provenance.md` status; the
placement of the I8 boundary test (task 2.6). No product Go code, no Taskfile change. Owner rulings are posted as
comments on #2, #3, #4, #18, #19, #20 and #8; this change records them.
