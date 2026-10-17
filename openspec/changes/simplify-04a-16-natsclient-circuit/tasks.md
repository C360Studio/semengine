# Tasks: simplify-04a-16-natsclient-circuit

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec deltas pass pre-owner review and are
  accepted by the owner with rulings on owner questions 1-4 (#223 comment 6233442619). Gate: `task spec:check` and
  `task docs:check`.
- [x] 1.2 Merge `origin/main` into the claim branch if it moved (#286 landed; merge `2c32679`). Gate: `task verify`
  on the merge (exit 0 at `91de7ed`).

## 2. Failing tests

- [x] 2.1 `TestIntegration_FailedCallsRefuseNoOtherCall`, with `WithHealthInterval(0)` (D1). Gate: fails on base
  (call 16 returns `ErrCircuitOpen`).
- [x] 2.2 `TestHealthMonitorTickReturnsStatusAndClose` (D2). Gate: passes on base; with `Status()` reading under `mu`
  it fails on its own deadline before cleanup runs (`task mutate:check`, reported as
  detected or not).
- [x] 2.3 The two replacement oracles: the outbound message count for `MaxAttempts` 4, and the responder that drops
  its first request (Design, Tests). Gate: both pass on base; each detects a mutant in which the retrying call makes one
  attempt whatever the config says (`task mutate:check`).
- [ ] 2.4 The tests Design, Tests lists are deleted or edited; `fakeJetStream` moves to a helper file. Gate: compile
  after 3.1-3.3.

## 3. Implementation

- [ ] 3.1 Remove the breaker and its call sites (D1). Gate: 2.1 passes; `go build ./...`; `task test:unit`.
- [ ] 3.2 Typed atomic status. Remove `Status.FailureCount`, `LastFailureTime`, `Reconnects`, `Client.Failures`,
  `Backoff` and `connStatus` (D2; Q4). Gate: 2.2 and 2.3 pass; `task test:unit` under `-race`;
  `task test:integration`.
- [ ] 3.3 `pkg/errs`, graphmutation and projection callers, and the comments in `graph` and `graph/kvcatalog` (D3).
  Gate: `go build ./...`; `task test:unit`.

## 4. Documentation

- [ ] 4.1 Spec deltas match the code; ADR-112 amendment; README and `doc.go` (D4). Gate: `task spec:check`,
  `task docs:check`.
- [ ] 4.2 Ledger natsclient, graphmutation and errs rows (D4). Gate: `task ledger:check`.
- [ ] 4.3 #291 (graph-ingest `Start`) is linked from the PR without a closing keyword, and #222 gets a comment
  correcting part c's premises. Gate: each is read back.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head (includes `task cover:check`, natsclient
  merged).
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive (`openspec archive simplify-04a-16-natsclient-circuit`) as the last content commit. Gate:
  `task spec:check`.
