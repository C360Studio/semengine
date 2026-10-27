# Tasks: fix-301-graph-ingest-test-seams

Retires graph-ingest's four test-only NATS fields (#301; owner ruling #301 comment 6453844541).

Working rules for every task:

- Tests come first. Each test's result on the base is recorded in `notes/<task>.md`, linked from a PR comment.
- Developers do not commit; the orchestrator commits each task.
- Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Probe

- [x] 1.1 The pause (design D2): a `deps.Logger` handler blocking on "Entity ingested" holds one input unacknowledged
  past a 2 s ack_wait, as an oracle consumer measures it. graph-ingest's consumer shows no redelivery while the input
  is held, and the entity is stored while its guard record is not. Run it as a throwaway test, never committed, with
  source and output posted as a PR comment. If it fails, change the mechanism or stop for review; the proof is never
  waived.

## 2. Design review

- [x] 2.1 Codex design review of `design.md` and the `graph-transport-boundary` delta, with the 1.1 result. Gate:
  `task spec:check` and `task docs:check`. Codex: DESIGN REVIEW PASS at `5e4fb2f` (PR #436 comment 6455095656); its
  one MEDIUM, the task order, is fixed in tasks 3 and 4 below.
- [x] 2.2 Owner acceptance, recorded on PR #436 (2026-10-27).

## 3. Tests that pass on the base

- [ ] 3.1 Integration tests:
  - the ack_wait config test read from the server;
  - the pause in the readiness, long-apply and process-killed tests;
  - the not-started refusal test, whose doc comment (`failed_start_callbacks_integration_test.go:9`) names the
    requirement "graph-ingest refuses request work until Start completes";
  - the suite adapter wrapping `c.consumers`.
  These pass on the base. Gate: `go test -tags integration ./processor/graph-ingest/` and `task verify`. Hold: owner
  acceptance (2.2).

## 4. Code

- [ ] 4.1 Unit tests and `inputLane` in one commit (D1, D2): `startStreamLane` builds an `inputLane`, the straddle test
  is added, and `consume.go` gains `inputLane.deliver`, which `setupJetStreamConsumer` passes to natsclient. The tests
  cannot compile without the type, so their failure on the base is shown with `go test -overlay` (tests only) and
  recorded in `notes/4.1.md`. Gate: `go test -race ./processor/graph-ingest/` and `task verify`.
- [ ] 4.2 Delete the D3 tests and the straddle pair. Remove the four fields and their nil-checks. Run M1 and M2 with
  `task mutate:check` and record the results in `notes/4.2.md`. Gate: the D4 grep prints nothing; `task cover:check`
  and `task verify`.
- [ ] 4.3 Codex code review of record at a named commit with a local `task verify`. The PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`. Hold: Codex code review.

## 5. Archive

- [ ] 5.1 Sync the `graph-transport-boundary` delta and archive, as the last content commit. Gate:
  `task spec:check` and `task docs:check`.
