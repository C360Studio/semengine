# Tasks: natsfixture-host-port

Each code task writes its test first and records the test failing on the code before the change: the output and the
commit it ran on. Tests and their names are in design "Tests". A task's notes on choices the design does not spell
out go in `notes/<task>.md`, with a one-line pull request comment that links the file. No `time.Sleep` anywhere in the
package; waits use `probe.Await` under a bounded context. Tests that bind a port learned at run time spell the address
with `net.JoinHostPort` (design D6). New integration tests go before `TestMaxPayloadIsSettable`, not at the end of
`fixture_integration_test.go`, where PR #93 appends its own.

T7's command, used by tasks 2.1, 3.5 and 6.2 (T5's is the same with `-run TestRestartURLDials`):

```bash
SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS=1800 task test:integration -- -run TestNextDockerPortsHeld ./internal/harness/natsfixture/
```

## 1. Record and decide

- [x] 1.1 Copy `inventory.md`, `design.md`, `proposal.md`, `tasks.md`, `.openspec.yaml` and
  `specs/nats-fixture/spec.md` into `openspec/changes/natsfixture-host-port/` (orchestrator). `probes/` is posted by
  1.2, not copied. Check: `task spec:check`.
- [x] 1.2 Post `probes/` on PR #200: each program with its source, and its command, versions and output from
  `probes/README.md` (orchestrator). Check: `notes/1.2.md` names the comment behind each premise row that cites
  `probes/` (P1, P4 to P11).
- [x] 1.3 Hold: independent pre-owner design review. The reviewer's pass on `design.md` and the delta is recorded on
  PR #200.
- [ ] 1.4 Hold: owner acceptance on #180, answering design questions Q1 and Q2.

## 2. Reproduce issue #180 first (design D3, D6)

- [ ] 2.1 In `internal/harness/natsfixture/fixture_integration_test.go`, write T7 `TestNextDockerPortsHeld` following
  design D6's steps, and change T5 `TestRestartURLDials` to require the same `URL()` after Restart. In a scratch
  worktree at `main` that adds only these two test changes, run T7 five times and T5 once, and record each result in
  `notes/2.1.md`. Expected on Docker Desktop for Mac: T7 fails 5 of 5, and T5 fails because Restart gets a new port.

## 3. Start picks the port and publishes it on 127.0.0.1 (design D1, D2, D4, D5)

- [ ] 3.1 Write T1 `TestPublishedPortReadsOneInspect` and T8 `TestLocalDockerHost` (unit tests in
  `fixture_test.go`) against stubs that return nothing, and record them failing.
- [ ] 3.2 Extend T2 `TestFixtureRoundTrip`. Read the host from `URL()` with `net/url`, never as a `nats://` literal
  (`TestNoFixedAddressesInTests`), and compare it with `docker port`. Record T2 failing on the current code.
- [ ] 3.3 Implement:
  - `deps.pickPort`, counted, which binds `127.0.0.1:0` through `net.ListenConfig.Listen(ctx, ...)` and closes it;
  - the `HostConfigModifier` binding and `URL()` on `127.0.0.1`;
  - the host-phase check, which refuses a gateway address;
  - `deps.mappedPort` reading one `Inspect`;
  - a start error that names the binding.

  Run `go mod tidy`. T1, T2 and T8 pass, and so do the S1 tests unchanged.
- [ ] 3.4 Write T3 `TestHolderOnAllAddressesNeverGetsTheConnection` and T4 `TestPickedPortTakenBeforeDockerBinds`,
  with `pickPort` forced to a port the test holds. T3 chooses its expectation by `runtime.GOOS`, never by a skip:
  on darwin, Start succeeds and every `127.0.0.1` dial reaches NATS; on linux, Start fails at phase `start` naming
  the address. T4 requires phase `start`, one attempt, the address named, no container left, and Stop nil. Record
  both on Docker Desktop. If either does not hold, stop and report to the orchestrator before 3.5. CI's run before
  merge is the first measurement of T3's linux branch; a disagreement there goes to the orchestrator.
- [ ] 3.5 On the fix, run T7 five times and record each result in `notes/3.5.md`. Expected: 0 of 5 fail.
- [ ] 3.6 Hold: Codex review of sections 2 and 3 on PR #200, at 3.5's commit, before section 4 starts.

## 4. Restart keeps its port (design D3)

- [ ] 4.1 Write T6 `TestRestartWithItsPortTaken`, in which the stop-container hook binds the port on `127.0.0.1`
  after the real stop (waiting with `probe.Await`). Record T6's outcome, and T5 (2.1) passing on 3.3's code. If T6
  does not fail at phase `start-container` naming the address, stop and report to the orchestrator.
- [ ] 4.2 Remove Restart's host re-derivation (`restart.go:100-104`) and update the fakes at `fixture_test.go:124`,
  `:155-156` and `:187` to the loopback host. T5, T6 and every existing Restart test pass.
- [ ] 4.3 Hold: Codex review of section 4 on PR #200, at 4.2's commit, before section 5 starts.

## 5. Text (design D2, D3, D5)

- [ ] 5.1 Correct the doc comments:
  - `fixture.go:417`: the URL is on `127.0.0.1`, at a port the fixture picked;
  - `restart.go:28-30`: Restart keeps the port, and callers still re-dial;
  - `errors.go:20-25`: the start phase includes the pick;
  - `natsclient/integration_test.go:47-51`: the restarted broker keeps its URL, and the first client stays down
    because it has no reconnects.

  Check: `task lint`, `task build`.
- [ ] 5.2 Apply `lint-test-ports.diff`, `docs-testing.diff` and `ledger.diff`. Check: `task docs:check`,
  `task ledger:check`, `TestLintTestPortsPasses`, `scripts/lint-test-ports_fixture_test.sh`.

## 6. Gates

- [ ] 6.1 Run `task verify` on a clean tree at the last code commit, including `cover:check` with natsfixture at 80% or
  more. Record the commit and result in `notes/6.1.md`.
- [ ] 6.2 Post the flake evidence on PR #200: T7's command, the before count (2.1), the after count (3.5) and the
  earlier rate on this machine (about 16 of 181 recorded full runs, inventory open question 7).
- [ ] 6.3 Hold: Codex's review of record on PR #200, at 6.1's commit. The PR body names `implemented-by: claude (opus)`
  and `reviewed-by: codex`.
- [ ] 6.4 Archive the change and sync the `nats-fixture` spec as the last content commit. Check: `task spec:check`.
  The merge then follows the protocol's merge gate.
