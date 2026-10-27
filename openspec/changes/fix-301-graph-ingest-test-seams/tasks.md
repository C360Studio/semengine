# Tasks: fix-301-graph-ingest-test-seams

Retires graph-ingest's four test-only NATS fields (#301; owner ruling #301 comment 6453844541).

Working rules for every task:

- Tests come first. Each test's result on the base is recorded in `notes/<task>.md`, linked from a PR comment.
- Developers do not commit; the orchestrator commits each task.
- Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Probe

- [ ] 1.1 The pause (design D2): a `deps.Logger` handler blocking on "Entity ingested" holds one input unacknowledged
  past a 2 s ack_wait, as an oracle consumer measures it. graph-ingest's consumer shows no redelivery while the input
  is held, and the entity is stored while its guard record is not. Run it as a throwaway test, never committed, with
  source and output posted as a PR comment. If it fails, change the mechanism or stop for review; the proof is never
  waived.

## 2. Design review

- [ ] 2.1 Codex design review of `design.md` and the `graph-transport-boundary` delta, with the 1.1 result. Gate:
  `task spec:check` and `task docs:check`.
- [ ] 2.2 Owner acceptance, recorded on PR #436.

## 3. Tests

- [ ] 3.1 Unit tests: `startStreamLane` builds an `inputLane`, and the straddle test is added (D2). On the base these
  fail to compile; record that. Hold: Codex design review (2.1) and owner acceptance (2.2).
- [ ] 3.2 Integration tests:
  - the ack_wait config test read from the server;
  - the pause in the readiness, long-apply and process-killed tests;
  - the not-started refusal test;
  - the suite adapter wrapping `c.consumers`.
  These pass on the base. Gate: `go test -tags integration ./processor/graph-ingest/` and `task verify`.

## 4. Code

- [ ] 4.1 `inputLane.deliver` in `consume.go`, with `setupJetStreamConsumer` passing it to natsclient (D1). Gate:
  `go test -race ./processor/graph-ingest/` and `task verify`.
- [ ] 4.2 Delete the D3 tests and the straddle pair. Remove the four fields and their nil-checks. Run M1 and M2 with
  `task mutate:check` and record the results in `notes/4.2.md`. Gate: the D4 grep prints nothing; `task cover:check`
  and `task verify`.
- [ ] 4.3 Codex code review of record at a named commit with a local `task verify`. The PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`. Hold: Codex code review.

## 5. Archive

- [ ] 5.1 Sync the `graph-transport-boundary` delta and archive, as the last content commit. Gate:
  `task spec:check` and `task docs:check`.
