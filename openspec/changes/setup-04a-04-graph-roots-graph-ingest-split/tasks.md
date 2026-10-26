# Tasks: setup-04a-04-graph-roots-graph-ingest-split

Moves only (#300 G and D's seams; owner ruling of 2026-10-26, #300 comment 6416204673). Behaviour is unchanged, so the
oracle is design D3's script against the base `c1caa45` and the existing tests. Each task ends only when
`task verify` passes on a clean tree at its commit (#171). Review stops (Codex) after the design and after task 3.1.

## 1. Design

- [x] 1.1 Inventory and design pass Codex review (`INVENTORY PASS` at `d3c178c`, design pass at `6fd50fe`) and the
  owner accepts the design (2026-10-26): option B, and the non-move edits of the Owner acceptance section. Gate:
  `task spec:check` and `task docs:check`.

## 2. The move

- [ ] 2.1 One commit, `refactor(graph-ingest): split component.go into files, moves only (addresses #300)`: the moves
  of design D1 and the census update of D4 in `natsclient/consumer_policy_callsite_test.go`, nothing else. Its parent is
  the design head. Run D3's script as `bash proof.sh c1caa45 <this commit>`, and record in `notes/2.1.md` the script,
  its output (the pass line), and `git diff -M -C --stat HEAD^ HEAD`. Gate: the script exits 0 and `task verify` passes.

## 3. Citations and ledger

- [ ] 3.1 The three Go comments of design D4 and the ledger destination of D5, in one commit. Gate: `task verify`,
  and `git diff --name-only <move commit> HEAD` lists only those four files and paths under `openspec/`.

## 4. Review and records

- [ ] 4.1 Codex code review of the pull request: its review record approves a named head commit, citing a local
  `task verify` at that commit; the pull request body carries `implemented-by:` and `reviewed-by:`, says "addresses
  #300" and "addresses #109", and has no closing keyword. Gate: the approving review record and the `reviewed-by:`
  line.
- [ ] 4.2 The issue comments of design D4 on #300, #109, #111, #134, #301 and #106, each naming the move commit,
  "pending merge", and the new `file:line` by function. Gate: each comment read back with
  `gh issue view <n> --comments`.

## 5. Archive

- [ ] 5.1 `openspec archive setup-04a-04-graph-roots-graph-ingest-split --yes` as the last content commit; with
  `skip_specs: true` there is no spec to sync. Gate: `task spec:check` and `task docs:check`.
