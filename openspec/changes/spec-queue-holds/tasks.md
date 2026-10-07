# Tasks: spec-queue-holds

Each task names the outcome and the gate that proves it. An unticked task that carries `Hold:` waits for what it
names. Until this change lands, `task spec:queue` shows a hold only on a task's first line, so every hold below is
written there, straight after the task number. "This pull request" is PR #113; evidence is recorded there as a
comment. Every outcome below is reached on the branch, in or before the archive commit. (D) is the developer, (W) the
technical writer.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 113` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS` on the file with sha256 `70882609…bff3`,
      recorded on this pull request. The copy in this change differs only by one repeated word removed at `:183`.
- [x] 1.2 Hold: independent pre-owner design review. The reviewer's verdict on `design.md`, `proposal.md`, this file
      and the `spec-queue` delta is `DESIGN REVIEW PASS`, recorded on this pull request.
- [x] 1.3 Hold: the owner's answers to Q1, Q2 and Q3 on #76 (`design.md`, "Questions for the owner"). They are
      recorded in `design.md` as owner's rulings, with the comment's link. If the owner wants no failing command,
      task 2.3, the check's parts of tasks 2.4-2.6 and 3.1 (`design.md`, Q1, lists them), D9, D10 and the delta's
      third requirement are dropped. If the owner chooses the first-line check, the tasks, requirement and texts of
      `design.md`, "If the owner chooses the first-line check", are added. Neither needs another design round.

## 2. The queue reads whole tasks, and spec:check gains the check

- [ ] 2.1 (D) Written first: `TestSpecQueueHolds` in
      `internal/harness/contract/specqueue_test.go`, with the harness `runSpecQueue` and the cases H1-H11 of
      `design.md`, "Tests" (D6). Run against the unchanged `scripts/openspec-queue.sh`, H1, H2, H3, H5, H9, H10 and
      H11 fail, and H4, H6, H7 and H8 pass. That output is recorded on this pull request.
      Gate: the recorded failing run.
- [ ] 2.2 (D) `TestSpecQueueFirstLineCaveats`: the nine cases of SemStreams `8b99efe9`
      `scripts/openspec-queue_fixture_test.sh:79-113`, with their texts word for word, as the delta's second
      requirement states them. They pass on the unchanged script. Gate: `task test:unit`.
- [ ] 2.3 (D) Written first: `TestSpecQueueCheck` (cases C1-C7) and `TestSpecCheckWiring`, as
      `design.md`, "Tests", says. Run against the unchanged script and `Taskfile.yml`, every case fails. That output
      is recorded on this pull request. Gate: the recorded failing run.
- [ ] 2.4 (D) `scripts/openspec-queue.sh` reads the whole block of each open task and prints holds
      as `design.md`, D1-D5, says, and gains `--check` as D9 says, using only what the script already runs. Its header
      comment says so. `Taskfile.yml`'s `spec:check` runs the check as D10 says. The four tests pass. Committed with
      2.1-2.3, so that no commit is red. Gate: `task verify`.
- [ ] 2.5 (D) Shown able to fail, by hand (`docs/testing.md`, "Run it by hand"): the wrong changes
      of `design.md`, "Tests", each caught by the case named for it. The baseline, wrong-change and restored runs,
      and what is not covered, are recorded on this pull request.
- [ ] 2.6 (D) The new script, run by hand, gives what `design.md`, "On real files", says. The queue
      prints 22 lines for #93's `tasks.md` at `c758aaf5`, at the line numbers of `inventory.md:569-580`; `L62` for
      `b91c60d`; `L258` and `L381` for `a7dbf9d`. `--check` reports nothing on the 15 files of P14. The output is
      recorded on this pull request.

## 3. Documents

- [ ] 3.1 (W) `.agents/protocol.md:24-26`, `AGENTS.md:35` and `:107`, preflight `SKILL.md:31`,
      `docs/testing.md:338-341`, `Taskfile.yml:111` and `scripts/doctor.sh:40-42` read as `design.md`, D8, says.
      Gate: `task docs:check`, and `task doctor` still passes.

## 4. Review

- [ ] 4.1 Codex's implementation review of this pull request, recorded on it, naming the commit it read. Each
      finding is fixed or answered before 5.1.
- [ ] 4.2 The description has one line `implemented-by:` that names `claude` and one line `reviewed-by:` that names
      `codex`, written once Codex's newest review record approves.

## 5. In the archive commit

- [ ] 5.1 (W) The archive command creates `openspec/specs/spec-queue/spec.md`, in the same commit as the spec sync
      and the ticks this commit makes. The command is `openspec archive` with this change's id. The same commit
      writes the spec's Purpose: what the queue reads and prints, that it is advisory, and what `--check` fails on.
      It also gives `docs/repository-map.md` a row for the archived change and lists the `spec-queue` spec as
      present. Gate: `task spec:check` and `task docs:check` on that commit.
