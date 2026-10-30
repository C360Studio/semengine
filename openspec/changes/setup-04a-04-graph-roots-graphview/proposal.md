# Proposal: setup-04a-04-graph-roots-graphview

Approved-by:

## Why

Change 4 of epic #9 ports the graph roots one package per pull request in the owner's order (#9 comment 6386856657);
item 7 is "`pkg/graphview`; second-lane capable", and item 9, `processor/graph-query`, imports it. The package keeps
one in-memory copy of a KV bucket's current values from a single watch and serves many local readers a snapshot
followed by coalesced changes, so N readers cost one watch instead of N (ADR-081). SETUP 03B admits it at tier 0 as
the view layer of change observation (design D14; semboids is its consumer), and `docs/contract.md:120` already
promises it. It has no provider code.

## What Changes

- Port `pkg/graphview` whole (1,193 pin lines). `View[T]`'s `Start(ctx)`/`Stop()` become `Run(ctx)`, which returns
  only when every goroutine the view started and the watch's delivery have ended (`background-work` spec, ADR-113),
  stopping the watch through `internal/kvwatch.Stop`.
- Ending `Run`'s context is a clean stop (`ErrViewStopped`, no `OnWatcherLost`); only an update channel that closes
  while the context is live is a lost watch. The pin's spec counted a context ending as a loss, so every semboids
  shutdown bumped its lost-watch metric.
- Unread surface with no contract line is dropped: `AppliedRevision`, `CaughtUp`, `View.Poisoned`,
  `Snapshot.Sequence`, `EntryMeta.Revision`; `DefaultTickInterval` becomes unexported. `Subscribe`, `List` and
  `Applied` stay: ADR-081 and ADR-085 promise them.
- Tests move to `synctest` and `natsfixture.Open`; a seeded history test checks the snapshot-and-changes guarantee
  against a model; new tests prove the stop/loss split on a real watch and that no watch delivery outlives `Run`.
- Every requirement of the pin's `graph-view-subscription` spec is carried (inventory 6a). ADR-081 is ported with
  its decisions kept. Ledger row, repository map, 80% coverage
  target (03B D10), one line in the `kv-or-stream` skill.

## Capabilities

- New `graph-view-subscription`.

## Impact

- New `pkg/graphview`; new import edge to `internal/kvwatch`. No subject, bucket, stream, payload, metric or config
  key. No caller in the tree until `processor/graph-query` (item 9); semboids adopts it when it moves to SemEngine.
- Consumers moving from SemStreams replace `Start`/`Stop` with `Run` and their loss hook with `Run`'s return.
- Advances #195 (this package's one site). Stays open: #195, #461. Addresses #9.

## Questions for the owner

1. **New method.** The view's start and stop become one `Run` call that blocks until the view is done, the same
   shape the embedding worker and cache took. Accept it as the package's public lifecycle?
2. **Two optional removals.** (a) A loop that is meant to drop deleted keys when a view restarts can never do
   anything, because a view never restarts (a replacement starts empty); deleting it leaves every test passing.
   (b) A hidden option that lets tests swap the view's ticker; the new tests do not need it. Remove both? (Optional:
   keeping them is allowed.)
3. **Shutdown is not a failure.** SemStreams treated a normal shutdown as a broken watch; we treat it as a normal
   stop. The cleanup-on-restart rule is kept, and holds automatically because a view never restarts. OK?
