# Inventory: setup-04a-04-graph-roots-graphview

base: `d3c1e131d3cfa10b0d01d895016bb0945d7aecbf` (main at the claim; the claim commit `364c0a5` on
`claude/port-graphview` adds no file; `origin/main` has since moved to `c010d0b`, which touches only `AGENTS.md`).
Pin paths are SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, written `pin:<path>:<line>`, read from a fresh
pin tarball at `/tmp/gv-pin/src` (local only). All 15 files of `pkg/graphview` there are blob-equal to
`gh api repos/C360Studio/semstreams/contents/pkg/graphview?ref=8b99efe9` (`git hash-object` equals the API `sha` for
each). SemStreams `main` is identical to the pin (`compare/8b99efe9...main`: `identical`, ahead 0), so no unmerged
work touches the package. Consumer cites: semboids `8c03cc53836ced93a5df7064473c63ff144e64f1` via `gh api … ?ref=`
(the SHA SETUP 03B §5.3 cites), read for "the workload it drives" (`docs/inventory-scope.md:18`).

Status: round 2, inventory only (round 1 FAIL at `1049ab1`: the pin's capability spec added as § 6a; G6 probed;
semboids' shutdown evidence in § 12; #475 re-listed).

Kind of change: a port of one package, by the owner's order ruling (#9 comment 6386856657, item 7: "`pkg/graphview`;
second-lane capable") under the change 4 scope ruling (#9 comment 6386603659:
tier-0 halves only). The package has no provider code; nothing is excluded.

Briefing hypotheses checked:

- "No open issue names graphview": two do. #195 (natsfixture adoption) names `pkg/graphview` in its 2026-10-24 drift
  comment (one `natsclient.NewTestClient` site at the pin, `pin:pkg/graphview/integration_test.go:43`); epic #9 names
  it in the order ruling. § 3.
- "graph-query adopts it": at the pin `processor/graph-query` reads it for one thing, the COMMUNITY_SUMMARIES serving
  view (`pin:processor/graph-query/summary_view.go:137`), whose records are LLM summaries
  (`summary_view.go:39-45`) written only by `graph/clustering.EnhancementWorker`, which requires an
  `*LLMSummarizer` (`pin:graph/clustering/enhancement_worker.go:55`, `:105-109`). The package's tier-0 admission
  rests elsewhere: SETUP 03B design D14 admits it as "the view layer of change observation", with semboids as the
  consumer (`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:220`, `:619-621`, `:720`). § 4.
- "The embedding vector cache was not converted": correct; ADR-081 marks it "Convert"
  (`pin:docs/adr/081-graph-view-subscription.md:202`) and neither the pin nor PR #458 converted it. § 5.

## Pin probe

go1.26.4 darwin/arm64, on the pin copy, unit tests only (the one integration test needs Docker).

- `go test -race -count=1 -p 1 -cpu 1 -cover ./pkg/graphview`: ok, 1.597 s, coverage 91.4%.
- `go test -count=5 -p 1 -cpu 1 -shuffle=on -v ./pkg/graphview`: ok, 1.529 s, `-test.shuffle 1793360270761281000`.
- Lifecycle suite: not applicable; the package holds no service (`View[T]` is background work, § 8).
- Test-text rules: no `time.Sleep`, no `Skip`, one build tag (`integration`, `integration_test.go:1`). Wall-clock
  waits: 21 `time.After(testWait)` failure bounds in `harness_test.go`, `hooks_test.go`, `meta_test.go`,
  `view_test.go`, `stop_join_test.go`, and two negative windows that assert "has not returned within 50 ms"
  (`stop_join_test.go:64`, `:117`), the elapsed-wall-clock shape AGENTS.md's #392 row refuses.
- Mutation probe (copy at `/tmp/gv-pin/mut`): deleting the ghost-key loops in `markCaughtUp`
  (`pin:pkg/graphview/view.go:713-722`) leaves the whole suite passing, including
  `TestFreshReplacementReconcilesGhostKeys` (`go test -count=1 ./`: ok). § 9, S4.

## 1. The claimed gap

`pkg/graphview` is absent: `git grep -n graphview -- '*.go'` at base returns nothing; `docs/repository-map.md` has no
row; `docs/admission-ledger.yaml` has no row (`grep -n graphview` empty). It is named, not present, in
`docs/contract.md:120` ("The engine provides a view helper (`pkg/graphview`) that gives you a snapshot followed by a
stream of changes"), `docs/tier1-cross-check.md:62`, and the port notes of ADR-083 (`:14`) and ADR-085 (`:15`).

## 2. Every current spelling of the facts the package models

| Fact | Spellings in the tree at base | Pin spelling |
| --- | --- | --- |
| KV entry is a tombstone | `natsclient/kv.go:739` `IsKVTombstone`, used by `graph/follower/follower.go:263`, `graph/embedding/vector_cache.go:114`, `graph/readiness/watcher.go:307`, `pkg/lifecycle/manager_query.go:261`, `processor/graph-ingest/poison_inventory.go:407` | `pin:pkg/graphview/view.go:622` `case jetstream.KeyValueDelete, jetstream.KeyValuePurge:` (a second spelling) |
| Stop a KV watch and wait for its delivery | `internal/kvwatch/kvwatch.go:36` `Stop` (follower, embedding) | `view.go:593` `defer func() { _ = watcher.Stop() }()` (no drain) |
| Lifecycle already-started | `pkg/errs/errs.go:43` `ErrAlreadyStarted` | `pin:pkg/graphview/errors.go:30` own `ErrAlreadyStarted` |
| Key not found | `pkg/errs/errs.go:64` `ErrKeyNotFound`; `jetstream.ErrKeyNotFound` | `errors.go:27` own `ErrKeyNotFound` |
| Revision watermark + write time | `internal/revlag` `IndexedAt` (pin `pkg/revlag`) | `view.go:344` `AppliedRevision` and `view.go:393` `Applied` (two spellings inside the package) |
| Serving projection over a KV watch | `graph/embedding/vector_cache.go:61` `VectorCache.Run` | `View[T]` |

## 3. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,changedFiles,files` at 2026-10-30:

- #476 (this claim), 0 files.
- #475 `feat(query): port graph/query (change 4, PR 7 of 9)`, draft, 5 files, all under
  `openspec/changes/setup-04a-04-graph-roots-query/` (no overlap). Its `design.md:153-154` states the same order:
  whichever of #475 and #476 merges second merges `origin/main` and re-runs `task verify`. At the pin
  `graph/query` does not import `pkg/graphview` and `pkg/graphview` does not import it (`grep -rn graphview
  graph/query` empty). Expected shared files: `docs/admission-ledger.yaml`, `docs/repository-map.md` (text only).
  ADR-081 lists `graph/query/client.go:221` as "convert only in multi-reader processes"; the pin never converted it.
- #464 `chore(deps): bump nats.go from 1.54.0 to 1.55.0` (Dependabot): `go.mod`, `go.sum`. § 7's probe is at
  v1.54.0 and `internal/kvwatch`'s doc names v1.54.0 behaviour.
- #465 markdownlint bump: no overlap.

### Open issues naming the package

Queries: `gh issue list --state open --search "<q>" --limit 200` for `graphview`, `ADR-081`, `081`, `kvwatch`,
`SnapshotAndSubscribe`, `WatcherSource`, `fan-out`, `COMMUNITY_SUMMARIES`, `summary view`, `AGENT_LOOPS`, `view`,
`watcher`, `Subscription`, `semboids`.

- **#195** adoption: `natsfixture.Open` for each package's test client. Names `pkg/graphview` (drift comment
  2026-10-24: one site). Kept.
- **#9** epic: names it in the order ruling. Kept (epic; `Addresses`, never `Closes`).
- **#461** adoption: `internal/kvwatch` at the other KV-watch stop sites. Does not name the package; the port adds a
  new `kvwatch.Stop` caller (§ 2). Listed for the design.
- Names the word, not the package: #105, #106 (graph-query, "summary view" / "view"), #103, #110 ("watcher"), #83
  ("Subscription"), #361, #141, #26, #25, #7, #300 ("semboids").

### Specs, ledger, records

- Specs that bind the port: `background-work` (non-service background work stops as `Run`/`Close`/`Shutdown`;
  `openspec/specs/background-work/spec.md:11-17`), `harness-boundaries` (test text; consumer creation
  `:1035-1040` — `WatchAll` is not a listed creation call), `nats-fixture`. No capability spec covers the package.
- Foundation design: `pkg/graphview` is a change-4 package (`setup-04a-foundation/design.md:167-170`), `View[T]` is a
  lifecycle owner with a non-context `Stop` adapt row (`:103`, `:382-397`, `:423`), and `graphview.Subscription[T]` is
  a handle, not an owner: "the resource is created and ended by the owner that issued the handle" (`:95-98`).
- 03B: D10 extended critical list (80% coverage) includes `pkg/graphview` (`setup-03b…/design.md:467`); D14 admits
  it at tier 0 (`:619-621`).
- Admission ledger: no row.

## 4. Who reads the surface

Pin-wide non-test grep outside the package; semboids at `8c03cc53`.

Pin importers: `processor/graph-query` (`summary_view.go`, `component.go`), `processor/agentic-dispatch`
(`http_activity.go`, `component.go`; agentic, moved to semteams: a consumer adapter), `pkg/fusion/engine_graph.go:62`
(comment only). semboids: `internal/api/graphstream.go`, `graphstream_views.go`, `graphstream_decode.go`,
`graphstream_metrics.go`, `service.go`.

| Symbol | Readers |
| --- | --- |
| `New`, `WithHooks`, `Hooks`, `View.Start`, `View.Stop`, `DecodeFunc`, `EntryMeta` (`.Created`) | graph-query, agentic-dispatch, semboids |
| `View.Get` | graph-query `summary_view.go:296` |
| `WatcherSource` | agentic-dispatch `http_activity.go:43`, semboids `graphstream_views.go:82` |
| `WithTickInterval` | semboids `graphstream_views.go:145` |
| `View.WaitCaughtUp` | agentic-dispatch `:279` |
| `View.SnapshotAndSubscribe`, `Snapshot` (`.Entries`, `.Poisoned`), `Entry`, `Subscription` (`Deltas`, `Err`, `Unsubscribe`), `Delta` (all fields), `DeltaOp` consts, `PoisonError` | agentic-dispatch, semboids `graphstream.go:181`, `:215`, `:241-247` |
| `ErrWatcherLost` | agentic-dispatch `:280`, `:301` |
| Hooks fields `OnApply`, `OnCaughtUp`, `OnWatcherLost`, `OnPoison`, `OnSubscribers`, `OnFanOut` | semboids `graphstream_metrics.go:92-116`; agentic-dispatch `:139-185` |
| `Hooks.OnTick` | none in production: agentic-dispatch `http_activity.go:165-168` forwards it to `activityTestHooks`, test synchronization zero in production (`component.go:139-141`); dropped as a test back door (Codex design review, PR #476 comment 6522851399) |
| **No reader**: `View.Subscribe`, `View.List`, `KeyedEntry`, `View.CaughtUp`, `View.AppliedRevision`, `View.Applied`, `View.Poisoned`, `DefaultTickInterval`, `Snapshot.Sequence`, `Snapshot.Revision`, `Snapshot.AppliedAt`, `EntryMeta.Revision`; sentinels `ErrNotReady`, `ErrViewStopped`, `ErrKeyNotFound`, `ErrAlreadyStarted` by name | — |

Contract lines for unread symbols: ADR-081 Decision 3 names delta-only attach (`Subscribe`), Decision 6 names
"`Get`/bounded list" (`List`), Decision 5 names the caught-up watermark; ADR-085 Decision 5 names the applied-revision
and write-time pair "as one atomic pair, carried on snapshots as well" (`Applied`, `Snapshot.Revision`,
`Snapshot.AppliedAt`; tree `docs/adr/085-gate-on-health-report-freshness.md:115-119`). None names `CaughtUp`,
`AppliedRevision`, `View.Poisoned`, `DefaultTickInterval`, `Snapshot.Sequence` or `EntryMeta.Revision`.

## 5. The problem shape

- **Serving projection over one KV watch, fanned out to local readers.** Closest in the tree:
  `graph/embedding/vector_cache.go:61` (`Run(ctx)`, `kvwatch.Stop`, withdraws authority on loss) and
  `graph/follower/follower.go:190` (`Run(ctx)`). Both are `background-work`'s `Run` shape. No tree code fans one
  watch out to many readers.
- **Per-client watch** (the trap ADR-081 retires): `service/message_logger_kv_watch.go:137` opens one `WatchAll` per
  SSE client; ADR-081's table marks it "Convert" (`pin:docs/adr/081-graph-view-subscription.md:201`).
- Adoption candidates once the view is in the tree (ADR-081 table, re-checked here): `service/message_logger_kv_watch.go:137`
  (per-client SSE), `graph/embedding/vector_cache.go:72` (serving projection), and at their ports
  `processor/graph-query` `community_cache.go:69`, `graph/clustering/enhancement_worker.go:239` (delta-only).

## 6. Decision records at the pin, and their state here

Search: all 85 records at the pin, `grep -l` for `graphview`, `pkg/graphview`, `SnapshotAndSubscribe`,
`WatcherSource`, `PoisonError` → 081, 082, 083, 084, 085, 090, 092; then ADR-079 (cited by the package's doc,
`pin:pkg/graphview/doc.go:13`, `errors.go:33`). Each read.

| ADR | Decides (for this package) | Implemented by pin code here? | In tree |
| --- | --- | --- | --- |
| 081 graph view subscription | the package: one `WatchAll`, decode once, coalesce per tick, snapshot+delta at one sequence, per-subscriber LWW backpressure, fail closed, ghost-key reconcile on re-bootstrap, point reads; coexists with raw `WatchAll` | yes — binding | not ported |
| 085 gate on health, report freshness | D5: the view tracks the write time of the newest applied revision beside the revision, one atomic pair, on snapshots too; no API gates on it (`view.go:357-397`) | yes — binding | ported; port note `:15` says graphview not ported |
| 079 per-entity poison | D3: projection owners keep sticky reset-required semantics; the view only supplies the per-key signal (ADR-081 G6) | the signal only; reset is the consumer's | not ported |
| 083 readiness as distributed state | names graphview as the coherent-snapshot surface (`:174`) | no | ported |
| 084 readiness licenses health | mentions graphview (`:163`) | no | ported |
| 082 bounded-lag readiness | rejected a shared predicate "reused by graphview G5" as premature (`:104`) | no | not ported |
| 090 materialized views | rejects "extending `pkg/graphview` into a general durable-projection runtime" (`:66`) | no (a non-goal) | not ported |
| 092 lifecycle poison | admits no `pkg/graphview` dependency for lifecycle (`:31`) | no | not ported |
| 113 Start context (SemEngine) | a lifecycle owner's run context outlives Start; Stop is the only stop | pin violates it: `view.go:217` `runCtx, cancel := context.WithCancel(ctx)` and `:200-202` (Start's ctx ending is a watcher loss) | accepted |

## 6a. The pin's capability spec

`pin:openspec/specs/graph-view-subscription/spec.md` (291 lines; `grep -c '^### Requirement'`: 13 requirements). The
pin tests cite it by requirement (`pin:pkg/graphview/stop_join_test.go:45`: `// spec: graph-view-subscription / View
lifecycle and ownership`). It, not ADR-081, counts context cancellation as a loss (`:147-150`; ADR-081 has no
"cancel", `grep -ci cancel` 0), matching `pin:pkg/graphview/view.go:200-202`. Each requirement against the staged
spec (`specs/graph-view-subscription/spec.md`):

| Pin requirement (line) | Disposition | Proving test |
| --- | --- | --- |
| Single-watcher shared projection (`:16`) | carried: "Each write is decoded once". Its trusted-decode clause is moot: `UnmarshalEntityStateTrusted` is unexported in graph-ingest here (`simplify-04a-13-graph/design.md:144`) | new `TestDecodeOncePerWrite` (no pin test counts decodes) |
| View-rate coalescing (`:36`) | merged into "Snapshot and changes form one history" | `TestCoalescingLWWWithinWindow`, history test |
| Tombstone propagation (`:63`) | carried | `TestDeleteDeliveryToAttachedSubscriber` |
| Snapshot-and-delta consistency (`:81`) | merged into the history requirement | `TestSnapshotDeltaSeamScripted`, history test |
| Read-after-write coherence (`:102`) | merged into the history requirement | `TestRacingAttachNeverObservesPreAppliedState`, `TestTickSeamNoStaleDeliveryToLateAttacher` |
| Readiness gate on initial replay (`:128`) | carried; "expose caught-up state and watermark" (`:132-134`) is carried by `WaitCaughtUp`, the `OnCaughtUp` hook and `Applied` | `TestBootstrapGatesAttach`, `TestAppliedAdvancesWithTheRevisionWatermark` |
| Watcher loss fails closed (`:147`) | carried, **changed**: context cancellation leaves the loss list and becomes a stop; re-bootstrap without ghost keys (`:162-165`) holds because a view runs once | `TestRunLostWatchFailsClosed`, `TestRunContextEndIsAStop`, `TestRunOnce` |
| Decode failure surfaces as poison (`:167`) | carried (the latch scenario: `*PoisonError` unwraps to the decode error) | `TestPoisonSurfacedNotDelivered` (`require.ErrorAs`, `delta_test.go:171`) |
| Per-subscriber at-most-once backpressure (`:188`) | carried | `TestSlowSubscriberBackpressure` |
| Coherent point reads (`:208`) | carried | `TestGetAndListSurface`, `TestRacingAttachNeverObservesPreAppliedState` |
| View lifecycle and ownership (`:221`) | carried as the `Run` requirements plus "Detach releases its state" | `TestUnsubscribeReleasesState`, `TestSubscriberContextCancelDetaches`, `TestRunLeavesNothingBehind` |
| Coexistence with raw WatchAll (`:237`) | carried | review-only: no wrong change in the view fails a test of it |
| The view reports its currency (`:248`) | carried | `TestAppliedPairIsNeverTorn`, `TestAppliedIsZeroBeforeFirstApply`, `TestSnapshotCarriesAppliedAt` |

None dropped. The trusted-decode clause is the only text not carried, and nothing here can call that API.

## 7. Library behaviour the port relies on (#170, ADR-112)

Already probed at nats.go v1.54.0 / nats-server v2.15.1 on PR #458 comment 6472856197: P8 (empty bucket: first entry
is the nil marker), P9 (`Updates()` closed after `Stop`), P10 (snapshot = last value per key; a deleted key arrives as
one delete op), P11 (watch context cancelled: `Updates()` closes; later `Stop` returns `ErrBadSubscription`), P12
(`WatchAll` with an ended context errors).

New probe, `/tmp/gv-probe/main.go` (embedded server, file store, `go run .`, go1.26.4), at the same versions:

| # | Statement | Result |
| --- | --- | --- |
| G1 | `Updates()` buffer | capacity 256 |
| G2 | snapshot order and markers | entries in ascending revision (`b@1 a@4 c@5(del) d@7(purge)`), then nil; purge arrives as `KeyValuePurgeOp` |
| G3 | live order across keys, with delete and purge | ascending (`x@8 a@9 x@10 a@11(del) x@12(purge)`); every delivered revision ascending in this run |
| G4 | `Created()` on delete and purge markers | non-zero |
| G5 | a second nil marker or extra entry within 500 ms after live entries | none |
| G6 | `DeleteKeyValue` under a live `WatchAll` (`/tmp/gv-probe-g6/main.go`, one run each: embedded v2.15.1, and the repo's image `nats:2.15.0-alpine@sha256:ac8f88a6…d8d4` via `NATS_URL`) | `Updates()` closes 20.0 s after the delete on both, after nats.go logs "stream not found: recreating ordered consumer"; a later `Stop` returns `nats: invalid subscription` |

G6's 20 s comes from missed heartbeats and the failed consumer recreate (`js.go:2356-2358`, `:2400-2414` at
v1.54.0); the delay is not promised. G3 is one run: ascending delivery is the ordered consumer's purpose but no
tree test drives a reorder; ADR-081 G2 states it as an assumption. G5 is bounded by 500 ms: "exactly one nil
marker" is not promised. The pin code tolerates both (`view.go:647` guards the watermark by revision; `:709` ignores a
second marker).

## 8. Lifecycle owners, NATS use, payloads

- **`View[T]`** (`view.go:139`): not a service, so `background-work` binds. `Start(ctx)` (`:203`) derives its run
  context from `ctx` (`:217`), starts a ticker goroutine (`:225`) and a watcher goroutine (`:261`); `Stop()` (`:269`)
  takes no context and waits on `wg.Wait()` with no bound (`:302`); a stopped view refuses `Start` (`:210-212`).
  Goroutines call caller callbacks (decode `:625`, hooks `:636`, `:673`, `:726`, `:756-759`, `:818-822`), so `Close`
  does not qualify (`background-work` spec `:34-49`). It holds `cancel` (allowed) and no context. No
  `context.Background`, `TODO`, `WithoutCancel`, `WithValue` or `select {}` in production code (grep).
- **`Subscription[T]`**: one delivery goroutine per subscription (`view.go:490`, `:514`), joined by the view's
  `wg` (`subscription.go:73`); ends on `Unsubscribe`, the subscription's context (`:79-81`, which calls the
  `OnSubscribers` hook through `removeSub`), view stop, or loss. A handle, not an owner (foundation `:95-98`).
- Terminal actions traced (callback order at the pin):
  1. `Stop`: state stopped, every subscription `Err` = `ErrViewStopped` then `Deltas` closes, `OnSubscribers(0)`,
     cancel, join (`:276-302`); the watcher goroutine then sees `ctx.Done`, `failClosed` returns early on stopped
     (`:736`), no `OnWatcherLost`; `watcher.Stop()` without drain (`:593`).
  2. Updates channel closes: `failClosed` (`:734`): state failed, pending cleared, every subscription `Err` = wrapped
     `ErrWatcherLost` then close, `OnSubscribers(0)` if any, then `OnWatcherLost(cause)`; the view stays failed until
     `Stop`.
  3. Start's context ends without `Stop`: same as 2 with cause "watch context cancelled" (`:599-601`), so a
     shutdown is reported as a loss and `OnWatcherLost` fires.
  4. `WatchAll` fails: state failed, `Start` returns the error, the ticker keeps running until `Stop` (`:230-240`).
  5. Subscription context ends: the delivery goroutine calls `detach` → `removeSub` → `OnSubscribers(n)`
     (`subscription.go:79-81`, `view.go:544-556`).
  6. `Err` is set before `done` closes and `Deltas` closes after `done` (`subscription.go:59-67`, `:73-81`), so a
     reader that sees the close reads the reason. A batch already taken may still be sent after `Unsubscribe`
     returns (`:87-94`, select between send and `done`).
  Port hazard: under a `Run(ctx)` whose watch context derives from `ctx`, ending `ctx` closes `Updates()` (P11), so
  the pin's "channel closed" branch would misreport every shutdown as a loss unless the close is classified by
  whether `ctx` has ended.
- NATS: one `WatchAll` on an injected `WatcherSource`; no stream, consumer creation call, subject, or bucket
  creation. `jetstream.KeyValue` and `graph/kvcatalog.CatalogReader` (`graph/kvcatalog/kvcatalog.go:277`) satisfy
  `WatcherSource`.
- Payloads: no `message` import, no `GenericJSON`. Metrics: none; `Hooks` is the seam consumers build metrics on.
- Config: options only (`WithTickInterval`, `WithHooks`); a non-positive interval silently becomes the default
  (`view.go:183-185`).

## 9. Shape sweep (pin copy, every non-test file)

| # | Shape | Pin site | Finding |
| --- | --- | --- | --- |
| S1 | second spelling | `view.go:622` `case jetstream.KeyValueDelete, jetstream.KeyValuePurge:` | tombstone test beside `natsclient.IsKVTombstone` |
| S2 | second spelling | `view.go:344` `func (v *View[T]) AppliedRevision() uint64 {` | duplicates `Applied` (`:393`); its own doc says use `Applied` |
| S3 | second spelling | `errors.go:27`, `:30` | own `ErrKeyNotFound`, `ErrAlreadyStarted` beside `pkg/errs` |
| S4 | dead code | `view.go:713-722` `for k := range v.entries { if _, ok := seen[k]; !ok {` | a view starts once from an empty projection, every bootstrap apply marks `seen` (`:666-668`), and `applySkip` deletes; the loop can remove nothing (mutation probe) |
| S5 | copied shell | `view.go:591-615` `runWatcher` | the watch loop each KV follower writes (follower, VectorCache, readiness watcher) |
| S6 | back door (unexported, test only) | `view.go:106-110` `func withTickChannel(ch <-chan time.Time) Option {` and `:766-771` | swaps the ticker for tests; `synctest` time makes it unnecessary |
| S7 | dead surface | § 4 "No reader" row | see § 4 contract lines |
| S8 | layered interface | `view.go:42` `WatcherSource` | not a finding: two production implementers (jetstream.KeyValue, kvcatalog.CatalogReader) and consumers inject it |
| S9 | error text | none (`grep -n 'Error()\|strings\.'`: only `errors.go:49` and `view.go:446` `HasPrefix`) | — |
| S10 | doc drift | `view.go:360` `pkg/revlag.Watermark.IndexedAt`; `:384` `graph.IndexStatusInputs`; `doc.go:7` `gh#579`, `:19` `PR #583` | revlag is `internal/revlag`; `IndexStatusInputs` is in `graph/readiness`; issue numbers are SemStreams' |
| S11 | test oracle | `coherence_test.go:216-224` | the concurrent seam test takes its expected state from the view's own `List` |
| S12 | test timing | `stop_join_test.go:64`, `:117` | "not returned within 50 ms" assertions (#392 rule) |

## 10. Size against the one-package rule

Non-test pin lines: `view.go` 863, `subscription.go` 117, `delta.go` 97, `doc.go` 62, `errors.go` 54 = 1,193. Tests
2,072. Under the 3,000 bound; no waiver.

## 11. Collision table: in-process projection over a KV watch

| Dimension | Evidence |
| --- | --- |
| Semantic class | hold a bucket's current state in memory from one watch and serve it to readers |
| Owners | `graph/embedding.VectorCache` (`vector_cache.go:61`, EMBEDDING_INDEX vectors); `graph/follower.Follower` (`follower.go:190`, ENTITY_STATES status, no values); `graph/readiness.Watcher` (`watcher.go:274`, one key) |
| Catalogs | `graph/kvcatalog` names buckets; the view takes an injected source and names none |
| Status | VectorCache `FindSimilar` false when not authoritative; follower envelope; the view: `ErrNotReady` / `ErrWatcherLost` |
| Lifecycle | all three `Run(ctx)`; VectorCache and follower stop through `kvwatch.Stop`; the pin view `Start`/`Stop` |
| Ownership | each owns one watch; no claims or leases |
| Readers | VectorCache: graph-embedding (change 5), graph/query; follower: graph-index; view: § 4 |
| Writers | none write the bucket |
| Recovery | VectorCache and the view rebuild by replay on the next run or a new instance; no persisted state |

## 12. Adopter seam inventory

The adopter is a component author or a product (semboids) who has not read `view.go`.

1. **Must know** (pin shape): call `Start`, then `Stop` even when `Start` failed (the ticker runs until `Stop`,
   `view.go:225`, `:230-240`); after a loss, build a new view (a failed view never recovers, `doc.go:52-53`); wait for
   `WaitCaughtUp` or handle `ErrNotReady`; drain `Deltas` until closed, then read `Err`; never mutate a delivered
   value of pointer type (`delta.go:37-40`); hooks must not block (`view.go:51-53`); decode runs once per write on
   the watcher goroutine. Seven facts.
2. **If they do nothing**: a failed `Start` without `Stop` leaks the ticker goroutine; a lost view returns typed
   errors forever (not silent); a mutated shared value corrupts every subscriber's copy silently.
3. **Where they find out**: `ErrNotReady`/`ErrWatcherLost` are typed runtime errors; the `Stop`-after-failed-`Start`
   rule and value immutability are doc only (findings by the contract's ranking).
4. **What they should know**: run the view, wait for caught-up, read deltas until closed. graph-query wrote a
   supervisor that constructs, starts, waits for loss through `OnWatcherLost`, stops and retries
   (`summary_view.go:108-196`). semboids does not rebuild: `supervise` returns after its first success
   (`graphstream_views.go:120-133`), and its `stop()` cancels the context before `view.Stop()` (`:157-170`), so at
   the pin every semboids shutdown is reported as a loss and bumps its `watcherLost` metric
   (`graphstream_metrics.go:99-104`): consumer evidence for treating a context end as a stop.

## Intent check

No boundary moves: the package is admitted at tier 0 by 03B D14 and ordered by #9 comment 6386856657; nothing is
excluded or deferred. Table not triggered.

## Open evidence questions

- Whether nats.go v1.55.0 (#464) changes G1-G5 or P8-P12: unprobed.
- semteams' current use: GitHub code search finds `graphview` only in semteams docs; its agentic-dispatch copy was not
  inventoried (scope table limits semteams to `processor/rule`, `pkg/lifecycle` and the agentic seams).
