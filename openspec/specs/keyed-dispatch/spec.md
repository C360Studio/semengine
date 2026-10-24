# keyed-dispatch Specification

## Purpose
`internal/dispatch.KeyedPool` runs work on a fixed number of lanes, one goroutine each, so that work for one key runs
in order while different keys run in parallel. This capability says where an item runs, that every item the pool
accepts is processed, that `Shutdown(ctx)` is the pool's only stop, and what the queue-depth gauge counts.

## Requirements

### Requirement: Same-key work runs in submit order on one lane

`internal/dispatch.KeyedPool` SHALL send each item that `SubmitBlocking(ctx, key, work)` accepts to the lane
`fnv1a(key) % Lanes`, which stays the same for the pool's lifetime. Each lane SHALL run its items one at a time, in the
order they were accepted, on one goroutine, and SHALL pass `Process` its lane index.

#### Scenario: Items with one key run in submit order

- **WHEN** items a, b and c are submitted with the same key
- **THEN** `Process` is called with a, then b, then c, on one lane, and no two of those calls overlap

### Requirement: A full lane makes the submit wait

When the lane's queue is full, `SubmitBlocking` SHALL wait until the lane has room, or return `ctx.Err()` when its
context ends first.

#### Scenario: A full lane makes the submit wait

- **WHEN** the lane's queue is full and `SubmitBlocking` is called for it
- **THEN** the call has not returned while the lane is still busy, and it returns `ctx.Err()` when its context ends

### Requirement: A panic in Process is recovered

A panic in `Process` SHALL be recovered: `OnPanic`, when set, SHALL be called with the item and the recovered value, and
the lane SHALL go on to its next item.

#### Scenario: A panic does not stop the lane

- **WHEN** `Process` panics on one item and later items go to the same lane
- **THEN** `OnPanic` is called with that item, and the later items are processed

### Requirement: Every accepted item reaches Process, and Shutdown is the only stop

`SubmitBlocking` SHALL return nil only for an item that `Process` is then called with exactly once, before `Shutdown`
returns nil. A `SubmitBlocking` call that begins after the first `Shutdown` call SHALL return `ErrStopped`.

#### Scenario: A submit after Shutdown

- **WHEN** `SubmitBlocking` is called after the first `Shutdown` call
- **THEN** it returns `ErrStopped`

### Requirement: The run context reaches only Process

The context given to `NewKeyedPool` SHALL reach only `Process`: ending it SHALL NOT stop a lane, refuse a submit, or
drop a queued item, and each `Process` call after it ended SHALL receive the ended context.

#### Scenario: A submit after the run context ended

- **WHEN** the context given to `NewKeyedPool` has been cancelled and `SubmitBlocking` then returns nil for an item
- **THEN** `Process` is called with that item and the ended context, and a later `Shutdown` returns nil

#### Scenario: Items queued when the run context ends

- **WHEN** items wait in a lane's queue and the context given to `NewKeyedPool` is cancelled
- **THEN** `Process` is called with each of them, with the ended context, before `Shutdown` returns nil

### Requirement: Shutdown returns nil once every item is processed

`Shutdown(ctx)` SHALL return nil once every accepted item has been processed and every lane has exited, also when ctx
has ended by then, and SHALL return `ctx.Err()` when ctx ends first, as the `background-work` requirement "Background
work takes one of three shapes" states for `Shutdown`.

#### Scenario: Shutdown after the lanes exited, with an ended context

- **WHEN** every lane has exited and `Shutdown` is called with a context that has already ended
- **THEN** `Shutdown` returns nil

### Requirement: The queue-depth gauge counts waiting items

With `KeyedConfig.MetricsRegistry` set, the gauge `dispatch_queue_depth{pool=<Name>}` SHALL equal the number of items
accepted and not yet passed to `Process`, summed over every pool of that `Name` on the registry, whenever no submit or
lane pickup is in progress. The pool SHALL run no metrics sampler goroutine.

#### Scenario: A busy lane with items waiting

- **WHEN** one item is in `Process`, three more wait in its lane's queue, and no clock time has passed
- **THEN** the gauge reads 3, and after `Shutdown` returns nil it reads 0

#### Scenario: Pools sharing a name

- **WHEN** two pools of one `Name` on one registry each hold waiting items, and one of them is shut down
- **THEN** the gauge reads their sum, then the remaining pool's count, and 0 once both are shut down
