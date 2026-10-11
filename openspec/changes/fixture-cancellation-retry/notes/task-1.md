# Task 1: cancellation test repair

The existing `nats-fixture` contract allows one mapped-port replacement only while the
parent context is live and cleanup succeeded. [Flake #179](https://github.com/C360Studio/semengine/issues/179)
records the failing local verification case. The unchanged test at commit `829e2e54b00c19ece413b4b1c8bdd7f4ceda16a6`
had SHA-256 `7dd76d1fe3d6e4382a38402eb1ef885c2e89ffb2c18d75d6be051846651ee3ee`.
In the PR #118 full verify and a focused repeat, attempt 1 failed at mapped-port
with a live parent, attempt 2 was canceled during connect, and the old assertion
selected the first error. Those logs are local only under `.enjoy-logs/`.

The deterministic regression injected one mapped-port failure, then canceled
during connect. Before the assertion repair, this command exited 201:

```text
task test:integration -- -run '^TestS1_2CancellationDuringIO/connect_after_mapped-port_failure$' -count=1 -v ./internal/harness/natsfixture/
fixture_integration_test.go:345: Start = natsfixture: attempt 1 phase mapped-port ... (parent context live): injected mapped-port failure
    natsfixture: attempt 2 phase connect ... (parent context context canceled): context canceled, want context.Canceled at phase connect
--- FAIL: TestS1_2CancellationDuringIO/connect_after_mapped-port_failure
```

After the oracle repair, this command exited 0 on the final test file:

```text
task test:integration -- -run '^TestS1_2CancellationDuringIO$' -count=1 -v ./internal/harness/natsfixture/
--- PASS: TestS1_2CancellationDuringIO/start
--- PASS: TestS1_2CancellationDuringIO/connect
--- PASS: TestS1_2CancellationDuringIO/connect_after_mapped-port_failure
PASS
```

Both runs used Node 22 on `PATH`, `SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS=600`,
the canonical integration runner, and Docker access. Local logs:
`.enjoy-logs/pr118-cancel-retry-red-escalated.log` and
`.enjoy-logs/pr118-cancel-retry-green-final.log`; these ignored paths are
local only.

The explicit finite histories suffice instead of generated checks: cancellation
on the first start attempt, cancellation during connect, and cancellation
during connect after exactly one known mapped-port failure. The test inspects
the terminal error, every recorded container, and the start hook after parent
cancellation.

Full `task verify` ran on clean commit `e9b58fa43847751319b99f5a36097db0505ba785`
and failed. Markdown line-length errors were then corrected and `task docs:check`
passed. The other failure was a separate Docker mapped-port lookup in the restart
test, recorded in [#180](https://github.com/C360Studio/semengine/issues/180);
coverage correctly refused that failed integration run. The remaining gates,
including unit tests and the five shuffled repetitions, passed. Full verification
has not passed, and independent implementation review has not started. The
failed full-run log `.enjoy-logs/pr118-e9b58fa-verify.log` is local only.
