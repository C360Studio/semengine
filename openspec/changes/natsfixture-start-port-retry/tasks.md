# Tasks: natsfixture-start-port-retry

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/2.1.md` the reproduction
command and before/after results (5 runs on `main`'s code vs 5 runs with the fix, per `.agents/protocol.md` § Known
flakes), plus mutation check results.

## 1. Design

- [x] 1.1 Owner ruling on #345 Clarify gate (move `prepareDocker` before `freePort()` and retry `start` once on
  `"address already in use"`, 2 attempts total) recorded in `proposal.md`, `design.md`, and `specs/nats-fixture/spec.md`.
  Gate: `task spec:check` and `task docs:check`.

## 2. Implementation and tests (D1)

- [x] 2.1 Add `TestStartRetriesOnceOnPortInUse` (`recovers_on_second_attempt`, `fails_after_two_attempts`,
  `no_retry_when_context_cancelled`, `no_retry_on_non_port_in_use_error`) in
  `internal/harness/natsfixture/start_port_integration_test.go`, show that `recovers_on_second_attempt` fails 5/5 times
  on `main`'s single-attempt `Start` with #345's Docker `"address already in use"` error, then implement the
  `prepareDocker` ordering and two-attempt `"address already in use"` retry in `internal/harness/natsfixture/fixture.go`
  and verify 5/5 passes and mutation detection in `notes/2.1.md`. Gate:
  `go test -tags integration -race -count=5 -run TestStart ./internal/harness/natsfixture` and `task mutate:check`.
- [x] 2.2 Update `docs/test-infrastructure.md` and `scripts/lint-test-ports.sh` comments to describe the one
  `start`-phase retry on `"address already in use"`. Gate: `task docs:check` and `task lint`.
- [x] 2.3 Run `task verify` on a clean tree and post the `Review request` comment on the PR.

## 3. Review and close

- [ ] 3.1 Hold: awaiting cross-agent review (`claude` or `codex` `semengine-reviewer`) on the implementation commit;
  address any findings and record `reviewed-by:` in the PR body once approved.
- [ ] 3.2 Archive the change and sync `openspec/specs/nats-fixture/spec.md` as the last content commit. Gate:
  `task spec:check`.
