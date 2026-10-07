# Tasks: spec-queue-unreadable-tasks

Each task names the outcome and the gate that proves it. An unticked task that carries `Hold:` waits for what it
names. "This pull request" is PR #122; evidence is recorded there as a comment. Every outcome below is reached on the
branch, in or before the archive commit. (D) is the developer, (W) the technical writer.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 122` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS`, recorded on this pull request with the
      reviewed file's checksum.
- [x] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the `spec-queue` delta:
      `DESIGN REVIEW PASS`, recorded on this pull request.
- [x] 1.3 The owner's acceptance of the design, recorded on #115 or this pull request, including its departure from
      #115's recommendation 2: one ADDED requirement in place of scenarios added to requirements 1 and 3
      (`design.md`, "Owner decision").

## 2. The tests

- [x] 2.1 (D) `TestSpecQueueHolds` gains `H12 a tasks.md that is not UTF-8`, as its own `t.Run` beside H11, with the
      fixture and assertions of `design.md`, D2, and a comment naming the new requirement. The file comment's "all
      three requirements" becomes "all four". In the same commit, `scripts/openspec-queue.sh:47`, a line of the
      `--help` text, reads `# exits 0. An unreadable change list or tasks.md exits 2, as for the queue.`, still one
      line (`design.md`, D4). Gate: `task test:unit`.
- [x] 2.2 (D) `TestSpecQueueCheck` gains `C8 a tasks.md that is not UTF-8`, as its own `t.Run` after C7, with the
      fixture and assertions of `design.md`, D2, and a comment naming the new requirement. Gate: `task test:unit`.
- [ ] 2.3 (D) Shown able to fail, by hand (`docs/testing.md`, "Run it by hand"): the wrong changes M1a, M1b and M2 of
      `design.md`, D3, run one at a time, each against H12 and C8. For each, the baseline, wrong-change and restored
      runs, the checksums before and after, and the outcome by its name (detection, survivor, invalid or
      inconclusive), with what is not covered, recorded on this pull request. Gate: the recorded runs, with a
      detection wherever D3's table says the case must fail.
- [ ] 2.4 (D) `task verify` passes on the commit that carries 2.1 and 2.2, and that commit's CI run passes. Gate: CI
      `Required` on that commit, its run link recorded on this pull request.

## 3. Review

- [ ] 3.1 Codex's implementation review of this pull request, recorded on it, naming the commit it read. Each finding
      is fixed or answered before 4.1.
- [ ] 3.2 The description has one line `implemented-by:` that names `claude` and one line `reviewed-by:` that names
      `codex`, written once Codex's newest review record approves.

## 4. In the archive commit

- [ ] 4.1 (W) In the same commit as `openspec archive spec-queue-unreadable-tasks` and the spec sync:
      `docs/repository-map.md` gains a row for the archived change, as the `spec-queue-holds` row does, and its
      `openspec/specs/spec-queue/` row (`:60`), "synced by the `spec-queue-holds` archive", adds "amended by the
      `spec-queue-unreadable-tasks` archive", as the row at `:55` does. Gate: `task spec:check` and
      `task docs:check` on that commit.
