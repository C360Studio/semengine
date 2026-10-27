# Design: fix-301-graph-ingest-test-seams

- **Base:** `1106731` (main with the claim; graph-ingest is unchanged since `8927d82`). Every `path:line` is at the
  base.
- **Inventory:** `inventory.md` (INVENTORY PASS, round 2).
- **Rulings:** #301 comment 6453410228. The drop-or-waive precedent is the #192 ruling, comment 6127721456.
- **Library reads:** nats.go v1.54.0 and nats-server 2.15.x. Each read is cited. Each statement not yet run is tied
  to a probe task (tasks 1.1 to 1.5).
- **ADRs applied:** ADR-113 (Start's context), ADR-112 (natsclient adds nothing), ADR-108 (the processor shell).

## D1. Production builds the stream lane; tests build it the same way

- `setupJetStreamConsumer` waits for the stream (`consume.go:89`).
- It then calls `c.newStreamLane(submitCtx, port, created)`. This is an unexported constructor in `consume.go`. It
  holds everything that `consume.go:65-145` computes today:
  - port facts, subject, import lane and consumer config;
  - the ack_wait and heartbeat check;
  - the zero-creation-time refusal (`consume.go:92-94`);
  - the pool and the submit context.
- It passes `lane.deliver`, the body of today's closure (`consume.go:156-229`), to
  `c.natsClient.ConsumeStreamWithConfig`.
- Production is the constructor's only non-test caller. Tests call the same constructor, so they cannot build a lane
  that production would refuse.

| Test | Behaviour it proves |
|---|---|
| T1 (replaces `bindEntityStream`) | A port's ack_wait and heartbeat_interval reach `lane`'s consumer config with the defaults; a refused pair fails the constructor |
| T2 (replaces `TestStreamWithoutCreationTimeIsRefused`) | A zero creation time is refused |
| T3 | `startStreamLane` (14 callers) builds through the constructor; its callers do not change |
| T4 (ruling 1; replaces `:210` and `:327`) | See below |

T4 builds a lane on a component with a built pool, then runs `c.release` (`component.go:562`), the cleanup the shell
runs for a failed Start's rollback. It then calls `lane.deliver` with a valid Graphable input. The input's only
disposition is Nak (`consume.go:225-229`), and nothing panics after the cleared handles (`component.go:619`).

## D2. The boot sweep is proved by calling it

`startEntityStateGuard` already takes its bucket as an argument (`poison_inventory.go:306`), and `KVStore.Watch`
passes straight through (`natsclient/kv.go:729`).

- **T5** calls the sweep with `client.NewKVStore(wrapper)` for each of its three exits: a refused watch, updates
  closed before the end-of-snapshot marker, and a context that ends mid-sweep. Closed updates have a real trigger:
  nats.go closes them when the subscription closes (kv.go:1364-1368). Each exit returns a transient error, stops the
  watcher, and records no poison.
- **T6** proves "a failed sweep fails Start and the rollback leaves nothing" with a real denial.
  - Setup: an embedded server denies one user publish on `$JS.API.CONSUMER.CREATE.KV_ENTITY_STATES.>`. The sweep's
    ordered consumer is created there (kv.go:1355-1360, js.go:1954-1959, jsm.go:511-524). The precedent for the
    embedded server is `config/manager_lifecycle_integration_test.go:368-376`, with an `// Own server:` line.
  - Expected: Start fails, nothing is Unresolved, no query subject has a responder, Stop returns nil, and a second
    Start is refused.
  - Task 1.1 measures what the timeout costs.

## D3. Start-path faults are cued on log records

The component logger is `deps.GetLoggerWithComponent` (`component.go:325`, `component/dependencies.go:74-75`), so a
test handler that enables Debug sees Debug records. The precedent is M7's `cancelOnRecordHandler`
(`lifecycle_integration_test.go:508-540`).

- **T7 (`:448`, ADR-113).** Start's context is cancelled on one of two records: "graph-ingest subscribed (JetStream)"
  (Debug, `consume.go:244`) or "query handlers registered" (`query.go:58`). Expected: Start wraps `context.Canceled`,
  nothing is Unresolved, no graph-ingest request subject has a responder, and Stop returns nil. The test no longer
  says during which registration the context ended; the cue is between registrations.
- **T8 (`:577`).** The same cue also closes the client. Expected: Start keeps both `context.Canceled` and the
  registration's `ErrConnectionClosed` (`natsclient/client.go:1088-1091`), and T7's checks still hold (task 1.4).
- **T9 (`:101`).** The test uses two input ports. A handler blocks on "Setting up JetStream consumer" (Debug,
  `consume.go:104`) for the second stream. Expected:
  - Stop does not return while Start is held.
  - The cancelled Start fails.
  - The first durable consumer stays on the server (`:177-180`).
  - The server shows no pull loop on it (task 1.5).
- **T10 (replaces `:64`).** Start is held at "query handlers registered" or at "mutation handlers registered"
  (`mutation_runtime.go:33`). A real request is refused with `errs.ErrNotStarted` (`component.go:530-532`) and does no
  work. The window where a handler runs and then its registration fails stays proved in natsclient
  (`natsclient/subscribe_registration_test.go:320`, `:366`).

## D4. Apply-path holds are cued at "Entity ingested"

"Entity ingested" (Debug, `ingest.go:104`) is logged on the lane goroutine after the entity write
(`keyed_ingest.go:294`) and before the guard stamp (`:329`) and the ack. Tasks 1.2 and 1.3 run these cues before any
test relies on them.

- **T11 (`readiness_integration_test.go:175`, `:442`).** The first input is held there. Expected: no envelope reads
  caught-up, and bootstrap_complete is true.
- **T12 (`long_apply_integration_test.go:101`, "Long apply").** The hold outlasts the 2 s ack_wait. The server's
  consumer shows no redelivery past the wait. The InProgress count is dropped: it measured mechanism, not the
  scenario.
- **T13 (`settlement_integration_test.go:246`).** The child process blocks there for good and is killed. The input
  is redelivered and applied once.
- **T14 (`:329`, "Durable record fails").**
  1. The handler deletes the applied-seq bucket through a test client.
  2. The guard write fails, and the Warn record "redelivery-guard durable stamp failed" (`keyed_ingest.go:330`) is
     the cue.
  3. The test recreates the bucket.
  4. The redelivery is acknowledged, and the graph is as one application left it.

## D5. The suite adapter shows cleanup on the server (ruling 2)

`newSuiteOwner` (`lifecycle_adapter_test.go:34`) drops its `consumeStream` wrap and its counted calls. Observe adds two
server-derived entries:

- "request responder", when a request on a graph-ingest subject finds one;
- "input consumer pulling", when the server shows a pull loop (task 1.5).

graph-ingest's lifecycle tests then publish an input after Stop and assert it stays pending.

Callers covered:

- `lifecycle_integration_test.go:332`, `:406`, `:467`, `:553`, `:597`, `:854`;
- `health_integration_test.go:198`;
- `lifecycle_owner_test.go:132`.

`lifecycletest.go:317` (no call after a failed Start's Stop) now compares empty maps for graph-ingest. The
server-derived Unresolved entries are what prove that the rollback released everything.

## D6. The fields go

Removed: `component.go:67-84`, `consume.go:80-83`, `:152-155`, `poison_inventory.go:307-310` and
`component.go:526-528`. The proof is that
`git grep -n -w -E 'consumeStream|waitForStreamInput|watchEntityStates|subscribeRequests' -- processor/graph-ingest`
prints nothing.

## Coverage and mutants

Every production branch a removed test reached keeps a test:

| Branch | Test |
|---|---|
| Zero creation time | T2 |
| The three sweep exits | T5 |
| Nak after release | T4 |
| Not-started refusal | T10 |

Each code task gates on `task cover:check`, which holds `processor/graph-ingest merged` at 80%
(`scripts/cover-check.sh:31`), and records the percentage on the base and on its commit.

Mutants:

| Mutant | Must be caught by |
|---|---|
| M1: the constructor accepts a zero time | T2 |
| M2: `deliver` acks a refused submit | T4 |
| M3: the sweep returns nil on closed updates | T5 |
