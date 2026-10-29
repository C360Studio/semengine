# Inventory: setup-04a-04-graph-roots-graph-embedding

base: `08e5d9ddcb5452a76b5e670f51ce84686eb97417` (origin/main after PR #437; the claim commit `712b429` on
`claude/port-graph-embedding` adds no file). Pin paths are SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
written `pin:<path>:<line>`, read from the pin tarball copy at `/tmp/change4-pin.P2hr/src` (local only). Every file of
`graph/embedding` there was checked blob-equal against
`gh api repos/C360Studio/semstreams/contents/graph/embedding?ref=8b99efe9` (`git hash-object` of all 37 files
equals the API's `sha` for each). Consumer cites: semsource `e4febc0d` via `gh api … ?ref=`; the other starter
consumers by a read-only grep of their local checkouts (no SHA; see § 4).

Status: round 2, inventory only (round 1 reviewer findings 1-8 folded in). No target state, options or design
below except where the caller asked for the
PR-shape measurement (§ 10), which is stated as measurement plus the ruling that already settles it.

Kind of change: a port of one package, tier-0 half, by the owner's order ruling (#9 comment 6386856657, item 5:
"`graph/embedding` without `http_embedder.go` (#35); second-lane capable") and scope ruling (#9 comment 6386603659:
no LLM/provider code in change 4; `http_embedder.go` is a `defer-exclude` row naming #32).

Briefing hypotheses that did not hold, or that the tree answers:

- "Report the count without `http_embedder.go` and whether a split is needed": 3,082 non-test lines (§ 10). The
  owner already waived the bound for this package at 3,302 and ruled it "ports as one PR" (#9 comment 6386556850,
  ruling 1). No split question remains.
- "Dependencies on moved packages (pkg/resource, pkg/cache, lifecyclecleanup, keyed dispatch)": the package imports
  none of them. Its only module imports are `pkg/errs` and `storage` (both in the tree, public) plus nats.go
  `jetstream`/`nats` and `golang.org/x/sync/singleflight` (§ 6).
- "`embedding.Worker` adopts `KeyedPool` (#107)": the Worker is not a keyed pool; it runs N goroutines reading one
  KV watch channel (`pin:graph/embedding/worker.go:350-357`). #107's drift note already says "judge it on its own"
  (§ 3); that note is not marked as a ruling (O2).
- "GenericJSON payloads needing an `adapt` item": none. The package imports no `message` package; its records are
  KV values, not `BaseMessage` payloads (§ 8).
- "The Owner 2026-10-28 #443 rule (reuse `Client.WaitConsumerHandlers`)": not reached. The package creates no
  JetStream consumer through natsclient; it uses KV watches (`WatchAll`) only (§ 8).

## Pin probe

go1.26.4 darwin/arm64, on the pin copy, unit tests only (no build tag).

- `go test -race -count=1 -p 1 -cpu 1 -cover ./graph/embedding`: ok, 3.807 s, coverage 75.6%.
- `go test -count=5 -p 1 -cpu 1 -shuffle=on -v ./graph/embedding`: ok, 4.454 s, `-test.shuffle 1793200020430019000`.
- Lifecycle suite: not run. It is this tree's harness and does not build against the pin. Neither `Worker` nor
  `Storage` is a component or service; both start background goroutines, so the `background-work` spec, not the
  lifecycle suite, is the bar they meet in the tree (§ 8).
- Integration test (`worker_storeresolver_integration_test.go`, `//go:build integration`): not run (Docker cost).
- Test-text rules over `pin:graph/embedding/*_test.go` (27 files, 5,419 lines, 105 `func Test`; without the two
  HTTP test files 25 files, 5,091 lines, 98 tests):
  - `time.Sleep` 2: `review_findings_test.go:364` and `worker_failure_reason_test.go:201`, both
    `time.Sleep(150 * time.Millisecond)`.
  - `t.Skip`/`Skipf`/`SkipNow` 0. Build tags other than `integration`: none.
  - `t.Parallel` in 14 files, none in the integration-tagged file.
  - Test clients outside the fixture: `worker_storeresolver_integration_test.go:22` —
    `testClient := natsclient.NewTestClient(t, natsclient.WithKV())` (#195's one graph/embedding site).
  - Unit tests run on in-memory fakes of `jetstream.KeyValue` (`memkv_test.go`, `watchkv_test.go`), not on NATS.

## 1. The claimed gap

- The package is not in the tree: `ls graph/` at base has no `embedding` directory; `git grep -n 'graph/embedding'
  -- '*.go'` returns nothing. The bucket names exist: `graph/constants.go:29` — `BucketEmbeddingIndex =
  "EMBEDDING_INDEX"`, `:30` — `BucketEmbeddingDedup = "EMBEDDING_DEDUP"`; catalog rows
  `graph/kvcatalog/kvcatalog.go:186-187` — `derived(graph.BucketEmbeddingIndex, "graph-embedding", …)`, owner-only
  writes (`owned` → `IsFrameworkOwnedBucket` true, `:230-232`).
- Its two in-module imports are present: `pkg/errs` (`WrapInvalid` `pkg/errs/errs.go:388`, `WrapTransient` `:370`,
  `ErrMissingConfig` `:68`, `ErrInvalidConfig` `:67`) and `storage` (`storage/storage.go:96` — `type StreamableStore
  interface {`); `storage/storeregistry/storeregistry.go:86` — `func (r *Registry) Streamable(instance string)
  (storage.StreamableStore, bool) {` satisfies the pin's `StoreResolver`.
- `golang.org/x/sync` is `// indirect` at `go.mod:94` (v0.24.0; pin v0.20.0); the port makes it direct.
  nats.go is v1.54.0 here (`go.mod:17`), v1.52.0 at the pin.

## 2. Every current spelling of the facts the package models

| Fact | Spellings |
| --- | --- |
| Embedder interface | `pin:graph/embedding/embedder.go:14` — `type Embedder interface {`; `pin:graph/query/classifier_embedding.go:17` — `type Embedder interface {` (Embed/EmbedBatch; graph/query's PR) |
| Cosine similarity | `pin:graph/embedding/vector.go:16` — `func CosineSimilarity(a, b []float32) float64 {`; `pin:graph/query/classifier_embedding.go:244` — `func cosineSimilarity(a, b []float32) float64 {` (graph/query's PR). None in the tree. |
| The text-length cap's default | `pin:…/worker.go:930-938` (inline lane: cap 0 = no truncation); `pin:…/worker.go:1054-1057` — `if limit <= 0 { limit = defaultMaxSourceTextLen }` (offloaded lane: 0 = 8,000); doc `:287` — `// Default: 0 (unlimited).`; the component's per-embedder default (`pin:processor/graph-embedding/component.go` `maxSourceTextLen`, change 5) |
| Which bucket the Worker reads | `NewStorage(indexBucket, dedupBucket)` (`pin:…/storage.go:194`) and again `NewWorker(storage, embedder, indexBucket, logger)` (`pin:…/worker.go:248` — `indexBucket jetstream.KeyValue,`) |
| Revision-mismatch classification | `pin:…/storage.go:498` (`errors.Is(err, jetstream.ErrKeyExists)`) and `:501-502` (message text) |
| Shutdown-error classification | `pin:…/worker.go:54-58` (`errors.Is`) and `:61-63` (message text) |
| Read-modify-write on EMBEDDING_INDEX | three hand-written CAS loops `pin:…/storage.go:308`, `:367`, `:438` — `for attempt := 0; attempt < maxCASRetries; attempt++ {`; the tree's one primitive `natsclient/kv.go:346` — `func (kv *KVStore) UpdateWithRetryRead(` (ADR-110 D2) |
| Read-modify-write on EMBEDDING_DEDUP | `pin:…/storage.go:592` — `existing, err := s.GetByContentHash(ctx, contentHash)` then `:628` — `if _, err := s.dedupBucket.Put(ctx, contentHash, data); err != nil {` (unconditional Put; not in ADR-110's site list) |
| KV watch + snapshot sentinel loop | Worker `pin:…/worker.go:398-439`; vector cache `pin:…/storage.go:801-860`; `ScanFailed` `pin:…/storage.go:727-774`; tree: `graph/follower/follower.go:188`, `graph/readiness/watcher.go:195` (both `Run(ctx)`) |

## 3. Adjacent claims

### Open pull requests

Round 3 note (design): #455 merged on 2026-10-28 as `f92753b`; the design's base is that commit.

`gh pr list --state open`, re-read in round 2: three open; #458 and #452 drafts, #455 ready for review.

| PR | Files | Overlap |
| --- | --- | --- |
| #458 (this claim) | none yet | — |
| #455 fix(follower): bound the wait for the watch's delivery to close (#454); ready for review | `graph/follower/*`, its OpenSpec change | No shared file. Same problem shape: #454 shows a KV watch's `Updates()` can stay open forever if the watch's context ends before nats.go installs the close handler. The pin Worker and vector cache never wait for `Updates()` to close (they exit on `ctx.Done()`), so the pin shape is not exposed; a design that adds a drain-until-closed step inherits #454 and merges after #455. |
| #452 chore(harness): adopt ateam v0.3.3 (PR A, #448); draft | 69 files: `.agents/**`, `.claude/**`, `AGENTS.md`, `Taskfile.yml`, `.github/workflows/ci.yml`, `scripts/verify.sh`, `docs/testing.md`, `harness-boundaries` and `merge-gate` spec deltas | No shared file, but it changes the gate this PR runs (`task verify`, CI, merge gate) and the contracts the design cites (`.agents/contracts/semengine-*.md`); if it merges first, re-run verify under the new gate and re-pin contract cites. |

Merge order: independent of both; none blocks. The ledger is touched by none of them.

### Open issues naming the package

Query: `gh issue list --state open --search embedding`, then `BM25`, `Embedder`, `graph-embedding`,
`EMBEDDING_INDEX`, `embedding.Worker`, `DedupKey`, `CosineSimilarity`.

| # | How it names the package | Work, as Bounded port scope classes it | Where |
| --- | --- | --- | --- |
| #35 | `graph/embedding` split of `http_embedder.go` | Done by this PR's exclusion (`defer-exclude` row naming #32; tier-0 build has no `go-openai`). Its body's third proving test ("tier-1 → tier-0 fallback test (Slice 04C)") cannot run here. | Retire with this PR **or** keep open to 04C — owner question O1 |
| #107 | `graph/embedding/worker.go:181-227` "a third hand-rolled pool" | Required by its body ("`embedding.Worker` adopt[s] KeyedPool"), but its 2026-10-24 drift note says the Worker "is not a keyed pool … judge it on its own in graph/embedding's PR"; graph-index half done (#414) | This PR decides the Worker half; owner question O2 |
| #141 | `graph/embedding/storage.go:308`, `:367`, `:438` | Required: ADR-110 D2/D7, ruled 2026-10-10 (#141 comment 6097826823): binds every read-modify-write over a framework-owned bucket at the package's port. Also binds `SaveDedup` (`:592-628`) by D2's first sentence, though D7 does not list it. | Adopted in this PR; #141 stays open (clustering, spatial, temporal sites) |
| #195 | change-4 count note: `graph/embedding` has 1 `NewTestClient` site | Adoption, test-only | This PR's integration test moves to `natsfixture`; #195 stays open for other packages |
| #96 | `pin:graph/embedding/bm25_embedder.go#L125` (`Generate` learns from queries) | The fix is in `graph/query`'s adapter (`classifier_embedding.go:43-55` calls `Generate` with `context.Background()`); `GenerateQuery` already exists here | Stays: `graph/query` PR (order item 6) |
| #32 | names `graph/embedding/http_embedder.go` as a deferred home | Home of the excluded file; exit is 04C or an owner ruling | Stays |
| #109 | comment 2026-10-18: "`embedding.Worker` watches `EMBEDDING_INDEX`, not `ENTITY_STATES`" | Follower adoption is `processor/graph-embedding`'s (change 5) | Stays |
| #155 | names `http_embedder.go` as a `model` reader; "no reader in the tree holds through change 4" | Excluding the file keeps that premise | Stays |
| #10 | epic 04B: BM25 known-answer, restart, rebuild | BM25 statistics are process-local and order-dependent (`pin:…/bm25_embedder.go:113-118`, SemStreams gh#619) | Stays (04B) |
| #11 | epic 04C: provider embeddings | the excluded HTTP embedder | Stays (04C) |
| #9 | epic | — | `Addresses #9` |

Names the word, not the package: #94, #95 (graph/query classifier), #106 (`processor/graph-embedding/query.go`), #134,
 #15, #7, #196 (lists `processor/graph-embedding/component.go:298`, change 5; `embedding.Worker` is not on its
adopter list).

### Specs, ledger, records

- `openspec/specs/background-work/spec.md:11-27`: goroutines outliving their starting call, outside a service,
  stop as `Run(ctx)`, `Close()` or `Shutdown(ctx)`; `:79-82` a `synctest` test and nil-context refusal. Binds every
  later port.
- ADR-113 (`docs/adr/113-start-context-bounds-start.md:57-77`): ending the Start context stops nothing; `Stop(ctx)`
  is the only stop. Applies to the component in change 5; for background work it is met by `Run(ctx)`.
- Archived foundation design D6/D7 (`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:385-399`,
  `:423`): `embedding.Worker.Stop()` → a context-taking ender is an `adapt` row "ported in 4"; its caller
  `processor/graph-embedding/component.go:882` is edited in change 5 (`:424`).
- Ledger (`docs/admission-ledger.yaml`): no `graph/embedding` row exists. Rows that name it: `pkg/revlag`
  (`:3361-3393`, graph-embedding is a later importer) and `storage/objectstore` (`:3565`). File-level
  `defer-exclude` format: header `:15`.
- ADR port notes that say "not yet ported": `docs/adr/066-honest-graph-index-readiness.md:18`,
  `docs/adr/063-store-substrate-and-resolver.md:13-15`.

## 4. Who reads the surface (pin-wide; `grep` over the pin copy, non-test files)

Importers at the pin: `graph/query/classifier_embedding.go` and
`processor/graph-embedding/{component,query,readiness}.go`.
Starter consumers: none imports `graph/embedding` (read-only grep of the semsource, semconnect, semteams, semboids
checkouts: 0 files). semsource composes the component by config: `semsource:cmd/semsource/run.go:724` —
`embedderType := "bm25"`, `:812-817` — `"graph-embedding": {` … `"embedder_type": embedderType,` (at `e4febc0d`).

| Symbol | Pin production readers outside the package | Disposition input |
| --- | --- | --- |
| `Embedder`, `NewStorage`, `Storage`, `Worker`, `NewWorker`, all six `With*` | `processor/graph-embedding/component.go:217`, `:253-255`, `:1001`, `:1017-1040` | caller in admitted, unported package (change 5) |
| `BM25Embedder`, `BM25Config`, `NewBM25Embedder` | `graph/query/classifier_embedding.go:38-40`, `:73`; `component.go:911` | graph/query is next (PR 7) |
| `GenerateQuery` | `processor/graph-embedding/query.go:218` | change 5 |
| `CosineSimilarity`, `ScoredEntity`, `ScoredEntityLess`, `StatusGenerated` | `query.go:150`, `:295-339` | change 5 |
| `Record`, `StorageRef`, `SavePendingGuarded` | `component.go:1907`, `:1993`, `:2025` | change 5 |
| `GetEmbedding`, `DeleteEmbedding`, `ListGeneratedEntityIDs`, `ScanFailed`, `StartVectorCache`, `FindSimilarFromCache` | `query.go:134`, `:281`, `:290`, `:317`; `component.go:1012`, `:1073`, `:1562`, `:1612`, `:1858` | change 5 |
| `TerminalOutcome`, `OutcomeFailed/Skipped/Deleted`, `Reason*` (3), `MaxSourceTextLenCeiling`, `WorkerMetrics` | `component.go` (16 sites), `readiness.go:38`, `metrics.go:396-489` | change 5 |
| `SaveGenerated`, `SaveFailed`, `GetByContentHash`, `SaveDedup`, `EmbedderIdentity`, `DedupKey`, `InProcessDedupKey`, `ErrCASExhausted`, `ErrRecordGone`, `ErrSupersededRevision`, `DedupRecord`, `FailedEntry`, `Status`, `StatusPending/Failed`, `OutcomeGenerated`, `GeneratedCallback`, `TerminalCallback`, `StoreResolver` | in-package production readers only (`worker.go:558-782`, `:1173`; `storage.go:592`) or as parameter types of the `With*` methods | read, not dead; exported without an outside reader (optional, § 9 S20) |
| `ContentHash` (`pin:…/dedup.go:20`) | none in production anywhere; tests only (`dedup_key_test.go`, `worker_dedup_identity_test.go`, HTTP test) | **dead** |
| `SavePending` (`pin:…/storage.go:206`) | none in production; 48 test call sites here, six graph-embedding test files | **dead** |
| `SavePendingWithStorageRef` (`pin:…/storage.go:244`) | none in production (`doc.go:112` example only); tests | **dead** |
| `Record.ContentFields` (`pin:…/storage.go:120`) | written only by dead `SavePendingWithStorageRef`; never read | **dead**; doc `:117-118` describes absent behaviour |
| `Record.GeneratedAt`, `DedupRecord.FirstGenerated`, `DedupRecord.EntityIDs` | written, never read (pin-wide grep) | dead fields; `EntityIDs` also grows (§ 9 S9) |
| `euclideanDistance`, `dotProduct`, `magnitude`, `normalize` (`pin:…/vector.go:50-105`) | none, not even tests | **dead unexported** |
| `HTTPEmbedder`, `HTTPConfig`, `NewHTTPEmbedder` | `component.go:931`, `:944` (tier-1 path) | excluded by ruling |

## 5. The problem shape

- **Watch a KV bucket and work each entry** (Worker, vector cache, `ScanFailed`). Closest in the tree:
  `graph/follower/follower.go:188` — `func (f *Follower) Run(ctx context.Context) error {` and
  `graph/readiness/watcher.go:195` (`Run(ctx)`), the `background-work` spec's preferred shape. `graph/follower` is
  ENTITY_STATES-specific (ADR-108); the Worker watches EMBEDDING_INDEX.
- **Read-modify-write on a framework-owned bucket**: `natsclient/kv.go:346` `UpdateWithRetryRead` (ADR-110 D2).
  Storage takes a raw `jetstream.KeyValue` (`pin:…/storage.go:177-178`), not a `*natsclient.KVStore`.
- **Serving projection over a KV watch** (vector cache): at the pin, `pkg/graphview` (ADR-081, order item 7, not in
  the tree); ADR-081 marks the embedding cache and worker "Convert" (`pin:docs/adr/081-graph-view-subscription.md:202`),
  which the pin never did.
- **Bounded fan-out of work**: `internal/dispatch.KeyedPool` (`internal/dispatch/keyed_pool.go`; graph-ingest and
  graph-index use it). The Worker's fan-out has no key and no ordering promise; the SourceRevision guard in storage
  orders writes instead (`pin:…/storage.go:396`).
- Adoption sweep (establishing side): nothing here is a new reusable primitive; no sweep owed.

## 6. Decision records at the pin, and their state here

Search: all 85 records at the pin, `grep -l` for `graph/embedding`, `EMBEDDING_INDEX`, `EMBEDDING_DEDUP`,
`BM25Embedder`, `embedding.Worker`, `DedupKey`, `ScoredEntity`, `FindSimilarFromCache`, `StartVectorCache`,
`SavePendingGuarded` → ADR-037, -043, -063, -066, -071, -081; then a second pass for `graph-embedding|BM25|Embedder`
(`grep -l -E`), which adds 040, 042, 045, 054, 059, 061, 062, 065, 068, 073, 083, 085, 086, 088; each hit was read for
what it decides about this package (040, 042, 045, 059, 062, 065 name embedding only in passing).

| ADR | Decides (for this package) | Implemented by pin code here? | In tree |
| --- | --- | --- | --- |
| 066 honest readiness | hop-2 terminal callback drains the watermark at the record's SourceRevision (`worker.go:107`, `:471-486`; `storage.go:105-114`) | yes — binding | ported; port note `:18` says graph/embedding not yet ported |
| 063 store substrate & resolver | per-fetch `StoreResolver`, never cached; `content_unresolved` vs `content_resolve_error` (`worker.go:164-178`, `:996-1100`) | yes — binding | ported; port note `:13-15` |
| 084 readiness licenses health | a poison record pins the watermark, not skipped (`worker.go:466-470`) | yes — binding | ported |
| 110 write seam (SemEngine) | D2/D7: RMW loops at `storage.go:308/367/438` move onto `UpdateWithRetryRead` at port | to be adopted | accepted |
| 113 Start context (SemEngine) | run context outlives Start ctx; Stop is the only stop | pin violates it: `worker.go:338` — `runCtx, cancel := context.WithCancel(ctx)` | accepted |
| 071 fusion domain-scoped NL retrieval | the `keep` predicate in `FindSimilarFromCache` filters before cosine (`storage.go:883-888`) | yes — binding for that parameter | not ported |
| 081 graph view subscription | convert the vector cache and worker onto `pkg/graphview` | no — the pin did not convert | not ported |
| 037 model/wire | `http_embedder.go` migrates to `wire.EmbeddingsClient` | file excluded; ruling #9 6386603659 covers the drop | not ported |
| 043 detonation corpus | BM25 warm cache in the classifier | graph/query's | not ported |
| 068, 083, 088 | name graph-embedding the component (watermark, envelope) | component, change 5 | 083, 088 ported |
| 085 gate on health, report freshness | `pin:docs/adr/085-gate-on-health-report-freshness.md:162-164` — graph-embedding "requires each enumerated entity to reach a terminal embedding outcome, and deliberately has no `Ready` shortcut": rests on this package's `TerminalCallback` firing once per terminal outcome (`worker.go:471-486`) | yes, through the callback — binding on the callback's contract | ported |
| 073 ingestion + retention contract | `pin:docs/adr/073-graph-ingestion-retention-contract.md:303-306` — "A GDPR erase must fan out to **embeddings (BM25 + neural)** …"; `:133` stale embedding-vector/dedup entries | no: the pin has no erasure fan-out; `DeleteEmbedding` removes the index record only, and EMBEDDING_DEDUP keeps the vector under its content key | not ported |
| 054 indexing eligibility | which entities are embedded at all (`:16-19`, `:36`) | component's hop-1 policy (change 5), not this package | not ported |
| 061 remove community `SemanticProvider` | records that semantic search ranks from the embedding index (`:35`, `:49`) | no code here | not ported |
| 086 semantic co-location edges | clustering gates on embedding readiness (`:16`, `:67-68`) | graph-clustering (change 7) | not ported |

Changed or dropped decisions needing a new owner word: none found. The drops (037 via the exclusion) and changes
(113 over the pin's run context; 110 over the hand loops) are already ruled.

## 7. Library behaviour the port relies on (#170, ADR-112)

Probe at nats.go v1.54.0 / nats-server v2.15.1 (`go.mod:16-17`), embedded server, JetStream file store, posted with its
source and output on PR #458, comment 6472856197 (run there as `go run .`). The architect's local runs, `go run .`
and `go run -race .`, printed the same lines for P1-P13; P14's arrival order is not promised.

| # | Statement the pin code depends on | Result |
| --- | --- | --- |
| P1 | `Update` at a stale revision | `errors.Is(err, jetstream.ErrKeyExists)` true; text `…err_code=10071 description=wrong last sequence…`; `*jetstream.APIError` code 400, err_code 10071 |
| P2 | `Create` on an existing key | `ErrKeyExists` |
| P3 | `Create` over a delete marker / purge marker | nil / nil (`storage.go:299`'s claim holds) |
| P4 | `Get` of a deleted or never-written key | `== jetstream.ErrKeyNotFound` true (the pin's `==` at `storage.go:513`, `:560` holds at this version) |
| P5 | `Delete` of a never-written key | nil (the `ErrKeyNotFound` branch at `storage.go:642` is unreachable) |
| P6 | `Update(key, v, 0)` on an absent key | nil — it creates the key |
| P7 | `ListKeys` on an empty bucket | no error, 0 keys (the `ErrNoKeysFound` branch at `storage.go:656` is unreachable) |
| P8 | `WatchAll` on an empty bucket | first entry is the nil sentinel |
| P9 | `Updates()` after `watcher.Stop()` | closed |
| P10 | `WatchAll` snapshot | last value per key; a deleted key arrives as one `KeyValueDeleteOp` |
| P11 | watch context cancelled | `Updates()` closes; a later `Stop()` returns `nats: invalid subscription` (`errors.Is(nats.ErrBadSubscription)` true) |
| P12 | `WatchAll` with an ended context | `context canceled` |
| P13 | second `Stop()` | `ErrBadSubscription` |
| P14 | one `Updates()` channel read by 3 goroutines | each entry, and the nil sentinel, reaches exactly one reader (Go channel semantics); which reader gets which entry, and the order entries arrive across readers, is not promised |

Not probed, carried as "not promised": `storage.go:802-803` ("explicit watcher.Stop() … avoids the nats.go race
between Stop() and the internal message-handler goroutine"); #454's close-handler window (not reproduced there).

## 8. Lifecycle owners, NATS use, payloads

- **`Worker`** (`pin:…/worker.go:181`): lifecycle owner by the foundation scan (`worker.go:326,364`). `Start(ctx)`
  opens one `WatchAll` on EMBEDDING_INDEX and starts `workers` goroutines (default 5, `:259`) that share its
  `Updates()` (P14); `Stop()` takes no context, cancels, stops the watch, and waits on `wg.Wait()` with no bound
  (`:390`); `started = false` (`:392`) lets the same instance start again. The goroutines call caller-supplied
  callbacks (`:484`, `:851`) and the embedder, so `Close()` does not qualify (`background-work` spec `:34-40`).
  It holds `cancel` (allowed) and no context. No `context.Background`/`TODO` in production code (grep).
- **`Storage`** (`pin:…/storage.go:176`): `StartVectorCache(ctx)` starts a goroutine that runs until `ctx` ends
  (`:801`) and has no ender and no join: not counted among the foundation's 38 owners (the scan required an ending
  method), but background work under the `background-work` spec. `ScanFailed` joins before it returns (not
  background work).
- NATS: three KV buckets' worth of calls, all on `jetstream.KeyValue` handles passed in: `Get`, `Put`, `Create`,
  `Update`, `Delete`, `ListKeys`, `WatchAll` (×3). No streams, no consumers, no subjects, no request/reply.
- Payloads: no `message` import; no `GenericJSON`; nothing to register. No `adapt` item under #9.
- Metrics: no Prometheus import; `WorkerMetrics` (11 methods) plus a type-asserted optional
  `contentUnresolvedMetrics` (`:171-173`, `:1017`) are implemented by graph-embedding's adapter (change 5).

## 9. Shape sweep (pin copy, every non-test file except `http_embedder.go`)

`R` = candidate required work, with the basis named; `O` = optional under Bounded port scope; `D` = a defect to fix
in the port or file. Round 2: S8 and S13 reclassified (no named trigger); S10b and the `doc.go:168` link added.

| # | Shape | `pin:graph/embedding/<file>:<line>` — text | Basis | R/O/D |
| --- | --- | --- | --- | --- |
| S1 | lifecycle | `worker.go:338` — `runCtx, cancel := context.WithCancel(ctx)`: ending Start's ctx stops the Worker | ADR-113; `background-work` spec | R |
| S2 | lifecycle | `worker.go:364` — `func (w *Worker) Stop() error {` … `:390` — `w.wg.Wait()`: no context, unbounded join; `:392` restartable | `background-work` spec (`Run(ctx)` preferred); foundation D6 adapt row | R |
| S3 | lifecycle | `storage.go:801` — `go func() {` in `StartVectorCache`: never joined, stops only on ctx | completed-joins admission gate; `background-work` spec | R |
| S4 | 1 second spelling | `worker.go:248` — `indexBucket jetstream.KeyValue,` beside `NewStorage(indexBucket, dedupBucket)` | two inputs for one fact; a mismatch is silent | O |
| S5 | 1 second spelling | cap default: `worker.go:287` — `// Default: 0 (unlimited).` vs `:1054-1057` (offloaded 0 → 8,000); inline 0 = no cap (`:930`) | doc not kept; contradicts `:946-950` lane-independence claim; unreachable via the component (it always sets a cap) | O (D if a direct caller exists — none at pin) |
| S6 | 5 error text | `storage.go:501-502` — `return strings.Contains(msg, "wrong last sequence") \|\| strings.Contains(msg, "10071")` | P1: `errors.Is` already matches; text branch is a second spelling | O (goes away with S11) |
| S7 | 5 error text | `worker.go:61-63` — `strings.Contains(errStr, "invalid subscription") \|\| …"consumer not found"` | P11/P13: `errors.Is` matches | O (goes with S2) |
| S8 | 5 error text | `worker.go:1168` — `if strings.Contains(errorMsg, "context canceled") {` on a formatted message | Consequence: an error whose text contains the phrase is classed as shutdown — `OutcomeSkipped`, terminal, the watermark passes a record still pending (against ADR-066 §3). No real-world trigger at tier 0: the only derived context is `runCtx` (`worker.go:338`) and BM25 returns only `ctx.Err()`, so the text arises only at shutdown. Reachable only through a product `Embedder` or store that cancels a context of its own (#185) | O (shape 5; MEDIUM at most) |
| S9 | 5 error text | `worker.go:1126-1136` — `classifyEmbedErr` by substring (`connection refused`, `timeout`, `dimension`) | the doc says provider SDKs flatten errors; with HTTP excluded, BM25 returns only `ctx.Err()`; reachable by a product's own `Embedder` | O |
| S10 | 4 no reader | `dedup.go:20` `ContentHash`; `storage.go:206` `SavePending`; `:244` `SavePendingWithStorageRef`; `vector.go:50,67,83,94` | Surface audit (a); 48 test sites use `SavePending` as setup | R (drop) |
| S10b | 4 advertised absent | `worker.go:146-147` — `// SetPending sets the current pending embeddings gauge` / `SetPending(count float64)`: no call in the package (grep); `doc.go:163` — `//   - SetPending(): Current pending embedding count`; the pin adapter implements it (`processor/graph-embedding/metrics.go:431-432`), so the gauge is never set | Surface audit (c); interface method with no caller | R (drop the method, or a caller sets it) |
| S11 | RMW | `storage.go:308`, `:367`, `:438` — `for attempt := 0; attempt < maxCASRetries; attempt++ {` | ADR-110 D2/D7 (ruled, #141) | R |
| S12 | RMW | `storage.go:592` … `:628` — `SaveDedup` reads then `Put`s unconditionally | ADR-110 D2 first sentence (framework-owned bucket). With `EntityIDs` dropped (S13) it is still keep-if-same-space-else-replace, a write conditional on what it read, so D2/D7 still bind it | R |
| S13 | 4 no reader | `storage.go:602` — `record.EntityIDs = append(record.EntityIDs, entityID)`: written, never read (pin-wide grep). It grows only when a same-space record appears between a worker's dedup miss and its `SaveDedup` (concurrent generation): `dedupRecordUsable` (`worker.go:742-757`) applies the same Model/Dimensions test as the append (`storage.go:598-602`), and an unchanged re-embed is a dedup hit that returns at `worker.go:641-643` without `SaveDedup`. Growth is bounded by those races; no named trigger | Surface audit (b) | R (drop field) |
| S14 | 4 advertised absent | `storage.go:117-118` — `// When StorageRef is set, Worker fetches content from ObjectStore` / `// and uses ContentFields to extract text`: the Worker never reads `ContentFields` | Surface audit (c) | R (drop field) |
| S15 | 4 advertised absent | `bm25_embedder.go:82` — `// B=1.0 means full normalization, B=0.0 means no normalization`; `:94-96` maps `B == 0` to 0.75; negative `K1`/`B` unchecked | Surface audit (c) | R (doc or behaviour) |
| S16 | 4 advertised absent | HTTP text after the exclusion, and one dead link (`doc.go:168` — `[github.com/c360studio/semstreams/graph/indexmanager]`, no `graph/indexmanager` at the pin): `doc.go:8`, `:19`, `:40-43`, `:75-79`, `:136-141`, `:155`; `README.md:8`, `:38-44`, `:70-71`, `:160-161`; `embedder.go:36`; `dedup.go:48`, `:69`, `:105`; `worker.go:186`, `:232`, `:293`, `:598` | Surface audit (c) once the file is out | R |
| S17 | legacy compatibility | `storage.go:110-113` (SourceRevision 0 = "a legacy record"); `worker.go:732-746` (`r.Model == ""` legacy dedup records); `storage.go:68-90` (pre-#635 rolling-upgrade rationale for `IdentityText`) | owner 2026-10-11, #91 comment 6109293267: no SemStreams state migration, no legacy readers | O (comments and branches may go; behaviour within SemEngine is unchanged) |
| S18 | dead field | `worker.go:225` — `stopping bool`: written at `:372`, never read | Bounded port scope | O (goes with S2) |
| S19 | 3 one implementation | `WorkerMetrics` (11 methods, `worker.go:111`) + optional `contentUnresolvedMetrics` found by type assertion (`:1017`): a metrics adapter without the extra method silently drops a counter | one implementer (change 5) | O |
| S20 | 4 exported, in-package readers only | `SaveGenerated`, `SaveFailed`, `GetByContentHash`, `SaveDedup`, `DedupKey`, `InProcessDedupKey`, `EmbedderIdentity`, the three sentinels | readers exist; could be unexported | O |
| S21 | panic on input | `storage.go:925-926` — `if len(results) > limit { results = results[:limit] }`: a negative `limit` panics | sole caller guards `limit <= 0` (`processor/graph-embedding/query.go:121-122`); no real-world trigger at pin | O (MEDIUM at most, #185) |
| S22 | silent degrade | `storage.go:794-796` and `:730-732` — a cancelled-context `WatchAll` error returns nil; `ScanFailed` returns a partial seed with nil on cancel (`:741-743`, declared) | "Failure paths fail closed; any degrade declared" | O (declared at `:722-726`) |
| S23 | unreachable branch | `storage.go:656` (`ErrNoKeysFound`, P7); `:642` (`ErrKeyNotFound` on Delete, P5) | harmless at v1.54.0 | O |
| S24 | known model limit | `bm25_embedder.go:113-118`: statistics are process-local, unpersisted, order-dependent; a restart re-learns while EMBEDDING_INDEX/DEDUP keep older vectors; the dedup key (`Model()` = `bm25-go-k1.5-b0.75`) does not change | #10 (04B) owns BM25 qualification; SemStreams gh#619 | O here; stays under #10 |
| S25 | test text | `review_findings_test.go:364`, `worker_failure_reason_test.go:201` — `time.Sleep(150 * time.Millisecond)` | `harness-boundaries` "No sleeps" (verify) | R |
| S26 | test client | `worker_storeresolver_integration_test.go:22` — `natsclient.NewTestClient(t, natsclient.WithKV())` | not in the tree; `natsfixture` (#195) | R (needed to build) |

No shape-2 copied shell (no `Health`/`DataFlow` in the package). No back door found: the `With*` setters are the
production wiring at `component.go:1017-1040`.

## 10. Size against the one-package rule

Non-test Go lines at the pin (`wc -l`): `worker.go` 1,218; `storage.go` 930; `bm25_embedder.go` 438; `doc.go` 171;
`dedup.go` 133; `vector.go` 105; `embedder.go` 57; `derived_reasons.go` 30; `http_embedder.go` 220. Total 3,302;
**without `http_embedder.go`, 3,082** (the foundation design's "ruled ceiling figure", `design.md:30`). Dropping
S10's dead code alone takes roughly 130 more.

Two lifecycle owners (Worker; Storage's cache goroutine). A Store/Worker split would build (Storage does not import
the Worker), but the owner has already ruled this package "ports as one PR" with a waiver at 3,302 (#9 comment
6386556850, ruling 1), and 3,082 is under it. One PR, no further waiver needed.

## 11. Collision table: vector cache and Worker

Existing durable and coordination owners the port carries (not a new primitive).

| Dimension | Evidence |
| --- | --- |
| Semantic class | (a) entity → vector index and its pending/failed state; (b) content-hash → vector reuse; (c) an in-memory serving copy of (a) |
| Owners | graph-embedding (catalog `kvcatalog.go:186-187`); at the pin `Storage` writes both buckets, `processor/graph-embedding` writes pending records (`component.go:1907`, `:2025`) and deletes (`:1562`, `:1612`, `:1858`) |
| Catalogs | `graph/kvcatalog` rows above, `ClassDerived`, owner-only writes |
| Status | readiness envelope and failed map live in the component (change 5); the package reports terminals via `TerminalCallback` |
| Lifecycle | Worker S1/S2; cache S3; `Record.Status` pending → generated/failed; no TTL on either bucket (catalog) |
| Ownership | one writer per bucket by catalog; several Worker goroutines race per entity, ordered by `SourceRevision` (`storage.go:396`, `:455`, `:465`) |
| Readers | Worker; cache; graph-embedding `query.go` (`GetEmbedding`, `ListGeneratedEntityIDs` + `GetEmbedding` per key, cache) |
| Writers | Storage only (above); tests use fakes |
| Recovery | restart re-delivers last value per key (P10); FAILED re-processed only from the snapshot (`worker.go:531`); cache rebuilt from the watch, invalidated on decode error (`storage.go:844`) |

Pin search closing empties: `grep -rn 'EMBEDDING_INDEX\|EMBEDDING_DEDUP' --include='*.go'` at the pin outside
`graph/`, `processor/graph-embedding`, `service/` and tests: none; tree: `graph/constants.go`, `graph/kvcatalog`,
`service/message_logger_http.go:178,351` (doc strings).

## 12. Adopter seam inventory

Reached from outside: `Embedder` (a product's own embedder is the tier-1 seam, #32), `BM25Embedder`/`BM25Config`,
`Storage`/`Worker` (today only `processor/graph-embedding`).

1. **What an `Embedder` implementer or caller must know:** (a) `Dimensions()` returns 0 until the width is known,
   never a guess, and is safe concurrently (`embedder.go:31-45`); (b) `Generate` is the document side and, for BM25,
   changes corpus statistics, while queries go through `GenerateQuery` (#96); (c) `Model()` is folded into a durable
   key, so it must change whenever vectors change; (d) the owner calls `Close`. A `Worker` builder must also know
   (e) to call `WithEmbedderType`, (f) `WithMaxSourceTextLen` (S5), (g) `WithStoreResolver` or offloaded bodies are
   excluded, (h) pass the same bucket twice (S4). Eight items: a design finding (more than two).
2. **If they do nothing:** (b) query traffic silently drifts BM25 rankings (#96); (c) a fixed `Model()` across a
   statistics change (BM25 after restart, S24) serves stored vectors from another state with no signal; (e) the
   identity's `Type` is "" — keys still differ by model, so silent but harmless; (f) inline and offloaded lanes embed
   different bytes; (g) one Warn line, then a counter; (h) the Worker watches one bucket and writes another, silently.
3. **Where they find out:** (a)–(c), (e), (f), (h): doc or nowhere; (g): log line and metric.
4. **What they should have to know:** nothing beyond implementing the interface. The gap is (b), (c), (f) and (h);
   (b) is graph/query's fix (#96), (c) is #10's BM25 question, (f) and (h) are local to this package.

Prefer observation: (a) asks the embedder to report a fact the Worker could observe from the first vector's length;
the pin already stores `len(vector)` as the record's dimensions (`worker.go:780`).

## Intent check

This change moves one boundary: neural (provider) embeddings stay out of change 4.

| Capability (`AGENTS.md` § What this is for) | State | Ruling |
| --- | --- | --- |
| Index / query with in-process BM25 vectors (tier 0) | admitted | 03B tier model; #9 6386856657 item 5 |
| Neural embeddings via a provider (`http_embedder.go`) | deferred | #9 comment 6386603659; #32, #11 |
| BM25 qualification (known answers, restart, rebuild) | deferred to 04B | #10 |

## Owner questions

Answered 2026-10-28 (PR #458 comment 6473224284): O1, this PR retires #35 and the file gets a `defer-exclude`
row naming #32; O2, this PR may retire #107 on the finding, else the Worker adopts the keyed pool (design D6).

- O1. #35's work (leave `http_embedder.go` out; tier 0 builds without `go-openai`) is done by this PR, but its body
  also lists a tier-1 → tier-0 fallback test that only Slice 04C can write. Retire #35 with this PR and leave #32 as
  the one home of the deferred file, or keep #35 open until 04C? Your answer also settles the ledger row: #35's body
  asks for an `adapt` row whose `proving_tests` name that test, while #9 comment 6386603659 makes the file a
  `defer-exclude` row naming #32.
- O2. Only if the design keeps the Worker's plain fan-out: #107's body asks the Worker to adopt the keyed pool. The
  owner's ruling on it (#9 comment 6386556850, ruling 2) says only "#107 (keyed pool): done in graph-index's PR";
  the 2026-10-24 drift note's "judge it on its own in graph/embedding's PR" is not marked as a ruling.
  If the design shows the per-entity ordering already comes from the revision guard
  and the Worker stays unkeyed, may this PR retire #107 on that finding?

## Open evidence questions

- E1. Does any deployment run with two `Worker`s on one EMBEDDING_INDEX (two processes)? The revision guard orders
  their writes; nothing else coordinates them. Not measured.
- E2. Withdrawn in round 2: S13's growth is bounded by concurrent-generation races, and the field is dropped as dead.
