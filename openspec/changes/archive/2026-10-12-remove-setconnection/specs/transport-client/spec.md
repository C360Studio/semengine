# transport-client

## MODIFIED Requirements

### Requirement: Close and Drain are bounded by the caller's context

`Subscription.Drain(ctx)` SHALL refuse a nil context (kept from the pin). `Client.Close(ctx)` and `Client.Connect(ctx)`
SHALL refuse a nil context without touching the connection (changed behaviour: at the pin `Close(nil)` panics on a
connected client, and `Connect(nil)` dials, then panics, leaking the dialled connection; design P13). `Close` SHALL
bound its drain by the caller's context deadline and return within it, reporting the drain outcome, and a later
`Close` SHALL make no server call, and SHALL return nil once the first
`Close`'s cleanup and the join are complete; none of these SHALL be
repeated or retried by the client on the caller's behalf. When the context ends before the drain has finished,
`Close` SHALL close the socket of a connection this client dialled and end any dial nats.go has in progress to
reconnect it. That ends at once a write the server is not reading, and closes the connection; the drain is cut short.

#### Scenario: Close under an ended context

- **WHEN** Close is called with a deadline shorter than the drain needs
- **THEN** Close returns within the deadline, the connection is closed, and a second Close with a live context returns
  nil once no handler of the client is still running (see "A nil Close means every handler returned")

#### Scenario: Close while the server is not reading

- **GIVEN** a connection the client dialled, whose server has stopped reading, with a publish held in the write
- **WHEN** Close is called with a 50 ms deadline
- **THEN** Close returns the context's error within the deadline, the connection is closed, and the held publish
  returns an error

#### Scenario: Close while a reconnect is dialling

- **GIVEN** a client whose connection the server dropped, while nats.go dials a reconnect that is not answered
- **WHEN** Close is called with a 50 ms deadline
- **THEN** Close returns the context's error within the deadline

#### Scenario: Nil Close context

- **WHEN** Close is called with a nil context on a connected client
- **THEN** Close returns an error naming the nil context, the connection stays open, and no drain is attempted

#### Scenario: Nil Connect context

- **WHEN** Connect is called with a nil context
- **THEN** Connect returns an error naming the nil context and no dial is attempted

#### Scenario: Nil Drain context

- **WHEN** Drain is called with a nil context
- **THEN** Drain returns an error naming the nil context and makes no server call

### Requirement: Close refuses new work

Once `Close` has begun, `Connect`, `Subscribe`, `SubscribeForRequests` and the three Consume methods SHALL return
`nats.ErrConnectionClosed`, decided by Close having begun, not by `Status()`. `Connect` SHALL refuse before it
dials. A consumer setup admitted before Close began SHALL keep its claim until its handler invocations have
returned.

#### Scenario: Connect after Close

- **WHEN** Connect is called after Close has begun
- **THEN** it returns ErrConnectionClosed, no dial is attempted, and Status is unchanged

#### Scenario: Subscribe during the drain

- **WHEN** Subscribe or a Consume method is called while Close is draining
- **THEN** it returns ErrConnectionClosed and no handler of it ever runs

#### Scenario: A consumer refused during Close keeps its claim

- **WHEN** a consumer setup admitted before Close began has started native delivery
- **THEN** it returns ErrConnectionClosed with no handle only after its handler invocations returned, or, if its
  setup context ends first, that context's error at that point; either way its claim is held, and Close does not
  return nil, until the handlers have returned

## ADDED Requirements

### Requirement: Close returns nil only after everything the client owns has finished

`Client.Close` SHALL return nil only after every piece of work the client owns has finished: its background work,
every running invocation of a message handler passed to `Subscribe`, `SubscribeForRequests` or a Consume method,
the event handlers of the connection it closes when this client dialled it, and its async publish error handler as
far as nats.go lets the client observe it (D3 declares the one residual). This SHALL hold on every Close path, a
drain that runs out of time included, which Close SHALL report as a transient error wrapping
`nats.ErrDrainTimeout` whichever timer ran out first. Each Close SHALL return within its own context and SHALL NOT
wait on another caller's drain. A Close called from inside one of the client's callbacks SHALL return its context's
error and never nil.

#### Scenario: A nil Close means every handler returned

- **WHEN** a message handler passed to Subscribe or a Consume method is running and Close is called, first with an
  ended context and then with a live one
- **THEN** the first Close returns the context's error, and the second returns nil only after the handler returned

#### Scenario: A drain that runs out of time is reported

- **WHEN** a message handler is still running when the drain timeout passes, and Close's context is live
- **THEN** the first Close returns an error wrapping the drain timeout, after the handler has returned

#### Scenario: Each Close observes its own context

- **WHEN** one Close is draining and a second Close is called with an ended context
- **THEN** the second Close returns the context's error without waiting for the drain

#### Scenario: Close from inside a client callback

- **WHEN** a client callback calls Close with a bounded context
- **THEN** Close returns that context's error and never nil

## REMOVED Requirements

### Requirement: A nil Close means everything the client owns has finished

**Reason**: Its last sentence and two scenarios describe a subscription or consumer left on a connection that
`SetConnection` replaced. `SetConnection` is removed (#184), so no client can reach that state. The rest of the
requirement is kept, word for word, as the ADDED requirement "Close returns nil only after everything the client owns
has finished". OpenSpec's archive refuses a MODIFIED block that drops scenarios, so the requirement is removed and
added under that heading instead of modified.

**Migration**: None. No caller of `SetConnection` exists, here, in the four consumers, or at the pin outside tests.
