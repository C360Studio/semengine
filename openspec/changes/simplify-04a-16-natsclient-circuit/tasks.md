# Tasks: simplify-04a-16-natsclient-circuit

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec deltas pass pre-owner review and are
  accepted by the owner with rulings on owner questions 1-3. Gate: `task spec:check` and `task docs:check`.
  Hold: owner rulings on questions 1-3.
- [ ] 1.2 Merge `origin/main` into the claim branch if it moved. Gate: `task verify` on the merge.

## 2. Failing tests

- [ ] 2.1 `TestIntegration_FailedCallsRefuseNoOtherCall` (D1). Gate: fails on base (call 16 returns
  `ErrCircuitOpen`).
- [ ] 2.2 Tests that name removed symbols are deleted or edited as Design, Tests lists; `fakeJetStream` moves to a
  helper file. Gate: fail to compile on base after the 3.x removals are stubbed, then compile.

## 3. Implementation

- [ ] 3.1 Remove the breaker and its call sites (D1). Gate: 2.1 passes; `go build ./...`; `task test:unit`.
- [ ] 3.2 Typed status; remove `Status.FailureCount`, `LastFailureTime`, `Reconnects` and `connStatus` (D2). Gate:
  `task test:unit` under `-race`; `task test:integration`.
- [ ] 3.3 graphmutation and projection callers (D3). Gate: `go build ./...`; `task test:unit`.

## 4. Documentation

- [ ] 4.1 Spec deltas match the code; ADR-112 amendment; README and `doc.go` (D4). Gate: `task spec:check`,
  `task docs:check`.
- [ ] 4.2 Ledger natsclient and graphmutation rows (D4). Gate: `task ledger:check`.
- [ ] 4.3 The two issues in Design, Open issues, are filed and linked from the PR without a closing keyword, and #222
  gets a comment correcting part c's premises. Gate: each is read back.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head (includes `task cover:check`, natsclient
  merged).
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive (`openspec archive simplify-04a-16-natsclient-circuit`) as the last content commit. Gate:
  `task spec:check`.
