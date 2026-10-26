# Design: setup-04a-04-graph-roots-graph-index

Base `39a17a9`. Inventory: `inventory.md` (not yet reviewed; drafted in the same pass at the caller's direction).
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
    Key             string        // GRAPH_STATUS key, readiness.KeyGraphIndex
    Subsystem       string        // Prometheus subsystem, "graph_index"
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
  the refusal's reason, and the entry is still delivered (ADR-108 D4; the pin had no validating phase, G6). At the end
  marker the bootstrap target is the highest revision observed.
- A `WatchAll` that fails, or an `Updates()` channel that closes while `Run`'s context is live, sets the *feed lost*
  state: `Stuck` names it, the envelope reads `degraded` from the next tick, and `Run` keeps publishing until its
  context ends. Nothing rebinds the watcher; the state holds until the process restarts (owner question 2). At the pin
  both paths returned with no mark (G5); ADR-108 D4 orders a failing test first.
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
- On `Stop`, work already queued in the keyed pool is processed (with the ended run context) before `Stop` returns;
  the pin's dispatcher dropped it (D7).

## D5. The readiness reader (ADR-109 D1)

`QueryReady` is the pin's `ensureQueryReady` (`query.go:208-262`), moved: reset latched → fatal
`graph_state_reset_required`; `Failed()` above zero → transient `index_not_ready` ("unresolved index write failures");
bootstrap latched → nil, at once, with no target read (the sticky path, kept: a post-bootstrap `degraded` keeps
serving, ADR-084 D7 as ADR-109 changed it); otherwise compute the envelope, latch if built, and ask
`readiness.EvaluateReadinessGate(readiness.Reading{Status: s, Fresh: true})` (the call the gate's doc names,
`graph/readiness/readiness_gate.go:80-81`): `DeferNone` → nil, any other reason → transient `index_not_ready`
("initial build has not completed"), the reason in the debug log. A lost feed refuses nothing: the reply carries
`indexed_revision`, so a caller sees how far the answer reaches (ADR-109 D4).

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
it. Keys: `ports`, `workers` (1-16, default 1; lanes), `coalesce_ms` (0 = no coalescing), `startup_attempts` (default
30), `startup_interval_ms` (default 500). Each has a test that fails when it is ignored. `batch_size` is refused as
unknown (semsource does not set it, `cmd/semsource/run.go:772-781` *(local checkout, unpinned)*).

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

- `TestGraphIndexLifecycleSuite` (integration): `lifecycletest.Run` through an `Observe` adapter listing the follower's
  watcher, the pool, the coalescer and the subscriptions; failing factory: `ENTITY_STATES` absent.
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
- `TestStopProcessesQueuedWork`: work queued in the pool when `Stop` runs is processed before `Stop` returns (D7).

Generated checks: the pin's `reconciliation_prop_test.go` with its reference model (`reconciliation_model_test.go`)
ports, recording seed and completed checks. Mutation rows (`task mutate:check`, unit tests only, `notes/6.1.md`): M1
`QueryReady` skips the failed check; M2 the latch uses the live target; M3 the validating phase skipped; M4 the lost
feed not marked; M5 `Healthy` reads `ErrorCount`; M6 `indexed_revision` read after the bucket read (expected
`TestReplyRevisionIsALowerBound`: a write landing between the two reads); M7 completion skipped on a failed write
(watermark pins). Each names the test expected to fail.

## D12. The alias index (G8; owner question 3)

At the pin a dropped or changed alias, and a deleted entity's aliases, keep resolving: `ALIAS_INDEX` is `alias →
entity` with no owner axis, and "production removal is incomplete" (`pin:docs/adr/068…:34-36`; ADR-077 `:44`, `:188`
left it for later). The developer contract (`:158`) requires `[A] -> [B] -> []` on every read a query can see, and the
alias query is one. Two answers: (a) fix it here with an owner-keyed record per alias, reconciled the way
`owner_reconcile.go` already reconciles PREDICATE, NAME and INCOMING (about 80 lines and a layout ADR-077 did not
choose); (b) keep the pin's behaviour, file the defect (`status:blocked` on this PR), and have the owner waive the gate
for this one index. Recommended: (b), because this PR is already over the size bound, the alias layout is a decision
ADR-077 deferred, and no consumer in scope calls `graph.index.query.alias` directly (searched semsource, semconnect,
semteams, semboids *(local checkouts, unpinned)*: no hit). Its one reader at the pin is graph-query's entity resolver
(`pin:processor/graph-query/entity_resolver.go:14`, `router.go:26`), which arrives in PR 9: a stale alias resolves a
name to the entity that once carried it. The filed issue names that reader, so PR 9's design meets it.

## D13. #108: measure, record, decide later

Kept: rebuild on every start (`WatchAll` replays every key), as at the pin. Task 1.3 runs the pin's
`owner_filter_load_integration_test.go` load shape at the pin and on the port, recording entities, KV writes and the
`index_not_ready` window on #108 and in `notes/1.3.md`. A resume point or skip-unchanged writes is new mechanism and
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
  Health's stuck states; DataFlow inputs; second Start refused; replacement through every read; Stop processes queued
  work.
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

Merge order: no open PR overlaps (#413 touches `service` and `composition` only). This PR merges before PRs 8
(`graph/clustering`) and 9 (`processor/graph-query`), which read its buckets and verbs.

## D18. Decisions kept, changed, dropped

Kept: ADR-066 (watermark, LastSeq target, stuck detector in the envelope), ADR-083 (envelope on `GRAPH_STATUS`),
ADR-084 D2 and D4, ADR-085's gate as ADR-109 narrowed it, ADR-088 (one key), ADR-109 D1-D4, ADR-113, ADR-095, ADR-058,
ADR-074 `:69-71`, ADR-077, ADR-078, ADR-081's raw-watch rule. Applied, already decided: ADR-108 D3(a) second Start
refused; D4 validating phase and declared lost feed; ADR-109 D2 error count leaves `Healthy`. Changed, for the owner:
graph-index's envelope now carries `failed_count` (pin: 0) (D3); `DataFlow` counts inputs, not writes (D4); `Stop`
processes queued pool work (D7, from #107's pool). Dropped: `batch_size`; the dead writers (D8). ADR-068's alias
exception: owner question 3.

## Owner questions

1. **Size: one pull request of about 4,660 pin lines (about 3,800 in the tree), or two?** Recommended one, with your
   waiver for that number: the follower and the index it feeds are one unit, and the query half without the writer
   (or the writer without its queries) is half a component. The other answer (Option D) is two reviews of about 3,630
   and 1,030 pin lines; the first is still over the bound.
2. **When graph-index loses its feed from `ENTITY_STATES`, does it stay unhealthy until restart?** Recommended yes for
   this pull request: it reports the loss (health, envelope) and an operator or supervisor restarts it, as ADR-108
   asks. Whether a broker restart closes the watcher at all is not measured (inventory E2); if a probe shows it does,
   reconnecting becomes its own issue, like graph-ingest's #216. The other answer is reconnect logic now, which is new
   mechanism with no measured trigger.
3. **The alias index keeps old aliases (D12): fix it here, or keep the pin's behaviour and file it?** Recommended keep
   and file, with your waiver for this one index: the fix needs a layout the pin's records left open, and the only
   reader, graph-query's name resolver, arrives two pull requests later. Until it is fixed, a renamed or deleted
   entity's old alias still finds it. The other answer adds about 80 lines and a layout decision to an already large PR.

Not owner questions, settled by records: the second Start refusal (ADR-108 D3(a)); the error count leaving `Healthy`
(ADR-109 D2); context ownership (ADR-113); `min_revision` waiting for graph-query (ingest-kernel D18).
