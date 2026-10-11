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
A connection installed through `SetConnection` was not dialled by the client, and a write held on it holds `Close` as
"A write the server is not reading holds the connection" states (ADR-112).

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

### Requirement: A consumer's setup context does not parent its handlers

`ConsumeStreamWithConfig` and `ConsumeInternalStreamWithConfig` SHALL use their context for setup only and SHALL
NOT retain it; a write the server is not reading holds the setup as "A write the server is not reading holds the
connection" states. Each handler's context SHALL descend from a context the client owns for that consumer, which the
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
those waits; "A write the server is not reading holds the connection" states them for every call the client makes.

When the call returns no subscription after subscribing (the round trip did not complete, or Close had begun), it
SHALL first end the subscription: it unsubscribes it, so the subscription leaves the client's set of subscriptions and
is not restored on a reconnect, and on a connected client its UNSUB is queued to the server, which drops the
subscription when it reads that line; on a connection Close is draining or has closed, Close's drain or the close
ends it instead. The call SHALL NOT wait for a handler invocation of the subscription that is still running; Close
joins that invocation as it joins every other. This differs from a consumer setup refused
during Close ("A consumer refused during Close keeps its claim"), which waits for its handlers: a core subscription
holds no claim, its handlers' context descends from the call's own context, and Close's join already covers every
subscription of a failed setup.

The error SHALL be decided in this order. If Close has begun when the call fails, the call SHALL return
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

## ADDED Requirements

### Requirement: Connect is bounded by its context

`Client.Connect(ctx)` SHALL return a transient error matching the context's error, without dialling, when the
context has already ended, unless Close has begun (Close refuses new work). While Connect dials and while the
server's answer to the connection is pending, the context's deadline and its cancellation SHALL end the attempt:
Connect ends the dial or closes the socket it dialled, and returns a transient error matching the context's error. The
context SHALL NOT bound the connection's later reconnects.

#### Scenario: An ended context

- **WHEN** Connect is called with a context that has already ended
- **THEN** Connect returns an error matching the context's error, and the server sees no connection

#### Scenario: A server that does not answer

- **GIVEN** a server address that accepts the connection and never answers
- **WHEN** Connect is called with a 100 ms deadline
- **THEN** Connect returns an error matching `context.DeadlineExceeded` within the deadline

#### Scenario: A reconnect after Connect's context has ended

- **GIVEN** a client connected with a context that has since ended
- **WHEN** the server drops the connection
- **THEN** the client reconnects, and a request answered by a responder succeeds

### Requirement: A write the server is not reading holds the connection

nats.go takes one lock for every write on a connection. While the server is not reading and the connection's send buffer
is full, a write holds that lock until it completes or nats.go's write timeout (one minute by default,
`WithWriteTimeout`) ends it, and every call that needs the lock waits, whatever its context: `Publish`, `Request`,
`Subscribe`, the JetStream and key-value calls, and a consumer's setup. A call behind more than one held write waits for
each in turn. The client does not bound these waits (ADR-112). `Close` is the exception: on a connection the client
dialled it closes the socket when its context ends ("Close and Drain are bounded by the caller's context"), and every
held call then returns an error. The client SHALL NOT hold its own lock while it calls into nats.go, so a call held this
way does not hold `Close` or `Connect`. When the write timeout ends a write that was partly sent, the connection can
stay connected and carry nothing until nats.go's stale-connection check closes it and reconnects. That check sends a
PING every 30 s by default (`WithPingInterval`), and nats.go declares the connection stale at the third PING with no
answer. The client does not promise when that happens, nor that the connection is usable sooner.

#### Scenario: A held call does not hold Close

- **GIVEN** a `Subscribe` held behind a write the server is not reading
- **WHEN** Close is called with a 50 ms deadline
- **THEN** Close returns the context's error within the deadline, and the `Subscribe` returns an error matching
  `nats.ErrConnectionClosed`

### Requirement: The write timeout and the PING interval are settable

`WithWriteTimeout(d)` SHALL set nats.go's write timeout, and `WithPingInterval(d)` SHALL set the interval of nats.go's
PINGs, for every connection the client dials. `NewClient` SHALL refuse either option with a value that is not
positive, returning an invalid-class error. Without them, the client SHALL use nats.go's one-minute write timeout and
a 30 s PING interval.

#### Scenario: A held write ends at the write timeout

- **GIVEN** a client with `WithWriteTimeout(300 * time.Millisecond)` and `WithPingInterval(300 * time.Millisecond)`,
  whose server has stopped reading, with a publish held in the write
- **WHEN** 300 ms pass
- **THEN** the held publish returns an error, and once the server reads again the client reconnects and a request
  answered by a responder succeeds

#### Scenario: A value that is not positive

- **WHEN** `NewClient` is called with `WithWriteTimeout(0)` or `WithPingInterval(-time.Second)`
- **THEN** it returns an error for which `errs.IsInvalid` reports true, and no client

### Requirement: The client's refusals carry the transient class

`ErrNotConnected` SHALL be transient by `errs.IsTransient` and by `errs.Classify`. `natsclient.ErrCircuitOpen` SHALL be
the sentinel `errs.ErrCircuitOpen`, so one `errors.Is` check matches it and both functions call it transient.

#### Scenario: Not connected

- **WHEN** a call returns `ErrNotConnected`
- **THEN** `errs.IsTransient` reports true and `errs.Classify` returns `ErrorTransient`

#### Scenario: Circuit open

- **WHEN** a call returns `natsclient.ErrCircuitOpen`
- **THEN** it matches `errs.ErrCircuitOpen` with `errors.Is`, and `errs.IsTransient` reports true
