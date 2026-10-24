# Proposal: setup-04a-04-graph-roots-revlag

## Why

Change 4 (`setup-04a-04-graph-roots`) of epic #9 ports eight graph packages, one per pull request (#159). The owner
set the order on 2026-10-24 (#9 comment 6386856657): `pkg/revlag` first, then the processor shell (#109), then
graph-index, the watermark's first reader in this tree. `pkg/revlag` is 213 pin lines at SemStreams `8b99efe9`
(`watermark.go`, plus a 329-line test). It is the low-water-of-pending watermark that graph-index and graph-embedding
use to report, honestly, how far they have caught up (pin ADR-066 section 1, ADR-083 decision 3).

## What Changes

- Port `pkg/revlag` as `internal/revlag` with an `adapt` ledger row. It moves to `internal/` because no starter
  consumer imports it and no exported signature names its type (#9 comment 5953295358).
- Keep `Watermark`, `New`, `Observe`, `Complete`, `Observed` and `IndexedAt`. Drop `Indexed`, which has no production
  reader at the pin and returns the same floor as `IndexedAt`. Reword the doc lines that name it.
- Port the pin's 13 tests. Add three examples the pin lacks, a concurrent reader-and-writer test, a retention test,
  and a Rapid history checked after every step against the pin's own code.
- `Complete` and the floor read stop scanning every in-flight revision (owner, Q1, PR #407 comment 6388065578). Every
  result stays equal to the pin's, and retained state stays proportional to the in-flight count.
- Correct `docs/repository-map.md` (change 3 is complete) and ADR-066's port note.

## Capabilities

- New capability `revision-watermark`: the floor, key-scoped completion, `Observed`, monotonicity under ascending
  observation, the commit-time bound, retained memory, and concurrent use.

## Impact

- New package `internal/revlag` (about 230 production lines), its ledger row, a map row, the ADR-066 port-note edit,
  and the `revision-watermark` spec. No importer until graph-index. `scripts/port-status.sh`
  counts it with no change.
