# transport-client

## MODIFIED Requirements

### Requirement: Close and Drain are bounded by the caller's context

`Subscription.Drain(ctx)` SHALL refuse a nil context (kept from the pin). `Client.Close(ctx)` and `Client.Connect(ctx)`
SHALL refuse a nil context without touching the connection (changed behaviour: at the pin `Close(nil)` panics on a
connected client, and `Connect(nil)` dials, then panics, leaking the dialled connection; design P13). `Close` SHALL
end its wait for the drain when the caller's context ends, reporting the drain outcome, and a later
`Close` SHALL make no server call, and SHALL return nil once the first
`Close`'s cleanup and the join are complete; none of these SHALL be
repeated or retried by the client on the caller's behalf. The caller's context does not bound a write on the
connection that the server is not reading: `Close` waits for each such write until it completes or nats.go's write
timeout (one minute by default) ends it. The write timeout ends each held write; nothing bounds the wait as a whole,
and this requirement does not (ADR-112 §1).

#### Scenario: Close under an ended context

- **GIVEN** a connection whose server is reading
- **WHEN** Close is called with a deadline shorter than the drain needs
- **THEN** Close returns within the deadline, the connection is closed, and a second Close with a live context returns
  nil once no handler of the client is still running (see "A nil Close means every handler returned")

#### Scenario: Nil Close context

- **WHEN** Close is called with a nil context on a connected client
- **THEN** Close returns an error naming the nil context, the connection stays open, and no drain is attempted

#### Scenario: Nil Connect context

- **WHEN** Connect is called with a nil context
- **THEN** Connect returns an error naming the nil context and no dial is attempted

#### Scenario: Nil Drain context

- **WHEN** Drain is called with a nil context
- **THEN** Drain returns an error naming the nil context and makes no server call

### Requirement: A nil Close means everything the client owns has finished

`Client.Close` SHALL return nil only after every piece of work the client owns has finished: its background work,
every running invocation of a message handler passed to `Subscribe`, `SubscribeForRequests` or a Consume method,
the event handlers of the connection it closes when this client dialled it, and its async publish error handler as
far as nats.go lets the client observe it (D3 declares the one residual). This SHALL hold on every Close path, a
drain that runs out of time included, which Close SHALL report as a transient error wrapping
`nats.ErrDrainTimeout` whichever timer ran out first. Each Close SHALL end its wait when its own context ends and
SHALL NOT wait on another caller's drain; a write the server is not reading holds it as "Close and Drain are bounded
by the caller's context" states. A Close called from inside one of the client's callbacks SHALL return its context's
error and never nil. A client-owned subscription left on a connection that `SetConnection` replaced SHALL be
unsubscribed and its running handler joined; the replaced connection itself belongs to the `SetConnection` caller.
A client-created consumer whose connection, the one its JetStream handle was made on, was replaced through
`SetConnection` SHALL be stopped when Close begins and its handlers joined, and its claim SHALL be released only after
they return.

#### Scenario: A nil Close means every handler returned

- **WHEN** a message handler passed to Subscribe or a Consume method is running and Close is called, first with an
  ended context and then with a live one
- **THEN** the first Close returns the context's error, and the second returns nil only after the handler returned

#### Scenario: A drain that runs out of time is reported

- **WHEN** a message handler is still running when the drain timeout passes, and Close's context is live
- **THEN** the first Close returns an error wrapping the drain timeout, after the handler has returned

#### Scenario: Each Close observes its own context

- **GIVEN** a connection whose server is reading
- **WHEN** one Close is draining and a second Close is called with an ended context
- **THEN** the second Close returns the context's error without waiting for the drain

#### Scenario: Close from inside a client callback

- **WHEN** a client callback calls Close with a bounded context
- **THEN** Close returns that context's error and never nil

#### Scenario: A subscription on a replaced connection

- **WHEN** a client subscription's connection has been replaced through SetConnection and its handler is running
  when Close is called with a live context
- **THEN** Close unsubscribes it and returns nil only after the handler returned, and the replaced connection stays
  open

#### Scenario: A consumer on a replaced connection

- **WHEN** a client-created consumer's connection, the one its JetStream handle was made on, has been replaced through
  SetConnection, and Close is called with a live context
- **THEN** Close stops the consumer, returns nil only after its handlers returned and its claim was released, and the
  replaced connection stays open

### Requirement: A consumer's setup context does not parent its handlers

`ConsumeStreamWithConfig` and `ConsumeInternalStreamWithConfig` SHALL use their context for setup only and SHALL
NOT retain it; the context ends setup's waits for the server, not a write the server is not reading (ADR-112).
Each handler's context SHALL descend from a context the client owns for that consumer, which the
client SHALL cancel when `Close` begins; `Close` SHALL still join every handler invocation (owner ruling, #9 comment
5994720412 item 2). `ConsumeStreamWithConfigContexts` keeps the caller's `handlerCtx` as the handlers' parent.

#### Scenario: A handler after the setup deadline

- **GIVEN** a consumer started with a setup context whose deadline then passes
- **WHEN** a message is delivered after that deadline
- **THEN** its handler receives a context that has not ended

#### Scenario: Close begins while a handler runs

- **GIVEN** a consumer whose handler is running
- **WHEN** `Close` begins
- **THEN** the handler's context ends, and `Close` returns nil only after the handler has returned

### Requirement: Subscribe returns after the server has read the SUB

SUB and UNSUB are the NATS protocol lines that add and remove a subscription; the server answers a PING with a PONG
once it has read every line sent before it on that connection. `Subscribe` and `SubscribeForRequests` SHALL return a
subscription only after the NATS server has read its SUB: after subscribing, the client SHALL send a PING on the
connection the subscription was made on and wait for the PONG (one round trip), so a message or request sent from any
connection to that server after the call returns finds the subscription, unless the server refused the SUB (changed
behaviour: at the pin both returned while the SUB could still be in the client's write buffer,
`natsclient/client.go:834-849` and `request.go:359` at the pin; #144). The wait for the PONG SHALL end when the server
answers, when the call's context ends, or when `DefaultRequestTimeout` has passed, whichever comes first. A call whose
context has already ended SHALL send no SUB and SHALL return a transient error matching the context's error.

The call's context and `DefaultRequestTimeout` bound the wait for the PONG only. Each step of the call that takes
nats.go's connection lock (checking the connection, writing the SUB and the PING, and, on a call that fails, clearing
the pending PONG and writing the UNSUB) first waits for any write in progress on that connection. While the server is
not reading and the connection's send buffer is full, a write on that connection, the call's own SUB or PING or
another goroutine's, is held, and so is the call, until that write completes or nats.go's write timeout (one minute by
default) ends it; a call behind more than one held write waits for each in turn. This requirement does not bound
those waits: nats.go's write timeout ends each held write, and nothing bounds the wait as a whole, for every call the
client makes, Close included (ADR-112 §1).

When the call returns no subscription after subscribing (the round trip did not complete, or Close had begun), it
SHALL first end the subscription: it unsubscribes it, so the subscription leaves the client's set of subscriptions and
is not restored on a reconnect, and on a connected client its UNSUB is queued to the server, which drops the
subscription when it reads that line; on a connection Close is draining or has closed, Close's drain or the close
ends it instead. The call SHALL NOT wait for a handler invocation of the subscription that is still running; Close
joins that invocation as it joins every other. This differs from a consumer setup refused
during Close ("A consumer refused during Close keeps its claim"), which waits for its handlers: a core subscription
holds no claim, its handlers' context descends from the call's own context, and Close's join already covers every
subscription of a failed setup.

The error SHALL be decided in this order. If Close has begun when the round trip ends, the call SHALL return
`nats.ErrConnectionClosed`, whatever the round trip's outcome and whether or not the call's context has ended, decided
by Close having begun and not by `Status()`. Otherwise the error SHALL be transient and match `context.Canceled` or
`context.DeadlineExceeded` when the context ended or the bound passed, and SHALL be transient and match
`ErrNotConnected`, not `nats.ErrConnectionClosed`, when the connection was lost or closed by anything other than this
client's Close. This requirement does not require the call to report a SUB the server refused, for permissions or for
the connection's limit on subscriptions; with nats.go v1.54.0 the call returns the subscription and nil. This
requirement does not say when a responder in another component has subscribed, nor what holds after a reconnect or
across servers in a cluster.

#### Scenario: A request from another connection right after the call

- **WHEN** `SubscribeForRequests`, or `Subscribe` with a handler that replies, returns without error on a subject the
  server permits the client to subscribe to, and another connection to the same server sends a request on the subject
  at once
- **THEN** the handler answers it, and the request never fails with "no responders"

#### Scenario: The server does not answer

- **WHEN** `Subscribe` is called with a context that has no deadline and the server reads every line but never
  answers the PING
- **THEN** the call returns once `DefaultRequestTimeout` has passed, with no subscription and a transient error
  matching `context.DeadlineExceeded`, and the server then reads an UNSUB for the subscription

#### Scenario: The caller's context ends during the round trip

- **WHEN** the call's context is cancelled while the round trip waits for a server that reads every line but has not
  answered
- **THEN** the call returns at once with no subscription and a transient error matching `context.Canceled`, and the
  server then reads an UNSUB for the subscription

#### Scenario: An ended context

- **WHEN** `Subscribe` is called on a connected client with a context that has already ended
- **THEN** it returns a transient error matching the context's error, and the server reads no SUB

#### Scenario: The connection is lost during the round trip

- **WHEN** the connection drops while the round trip waits for the server
- **THEN** the call returns a transient error that matches `ErrNotConnected` and does not match
  `nats.ErrConnectionClosed`, and the subscription is not among those the client restores when it reconnects

#### Scenario: A handler is running when the call fails

- **WHEN** a message reaches the subscription's handler while the round trip waits, the handler is still running, and
  `DefaultRequestTimeout` passes
- **THEN** the call returns a transient error matching `context.DeadlineExceeded` while the handler still runs, and a
  Close with a live context returns nil only after that handler returned

#### Scenario: Close begins during the round trip

- **WHEN** a message reaches the subscription's handler while the round trip waits, the handler is still running,
  Close begins, and the server then answers
- **THEN** the call returns an error matching `nats.ErrConnectionClosed` and no subscription while the handler still
  runs, and Close returns nil only after that handler returned

#### Scenario: Close ends the connection during the round trip

- **WHEN** Close begins while the round trip waits, the server never answers, and Close closes the connection when its
  context ends
- **THEN** the call returns an error matching `nats.ErrConnectionClosed` and not `ErrNotConnected`
