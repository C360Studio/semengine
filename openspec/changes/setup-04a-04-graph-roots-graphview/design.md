# Design: setup-04a-04-graph-roots-graphview

Base: `d3c1e13` (main at the claim; `c010d0b` since touches only `AGENTS.md`). Inventory: `inventory.md` (round 1;
this design waits on its `INVENTORY PASS`). Change 4, PR 8 of 9 (#9 comment 6386856657, item 7). Tier 0 (#9 comment
6386603659; 03B D14). Draft PR #476. Governing records: ADR-081 (binding, ported here), ADR-085 Decision 5 (binding,
ported), ADR-113 and the `background-work` spec (SemEngine's), ADR-079 (consumer side only), inventory 6. Probes:
inventory 7 (G1-G5 here, P8-P12 from PR #458 comment 6472856197).

Coined terms: a *view* is a `View[T]`, one in-memory copy of a KV bucket's current values built from one watch; a
*subscription* is one local reader attached to it; a *tick* is the view's coalescing window (250 ms by default); a
*loss* is the watch's update channel closing while `Run`'s context is still live.

## Options

- **A. Port whole, `Run(ctx)` in place of `Start`/`Stop`, dead surface dropped (recommended).** 1,193 pin lines, one
  PR. Same shape as `graph/embedding.VectorCache` and `graph/follower` (inventory 5).
- **B. Carry `Start(ctx)`/`Stop()` as at the pin.** Fails the `background-work` spec (binds every later port,
  `spec.md:11-17`) and the foundation's adapt row for `View[T]` (`setup-04a-foundation/design.md:382-397`, `:423`).
- **C. `Start(ctx)` with a bounded `Stop(ctx)`.** Meets the adapt row, but `background-work` reserves start/stop pairs
  for services; `Run` is its preferred shape and removes adopter fact 1 (inventory 12: call `Stop` after a failed
  `Start`).
- **D. Do nothing now.** `processor/graph-query` (item 9) cannot build; `docs/contract.md:120` keeps naming a package
  that is absent.

## D1. Files

`pkg/graphview/`: `doc.go`, `errors.go`, `delta.go`, `subscription.go`, `view.go`, `README.md` (new: the contract
line for products, D3), tests. Outside the package: `docs/adr/081-graph-view-subscription.md` (ported, port note);
the ADR-085 port note (`:13-15`) says graphview is now ported; `docs/admission-ledger.yaml` rows for the package and
ADR-081; `docs/repository-map.md` row; `scripts/cover-check.sh` target `pkg/graphview unit` with the same entry in
`d11Targets` (`internal/harness/toolcheck/cover_test.go:46`) (03B D10 lists the package; pin unit coverage 91.4%;
`merged` only if the unit lane falls under 80% after D2, recorded on the PR); one line in
`.agents/skills/kv-or-stream/SKILL.md`: many local readers of one bucket share a `graphview.View`, never a
`WatchAll` each (ADR-081's lesson; the pin's guidance names nothing for the package, inventory 6). New import edge:
`pkg/graphview` → `internal/kvwatch` (no exported signature names it).

## D2. The view runs until its context ends (`background-work`, ADR-113)

Required: `background-work` (binds the port), the foundation adapt row, ADR-113 (pin `view.go:217` derives continuing
work from a Start context), and the completed-joins admission gate (pin `view.go:593` stops the watch without the
drain #454 needs). Observable behaviour, each with its test (D6):

- `New` is unchanged. `Run(ctx) error` refuses a nil context before touching the source. A view runs once: `Run`
  while one runs returns `ErrAlreadyStarted`; after `Run` returned, `ErrViewStopped`. A replacement is a new view, as
  at the pin (`doc.go:52-53`; `TestStartCannotReuseLiveOrStoppedView`).
- `Run` opens one `WatchAll` under a context derived from `ctx`. If it fails, `Run` returns `ctx.Err()` when `ctx`
  has ended (the view is then stopped), else the open error, the view failed (`WaitCaughtUp` and reads return the
  wrapped `ErrWatcherLost`), with nothing left running.
- `Run` reads the watch and the tick on the caller's goroutine; the only goroutines the view starts are one delivery
  goroutine per subscription, started by the attach call and joined by `Run` (the foundation's handle rule, `:95-98`).
- **Ending `ctx` is a stop, not a loss (changed).** At the pin a Start context ending reported a loss
  (`view.go:599-601`, `OnWatcherLost` fired). Here, and on the real library too (ending the watch context closes
  `Updates()`, P11), a closed channel is a loss only while `ctx` is live. On `ctx` ending, in this order: reads and
  attaches return `ErrViewStopped`; every subscription's `Err` is `ErrViewStopped` before its `Deltas` closes;
  `OnSubscribers(0)` fires if any were attached; `OnWatcherLost` does not fire; the watch stops through
  `kvwatch.Stop`; `Run` returns `ctx.Err()`.
- **On a loss**, in this order: reads and attaches return the error wrapping `ErrNotReady`, `ErrWatcherLost` and the
  cause, and keep returning it; every subscription's `Err` wraps `ErrWatcherLost` before its `Deltas` closes;
  `OnSubscribers(0)` fires if any were attached; `OnWatcherLost(cause)` fires once; the watch stops through
  `kvwatch.Stop`; `Run` returns an error wrapping `ErrWatcherLost`.
- `Run` returns only after every subscription goroutine and every hook call made by an attach or detach in progress
  has returned (pin `TestStopJoinsSubscriberCallback`), and after the watch's delivery has closed or
  `kvwatch.DrainBudget` ran out after `ctx` ended, in which case its error is joined with `kvwatch.ErrDeliveryOpen`.
  No detached context is created. A hook or decode that blocks holds `Run`; the composing component bounds its wait
  (ADR-113).
- After `Unsubscribe` or the subscription's context ends, `Deltas` closes with `Err` nil or the context's error; a
  batch already taken may still be delivered before the close (pin behaviour, `subscription.go:87-94`, now stated).
  `Unsubscribe` does not wait for the goroutine; `Run` does.

## D3. Surface

Kept, as at the pin: `New`, `Option`, `WithTickInterval`, `WithHooks`, `Hooks` (all seven fields; `OnTick` has a
production caller at the pin in the consumer adapter `processor/agentic-dispatch`, so it stays framework API, #190),
`DecodeFunc`, `EntryMeta.Created`, `WatcherSource`, `View.WaitCaughtUp`, `Get`, `SnapshotAndSubscribe`,
`Subscription` (`Deltas`, `Err`, `Unsubscribe`), `Delta`, `DeltaOp`, `Entry`, `Snapshot.Entries`/`Poisoned`,
`PoisonError`, the five sentinels. Kept with no reader, on a contract line: `Subscribe` (ADR-081 Decision 3,
delta-only attach), `List` and `KeyedEntry` (Decision 6), `Applied`, `Snapshot.Revision`, `Snapshot.AppliedAt`
(ADR-085 Decision 5). The sentinels `ErrNotReady`, `ErrViewStopped`, `ErrKeyNotFound`, `ErrAlreadyStarted` have no
reader by name and stay: each is the typed error a method's contract returns; without them a caller classifies by
text (sweep shape 5). Contract lines: the package `README.md` and the ledger `contract` field; each kept symbol with
no reader gets a test calling it as a product would (package `graphview_test`).

Changed: `Start(ctx)` and `Stop()` become `Run(ctx)` (D2). Dropped as unread with no contract line (inventory 4):
`AppliedRevision` (a second spelling of `Applied`, S2), `CaughtUp`, `View.Poisoned` (the poison set is on snapshots,
deltas and `Get`), `Snapshot.Sequence`, `EntryMeta.Revision`; `DefaultTickInterval` becomes unexported (the
`WithTickInterval` doc states 250 ms). This is new exported surface by signature (`Run`) and needs the owner's
acceptance with the proposal.

## D4. Corrections and retained findings (Bounded port scope)

Required: D2 and D3 above; S10 docs say what ships (`internal/revlag`, `graph/readiness.IndexStatusInputs`,
SemStreams' issue numbers marked as such); the tests of D6. Retained, optional, with reason:

- S1 tombstone switch (`view.go:622`): `natsclient.IsKVTombstone` would pull `natsclient`'s dependencies into a leaf
  package products import; the two lines stay.
- S3 own `ErrKeyNotFound` and `ErrAlreadyStarted`: kept, the package's error vocabulary as at the pin.
- S5 the watch loop each follower writes: inherited; #461 tracks the stop half.
- **S4 the ghost-key loop (`view.go:713-722`) and S6 the test-only tick channel (`view.go:106-110`, `:766-771`):
  optional removals, recommended.** S4 can remove nothing (inventory 9, mutation probe): a view runs once from an
  empty copy, so ADR-081 G5's outcome ("no ghost keys after re-bootstrap") holds by construction; its test is kept,
  renamed `TestReplacementViewHasNoGhostKeys`. S6 swaps a dependency for tests only; `synctest` advances the real
  ticker. Owner decision (proposal question 2).

Not affected: no subject, bucket, stream, consumer creation, payload, metric or config key. Values stay shared, not
copied (ADR-081 Decision 4); "do not mutate a delivered pointer value" stays a doc fact, a named cost.

## D5. ADRs

ADR-081 is ported with a port note (pin path and SHA; this change and ledger row; the SemStreams sites it names are
SemStreams'; not ported here: the consumers in its table; two decisions changed, named for the owner): Decision 5's
"context cancellation" as a loss becomes a stop (D2), and its re-bootstrap reconcile is met by the one-run rule (S4,
if accepted). ADR-085 Decision 5 is kept unchanged (`Applied` and the snapshot pair). ADR-079 is not ported: its
Decision 3 binds projection owners, which are consumers; the view supplies only the per-key signal. ADR-082, -083,
-084, -090, -092 name the package in passing (inventory 6).

## D6. Proving tests

Unit, `synctest` on the pin's fake source (its fake closes `Updates()` when the watch context ends, as P11 shows
nats.go does): `TestRunRefusesNilContext`, `TestRunOnce`, `TestRunLeavesNothingBehind` (run, both attach kinds,
deliver, cancel; the bubble ends), `TestRunContextEndIsAStop`, `TestRunLostWatchFailsClosed` (order of `Err`,
`OnSubscribers(0)`, `OnWatcherLost`), `TestRunOpenFailure`, `TestRunJoinsWatcherAcquisition` and
`TestRunJoinsSubscriberCallback` (the pin's two, with `synctest.Wait` in place of the 50 ms windows, S12), and the
pin's other 36 unit tests, adapted to `Run` (the four `Start` tests become the `Run` tests above), with the S11
oracle taken from the writes, not from `List`.

Generated: `TestViewHistoryMatchesModel`, seeded, records seed and run count: random sequences of put, delete, purge,
skip, poison, attach (both kinds), unsubscribe and tick against a reference model (last operation per key); each
subscriber's snapshot plus deltas equals the model at every tick, no key's revision falls, a poisoned key is never an
upsert (ADR-081 G1, G2, G4, G6). Integration, `natsfixture.Open` (#195): the pin's
`TestIntegration_GraphViewOverRealBucket`, `TestRunLeavesNoWatchDelivery` (backlog over 256 entries, G1;
`natsfixture.AwaitNativeDeliveryBlocked`), `TestRunContextEndIsNotALoss` (real watch, cancel, `-count=20`; no
`OnWatcherLost`), `TestRunReportsLostWatch` (bucket deleted). Mutation checks (`task mutate:check`): close counted
as a loss regardless of `ctx`; `kvwatch.Stop` replaced by `watcher.Stop`; the attach-sequence filter
(`view.go:840`) removed. Each named test fails on its mutant; survivors reported as survivors.

## D7. Issues, adoption and merge order

Issue #195 stays open (other packages); this PR moves its one site and comments. #9 is addressed. #461 does not name the
package; the view is a new `kvwatch.Stop` caller and is noted there. Adoption sweep (one issue, task 1.4): in the tree
`service/message_logger_kv_watch.go:137` (one `WatchAll` per SSE client) and `graph/embedding/vector_cache.go:72`
(serving projection); at their ports `processor/graph-query` `community_cache.go:69`,
`graph/clustering/enhancement_worker.go:239` (delta-only). Not a migration obligation.
Merge order: #475 (`graph/query`) shares no code with this PR, so either merges first; the second merges
`origin/main` (ledger and repository-map text). If #464 (nats.go 1.55.0) merges first, task 1.2 re-runs G1-G5 at
the new version before 5.1.
