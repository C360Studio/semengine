# setup-03b-contract-boundary

## Why

SemEngine has a harness and no ported package (`docs/repository-map.md`; `openspec/specs/` holds four harness
capabilities and no graph capability). SETUP 03B (epic #8) must approve the retained contract, the boundary, and the
port set before SETUP 04A extracts anything. SemEngine is a live semantic knowledge graph with durable execution, two
halves admitted by owner mandate at a named tier with a named qualifying workload (`AGENTS.md`; #8 Q14). The first
inventory pass measured 65 packages / 126,926 non-test lines at the frozen pin `8b99efe9` (`inventory.md` A1). Under the
ruled tier-0 scope for both halves, with four starter consumers (semsource, semconnect, semboids, semteams), BM25 at
tier 0 and `composition/cli` admitted, tier 0 is 65 packages / 140,842 lines (`design.md` D4, from
`inventory-3-pass3.md` §2.2 set H). Three reproduced substrate defects (#15, #16, #17), three
contract requests (#18, #19, #20), graph-ingest's settlement under process replacement, and the operator surface had no
SemEngine contract. The owner ruled the questions this change raised on 2026-10-01 except one, which holds a task; this
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
  `pkg/graphview` at tier 0; the embedding family at tier 0 for BM25, with its one provider file
  (`graph/embedding/http_embedder.go`) behind the tier-1 seam; `graph/llm` and `model/wire` in the tier-2 slot, empty
  at MVP. The capability seam is the last task of Slice 04A. Tier 0 = 65 / 140,842, replacing 67 / 129,063.
- The tier model (D3): tier 0 = no external provider, tier 1 = embedding provider, tier 2 = LLM provider; milestones
  #9–#11 are slices 04A (tier-0 foundation), 04B (tier-0 lexical) and 04C (tier-1 neural).
- The critical coverage list extended with the packages entering tier 0, `composition/cli` among them (D10).
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
- `docs/admission-ledger.yaml` gains a header note on the port-refactor convention; its package rows (one per tier-0
  and separated package, dispositions per D4) are added by the Slice 04A changes as they port each package (epic #9,
  "Carried to 04A"). The twelve port refactors are issues #25–#36.
- `docs/contract.md`, "What SemEngine guarantees": the contract written for a developer new to the repository.
- `docs/setup-plan.md` amended in place for the tier model and admission by mandate.
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
- BM25 at tier 0, `composition/cli` admitted, the tier model and slices, and the extended critical list:
  [comment 5932313950](https://github.com/C360Studio/semengine/issues/8#issuecomment-5932313950).
- The operator surface: [comment 5931143569](https://github.com/C360Studio/semengine/issues/8#issuecomment-5931143569),
  adopting the review in [comment 5931117077](https://github.com/C360Studio/semengine/issues/8#issuecomment-5931117077).

Each decision in `design.md` states its ruling. One question is open: whether the rule core refuses an unknown action
type at load or fails at fire time (D12; task 4.9). The architect's unruled forks (E1 registration shape, E2 field
shape, `graph/llm` split shape) are design options for the 04A changes, not holds.

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
(`inventory.md`, `inventory-2-scope.md`, `inventory-3-pass3.md`); `docs/admission-ledger.yaml` (a header note on port
refactors); `docs/contract.md` (new); `docs/setup-plan.md` (amended); `docs/repository-map.md` (boundary and port set
recorded); `docs/provenance.md` status; `.agents/contracts/` (the owner's documentation rule); the preflight skill's
`cover:check` row; issues #25–#36 and the "Carried to 04A" list on #9; the placement of the I8 boundary test (task 2.6).
No product Go code, no Taskfile change. Owner rulings are posted as comments on #2, #3, #4, #18, #19, #20 and #8; this
change records them.
