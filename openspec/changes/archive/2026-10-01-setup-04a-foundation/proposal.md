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

The design is accepted (owner rulings on #9, 2026-10-01; `design.md`, "Owner rulings"). It cuts the tier-0 set into
seven changes by root-set closure, each admitting its packages with their tests, ledger rows and repair proofs green,
and fixes the first change's scope: the transport and message floor plus the harness extension. This change itself
ports nothing and adds no spec delta; it records the inventory, the design and the rulings, and archives as
design-only.

`inventory.md` records the facts the design rests on, at the pin and at the current consumer heads: the dependency
structure of the tier-0 set (acyclic; 14 leaves, 16 roots; the closure of each named root set), the ceiling reproduced
at the pin (65 / 140,842), the harness surface and its collisions with what the ported tests already own, the records
to extend, and the adopter seams reached from SemSource, SemConnect and semboids. Both files passed independent
review before the owner read them (record on PR #47).

What follows this change: `setup-04a-01-floor`, then the six changes named in `design.md` D2, each with its own spec
deltas, tasks and holds, and the developer, reviewer and writer loop under the admission heuristic.

## Capabilities

None. `.openspec.yaml` keeps `skip_specs: true`: this change is design-only, and the spec deltas it names (D10) are
drafted in each of the seven changes' own OpenSpec change.

## Impact

Documents only: this folder, and `docs/repository-map.md` at archive time. No Go code, no ledger row, no ported
package.
