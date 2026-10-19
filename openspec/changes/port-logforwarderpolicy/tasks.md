# Tasks: port-logforwarderpolicy

Each task names its outcome and the gate that proves it. Tests come first. Each stop's task ends only when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner. Gate: `task spec:check` and `task docs:check`.

## 2. Port

- [x] 2.1 Tests first: `internal/logforwarderpolicy/policy_test.go` from the pin, unchanged (the compile failure on
  the base noted); then `policy.go` unchanged; the ledger row; a `docs/repository-map.md` row. Gate:
  `go test -race ./internal/logforwarderpolicy` and `task ledger:check`.
- [x] 2.2 `task mutate:check` for M1-M7 (detections) and S1-S5 (survivors recorded) in `notes/2.2.md`; `task verify`
  passes on a clean tree.
- [ ] 2.3 Cross-agent review stop (Codex); `reviewed-by:` in the PR body.

## 3. Close

- [ ] 3.1 Archive (`openspec archive port-logforwarderpolicy`) as the last content commit. Gate: `task spec:check`
  and `task docs:check`.
