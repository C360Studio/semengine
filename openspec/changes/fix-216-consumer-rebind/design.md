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

## D1. A lost input binding is stuck until the process restarts

Observable behaviour. On each readiness tick, for each bound consumer, graph-ingest records a **lost input binding**
when either holds:

1. the consume loop it started for that consumer has ended, and graph-ingest's own `Stop` did not end it (d, e, f);
2. the tick reads the input stream and its creation time is not the generation read at bind (a after the producer
   creates the stream again, a then b, d, e).

Once recorded it is never cleared: `Health()` reads `Healthy: false`, `Status: "degraded"`, and `LastError` names the
stream and consumer and says the process must be restarted, on every call until the component stops. The moment it is
recorded, graph-ingest logs one Error line naming the stream, the consumer and which signal fired. The readiness
envelope for that tick and every later one is degraded, as for a failed observation (`readiness.go:132`, one compute
for both channels, `:93-101`). A lookup that fails for any other reason keeps today's behaviour: stuck for that tick
only, clear on the next good one. Nothing is rebound, stopped, drained or deleted; the old loop stays as it is until
`Stop` drains it as today (`component.go:574-587`).

Why both signals. Signal 1 alone misses case a, where the loop never ends (probe a: `closed=false` after 77 s).
Signal 2 alone misses f, where the stream is untouched. Today's lookup catches a and d only until someone creates the
consumer again; the latch keeps them unhealthy afterwards. Every case a, d, e and f then ends unhealthy until the
process restarts. Case a before the producer creates the stream again is unhealthy through today's failed lookup
(stream not found), and latched from the first tick after the stream exists.

Why a latch and no clearing. A consume loop that has ended does not restart (probe e and f), and a loop that resumes
on a recreated stream stamps the old generation (`keyed_ingest.go:88`; inventory § 2.4). Clearing would report a
binding healthy that the ruling says only a restart repairs. A restart reads the generation again.

Constraints the developer keeps:

- The tick reads the loop's end through the handle's `Closed()` channel, read without blocking. What the readiness
  records hold is that receive-only channel and the bind-time generation, never the handle: observation stays
  read-only and separate from lifecycle authority (ADR-095 `:66`; `readiness.go:200-203`). Stop keeps the exact
  handle and drains it (ADR-094 `docs/adr/094-...md:128-131`); no `ConsumeContext.Stop` is added.
- No new goroutine: the work runs inside the existing tick, under the run context `Start` creates (ADR-113
  `docs/adr/113-...md:62-64`).
- The stream's creation time comes from the same read the tick already makes (`readiness.go:209-212`) and is compared
  with the generation as `consume.go:96` stores it. No error text is inspected.
- No natsclient change and no new field on `Component` reachable only from tests (#170; #301).
- Library facts this rests on: the loop ends after a terminal status and `Closed()` fires (probe d, f; `pull.go:740`,
  `:273-281`, `:306-317`); in a, `Closed()` does not fire (probe a); a recreated stream has a new creation time
  (probe a, b) and a file stream keeps it across a restart (probe c). `Closed()` can fire early after a stop or a
  forced close (#83); either happens only while the client or component is stopping, when an unhealthy reading is
  true. Timings (tick cadence, nats.go's 30 s re-pull) are not promised.

Not changed, and stated: inputs a resumed old loop delivers before the tick records the loss carry the old generation
and can be acknowledged without being applied. That is the declared limitation "Stream and consumer created again while
disconnected" (`openspec/specs/graph-ingest-recovery/spec.md:180-185`, owner ruling #91 comment 6113959050); this
change makes it visible, not closed. A stream recreated between the generation read and the bind
(`spec.md:165-173`) is also recorded by signal 2 on the first tick; the spec does not promise that.

## D2. Text

- `openspec/specs/graph-ingest-recovery/spec.md:21-25`: the memory-restart scenario says graph-ingest reads unhealthy
  until its process is restarted, and publishing again after the restart applies the message (spec delta).
- `docs/contract.md:79`: the memory row's "Your part" becomes: restart the process when `/readyz` fails (`/healthz`
  only says the listener answers, `service/doc.go:177`), then re-publish from your source.
- `processor/graph-ingest/README.md:188`: a lost binding reads unhealthy until restart; nothing binds a new one.
- `docs/admission-ledger.yaml`, graph-ingest row (`:2381-2386`): one `known_risks` clause (a lost binding latches
  `Health()` until restart; resumed-loop inputs are the declared limitation) and the new test file in
  `proving_tests`.

## Tests

All in `processor/graph-ingest`, integration tag, waiting through `awaitHealthAfterTick`
(`health_integration_test.go:22-31`), which runs its own tick per observation. "Still unhealthy" means a fresh tick
run after the step, not elapsed time.

| Test | Case | Asserts | On the base |
| --- | --- | --- | --- |
| T1 `TestIntegration_LostBindingAtBrokerRestartStaysUnhealthy` (own server: restarts it; `require.Equal` on `f.URL()` before and after, `natsfixture/restart.go:25-29`) | a, a then b | memory stream; graph-ingest stays running across `Restart`; stream created again: unhealthy, `LastError` names the consumer and "restart"; consumer then created again under its name: still unhealthy | fails at the last step (the lookup succeeds, healthy) |
| T2 `TestIntegration_LostBindingStreamRecreatedInPlaceStaysUnhealthy` | d, e | stream deleted and created again: unhealthy; consumer created again: still unhealthy; the envelope from `computeBacklogStatus` is degraded | fails at the last step |
| T3 `TestIntegration_HealthStaysStuckAfterConsumerRecreated` (replaces `..._ConsumptionNotChecked`, `health_integration_test.go:80`) | f | consumer deleted: unhealthy; created again: still unhealthy | fails at the last step |
| T4 `TestIntegration_FileStreamRestartStaysHealthy` (own server) | c | file stream; across `Restart`: healthy on a fresh tick and a message published after the restart is applied | passes (pins kept behaviour) |
| T5 `TestIntegration_StopRecordsNoLostBinding` | Stop | after `Stop`, no lost-binding log line was written | passes |

`TestIntegration_HealthReadsLostConsumer` (`:57`) stays unchanged and must still pass. Examples suffice; the inputs
do not interact beyond the five cases (`docs/testing.md`).

Mutation rows (`task mutate:check`, integration tests by `-run`, recorded in `notes/3.2.md`):

| Mutant | Expected detection |
| --- | --- |
| M1 signal 1 removed | T3 |
| M2 signal 2 removed | T1 |
| M3 the latch cleared by a good observation | T1, T2, T3 |
| M4 the latch recorded when `Stop` ends the loop | T5 |
| M5 every failed lookup latched | T4, only if a tick fails during the restart; otherwise reported as a survivor and listed as uncovered on the PR |

## Dispositions

- #216: retired (`Closes #216`). The healthy-but-not-consuming case (e, f) is part of it and is not filed apart.
- #15: stays. Its replay-protection work is wider; this change keeps the resumed-loop case declared (D1).
- #301: stays. No test-only field is added; T1 to T5 use real streams.
- #83: stays. The early `Closed()` it tracks only occurs while stopping (D1).

## Merge order

Open PR #414 shares no file (inventory § 3.1). If it merges first, this branch merges `origin/main`; nothing in
`graph/readiness` is read differently.
