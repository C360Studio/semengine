# Tasks: adopt-ateam-spec-queue

Track: `design` (adds spec requirements). Owner ruling on #447 (2026-10-28): proposal approval and the design track
are adopted. A hold is written as `Hold:` inside the unticked task it stops.

## 1. Proposal and design

- [x] 1.1 Proposal, design, tasks and the `spec-queue` delta committed; draft PR opened with `Closes #450` and
  `Addresses #446`. Gate: `task spec:check` and `task docs:check`.
  Done in b070579; draft PR #472. Owner approved the proposal as proposed (D1 validate line kept, D2 python3 row kept)
  in session on 2026-10-29 (`Approved-by:` filled, recorded on #450).

## 2. Task wiring, script, test

- [ ] 2.1 `Taskfile.yml`: `spec:queue` runs `ateam state spec-queue`; `spec:check` runs
  `npx --no-install openspec validate --all --strict --no-interactive` then `ateam state spec-check` (design D1).
  Gate: `task spec:check` prints `holds: ok (1 tasks.md read)` and exits 0; `task spec:queue` lists this change
  with its `OPEN-Q` line; with `node_modules` renamed aside, `task spec:check` exits non-zero on the first line.
- [ ] 2.2 `scripts/openspec-queue.sh` and `internal/harness/toolcheck/specqueue_test.go` deleted; `lines` moved to
  the new test file; `scripts/doctor.sh` python3 comment names `scripts/port-status.sh` (design D2). Gate:
  `task test:tools` passes; `scripts/doctor.sh` prints the python3 row; `git grep openspec-queue` outside
  `openspec/changes/archive` finds nothing.
- [ ] 2.3 `TestSpecTaskWiring` in `internal/harness/toolcheck/specwiring_test.go` (design D3) with its sensitivity
  cases. Gate: `go test ./internal/harness/toolcheck -run TestSpecTaskWiring -count=1`.

## 3. Docs

- [ ] 3.1 `docs/testing.md`, the `.agents/rules-index.md` row and `internal/harness/toolcheck/doc.go` name the
  new test; `AGENTS.md` unchanged. Gate: `task docs:check`, `task ateam:doctor`.

## 4. Review and archive

- [ ] 4.1 In-house review, then the cross-agent record on the PR in ateam's grammar; `implemented-by:` and
  `reviewed-by:` with `(model, effort)` in the PR body.
- [ ] 4.2 Archive as the last content commit, syncing the `spec-queue` delta; the merged spec's Purpose paragraph
  names the ateam commands. Gate: `task spec:check` and `task docs:check`.
