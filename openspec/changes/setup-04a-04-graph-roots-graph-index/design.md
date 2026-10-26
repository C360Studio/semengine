# Design: setup-04a-04-graph-roots-graph-index

Base `39a17a9`. Inventory: `inventory.md` (round 2, after DESIGN CHANGES REQUESTED on `ad8460a`; not yet passed).
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
  (recommended). Over the size bound; needs a waiver by count (D1, owner question 1).
- **D. C split in two: writer half (component, follower, index writes, envelope), then the query half (`query.go`,
  the by-name handler, the reply envelope, the verb table).** Still over the bound for the first half (about 3,630 pin
  lines, inventory 10), and graph-index lands without its read surface for one PR, which "ported whole" allows only
  on the owner's word.
- **E. Do nothing now.** graph-query (PR 9) and graph/clustering (PR 8) read graph-index's buckets; the change stalls.

## D1. Unit of work and size

One pull request, one OpenSpec change. Pin lines carried: about 4,660 (graph-index 4,527, plus about 130 from `graph`
and `graph/readiness` that the ingest-kernel design deferred to graph-index, inventory 10). About 900 of them do not
enter the tree (D8, D4's shell scaffolding); the tree gains about 3,800. The rule's bound is about 3,000 (#159), so this
needs the owner's waiver for that number (owner question 1). Option D is the alternative; neither split gets under the
bound.

## D2. Packages and files

- `processor/graph-index/` (package `graphindex`): the pin's files less `keyed_dispatcher.go`, `kv_contract_benchmark.go`
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

Two functions in a config, not an interface: one plug-in today, and `internal/dispatch.KeyedConfig.Process` is the tree's
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
  `Run` publishes the envelope once at once, then every `readiness.DefaultHeartbeat`.
- Every delivered revision is observed in the watermark before `Deliver` is called with it. Until the snapshot's end
  marker, each non-deleted value is decoded with `graph.UnmarshalEntityState(key, value)`; a refusal latches reset with
  the refusal's reason, and the entry is still delivered (ADR-108 D4; graph-index's pin loop had no validating phase,
  G6; spatial, temporal and embedding stop applying after a reset, so whether to apply after a reset stays the
  plug-in's: graph-index keeps indexing the other entities, as at the pin). At the end marker the bootstrap target is
  the highest revision observed.
- A `WatchAll` that fails, or an `Updates()` channel that closes while `Run`'s context is live, sets the *feed lost*
  state: `Stuck` names it, the envelope reads `degraded` from the next tick, and `Run` keeps publishing until its
  context ends. Nothing rebinds the watcher; the state holds until the process restarts. At the pin graph-index's two
  paths returned with no mark (G5); ADR-108 D4 (`:104-106`) asks only that the loss be declared, with a failing test
  first. Whether queries are refused meanwhile is owner question 2.

  The probe (task 1.1, run before the owner sees question 2): a nats-server restart on the same port and store dir
  does **not** close `Updates()` at nats.go v1.54.0 / nats-server v2.15.1. In four runs the channel stayed open, stalled,
  and resumed: a write made 0.4-1.5 s after the restart arrived 8-16 s later (about 18 s after the server stopped, both
  for a 1 s and a 10 s outage), with no snapshot replay, no second end marker and no close. In one run a write the
  client had timed out on during reconnect was stored as revision 4 and then overwritten by revision 5 before the
  watcher resumed; the watcher delivered only 5. Program `/tmp/gi-probe/main.go` (module pins the two versions), built
  `go build`, run `./probe <seconds down>`; output `/tmp/gi-probe/out-down1s.txt`, `out-down1s-run2.txt`,
  `out-down1s-run3.txt`, `out-down10s.txt` (local only; posted on PR #414 by task 1.1). Embedded server
  (`server.NewServer`, JetStream file store), client `MaxReconnects(-1)`, `ReconnectWait(2s)` as natsclient's defaults
  (`natsclient/client.go:308-309`). What follows: an ordinary broker restart is not a lost feed; it is a stall of up to
  about 18 s that nothing declares (the stuck detector needs 30 s). The lost feed fires on a `WatchAll` that fails at
  start, or a close the probe did not produce. A revision superseded during a stall is never delivered, which the
  watermark tolerates (it floors over observed revisions) and which is evidence for #103's open conflict (ADR-066
  `:408-409` against ADR-110 `:192`).
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
then subscribes the handlers. `cleanup` stops taking requests, ends the run context and joins every goroutine and the
pool within Stop's context, tolerating a partial start.

Declared behaviour changes, each from a binding record:

- A second `Start` on a running component returns `errs.ErrAlreadyStarted`; the pin returned nil (ADR-108 D3(a), G2).
- Ending Start's context after `Start` returned stops nothing (ADR-113, G1).
- `Healthy` is false only while a stuck state holds; `ErrorCount` never lowers it (ADR-109 D2, G3). The pin's delete
  and list failures lowered it until restart (`component.go:2106`, `:2134`, `:2151`); now an unresolved write is a
  stuck state that clears on repair.
- `Initialize` no longer re-validates (ADR-108 D3(d), G9).
- `DataFlow` counts inputs: one per delivered `ENTITY_STATES` entry, update or delete, with the value's bytes;
  `ErrorCount` counts each entity whose required writes ultimately failed and each reset. The pin counted index writes
  as messages at eight sites with three meanings (S7); `Messages` now means what the shell's spec says ("input
  messages counted"), as graph-ingest's #307 ruling 1 did.

Not a change: work still queued in the keyed pool at `Stop` writes nothing. `cleanup` ends the run context before the
pool's `Shutdown`, and the pool hands that context to every `Process` call (`internal/dispatch/keyed_pool.go:115-118`),
so each queued item sees an ended context and its KV calls fail. The pin's dispatcher dropped queued items; the effect
is the same, and the next start rebuilds every index (D13). No requirement promises otherwise.

## D5. The readiness reader (ADR-109 D1)

`QueryReady` is the pin's `ensureQueryReady` (`query.go:208-262`), moved: reset latched → fatal
`graph_state_reset_required`; `Failed()` above zero → transient `index_not_ready` ("unresolved index write failures");
bootstrap latched → nil, at once, with no target read (the sticky path, kept: a post-bootstrap `degraded` keeps
serving, ADR-084 D7 as ADR-109 changed it); otherwise compute the envelope, latch if built, and ask
`readiness.EvaluateReadinessGate(readiness.Reading{Status: s, Fresh: true})` (the call the gate's doc names,
`graph/readiness/readiness_gate.go:80-81`): `DeferNone` → nil, any other reason → transient `index_not_ready`
("initial build has not completed"), the reason in the debug log. A lost feed: owner question 2. As drafted it refuses
nothing and the reply's `indexed_revision` shows how far the answer reaches (ADR-109 D4). At the pin the three later
adopters refuse with transient `index_not_ready` while their watch is unavailable
(`pin:processor/graph-embedding/component.go:1727-1729`, `pin:processor/graph-index-spatial/component.go:869-871`,
`pin:processor/graph-index-temporal/component.go:899-901`); ADR-109 D1 reserves that code for the startup replay and
unresolved writes, so refusing on a lost feed widens ADR-109 and is the owner's call.

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
  read, so every revision at or below it is reflected. Refusals stay classified errors with no envelope.
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
- `TestFollowerDeclaresLostFeed`: a watcher whose `Updates()` closes (a fake reader) and a failed `WatchAll`: `Stuck`
  names feed lost, the next envelope is `degraded`, `Run` keeps publishing. Written failing first (ADR-108 D4).
- `TestQueryDuringStartupReplayIsIndexNotReady` (integration): each of the eight verbs during the replay answers class
  transient, code `index_not_ready`; after the latch, the same request succeeds.
- `TestLatchBootstrap_ContinuousWrite` (ported, `pin:…/watermark_test.go:271`): the latch sets at the fixed target
  while writes continue (ADR-109 D4).
- `TestIntegration_KVRevisionAndIndexedRevisionShareOneSpace` (ported, `revision_space_integration_test.go:38`).
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

## D12. The alias index (G8; owner question 3)

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

Two answers: (a) fix it here: an owner-keyed record per alias, reconciled the way `owner_reconcile.go` already
reconciles PREDICATE, NAME and INCOMING; its size is not measured (the earlier "about 80 lines" was an estimate and is
withdrawn), and it chooses a layout ADR-077 left open; (b) keep the pin's behaviour, file the defect (`status:blocked`,
linked from this PR without a closing keyword), and have the owner waive the integrity gate for this one index until
the filed issue lands, at the latest with PR 9. Under (b) the `graph-index` spec's replacement requirement names alias
as the exception with that issue's number. Recommended: (b), because this PR is already over the size bound and the
layout is a decision; the issue names PR 9 as the last point it can land, since that is when a consumer first sees it.

## D13. #108: measure, record, decide later

Kept: rebuild on every start (`WatchAll` replays every key), as at the pin. Task 1.4 runs the pin's
`owner_filter_load_integration_test.go` load shape at the pin and on the port, recording entities, KV writes and the
`index_not_ready` window on #108 and in `notes/1.4.md`. A resume point or skip-unchanged writes is new mechanism and
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
  Health's stuck states; DataFlow inputs; second Start refused; replacement through every read except alias, which is
  named with its issue under owner question 3's answer (b).
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
issue #107 (graph-index half; `embedding.Worker` is PR 5), #109 (follower lands; adopters in changes 5 and 7; ADR-108 Status
keeps it open), #110 (item 1 done; items 2-3 elsewhere), #103 (envelope producer; `min_revision` and generation stay),
issue #106 (graph-index's verbs; the boot refusal with graph-query), #68 and #195 and #196 (graph-index's lines).
Stays: #108 (D13), #408 (`internal/revlag`'s test proof; optional under Bounded port scope, since it adds no guarantee
graph-index relies on and the watermark's own property test already runs against the pin's code; it can ride this PR
only on the owner's word), #300 (graph-ingest's sections; this PR touches no graph-ingest file). New, filed by the
orchestrator: G8 if owner question 3 is answered (b).

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
refused; D4 validating phase and declared lost feed; ADR-109 D2 error count leaves `Healthy`. Changed, for the owner:

- ADR-108 D4's plug-in contract (`docs/adr/108-processor-shell.md:111-112`): the plug-in does not return its outcome
  from delivery; outcomes come back through `Complete`, `Failed` and `Reset`, because graph-index's coalescer and keyed
  pool make apply asynchronous (D3).
- graph-index's envelope carries `failed_count` (pin: 0) (D3).
- `DataFlow` counts inputs, not writes (D4).
- `batch_size` is refused: semconnect's two configs fail at boot until the line is deleted; semteams' published schema
  lists it (D9).
- Eight more reserved subjects: a stream capturing `graph.index.query.*` is refused at boot (D17, #16).

Dropped: the dead writers (D8). Open for the owner: ADR-068's alias exception (question 3); widening ADR-109 D1's
`index_not_ready` to a lost feed (question 2).

## Owner questions

1. **Size: one pull request of about 4,660 pin lines (about 3,800 in the tree), or two?** Recommended one, with your
   waiver for that number: the follower and the index it feeds are one unit, and the query half without the writer
   (or the writer without its queries) is half a component. Your order ruling itself measured graph-index at 4,527
   lines and listed it, with the follower, as one item (#9 comment 6386856657, item 3). The other answer (Option D) is
   two reviews of about 3,630 and 1,030 pin lines; the first is still over the bound.
2. **When graph-index has lost its feed from `ENTITY_STATES`, should its queries be refused as "not ready", or keep
   answering?** The probe (D3) shows an ordinary NATS server restart does not lose the feed: the watch pauses for up to
   about 18 seconds and then catches up by itself. A lost feed only happens when the watch cannot start, or ends for a
   reason the probe did not produce; it is reported in health and the status record and lasts until restart.
   Recommended: refuse with the retryable "not ready" code while it lasts, as the pin's embedding, spatial and temporal
   indexes do, so nobody reads a frozen index as live. This widens ADR-109, which keeps that code for startup and
   unrepaired writes. The other answer keeps answering, with the reply's revision showing how stale it is.
3. **The alias index keeps old aliases (D12): fix it here, or keep the pin's behaviour and file it?** Recommended keep
   and file, with your waiver for this one index, the fix landing no later than graph-query (PR 9). Until then nothing
   reads it; from PR 9 on, semsource's look-up by alias would return an entity that dropped the alias, or the ID of a
   deleted entity, with no error. The other answer fixes it here: unmeasured size, plus a key-layout decision the
   pin's records left open, in an already large PR.

Not owner questions, settled by records: the second Start refusal (ADR-108 D3(a)); the error count leaving `Healthy`
(ADR-109 D2); context ownership (ADR-113); `min_revision` waiting for graph-query (ingest-kernel D18).
