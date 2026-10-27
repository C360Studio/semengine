# Tasks: fix-301-graph-ingest-test-seams

This change retires graph-ingest's four test-only NATS fields (#301). Owner rulings: #301 comment 6453410228.

Working rules for every task:

- Tests come first. Each rewritten test runs on the base before its production change, and its result is recorded in
  `notes/<task>.md`, linked from a PR comment.
- Probes run outside production code: a throwaway test that is never committed. Its source and output are posted as a
  PR comment, with the result in `notes/1.x.md`.
- Developers do not commit; the orchestrator commits each task.
- Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Probes

A probe that fails changes the design's mechanism or stops for review. It never becomes a fallback that waives the
proof.

- [ ] 1.1 P2 (design D2, T6): with a user denied publish on `$JS.API.CONSUMER.CREATE.KV_ENTITY_STATES.>`, Start fails
  at the boot sweep. Record the error, and how long that takes under a 30 s Start context.
- [ ] 1.2 P5 (D4, T11 to T13): a Debug handler that blocks on "Entity ingested" holds the input unacknowledged past a
  2 s ack_wait. The server shows no redelivery while it is held, and the stored entity is written while the guard
  record is not.
- [ ] 1.3 P6 (D4, T14): deleting the applied-seq bucket from that handler makes the guard write fail with the Warn at
  `keyed_ingest.go:330`. After the test recreates the bucket, the redelivery's guard write succeeds through
  graph-ingest's existing handle, and the input is acknowledged.
- [ ] 1.4 :577 (D3, T8): closing the client and cancelling Start on the cue gives a Start error that keeps both
  `context.Canceled` and `ErrConnectionClosed`, nothing Unresolved, and a nil Stop.
- [ ] 1.5 :101 drain read (D3 T9, D5): name the consumer-info read that shows, on the server, whether graph-ingest's
  pull loop on a durable consumer is running. Show it reads "running" while started and "not running" after a
  rollback's drain.

## 2. Design review

- [ ] 2.1 Codex design review of `design.md` and the `lifecycle-suite` delta, with the task 1 results. Gate:
  `task spec:check` and `task docs:check`.
- [ ] 2.2 Owner acceptance of the reviewed design, recorded on PR #436.

## 3. Stream lane (D1)

- [ ] 3.1 Tests T1 to T4 in `stream_lane_test.go`; T1 replaces `bindEntityStream` in `config_fields_test.go`, and
  `startStreamLane` in `statement_metadata_test.go` builds through the constructor. On the base, these fail to compile
  (no constructor); record that. Hold: tasks 2.1 and 2.2.
- [ ] 3.2 `newStreamLane` and `deliver` in `consume.go`, called by `setupJetStreamConsumer`. The fields stay until 5.2.
  Gate: `go test -race ./processor/graph-ingest/`, `task cover:check` and `task verify`.
- [ ] 3.3 Codex review checkpoint of 3.1 and 3.2 at a named commit. Hold: Codex review of 3.1 and 3.2.

## 4. Start path (D2, D3, D5)

- [ ] 4.1 T5 and T6: the boot sweep's exits called directly, and the real denial with its `// Own server:` line.
  Replaces `TestGraphIngest_FailedBootSweepFailsStart`'s use of `watchEntityStates`. Passes on the base. Hold: 3.3.
- [ ] 4.2 T7 to T10 and the server-side suite adapter (D5) in `lifecycle_integration_test.go`,
  `failed_start_callbacks_integration_test.go` (`:64`) and `lifecycle_adapter_test.go`. Passes on the base. Gate:
  `go test -tags integration ./processor/graph-ingest/` and `task verify`.
- [ ] 4.3 Codex review checkpoint of 4.1 and 4.2. Hold: Codex review of 4.1 and 4.2.

## 5. Apply path and removal (D4, D6)

- [ ] 5.1 T11 to T14 in `readiness_integration_test.go`, `long_apply_integration_test.go` and
  `settlement_integration_test.go`. Passes on the base. Hold: 4.3.
- [ ] 5.2 Remove the four fields and their nil-checks (D6). Delete `:210` and `:327` (replaced by T4). Run mutants M1
  to M3 with `task mutate:check` and report each as detected or survived in `notes/5.2.md`. Gate: the D6 grep prints
  nothing; `task cover:check` and `task verify`.
- [ ] 5.3 Codex code review of record at a named commit with a local `task verify`. The PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`. Hold: Codex code review.

## 6. Archive

- [ ] 6.1 Sync the `lifecycle-suite` delta into `openspec/specs/lifecycle-suite/spec.md` and archive, as the last
  content commit. Gate: `task spec:check` and `task docs:check`.
