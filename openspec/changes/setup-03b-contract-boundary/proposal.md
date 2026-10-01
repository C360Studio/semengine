# setup-03b-contract-boundary

## Why

SemEngine has a harness and no ported package (`docs/repository-map.md`; `openspec/specs/` holds four harness
capabilities and no graph capability). SETUP 03B (epic #8) must approve the retained contract, the boundary, and the
port set before SETUP 04A extracts anything. The measured closure at the frozen pin `8b99efe9` is 65 packages /
126,926 non-test lines (`inventory.md` A1), two above the plan's 63-package ceiling; the increase is five entrants
and three departures (A1.1), and `engine` and `flowstore` no longer exist at the pin. Three reproduced substrate
defects (#15, #16, #17) and three contract requests (#18, #19, #20) have no SemEngine contract yet. The owner ruled
the questions this change raised (#2, #3, #4, #18, #19, #20, and SemConnect's status, A10) on 2026-10-01; this
change records those rulings and turns them into rows and tasks.

## What Changes

- The retained-contract matrix (`design.md`), complete against SemSource's 03A baseline at the pin, with a second
  observed column reserved for SemConnect (PR semconnect#74) so a second consumer adds a column, not a restructure.
- The registration cut: explicit per-package registration in each consumer's composition root; no SemEngine
  aggregator (D1).
- The port set: 65 at the pin, with sixteen packages separated at tier 0 (agentic group, `pkg/lifecycle`,
  higher-tier graph libraries, `graph-gateway`) and two SemConnect leaves admitted (`graph/geo/geojson`,
  `vocabulary/export`) → ceiling re-measured to 67 / 129,063 (D4, D9); tier-0 target about 49 packages. The six
  higher-tier graph libraries stay dormant through the first green tier-0 extraction; their capability seam is the
  last task of 04A.
- Port refactors (seams, file-level adapts, dropped fields) are tracked as the risk class `class:port-refactor`:
  each is an `adapt` ledger row citing its matrix row and proving test (D4a).
- The capability-level word is "tier", with the fallback ladder as a contract element (D3, #4).
- A SemEngine boundary test that no file imports `github.com/c360studio/semstreams` (D2, I8), placed in the first
  04A change that carries the `harness-boundaries` delta.
- The fusion and graph-tool boundary: four layers mapped to code; `Engine.Fuse` lens path kept, package-level
  `fusion.Fuse` deferred, `searchGraph` kept under its degraded contract with community/LLM behind a seam;
  find/anchor/ask recorded as cases (D5).
- Repair-before-port rows for #15, #16, #17, #19, #20 and the ruling-4 lifecycle debt (SS#1411, #1415, #1218,
  #1220, #1417, #1145/#1147), each with its tier-0 (#9) gate evidence (D11).
- The critical package list for the 80% coverage gate (D10).
- `docs/admission-ledger.yaml` seeded with one row per package in the 67 (plus the two SemSource-composed
  components outside it), dispositions per D4.
- Spec deltas for the new contract invariants are **drafted in `design.md`** and carried by the SETUP 04A extraction
  changes, because `openspec/specs/` is current truth verified against code; this change sets `skip_specs: true`.

## Owner rulings

The owner ruled Q1–Q13 on 2026-10-01
([#8 comment 5929656835](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929656835); questions in
[comment 5929442167](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929442167)), copied to
issues #2, #3, #18, #19, and #20. The word "tier" was ruled on
[#4](https://github.com/C360Studio/semengine/issues/4#issuecomment-5929716018). Each decision in `design.md` states
its ruling; the tasks that depended on them carry no hold.

## Non-goals

Extraction of any package; SemStreams patches (frozen pin, ruling 3); the applied-input barrier primitive (#18,
deferred with consequence recorded); a terminal operation-status lookup (#20, deferred); SemSource mainline cutover;
SemConnect cutover; `output/websocket` and `processor/graph-clustering` admission (deferred to a consumer-raised
row); physical purge (semsource#210); generation and community enrichment admission.

## Capabilities

### New Capabilities

None in this change (`skip_specs: true`). Drafted for SETUP 04A: `graph-transport-boundary` (#16),
`graph-ingest-recovery` (#15, acknowledged-write contract), `config-desired-state` (#17), `projection-mutation`
(#19, #20).

### Modified Capabilities

None in this change. Drafted for SETUP 04A: `harness-boundaries` gains the aggregator-free import requirement (T-B8)
and the no-SemStreams-import requirement (I8).

## Impact

`openspec/changes/setup-03b-contract-boundary/{proposal,design,tasks,inventory}.md`; `docs/admission-ledger.yaml`
(new rows and a header note on port refactors); `docs/repository-map.md` (boundary and port set recorded);
`docs/provenance.md` status; the placement of the I8 boundary test (task 2.6). No
product Go code, no Taskfile change. Owner rulings are posted on #2, #3, #4, #18, #19, #20 and #8 as comments; this
change records them.
