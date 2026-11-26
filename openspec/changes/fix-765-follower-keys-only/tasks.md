# Tasks: fix-765-follower-keys-only

Each code task ends at its own tests passing; one clean-tree `task verify` runs before the push. One pull request:
`Addresses #765`. Code in `processor/graph-index` starts after #770 merges.

## 1. Proposal

- [ ] 1.1 The owner fills `Approved-by:` in `proposal.md`.

## 2. Code and test

- [ ] 2.1 The keys-only field on `follower.Config`. The follower's integration tests run in both modes and show, at
  the pinned nats.go and nats-server, that with keys-only a delete and a purge still arrive with their operation,
  revisions still ascend, and the end-of-replay marker still arrives. A test asserts `Entry.Value` is nil and
  `Entry.Bytes` is 0 with the field set. Record the runs in `notes/2.1.md`.
- [ ] 2.2 The progress log: while the initial build runs, the follower logs at Info the entries delivered and the
  watermark's observed and indexed revisions, on a cadence it already has. Test: a replay logs at least one progress
  line before the build completes, and none after it.
- [ ] 2.3 graph-index sets the field and counts its input bytes from its own entity read. Test: after a replay, the
  input-bytes count equals the stored values' total length. Accept: the graph-index tests pass.

## 3. Close

- [ ] 3.1 `task verify` on a clean tree, push, CI green.
- [ ] 3.2 Archive the change as the last content commit. Accept: `task spec:check`.
