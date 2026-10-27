# Design: fix-216-consumer-rebind

Base `6d500e0` (main `ef037ea` plus the claim and inventory commits; no code change). Inventory: `inventory.md`
(INVENTORY PASS). Owner ruling: #216 comment 6443457897, "Yes, restart is the recovery (small fix)" (the inventory's
option O1). Retires #216 (`Closes #216`). Every `path:line` is at the base; probe cases a to f are the inventory's
§ 1.2, run at nats.go v1.54.0 and nats-server v2.15.1 (`/tmp` source and output are posted on PR #431, probe comment
6443330920).

## Context

- graph-ingest binds one durable consumer per JetStream input port in `Start`, after reading the stream's creation
  time as its generation (`processor/graph-ingest/consume.go:89`, `:96`), and keeps the native handle
  (`consume.go:231`), as ADR-095 requires (`docs/adr/095-...md:51`).
- Its readiness tick (every 5 s, `readiness.go:19`) looks each bound consumer up by name
  (`readiness.go:204-226`). A failed lookup is a stuck state that clears on the next good tick (`readiness.go:142-149`,
  `component.go:392-401`).
- What the loop does when the binding goes (inventory § 2.3): a, memory stream lost at a broker restart: the loop
  runs on and `Closed()` never fires; the lookup fails until someone creates the stream and consumer again. d, stream
  deleted in place: the loop ends (`jetstream/pull.go:740-741`, `:273-281`) and the lookup fails. e and f, a
  consumer created again after the loop ended: the lookup succeeds and `Health()` reads healthy, while nothing is
  delivered. a then b: if the consumer is created again after a broker restart, the old loop resumes about 30 s later
  (not promised) with the old generation. c, file stream: nothing is lost; the stream keeps its creation time
  (probe c; `guard_generation_integration_test.go:157`).

## D1. A loop that ends without Stop is stuck until the process restarts

Observable behaviour. On each readiness tick, for each bound consumer, graph-ingest records a **lost input binding**
when the consume loop it started for that consumer has ended before graph-ingest's own `Stop` began. A loop that ends
after `Stop` began is not recorded. Once recorded it is never
cleared: `Health()` reads `Healthy: false`, `Status: "degraded"`, and `LastError` names the stream and consumer and
says the process must be restarted, on every call until the component stops. The readiness envelope for that tick and
every later one is degraded, as for a failed observation (`readiness.go:132`; one compute for both channels,
`:93-101`). The moment it is recorded, graph-ingest logs one Error line naming the stream and consumer. A failed
lookup keeps today's behaviour: stuck for that tick only, clear on the next good one. Nothing is rebound, stopped,
drained or deleted; the old loop stays as it is until `Stop` drains it (`component.go:574-587`).

How each case ends:

- a, memory stream lost at a broker restart: the loop does not end (probe a: `closed=false` after 77 s), so nothing is
  recorded. The existing lookup fails, first "stream not found", then "consumer not found" once the producer creates
  the stream again, so `Health()` is false until the process restarts, for as long as nobody creates the consumer
  again. If someone does ("a then b"), the old loop resumes with the old generation and `Health()` reads healthy: that
  is the declared limitation "Stream and consumer created again while disconnected" (`spec.md:180-185`, owner ruling
  #91 comment 6113959050), unchanged.
- d, stream deleted in place: the loop ends (probe d) and is recorded; e, a consumer created again afterwards, and f,
  only the consumer deleted and created again: the loop has ended (probe e, f), so the record holds and `Health()`
  stays false. These are what change.

Why a latch. A consume loop that has ended does not start again (probe e, f), so clearing the state would report a
binding healthy that only a restart repairs (the ruling). A restart reads the generation again.

Constraints the developer keeps:

- The tick reads the loop's end through the handle's `Closed()` channel, without blocking. What the readiness records
  hold is that receive-only channel, never the handle: observation stays read-only and separate from lifecycle
  authority (ADR-095 `:66`; `readiness.go:200-203`). `Stop` keeps the exact handle and drains it (ADR-094
  `docs/adr/094-...md:128-131`); no `ConsumeContext.Stop` is added.
- No new goroutine: the check runs inside the existing tick, under the run context `Start` creates (ADR-113
  `docs/adr/113-...md:62-64`). No error text is inspected.
- The fact "`Stop` has begun" that the tick reads is published so that a tick running concurrently with `Stop` reads
  it without a data race, and it is not a field on the handle-bearing consumer binding (`component.go:215-217`), so
  the tick never reaches lifecycle state. `cleanup`'s order is not changed: Drain and closure come before Start-owned
  work is cancelled (ADR-094 `:128-131`), so ticks still run while `Stop` drains.
- No natsclient change and no new field on `Component` reachable only from tests (#170; #301).
- Library facts this rests on: the loop ends after a terminal status and `Closed()` fires (probe d, f; `pull.go:740`,
  `:273-281`, `:306-317`); in case a it does not (probe a). `Closed()` can also fire early for a subscription that is
  no longer valid (#83): after a stop, a forced close, or when nats.go closes the connection by itself once a finite
  `WithMaxReconnects` (`natsclient/options.go:15`) runs out. In each, unhealthy is the true reading; when nats.go does
  this is not promised. Timings (tick cadence, nats.go's 30 s re-pull) are not promised.

The window case, a stream recreated between the generation read and the bind (`spec.md:165-173`), is not touched: its
consumer is the new stream's and its loop does not end.

## D2. Text

- `openspec/specs/graph-ingest-recovery/spec.md:21-25`: the memory-restart scenario says graph-ingest reads unhealthy
  until its process is restarted (while nobody creates its consumer again), and publishing again after the restart
  applies the message (spec delta).
- `docs/contract.md:79`: the memory row's "Your part" becomes: restart the process when `/readyz` fails (`/healthz`
  only says the listener answers, `service/doc.go:177`), then re-publish from your source.
- `processor/graph-ingest/README.md:188`: a loop that ends reads unhealthy until restart; nothing binds a new one.
- `docs/admission-ledger.yaml`, graph-ingest row (`:2381-2386`): one `known_risks` clause (an ended loop latches
  `Health()` until restart; "a then b" stays the declared limitation) and the new test files in `proving_tests`.

## Tests

All in `processor/graph-ingest`, integration tag, waiting through `awaitHealthAfterTick`
(`health_integration_test.go:22-31`), which runs its own tick per observation. "Still unhealthy" means a fresh tick
run after the step, not elapsed time.

| Test | Case | Asserts | On the base |
| --- | --- | --- | --- |
| T1 `TestIntegration_MemoryStreamLostAtBrokerRestartStaysUnhealthy` (own server: restarts it) | a | memory stream; graph-ingest stays running across `Restart`; the stream is created again: unhealthy, `LastError` names the backlog observation, on a fresh tick and on one after a message is published | passes: pins today's lookup path for the corrected scenario |
| T2 `TestIntegration_LoopEndedByStreamRecreatedStaysUnhealthy` | d, e | stream deleted and created again: unhealthy, `LastError` names the restart, the envelope from `computeBacklogStatus` is degraded; consumer created again: still unhealthy | fails at the last step |
| T3 `TestIntegration_HealthStaysStuckAfterConsumerRecreated` (replaces `..._ConsumptionNotChecked`, `health_integration_test.go:80`) | f | consumer deleted: unhealthy; created again: still unhealthy | fails at the last step |
| T4 `TestIntegration_FileStreamRestartStaysHealthy` (own server) | c | file stream; across `Restart`: healthy on a fresh tick, and a message published after the restart is applied | passes: pins kept behaviour |

T1 and T4 keep graph-ingest's client connected across `Restart`, which `Restart`'s doc says a caller does not do
(`internal/harness/natsfixture/restart.go:24-29`: handles are dead, URL has stayed the same but Docker does not promise
it). The departure is the point of both tests: graph-ingest's own client reconnects as a production client does, and
`Restart` drains only the fixture's connection. Each asserts `f.URL()` is unchanged across `Restart` with
`require.Equal`, so a remapped port fails the test loudly instead of passing it for the wrong reason. "Publishing again
after a restart applies it" is already proved with a new instance by
`TestIntegration_IngestGuard_StreamRecreatedWithLowerSequences` (`guard_generation_integration_test.go:71`).

`TestIntegration_HealthReadsLostConsumer` (`:57`) stays unchanged and must still pass. Examples suffice; the inputs
do not interact beyond these cases (`docs/testing.md`).

Unproven, stated on the PR and in `notes/3.2.md`:

- "The observation succeeds again" (modified health requirement) loses its only test, which T3 replaces. Every
  in-test way to fail an observation (deleting the consumer or the stream) now also ends the loop, and a tick cannot
  be ordered inside `Restart`, so no test fails an observation while the binding is intact.
- "Stop ends the loop": `cleanup` drains and waits on `Closed()` (`component.go:574-587`) before it ends the run
  context and waits for the status loop (`:602-610`), so a tick can land after the drain, and no existing surface
  orders one there. No test is written for it.

Mutation rows (`task mutate:check`, integration tests by `-run`, recorded in `notes/3.2.md`):

| Mutant | Expected |
| --- | --- |
| M1 the ended-loop check removed | detected by T2, T3 |
| M3 the record cleared by a good observation | detected by T2, T3 |
| M4 the record made for a loop that ended after `Stop` began | survivor expected (no ordering, above); listed as uncovered |
| M6 a failed lookup not stored as the fault (`readiness.go:144-148`) | detected by T1 |
| M5 every failed lookup recorded as lost | survivor expected, unless a tick happens to fail during T4's restart; listed as uncovered |

## Dispositions

- #216: retired (`Closes #216`). The healthy-but-not-consuming case (e, f) is part of it and is not filed apart.
- #15: stays. Its replay-protection work is wider; "a then b" stays the declared limitation (D1).
- #301: stays. No test-only field is added; T1 to T4 use real streams.
- #83: stays. The early `Closed()` it tracks fires after a stop, a forced close, or nats.go closing the connection
  once finite reconnects run out (not promised); in each, unhealthy is the true reading (D1).

## Merge order

Open PR #414 shares no file (inventory § 3.1). If it merges first, this branch merges `origin/main`; nothing in
`graph/readiness` is read differently.
