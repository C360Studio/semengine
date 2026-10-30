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
running, and SHALL return `ErrViewStopped` on a view whose `Run` has returned, whether it stopped or failed. A view
SHALL never re-bootstrap, so a replacement view holds no key its own replay did not deliver.

#### Scenario: A nil context, a second Run

- **WHEN** `Run` is called with a nil context, or while another `Run` on the view runs, or after `Run` returned on a
  stop or a lost watch
- **THEN** it returns an error and starts nothing, and the running `Run` keeps running
- **AND** the tests that verify this are `TestRunRefusesNilContext` and `TestRunOnce`

### Requirement: Ending the context is a stop, not a loss

When `Run`'s context ends, the view SHALL refuse reads and attaches with `ErrViewStopped`, SHALL set each
subscription's `Err` to `ErrViewStopped`, or to the subscription context's error when that ended first, before
closing its `Deltas`, SHALL NOT call `OnWatcherLost`, and `Run` SHALL return `ctx.Err()`, even when the watch's
update channel has closed by the time `Run` reads it.

#### Scenario: Context end and channel close both ready

- **WHEN** an `OnApply` hook holds `Run`, the test cancels `Run`'s context, the source closes the update channel,
  and the hook returns, on 200 fresh views
- **THEN** on every view `OnWatcherLost` is never called, a subscription attached with a live context reports
  `ErrViewStopped`, and `Run` returns `context.Canceled`
- **AND** the test that verifies this is `TestRunContextEndIsAStop`

### Requirement: A lost watch fails closed

When the watch's update channel closes while `Run`'s context is live, the view SHALL refuse reads and attaches with an
error matching `ErrNotReady` and `ErrWatcherLost`, SHALL set every subscription's `Err` to an error matching
`ErrWatcherLost` before closing its `Deltas`, SHALL call `OnWatcherLost` once after that, and `Run` SHALL return an
error matching `ErrWatcherLost`. How soon a deleted bucket closes the channel is not promised.

#### Scenario: The update channel closes

- **WHEN** the source closes the update channel while `Run` has a live context and two subscriptions
- **THEN** both subscriptions close with `ErrWatcherLost` before `OnWatcherLost` fires once, `Get` returns an error
  matching `ErrWatcherLost`, and `Run` returns an error matching `ErrWatcherLost`
- **AND** the test that verifies this is `TestRunLostWatchFailsClosed`

#### Scenario: The bucket is deleted

- **WHEN** a real bucket is deleted under a running view
- **THEN** once nats.go closes the update channel, `Run` returns an error matching `ErrWatcherLost`
- **AND** the test that verifies this is `TestRunReportsLostWatch`

#### Scenario: The watch cannot open

- **WHEN** the source's `WatchAll` fails while the context is live
- **THEN** `Run` returns that error, `WaitCaughtUp` returns an error matching `ErrWatcherLost`, `OnWatcherLost` is
  not called, and nothing is running
- **AND** the test that verifies this is `TestRunOpenFailure`

### Requirement: Each write is decoded once

The view SHALL call its decode function once per delivered write, whatever the number of subscriptions, and SHALL
open no watch or consumer per subscription.

#### Scenario: Many subscribers

- **WHEN** five subscriptions are attached and one write lands
- **THEN** the decode function is called once for it and every subscription receives the change
- **AND** the test that verifies this is `TestDecodeOncePerWrite`

### Requirement: Deletes reach every subscriber

A delete or purge of a key SHALL reach every attached subscription as a `DeltaDelete` at its revision, in the same
lane as upserts, and SHALL NOT be followed by an older value of the key.

#### Scenario: Attached subscriber learns of the delete

- **WHEN** a key held in a subscriber's snapshot is deleted and the next tick fires
- **THEN** the subscriber receives a delete delta for it at the delete's revision
- **AND** the test that verifies this is `TestDeleteDeliveryToAttachedSubscriber`

### Requirement: Point reads are coherent with the stream

`Get` and `List` SHALL read the same copy the deltas come from, SHALL never return a value older than one the view
has applied, and SHALL honour the readiness and poison rules; `List` SHALL sort by key before applying its limit and
SHALL omit poisoned keys.

#### Scenario: Point read after an apply

- **WHEN** the view has applied a key at revision R
- **THEN** `Get` returns it at R or newer, or not-found when deleted at or after R, and `List` agrees
- **AND** the test that verifies this is `TestGetAndListSurface`

### Requirement: Detach releases its state

After `Unsubscribe` or the end of a subscription's context, the view SHALL hold no pending changes for it, SHALL
close its `Deltas`, and later ticks SHALL deliver nothing to it.

#### Scenario: Unsubscribed or cancelled

- **WHEN** a subscription unsubscribes, or its context is cancelled, and further ticks fire
- **THEN** its `Deltas` is closed and no pending buffer is kept for it
- **AND** the tests that verify this are `TestUnsubscribeReleasesState` and `TestSubscriberContextCancelDetaches`

### Requirement: The view coexists with raw watches

A view SHALL open only its own watch and SHALL place no constraint on another watch or consumer of the same bucket.

#### Scenario: An independent watch beside a view

- **WHEN** a view runs over a bucket and another `WatchAll` on the same bucket receives writes
- **THEN** the independent watch receives every revision, as it would with no view
- **AND** the test that verifies this is `TestIndependentWatchUnaffected`

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
