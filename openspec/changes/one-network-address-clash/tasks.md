# Tasks: one-network-address-clash

Each task ends when its gate passes; 2.4 ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory passed, design reviewed, and accepted by the owner, with answers to the three owner questions.
  Hold: owner acceptance on #328. Gate: `task spec:check`.
- [ ] 1.2 The probe (`notes/probe.md`: source, commands, versions, output) posted as a comment on PR #332. Gate: the
  comment URL recorded here.

## 2. Implement

- [ ] 2.1 The two existing tests' texts updated, the label row added, and the three new tests written; red on the
  current code (R1-R13, the label row, the holder-order test, and the two updated literals), the run recorded; A1-A7
  green before and after. Gate: `go test -race ./component`, run recorded.
- [ ] 2.2 `transport`, `clashes`, `hostAddr`, `exclusiveClaims` and `claimsClash`; both checks use `claimsClash`; 2.1
  green; M1-M6 detected by `task mutate:check`; the ruling table's lines rewritten at this commit. Gate:
  `go test -race ./component` and `task lint`.
- [ ] 2.3 The spec delta is true of the code, each scenario's literal matching its test. Gate: `task spec:check`.
- [ ] 2.4 README, `port_network.go` comment and the ledger `component` row updated. Gate: `task verify`.

## 3. Follow-up, review and close

- [ ] 3.1 Adoption issue filed: `composition` uses the registry's clash rule, not `ExclusiveResources` text; linked
  from the PR without a closing keyword.
- [ ] 3.2 Cross-agent review; `reviewed-by:` in the PR body.
- [ ] 3.3 Archive (`openspec archive one-network-address-clash`) as the last content commit. Gate: `task spec:check`
  and `task docs:check`.
