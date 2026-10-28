# Design: setup-04a-04-graph-roots-embedding

Base: main including #455 (`f92753b`); the branch merges it at `6b146a8`. The inventory was taken at `08e5d9d`; #455
touched only `graph/follower`, whose drain D4 now reuses.
Inventory: `inventory.md` (round 2, `INVENTORY PASS`). Change 4, PR 6 of 9 (#9 comment 6386856657, item 5). Tier 0
only (#9 comment 6386603659). Draft PR #458. Owner rulings for this change: PR #458 comment 6473224284 (#35 retired
here; #107 retired on the D6 finding, or the Worker adopts the keyed pool). Codex's design review of `5eb19e8`
(PR #458 comment 6474241300) showed the finding does not hold, so the Worker adopts the keyed pool (D6) and #107 is retired
by doing its work. Governing records: ADR-066, ADR-063,
ADR-084, ADR-085 (binding, ported), ADR-110 and ADR-113 (SemEngine's), ADR-071 (keep predicate), inventory 6. Probes:
inventory 7 (P1-P14), posted on PR #458 comment 6472856197.

Coined terms: *hop 1* is `processor/graph-embedding` writing a pending record per entity revision; *hop 2* is the
Worker turning a pending record into a generated or failed one; the *revision guard* is the rule that a hop-2 save for
an older source revision never replaces a newer record (`pin:graph/embedding/storage.go:396`, `:455`, `:465`); hop 1's
pending write refuses only to replace a generated record at the same or a newer revision (`:313`), so an older pending
record may replace a newer pending one, as at the pin; a
*terminal outcome* is a record leaving pending (generated, failed, deleted, skipped).

## Options

- **A. One pull request, the package minus `http_embedder.go` (chosen, ruled).** 3,082 pin lines, under the waiver
  at 3,302 (#9 comment 6386556850, ruling 1).
- **B. Storage first, then the Worker.** Builds (Storage does not import the Worker), but the owner ruled one PR.
- **C. Carry the pin's `Start`/`Stop` and hand loops unchanged.** Fails the `background-work` spec (binds every later
  port) and ADR-110 Decision 7 (binds these three loops at this port, ruled on #141).
- **D. Do nothing now.** `graph/query` (item 6) cannot build.

## D1. Files

`graph/embedding/`: `doc.go`, `README.md`, `embedder.go`, `bm25_embedder.go`, `dedup.go`, `vector.go`, `storage.go`,
`vector_cache.go` (new file, the cache split out of `storage.go`, D5), `worker.go`, `derived_reasons.go`, tests. Not
carried: `http_embedder.go`, `http_embedder_test.go`, `http_embedder_dimensions_test.go`.

Outside the package (D4, D11): `internal/kvwatch` (new) receives the body of `graph/follower`'s `stopWatch`, its
5 s budget and its delivery-open error, with bucket-free text (`kvwatch.Stop`, `kvwatch.ErrDeliveryOpen`); each caller
wraps the error with its own bucket. `graph/follower` keeps `stopWatch` as that wrapper ("follower: … ENTITY_STATES
watch", as today) and keeps `TestStopWatchGivesUpOnADeliveryThatNeverCloses` under that name, matching through
`errors.Is` (`follower_test.go:81`), so the `entity-state-follower` spec needs no delta. `internal/harness/natsfixture`
receives the follower test's `nativeDeliveryStacks` and `awaitNativeDeliveryBlocked`, and the follower's tests call
them. One spelling of each. The drain does not go into `natsclient`: that would add mechanism to a public package
against the owner's #170 ruling; `natsclient`'s own copy in `keysByFilter` (`natsclient/kv.go:699`, kept on #420) is
the second spelling D11's issue covers.

`internal/dispatch` gains `(*KeyedPool).Wait()`: it returns once every lane has exited, which happens only after
`Shutdown` has begun, so a `Wait` called earlier blocks until then; it takes no context and adds no bound. It is the
join the Worker's `Run` needs after `Shutdown(ctx)` with an ended `ctx` (D4), the part the pin's `wg.Wait` played. One
caller (the Worker); graph-index and graph-ingest keep calling `Shutdown` under their `Stop`'s live context, unchanged.
Its requirement is a delta to the `keyed-dispatch` spec.

## D2. Surface

Ports as at the pin: `Embedder`; `BM25Embedder`, `BM25Config`, `NewBM25Embedder`; `EmbedderIdentity`, `DedupKey`,
`InProcessDedupKey`; `CosineSimilarity`; `Status` and its three values; `Record` (less `ContentFields`), `StorageRef`,
`DedupRecord` (less `EntityIDs`), `ScoredEntity`, `ScoredEntityLess`, `FailedEntry`; `ErrRecordGone`,
`ErrSupersededRevision`; the `Storage` methods `SavePendingGuarded`, `SaveGenerated`, `SaveFailed`, `GetEmbedding`,
`GetByContentHash`, `SaveDedup`, `DeleteEmbedding`, `ListGeneratedEntityIDs`, `ScanFailed`; `MaxSourceTextLenCeiling`,
`GeneratedCallback`, `TerminalOutcome` and its four values, `TerminalCallback`, `WorkerMetrics` (less `SetPending`,
plus `IncContentUnresolved`, D7), `StoreResolver`, `NewWorker` and its seven `With*` setters; `Reason*` (3).

Changed signatures, each called only by `processor/graph-embedding` (change 5, edited at its port):

```go
func NewStorage(index, dedup *natsclient.KVStore) *Storage            // was two jetstream.KeyValue (D3)
func NewWorker(storage *Storage, embedder Embedder, logger *slog.Logger) *Worker // indexBucket dropped (D4)
func (w *Worker) Run(ctx context.Context) error                        // replaces Start(ctx) and Stop() (D4)
func NewVectorCache(storage *Storage) *VectorCache                     // replaces StartVectorCache (D5)
func (c *VectorCache) Run(ctx context.Context) error
func (c *VectorCache) FindSimilar(excludeID string, query []float32, keep func(string) bool, limit int) ([]ScoredEntity, bool)
```

Not carried (surface audit (a)/(b)/(c), inventory 4 and 9): `ContentHash`, `SavePending`, `SavePendingWithStorageRef`
(no production reader; 48 test sites move to `SavePendingGuarded`), `Record.ContentFields` (S14), `DedupRecord.EntityIDs`
(S13), `WorkerMetrics.SetPending` (S10b), `euclideanDistance`, `dotProduct`, `magnitude`, `normalize` (S10),
`ErrCASExhausted` (D3), the `HTTP*` types. Kept with a reason: `Record.GeneratedAt` and `DedupRecord.FirstGenerated`
have no code reader but are what an operator sees when listing the buckets (`service/message_logger_http.go:351`).
Kept, optional (S20): the exported `Storage` methods whose only readers are in the package.

## D3. Writes: one read-modify-write primitive (ADR-110)

`Storage` holds two `*natsclient.KVStore` and writes every read-modify-write through `UpdateWithRetryRead`, whose
callback decides from the bytes and revision it read (ADR-110 Decision 2; Decision 7 names `storage.go:308`, `:367`,
`:438`; ruled on #141, comment 6097826823). Observable behaviour, each the pin's except where marked:

- `SavePendingGuarded` writes the pending record unless the stored record is generated at the same or a newer source
  revision; then it writes nothing and returns `saved == false`. An absent key, a delete or purge marker included, is
  created (P3).
- `SaveGenerated` and `SaveFailed` return `ErrRecordGone` and write nothing when the key is absent; return
  `ErrSupersededRevision` when the stored record carries a newer source revision; `SaveFailed` also returns
  `ErrSupersededRevision` over a generated record at the same revision. Both errors match through `errors.Is`.
- `SaveDedup` keeps a stored record from the same vector space (model and dimensions) and writes nothing; otherwise it
  writes the new vector. **Changed:** the pin rewrote the record unconditionally with an extra entity ID (S12, S13).
- Retry exhaustion returns an error matching `natsclient.ErrKVMaxRetriesExceeded`; the retry budget is the KVStore's
  (`natsclient/kv.go:36-46`), set by whoever builds it. **Changed:** the pin's own budget of 3 and `ErrCASExhausted`
  go; the Worker treats exhaustion as the pin treated `ErrCASExhausted`: the record stays pending and the outcome is
  not terminal (`pin:…/worker.go:814-818`).
- The error-text revision check (`storage.go:501-502`, S6) goes with the loops; conflict detection is natsclient's.
- **No text: delete only what was read.** When the text the Worker resolves for a record is empty, it removes the
  record only if the key is still at the KV revision the Worker read it at, through the existing
  `natsclient.KVStore.DeleteAtRevision` (`natsclient/kv.go:540`; one fenced attempt, no retry), behind an unexported
  `Storage` method, so no exported surface is added. Outcomes: removed → `OutcomeSkipped`, terminal, as at the pin;
  refused because the key moved or is gone (`ErrKVRevisionMismatch`, `ErrKVKeyNotFound`) → newer work owns the entity,
  nothing is removed, `OutcomeDeleted`, terminal (like a superseded save); any other error → nothing is reported as
  done: not terminal, the record stays pending, and the error is logged. **Changed:** the pin deleted the key
  unconditionally and reported success even when the delete failed (`pin:…/worker.go:555-563`), so an older
  revision with an empty offloaded body could delete a newer revision's pending record or generated vector (Codex
  review, PR #458 comment 6474241300, finding 1; reachable through hop 1's storage-reference path,
  `pin:processor/graph-embedding/component.go:1827`, `:2025`). The keyed pool (D6) does not prevent this alone: hop 1
  writes M while N runs. `DeleteAtRevision` refuses a stale revision and removes a current one
  (`natsclient/kv_integration_test.go:205-219`); the moved-key and deleted-key cases are this change's no-text tests.
- Reads keep their pin results through the KVStore: `GetEmbedding` and `GetByContentHash` return `(nil, nil)` for an
  absent key; `DeleteEmbedding` of an absent key returns nil (P5). `ListGeneratedEntityIDs` returns every key, pending
  and failed included, as at the pin (its doc is corrected; the caller filters by status). **Cost:** it now lists
  through `KVStore.Keys`, bounded by the KVStore's `Timeout` option (default 5 s), where the pin streamed under the
  caller's context; whoever builds the KVStore sets the bound for its bucket size. The same `Timeout` option bounds
  every read and every `UpdateWithRetryRead` call, retries included (`natsclient/kv.go:353`), as for every KVStore
  user in the tree. Declared here, not a question.

## D4. The Worker runs until its context ends (`background-work`, ADR-113)

Settled records, applied: the `background-work` spec (`Run(ctx)` preferred; the Worker calls caller callbacks, so
`Close` does not qualify) and ADR-113 (the component that runs the Worker creates the run context in its `Start` and
ends it in `Stop`; this package derives nothing from a Start context). The foundation design's adapt row for
`Worker.Stop()` (`setup-04a-foundation` D6/D7) is met by this shape, as `graph/readiness.Watcher.Run` met its row.

- `Run(ctx)` refuses a nil context with an invalid-input error before starting anything. A second `Run` on the same
  Worker while one is running is refused; after `Run` returns, `Run` may be called again and starts a new watch,
  whose snapshot re-processes failed records once, as a pin restart did.
- It opens one watch on `EMBEDDING_INDEX` under a context it derives from `ctx`, reads it on its own goroutine, and
  hands each put to a keyed pool (`internal/dispatch.KeyedPool`, D6) under the entity ID, with `workers` lanes
  (default 5, floored at 1) and a bounded queue per lane; a full lane makes the read wait. Whether an entry arrived
  during the snapshot is fixed when `Run` reads it, so the pin's per-worker race on that flag (`worker.go:423-433`)
  is gone. A watch or pool that cannot start returns its error, nothing left running.
- It ends when `ctx` ends or when the watch's update channel closes while `ctx` is live (the lost watch). In this
  order, as observable behaviour:
  1. It stops reading and cancels the derived context: a read waiting on a full lane returns, and nats.go's own
     unsubscribe goroutine for the watch (`js.go:2063-2068`) is released (not joined).
  2. It calls `pool.Shutdown(ctx)` only to begin the drain (with `ctx` ended it starts the drain and returns
     `ctx.Err()` at once, `internal/dispatch/keyed_pool.go:284-310`; its return is not the join), then joins the lanes
     with `pool.Wait()`, a context-free wait added to `internal/dispatch` that returns once every lane has exited
     (D1). Items already accepted are processed first, under `ctx`; after a cancel each ends quickly through the
     shutdown path (D7), as at the pin. The join has no bound of its own, as the pin's `wg.Wait` had none and the
     `background-work` spec's `Run` requires it: a caller callback that blocks holds `Run`, and the composing component
     bounds its own wait for `Run` in its `Stop(ctx)` (ADR-113). No detached context is created.
  3. It stops the watch through `kvwatch.Stop` (D1), which reads the update channel until nats.go closes it, ending
     a delivery blocked on a full buffer (the backlog at v1.54.0 that `graph/follower`'s
     `TestFollowerRunReturnsAndLeavesNothing` drives), for at most 5 s after `ctx` ended, the bound #454 needs.
     Entries read and dropped here stay pending in the bucket and come back in the next `Run`'s snapshot.
- It returns `ctx.Err()` when `ctx` ended, an error saying the watch was lost on the other path (**changed:** at the
  pin the goroutines returned silently and the Worker consumed nothing while healthy), joined with the drain's error
  when the delivery did not close. A stop error that says only that the watch was already gone is not reported
  (`kvwatch.Stop`'s rule, the follower's). **Changed:** no error-text fallback (S7).
- So when `Run` returns, no goroutine it started is running, no nats.go delivery for its watch is running except in
  the #454 window (where nats.go left none to join and `Run` says so), and nats.go's unsubscribe goroutine has been
  released.
- The `stopping` field goes (S18). The pin's per-entry panic recovery (`handleKVEntrySafe`) stays inside the pool's
  work function, so a panic keeps the record pending and fires no terminal callback, as at the pin.

## D5. The vector cache is its own background work

`VectorCache` holds the in-memory copy of generated vectors that `Storage` held at the pin. `Run(ctx)` refuses nil,
refuses a second `Run` while one is running (as the Worker's, D4), watches `EMBEDDING_INDEX` through the Storage's
index KVStore on the caller's goroutine, and ends exactly as D4's `Run` does: on `ctx` ending or a lost watch, it
cancels its derived context and stops the watch through `kvwatch.Stop`, returning `ctx.Err()` or a lost-watch error.
`FindSimilar` returns `(nil, false)` before the watch's snapshot has been applied, after a record failed to decode,
after the watch was lost, and after `Run` returned; the caller then reads KV, as at the pin. Results are sorted by
`ScoredEntityLess` before the limit; `keep` filters before scoring (ADR-071). **Changed, optional and cheap:** a limit
of 0 or less returns an empty result instead of panicking on a negative slice bound (S21).

## D6. Ordering per entity: the keyed pool (#107)

Owner ruling item 2 (PR #458 comment 6473224284): retire #107 on a finding if the revision guard gives the ordering
that matters, otherwise adopt the keyed pool. Codex's review (PR #458 comment 6474241300, finding 2) shows it does
not: with the pin's shared channel, revision N's failure can be saved, its terminal callback can pause, revision M's
success can be saved and its callback can remove the component's failure entry, and N's late callback then puts the
obsolete failure back (`pin:processor/graph-embedding/component.go:1125-1155` checks the revision only against an
entry that exists). The guard orders what is stored, not what is reported.

So the Worker adopts `internal/dispatch.KeyedPool` (`internal/dispatch/keyed_pool.go`; `pkg/dispatch` is its pin
path), keyed by entity ID, and the work handed to it includes the terminal callback. Observable behaviour:

- One entity's entries are processed one at a time, in the order the watch delivered them, from reading the record
  through the store write to the terminal and generated callbacks; different entities run in parallel across lanes
  (`keyed-dispatch` spec, "Same-key work runs in submit order on one lane"). Nothing here depends on the watch's
  delivery order for one key: if the watch ever delivered M before N, N's save is checked against the record stored
  when it runs, and is refused (reporting `OutcomeDeleted` or `OutcomeSkipped`) only when that record is generated at
  a newer revision, as below. Delivery order is not promised here.
- So processing and callbacks for one entity follow the order of the watch entries the Worker accepted, and each
  hop-2 save is checked against the source revision stored when the save runs: a save older than a stored generated
  record is refused, and its callback reports `OutcomeDeleted` or `OutcomeSkipped`, never `OutcomeFailed`
  (`markFailed` returns `OutcomeFailed` only for a stored failure, `pin:…/worker.go:1174-1178`). This is not a promise
  that stored source revisions only move forward: D3 keeps the pin's choice that an older pending write replaces a
  newer pending or failed record, so the sequential history Pending(M), Failed(M), Pending(N), Failed(N) with N < M
  ends at N's failure. Only a generated record blocks an older write.
- The revision guard (D3) stays: hop 1 writes pending records while the Worker runs, so a hop-2 save still races a
  newer write, and the guard refuses a save older than the generated record stored when it runs. The keyed pool does
  not replace it, and it does not cover hop 1.
- Not designed here: the failed map in `processor/graph-embedding` (change 5). Its port must still refuse to put back
  a failure older than the entity's latest terminal outcome; the `graph/embedding` ledger row's `known_risks` says so
  (D8).

Issue #107 is retired by this work: the graph-index half merged in #414, and the Worker half is this pull request's.

## D7. Smaller corrections

- **Metrics.** `WorkerMetrics.SetPending` goes (no caller; S10b). `IncContentUnresolved` joins `WorkerMetrics`, so an
  adapter cannot silently miss it through the pin's type assertion (S19; optional, taken: the interface is edited
  anyway). The package still imports no Prometheus code; the component registers collectors (change 5).
- **Shutdown is classified by the error, not its text.** A failure counts as shutdown only when its error matches
  `context.Canceled` and the Worker's context has ended. **Changed:** the pin searched the message for "context
  canceled" (`worker.go:1168`, S8; no tier-0 trigger, optional, taken because a product `Embedder` can trigger it).
- **One text-length default.** A cap of 0 means 8,000 runes on both lanes; at the pin the inline lane read 0 as
  unlimited (S5). Unreachable through the component, which always sets a cap; optional, taken with the doc rewrite.
- **Docs.** `doc.go`, `README.md` and comments drop the HTTP embedder, the dead `graph/indexmanager` link and the
  SemStreams rolling-upgrade notes (S16, S17; owner 2026-10-11, #91 comment 6109293267: no SemStreams state is
  migrated); `BM25Config.B`'s doc says 0 means the default 0.75 (S15).
- **Callback order.** `GeneratedCallback`'s and `TerminalCallback`'s docs say callbacks for one entity arrive in the
  order its entries were processed (D6) and that a refused save fires no generated callback.
- **Kept, optional:** `classifyEmbedErr`'s substring classes (S9), for a product's own `Embedder`.

## D8. ADRs, ledger, docs

- Ledger: new row `graph/embedding`, `adapt`, naming D2-D7 and the dropped surface; new row
  `graph/embedding/http_embedder.go`, `defer-exclude`, destination none, `known_risks` naming #32 and the 04C fallback
  test (owner ruling item 1; #9 comment 6386603659). The `graph/embedding` row's `known_risks` also says that
  change 5's failed map must not put back a failure older than the entity's latest terminal outcome (D6), and that
  `processor/graph-embedding`'s port (change 5) must state its reaction when `Worker.Run` or `VectorCache.Run` returns
  a lost-watch error while its run context is live (report it through `Health`, or restart the work), so the error
  is never dropped.
- ADR-066 (`:18`) and ADR-063 (`:13-15`) port notes: `graph/embedding` is ported. ADR-071 is not carried: its one
  decision here is the `keep` filter, specified by this change's spec; the rest is `pkg/fusion`'s (change 5). ADR-081
  and ADR-073's erasure fan-out are not implemented at the pin (inventory 6).
- `docs/repository-map.md`: a `graph/embedding` row. `scripts/cover-check.sh`: `graph/embedding merged` at 80% (on the
  critical list, `setup-03b-contract-boundary` D10).

## D9. Proving tests

Unit (fakes of `jetstream.KeyValue` behind `natsclient.NewClient("").NewKVStore`): the pin's BM25, dedup, truncation,
identity and failure-reason tests, without HTTP and without `time.Sleep` (S25); `TestWorkerRunRefusesNilContext`,
`TestWorkerSecondRunRefused`, `TestVectorCacheSecondRunRefused`, `TestVectorCacheNotAuthoritative` (before snapshot,
after decode error, after `Run`), `TestFindSimilarOrderAndLimit`, `TestFailureWithCanceledTextIsAFailure`,
`TestBothLanesEmbedIdenticalBytes`, `TestTerminalCallbackOncePerOutcome`, `TestPanickingEntryStaysPending` (the pin's
`TestWorkerPanicCostsOneEntryNotTheGoroutine`), `TestCASExhaustionIsNotTerminal`, `TestNoTextDeleteFailureIsNotTerminal`
(the fake's fenced delete fails with a transport error: no terminal callback, record still pending). The `synctest`
tests the `background-work` spec asks for (`TestWorkerRunLeavesNothingBehind`, `TestVectorCacheRunLeavesNothingBehind`)
run on these fakes, where no nats.go goroutine exists; the nats.go side is the integration tests below.

Integration (`natsfixture`, shared server via the reset; #195):

- `TestWorkerRunLeavesNoWatchDelivery` and `TestVectorCacheRunLeavesNoWatchDelivery`: 600 records (above the 256-entry
  watch buffer), a callback or entry that blocks until the context ends, `natsfixture.AwaitNativeDeliveryBlocked`
  before cancelling, ten cycles; after `Run` returns no goroutine is in a Worker, VectorCache or pool method and
  `natsfixture.NativeDeliveryStacks` is empty (the follower's test shape).
- `TestWorkerRunReportsLostWatch` and `TestVectorCacheRunReportsLostWatch`: the bucket is deleted under a live `Run`;
  `Run` returns a lost-watch error, and `natsfixture.NativeDeliveryStacks` is then empty (the delivery has ended).
  nats.go's unsubscribe goroutine (`js.go:2063-2068`) is released by the cancelled watch context, not joined, so the
  test waits for its stack, matched by its own pattern, to disappear with `probe.Await` (#392: wait on the event,
  never assert at once).
- `TestRevisionGuardHistoryMatchesModel`: a sequential generated history (`pgregory.net/rapid`) of up to 30 calls of
  `SavePendingGuarded`, `SaveGenerated`, `SaveFailed`, `DeleteEmbedding` and the fenced no-text delete over two
  entities and revisions 1-4. The model maps entity → (status, revision, KV revision) or absent and encodes D3
  exactly: pending is refused only over generated at the same or a newer revision (an older pending replaces a newer
  pending); generated and failed are `ErrRecordGone` on absent and `ErrSupersededRevision` below the stored revision;
  failed is also refused over generated at an equal revision; the fenced delete removes only at the KV revision it is
  given. Each call's result and the stored record are checked against the model. Bounded at 50 runs; the seed and
  completed-run count go in `notes/2.2.md`, and a failure prints its replayable sequence.
- `TestKeyedCallbacksFollowRevisionOrder` (the #107 regression for Codex's finding 2): pending N's body fetch fails,
  so `SaveFailed(N)` is stored; N's terminal callback blocks until the test has written pending M and seen it in the
  bucket; then it is released. The terminal callbacks arrive as Failed(N) then Generated(M), never the reverse, and
  M's processing does not start before N's callback returns. Shown failing against the pin's shared-channel shape
  in `notes/2.3.md`.
- `TestWorkerSaveRefusedWhenNewerPendingArrives` (the guard against hop 1): the fake embedder holds N's text until the
  test has written pending M and seen it in the bucket; then N is released. N's save is refused, `onGenerated` does
  not fire for it and its outcome is `OutcomeDeleted`; M is then processed and the record ends generated at M.
- `TestNoTextDeleteKeepsNewerPending` and `TestNoTextDeleteKeepsNewerGenerated` (Codex's finding 1): N carries a
  storage reference whose body is empty, and the fake store holds N's fetch until the test has written pending M
  (first test) or written M's generated record directly through `Storage`, as another process's Worker would (second
  test), and seen it in the bucket. After release, N removes nothing and reports `OutcomeDeleted`; the record is
  still pending at M, or still generated at M with M's vector.
- `TestSaveDedupKeepsSameSpaceReplacesOther`; `TestWorkerResolvesLiveRegistryStatePerFetch` (ported).
- Mutation (`task mutate:check`, recorded in `notes/2.2.md`; survivors reported as survivors): `SaveGenerated`'s
  revision comparison removed → the history test and `TestWorkerSaveRefusedWhenNewerPendingArrives` fail;
  `SaveFailed`'s removed → the history test fails; the no-text delete made unconditional → both no-text tests and the
  history test fail.

Contract: `TestEmbeddingImportsNoProviderModule` (`go list -deps ./graph/embedding` names neither `go-openai` nor
`model`; #35's proving test). Each changed behaviour is shown failing against the pin's shape in `notes/2.x.md`.

## D10. Issues and merge order

| Issue | Disposition |
| --- | --- |
| #35 | retired (ruling item 1): the file is excluded and `TestEmbeddingImportsNoProviderModule` holds; the fallback test is #32's |
| #107 | retired by adopting the keyed pool (D6; ruling item 2's fallback); graph-index's half merged in #414 |
| #141 | advanced: this package's three loops and `SaveDedup` move (D3); stays open for clustering, spatial, temporal |
| #195 | advanced: this package's one site; stays open |
| #32, #96, #109, #155, #10, #11 | stay (inventory 3) |

Filed by this design: one adoption issue for `internal/kvwatch` (D11). Every defect found in this package is
corrected here (D3-D7). Merge order: #455 merged. #452 (draft)
shares no file but changes the gate (`task verify`, CI, merge gate) and the contracts; whichever merges second merges
`origin/main` and re-runs `task verify` under the gate then on main. Nothing waits on the other.

## D11. Adoption sweep for `internal/kvwatch`

Moving the follower's drain into `internal/kvwatch` makes it a primitive two packages share, so the establishing
side's sweep is owed (architect contract, "The adoption sweep"). Sites in the tree that stop a KV watch, at `6b146a8`,
for one tracking issue, none a migration obligation of this change: `natsclient/kv.go:699` (`keysByFilter`, its own
5 s drain: a second spelling, kept by the owner on #420; not merged into `natsclient` here, which would add mechanism to
a public package against #170, D1); `graph/readiness/watcher.go:279`; `config/manager.go:601`;
`service/message_logger_kv_watch.go:174`; `processor/graph-ingest/poison_inventory.go:326`, `:353`;
`pkg/lifecycle/manager_query.go:224`; `natsclient/storagereport/storage_report_consumer.go:220`. Each either drains
until close or can leave a backlogged delivery running after it returns; the issue asks which, per site.
