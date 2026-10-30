# Design: setup-04a-04-graph-roots-graphview

Base: `d3c1e13`; round 1 committed at `1049ab1`. Inventory: `inventory.md` (round 2). Change 4, PR 8 of 9 (#9
comment 6386856657, item 7). Tier 0 (#9 comment 6386603659; 03B D14). Draft PR #476. Governing records: the pin's
capability spec `graph-view-subscription` (inventory 6a, every requirement mapped), ADR-081 (binding, ported),
ADR-085 Decision 5 (binding, ported), ADR-113 and the `background-work` spec, ADR-079 (consumer side only). Probes:
inventory 7 (G1-G6; P8-P12 from PR #458 comment 6472856197).

Coined terms: a *view* is a `View[T]`, one in-memory copy of a KV bucket's current values built from one watch; a
*subscription* is one local reader attached to it; a *tick* is the view's coalescing window (250 ms by default); a
*loss* is the watch's update channel closing while `Run`'s context is still live.

## Options

- **A. Port whole, `Run(ctx)` in place of `Start`/`Stop`, unread surface dropped (recommended).** One PR, 1,193 pin
  lines, the shape of `graph/embedding.VectorCache` and `graph/follower`.
- **B. Carry `Start`/`Stop`.** Fails `background-work` (`spec.md:11-17`) and the foundation adapt row (`:382-397`).
- **C. `Start(ctx)` with a bounded `Stop(ctx)`.** A service's shape; keeps adopter fact 1 (inventory 12).
- **D. Do nothing.** `processor/graph-query` (item 9) cannot build; `docs/contract.md:120` names an absent package.

## D1. Files and edges

`pkg/graphview/` (the pin's five files, a new `README.md` as the product contract line, tests). New import edge
`pkg/graphview` → `internal/kvwatch` (no exported signature names it). One line in `.agents/skills/kv-or-stream/
SKILL.md`: many local readers of one bucket share a `graphview.View`, never a `WatchAll` each (the lesson of ADR-081
and the pin spec's purpose; no pin skill or contract section names the package). Records, ledger, repository map and
the coverage target: tasks 4.1-4.3.

## D2. The view runs once, until its context ends (`background-work`, ADR-113)

Required: `background-work`, the foundation adapt row, ADR-113 (pin `view.go:217`) and completed joins (pin
`view.go:593` stops the watch without the drain #454 needs). Observable behaviour:

- `New` is unchanged. `Run(ctx)` refuses nil before touching the source. `Run` on a running view returns
  `ErrAlreadyStarted`; on a view whose `Run` has returned, for any reason, `ErrViewStopped` (**changed:** the pin
  returned `ErrAlreadyStarted` for a failed view, `view.go:213-215`). A replacement is a new view.
- `Run` opens one `WatchAll` under a context derived from `ctx`. If that fails, `Run` returns `ctx.Err()` when `ctx`
  has ended (view stopped), else the open error (view failed: reads and `WaitCaughtUp` return the wrapped
  `ErrWatcherLost`); `OnWatcherLost` does not fire, as at the pin; nothing is left running.
- `Run` reads the watch and the tick on the caller's goroutine; the view's only goroutines are one delivery goroutine
  per subscription, started by the attach and joined by `Run` (foundation handle rule, `:95-98`).
- **Ending `ctx` is a stop (changed from the pin spec's "Watcher loss fails closed", `:147-150`, and `view.go:200-202`).**
  Ending the watch context closes `Updates()` (P11), so a close is a loss only while `ctx` is live. On `ctx` ending:
  reads and attaches return `ErrViewStopped`; each subscription's `Err` is `ErrViewStopped`, or its own context's
  error when that ended first, set before its `Deltas` closes; `OnWatcherLost` does not fire; the watch stops through
  `kvwatch.Stop`; `Run` returns `ctx.Err()`. semboids at the pin counts every shutdown as a loss (inventory 12).
- **On a loss**: reads and attaches return the error wrapping `ErrNotReady`, `ErrWatcherLost` and the cause, from
  then on; each subscription's `Err` wraps `ErrWatcherLost` (or is its own context's error when that ended first),
  set before its `Deltas` closes; then `OnWatcherLost(cause)` fires once; the watch stops through `kvwatch.Stop`;
  `Run` returns an error wrapping `ErrWatcherLost`. The pin's `TestWatcherLossFailsClosed` order is kept.
- `Run` returns only after every subscription goroutine and every hook call made by an attach or detach in progress
  has returned (pin `TestStopJoinsSubscriberCallback`), and after the watch's delivery closed or
  `kvwatch.DrainBudget` ran out after `ctx` ended (error joined with `kvwatch.ErrDeliveryOpen`). A blocking hook or
  decode holds `Run`; the composing component bounds its wait (ADR-113).
- `OnSubscribers` stays an eventually-correct gauge while the view runs (pin doc, `view.go:73-75`). Its last value
  after `Run` returns is **not promised**: an attach's `OnSubscribers(1)` computed before the stop may land after the
  stop's `OnSubscribers(0)`. A consumer that needs zero sets it when `Run` returns. Fixing it would add ordering
  mechanism to a gauge.
- After `Unsubscribe` or the subscription's context ends, `Deltas` closes; a batch already taken may still be
  delivered first (`subscription.go:87-94`). `Unsubscribe` does not wait for the goroutine; `Run` does.

## D3. Surface

Kept as at the pin: `New`, `Option`, `WithTickInterval`, `WithHooks`, `Hooks` (all seven; `OnTick` has a production
caller in the consumer adapter `processor/agentic-dispatch`, #190), `DecodeFunc`, `EntryMeta.Created`,
`WatcherSource`, `WaitCaughtUp`, `Get`, `SnapshotAndSubscribe`, `Subscription`, `Delta`, `DeltaOp`, `Entry`,
`Snapshot.Entries`/`Poisoned`, `PoisonError`, the five sentinels. Kept with no reader, on a contract line:
`Subscribe` (ADR-081 D3), `List`/`KeyedEntry` (D6), `Applied`, `Snapshot.Revision`/`AppliedAt` (ADR-085 D5); the
four unread sentinels are the typed errors the methods' contracts return. The caught-up state and watermark that
ADR-081 D5 and pin spec `:132-134` ask the view to expose are carried by `WaitCaughtUp`, the `OnCaughtUp` and
`OnWatcherLost` hooks and `Applied`. Contract lines: the package `README.md` and the ledger `contract` field; each
unread kept symbol gets a test calling it as a product would (package `graphview_test`).

Changed: `Start`/`Stop` become `Run` (D2), new exported surface for the owner's acceptance. Dropped as unread with no
contract line (inventory 4): `AppliedRevision` (second spelling of `Applied`), `CaughtUp`, `View.Poisoned`,
`Snapshot.Sequence`, `EntryMeta.Revision`; `DefaultTickInterval` becomes unexported.

## D4. Corrections and retained findings (Bounded port scope)

Required: D2, D3, the docs drift S10, the tests of D6. Retained, optional: S1 the tombstone switch (`natsclient`
would pull its dependencies into a leaf package products import); S3 the package's own sentinels; S5 the watch loop
(#461 tracks the stop half). **Optional removals, recommended (proposal question 2):** S4 the ghost-key loop
(`view.go:713-722`), which can remove nothing because a view runs once from an empty copy, and S6 the test-only tick
channel (`view.go:106-110`, `:766-771`), which `synctest` makes unnecessary. If S4 goes, `TestFreshReplacementReconcilesGhostKeys`
goes with it (it cannot fail, inventory probe) and the pin spec's re-bootstrap scenario rests on `TestRunOnce`,
checked by a mutant that allows a second `Run`. Delivered pointer values stay shared (ADR-081 D4), a named doc cost.

## D5. ADRs

ADR-081 is ported with a port note: pin path and SHA, this change and its ledger row, the SemStreams sites in its
tables are SemStreams', and its decisions are kept. The note names the pin spec requirement "Watcher loss fails
closed" as the record that changes (a context end is a stop, D2) and says its re-bootstrap rule holds because a view
runs once. ADR-085 Decision 5 is kept (`Applied`, the snapshot pair). ADR-079 is not ported: its Decision 3 binds
projection owners, which are consumers. ADR-082, -083, -084, -090, -092 name the package in passing.

## D6. Proving tests

Each spec scenario names its test. Unit tests run in `synctest` on the pin's fake source, changed to close
`Updates()` when the watch context ends (as P11 shows nats.go does; the pin fake ignores it, `harness_test.go:56`).
`TestRunContextEndIsAStop` builds the both-ready case directly: an `OnApply` hook holds `Run`, the test cancels
`ctx`, the fake closes `Updates()`, the hook returns, so `Run`'s next read finds both ready; repeated 200 times in
the test on fresh views, each must report a stop. A lost watch is driven from the fake; on a real server
`TestRunReportsLostWatch` deletes the bucket and waits for the close, about 20 s at v1.54.0 (G6, not promised).

Generated: `TestViewHistoryMatchesModel`, seeded, recording seed and run count: random puts, deletes, purges,
skips, poisons, attaches of both kinds, unsubscribes and ticks against a model (last operation per key); every
subscriber's snapshot plus deltas equals the model at every tick and no key's revision falls (ADR-081 G1, G2, G4,
G6). Integration on `natsfixture.Open` (#195): the pin's real-bucket test, `TestRunLeavesNoWatchDelivery` (backlog
over 256 entries, G1), `TestRunReportsLostWatch`, `TestIndependentWatchUnaffected`. Mutation checks
(`task mutate:check`): a close counted as a loss regardless of `ctx` (`TestRunContextEndIsAStop`); `kvwatch.Stop`
replaced by `watcher.Stop` (`TestRunLeavesNoWatchDelivery`); the attach-sequence filter (`view.go:840`) removed (the
history test); a second `Run` allowed (`TestRunOnce`); decode moved per subscriber (`TestDecodeOncePerWrite`).
Survivors are reported as survivors.

## D7. Issues and merge order

Issue #195 stays open (other packages); this PR moves its one site. #9 is addressed. #461 does not name the package;
the view is a new `kvwatch.Stop` caller, noted there. Adoption sites: inventory 5, one issue (task 1.4).
Merge order: PR #475 shares no file; whichever merges second merges `origin/main` (ledger and repository-map text), as #475's
`design.md:153-154` also says. If #464 (nats.go 1.55.0) merges first, task 1.2 re-runs G1-G6 at the new version.
