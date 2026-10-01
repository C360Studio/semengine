# setup-04a-foundation

## Why

Slice 04A is the first extraction slice: the tier-0 graph foundation with no embedder and no model service
(epic #9; `docs/setup-plan.md`, SETUP 04A). SETUP 03B fixed what is ported — 65 packages, 140,842 non-test lines at
the pin `8b99efe9` (archived change `2026-10-01-setup-03b-contract-boundary`, D4) — and the contract it must keep
(`docs/contract.md`). It left to 04A the order of porting and the records each port must extend: the admission
ledger, the five drafted spec deltas, the repair-before-port rows #15, #16 and #17, the port refactors #25–#36, and
the harness extension that the restart and process-kill proving tests need (#9, "Carried to 04A", items 1–9).

The 65 packages cannot land in one change. How they are cut into changes, in what order, and what the first change
must touch is a design question that rests on measured facts: the import structure of the set, what the harness
can and cannot host today, what the records already hold, and what each starter consumer composes.

## What Changes

Nothing yet. This change is in its inventory phase. `inventory.md` records those facts at the pin and at the current
consumer heads: the dependency structure of the tier-0 set (acyclic; 14 leaves, 16 roots; the closure of each named
root set), the ceiling reproduced at the pin (65 / 140,842), the harness surface and its collisions with what the
ported tests already own, the records to extend, and the adopter seams reached from SemSource, SemConnect and
semboids. It deliberately holds no options, recommendation, or slice order.

The next steps, in order, are: options with their costs for cutting the set into changes; an independent design
review; the owner's acceptance; then the first change's spec deltas, tasks and holds, with implementation following
the developer, reviewer and writer loop under the admission heuristic.

## Capabilities

None yet. `.openspec.yaml` sets `skip_specs: true` only because no ruled design exists to write a spec change from.
The design removes that setting when it adds its spec changes.

## Impact

Documents only in this phase: this folder. No Go code, no ledger row, no ported package.
