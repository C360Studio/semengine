# background-work

## ADDED Requirements

### Requirement: Background work takes one of three shapes

Background work is a goroutine that outlives the call that started it, in code that is not a service; a service is
held to the `lifecycle-suite` capability instead. A goroutine joined before the call that started it returns is not
background work. Background work SHALL stop in one of three shapes. `Run(ctx) error` is preferred: no goroutine is
started for the caller, the caller runs `Run` on a goroutine it owns, cancelling the context stops the work, and
`Run` returning is the join. `Close() error`, or a stop function the starting call returns, cancels and joins with no
timeout, and SHALL be used only when the goroutine waits on nothing but what the stop controls, such as its own ticker
or done channel; a goroutine that calls a caller-supplied callback, waits on in-flight requests, or performs network
I/O the cancel cannot end does not qualify. `Shutdown(ctx) error` SHALL be used for those: it stops the work, waits
for it, and returns `ctx.Err()` when the context ends first; the work then exits once what it waited on returns, and a
later `Shutdown` returns nil. The rule binds every package in the module and every later port.

#### Scenario: Run returns when its context ends

- **WHEN** a caller cancels the context of a running `Run(ctx)`
- **THEN** `Run` returns `ctx.Err()`, and no goroutine it started is still running

#### Scenario: Close joins the goroutine it owns

- **WHEN** `Close()` is called on a type whose goroutine waits only on its own ticker and done channel
- **THEN** `Close` returns after that goroutine has exited

#### Scenario: Shutdown under a blocked callback

- **WHEN** a caller's callback blocks inside the type's goroutine and `Shutdown(ctx)`'s context ends
- **THEN** `Shutdown` returns `ctx.Err()`

#### Scenario: Shutdown after the callback returns

- **WHEN** `Shutdown(ctx)` has returned `ctx.Err()` and the blocked callback then returns
- **THEN** the goroutine exits, and a later `Shutdown` returns nil

#### Scenario: Close chosen for a goroutine that runs a callback

- **WHEN** a type whose goroutine calls a caller-supplied callback stops through `Close()`
- **THEN** the port review rejects it for `Shutdown(ctx)` or `Run(ctx)`

### Requirement: No fixed shutdown timeout

Stopping background work SHALL NOT wait on a fixed duration in place of a join: no `time.After`, timer or
`context.WithTimeout` with a constant budget around the wait for a goroutine to exit. A bounded wait SHALL take its
bound from the caller's context. Terminal finalization work with no caller context (a flush, a durability write, a
test cleanup) MAY run under a budgeted context, and its goroutines still join.

#### Scenario: Fixed wait in a Close

- **WHEN** a `Close` waits on its goroutine's done channel or `time.After(5 * time.Second)`
- **THEN** the port review rejects it; `Close` waits on the done channel alone, or the type takes `Shutdown(ctx)`

### Requirement: Nothing left behind, nil refused

Code that runs background work SHALL have a unit test that starts it, uses it and stops it inside `synctest.Test`, so
the bubble proves every goroutine it started has exited. A nil context SHALL be refused at the call that receives it:
with an error where the call returns one, otherwise by a panic at the call before any goroutine starts; never by a
panic in a background goroutine.

#### Scenario: Stopped inside a bubble

- **WHEN** the work is started, used and stopped inside `synctest.Test`
- **THEN** the bubble returns, so no goroutine the work started is left running

#### Scenario: Nil context at the entry

- **WHEN** the call that starts the work is given a nil context
- **THEN** it refuses at the call, and no goroutine has started
