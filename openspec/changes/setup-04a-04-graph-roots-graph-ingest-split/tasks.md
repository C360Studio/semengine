# Tasks: setup-04a-04-graph-roots-graph-ingest-split

Moves only (#300 G and D's seams; owner ruling of 2026-10-26, #300 comment 6416204673). Behaviour is unchanged, so the
oracle is design D3's two checks against the base `c1caa45` and the existing tests. Each task ends only when
`task verify` passes on a clean tree at its commit (#171). Review stops (Codex) after the design and after task 3.1.

## 1. Design

- [ ] 1.1 Inventory and design pass Codex review (`INVENTORY PASS`, then design review) and the owner accepts the
  design, with option B or C of its Options section. Gate: `task spec:check` and `task docs:check`.

## 2. The move

- [ ] 2.1 One commit, `refactor(graph-ingest): split component.go into files, moves only (addresses #300)`: the
  moves of design D1 and the census update of D4 in `natsclient/consumer_policy_callsite_test.go`, nothing else. Run
  D3's P1 and P2 against `c1caa45` and record in `notes/2.1.md` the commands, their empty output, the two mutant runs
  that show each check can fail, and `git diff -M -C --stat c1caa45 HEAD`. Gate: `task verify`.

## 3. Citations and ledger

- [ ] 3.1 The three Go comments of design D4 and the ledger destination of D5, in one commit. Gate: `task verify`,
  and D3 rerun at this commit prints nothing.

## 4. Review and records

- [ ] 4.1 Codex code review of the pull request at its head, citing the commit and a local `task verify`;
  `implemented-by:` and `reviewed-by:` in the pull request body, which says "addresses #300" and "addresses #109"
  and carries no closing keyword. Gate: `task merge:check -- <n>`, then CI green once before merge.
- [ ] 4.2 The issue comments of design D4 on #300, #109, #111, #134, #301 and #106, each naming the move commit,
  "pending merge", and the new `file:line` by function. Gate: each comment read back with
  `gh issue view <n> --comments`.

## 5. Archive

- [ ] 5.1 `openspec archive setup-04a-04-graph-roots-graph-ingest-split --yes` as the last content commit; with
  `skip_specs: true` there is no spec to sync. Gate: `task spec:check` and `task docs:check`.
