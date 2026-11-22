# Tasks: fix-564-embedding-clean-stop-warning

Tests come first. Each code task ends at a clean-tree local `task verify`. Single PR, `Closes #564`.

## 1. Proposal

- [ ] 1.1 Proposal (`proposal.md`); `task spec:check` and `task docs:check` pass.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 A test in `internal/dispatch` that `Shutdown` under an already-ended context, then `Wait`, logs no
  warning, and that a context ending while `Shutdown` waits on a busy lane logs it; then `Shutdown` warns only in the
  second case, and its comment says so. Every existing `internal/dispatch` test passes unchanged.

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive fix-564-embedding-clean-stop-warning --yes`; `task spec:check` and
  `task docs:check` pass.
