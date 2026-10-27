# Design: fix-301-graph-ingest-test-seams

- **Base:** main `1106731`; graph-ingest is unchanged since `8927d82`.
- **Ruling:** #301 comment 6453844541. It replaces the design reviewed at `18394ea`. Ruling 1 of comment 6453410228
  stands.
- **Library reads:** nats.go v1.54.0.

## D1. The per-message body is a method

`setupJetStreamConsumer` keeps its setup and its checks (`consume.go:64-150`).

- It builds `inputLane{c, pool, consumer, heartbeat, generation, subject, importLane}`. These are the values the
  closure captures today (`consume.go:150-156`, `:208-220`).
- It hands natsclient `func(_ context.Context, m jetstream.Msg) { lane.deliver(submitCtx, m) }`. `deliver` is
  today's closure body (`consume.go:156-229`).
- `inputLane` holds no context (AGENTS.md § 1; `internal/harness/contract/context_test.go:22`).
- Production is the only caller of `deliver` outside tests.

## D2. Kept tests and how each reaches its case

- **Message-fed tests** (`startStreamLane`, 14 refs) build an `inputLane` and call `deliver`.
- **ack_wait config** (`TestConfigAckWaitAndHeartbeatInterval`) starts graph-ingest on the fixture with the port's
  values and reads AckWait back from the server's consumer info. A refused pair fails Start, and the server holds no
  graph-ingest consumer.
- **The pause.** A test handler set as `deps.Logger` blocks on the Debug record "Entity ingested" (`ingest.go:104`).
  That record is logged after the entity write (`keyed_ingest.go:294`) and before the guard stamp (`:329`) and the
  ack. The component logger is `deps.GetLoggerWithComponent` (`component/dependencies.go:74-75`). The pause is used
  by:
  - readiness: no envelope reads caught-up while the input is held (`readiness_integration_test.go:175`, `:442`);
  - long apply: held past the 2 s ack_wait, which the oracle consumer measures (`long_apply_integration_test.go:138-154`),
    and the input is not redelivered;
  - process killed between apply and ack: the child pauses for good and is killed, and the input is redelivered and
    applied once (`settlement_integration_test.go:246`).
- **Not-started refusal** (rewrites `failed_start_callbacks_integration_test.go:64`). The same handler pauses on
  "query handlers registered" (`query.go:58`) or "mutation handlers registered" (`mutation_runtime.go:33`). A real
  request then gets `errs.ErrNotStarted` (`component.go:530-532`) and does no work. The test then cancels Start's
  context and releases the pause, so Start fails; the rollback leaves nothing held and a following Stop returns nil.
  This keeps the spec's two "during a failed Start" scenarios, reworded from "during a registration that then fails"
  (a natsclient window, proved by `natsclient/subscribe_registration_test.go:392`, `:436`) to "after registration,
  and Start then fails".
- **Straddle (ruling 1).** The test calls `c.lifecycle.Start(ctx, failingStart, c.release)`, as
  `lifecycle_owner_test.go:136-140` does. The failing start builds the pool and the lane. The test then calls
  `lane.deliver` with the submit context the rollback ended. The input's only disposition is Nak
  (`consume.go:225-229`), and nothing panics.
- **Suite adapter.** After a Start that returned nil, `suiteOwner.Start` wraps each handle in `c.consumers` to count
  binds, drains and stops. Only `release` reads that slice (`component.go:575-625`). The lifecycle-suite spec is
  unchanged.

## D3. Deleted tests

The production branches these tests reached stay, failing closed.

| Deleted test | What still covers the behaviour |
|---|---|
| `TestStreamWithoutCreationTimeIsRefused` | `consume.go:92-94` stays, unproven |
| `TestGraphIngest_FailedBootSweepFailsStart` | `poison_inventory.go:311-335` stays |
| `TestIntegration_Settlement_FailedGuardWriteIsRedelivered` | "Durable record fails" stays proved by `keyed_ingest_test.go:432` (nak) |
| `TestGraphIngest_StartFailsWhenItsContextEndsDuringHandlerSetup` | ADR-113 stays proved by `lifecycle_integration_test.go:393` and `:536` |
| `TestGraphIngest_StartReportsItsContextWhenHandlerSetupFails` | ADR-113 stays proved by `lifecycle_integration_test.go:393` and `:536` |
| `TestGraphIngest_FailedStartRollbackOwnsExactConsumerAndPreservesDurable` | not proved after the change (owner-ruled deletion) |

The straddle pair `:210` and `:327` is replaced by the D2 straddle test.

## D4. Fields go; coverage

- **Removed:** `component.go:67-84`, `:526-528`; `consume.go:80-83`, `:152-155`; `poison_inventory.go:307-310`.
- **Proof:** `git grep -n -w -E 'consumeStream|waitForStreamInput|watchEntityStates|subscribeRequests' --
  processor/graph-ingest` prints nothing.
- **Coverage:** `processor/graph-ingest merged` measured 92.4% on the base and 91.8% with the deleted tests skipped,
  against an 80% floor (`scripts/cover-check.sh:31`). Each code task gates on `task cover:check`.
- **Mutants:**
  - M1: `deliver` acks a refused submit. The straddle test must catch it.
  - M2: drop the `startCompleted` check. The not-started test must catch it.
