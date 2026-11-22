# Tasks: fix-605-natsclient-poller-readiness

Each code task ends at a clean-tree local `task verify`. Single PR, `Addresses #605`.

## 1. Proposal

- [x] 1.1 Proposal (`proposal.md`) and spec delta; `task spec:check` and `task docs:check` pass. Approved 2026-11-22
  (#605).

## 2. Code

- [x] 2.1 `natsclient`: remove the health poller and `WithHealthInterval`, so only `Connect`, the connection handlers
  and `Close` set `Status()`; delete every `WithHealthInterval` call in the repository (tests, the scorecard harness,
  `docs/test-infrastructure.md`) and the poller's test.
- [ ] 2.2 `natsclient`: remove `RequestReadyClassified`, `requestMsgReady` and the readiness constants, their tests
  and rows, and reword the comment in `stream.go` that cites the constants.

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive fix-605-natsclient-poller-readiness --yes`, which syncs `transport-client`;
  `task spec:check` and `task docs:check` pass.
