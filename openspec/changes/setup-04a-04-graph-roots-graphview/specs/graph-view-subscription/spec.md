# graph-view-subscription

## ADDED Requirements

### Requirement: Run returns only after everything it started has ended

`graphview.View.Run(ctx)` SHALL hold one watch of its bucket until `ctx` ends or the watch is lost, and SHALL return
only after every goroutine the view started, every hook call made by an attach or detach in progress, and the watch's
delivery have ended, or with an error joined with `kvwatch.ErrDeliveryOpen` when that delivery did not close within
`kvwatch.DrainBudget` after `ctx` ended.

#### Scenario: Started, used and stopped

- **WHEN** `Run` is started inside `synctest.Test`, a snapshot subscription and a delta-only subscription attach and
  receive changes, and the context is cancelled
- **THEN** `Run` returns `context.Canceled` and the bubble ends with no goroutine left
- **AND** the test that verifies this is `TestRunLeavesNothingBehind`

#### Scenario: Stopped with a backlogged watch

- **WHEN** `Run`'s context is cancelled while nats.go's watch delivery is blocked on a full updates buffer
- **THEN** `Run` returns only after that delivery has ended
- **AND** the test that verifies this is `TestRunLeavesNoWatchDelivery`

### Requirement: A view runs once

`Run` SHALL refuse a nil context before touching its source, SHALL return `ErrAlreadyStarted` on a view that is
running, and SHALL return `ErrViewStopped` on a view whose `Run` has returned.

#### Scenario: A nil context, a second Run

- **WHEN** `Run` is called with a nil context, or while another `Run` on the view runs, or after `Run` returned
- **THEN** it returns an error and starts nothing, and the running `Run` keeps running
- **AND** the tests that verify this are `TestRunRefusesNilContext` and `TestRunOnce`

### Requirement: Ending the context is a stop, not a loss

When `Run`'s context ends, the view SHALL refuse reads and attaches with `ErrViewStopped`, SHALL set every
subscription's `Err` to `ErrViewStopped` before closing its `Deltas`, SHALL NOT call `OnWatcherLost`, and `Run`
SHALL return `ctx.Err()`, even though ending the watch's context closes its update channel.

#### Scenario: Cancelled on a real watch

- **WHEN** a view over a real bucket with an attached subscription has its `Run` context cancelled, twenty times
- **THEN** `OnWatcherLost` is never called and every subscription's `Err` is `ErrViewStopped`
- **AND** the tests that verify this are `TestRunContextEndIsNotALoss` and `TestRunContextEndIsAStop`

### Requirement: A lost watch fails closed

When the watch's update channel closes while `Run`'s context is live, the view SHALL refuse reads and attaches with an
error matching `ErrNotReady` and `ErrWatcherLost`, SHALL set every subscription's `Err` to an error matching
`ErrWatcherLost` before closing its `Deltas`, SHALL call `OnWatcherLost` once after that, and `Run` SHALL return an
error matching `ErrWatcherLost`. The view SHALL never serve its frozen copy as live.

#### Scenario: The update channel closes

- **WHEN** the bucket is deleted while `Run` has a live context and two subscriptions
- **THEN** both subscriptions close with `ErrWatcherLost`, `Get` returns an error matching `ErrWatcherLost`, and
  `Run` returns an error matching `ErrWatcherLost`
- **AND** the tests that verify this are `TestRunReportsLostWatch` and `TestRunLostWatchFailsClosed`

#### Scenario: The watch cannot open

- **WHEN** the source's `WatchAll` fails while the context is live
- **THEN** `Run` returns that error, `WaitCaughtUp` returns an error matching `ErrWatcherLost`, and nothing is running
- **AND** the test that verifies this is `TestRunOpenFailure`

### Requirement: Reads and attaches wait for the initial replay

`Get`, `List`, `SnapshotAndSubscribe` and `Subscribe` SHALL return `ErrNotReady` until the watch's initial replay has
been applied, and `WaitCaughtUp` SHALL return nil once it has.

#### Scenario: Attach during the replay

- **WHEN** a reader attaches before the end-of-replay marker
- **THEN** it receives `ErrNotReady`, never a partial snapshot
- **AND** the test that verifies this is `TestBootstrapGatesAttach`

### Requirement: Snapshot and changes form one history

A subscription attached by `SnapshotAndSubscribe` SHALL receive in its snapshot every change applied before the attach
and in its deltas exactly the changes applied after it; within and across batches, a key's revision SHALL never
fall; a batch SHALL hold at most one delta per key, the newest, sorted by revision; a delete SHALL ride the same
lane. `Subscribe` SHALL deliver the changes after the attach with no snapshot.

#### Scenario: A generated history

- **WHEN** a seeded random sequence of puts, deletes, purges, skipped and poisoned writes, attaches, unsubscribes
  and ticks runs against the view and a reference model
- **THEN** each subscriber's snapshot plus deltas equals the model at every tick and no key's revision falls
- **AND** the test that verifies this is `TestViewHistoryMatchesModel`

#### Scenario: A late attach across a tick

- **WHEN** a subscriber attaches after a newer write than a change still waiting for the tick
- **THEN** it never receives the older change
- **AND** the test that verifies this is `TestTickSeamNoStaleDeliveryToLateAttacher`

### Requirement: A value that fails decoding is poison, never an upsert

When the decode function returns an error for a write, the view SHALL remove the key's value, SHALL deliver a
`DeltaPoison` carrying a `*PoisonError`, SHALL return that error from `Get` for the key and list it in
`Snapshot.Poisoned`, and SHALL keep serving other keys, until a newer write or a delete replaces it. A write the
decode function does not keep SHALL make the key absent.

#### Scenario: Poisoned write

- **WHEN** a key's newest write fails decoding
- **THEN** subscribers receive a poison delta, not an upsert, and other keys keep flowing
- **AND** the test that verifies this is `TestPoisonSurfacedNotDelivered`

### Requirement: A slow subscriber never blocks the view

A subscriber that does not read SHALL hold at most one pending delta per changed key, newest wins, and SHALL NOT delay
the watch, other subscribers or `Run`'s stop.

#### Scenario: One reader stalls

- **WHEN** one of three subscribers stops reading for five ticks
- **THEN** the other two receive every tick, and the stalled one converges to the newest value when it resumes
- **AND** the test that verifies this is `TestSlowSubscriberBackpressure`

### Requirement: The view reports its currency without gating on it

`Applied` SHALL return the highest applied revision and that write's server time from one read, zero time before
anything is applied, and snapshots SHALL carry the same pair; no call SHALL refuse or wait because of the time.

#### Scenario: The pair is never torn

- **WHEN** writes apply while `Applied` is read concurrently
- **THEN** every pair read names one write
- **AND** the test that verifies this is `TestAppliedPairIsNeverTorn`
