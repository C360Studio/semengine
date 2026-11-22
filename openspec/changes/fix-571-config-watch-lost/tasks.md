# Tasks: fix-571-config-watch-lost

Tests come first. Each code task ends at a clean-tree local `task verify`. Single PR, `Closes #571`.

## 1. Proposal

- [x] 1.1 Proposal (`proposal.md`) and spec deltas; `task spec:check` and `task docs:check` pass. Approved 2026-11-22
  (#571).

## 2. Code

- [x] 2.1 `config`: tests that a watch loop whose updates close while the run context is live logs an error naming
  the watch and turns `Health()` unhealthy, and that `Stop` does neither; then both watch loops mark the watch lost,
  and `(*Manager).Health()` reports it.
- [ ] 2.2 `service`: a test that `/health` answers 503 with an unhealthy `config` sub-status for a manager whose
  config watch is lost; then `handleSystemHealth` adds the sub-status.

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive fix-571-config-watch-lost --yes`, which syncs `config-manager` and
  `operator-surface`; `task spec:check` and `task docs:check` pass.
