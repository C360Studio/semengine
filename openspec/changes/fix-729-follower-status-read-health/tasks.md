# Tasks: fix-729-follower-status-read-health

Tests come first. Each code task ends at a clean-tree local `task verify`. Single PR (#731), `Closes #729`.

## 1. Proposal

- [x] 1.1 Proposal (`proposal.md`); `task spec:check` and `task docs:check` pass. Approved 2026-11-23 (#729).

## 2. Code

- [ ] 2.1 Follower unit test: after a failed target read `Stuck` names "ENTITY_STATES status read failed" and the
  feed is not lost; after a successful read on the same follower it is gone. Shown failing first.
- [ ] 2.2 The follower records its latest target read's outcome in `compute`, and `Stuck` names a failed one.
- [ ] 2.3 `TestSmoke_BrokerOutageUnderRunningStack` requires graph-index, graph-lexical and graph-clustering
  unhealthy during the held outage, each naming the failed read, in place of the logged reading.

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive fix-729-follower-status-read-health --yes`, which syncs
  `entity-state-follower`; `task spec:check` and `task docs:check` pass.
