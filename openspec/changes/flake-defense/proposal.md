# flake-defense

## Why

A flaky test is one that passes and fails on the same code. Owner direction, 2026-10-01 (issue #42): "flaky tests
have to not only be fixed but we have to stop the bleeding properly. flaky tests were part of what spelled doom for
semstreams".

On the day that was said, a test in this repository's own harness had failed in four CI runs on pull requests that
did not touch it (issue #40), and a fifth run failed without naming any test. SemStreams filed 23 flaky-test issues
and closed 21 of them one at a time, after the fact. Fixing #40 does not stop the next one.

`inventory.md` records what lets a flaky test be born, merged and survive here today. In short: most tests run two
or three times before merge, always under the same settings, and the #40 defect passed those settings; sleeps and
skips are forbidden only in prose; a green check stays valid however far `main` has moved; and the rule against
merging past a known flake is read by no command.

## What Changes

The design is in `design.md`, third revision. It passed independent design review (task 2.2), and the owner ruled on
its questions (task 2.3). The third revision records the ruling and waits for a check of that diff (task 2.4).

- **Varied, repeated unit runs.** `task test:unit` runs under the race detector at one CPU. A new step,
  `task test:repeat`, runs the unit suite five times without the race detector at one CPU, in shuffled order. On
  the old #40 code that setting failed 100 times in 100. A contract test pins both command lines.
- **Harness tests made cheap and complete.** The lifecycle suite's matrix test stops waiting 2 s of real time per
  execution, its cleanup helper joins every worker, and its three hand-kept failpoint lists become one table.
- **A failing gate names the test.** `scripts/cover-check.sh` stops discarding test output.
- **Three text checks with no accepted exceptions:** no `time.Sleep` in tests or harness code, no skip calls, and no
  build tag on a test other than `integration`. The one existing skip becomes a failure with its reason. Test files
  ported later are held to the same checks and land repaired.
- **The carried port guard** stops recommending a pattern SemStreams removed as a race, and loses its inline
  exemption marker. Its two ledger rows change from `carry` to `adapt`.
- **A known flake stops merges.** A known flake is an open issue labelled `class:flake`: a test or check that passes
  and fails on the same tree. A failed network fetch is not one. A new CI job, `merge-check`, fails while a known
  flake is open unless the pull request closes every open one. `Required` needs it. There is no waiver.
- **A pull request must be up to date with `main`.** The ruleset's strict setting is turned on, and every CI run
  asks GitHub which rules are in force on `main` and fails unless they still require it.
- **The merge rule's text** in `.agents/protocol.md`, `AGENTS.md` and the preflight skill names these commands, and
  the owner-waiver sentence leaves the protocol's Land step.

Not in this change: a scheduled run, issues filed by automation, a guard on re-runs, quarantine or retries, any change
to the Docker-backed invocation (those tests stay at one execution), and the network fetches in CI. `design.md` says
why for each.

## Capabilities

### New Capabilities

- `merge-gate`: the unit-test invocations `task verify` runs, the rule that a failing gate names the test, the
  known-flake check, the up-to-date rule, and the jobs `Required` needs.

### Modified Capabilities

- `harness-boundaries`: adds "No sleeps in tests" and "No skipped or hidden tests"; changes "No fixed addresses in
  tests" so the port guard has no exemption and its message names nothing outside this repository.
- `lifecycle-suite`: adds "Complete sensitivity matrix".

`integration-test-runner` and `nats-fixture` do not change.

## Impact

- `Taskfile.yml`, `scripts/verify.sh`: one changed command, one new step. `task verify` takes about 90 s longer.
- `scripts/cover-check.sh`, `scripts/lint-test-ports.sh` and its fixture test: small changes.
- `scripts/merge-check.sh`: new.
- `.github/workflows/ci.yml`: a second job that reads issues, pull requests and the rules on `main`; `required`
  needs it.
- `internal/harness/contract`: new tests. `internal/harness/lifecycletest/refowner_test.go` and
  `internal/harness/runner/runner_test.go`: test changes only. No production code.
- `docs/admission-ledger.yaml`, `docs/provenance.md`, `docs/repository-map.md`, `.agents/protocol.md`,
  `AGENTS.md`, the preflight skill: text.
- GitHub: one new label, and one ruleset setting. After it, every merge puts every other open pull request behind
  `main`, and each needs one more CI run before it can merge.
- Slice 04A (PR #48): ported test files land repaired; the text checks and the repeat step apply to ported packages as
  they arrive; the up-to-date rule applies from the moment it is made. The size of that bill is not measured in this
  change. `design.md`, "Effects on slice 04A".
