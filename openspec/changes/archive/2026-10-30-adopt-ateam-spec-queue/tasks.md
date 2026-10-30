# Tasks: adopt-ateam-spec-queue

Track: `design` (adds spec requirements). Owner ruling on #447 (2026-10-28): proposal approval and the design track
are adopted. A hold is written as `Hold:` inside the unticked task it stops.

## 1. Proposal and design

- [x] 1.1 Proposal, design, tasks and the `spec-queue` delta committed; draft PR opened with `Closes #450` and
  `Addresses #446`. Gate: `task spec:check` and `task docs:check`.
  Done in b070579; draft PR #472. Owner approved the proposal as proposed (D1 validate line kept, D2 python3 row kept)
  in session on 2026-10-29 (`Approved-by:` filled, recorded on #450).

## 2. Task wiring, script, test

- [x] 2.1 `Taskfile.yml`: `spec:queue` runs `ateam state spec-queue`; `spec:check` runs
  `npx --no-install openspec validate --all --strict --no-interactive` then `ateam state spec-check` (design D1).
  Gate: `task spec:check` prints `holds: ok (1 tasks.md read)` and exits 0; `task spec:queue` lists this change
  with its `OPEN-Q` line; where `npx` reaches no `openspec` at all, `task spec:check` exits non-zero on the first
  line.
  Done in 6778e4e: `holds: ok (1 tasks.md read)`, exit 0; the queue prints `1/7 ok` (the hold went in 67e990c), and
  on b070579's `tasks.md` `OPEN-Q   L10    Hold: OPEN-Q proposal awaits owner approval.`; the first line stops only
  with no openspec reachable anywhere (`exit status 1`, see `notes/2.1.md`).
- [x] 2.2 `scripts/openspec-queue.sh` and `internal/harness/toolcheck/specqueue_test.go` deleted (its `lines` helper
  had no other caller and goes with it); `scripts/doctor.sh` python3 comment names `scripts/port-status.sh` (design
  D2). Gate:
  `task test:tools` passes; `scripts/doctor.sh` prints the python3 row; `git grep openspec-queue` outside
  `openspec/` finds only the planted wrong command in the 2.3 test.
  Done in 6778e4e: `task test:tools` `ok` for mutcheck, pindiff, toolcheck; doctor `ok    python3       3.14.5`;
  `git grep` finds only this change's files, the `spec-queue` spec (rewritten in 4.2) and the 2.3 fixture.
- [x] 2.3 `TestSpecTaskWiring` in `internal/harness/toolcheck/specwiring_test.go` (design D3) with its sensitivity
  cases. Gate: `go test ./internal/harness/toolcheck -run TestSpecTaskWiring -count=1`.
  Done in 6778e4e: `ok  github.com/c360studio/semengine/internal/harness/toolcheck 0.250s`; choices in `notes/2.3.md`.

## 3. Docs

- [x] 3.1 `docs/testing.md`, the `.agents/rules-index.md` row and `internal/harness/toolcheck/doc.go` name the
  new test; `AGENTS.md` unchanged. Gate: `task docs:check`, `task ateam:doctor`.
  Done in eea53f3: `Summary: 0 issues in 0 files`; `ateam:doctor` `"violations": null`, exit 0.

## 4. Review and archive

- [x] 4.1 In-house review, then the cross-agent record on the PR in ateam's grammar; `implemented-by:` and
  `reviewed-by:` with `(model, effort)` in the PR body.
  Done: in-house `semengine-reviewer` APPROVE at 1d6cee2 (three wording findings, fixed in df4aef7); Codex record
  PR #472 comment 6519476407, `Reviewed at df4aef7`, `Verdict: APPROVE`, `reviewed-by: codex (gpt-6-astra, high)`;
  its one MEDIUM (`docs/repository-map.md` toolcheck row still named the queue script) is fixed in the archive commit.
- [x] 4.2 Archive as the last content commit, syncing the `spec-queue` delta; the merged spec's Purpose paragraph
  names the ateam commands. Gate: `task spec:check` and `task docs:check`.
  Done in the archive commit; both gates green.
