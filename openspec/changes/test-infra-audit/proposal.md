# Proposal: test-infra-audit

## Why

Integration tests run NATS in Docker through testcontainers, and that part works. What fails is the machinery built
around it: a host lock shared with SemStreams, an admission token, a 573-line bash runner with 1,216 lines of contract
tests, a second container cleanup beside Ryuk, and a fixture that tears down streams inside a container it is about to
remove. Most recorded flakes and harness bugs exist only because of that machinery: issues #50, #62, #124, #126,
and issues #128, #201, #202 and #203 (`inventory.md` § 4). Every test also starts its own container: 271 in one full
run, and each one is a new host-port allocation exposed to the Docker Desktop port fault (#180).

The owner's bar (2026-10-12): remove the clever in favour of simple yet detailed; keep NATS in Docker; verify in
minutes; do not coordinate with SemStreams (#205 comment 6132580576); turn Ryuk off and label, remove and sweep
(comment 6132995614); share one server per package, with rules and the checks behind them.

## What changes

- `task test:integration` becomes plain `go test` with fixed flags and Ryuk off. The host lock, the admission token,
  the process-group runner, the leak check and the evidence directory are removed. `go test -tags integration
  ./natsclient` runs on its own (#129).
- Every container the harness starts carries a SemEngine label. Each test process, before its first container, removes
  labelled containers older than 30 minutes, which only a killed or crashed run leaves behind.
- Each package's integration tests share one NATS server, started in `TestMain`. A test reaches it only through a reset
  that leaves the server with no stream, no consumer and no other client connection, and fails the test when it cannot.
  A test that needs its own server (restart, stop, a server option, account-wide totals) says why in the code.
- `natsfixture` drops what removing the container already does: owned-resource teardown, observed absence, consumer
  tracking, its name generator, 12 of its 13 test seams, and three features with no user.
- New guidance, `docs/test-infrastructure.md`, and one `AGENTS.md` row per rule, each naming its check.
- `docs/setup-plan.md` stops requiring Docker admission shared with SemStreams.

## Impact

- Specs: `integration-test-runner`, `nats-fixture`, `harness-boundaries`, `process-host`. `merge-gate` and `spec-queue`
  are unchanged (design § Specs).
- Removed: `scripts/test-integration.sh`, `scripts/admission-lock.sh`, `internal/harness/runner`,
  `natsfixture/{admission,evidence,names}.go` and most of `deps.go` and `stop.go`; CI's evidence upload.
- Changed: `Taskfile.yml`, `scripts/cover-check.sh`, `scripts/doctor.sh`, `.github/workflows/ci.yml`, the test helpers
  of `natsclient`, `processor/graph-ingest`, `graph/kvcatalog`, `pkg/lifecycle` and `component`.
- Issues: closes #129's refusal, #202, #128, #203; #180 stays open (a Docker Desktop defect this change does not fix;
  it lowers exposure from about 271 allocations a run to about 60).
- Three pull requests, in order (design § Pull requests). PR #188 merges before the third.
