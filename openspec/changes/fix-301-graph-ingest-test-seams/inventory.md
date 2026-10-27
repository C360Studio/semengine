# Inventory: fix-301-graph-ingest-test-seams

**Superseded in scope.** The owner rescoped #301 on 2026-10-27 (#301 comment 6453844541) after this inventory; `design.md`
follows the rescope. The field and test lists below still describe the base.

Architect inventory, revision 2, verbatim. Claude's reviewer returned INVENTORY PASS in round 2; its two MEDIUM findings
and the NIT are carried into `design.md` (the P6 citation is `keyed_ingest.go:330`; the straddle unit test runs on a
component whose `release` has run; the #192 ruling is comment 6127721456).

Probes in this round are source reads at the pinned library versions (nats.go v1.54.0, nats-server 2.15.0/2.15.1). None
was run, because a run would need a probe file in the tree and this role is read-only. Each is marked "read" or
"unprobed (run)".

## Fields: where each is declared, the production default, and the production reader

Unchanged; the reviewer confirmed these lines.

| Field | Declared | Production default and reader |
|---|---|---|
| waitForStreamInput | component.go:69 | `c.waitForStream` (consume.go:252), read at consume.go:80-83 |
| consumeStream | component.go:70-75 | `c.natsClient.ConsumeStreamWithConfig`, read at consume.go:152-156 |
| watchEntityStates | component.go:77 | `bucket.Watch`, read at poison_inventory.go:307-310 |
| subscribeRequests | component.go:80-84 | `c.natsClient.SubscribeForRequests`, read at component.go:526-528 |

PR #431 added no test-only field. Its `boundConsumer.closed` has a production reader in readiness.go.

## Correction: other test seams of the same kind exist and are already ruled

- `natsclient/client.go:86-90` `netDial` is kept under a waiver. The owner ruled on 2026-10-12 (#192 comment 6127721456)
  and in the same ruling removed `commitHook` and deleted its tests. That is the drop / waive pattern for this issue.
- `service/metrics.go:43-46,220-224` `testListener`.
- `service/service_manager.go:72-75` `httpListener`, which only tests read.

## Tests that depend on the fields (the write lines are as in round 1)

- waitForStreamInput:
  - config_fields_test.go:214 `bindEntityStream`, used by TestConfigAckWaitAndHeartbeatInterval (:238).
  - statement_metadata_test.go:83 `startStreamLane`, 14 refs across the statement_metadata, derived_time,
    fact_lane_fence and write_modes tests.
  - guard_generation_test.go:234, in :222.
  - failed_start_callbacks_integration_test.go:347, in :327.
- consumeStream: the same three unit helpers, plus:
  - `newSuiteOwner` (lifecycle_adapter_test.go:34-36). Its users are lifecycle_integration_test.go:332 (the suite),
    :406, :467 (T-G4), :553, :597 (:577), and :854 `newRelayedComponent`, which feeds :898, :953 and
    health_integration_test.go:198. Also lifecycle_owner_test.go:132 (unit).
  - lifecycle_integration_test.go:127, in :101.
  - failed_start:237 (:210) and :351 (:327).
  - long_apply:74 (:101).
  - readiness:211 (:175) and :471 (:442).
  - settlement:122 `startSettlementIngest`. Its users are :329 and the child helper `runSettlementHelper` (:73-84). With
    the held wrap (`settlementHeld`) that helper serves **TestIntegration_Settlement_ProcessKilledBetweenApplyAndAck
    (:246, helper started at :261)**.
- watchEntityStates: lifecycle_integration_test.go:231, in :187.
- subscribeRequests: failed_start:152 (:64) and :370 (:327); lifecycle_integration_test.go:472 (:448) and :602 (:577).
- Long-apply and settlement use consumeStream only as a **timing hook**, to write the private field
  `c.ingestGuardBucket` after Start acquires the bucket and before the consumer binds. That is test code, not production
  surface.

## Library facts read this round

- **P1 (read): a test logger can see Debug records.** The component's logger is `deps.GetLogger().With(...)`
  (component/dependencies.go:74-75), so a test handler whose `Enabled` admits Debug receives them. The existing
  precedent handler admits Info and above only (lifecycle_integration_test.go:515-517); a cue on a Debug record needs
  its own handler.
- **P2 (read): a permission rule can refuse the boot sweep's watch.** KV `Watch` subscribes an ordered consumer through
  `pushJS.Subscribe` (nats.go jetstream/kv.go:1355-1360). Its name is a generated hash (js.go:1954-1959). On a server at
  2.9 or later, with a filter, the create request goes to
  `$JS.API.CONSUMER.CREATE.KV_ENTITY_STATES.<hash>.$KV.ENTITY_STATES.>` (jsm.go:511-524, js.go:160).
  - So denying publish on `$JS.API.CONSUMER.CREATE.KV_ENTITY_STATES.>` refuses only the sweep's watch. The input
    consumer is on the ENTITY stream.
  - Precedent: an embedded server that denies one user one subject
    (config/manager_lifecycle_integration_test.go:368-376).
  - Unprobed (run): a denied request returns no reply, so the watch fails on its timeout or on Start's context, not at
    once. The precedent measured 25 s for five watches.
- **P3 (read): "updates closed before the end-of-snapshot marker" has a real trigger.** nats.go closes `Updates()` from
  the subscription's closed handler (kv.go:1364-1368), which also fires when the connection closes. A client closed
  during the sweep is the trigger, so this case is not make-believe. The round-1 "drop" is withdrawn.
- **P4 (read): the sweep can be tested without the field.** `startEntityStateGuard(ctx, bucket *natsclient.KVStore)`
  takes its bucket as an argument (poison_inventory.go:306). `KVStore.Watch` passes straight through to `bucket.Watch`
  (natsclient/kv.go:729), and `client.NewKVStore(wrapper)` (kv.go:55) is already used in tests
  (commit_ambiguity_integration_test.go:88). A test can call the sweep on a wrapped store for all three exits.
- **P5 (read): a log record marks the point between apply and ack.** `"Entity ingested"` (Debug, ingest.go:104) is
  logged after the entity write and before the guard stamp and the ack (keyed_ingest.go:210-330, applyIngest;
  ingestEntity at about :85, guard stamp after). A log handler that blocks there holds an input while it is being
  applied, on the lane goroutine. Unprobed (run).
- **P6 (read): a failed guard write logs a cue.** The Warn record "redelivery-guard durable stamp failed"
  (keyed_ingest.go, about :121) can signal that the failure happened. A guard write fails for real when the test deletes
  the applied-seq KV stream: a publish to a subject with no stream gets no responders. Unprobed (run): does a recreated
  bucket accept the redelivery's write through the same KVStore handle?

## Classification of each test

- **The boot sweep (:187), three exits.** Move the three exits to tests that call the sweep directly on a wrapped
  KVStore (P4). Prove "a failed sweep fails Start and the rollback leaves nothing running" with one real denial (P2).
  Classified real fault / no field. Cost: the denial case waits out a timeout.
- **T-G4 (:448).** Cue on the Debug record "graph-ingest subscribed (JetStream)" (consume.go:244) for the query row,
  using a Debug-enabled handler (P1). Cue on "query handlers registered" (query.go:58) for the mutation row.
  - Both cues cancel *between* registrations. The next registration is then refused by `ctx.Err`
    (natsclient/client.go:1095).
  - The assertion "no registration after the one during which the context ended" (:487-497) becomes: Start wraps
    Canceled, nothing is Unresolved, Stop returns nil, and a request to the later subjects gets no responders.
  - Classified real fault, with the assertion restated.
- **:577 (#315).** Unprobed (run): does closing the client on the cue still leave `Unresolved` empty and `Stop == nil`
  (:625-633), given that the rollback then drains on a closed client? If not, the case narrows to T-G4. Classified real
  fault if the probe passes.
- **:101.** Use two input ports and cancel on the second port's Debug record "Setting up JetStream consumer"
  (consume.go:104, which carries the stream attribute). A handler that blocks on that record holds Start for the
  overlapping-Stop check. The "durable kept" check stays on the server (:177-180). "Drained exactly once" needs a
  server-side read; unprobed (run). Classified real fault, partly unprobed.
- **Readiness (:175, :442) and long apply (:101).** Hold the input at "Entity ingested" (P5) instead of the consume
  barrier. The test then holds the apply on the lane goroutine, but no longer holds the write itself.
  - For long apply, the `progressCountingMsg` InProgress count and the "write on graph-ingest's own context after the
    hold" check go. The oracle becomes "not redelivered past ack_wait", read from the consumer on the server.
  - PauseConsumer (nats.go jetstream.go:211) stays as a fallback for readiness.
  - Classified real cue (unprobed). The long-apply oracle is restated.
- **ProcessKilledBetweenApplyAndAck (:246).** The child blocks its log handler at "Entity ingested" (P5) and is killed.
  Classified real cue (unprobed).
- **FailedGuardWriteIsRedelivered (:329).** Delete the applied-seq bucket, wait for the Warn record, recreate the
  bucket, and observe the redelivery (P6). Classified real fault (unprobed).
- **bindEntityStream (TestConfigAckWaitAndHeartbeatInterval).** No fault is involved. Two options:
  - (a) Split the consumer-config build out of `setupJetStreamConsumer` into a pure function that production calls.
    Stays a unit test.
  - (b) Make it an integration test that reads ConsumerInfo. Its refusal row (:281-289) must then check that no consumer
    exists on the server.
  - Recommendation: (a).
- **startStreamLane (14 refs).** Use a named method, which only copies how production is built if production builds the
  lane. Production would build `streamLane{pool, submitCtx, cfg, heartbeat, generation, subject, importLane}` through
  one constructor whose only NATS input is the stream's creation time, and setupJetStreamConsumer would call
  `lane.deliver` as the callback. Tests use the same constructor, so they cannot build a lane production refuses. The
  zero-generation refusal (consume.go:92-96) stays in that constructor.
- **TestStreamWithoutCreationTimeIsRefused (:222).** I know of no trigger for a zero `Created`; this was not probed.
  With the constructor above it becomes a unit test of that constructor: no field, and the guard stays as a fail-closed
  branch. If it is dropped instead, the guard stays as a commented fail-closed branch. The effect on the 80% floor
  (scripts/cover-check.sh:31) is not measured.
- **The suite adapter's call counting** (spec openspec/specs/lifecycle-suite/spec.md:176-185, "count every cleanup or
  external call"; users listed above). Without consumeStream, a no-op Stop can be shown on the server: after Stop, a
  published input stays pending, and query subjects get no responders. Whether that observation satisfies "count" is a
  spec-reading question for the design. Classified unresolved until then.
- **The straddle tests: FailedConsumerSetupCallbackNaksAfterRollback (:210) and FailedStartDrainsAndNaksHeldInput
  (:327).** The callback logs nothing on its normal path before the submit (consume.go:156-229), so a log record cannot
  hold it. Its deterministic core, "an input that reaches a shut-down pool is Nak'd" (consume.go:225-229), can be tested
  through `lane.deliver` against a shut-down pool with no NATS. The straddle itself is the #185 timing: Client.Close
  during setup (natsclient/owned_delivery.go:181-197). Classified no deterministic real cue.
- **FailedRegistrationRunsNoRequestWork (:64).** The refusal can be shown with a real cue: hold Start at "query handlers
  registered" (query.go:58), send a real request, and receive ErrNotStarted (component.go:530-532). The "handler ran,
  then the flush failed" window (natsclient/client.go:1132-1145) is a #185 timing condition, and
  natsclient/subscribe_registration_test.go proves it on the natsclient side. Classified refusal real; the window has no
  real cue.

## Premises, overlaps and governing records

The reviewer confirmed all of these.

- "Start replaces the entity bucket before the sweep" still holds (component.go:643-648, :475). It no longer matters,
  because the sweep takes the bucket as an argument (P4).
- "Client has no seam" still holds (natsclient/client.go:62; component/dependencies.go:43).
- Open PRs: #436 only. Open issues: #300, #270, #195, #196. No conflicts and no merge-order constraint.
- Governing: ADR-113, ADR-112, ADR-108, the lifecycle-suite spec, the no-back-door rule
  (semengine-developer.md:118-123), and the 80% floor.

## Owner question, narrowed

Two tests still have no deterministic real cue: :210 and :327, the callback that straddles a failed-Start rollback.
Pending one probe each:

- the suite adapter's "count" wording;
- :577 closing the client and the rollback still releasing everything;
- :101's server-side drain read.

The options for the two tests:

- (a) Drop them. natsclient proves the window, and #185 rates it MEDIUM at most.
- (b) Replace them with a unit test of `lane.deliver` against a shut-down pool, which proves the Nak.
- (c) Keep one narrow seam with a waiver, as `netDial` was kept (#192 comment 6127721456).

**Recommendation: (b).** It keeps the fail-closed behaviour proved with no field. It matches the owner's #194 ruling,
which deleted the commitHook tests and kept a waiver only where nothing else could prove the behaviour. With (b), all
four fields go.
