# Tasks: fix-678-fixture-password

Tests come first. Each code task ends at a clean-tree local `task verify`. Single PR, `Closes #678`.

## 1. Proposal

- [x] 1.1 Proposal and spec delta (`proposal.md`, `specs/nats-fixture/spec.md`); `task spec:check` and
  `task docs:check` pass. Approved 2026-11-17 (#678).

## 2. Code

- [x] 2.1 Test first: `TestBrokerRefusesAClientWithoutItsPassword`
  (`internal/harness/natsfixture/fixture_integration_test.go`) dials the broker's host port with nats.go directly,
  without the password and with a second broker's, and wants `nats.ErrAuthorization`; two brokers' passwords differ;
  `URL()` itself connects. Shown to fail with `--auth` removed and with a fixed password (`notes/2.1.md`).
- [x] 2.2 Implement: the password in `fixture.go`'s start command and `clientURL`; `restart.go` reads it back from the
  old URL; graph-ingest's relay keeps the URL's user part; `docs/test-infrastructure.md` says why a hand-built URL is
  refused. Accept: the natsfixture, graph-ingest and graph-query integration suites pass; full `task verify` passes on
  a clean tree.
- [x] 2.3 Archive: `openspec archive fix-678-fixture-password --yes` promotes the delta; `task spec:check` and
  `task docs:check` pass.
