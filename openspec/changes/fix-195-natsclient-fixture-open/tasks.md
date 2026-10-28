# Tasks: fix-195-natsclient-fixture-open

Light process, owner 2026-10-28: proposal and tasks; no design document, no spec delta; Codex reviews the code.
The change moves no behaviour a test asserts, so no new test comes first: the package's integration tests, every one
of which opens its client through `newFixtureClient` or `newFixtureClientAt`, are the proof that nothing moved, and
`natsfixture`'s own tests (`internal/harness/natsfixture/open_test.go`) prove `Open`.

## 1. Proposal

- [x] 1.1 Proposal and tasks committed. Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [ ] 2.1 `newFixtureClientAt` opens its `Client` through `natsfixture.Open` with a package-local
  `openFixtureClient`. Gate: `git grep -n 'client.Connect(ctx)' natsclient/test_helpers_integration_test.go` prints
  only the line inside `openFixtureClient`; `go test -race -count=1 -tags integration ./natsclient/` passes;
  `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record on the PR. The PR body carries `implemented-by: claude (opus)` and
  `reviewed-by: codex`.
  Hold: Codex reviews the code; the owner starts Codex.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive --skip-specs`, no spec to sync). Gate:
  `task spec:check` and `task docs:check`.
