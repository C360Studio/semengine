# Design: simplify-04a-19-graph-ingest

This design rests on the inventory (`inventory.md`), which has not had its `INVENTORY PASS` yet; premises cite its
sections. Line numbers are at the base commit `8c2064e`. Bugs come first (#174), then ADR-109's follow-up (#183,
with #216's test), then the test-only fields (#295), then #139.

## D1. Bookkeeping runs after every write that returns an error (#174)

- Each of the five writes in `entity_writes.go` (create, replace, conditional replace, append, delete; inventory § 1
  #174) runs the bookkeeping when its write call returns an error as well as when it commits: the cache entry for the
  key
  is invalidated, and when the poison inventory holds the key, the record is checked against the key's current bytes by
  the existing `verifyEntityPoisonRecord` (`poison_inventory.go:148`). On the healthy path the poison check is the one
  atomic load the inventory already pays.
- Why every error and not only the ones whose outcome is unknown: invalidating a cache entry is always safe (the next
  read goes to the bucket), and the poison check re-reads only a key that is already recorded. Sorting errors into
  "definitely not written" and "may have been written" would be a second spelling of natsclient's re-run rule
  (`natsclient/kv.go:324-328`); this needs none.
- Real-world trigger (#185): a NATS request timeout under broker load loses the reply to a write the server stored.
  After an unconfirmed **delete** of a poisoned entity (the operator's repair), the record stays and, after D3,
  `Health()` and `/readyz` stay not ready until something touches the key. After an unconfirmed append, batch and prefix
  queries serve the cached entry from before the write until it expires.
- Options: (a) chosen; (b) classify the error first (more code, a second spelling); (c) remove the entity cache, #139
  C (needs the latency measurement #139 asks for; listed in Q1's follow-up); (d) do nothing.
- Observable and tests, both integration tests with `natsfixture.FaultKV` (`faultkv.go:56`), each failing on the base:
  - `TestIntegration_UnconfirmedAppendInvalidatesCache`: a batch query caches entity E; an append whose update is
    stored but answered with `nats.ErrTimeout` (`FailAfter(KVUpdate, …)`) fails; the next batch query returns the
    appended triple.
  - `TestIntegration_UnconfirmedDeleteClearsPoison`: E is stored as a value graph-ingest refuses and is recorded in
    the poison inventory; a delete answered with `nats.ErrTimeout` after it is stored fails; the inventory no longer
    holds E (read through `DebugStatus`, which lists the records). It does not assert `Healthy`: until D3 lands, the
    failed delete also adds to the error count (`component.go:2343-2344`), which `Healthy` reads on the base. Once D3
    has landed, batch 2's task adds the `Healthy` assertion to this test.
- #270 (one unexplained failure of the commit-ambiguity test in the same file area) is not touched; if it fails again
  on this branch it is filed as a flake before the next push (protocol, Known flakes).

## D2. The backlog latch leaves (#183 item 1)

ADR-109 Decision 3: `bootstrap_complete` is true once `Start` returns.

- graph-ingest publishes `bootstrap_complete: true` on every envelope. Its boot work is the `ENTITY_STATES` sweep, which
  `start` finishes before the status loop starts (inventory § 1). `bootBacklogDrained` and the `outstanding == 0`
  latch go; so does #139 B4's fallback branch (`readiness.go:262-266`), which only runs in the case it gets wrong.
- `bootstrap_scope` keeps today's meaning: the backlog read at bind, summed over consumers, or the first full
  observation when a bind read failed. The `graph.IndexStatusResponse` doc comments (`BootstrapComplete`,
  `graph/index_status.go:37-41`, which says it "latches true once"; `BootstrapScope`, `:106-107`) say that for a backlog
  producer `bootstrap_complete` means its boot work finished, not that this scope was worked off.
- A failed observation still forces `ready: false` and `state: degraded` (`ComputeBacklogStatus`, unchanged).
- Options: (a) chosen (ADR-109, ruled); (b) keep the latch (contradicts the ruling).
- Tests: `TestIntegration_ReadinessEnvelope_BootstrapCompleteWithBacklog`: with messages outstanding on a bound
  consumer, the first envelope after `Start` has `bootstrap_complete` true, `ready` false and `lag` > 0. It fails on
  the base. The latch tests in `readiness_test.go` go with the latch; the scope tests stay as tests of the smaller scope
  function. How the test holds a backlog depends on Q2 (today's `consumeStream` barrier, or a stored guard record
  graph-ingest refuses, which leaves the input unacknowledged).

## D3. `Health()` reads stuck states (#183 item 3, #216's test)

ADR-109 Decision 2: stuck states, not an error count.

- `Healthy` is `running`, no poisoned entity, and the latest status tick's backlog observation did not fail. The status
  loop's one compute (`readiness.go:99-102`) feeds the envelope, the gauges and this term, so `Health()` and the
  envelope never disagree about the same tick. A tick cut short by shutdown records nothing.
- `ErrorCount` is still reported and never lowers `Healthy`; the "errors occurred during processing" message goes.
  `Status` is `degraded` while a stuck state holds; `LastError` names each state that holds (poison sample as today;
  the failed observation with the consumer it could not read).
- `README.md` § Health and the `graph-ingest-recovery` spec say the same (spec delta).
- Options: (a) chosen; (b) a separate goroutine or a call to NATS inside `Health()` (I/O in a status read; two
  computes).
- Tests (integration, each failing on the base):
  - `TestIntegration_HealthReadsLostConsumer` (#216, the owner's one test): with graph-ingest running, the test
    deletes its durable consumer through the JetStream API and calls `refreshReadinessStatus`; `Healthy` is false and
    `LastError` names the backlog observation. It asserts unhealthy only. It does not recreate the consumer: graph-ingest
    does not rebind a deleted consumer, and whether consumption resumes on a recreated one is not probed or promised
    (#216 comment 6249597351). It does not close #216.
  - `TestIntegration_HealthClearsWhenObservationSucceeds` (build tag `integration`, NATS fixture): ADR-109's "healthy
    again when it clears", worded on the observation, not on consumption. With graph-ingest running, the test deletes
    graph-ingest's own durable consumer and calls `refreshReadinessStatus`: unhealthy. It then creates a consumer with
    the same stream, name and configuration through the JetStream API and calls `refreshReadinessStatus` again:
    healthy. The test name and its comment say consumption recovery is not checked or promised by this test (graph-ingest
    does not rebind a deleted consumer, #216); it proves only that `Health()` follows the observation.
  - Reading after a direct `refreshReadinessStatus` call: the running loop can land an older compute after the test's
    call, so these tests wait with `probe.Await` (`internal/harness/probe/await.go:21`) on the condition, or assert on
    the value `computeBacklogStatus` returned to the call; never on one read of the KV key.
  - `TestIntegration_HealthIgnoresErrorCount`: an undecodable message is published; it is terminated and
    `ErrorCount` reads at least 1; `Healthy` is true.
  - `TestPoisonRepairRecoveryWithoutRestart` (`poison_scoping_test.go:71`) keeps proving the poison term.

## D4. `published_at` leaves (#183 item 2, ADR-109 Q1)

- `graph.IndexStatusResponse.PublishedAt` and its JSON key go; `readiness.Publisher.Publish` stops stamping it and its
  doc comment drops the sentence; `graph/readiness/watcher.go:22`'s "published_at is not read here" goes.
  `TestPublishStampsPublishedAt` goes; `TestIndexStatusResponseWire` (`graph/index_status_test.go:41-42`) keeps
  `published_at` in its legacy-field input and asserts it is dropped on decode, as `phase` is. That test is the one
  place the key stays spelled. No reader in this tree or in a consumer (inventory § 1 #183 item 2).
- Spec: `graph-transport-boundary`'s envelope requirement becomes "carries no legacy fields" (delta). Ledger: the
  `graph` (`:1990`) and `graph/readiness` (`:2041`) rows say it was removed (#183).
- This touches `graph` and `graph/readiness`, outside the epic line's one package; #183 names both files, and the
  ruling binds it here (ADR-109 Consequences: "One follow-up pull request").

## D5. The heartbeat override and the clock override leave (#295)

- `statusInterval`, `statusTickInterval`, `statusNowFn` and `statusNow` go; the loop ticks at `statusMetricsInterval`
  and reads `time.Now()`. `statusNowFn` has no writer (inventory table).
- Tests: the loop's cadence is proved once without NATS, in `testing/synctest`, as PR #289 does for the Watcher: a
  component built by the factory, no bound consumer, a fake `readiness.StatusWriter`; one publish at start, one per
  `DefaultHeartbeat`, none after the context ends. The five integration tests that set `statusInterval` call
  `refreshReadinessStatus` to publish now instead of waiting for a fast tick, and read as D3 says (`probe.Await` or
  the call's own result). No test waits on the real cadence; the `synctest` test proves the loop.
- Options: (a) chosen; (b) wait at production cadence in all five (about 5–15 s each); (c) a config key for the
  interval (new surface with no one it is for).

## D6. Help text (#287)

`mutation_rejections_total`'s help becomes "Graph-mutation requests rejected, by subject and reason: the error's
classified code, `internal` when it carries none." No test reads help text; none is added (no behaviour).

## D7. #139: what this change does

Each item's binding basis (protocol, Bounded port scope) or the owner's schedule (#223's PR 10 line: plain functions,
tests through the factory):

- **Ruled items.** I: `handlesMu` goes. The argument, not the race run, is the proof: the handles (`cancel`, the two
  pool cancels, `statusDone`, `consumers`, `subscriptions`, `ingestPool`) are written only by `start` and read and
  cleared only by `release`/`cleanup`, and `internal/lifecycleguard` runs those one at a time on the caller's
  goroutine for `Start` and `Stop`. The status loop goroutine closes its own `statusDone` channel and reads no handle;
  each consumer callback holds the `pool` local it was built with (`component.go:1522`), not `c.ingestPool`. The
  developer confirms this list by reading every access, and the `-race` run of the lifecycle and failed-start tests
  is recorded as evidence. K: a missing `ports` key takes `DefaultConfig()`'s ports, a present block is used as written,
  the lane clamp
  moves to `ApplyDefaults`, and `Validate` only checks (it runs twice today, `component.go:621`, `:938`). J: the
  nil-registry branch (`canonical_mutations.go:184-190`) goes; the factory's refusal keeps O-15's intent.
- **Tests through the factory; nil guards go.** The 15 `&Component{}` literals left after D2 build through
  `CreateGraphIngest`; then the nil checks on dependencies the factory refuses or always builds (inventory table,
  row A1) go, and `panicTB` with them. Checks on handles `Start` builds (row A2: `entityBucket`, `entityCache`,
  and `hierarchyInference`, which is also nil by design when `enable_hierarchy` is off) stay: exported `CreateEntity`
  reaches `entityBucket` before `Start`
  (`component.go:2075` → `:2336`), and a guard there turns a nil dereference into an error. The nil
  applied-sequence-bucket branch stays: the spec requires it
  (`graph-ingest-recovery/spec.md:107-108`).
- **Plain functions.** The four methods that never read the receiver become functions: `sweepEntityStateGuardEntry`,
  `extractEntityFromMessage`, `validateTriplePredicates`, `classifyEntityQueryError`.
- **Test-only code.** `handleMessage` (no production caller) goes; its two tests drive `processIngest` instead.
  `repopulateHook` goes: the stale-repopulation test injects its write from a test-side `jetstream.KeyValue` wrapper's
  `Get`, which runs in the same window, and the failed-start test asserts the not-started refusal it already receives.
- **One spelling.** The indexing-profile statement uses `graph.SourceIndexingProfile` (`component.go:1964`), F7.
- **Lint reaches every file.** The four "Code generated … DO NOT EDIT" headers go, so revive reads those files; what it
  then reports is fixed. `metrics_test.go`'s assertions that cannot fail check counter deltas, and
  `TestComponent_DataFlow_ReturnsMetrics` checks that the rates move (Exports).
- **Text.** `doc.go` becomes one paragraph pointing to the README; carried citations that resolve only in SemStreams
  say so or go; the doubled `platform` comment is one; the ledger row's `known_risks` drops #174 and `proving_tests`
  follows the renamed tests.

## D8. #139: what stays (owner questions 1 and 2)

- Optional maintenance, carried or not tied to a binding requirement, not done here: B's two metering wrappers and
  re-decode; B's query error-code spellings; C's entity cache (needs a measurement); D's fact-lane re-validation, the
  `pkg/retry` loop, repeated shapes, file seams and inner timeouts; F1–F6, F8, F9, F12; G's file split; H's receipt.
  Two of them are defects and are filed whatever Q1 decides (protocol, File): `messagesProcessed` counts writes, so
  `DataFlow` reports writes per second as messages (#139 D); a revision mismatch logs two WARNs (#139 B).
  F11 is `natsclient`'s.
- `waitForStreamInput`, `consumeStream` (from the pin) and `watchEntityStates`, `subscribeRequests` (added by the
  port) stay pending Q2. Their tests prove failed-`Start` rollback, settlement and completed joins, and the client is
  the concrete `*natsclient.Client`, which natsclient's owner rule keeps from growing a fault mechanism.

## Exports (#190)

Kinds as in 8d. Changed or removed:

- `graph.IndexStatusResponse.PublishedAt`: removed; no reader; ADR-109 Q1.
- `(*Component).Health`: kept, meaning changes. API: `component.Discoverable`, `lifecycle-suite` spec; `README.md`
  § Health; ADR-109 Decision 2. Product-style test: `TestComponent_Health_Running`
  (`component_integration_test.go:42`, factory, `Start`, `Health`), plus D3's new tests.
- `(*Config).Validate`, `(*Config).ApplyDefaults`: kept; `Validate` stops changing the config. Plumbing:
  `component.go:621-622`, `:938`.
- `readiness.Publisher.Publish`: kept; stops stamping. Plumbing: `readiness.go:329`.

Kept unchanged, API, each with the test that drives it as a product would: `Config`, `DefaultConfig`,
`CreateGraphIngest` (`TestComponent_Health_Running`, which marshals `DefaultConfig()` into the factory);
`DeclarePorts`, `Register` (`TestRegister_AddsToRegistry`, `component_test.go:742`); `Initialize`, `Start`, `Stop`
(`TestGraphIngestLifecycleSuite`, `lifecycle_integration_test.go:308`); `InputPorts`, `OutputPorts`
(`TestConfigPortsAreTheComponentsPorts`, `config_fields_test.go:82`); `DataFlow`
(`TestComponent_DataFlow_ReturnsMetrics`,
`component_test.go:618`; today it asserts only `>= 0` and cannot fail, so task 6.1 makes it check that a processed
message moves `MessagesPerSecond` above 0 and an error moves `ErrorRate` above 0). Plumbing:
`CreateEntity` (`component.go:392`; semsparql calls it); `DebugStatus` (pin `service/component_manager_http.go:496-497`,
`service` admitted). No export is added.

## Open issues and merge order

- Retired: #174 (D1), #183 (D2–D4), #287 (D6), #295 (D5), #139 (D7, if Q1 is answered as recommended).
- Stays, unchanged by this change: #216 (fix with #291; this change adds its test), #291, #282, #270, #20 (owner's
  docket), #111, #196, #141, #195, #106, #107, #109, #110, #33, #134, #103, #15, #16 (inventory § 3).
- No open pull request overlaps (inventory § 3). PR 10 is the wave's last; it merges when ready.

## Owner questions

1. **The rest of the graph-ingest cleanup list (#139).** This change does the items your rulings or the rules require
   and the two you scheduled (plain functions, tests through the factory). About fifteen optional items remain, the
   biggest being splitting the 2,659-line `component.go` and dropping the query cache. **Recommended: move them to one
   new issue so #139 closes with this change.** The alternatives are keeping #139 open, or doing everything here,
   which roughly doubles the review.
2. **Four test-only fields that tests use to fake NATS behaviour.** Where a test can wrap something it already
   holds, this change does that (the cache hook goes that way, D7). For these four it cannot:
   - `watchEntityStates` (added during the port): the test would wrap the entity bucket, but `Start` replaces the
     bucket with a fresh one before the sweep runs (`component.go:1175`), so a wrapper never reaches it.
   - `subscribeRequests` (added during the port) and `consumeStream` (from SemStreams): the call goes to the NATS
     client itself, a concrete type with no place to wrap, and the client may not grow test machinery (your #170
     rule). Three unit tests use `consumeStream` only to capture a setting and could move to a real test stream; the
     seven integration tests that hold or fake a message cannot.
   - `waitForStreamInput` (from SemStreams): used only beside `consumeStream`'s fakes; it can go when that field goes.

   The two from SemStreams fall under the porting rule that leaves inherited code alone unless you schedule it. The two
   added during the port break the no-back-door rule, so keeping them needs your waiver. **Recommended: waive for
   now, keep all four, and file one issue for a way to make NATS fail on cue in tests that retires them.** The
   alternative is removing them now and losing or rewriting about ten tests that prove failed starts clean up and
   messages settle correctly.

**Answered 2026-10-18** (owner, Enjoy conversation "Simplification wave PR 10 pickup"; #223 comment 6249887011):
design accepted. Q1 as recommended: #139 closes with this change, the optional rest is #300, and the two defects are
filed as #302 and #303. Q2 as recommended: all four fields stay, the no-back-door rule is waived for
`watchEntityStates` and `subscribeRequests`, and #301 tracks retiring them.

FYI, no ruling needed:

- (a) After this change `/readyz` (once `service` is ported) goes not ready while graph-ingest cannot read its input
  backlog, and ready again when it can (ADR-109's stated cost). A lost consumer is visible, not fixed.
- (b) `published_at` disappears from the readiness JSON; nothing reads it.

## Spec deltas (draft text for `specs/<capability>/spec.md`)

`graph-transport-boundary`. The requirement's name changes, so the delta carries a `## RENAMED Requirements` entry
(FROM: "The readiness envelope carries its publish time and no legacy fields", TO: "The readiness envelope carries no
legacy fields") and a `## MODIFIED Requirements` block under the new name with the whole text below. The Purpose
(`spec.md:8-9`, "carries its publish time and no legacy fields") is not part of a delta; it is edited by hand at the
archive's spec sync to "carries no legacy fields".

```markdown
### Requirement: The readiness envelope carries no legacy fields

`graph.IndexStatusResponse`, the value a producer writes to its `GRAPH_STATUS` key, SHALL carry no `published_at`,
`phase`, `revision` or `last_synced` field; `indexed_revision` is the one spelling of the revision. graph-ingest SHALL
publish `bootstrap_complete` true on every envelope, its boot work having finished before `Start` returned.

#### Scenario: A published envelope

- **WHEN** a producer publishes its status
- **THEN** the stored JSON has no `published_at`, `phase`, `revision` or `last_synced` key

#### Scenario: graph-ingest with a backlog standing

- **WHEN** graph-ingest has started and messages are outstanding on its bound consumer
- **THEN** its envelope has `bootstrap_complete` true and `ready` false
```

`graph-ingest-recovery`. `## MODIFIED Requirements` carries the whole of "An interrupted application is redelivered,
not a stop" (`spec.md:78-98`, both scenarios), copied with two edits: "the error count its health reads" becomes "the
error count `Health()` reports", and the scenario's "the health error count" becomes "the `ErrorCount` `Health()`
reports". `## ADDED Requirements`:

```markdown
### Requirement: graph-ingest's health reads its stuck states

graph-ingest's `Health()` SHALL report `Healthy` false while it holds a poisoned entity record or while its latest
status tick could not read the backlog of a bound consumer, and true again once neither holds, without a restart. A
count of past errors SHALL NOT lower `Healthy`; it is reported as `ErrorCount`.

#### Scenario: The durable consumer is lost

- **WHEN** graph-ingest's durable consumer is deleted while graph-ingest runs
- **THEN** after the next status tick `Healthy` is false and `LastError` names the backlog observation

#### Scenario: The observation succeeds again

- **WHEN** a status tick's backlog observation fails and a later tick's succeeds
- **THEN** `Healthy` is false after the first and true after the second; this says nothing about whether graph-ingest
  is consuming (a deleted consumer is not rebound, #216)

#### Scenario: A processing error

- **WHEN** graph-ingest terminates an undecodable message
- **THEN** `ErrorCount` rises and `Healthy` stays true
```
