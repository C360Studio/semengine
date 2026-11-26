# Tasks: fix-768-graph-index-stop

Each code task ends at its own tests passing; one clean-tree `task verify` runs before the push. One pull request:
`Closes #768`.

## 1. Proposal

- [ ] 1.1 The owner fills `Approved-by:` in `proposal.md`. OPEN-Q: approval of the stopping-flag design (Fable read,
  #768).

## 2. Code and test

- [x] 2.1 A test, shown red on the current code first: graph-index started with a backlog it cannot finish quickly
  (one lane, many queued entities), then `Stop` under a short context returns nil, the queued entities not yet
  started are neither marked failed nor completed, and no `readiness withheld` warning is logged. Record the red run
  in `notes/2.1.md`.
- [ ] 2.2 The stopping flag: set in `cleanup` before `pool.Shutdown`; `processWork` returns first when it is set.
  The write in flight when the flag is set finishes. Accept: the 2.1 test and the graph-index tests pass.
- [ ] 2.3 graph-embedding: read what its queued items record at stop. File an issue if they log false failures or
  complete revisions they did not apply; otherwise say so in `notes/2.3.md`.

## 3. Close

- [ ] 3.1 `task verify` on a clean tree, push, CI green.
- [ ] 3.2 Archive the change as the last content commit. Accept: `task spec:check`.
