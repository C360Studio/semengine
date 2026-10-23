# transport-client

## MODIFIED Requirements

### Requirement: A setup context does not parent its handlers

`ConsumeStreamWithConfig`, `ConsumeInternalStreamWithConfig`, `Subscribe` and `SubscribeForRequests` SHALL use their
context for setup only and SHALL NOT retain it; a write the server is not reading holds the setup as "A write the server
is not reading holds the connection" states. Each handler's context SHALL descend from a context the client owns for
that consumer or subscription, which the client SHALL cancel after `Close` has begun, also when the consumer's or
subscription's delivery has already ended while a handler still runs; `Close` SHALL still join every handler
invocation (owner ruling, #9 comment 5994720412 item 2). `ConsumeStreamWithConfigContexts` keeps the caller's
`handlerCtx` as the handlers' parent. A `Subscribe` or `SubscribeForRequests` handler's context SHALL carry the trace
from the message's headers when it has one and no value of the call's context.

#### Scenario: A handler after the setup deadline

- **GIVEN** a consumer, or a `Subscribe` or `SubscribeForRequests` subscription, started with a setup context that then
  ends
- **WHEN** a message or request is delivered after that
- **THEN** its handler receives a context that has not ended

#### Scenario: Close begins while a handler runs

- **GIVEN** a consumer, or a `Subscribe` or `SubscribeForRequests` subscription, whose handler is running
- **WHEN** `Close` begins
- **THEN** the handler's context ends, and `Close` returns nil only after the handler has returned

#### Scenario: Close begins after the subscription ended while a handler runs

- **GIVEN** a `Subscribe` or `SubscribeForRequests` subscription unsubscribed while its handler still runs
- **WHEN** `Close` begins
- **THEN** the handler's context ends, and `Close` returns nil only after the handler has returned

#### Scenario: A handler carries no value of the setup context

- **GIVEN** a `Subscribe` subscription made with a setup context that carries a trace
- **WHEN** a message without trace headers is delivered, and then one with trace headers
- **THEN** the first handler's context carries no trace, and the second carries the trace from its headers

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
holds no claim, the handlers of both `Subscribe` and `SubscribeForRequests` run under a context the client owns ("A
setup context does not parent its handlers"), and Close's join already covers every subscription of a failed setup.

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
