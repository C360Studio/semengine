# Tasks: lock-identity-locale

Each task names the outcome and the gate that proves it. "This pull request" is PR #127; evidence is recorded there as
a comment. Every outcome below is reached on the branch, in or before the archive commit. (D) is the developer, (W)
the technical writer.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 127` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS` on revision 1, and its five corrections folded
      into revision 2, recorded on this pull request with both files' checksums.
- [x] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the two spec deltas:
      `DESIGN REVIEW PASS`, recorded on this pull request.
- [x] 1.3 The owner's acceptance of the design, recorded on #126 or this pull request, with an answer to each item of
      `design.md`, "Owner decisions".
- [x] 1.4 Q1 settled before 2.2: either the owner's word that the measure at the SemStreams pin stands, or, on the
      owner's word, a read of `scripts/run-integration-tests.sh` at SemStreams `main` recorded on this pull request
      that shows its owner parser ignores a key it does not know. If it does not, the design returns to review.
- [ ] 1.5 The independent pre-owner review of `design.md` revision 2 (D7, L7, I5, owner decisions 5, 6 and 8, the
      "Conformance" heading), this file's tasks 1.5 to 2.11 and the runner spec delta's publication text and three
      scenarios: `DESIGN REVIEW PASS`, recorded on this pull request.
- [ ] 1.6 The owner's acceptance of `design.md`, owner decision 8 (D7 and L7), recorded on #126 or this pull request.
      Hold: the owner's acceptance of D7 (#126 or PR #127)

## 2. The fix and its tests

- [x] 2.1 (D) The tests of `design.md`, D6, R4 and R1's "identity_utc unknown" among them, written first and run on
      the base code: T1, R2's "changed identity_utc", R4, "an owner read in another time zone", "identity_utc names
      another start time" and "identity_utc unknown" fail for the reasons D6 names (T1: runner B exits 0 having
      quarantined runner A's live lock; R4: the record has six keys); every other test passes. Gate: that run's
      output, recorded on this pull request with the system it ran on.
- [x] 2.2 (D) The runner, as `design.md`, D1 and D2 state: `scripts/test-integration.sh` writes `identity_utc` as the
      record's last line, reads it, and judges by it when it is usable; the comments at `:5-6`, `:134` and
      `scripts/admission-lock.sh:3-4` as D5 states. Gate: `bash -n scripts/test-integration.sh` and
      `task test:unit`, the runner tests among them passing.
- [x] 2.3 (D) Admission, as `design.md`, D2 and D4 state: `admission.go` reads and compares `identity_utc` when it is
      usable, with the fixed read under `Start`'s context, and falls back to `identity` otherwise; its comments as D5
      states; the admission tests of D6. Gate: `task test:unit`.
- [x] 2.4 (D) The runner row of `docs/admission-ledger.yaml` (`:203`, `:207-209`) as `design.md`, D5 states. Gate:
      `task ledger:check`.
- [x] 2.5 (D) Shown able to fail: the wrong changes M1 to M5 of `design.md`, D6, one at a time with
      `task mutate:check`, and R-M1 and R-M2 to `scripts/test-integration.sh` made in the tree with the `cp` and
      checksum procedure of the reviewer contract, § Required review workflow, item 8, the restored file's checksum
      matching the one taken before. Each report is pasted on this pull request with the system it ran on, beside the
      record of `docs/testing.md`, "What the pull request records". Gate: detection for M1, M2, M3 and M5, and for
      M4, R-M1 and R-M2 on macOS (on Linux, "survivor, equivalent on Linux"); a survivor or an inconclusive run is
      reported as such.
- [x] 2.6 (D) `task verify` passes on the commit that carries 2.2 to 2.4, its integration step admitting the fixture
      tests through the real runner's record on macOS, and that commit's CI run passes on ubuntu-24.04. Gate: CI
      `Required` on that commit, its run link recorded on this pull request.
- [ ] 2.7 (D) T2, T3 and T1's mode check of `design.md`, D7, written first and run on the code of `ecf2033`: T2 fails
      because runner B exits 0 having quarantined runner A's live lock and entered the fake Docker, and T3 because the
      run exits 0; T1's mode check and every other test pass. Gate: that run's output, recorded on this pull request
      with the system it ran on.
      Hold: the owner's acceptance of D7 (task 1.6)
- [ ] 2.8 (D) The runner publishes the owner record whole, as `design.md`, D7 states: written beside the lock
      directory after `mkdir`, its mode set, renamed to `owner`; a record that cannot be published removes the file
      and the lock directory and fails before any Docker call. Gate: `bash -n scripts/test-integration.sh` under
      `/bin/bash` 3.2 and `task test:unit`, T1 to T3 passing.
      Hold: the owner's acceptance of D7 (task 1.6)
- [ ] 2.9 (D) Shown able to fail: R-M3, R-M4 and R-M5 of `design.md`, D7, each made in the tree with the `cp` and
      checksum procedure of the reviewer contract, § Required review workflow, item 8, the restored file's checksum
      matching the one taken before, each report pasted on this pull request with the system it ran on. Gate:
      detection for each, R-M4 where the umask is not `077`; a survivor or an inconclusive run is reported as such.
- [ ] 2.10 (D) `task verify` passes on the commit that carries 2.8, its integration step admitting the fixture tests
      through the published record on macOS, and that commit's CI run passes on ubuntu-24.04. Gate: CI `Required` on
      that commit, its run link recorded on this pull request.
- [ ] 2.11 `design.md`, "Conformance", maps each accepted decision and D7 to the `file:line` that carries it out at
      the commit of 2.10, and to the test or record that shows it. Gate: every row filled, in a commit that changes
      no other file.

## 3. Review

- [ ] 3.1 Codex's implementation review of this pull request, recorded on it, naming the commit it read. Each finding
      is fixed or answered before 4.1.
- [ ] 3.2 The description has one line `implemented-by:` that names `claude` and one line `reviewed-by:` that names
      `codex`, written once Codex's newest review record approves.

## 4. In the archive commit

- [ ] 4.1 (W) In the same commit as `openspec archive lock-identity-locale` and the spec sync:
      `docs/repository-map.md` gains a row for the archived change, as the `admission-owner-identity` row (`:70`)
      does, and its specs row (`:55`) adds that `integration-test-runner` and `nats-fixture` are amended by the
      `lock-identity-locale` archive. Gate: `task spec:check` and `task docs:check` on that commit.
