# Tasks: one-exclusive-address-check

Tasks 2.1-2.6 are steps 1-6 of the design's "Change". Each task ends when its gate passes; 2.6 ends when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Design accepted by the owner on 2026-10-19, with a yes on one component claiming one address twice (#324
  comment 6278113073). Gate: `task spec:check`.

## 2. Implement

- [x] 2.1 `TestRegistryRefusesASecondComponentOnATakenAddress` with its sensitivity pair, green; M1 detected by
  `task mutate:check`. Gate: `go test -race ./component`.
- [x] 2.2 `TestRegistryRefusesOneComponentClaimingAnAddressTwice`, red on the current code, the run recorded.
- [x] 2.3 The refusal in `exclusiveResources`; `compactStrings`, the self-skip and its parameter deleted; 2.2 green;
  M2 detected by `task mutate:check`. Gate: `go test -race ./component` and `task lint`.
- [x] 2.4 Flowgraph's copy and its tests deleted; "two conflicts at once" rewritten with two KV buckets. Gate:
  `go test -race ./internal/flowgraph` and `task lint`.
- [x] 2.5 The spec delta is true of the code: each scenario names the test that drives it. Gate: `task spec:check`.
- [x] 2.6 Docs and both ledger rows updated; `task verify` passes on a clean tree. Gate: `task verify`.

## 3. Review and close

- [x] 3.1 Cross-agent review; `reviewed-by:` in the PR body. Codex approved at `a65ceef` (PR #327 review
  record); its two MEDIUM documentation findings are fixed in the design.
- [ ] 3.2 Archive (`openspec archive one-exclusive-address-check`) as the last content commit. Gate:
  `task spec:check` and `task docs:check`.
