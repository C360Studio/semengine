# Tasks: lock-owner-same-machine

The developer runs on Opus. Run `task test:unit` after each code task and `task verify` on a clean tree at the end.

## 1. Code (developer)

- [ ] 1.1 A test first for each judge that fails on today's code: an owner record whose host differs from the current
  name, with a dead pid, is judged stale (runner, `scripts/test-integration.sh`) and refused as not live (Go,
  `natsfixture` admission); with a live pid and its start time, it is live. Then drop the host comparison at
  `scripts/test-integration.sh:176` and `internal/harness/natsfixture/admission.go:93-97`, and fix their comments.
- [ ] 1.2 `TestR4CanonicalInvocationAndOwnerFormat` (`internal/harness/runner/runner_test.go:830`) checks the
  recorded host is not empty; remove `hostname(t)` if nothing else uses it.
- [x] 1.3 The spec deltas: every scenario or requirement in `openspec/specs/integration-test-runner/spec.md` and
  `openspec/specs/nats-fixture/spec.md` that treats an owner on another host specially, as MODIFIED requirements
  under `specs/` in this change. Check: `task spec:check`.
- [ ] 1.4 `task verify` on a clean tree at the last commit.

## 2. Close

- [ ] 2.1 Archive this change with `natsfixture-restart-port` as the last content commit. Check: `task spec:check`.

Task 2.1 is completed by the archive commit; the review, CI and `task merge:check -- 200` are recorded on PR #200.
