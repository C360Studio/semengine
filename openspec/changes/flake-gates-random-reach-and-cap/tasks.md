# Tasks: flake-gates-random-reach-and-cap

Each step ends when `task verify` passes on a clean tree at its commit (#171). One PR; closes #500.

## 1. Design

- [x] 1.1 Design track (`design.md`); owner ruling on #500 (comment 6580240783).

## 2. Guard

- [x] 2.1 `TestNoReachCheckAfterGeneratedRun` and its sensitivity test in `internal/harness/contract/testtext_test.go`
  (design D1, D2), every scenario of the spec delta shown able to fail. Gate:
  `go test -race -count=1 -cpu 1 -timeout 5m ./internal/harness/contract`.

- [x] 2.2 `service/component_manager_prop_test.go`: a witness history per branch before the run, counts only logged
  after (design D5); the guard then names no file. Gate: `go test -race -count=1 -cpu 1 -timeout 5m ./service/`.

## 3. Rule text

- [x] 3.1 Contract text (D3) and the cap (D4): `docs/testing.md`, developer, reviewer and architect contracts,
  `AGENTS.md` sections 3 and 4, `.agents/rules-index.md`. Gate: `task docs:check`, `task ateam:doctor`.

## 4. Land

- [ ] 4.1 Implementation verified and independently reviewed; specs synced and change archived.
