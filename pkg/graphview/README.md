# pkg/graphview

Keeps one in-memory copy of a NATS KV bucket's current values, built from a single watch, and gives each
local reader a snapshot followed by the changes after it. A component that serves many clients a live view of
a bucket uses one `graphview.View` instead of opening one watch per client. Each write is decoded and
validated once, however many readers there are.

A *view* is a `View[T]`: the copy plus its one watch. A *subscription* is one local reader attached to a view.
A *tick* is the view's coalescing window (250 ms by default): the changes to a key inside one window reach
readers as one change, the one with the greatest revision. A *loss* is the watch's update channel closing
while `Run`'s context is still live.

## Basic use

You give the view a bucket handle (anything with `WatchAll`, such as a `jetstream.KeyValue`) and a decode
function that turns one stored value into your type `T`. Run the view on its own goroutine, wait until it has
read the bucket's current values, then attach a reader:

```go
// decode has the type graphview.DecodeFunc[T].
view, err := graphview.New(bucket, decode)
if err != nil {
    return err
}

errc := make(chan error, 1)
go func() { errc <- view.Run(ctx) }()

if err := view.WaitCaughtUp(ctx); err != nil {
    return err
}
snap, sub, err := view.SnapshotAndSubscribe(ctx)
if err != nil {
    return err
}
defer sub.Unsubscribe()

// snap.Entries holds every key at attach time; sub.Deltas() delivers only what changed after it.
for batch := range sub.Deltas() {
    for _, d := range batch {
        // d.Op is DeltaUpsert, DeltaDelete or DeltaPoison
    }
}
err = sub.Err() // why the channel closed
```

The decode function returns `(value, keep, err)`. `keep == false` leaves the key out of the view. A non-nil `err`
*poisons* the key: readers get a `*PoisonError` for it (in a delta, in `Snapshot.Poisoned`, and from `Get`) and
never a stale value, and other keys keep flowing.

## What is here

| Name | What it does |
| --- | --- |
| `New`, `WithTickInterval`, `WithHooks` | Build a view over a `WatcherSource`; set the tick; install observation hooks |
| `Run` | Opens the watch and holds it until its context ends or the watch is lost |
| `WaitCaughtUp` | Blocks until the view has read the bucket's current values, the view fails or stops, or its context ends |
| `SnapshotAndSubscribe` | Copies the current values and attaches a reader in one step: the snapshot holds every change the view had applied, the subscription delivers every change after it, with no gap and no repeat |
| `Subscribe` | Attaches without a snapshot, for a reader that only reacts to changes |
| `Get`, `List` | Point reads of the same copy; `List` filters by key prefix, sorts by key, then applies the limit |
| `Applied` | The highest revision applied and the server write time of that write; zero time until something is applied. It reports currency and gates nothing (ADR-085 decision 5) |
| `Subscription` | `Deltas()` delivers batches with at most one change per key, sorted by revision; `Err()` says why it closed; `Unsubscribe()` detaches |
| `Hooks` | Optional callbacks: `OnApply`, `OnCaughtUp`, `OnWatcherLost`, `OnPoison`, `OnSubscribers`, `OnFanOut` |
| `ErrNotReady`, `ErrWatcherLost`, `ErrViewStopped`, `ErrKeyNotFound`, `ErrAlreadyStarted` | The errors the methods return; a failed view's errors match both `ErrNotReady` and `ErrWatcherLost` |

## Running and stopping

A view runs once. `Run` refuses a nil context, returns `ErrAlreadyStarted` while another `Run` is running, and
returns `ErrViewStopped` for a view whose `Run` has already returned, for any reason. To replace a view, build a new
one: it starts empty and serves only what its own watch delivers.

`Run` reads the watch on the caller's goroutine, so the caller owns the goroutine and the context and ends the
context to stop.

- **Ending the context is a stop.** Reads and attaches return `ErrViewStopped`, each attached subscription closes
  with `ErrViewStopped` as its `Err`, `OnWatcherLost` does not fire, and `Run` returns the context's error. A
  subscription that detached earlier keeps its own reason.
- **A loss fails closed.** If the watch's update channel closes while the context is live, the view never serves its
  frozen copy as current. Reads and attaches return an error matching `ErrNotReady` and `ErrWatcherLost`, each
  attached subscription closes with an error matching `ErrWatcherLost`, then `OnWatcherLost` fires once, and `Run`
  returns an error matching `ErrWatcherLost`. Treat it as an error to act on: report it or build a new view.
- **`Run` opens the watch under a context derived from its own.** If opening fails, `Run` returns the context's
  error when the context has ended; otherwise it returns the open error and the view is failed.

`Run` returns only after every subscription's goroutine and every hook call made by an attach or detach has
returned, and after the watch's delivery has closed, for at most `kvwatch.DrainBudget` (5 s) after the context
ends; if the budget runs out the error is joined with `kvwatch.ErrDeliveryOpen`. A hook or decode function that
never returns holds `Run`; the component that runs the view bounds its own wait (ADR-113).

How nats.go times a watch's shutdown or its close is not promised. The probes behind this package's behaviour are on
PR #476, comment 6522730160, and PR #458, comment 6472856197, at nats.go v1.54.0. For one case, deleting the bucket
under a live watch closed the update channel after about 20 s there, and that delay is not promised.

## Readers and delivery

- A reader that cannot keep up is never disconnected. Its pending changes coalesce per key, keeping the greatest
  revision, in a buffer bounded by the number of keys that changed. It never blocks the watch or other readers.
- Delivered values are shared between readers. When `T` is a pointer, do not change a delivered value.
- After `Unsubscribe` or the subscription's context ends, `Deltas` closes; a batch already taken may still arrive
  first. `Unsubscribe` does not wait for the delivery goroutine; `Run` does.
- `OnSubscribers` is not promised to hold the current count: attach and detach call it after releasing the lock, so
  under concurrent attach or detach, or across a stop, a value can be stale until the next change. A consumer that
  needs zero sets it when `Run` returns.
- A view holds one decoded copy of the bucket in memory. It saves work only where several readers would otherwise
  each hold a watch or a copy.

## Not here

- No cross-process fan-out. The view serves readers in its own process.
- A reader that needs its own key filter, history replay, every revision, or its own acknowledgement opens a real
  JetStream consumer instead; the view coexists with raw watches.
- Dropped from the SemStreams package: `Start` and `Stop` (now `Run`), `Hooks.OnTick`, `AppliedRevision`,
  `CaughtUp`, `View.Poisoned`, `Snapshot.Sequence` and `EntryMeta.Revision`.

Why the view is built this way: [ADR-081](../../docs/adr/081-graph-view-subscription.md). The admission ledger rows
for `pkg/graphview` and ADR-081 list what changed from SemStreams.
