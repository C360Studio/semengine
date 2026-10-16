# Inventory: simplify-04a-12-dispatch

base: `13e72fa72661a693a7649c9082bad62a8a591489` (origin/main `e593a28e56bf49eb847ae74f9cc20c62c0d11775` plus the empty
claim commit; draft PR #278)

Question: where does `internal/dispatch.KeyedPool` stop, what does it accept after a stop has begun, and which of its
callback, error, deps struct and sampler goroutine has a reader. Scope: #131 and #136 (epic #223, PR 8b; owner
2026-10-16, #223 comment 6216623583: "**8b — `internal/dispatch`**: #131 (work accepted after cancellation never
runs), #136."). This change brings in no package from the pin: `internal/dispatch` was ported by change 2
(`docs/admission-ledger.yaml:2019`), so the pin probe runs are not owed. Consumers are not read: no consumer can import
an `internal/` package.

## 1. The claimed gaps (re-measured)

### #131: an item accepted after the run context ends never runs

- `internal/dispatch/keyed_pool.go:246-266` — the lane loop: `case <-ctx.Done():` / `return` (`:250-251`) beside
  `case item := <-ch:` and `case <-p.drainCh:` (`:252`, `:254`).

- `internal/dispatch/keyed_pool.go:217-221` — `SubmitBlocking` checks only `if p.stopped {`, which only `Shutdown`
  sets (`:324` — `p.stopped = true`).

- `internal/dispatch/keyed_pool.go:231-234` — `select {` / `case <-ctx.Done():` / `case p.laneCh[lane] <- item:` —
  the lane channel is buffered (`:177` — `make(chan keyedItem[W], cfg.QueueDepth)`), so the send succeeds with no
  lane reading it.

- Wider than the issue's title: items already queued when the run context ends are dropped too.
  `keyed_pool.go:125-127` — "its cancellation aborts all lanes immediately (… buffered items are NOT drained)", and
  `doc.go:36-37` — "the lanes stop without processing what is still queued". `Process` and `OnPanic` run for none of
  them, and the queue-depth gauge is left counting them.

- The issue's reproduction (`TestSubmitAfterRunCancelStrandsItem`, #131 body) failed at PR #93 head `7bf1eb8` and,
  per the 2026-10-09 comment, at `88d0d3b`. The code at `:246-266` and `:216-240` is unchanged since; not re-run here.

- Real trigger (HIGH, #185): graph-ingest's pool context and submit context are siblings under its run context,
  `processor/graph-ingest/component.go:988-990` — `runCtx, cancel := context.WithCancel(ctx)` /
  `poolCtx, poolCancel := context.WithCancel(runCtx)` / `submitCtx, submitCancel := context.WithCancel(runCtx)`.
  When the `Start` context ends before `Stop`, both end together; in `SubmitBlocking`'s select both arms are ready and
  the send can win. The consume callback (`component.go:1610`) then neither acks nor naks; the server redelivers after
  AckWait. Reasoned from the code, as #131 says; not reproduced against NATS.

### #136: four pieces with no reader

1. `keyed_pool.go:350-370` — `func (p *KeyedPool[W]) metricsUpdater(ctx context.Context) {` with
   `ticker := time.NewTicker(1 * time.Second)` (`:354`), started at `:181-184`. Its only output is
   `p.metrics.queueDepth.Set(float64(depth))` (`:367`). No test reads `dispatch_queue_depth`
   (`git grep -n dispatch_queue_depth`: `doc.go:55`, `keyed_pool.go:404`, `:435`, `keyed_pool_test.go:438`, the last
   only as a key a counter squats on).
2. `keyed_pool.go:98` — `Process func(ctx context.Context, lane int, work W) error`; its only reader is
   `keyed_pool.go:298-303` — `if err != nil {` / `p.logger.Debug("dispatch: Process returned error",`. The one
   production `Process`, `processor/graph-ingest/keyed_ingest.go:202` —
   `func (c *Component) processIngest(ctx context.Context, lane int, work ingestWork) error {`, settles the message on
   every path before it returns (`:218`, `:224`). Its error is read by 25 test lines in graph-ingest
   (`git grep -n 'processIngest(' -- '*_test.go'`: `entity_writes_test.go`, `guard_generation_test.go`,
   `guard_record_test.go`, `hierarchy_birth_test.go`, `interrupted_work_test.go:280`, `keyed_ingest_test.go`,
   `poison_scoping_test.go`).
3. `keyed_pool.go:92` — `KeyOf func(W) string`; read once, `:198` — `key := p.keyOf(work)`. The one production
   `KeyOf`, `keyed_ingest.go:177` — `KeyOf:      func(w ingestWork) string { return w.entityID },`; the submit site
   already holds the key, `component.go:1602` — `entityID:    entity.ID,`.
4. `keyed_pool.go:115-118` — `type KeyedDeps struct {` with `MetricsRegistry` and `Logger`, read only by
   `NewKeyedPool` (`:151`, `:168`).

Item a (comment of 2026-10-09): wall-clock waits in `keyed_pool_test.go` — `:22` (`5*time.Second`), `:82`, `:89`,
`:201` (`case <-time.After(2 * time.Second):`), `:237` (`100*time.Millisecond`). `shutdown_test.go` in the same package
runs every test in `synctest.Test`.

## 2. Every spelling of "the pool has stopped"

- `keyed_pool.go:53` `stopped` (set by `Shutdown`, read by `SubmitBlocking`).
- `keyed_pool.go:50` `drainCh` (closed by `drain`, `:345`; read by lanes `:254` and the updater `:360`).
- `keyed_pool.go:54` `stopDone` (closed `:347`; read by `Shutdown` `:330`).
- The run context's `Done` (`:250` lanes, `:358` updater). Nothing tells `SubmitBlocking` about it: two stop facts,
  one of them invisible to the accept path. This is #131.

The stop contract is written out four times: `doc.go:34-48`, `keyed_pool.go:124-129` (`NewKeyedPool`), `:206-215`
(`SubmitBlocking`), `:306-317` (`Shutdown`).

## 3. Adjacent claims

- Open PRs (`gh pr list --state open --json number,title,isDraft,changedFiles,files`; none over 100 files):
  - #278 (this claim, 0 files).
  - #273 (draft, PR 7 `component`, 52 files) edits `processor/graph-ingest/component.go` at `:811`, `:825`, `:856`
    and `:1547` (`c.Meta().Name` to `c.name` in the `consumeStream` call). This change edits `:1610` in the same
    function, a different hunk, and does not touch `component/` or the `component-registration` spec.
  - #242–#248 are Dependabot bumps; none touches `internal/dispatch` or graph-ingest.
- Specs: no current spec names `KeyedPool` or `internal/dispatch` (`git grep -n -e KeyedPool -e internal/dispatch --
  openspec/specs` is empty). `background-work` (`openspec/specs/background-work/spec.md:11-47`, the `Shutdown(ctx)`
  shape; `:61-76`, nothing left behind, nil refused) binds the pool's stop and is unchanged. The pin's
  `openspec/specs/keyed-dispatch/spec.md` (at `8b99efe9`: same-key order, lane index, backpressure, "MUST drain
  in-flight work on graceful stop", metrics, panic recovery) was not ported; no SemEngine spec holds those invariants.
- Ledger: `docs/admission-ledger.yaml:2019-2039` (`pkg/dispatch` → `internal/dispatch`, `adapt`). Its contract
  (`:2025-2029`) and `dependencies_and_side_effects` (`:2030-2032`, "one goroutine per lane and a metrics updater")
  change with this design.
- Archived commitments: change 2 design D5 table row `dispatch.KeyedPool`
  (`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/design.md:251`) names `Shutdown(ctx)` only; task 3.6
  (`…/tasks.md:193-194`) — "the drain starts at the first `Shutdown`, so cancelling the run context leaves nothing
  running (`TestKeyedPoolRunContextCancelLeavesNothingRunning`, failing first)". Option A in the design drops that
  second sentence.
- Coverage floor: none for `internal/dispatch` (`grep -n dispatch scripts/cover-check.sh` is empty).
- `docs/repository-map.md:67` describes the package without its stop or callback shape; it stays true.

### Open issues naming the package

`gh issue list --state open --search "dispatch" --limit 200` (9 hits) and `--search "KeyedPool"` (4 hits):

| Issue | Names it in | Disposition |
| --- | --- | --- |
| #131 | title, body | retired by this PR (tasks 2.1–2.2, 3.1) |
| #136 | title, body, comment | retired by this PR (tasks 2.3–2.5, 3.2–3.3) |
| #107 | title, body (graph-index, `embedding.Worker` adopt it) | stays: change 4's packages; this narrows their API |
| #135 | body item 2 cites `shutdown_test.go:56` as a pattern | stays: work is `graph/readiness`; pattern kept |
| #195 | body: `internal/dispatch` lost its one fixture site in change 2 task 3.6 | stays: no work in this package |
| #223 | epic lines | `Addresses #223` |
| #25, #26, #27 | name the word ("action dispatch") | names the word, not the package |

## 4. Readers of the surface touched

`git grep -n 'internal/dispatch"'` (three importers; known positive `keyed_ingest.go:12` found) and
`git grep -c -w SubmitBlocking -- '*.go'`:

- Production: `processor/graph-ingest/keyed_ingest.go:173-188` (`NewKeyedPool`, `KeyOf`, `Process`, `OnPanic`,
  `KeyedDeps`), `component.go:557` (field), `:1024` (`buildIngestPool(poolCtx)`), `:1139-1144`
  (`c.ingestSubmitCancel()`, `c.ingestPool.Shutdown(ctx)`, `c.ingestPoolCancel()`), `:1610` (`SubmitBlocking`).
- Tests outside the package: `processor/graph-ingest/lifecycle_owner_test.go:34-52`,
  `interrupted_work_test.go:196`, `:254`, `:382`. Every graph-ingest test that builds a pool shuts it down:
  `config_fields_test.go:148`, `:183`; `guard_generation_test.go:230`; `interrupted_work_test.go:355`;
  `statement_metadata_test.go:76`; `lifecycle_owner_test.go` through `owner.Stop`.
- In-package tests: `keyed_pool_test.go` (19 `SubmitBlocking` lines), `shutdown_test.go` (7).
- graph-ingest's lane disposition when the pool context has ended: `keyed_ingest.go:206` calls
  `natsclient.ValidateHeartbeatDeliveryPolicy(ctx, …)`, which refuses an ended context
  (`natsclient/delivery_settlement.go:165` — `if err := ctx.Err(); err != nil {`), and `keyed_ingest.go:218` naks:
  `result := natsclient.SettleDelivery(work.msg, natsclient.DeliveryDecisionRetry, err)`.

## 5. Problem shape

Bounded dispatch with one stop: background work in the `Shutdown(ctx)` shape whose accept path and stop are one
handoff. Closest instance: `internal/cache/coalescing_set.go:196-217` (`Shutdown` refuses nil with
`errs.WrapInvalid`, and returns nil when its goroutine has exited even if ctx has ended, `:203-208`). It also keeps a
run-context arm (`:226`), but its pending keys stay readable through `Drain` (`:180`), so no accepted item becomes
unreachable. `context.AfterFunc` is in the tree at `natsclient/socket.go:54` (option B in the design). No new primitive
is established; `KeyedPool` is the existing one (ADR-072), so no adoption sweep is owed beyond #107. The collision
table and the intent check are not triggered: no new durable, communication or coordination primitive, and no
boundary moves.

## 6. Pin decision records

Every pin ADR (85 files) fetched at `8b99efe9` and searched for `pkg/dispatch`, `KeyedPool`, `SubmitBlocking`,
`keyed_pool`, `keyed pool`, `pkg/worker`; titles read for pool, worker, concurrency, dispatch, lane, shutdown.

- **ADR-072** (Keyed-concurrent entity ingest for graph-ingest) governs the package. It decides: a generic
  keyed-ordered pool in `pkg/dispatch` (`:131-133`); bounded per-lane queues and `SubmitBlocking` (`:174`); the
  primitive recovers panics in `Process` and keeps the lane alive (H2, `:166-170`); a failed submit must Nak (B1,
  `:165`); the stop order "cancel the submit ctx first … drain lanes … then close the KV store" and "no accepted
  (nil-return) submit can be stranded by a lane exiting first" (M3, `:212-220`); head-of-line blocking is a known
  limit (M2, `:221-223`); lane queue-wait, in-flight and queue-depth metrics (`:185-186`). Not ported to `docs/adr/`;
  cited by `docs/adr/108-processor-shell.md:104`, `110-write-seam.md:41`.
- **ADR-048** (BoundedDispatcher + `.triples` Substrate Primitives): `pkg/dispatch` wrapping `pkg/worker.Pool`
  (`:89-103`). Governs only the dropped `BoundedDispatcher` (change 2 D6, ruling E).
- ADR-046 (`:170`, `:261`, `:277`), ADR-050 (`:127`) and ADR-066 (`:128`, `:228`, `:281`) name `pkg/dispatch` or
  `pkg/worker` as a dependency or a layout precedent; none decides anything about `KeyedPool`.

## 7. Shape sweep (current tree)

`doc.go`:

1. Second spelling: the stop contract (§ 2), and `KeyOf`/`KeyedDeps` in the example (`:22-29`) and metrics text
   (`:52`). Required where D1–D4 change it; the text follows the code.

`errors.go`:

1. `:7-11` names "a missing KeyOf" in `ErrInvalidConfig`'s comment. Required with D2.

`keyed_pool.go`:

1. Two stop paths (`:246-266`, `:350-370`; § 1). Required: #131, and ADR-072 M3's "no accepted submit can be
   stranded".
2. Copied shell: the ticker goroutine `metricsUpdater` (`:350-370`) beside the lanes. Required: #136 item 1.
3. Layered interface with one implementation: none (no interface declared).
4. Exported surface with no reader: `Process`'s error (`:98`), `KeyOf` (`:92`), `KeyedDeps` (`:115`). Required:
   #136 items 2–4.
5. Classification by error text: none (`err.Error()` at `:302` only feeds a log).
6. Not in either issue: `Shutdown`'s final `select` (`:329-336`) picks at random between `stopDone` and an ended
   ctx, so a `Shutdown` after the join, under an ended ctx, can return `ctx.Err()`. MEDIUM (#185: needs a caller to
   pass an ended ctx after a completed join; graph-ingest clears its handle after a successful release,
   `component.go:1113`). It sits in the function D1 rewrites; the design folds it in, mirroring
   `coalescing_set.go:203-208`, rather than filing it.
7. Optional, stays: the nil-ctx refusal at `:319-321` is a bare `errors.New`, where `NewKeyedPool` wraps
   `ErrInvalidConfig` (`:135-136`) and `coalescing_set.go:198` uses `errs.WrapInvalid`. `Logger` nil →
   `slog.Default()` (`:151-154`) is the tree's pattern (13 production sites).

`keyed_pool_test.go`:

1. Wall-clock waits (§ 1, item a). Required: #136 item a.
2. Optional: `TestKeyedPool_ProcessUsesConstructorContext` builds its context with `context.WithValue` (`:58`) only to
   recognise it; the identity check `got != runCtx` (`:78`) already proves it. The design drops the value.

`shutdown_test.go`:

1. `:128-146` `TestKeyedPoolRunContextCancelLeavesNothingRunning` holds the contract option A drops; it gains a
   `Shutdown` (#131: "`shutdown_test.go:130` gaining a `Shutdown`").

Defects outside #131 and #136: item 6 above, folded into D1. Searches: every file of the package read in full; the
readers in § 4.

## Adopter seams

`internal/dispatch` cannot be imported outside this module; the adopter is a SemEngine package author (graph-ingest
today, graph-index and `embedding.Worker` under #107).

1. What they must know today: to cancel the submit context before `Shutdown` (`keyed_pool.go:211-215`); that ending the
   pool context drops queued work; that `Process`'s error goes only to a debug log; to supply `KeyOf`. After the
   design: cancel the submit context first, and call `Shutdown`.
2. If they do nothing: today, a pool context that ends first silently loses accepted items until AckWait. After
   option A, a pool never shut down keeps its lane goroutines until the process ends; a `synctest` test of the
   owner fails with a deadlock report (`background-work` "Nothing left behind").
3. Where they find out: today, nowhere. After: a `synctest` deadlock report in their test, and the compiler for D2–D4.
4. The gap: `Shutdown` must be called, which the `background-work` `Shutdown(ctx)` shape already requires of every
   such type.
