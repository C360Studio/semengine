# Design: setup-04a-04-graph-roots-graph-index

Base `39a17a9`. Inventory: `inventory.md`. Round 3: the owner ruling (PR #414 comment 6426254115) and the Fable read
(comment 6426199810) folded in; independent review next.
Change 4, PR 4 of 9 (#9 comment 6386856657, item 3: "`processor/graph-index` with the follower
(#107, #108, #110, #103; ADR-109 reader)"). Tier-0 only (#9 comment 6386603659): graph-index has no provider
code (inventory 8).
Governing records: ADR-066, 083, 084, 085, 088 (readiness), ADR-108 D1-D5, ADR-109 D1-D4, ADR-113, ADR-095, ADR-058,
pin ADR-074, 077, 078, 068, 060, 072, 081 (inventory 6). Draft PR #414.

Coined terms, as ADR-108 defines them: the *follower* is the part every component that projects `ENTITY_STATES` shares
(watch, startup replay, reset latch, watermark, readiness envelope); the *plug-in* is the part that differs, here
graph-index's index writes and queries. The *startup replay* is the rebuild after process start, up to the revision the
snapshot ended at (ADR-109 Context).

## Options

- **A. Port graph-index as at the pin, follower inline.** Smallest diff from the pin. Contradicts ADR-108 D2 and the
  owner's order ruling ("the follower lands with graph-index"); graph-embedding (change 5) copies the loop again.
- **B. Follower first, its own PR; graph-index after.** Each PR smaller. The follower would land with tests as its only
  callers, the finding the processor-shell design avoided (its "Unit of work"), and against the same ruling.
- **C. One PR: graph-index on `component.Lifecycle`, its watch loop moved into a follower package it composes**
  (chosen). Over the size bound; waived by count (D1).
- **D. C split in two: writer half (component, follower, index writes, envelope), then the query half (`query.go`,
  the by-name handler, the reply envelope, the verb table).** Still over the bound for the first half (about 3,630 pin
  lines, inventory 10), and graph-index lands without its read surface for one PR, which "ported whole" allows only
  on the owner's word.
- **E. Do nothing now.** graph-query (PR 9) and graph/clustering (PR 8) read graph-index's buckets; the change stalls.

## D1. Unit of work and size

One pull request, one OpenSpec change. Pin lines carried: about 4,660 (graph-index 4,527, plus about 130 from `graph`
and `graph/readiness` that the ingest-kernel design deferred to graph-index, inventory 10). About 900 of them do not
enter the tree (D8, D4's shell scaffolding); the tree gains about 3,800. The rule's bound is about 3,000 (#159), so this
needs the owner's waiver for that number. **Ruled** (owner, PR #414 comment 6426254115, item 1): one pull request,
waiver by count under #159 for about 4,660 pin lines carried and about 3,800 entering the tree; Option D is not taken.
The order ruling had already listed graph-index with the follower as one item (#9 comment 6386856657, item 3).

## D2. Packages and files

- `processor/graph-index/` (package `graphindex`): the pin's files less `keyed_dispatcher.go`,
  `kv_contract_benchmark.go`
  (its one production helper, `wildcardPositions`, moves to `predicate_index.go`), `race_enabled.go`, `race_disabled.go`
  and `watermark.go` (its content moves to the follower). `component.go` is split as graph-ingest's was (#411): factory
  and lifecycle in `component.go`, `config.go`, `metrics.go`, the index write path in `index.go`. Moves are part of the
  port, not a separate diff: nothing in this package exists in the tree.
- `graph/follower/` (package `follower`, new; ADR-108 D1: "one package under `graph/` that imports `graph` and
  `graph/readiness` and nothing under `processor/`; the first follower port names the package"). The name is the
  owner's call at acceptance, as `component.Lifecycle`'s was.
- `graph/readiness/index_status.go` gains `ComputeIndexStatus` and `IndexStatusInputs` (beside `ComputeBacklogStatus`,
  where D16 put the projection family), without `LastSynced` and `Revision` (#110 item 1).
  `graph/readiness/gauges.go` gains the two revision gauges.
- `graph/query_response.go` (new): `QueryResponse[T]` and `NewQueryResponse` as D18 decided.
- `graph/query_verbs.go`: graph-index's eight verbs.

## D3. The follower: what it owns, and its surface

Ownership is ADR-108 D4's: the bounded wait for `ENTITY_STATES`; one `WatchAll`; the snapshot-then-live replay with a
validating phase; the reset latch; the lost-feed state; the bootstrap latch and target; the watermark
(`internal/revlag`); the readiness tick (envelope to `GRAPH_STATUS`, gauges). graph-index keeps its coalescer, keyed
pool, repair loop, failed set, index codecs, buckets and handlers (ADR-108 D4-D5).

```go
type Entry struct {
    Key      string
    Revision uint64
    Deleted  bool // a delete or purge marker
    Bytes    int  // the delivered value's length; 0 for a delete or purge marker
}

type Config struct {
    Client          *natsclient.Client
    Key             string        // GRAPH_STATUS key, readiness.KeyGraphIndex; the gauges' subsystem is derived
                                  // from it ("graph-index" -> "graph_index"), so the name is spelled once
    MetricsRegistry *metric.MetricsRegistry
    Logger          *slog.Logger
    StartupAttempts int           // bounded wait for ENTITY_STATES
    StartupInterval time.Duration
    Deliver         func(ctx context.Context, e Entry) // the plug-in takes one entry, in delivery order
    Failed          func() int64                        // the plug-in's unresolved required writes
}

func Open(ctx context.Context, cfg Config) (*Follower, error)
func (f *Follower) Run(ctx context.Context) error
func (f *Follower) Complete(key string, revision uint64)
func (f *Follower) Reset(reason graph.StateResetReason)
func (f *Follower) QueryReady(ctx context.Context) error
func (f *Follower) Indexed() uint64
func (f *Follower) Stuck() []string
```

Two functions in a config, not an interface: one plug-in today, and `internal/dispatch.KeyedConfig.Process` is the
tree's
shape for this (`internal/dispatch/keyed_pool.go:89`). No exported signature names an internal type: the watermark is
held, never returned.

This changes the letter of ADR-108 D4, which has the plug-in "return one of: applied, failed …, or a reset reason"
(`docs/adr/108-processor-shell.md:111-112`). `Deliver` returns nothing; the three outcomes come back later, through
`Complete` (applied), `Failed` (the unresolved count) and `Reset`. The reason: graph-index's apply is asynchronous.
Between delivery and apply sit the coalescer and the keyed pool, which ADR-108 D4 itself gives to the plug-in
(`:112-113`), so the outcome does not exist when `Deliver` returns. Listed under D18 "Changed".

Where the code comes from: graph-index's loop has no validating phase and no lost-feed mark (G5, G6). The three later
adopters have both, and the follower is ported from their shape: `validating := true` and the decode at
`pin:processor/graph-index-spatial/component.go:718-757`, `:766-775` (also temporal `:738-786`, embedding
`:1444-1500`); the lost-feed flag at spatial `:705`, `:731` (temporal `:727`, `:749`; embedding `:1432`, `:1455`).

What a caller observes:

- `Open` does startup work under its context and holds nothing it does not return: it waits for `ENTITY_STATES` up to
  `StartupAttempts` × `StartupInterval` and then returns a transient error naming the bucket; it opens the bucket
  readers, ensures `GRAPH_STATUS` through `kvcatalog.EnsureCatalogBucket`, and registers the gauges on the injected
  registry. A nil context, nil client, empty key, nil `Deliver` or nil `Failed` is refused.
- `Run` runs on the caller's goroutine until its context ends, then returns nil, with no watcher and no ticker left
  (`background-work` spec, the `Run(ctx)` shape `graph/readiness/watcher.go:195` uses). A nil context is refused.
  `Run` publishes the envelope once at once, then every `readiness.DefaultHeartbeat`. The heartbeat runs on a
  goroutine of its own, which `Run` starts and joins before it returns, so a `Deliver` parked on a full pool lane never
  delays it. The reason: three missed heartbeats (15 s, `graph/readiness/watcher.go:119-123`, `:237`) make every
  outside reader of key `graph-index` read the envelope as unknown (semsource's MCP gateway, semconnect's
  index-readiness probe, inventory 9). The pin ran the tick as its own goroutine (`pin:…/component.go:1232`), as
  graph-ingest does (`processor/graph-ingest/component.go:536`). `Complete`, `Failed` and `Reset` may be called at any
  time, also after `Run` returned; they then change nothing anyone reads.
- Every delivered revision is observed in the watermark before `Deliver` is called with it. Until the snapshot's end
  marker, each non-deleted value is decoded with `graph.UnmarshalEntityState(key, value)`; a refusal latches reset with
  the refusal's reason, and the entry is still delivered (ADR-108 D4; graph-index's pin loop had no validating phase,
  G6; spatial, temporal and embedding stop applying after a reset, so whether to apply after a reset stays the
  plug-in's: graph-index keeps indexing the other entities, as at the pin). At the end marker the bootstrap target is
  the highest revision observed.
- The *feed lost* state is set when (i) `WatchAll` fails, (ii) the `Updates()` channel closes while `Run`'s context is
  live, or (iii) a heartbeat's status read shows that `ENTITY_STATES`'s backing stream was created after the one `Open`
  bound (its creation time differs). `Stuck` names it, the envelope reads `degraded` from the next tick, queries are
  refused (D5), and `Run` keeps publishing until its context ends. Nothing rebinds the watcher; the state holds until
  the process restarts (owner ruling, PR #414 comment 6426254115, item 2). At the pin graph-index's two paths returned
  with no mark (G5); ADR-108 D4 (`:104-106`) asks that the loss be declared, with a failing test first.

  The real triggers, measured at nats.go v1.54.0 / nats-server v2.15.1 (embedded server, JetStream file store, client
  `MaxReconnects(-1)`, `ReconnectWait(2s)` as natsclient's defaults, `natsclient/client.go:308-309`):

  - *A server restart with its store* is not a lost feed. Probe on PR #414, comment 6425769680: in four runs (1 s and
    10 s outages) `Updates()` stayed open and resumed with the next write, every run at 20.04 s, with no replay and no
    close. The mechanism (Fable read, PR #414 comment 6426199810, H1): the KV watcher is nats.go's legacy push ordered
    consumer (`jetstream/kv.go:1333-1334`), with an idle heartbeat every 5 s and an activity check every 10 s
    (`js.go:218-222`, `hbcThresh = 2`; `activityCheck` `js.go:2496`). A check that finds no activity while connected
    recreates the consumer at the next sequence; while disconnected it waits. So the stall after a reconnect is at
    most two check periods, 20 s: library constants, not promised (#170), sitting 10 s under the 30 s stuck detector,
    with nothing else declaring it in between.
  - *The bucket deleted under a live watch* closes `Updates()`: the recreate fails with "stream not found", the
    subscription is unsubscribed (`js.go:2354-2358`, taken at `:2415`), and the watcher's closed handler closes the
    channel (`jetstream/kv.go:1364-1367`). Measured: closed 18.00 s after the delete (one run). Trigger (ii).
  - *The bucket deleted and created again* (an operator wiping the graph, as graph-index's own reset message tells
    them to, `pin:…/query.go:211`; or a server restarted without its store, whose stream graph-ingest recreates) does
    **not** close it. The consumer is recreated at the old next sequence on the new stream: with three revisions
    before and six after, revisions 1-3 of the new bucket were never delivered, 4-6 were, at 20.04 s, and the channel
    stayed open past 45 s. A silent stall, invisible to the watermark (the new stream's last sequence 6 is above the
    floor). The old handle's status read returns the new stream, whose creation time differs. Trigger (iii) catches
    it at the next heartbeat.

  Delete and recreate probe: program and output in `/tmp/gi-probe-delete/` (`main.go`, `out-delete.txt`,
  `out-recreate.txt`, `out-recreate-6puts.txt`, `out-recreate-created.txt`), to be posted on PR #414 by task 1.3. One
  status read per heartbeat gives both the last sequence (the target, as `natsclient.BucketLastSeq` reads it,
  `natsclient/kv.go:110`) and the creation time; one read site, not two.

  `ENTITY_STATES` keeps one revision per key (`History: 1`, `graph/kvcatalog/kvcatalog.go:79-87`), so a revision
  overwritten before a watcher reads it is gone from the stream, with or without a restart (the restart probe's run 3:
  rev 4 stored, overwritten by rev 5, only 5 delivered). The watermark tolerates it: it floors over observed revisions.
- `Reset` latches the first reason and holds it for the process lifetime (ADR-074 `:69-71`, ADR-084 D4); later calls
  change nothing.
- `Indexed` is the watermark's floor: every delivered revision at or below it has been completed.
- `Stuck` returns the states that hold now, each a short sentence: the reset reason, the count of unresolved writes
  when `Failed()` is above zero, and feed lost. Empty when none holds.

The envelope, computed at each tick and by `QueryReady` (all from the pin's `watermark.go`, kept): `indexed` and its
commit time from the watermark, `target` from `natsclient.BucketLastSeq` on a status reader of its own (the pin's
second handle, `component.go:236-239`), `ComputeIndexStatus`, then the empty-graph override (target 0 after the end
marker reads ready) and the failed override (`Failed()` above zero reads `ready` false, `degraded`), then the bootstrap
latch (ADR-084 D2: set when the floor reaches the target fixed at the end marker, or when ready; never cleared). A
target read that fails reads `degraded`. The stuck-watermark detector (ADR-066 §4: no advance for 30 s while behind)
reads `degraded` in the envelope only; it is not a `Stuck` state, since ADR-109 D2 names the stuck states and a slow
replay is catch-up, which ADR-109 D4 keeps per request. One addition: the envelope's `failed_count` carries
`Failed()`; the pin's graph-index wrote 0 (`pin:graph/index_status.go:190-199`).

## D4. graph-index on `component.Lifecycle`

As graph-ingest (processor-shell D6): `Initialize` returns `c.shell.Initialize()`; `Start` refuses a nil client, then
`c.shell.Start(ctx, c.start, c.cleanup)`; `Stop` returns `c.shell.Stop(ctx)`; `DataFlow` returns `c.shell.DataFlow()`;
`Health` returns `c.shell.Health(c.follower.Stuck()...)`. `start` creates the run context from
`context.WithoutCancel(ctx)` (ADR-113; the pin derived it from Start's context, G1), builds the keyed pool and the
coalescer under it, opens the follower under Start's context, starts `Run` and the repair loop under the run context,
then subscribes the handlers. `cleanup` follows graph-ingest's order (`processor/graph-ingest/component.go:587-595`):
stop taking requests, stop delivery (the follower's context and the repair loop end, so nothing new is submitted),
drain the keyed pool with `Shutdown(ctx)` while the pool's own context is still live, then end the pool's context, all
within Stop's context, tolerating a partial start. One order for both shell adopters (Fable read, M2).

Declared behaviour changes, each from a binding record:

- A second `Start` on a running component returns `errs.ErrAlreadyStarted`; the pin returned nil (ADR-108 D3(a), G2).
- Ending Start's context after `Start` returned stops nothing (ADR-113, G1).
- `Healthy` is false only while a stuck state holds; `ErrorCount` never lowers it (ADR-109 D2, G3). The pin's delete
  and list failures lowered it until restart (`component.go:2106`, `:2134`, `:2151`); now an unresolved write is a
  stuck state that clears on repair.
- `Initialize` no longer re-validates (ADR-108 D3(d), G9).
- `DataFlow` counts inputs: one per delivered `ENTITY_STATES` entry, update or delete, with `Entry.Bytes`, counted in
  `Deliver` before the coalescer, so entries the coalescer folds together are each counted. The apply still reads the
  key's current authoritative state, not the delivered value. `ErrorCount` counts each entity whose required writes
  ultimately failed and each reset. The pin counted index writes
  as messages at eight sites with three meanings (S7); `Messages` now means what the shell's spec says ("input
  messages counted"), as graph-ingest's #307 ruling 1 did.

- Work the keyed pool has accepted when `Stop` runs is written before `Stop` returns nil (the pool's context is still
  live while it drains, `internal/dispatch/keyed_pool.go:115-118`, `:270-305`); a clean `Stop` logs no write failures
  and counts no errors for it. The pin's dispatcher dropped queued work. Entries still waiting in the coalescer are not
  submitted; the next start rebuilds every index (D13). A `Stop` whose context ends first returns that error (ADR-113).

## D5. The readiness reader (ADR-109 D1)

`QueryReady` is the pin's `ensureQueryReady` (`query.go:208-262`), moved: reset latched → fatal
`graph_state_reset_required`; `Failed()` above zero → transient `index_not_ready` ("unresolved index write failures");
bootstrap latched → nil, at once, with no target read (the sticky path, kept: a post-bootstrap `degraded` keeps
serving, ADR-084 D7 as ADR-109 changed it); otherwise compute the envelope, latch if built, and ask
`readiness.EvaluateReadinessGate(readiness.Reading{Status: s, Fresh: true})` (the call the gate's doc names,
`graph/readiness/readiness_gate.go:80-81`): `DeferNone` → nil, any other reason → transient `index_not_ready`
("initial build has not completed"), the reason in the debug log.

Lost feed, **ruled** (owner, PR #414 comment 6426254115, item 2): while the follower reports the feed lost, each of the
eight verbs answers class transient, code `index_not_ready`, checked after reset and before the failed count. This
applies the meaning ADR-109 D1 keeps for a watch that cannot bind or closes, the one the lifecycle manager already
answers (`pkg/lifecycle/manager_query.go:205`, `:234-236`; at the pin `:218`, `:248`); it is not a widening of
ADR-109. The pin's three later adopters do the same (`pin:processor/graph-embedding/component.go:1727-1729`,
`pin:processor/graph-index-spatial/component.go:869-871`, `pin:processor/graph-index-temporal/component.go:899-901`).
The state is a stuck state in `Health` and `degraded` in the envelope until the process restarts.

Which calls withhold: all eight handlers, as at the pin (ADR-109 D1 lists them). The first-build window gets the test
ADR-109 D1 asked change 4 to name: `TestQueryDuringStartupReplayIsIndexNotReady`. The pin's
`TestQueryStatus_NonEmptyReplay_NotReadyUntilCaughtUp` only checked that the call did not succeed.

Removed with the move (S13, S14): the pre-`Start` fallbacks (`watermark == nil` reads ready; the retired NAME_INDEX
signal; the missing-status-handle fallback) and the `statusInterval` field. Each was reachable only from tests; the
follower's tick test runs inside `synctest` instead of shortening the heartbeat. The follower is new code, so the
inherited-structure exception does not cover a test seam in it.

## D6. Replies: the envelope (D18, #103) and the verb table (#106)

- `graph.QueryResponse[T]{Data T; IndexedRevision uint64; Producer string; Timestamp time.Time}`, JSON `data`,
  `indexed_revision`, `producer`, `timestamp`; `graph.NewQueryResponse(producer string, indexedRevision uint64, data T)
  (QueryResponse[T], error)` refuses an empty producer (D18). Every successful reply of the eight handlers is one,
  with producer `graph-index` and `indexed_revision` = `follower.Indexed()` read before the handler's first bucket
  read, so every revision at or below it is reflected, with one exception while #415 is open (D12): on an `alias`
  reply, R means every alias added at revision R or below resolves; an alias dropped or changed, or whose entity was
  deleted, at R or below may still resolve to the old entity. The alias index is not repaired here. Refusals stay
  classified errors with no envelope.
- `min_revision` is not here: D18 declares it with its first embedder, the `graph.query.*` request types graph-query
  brings (PR 9). #103 stays open for it and for the generation marker (#15).
- `graph.QueryVerbs()` gains `outgoing`, `incoming`, `alias`, `predicate`, `predicateList`, `predicateStats`,
  `predicateCompound`, `byName` on `graph.index.query.*`, responder `graph.ResponderGraphIndex` (`"graph-index"`).
  graph-index subscribes by walking the table's entries for its responder and fails `Start` naming a declared verb
  with no handler (graph-ingest's rule, `processor/graph-ingest/query.go:44`). The contract test then refuses a
  literal of these subjects outside the table. Refusing a second subscriber at boot stays with #106 for graph-query's
  PR, which has the second responder family.

## D7. Keyed pool (#107)

graph-index submits to `internal/dispatch.KeyedPool` (`Lanes` = `workers`, per-lane `QueueDepth` = 1000 / lanes, at
least 1, as the pin's split; `Name` `graph_index`; the injected registry). `keyed_dispatcher.go` does not port. Same-key
work stays in submit order (the pool's contract, `keyed_pool.go:186-191`). `OnPanic` marks the entity failed so the
repair loop retries it.

## D8. Surface that does not port (surface audit)

Each has no production reader at the pin (inventory 7):

- S9: `UpdateOutgoingIndex`, `UpdateIncomingIndex` with `updateIncomingIndexBatch`, `UpdatePredicateIndex`,
  `DeleteFromPredicateIndex`, `DeleteFromAliasIndex`, `UpdateNameIndex` — dead, and each a back door that writes an
  index outside the watch path. S10: `UpdateAliasIndex`, `DeleteFromIndexes` become unexported (internal callers only).
- S11: `handleEntityDelete`, `processEntityUpdate`, `processEntityUpdateResult`, the nil-pool branch,
  `revisionCoalescer.Close`, `nameIndexForwardFilter`, `incomingIndexTargetFilter`, `raceEnabled` (its one reader is a
  `t.Skip`, which the harness forbids).
- S12: `subscribeForRequests` (#186). The three failed-start tests (`failed_start_subscription_test.go:48`, `:125`,
  `:201`) fail a real subscription instead: the developer picks the trigger, recorded in a task note.
- S15: `batch_size`.
- S16 (optional, done because tests are rewritten anyway): `reindexTotal`, `reindexUnchanged`; tests read
  `reindex_events_total`. `lastProjections` stays: it feeds that metric, #108's instrument (memory: one string per
  entity; recorded in the ledger row).

Tests that seeded indexes through S9 seed `ENTITY_STATES` and let the follower deliver: an oracle independent of
the code under test.

Kept, optional (Bounded port scope; none ties to a binding requirement): S4 (the owned-bucket set spelled five times;
cost: a sixth bucket edits five places), S5 (`map[string]interface{}` intermediate), S6 (fixed 5 s handler deadlines
inside natsclient's own; cost: two deadlines, the shorter wins), S8 (two narrow reader interfaces).

## D9. Configuration

Strict decoding (`DisallowUnknownFields`, as graph-ingest's `resolveConfig`); an unknown key fails construction naming
it. Keys: `ports`, `workers` (0 means the default, 1; otherwise 1-16; the lane count), `coalesce_ms` (0 = no
coalescing), `startup_attempts` (default 30), `startup_interval_ms` (default 500). Each has a test that fails when it is
ignored. `workers: 0` must keep working: semsource sends it to mean "use the default"
(`semsource:cmd/semsource/run.go:805-807` at `e4febc0d`), as the pin turns 0 into 1 (`pin:…/component.go:133-135`).

`batch_size` is refused as unknown (the strict-config rule is a verify gate, and nothing reads the field). That breaks
two consumer configs at boot, each naming the key: semconnect's `deploy/semstreams.json:65` and
`conformance/compose.semstreams.config.json:65` (both `"batch_size": 50` under `graph-index`, at `d0d06e00`); and
semteams publishes the field in `schemas/graph-index.v1.json:8` (at `ab480702`). semsource does not set it for
graph-index (`run.go:800-810` at `e4febc0d`). Each consumer deletes one line when it adopts SemEngine; listed under D18
"Changed".

## D10. Metrics (#68, ADR-058)

Collectors are built at construction and registered through `metric.RegisterOrGet` on the injected registry, using the
collectors it returns; no `sync.Once`, no global-registry fallback, namespace `semengine`, subsystem `graph_index`
(G7). A nil registry registers nothing (as `readiness.Gauges`). The readiness gauges are the follower's:
`readiness.NewRevisionGauges("graph_index")` (proposed name) adds `indexed_revision` and `target_revision` to the
shared set. Metric names keep the pin's apart from the namespace (`semstreams_` → `semengine_`, as every port did).

## D11. Proving tests

Unit tests run under `-race` and five times shuffled at one CPU; integration tests use `natsfixture` (#195's
graph-index line). Ported pin tests keep their oracles; the 24 sleeps and 11 skips go (harness rules), each skip's
test either rewritten against a real server or dropped with its reason in a task note.

- `TestGraphIndexLifecycleSuite` (integration): `lifecycletest.Run` through an `Observe` adapter listing what
  graph-index itself holds (the pool, the coalescer, the subscriptions); failing factory: `ENTITY_STATES` absent. The
  follower's watcher is private to `graph/follower` and is not listed (no test-only export, #186): after `Stop`, the
  test reads from the server that `ENTITY_STATES`'s backing stream has no consumer left, and the follower's own
  `TestFollowerRunReturnsAndLeavesNothing` covers its goroutines.
- `TestFollowerRunReturnsAndLeavesNothing` (`synctest`): after `Run`'s context ends, `Run` returns nil and no goroutine
  or ticker remains.
- `TestFollowerPublishesOnEveryHeartbeat` (`synctest`): envelopes at 0 and each `DefaultHeartbeat`.
- `TestFollowerSnapshotEndSetsBootstrapTarget` (integration, the nats.go v1.54.0 probe for the non-empty end marker):
  N entries, the target equals the highest revision at the marker; latch set when the floor reaches it, never before.
- `TestFollowerLatchesResetOnUnreadableSnapshotValue` (integration): an undecodable value written before `Open` latches
  reset; `QueryReady` is fatal `graph_state_reset_required`; `Health` false naming it.
- `TestFollowerDeclaresLostFeed` (integration, against the real triggers, option (a) of the Fable read's H2): during
  `Run`, delete `ENTITY_STATES` and wait, bounded, for an envelope reading `degraded` and `Stuck` naming the lost
  feed (the close arrives within two activity checks, 20 s; measured 18.00 s); a second case deletes and recreates the
  bucket and waits one heartbeat for the same, through trigger (iii). `QueryReady` then answers `index_not_ready`.
  Chosen over (b), a reader interface in `Config`: (a) needs no new production seam and uses the trigger an operator
  actually causes; its cost is about 20 s of integration time. Written failing first (ADR-108 D4).
- `TestStopWritesAcceptedWork` (integration): entries the pool accepted before `Stop` are in the index buckets when
  `Stop` returns nil, and `ErrorCount` is unchanged.
- `TestQueryDuringStartupReplayIsIndexNotReady` (integration): each of the eight verbs during the replay answers class
  transient, code `index_not_ready`; after the latch, the same request succeeds.
- `TestLatchBootstrap_ContinuousWrite` (ported, `pin:…/watermark_test.go:271`): the latch sets at the fixed target
  while writes continue (ADR-109 D4).
- `TestIntegration_KVRevisionAndIndexedRevisionShareOneSpace` (ported, `revision_space_integration_test.go:38`).
- `TestDataFlowCountsEachDeliveryBeforeCoalescing` (integration, Codex review M1): with `coalesce_ms` above zero,
  three quick updates of one entity with values of different sizes raise `Messages` by three and `Bytes` by the sum of
  the lengths the test wrote, and the index holds the last value's statements.
- `TestHealthIgnoresErrorCountAndTracksStuckStates`: a failed write lowers `Healthy` and its repair raises it; a
  nonzero `ErrorCount` alone reads healthy (ADR-109 D2 asks each port for both).
- `TestSecondStartIsRefused`, `TestStartContextEndStopsNothing` (also in the suite), `TestUnknownConfigKeyIsRefused`,
  one test per kept config key.
- `TestQueryResponseCarriesIndexedRevisionAndProducer` and the empty-producer refusal (D18's names), in `graph`.
- `TestGraphIndexServesExactlyTheDeclaredVerbs` (integration), as graph-ingest's.
- `TestComputeIndexStatus*` (ported from `pin:graph/index_status_test.go`), in `graph/readiness`.
- Replacement through every read (developer contract `:158`): the pin's `replacement_reconcile_integration_test.go`
  and `predicate_layout_smoke_integration_test.go`, ported; `[A] -> [B] -> []` on outgoing, incoming, predicate (exact,
  value, list, stats, compound) and name. Alias: D12.

Generated checks: the pin's `reconciliation_prop_test.go` with its reference model (`reconciliation_model_test.go`)
ports, recording seed and completed checks. Mutation rows (`task mutate:check`, unit tests only, `notes/6.1.md`): M1
`QueryReady` skips the failed check; M2 the latch uses the live target; M3 the validating phase skipped; M4 the lost
feed not marked; M5 `Healthy` reads `ErrorCount`; M6 completion skipped on a failed write (watermark pins). Each names
the test expected to fail.

Uncovered case, recorded as such in `notes/6.1.md`: `indexed_revision` is read before the handler's first bucket read
(D6). Showing that the order matters needs a write to land between the two reads, a timing only a test hook could
choose (#186), so no test forces it and there is no mutation row for it; review checks the order. The reply's revision
is still tested as a value (`TestQueryResponseCarriesIndexedRevisionAndProducer`, and an integration test that a reply
after a write at R carries at least R once the floor reaches R).

## D12. The alias index (G8; ruled: keep and file, #415)

At the pin a dropped or changed alias, and a deleted entity's aliases, keep resolving: `ALIAS_INDEX` is `alias →
entity` with no owner axis, and "production removal is incomplete" (`pin:docs/adr/068…:34-36`; ADR-077 `:44`, `:188`
left it for later). The developer contract (`:158`) requires `[A] -> [B] -> []` on every read a query can see, and the
alias query is one.

Who meets it: no consumer calls `graph.index.query.alias` directly, but semsource wires `graph.query.entityByAlias`
(`semsource:cmd/semsource/run_test.go:261`, `cmd/semsource/subject_ownership_test.go:44` at `e4febc0d`), which
graph-query (PR 9) answers by asking graph-index's alias verb and taking a hit as its answer, with no check that the
entity still carries the alias (`pin:processor/graph-query/query.go:157-175`; resolver `entity_resolver.go:14`,
`router.go:26`). Once PR 9 lands, an entity that dropped an alias is still returned for it with no error, and a deleted
entity's alias resolves to an ID that no longer exists.

**Ruled** (owner, PR #414 comment 6426254115, item 3): answer (b). graph-index keeps the pin's alias behaviour; the
integrity gate (developer contract `:158`, `[A] -> [B] -> []` through every public read) is waived for the alias read
alone until #415 lands. #415 is filed `status:blocked` and linked from this PR without a closing keyword. Its two
conditions: it asks the architect for the alias key layout ADR-077 left open, as a decision record; and the fix lands
as its own pull request before PR 9 (`processor/graph-query`), not inside it. The `graph-index` spec's replacement
requirement names alias as the exception, with #415.

## D13. #108: measure, record, decide later

Kept: rebuild on every start (`WatchAll` replays every key), as at the pin. Task 1.6 runs the pin's
`owner_filter_load_integration_test.go` load shape at the pin and on the port, recording entities, KV writes and the
`index_not_ready` window on #108 and in `notes/1.6.md`. A resume point or skip-unchanged writes is new mechanism and
waits for the owner's decision on #108 with that number. #108 stays open.

## D14. Ledger and decision records

New rows: `processor/graph-index` (`adapt`; adaptations: D3-D10; `known_risks`: G8 or its fix, S4-S6, S8 kept,
`lastProjections` memory, rebuild per start (#108)); `graph/follower` (tree code, `adapt` of the pin's graph-index loop,
source `processor/graph-index`); and one row per carried ADR. Edited: `graph` (QueryResponse returns; QueryVerbs adds
graph-index), `graph/readiness` (ComputeIndexStatus, revision gauges), `pkg/revlag` ("no importer until graph-index"
goes), `internal/dispatch` (adopter).

Carried into `docs/adr/` with a port note (path, pin SHA, packages, ledger row): ADR-077 and ADR-078 (graph-index's
index layouts and owner filters, which its code implements) and ADR-074 (the sticky reset graph-index and the follower
implement, `:69-71`). Not carried, with the reason: ADR-065 (superseded for PREDICATE by 078; its hashed NAME key is
in 077's matrix), ADR-068 (its guardrail is `graph/kvcatalog`'s and the `AGENTS.md` bucket rule; its alias line is
cited in D12), ADR-060 (the classified error mechanism is natsclient's, ADR-112), ADR-072 (the lanes are
`internal/dispatch`'s, ported without it), ADR-081 (governs `pkg/graphview`, PR 7; its "pipelines stay raw `WatchAll`"
is kept here), ADR-082 (replaced for consumers by ADR-109 D4), ADR-048 (stale at the pin).

## D15. Spec deltas

- New `graph-index`: configuration; the reply envelope on every verb; the startup-replay refusal; reset fatal;
  Health's stuck states; DataFlow inputs; second Start refused; replacement through every read except alias (#415);
  the lost-feed refusal; Stop writes accepted work.
- New `entity-state-follower`: Open's bounded wait and refusals; Run's shape; observe-before-deliver; the validating
  phase; the bootstrap target and latch; the lost feed; the reset latch; the envelope rules of D3.
- `graph-readiness` ADDED: `ComputeIndexStatus`'s projection; revision gauges.
- `graph-transport-boundary` MODIFIED: "Reserved request subjects have one declaration" and "The responder serves
  exactly its declared verbs" generalised to each responder; ADDED: "Index replies carry the revision they reflect".

## D16. Docs and records

`processor/graph-index/README.md` and `doc.go` (five buckets, not four; `batch_size` gone; the reply envelope; the
startup refusal); `graph/follower/doc.go`; `docs/repository-map.md` rows; developer contract `:155-158` and reviewer
contract `:226-228` drop "none is written in this tree yet"; the `query-pattern` skill gains graph-index's verbs;
ADR-108 Status gains a note (follower implemented; the other adopters later); comments on #109, #107, #110, #103,
issues #106, #68, #195 and #196 ("implemented in PR #414 at `<SHA>`, pending merge"), read back.

## D17. Issues and merge order

Retires: none by itself. Each named issue spans more than graph-index, so each gets `Addresses`, never `Closes`:
issue #107 (graph-index half; `embedding.Worker` is PR 5), #109 (follower lands; adopters in changes 5 and 7; ADR-108
Status
keeps it open), #110 (item 1 done; items 2-3 elsewhere), #103 (envelope producer; `min_revision` and generation stay),
issue #106 (graph-index's verbs; the boot refusal with graph-query), #68 and #195 and #196 (graph-index's lines).
Stays: #108 (D13), #408 (`internal/revlag`'s test proof; optional under Bounded port scope, since it adds no guarantee
graph-index relies on and the watermark's own property test already runs against the pin's code; it can ride this PR
only on the owner's word), #300 (graph-ingest's sections; this PR touches no graph-ingest file), #415 (the stale alias,
G8; its own PR before PR 9, ruling item 3). #103 gets a note on the bucket's history depth (task 6.3).

Adopter effect through #16's check: `composition/analyze.go:123-142` refuses any stream filter that captures a subject
in `graph.QueryVerbs()` (`reservedSubjects`, `:127-131`). Adding the eight `graph.index.query.*` verbs widens that
refusal with no code change: a composition with a stream on `graph.>`, `graph.index.>` or `graph.index.query.*` that
boots today fails at boot after this PR, naming the subject (#16's 2026-10-24 drift note asks this inventory to say
so). Searched at pinned SHAs: semconnect `deploy/semstreams.json` and `conformance/compose.semstreams.config.json`
(`d0d06e00`) and semsource `cmd/semsource/run.go` (`e4febc0d`) declare no such stream; semsource's
`graph.index.query.*` (`run.go:892`) is a request port, not a stream filter. Comment on #16, task 6.3.

Merge order, on behaviour: #413 (draft, no files yet) moves flow-path reachability into `composition`, beside the
capture check this PR widens. Neither depends on the other. Whichever merges second merges `origin/main` and re-runs
`composition`'s tests, which then exercise the widened reserved set; #413 must read reserved subjects from
`graph.QueryVerbs()` and not re-spell them (`TestReservedSubjectsDeclaredOnce` enforces it). This PR merges before PRs
8 (`graph/clustering`) and 9 (`processor/graph-query`), which read its buckets and verbs.

## D18. Decisions kept, changed, dropped

Kept: ADR-066 (watermark, LastSeq target, stuck detector in the envelope), ADR-083 (envelope on `GRAPH_STATUS`),
ADR-084 D2 and D4, ADR-085's gate as ADR-109 narrowed it, ADR-088 (one key), ADR-109 D1-D4, ADR-113, ADR-095, ADR-058,
ADR-074 `:69-71`, ADR-077, ADR-078, ADR-081's raw-watch rule. Applied, already decided: ADR-108 D3(a) second Start
refused; D4 validating phase and declared lost feed; ADR-109 D2 error count leaves `Healthy`; ADR-109 D1's second
meaning of `index_not_ready` (a watch that cannot bind or closes) applied to the lost feed (ruling item 2). Changed,
and accepted by the owner's ruling (PR #414 comment 6426254115):

- ADR-108 D4's plug-in contract (`docs/adr/108-processor-shell.md:111-112`): the plug-in does not return its outcome
  from delivery; outcomes come back through `Complete`, `Failed` and `Reset`, because graph-index's coalescer and keyed
  pool make apply asynchronous (D3).
- graph-index's envelope carries `failed_count` (pin: 0) (D3).
- `DataFlow` counts inputs, not writes (D4).
- `batch_size` is refused: semconnect's two configs fail at boot until the line is deleted; semteams' published schema
  lists it (D9).
- Eight more reserved subjects: a stream capturing `graph.index.query.*` is refused at boot (D17, #16).

- Stop drains accepted pool work in graph-ingest's order (D4).
- A recreated `ENTITY_STATES` stream is declared a lost feed (trigger (iii), D3), new beside the pin's two paths.

Dropped: the dead writers (D8). Waived: ADR-068's alias exception is kept for the alias read alone until #415 (ruling
item 3).

## Rulings

Owner ruling, PR #414 comment 6426254115 (2026-10-26), adopting the Fable design read (comment 6426199810):

1. One pull request, with a waiver by count for about 4,660 pin lines (about 3,800 in the tree) (D1).
2. Queries are refused with transient `index_not_ready` while the feed is lost; health false and the envelope
   `degraded` until restart; nothing rebinds; the lost-feed test uses the real trigger (D3, D5, D11).
3. The alias index keeps the pin's behaviour; #415 carries the fix, with an ADR for its key layout and its own pull
   request before PR 9 (D12).

Settled by records, not by the ruling: the second Start refusal (ADR-108 D3(a)); the error count leaving `Healthy`
(ADR-109 D2); context ownership (ADR-113); `min_revision` waiting for graph-query (ingest-kernel D18).
