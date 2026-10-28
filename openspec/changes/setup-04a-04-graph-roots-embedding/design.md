# Design: setup-04a-04-graph-roots-embedding

Base `f92753b` (main after #455; the inventory was taken at `08e5d9d`, and #455 touched only `graph/follower`).
Inventory: `inventory.md` (round 2, `INVENTORY PASS`). Change 4, PR 6 of 9 (#9 comment 6386856657, item 5). Tier 0
only (#9 comment 6386603659). Draft PR #458. Owner rulings for this change: PR #458 comment 6473224284 (#35 retired
here; #107 retired on the D6 finding, or the Worker adopts the keyed pool). Governing records: ADR-066, ADR-063,
ADR-084, ADR-085 (binding, ported), ADR-110 and ADR-113 (SemEngine's), ADR-071 (keep predicate), inventory 6. Probes:
inventory 7 (P1-P14), posted on PR #458 comment 6472856197.

Coined terms: *hop 1* is `processor/graph-embedding` writing a pending record per entity revision; *hop 2* is the
Worker turning a pending record into a generated or failed one; the *revision guard* is the rule that a write for an
older source revision never replaces a newer one (`pin:graph/embedding/storage.go:396`, `:455`, `:465`, `:313`); a
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
- Reads keep their pin results through the KVStore: `GetEmbedding` and `GetByContentHash` return `(nil, nil)` for an
  absent key; `DeleteEmbedding` of an absent key returns nil (P5). `ListGeneratedEntityIDs` returns every key, pending
  and failed included, as at the pin (its doc is corrected; the caller filters by status). **Cost:** it now lists
  through `KVStore.Keys`, bounded by the KVStore's `Timeout` option (default 5 s), where the pin streamed under the
  caller's context; whoever builds the KVStore sets the bound for its bucket size.

## D4. The Worker runs until its context ends (`background-work`, ADR-113)

Settled records, applied: the `background-work` spec (`Run(ctx)` preferred; the Worker calls caller callbacks, so
`Close` does not qualify) and ADR-113 (the component that runs the Worker creates the run context in its `Start` and
ends it in `Stop`; this package derives nothing from a Start context). The foundation design's adapt row for
`Worker.Stop()` (`setup-04a-foundation` D6/D7) is met by this shape, as `graph/readiness.Watcher.Run` met its row.

- `Run(ctx)` refuses a nil context with an invalid-input error before starting anything.
- It opens one watch on `EMBEDDING_INDEX` under `ctx` and processes entries on `workers` goroutines (default 5, floored
  at 1), as at the pin. A watch that cannot open returns its error, with nothing left running.
- When `ctx` ends, `Run` stops the watch and returns `ctx.Err()` only after every goroutine it started has exited; an
  entry in progress finishes first (callbacks run under `ctx`). A stop error that only says the watch was already gone
  (`nats.ErrBadSubscription`, `nats.ErrConnectionClosed`, `nats.ErrConsumerNotFound`, as `graph/follower` treats them
  at `follower.go` `stopWatch`) is not reported. **Changed:** no error-text fallback (S7). `Run` does not wait for the
  watch's update channel to close, so #454's open-forever delivery cannot hold it.
- If the update channel closes while `ctx` is still live, `Run` returns an error saying the watch was lost.
  **Changed:** at the pin the goroutines returned silently and the Worker consumed nothing while healthy.
- A second `Run` on the same Worker while one is running is refused with an error; after `Run` returns, `Run` may be
  called again and starts a new watch, whose snapshot re-processes failed records once, as a pin restart did.
- The `stopping` field goes (S18).

## D5. The vector cache is its own background work

`VectorCache` holds the in-memory copy of generated vectors that `Storage` held at the pin. `Run(ctx)` refuses nil,
watches `EMBEDDING_INDEX` through the Storage's index KVStore, and returns `ctx.Err()` after its goroutine-free loop
ends (it runs on the caller's goroutine; nothing to join). `FindSimilar` returns `(nil, false)` before the watch's
snapshot has been applied, after a record failed to decode, after the watch was lost, and after `Run` returned; the
caller then reads KV, as at the pin. Results are sorted by `ScoredEntityLess` before the limit; `keep` filters before
scoring (ADR-071). **Changed, optional and cheap:** a limit of 0 or less returns an empty result instead of
panicking on a negative slice bound (S21). A lost watch makes `Run` return an error, as in D4.

## D6. Ordering per entity: the #107 finding

Owner ruling item 2: retire #107 if the revision guard already gives the ordering that matters; otherwise adopt the
keyed pool (`internal/dispatch.KeyedPool` in this tree, `internal/dispatch/keyed_pool.go`; the ruling's
`pkg/dispatch` is its pin path). The finding, with what it rests on:

- **What can happen.** The Worker's goroutines share one watch channel (P14), so two revisions N < M of one entity
  can be in flight at once and finish in either order.
- **The stored record.** Every hop-2 write for an entity goes through the revision guard under compare-and-set (D3):
  a write for N after M's lands is refused (`ErrSupersededRevision`); a write for N over M's pending record is refused
  the same way; a failure never replaces a generated vector at the same revision. A deleted key refuses both
  (`ErrRecordGone`). So the record ends at M's outcome in every interleaving. Pin evidence:
  `TestSaveGenerated_NewerRevisionWinsOutOfOrderCompletion`, `TestSaveFailed_DoesNotClobberNewerSuccess`.
- **The callbacks.** A refused save fires no `onGenerated` (`pin:…/worker.go:791-805`), so no older vector reaches a
  cache. `onTerminal` may fire for N after M; both its readers are order-tolerant: the watermark drains in-flight
  revisions at or below the completed one (`revision-watermark` spec, "Completion is key-scoped and drains lower
  revisions"), and the component's failed map refuses an older revision (`pin:processor/graph-embedding/component.go:1131`,
  carried in change 5).
- **What a keyed pool would add.** Fewer wasted embeddings of revisions already superseded; no correctness property.
  Content dedup and the in-process collapse of identical text (singleflight) already bound that waste.

So the Worker keeps its plain goroutines and this pull request retires #107 on this finding. The standing proofs are
D9's `TestRevisionGuardHistoryMatchesModel` and `TestWorkerConcurrentRevisionsEndAtNewest`; if either cannot be made
to pass, the finding fails and the Worker adopts `KeyedPool` keyed by entity ID, per the ruling.

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
- **Kept, optional:** `classifyEmbedErr`'s substring classes (S9), for a product's own `Embedder`.

## D8. ADRs, ledger, docs

- Ledger: new row `graph/embedding`, `adapt`, naming D2-D7 and the dropped surface; new row
  `graph/embedding/http_embedder.go`, `defer-exclude`, destination none, `known_risks` naming #32 and the 04C fallback
  test (owner ruling item 1; #9 comment 6386603659).
- ADR-066 (`:18`) and ADR-063 (`:13-15`) port notes: `graph/embedding` is ported. ADR-071 is not carried: its one
  decision here is the `keep` filter, specified by this change's spec; the rest is `pkg/fusion`'s (change 5). ADR-081
  and ADR-073's erasure fan-out are not implemented at the pin (inventory 6).
- `docs/repository-map.md`: a `graph/embedding` row. `scripts/cover-check.sh`: `graph/embedding merged` at 80% (on the
  critical list, `setup-03b-contract-boundary` D10).

## D9. Proving tests

Unit (fakes of `jetstream.KeyValue` behind `natsclient.NewClient("").NewKVStore`): the pin's BM25, dedup, truncation,
identity and failure-reason tests, without HTTP and without `time.Sleep` (S25);
`TestWorkerRunRefusesNilContext`, `TestWorkerRunLeavesNothingBehind` and `TestVectorCacheRunLeavesNothingBehind`
(started, used and stopped in `synctest.Test`), `TestWorkerRunReportsLostWatch`, `TestVectorCacheRunReportsLostWatch`, `TestWorkerSecondRunRefused`,
`TestVectorCacheNotAuthoritative` (before snapshot, after decode error, after `Run`), `TestFindSimilarOrderAndLimit`,
`TestFailureWithCanceledTextIsAFailure`, `TestBothLanesEmbedIdenticalBytes`, `TestTerminalCallbackOncePerOutcome`,
`TestPanickingEntryStaysPending` (the pin's `TestWorkerPanicCostsOneEntryNotTheGoroutine`), `TestCASExhaustionIsNotTerminal`.

Integration (`natsfixture`, shared server via the reset; #195): `TestRevisionGuardHistoryMatchesModel` (generated
check, `pgregory.net/rapid`: up to 30 operations of `SavePendingGuarded`, `SaveGenerated`, `SaveFailed`,
`DeleteEmbedding` over two entities and revisions 1-4, from two goroutines; a map of entity → (status, revision) is
the model; bounded at 50 runs, seed and run count in `notes/2.2.md`), `TestSaveDedupKeepsSameSpaceReplacesOther`,
`TestWorkerConcurrentRevisionsEndAtNewest` (5 workers, revisions 1-20 of one entity written back to back; the record
ends generated at 20 and `onGenerated` never saw an older revision after a newer one landed),
`TestWorkerResolvesLiveRegistryStatePerFetch` (ported). Contract: `TestEmbeddingImportsNoProviderModule`
(`go list -deps ./graph/embedding` names neither `go-openai` nor `model`; #35's proving test). Each changed behaviour
is shown failing against the pin's shape in `notes/2.x.md`.

## D10. Issues and merge order

| Issue | Disposition |
| --- | --- |
| #35 | retired (ruling item 1): the file is excluded and `TestEmbeddingImportsNoProviderModule` holds; the fallback test is #32's |
| #107 | retired on D6's finding (ruling item 2); graph-index's half merged in #414 |
| #141 | advanced: this package's three loops and `SaveDedup` move (D3); stays open for clustering, spatial, temporal |
| #195 | advanced: this package's one site; stays open |
| #32, #96, #109, #155, #10, #11 | stay (inventory 3) |

Filed by this design: none; every defect found is corrected here (D3-D7). Merge order: #455 merged. #452 (draft)
shares no file but changes the gate (`task verify`, CI, merge gate) and the contracts; whichever merges second merges
`origin/main` and re-runs `task verify` under the gate then on main. Nothing waits on the other.
