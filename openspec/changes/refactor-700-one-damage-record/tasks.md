# Tasks: refactor-700-one-damage-record

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`, or a `task mutate:check` run. Premises are in `inventory.md` and `design.md`. One pull request,
draft #701, which closes #700.

## 1. Design

- [x] 1.1 Architect inventory and design (`inventory.md`, `design.md`, the spec delta), INVENTORY PASS, a passing
  pre-owner design review, and rulings on Q1 and Q2.

## 2. The shared record (design D2, D3)

- [x] 2.1 Tests first in `internal/damage`: the named cases and `TestPropRecordFollowsLatestEvent` (design "Proving
  tests"), shown failing against a stub; seed and completed runs recorded.
- [x] 2.2 `internal/damage`: `Record` and `Entry` with a package comment; `docs/repository-map.md` row.

## 3. The follower (design D5)

- [x] 3.1 `graph/follower/damaged.go` reads `damage.Record`; `damagedEntry`, `damagedRecord` and the hand-written
  sort go. `damaged_test.go`, `follower_test.go` and `follower_integration_test.go` pass with no edit. Ledger: the
  `graph/follower` row's dependencies name `internal/damage`.

## 4. graph-ingest (design D4)

- [ ] 4.1 `TestHealthNamesTenPoisonedEntities` (Health sample and the debug listing of 11), shown failing against a
  sample cap changed to 9.
- [ ] 4.2 `TestGaugeFollowsConcurrentRecordAndClear` (design "Proving tests"), shown failing against a gauge set after
  the lock is released, or recorded as inconclusive in `notes/4.2.md` if the race does not fail reliably. Then
  `processor/graph-ingest/poison_inventory.go` and `component.go:123-125` read `damage.Record`: record, drop, Health
  message, `DebugStatus` and the size fast path; the gauge follows the record (design D4), with the choice recorded in
  `notes/4.2.md`. The sweep keeps `entityPoisonRecord`. Change only the 10 size reads and the `poisonInventoryEntry`
  helper body (inventory section 4);
  every other graph-ingest test passes with no edit. Ledger: the `processor/graph-ingest` row's dependencies name
  `internal/damage`.

## 5. Close-out

- [ ] 5.1 Archive the change and sync `graph-ingest-recovery` (`openspec archive refactor-700-one-damage-record`), the
  last content commit.
