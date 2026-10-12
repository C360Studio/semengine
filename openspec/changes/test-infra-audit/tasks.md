# Tasks: test-infra-audit

Three pull requests, in the order below (`design.md` § Pull requests). Each developer reads `design.md` and only the
spec deltas its group names. Run `task test:unit` after each code task and `task verify` on a clean tree before the
review stop. Choices the design does not spell out go in `notes/<task>.md`.

## 1. Record (orchestrator)

- [x] 1.1 Inventory passed independent review, round 3, at c5787fe; owner rulings on #205: comment 6132580576
  (no SemStreams coordination; minutes, not hours) and comment 6132995614 (Ryuk off; label, remove, sweep).
- [ ] 1.2 Pre-owner design review passes.
- [ ] 1.3 Owner accepts the design and answers its questions on #205. Hold: owner ruling on #205.

## 2. Plain `go test` (pull request 1)

- [ ] 2.1 Extend `TestUnitInvocationsPinned` to the `test:integration` command and its environment, failing first on
  today's `Taskfile.yml` (`integration-test-runner`, "Canonical invocation").
- [ ] 2.2 Replace `test:integration` with the `go test` command, Ryuk off and the coverage fingerprint; `cover-check.sh`
  reads `coverage/` and refuses a missing or different fingerprint, with a test for each refusal.
- [ ] 2.3 Label every natsfixture container and sweep labelled containers older than 30 minutes before a process's
  first container, with the sweep's integration test (`nats-fixture`, "Labelled containers and the sweep").
- [ ] 2.4 Move the image pin to `internal/harness/natsfixture/nats-image`, embedded; `TestOneImagePin` and
  `doctor.sh` read it; remove `SEMENGINE_NATS_IMAGE` and its override.
- [ ] 2.5 Remove admission (`admission.go`, `ErrNotAdmitted`, their tests) and the evidence records; a failed start
  writes the container log to the test output.
- [ ] 2.6 Remove `scripts/test-integration.sh`, `scripts/admission-lock.sh`, `internal/harness/runner`, the lock lines
  in `doctor.sh` and the evidence upload in `ci.yml`.
- [ ] 2.7 Remove the fixture's name generator and the SemStreams substring list; `TestSemEngineAssignedNames` keeps
  the `semengine-` prefix rule; the two `Fixture.Name` callers use literal names.
- [ ] 2.8 Edit `docs/setup-plan.md` (the text in `design.md`'s handoff) and the runner lines of `docs/testing.md`.
- [ ] 2.9 Record the short check on the pull request (`design.md` § Short check, steps 1 to 3).
- [ ] 2.10 Sync `integration-test-runner`, `process-host`, and from `nats-fixture` the removed "Admission before
  Docker" and "Run-unique ownership and safe names" and the added "Labelled containers and the sweep", and from
  `harness-boundaries` "One image pin" and "SemEngine-assigned names", into `openspec/specs/`.

## 3. Smaller natsfixture (pull request 2)

- [ ] 3.1 Stop closes the connection and removes the container, nothing more; a repeated Stop is a no-op; tests first
  (`nats-fixture`, "Own server per test").
- [ ] 3.2 Remove `Consume`, `CreateKeyValue`, the consumer joins and the operation slot; Restart keeps #200's retry.
- [ ] 3.3 Keep one test seam, the mapped-port read, for the replacement-container test; remove the other 12; the error
  type becomes unexported, with the phase in its text.
- [ ] 3.4 Remove the fixture's lifecycle-floor test; the stream helpers become functions carrying the bounds.
- [ ] 3.5 natsfixture's 80% coverage floor holds.
- [ ] 3.6 Sync the rest of `nats-fixture`, except the shared server, the reset and "Connected value", into
  `openspec/specs/`.

## 4. One server per package (pull request 3)

- [ ] 4.1 Failing tests first for the shared server's Stop and the reset's three proofs (`nats-fixture`, "Shared server
  per package", "Reset to a verified-empty server").
- [ ] 4.2 Build the shared server, its reset and its connection checks; `Open` takes a URL.
- [ ] 4.3 Add `TestNoParallelIntegrationTests` and `TestOwnServerReason`, each with its sensitivity test.
- [ ] 4.4 Add `TestMain` to natsclient, processor/graph-ingest, graph/kvcatalog, pkg/lifecycle and component; their
  helpers call the reset; mark the own-server tests (`design.md` P7). Hold: PR #188 merges first.
- [ ] 4.5 Write `docs/test-infrastructure.md`, add the `AGENTS.md` rows, and point `docs/testing.md` at the page.
- [ ] 4.6 Record a full run's container count and wall time (`design.md` § Short check, step 3).
- [ ] 4.7 Sync the remaining deltas into `openspec/specs/` and archive the change.
