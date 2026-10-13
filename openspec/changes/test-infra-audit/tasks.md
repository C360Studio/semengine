# Tasks: test-infra-audit

Three pull requests, in the order below (`design.md` § Pull requests). Each developer reads `design.md` and only the
spec deltas its group names. Run `task test:unit` after each code task and `task verify` on a clean tree before the
review stop. Choices the design does not spell out go in `notes/<task>.md`.

## 1. Record (orchestrator)

- [x] 1.1 Inventory passed independent review, round 3, at c5787fe; owner rulings on #205: comment 6132580576
  (no SemStreams coordination; minutes, not hours) and comment 6132995614 (Ryuk off; label, remove, sweep).
- [x] 1.2 Pre-owner design review passes: round 2 at 69868ce, confirmed round 3 at a91c080.
- [x] 1.3 Post the probe's source, command, versions and output (`design.md` § Probe) on PR #206: comment
  6133623880.
- [x] 1.4 Owner accepts the design and answers its questions on #205: comment 6134269684.

## 2. Plain `go test` (pull request 1)

- [x] 2.1 Extend `TestUnitInvocationsPinned` to the `test:integration` command, failing first on today's
  `Taskfile.yml` (`integration-test-runner`, "Integration test command").
- [x] 2.2 Replace `test:integration` with the `go test` command and the coverage fingerprint; `cover-check.sh` reads
  `coverage/` and refuses a missing or different fingerprint, with a test for each refusal.
- [x] 2.3 In natsfixture's once-per-process step: set `TESTCONTAINERS_RYUK_DISABLED=true` first, then sweep labelled
  containers older than 30 minutes; label every container. Tests: "Ryuk is off" and "Only old labelled containers are
  swept" (`nats-fixture`, "Ryuk off, labelled containers and the sweep"). The Ryuk test sets no environment itself,
  `t.Setenv` included.
- [x] 2.4 Move the image pin to `internal/harness/natsfixture/nats-image`, embedded, refused without a digest;
  `TestOneImagePin` and `doctor.sh` read it; remove `SEMENGINE_NATS_IMAGE` and its override, and drop the image from
  the log lines at `natsclient/kv_key_contract_integration_test.go:55` and
  `natsclient/kv_watcher_ownership_integration_test.go:61` (they log the server version already).
- [x] 2.5 Remove admission (`admission.go`, `ErrNotAdmitted`, their tests) and the evidence records; a failed start
  writes the container log to the test output; prochost writes to the test's temporary directory and logs a child's
  output when its test fails (`process-host`).
- [x] 2.6 Remove `scripts/test-integration.sh`, `scripts/admission-lock.sh`, `internal/harness/runner`, the lock lines
  in `scripts/doctor.sh`, its Ryuk lines (`:77-83`, the Ryuk environment and properties-file report, and `:95-105`, the
  Ryuk image check), the evidence upload in `.github/workflows/ci.yml`, and the lock note in `scripts/verify.sh:9`.
- [x] 2.7 Remove the fixture's name generator and the SemStreams substring list; `TestSemEngineAssignedNames` keeps the
  `semengine-` prefix rule; the two `Fixture.Name` callers use literal names.
- [x] 2.8 Update every file that names the runner, the lock, the evidence directory or `SEMENGINE_NATS_IMAGE`:
  `docs/setup-plan.md` (the text in `drafts/setup-plan-change.md`); `docs/testing.md` (`:29-31`, `:57-69`, `:76`,
  `:358`, `:554-569`, `:572`; the first three name `FuzzCheckName`, `CheckName` and `Name(base)`, removed by 2.7);
  `docs/repository-map.md:47,50,51`; `.agents/skills/semengine-preflight/SKILL.md:42,61-64,84`;
  `.agents/skills/semengine-handoff/SKILL.md:28`; `.gitignore:15-16`; the admission-ledger rows whose destination
  this change removes (`docs/admission-ledger.yaml:211,244`), so `task ledger:check` passes. Finish with `git grep -n
  -e test-integration.sh -e admission-lock -e SEMENGINE_NATS_IMAGE -e SEMENGINE_EVIDENCE_DIR -e '\.evidence/'`
  finding nothing outside `openspec/changes/`, `docs/provenance.md`, the history note in `docs/admission-ledger.yaml`
  that reads "the broker's reported version and SEMENGINE_NATS_IMAGE",
  and the planted inputs of `internal/harness/contract/imagepin_test.go:24,27`. ADR-112's `.nats-image:5` citation
  (`docs/adr/112-natsclient-contract.md:13`) is a dated record and is left as written. `docs/setup-plan.md`'s "one NATS
  server per package" becomes true with pull request 3 (4.4).
- [x] 2.9 Add `AGENTS.md` rows 1 and 2 and replace the image-pin row (`drafts/agents-rows.md`).
- [ ] 2.10 Record the short check on the pull request (`design.md` § Short check, steps 1 to 3).
- [x] 2.11 Sync into `openspec/specs/`: all of `integration-test-runner` except "Concurrent runs on one daemon", whose
  scenario cites pull request 3's short check and so syncs at 4.7, and all of `process-host`; from `nats-fixture` the
  removed "Admission before Docker" and "Run-unique ownership and safe names", the modified "Phased start with typed
  failure" and the added "Ryuk off, labelled containers and the sweep"; from `harness-boundaries` "One image pin" and
  the removed and added name requirements.

## 3. Smaller natsfixture (pull request 2)

- [ ] 3.1 Stop closes the connection and removes the container, nothing more; a repeated Stop is a no-op; tests first
  (`nats-fixture`, "Own server per test").
- [ ] 3.2 Remove `Consume`, `CreateKeyValue`, the consumer joins and the operation slot; Restart keeps #200's retry
  (`nats-fixture`, "Restart keeps the container").
- [ ] 3.3 Keep one test seam, the mapped-port read, for the replacement-container test; remove the other 12; the error
  type becomes unexported, with the phase in its text.
- [ ] 3.4 Remove the fixture's lifecycle-floor test; the stream helpers become functions carrying the bounds.
- [ ] 3.5 natsfixture's 80% coverage floor holds.
- [ ] 3.6 Sync into `openspec/specs/` the rest of `nats-fixture` except "Shared server per package", "Reset to a
  verified-empty server" and "Connected value for a package's tests", and `harness-boundaries` "Bounded cleanup
  roots".

## 4. One server per package (pull request 3)

- [ ] 4.1 Failing tests first for the shared server's lazy start and Stop, and for the reset's four scenarios
  (`nats-fixture`, "Shared server per package", "Reset to a verified-empty server").
- [ ] 4.2 Build the shared server and its reset, counting connections through `$SYS.REQ.ACCOUNT.PING.STATZ`; `Open`
  takes a URL.
- [ ] 4.3 Add `TestNoParallelIntegrationTests` and `TestOwnServerReason`, each with its sensitivity test.
- [ ] 4.4 Add `TestMain` to natsclient, processor/graph-ingest, graph/kvcatalog, pkg/lifecycle and component; their
  helpers call the reset; the 8 own-server tests (`design.md` P7) call `natsfixture.New` themselves, each with its
  `// Own server:` line, so `TestOwnServerReason` reads every reason and `startFixture` serves shared tests only.
- [ ] 4.5 Write `docs/test-infrastructure.md` (`drafts/test-infrastructure.md`), add it to the canonical home of
  `AGENTS.md` rows 1 and 2, add rows 3 to 7, and point `docs/testing.md` at the page.
- [ ] 4.6 Record the short check's steps 1 and 3 again on shared servers, including a full run's container count.
- [ ] 4.7 Sync the remaining deltas into `openspec/specs/` and archive the change.
