# Tasks: admission-owner-identity

Each task names the outcome and the gate that proves it. An unticked task that carries `Hold:` waits for what it
names. "This pull request" is PR #125; evidence is recorded there as a comment. Every outcome below is reached on the
branch, in or before the archive commit. (D) is the developer, (W) the technical writer.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 125` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS`, recorded on this pull request with the
      reviewed file's checksum.
- [x] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the two spec deltas:
      `DESIGN REVIEW PASS`, recorded on this pull request.
- [x] 1.3 The owner's acceptance of the design, recorded on #124 or this pull request, with an answer to each item of
      `design.md`, "Owner decisions": the refusal of an `unknown` identity, the definition's home, the context's
      error, runner finding 1 accepted and finding 2 tracked in #126, and doctor's comment-only correction.

## 2. The fix and its tests

- [x] 2.1 (D) In `internal/harness/natsfixture/admission_test.go`, as `design.md`, D4 states: `plantLock` records
      the identity its oracle reads, and `TestAdmissionRequiresALiveOwner` gains an identity per case and the cases
      "owner pid names a process with another start time" and "owner identity unknown". Run on the base code, the
      two new cases fail because `Start` is admitted, and the rest pass. Gate: that run, recorded on this pull
      request.
- [ ] 2.2 (D) `admission.go` and `fixture.go:129` as `design.md`, D2 and D3 state: the read under `Start`'s context,
      the comparison, refusals that name their reason, the context's error after a failed read, no `kill(pid, 0)`.
      In the same commit, `TestAdmissionAdmitsALiveOwner` with its cancelled-context case, and the comments at
      `admission.go:24-26`, `:71-74` and `admission_test.go:117-120` rewritten to the new rule. Gate:
      `task test:unit`.
- [ ] 2.3 (D) `scripts/doctor.sh:107`'s comment says what doctor checks, as `design.md`, D1 states: host and pid
      only, so a pid reused by another process reads as busy there, though the runner would quarantine it. No other
      line of the script changes. Gate: `bash -n scripts/doctor.sh`, and the commit's diff of that file being that
      comment only, recorded on this pull request.
- [ ] 2.4 (D) Shown able to fail with `task mutate:check`: the wrong changes M1 to M4 of `design.md`, D4, one at a
      time, each report pasted on this pull request with the system it ran on, beside the record of
      `docs/testing.md`, "What the pull request records". Gate: detection for M1, M2 and M4, and for M3 on macOS (on
      Linux, "survivor, equivalent on Linux"); a survivor or an inconclusive run is reported as such.
- [ ] 2.5 (D) `task verify` passes on the commit that carries 2.1 to 2.3, its integration step admitting the
      fixture tests through the real runner's record on macOS, and that commit's CI run passes on ubuntu-24.04.
      Gate: CI `Required` on that commit, its run link recorded on this pull request.

## 3. Review

- [ ] 3.1 Codex's implementation review of this pull request, recorded on it, naming the commit it read. Each finding
      is fixed or answered before 4.1.
- [ ] 3.2 The description has one line `implemented-by:` that names `claude` and one line `reviewed-by:` that names
      `codex`, written once Codex's newest review record approves.

## 4. In the archive commit

- [ ] 4.1 (W) In the same commit as `openspec archive admission-owner-identity` and the spec sync:
      `docs/repository-map.md` gains a row for the archived change, as the `spec-queue-unreadable-tasks` row (`:69`)
      does, and its specs row (`:55`) adds that `integration-test-runner` and `nats-fixture` are amended by the
      `admission-owner-identity` archive. Gate: `task spec:check` and `task docs:check` on that commit.
