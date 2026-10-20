# Tasks: one-network-address-clash

Each task ends when its gate passes; 2.4 ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory passed, design reviewed, and accepted by the owner, #328 comment 6290643374 (Q1 refuse everywhere,
  Q2 `udp` in any letter case, Q3 the refused component named). Gate: `task spec:check`.
- [x] 1.2 The probe (`notes/probe.md`: source, commands, versions, output) posted as a comment on PR #332. Gate: the
  comment URL recorded here:
  <https://github.com/C360Studio/semengine/pull/332#issuecomment-6290561084>

## 2. Implement

- [x] 2.1 The two existing tests' texts updated, the label row added, and the three new tests written; red on the
  current code (R1-R13, the label row, the holder-order test, and the two updated literals), the run recorded; A1-A8
  green before and after. Gate: `go test -race ./component`, run recorded.
- [x] 2.2 `transport`, `clashes`, `hostAddr`, `exclusiveClaims` and `claimsClash`; both checks use `claimsClash` and
  the new refusal texts; 2.1 green; M1-M7 detected by `task mutate:check`; the ruling table's lines rewritten at this
  commit. Gate: `go test -race ./component` and `task lint`.
- [x] 2.3 The spec delta is true of the code, each scenario's literal matching its test. Gate: `task spec:check`.
- [x] 2.4 README, `port_network.go` comment and the ledger `component` row updated. Gate: `task verify`.
- [ ] 2.5 #339: `hostAddr` drops an IPv6 zone except on a link-local address; R14-R17 and A9-A10 added (R14-R16 red
  before the change, the run recorded); M8 and M9 detected by `task mutate:check`, M3 re-run as restated; the spec
  delta, ruling table, README and ledger `component` row updated; `notes/probe-339.md` posted as a PR comment.
  The PR body carries `Closes #339`. Gate: `go test -race ./component`, then `task verify`.

## 3. Follow-up, review and close

- [x] 3.1 Adoption issue filed: `composition` uses the registry's clash rule, not `ExclusiveResources` text; linked
  from the PR without a closing keyword. Filed as #338.
- [ ] 3.2 Cross-agent review; `reviewed-by:` in the PR body.
- [ ] 3.3 Archive (`openspec archive one-network-address-clash`) as the last content commit. Gate: `task spec:check`
  and `task docs:check`.
