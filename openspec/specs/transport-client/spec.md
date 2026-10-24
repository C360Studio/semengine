# transport-client Specification

## Purpose
The transport client is `natsclient`, the package that connects to NATS and wraps subscriptions, JetStream consumers
and requests. This capability states what a caller can rely on: what a publish acknowledgement does and does not
promise, when a consumed message is settled, that a nil context is refused before a call acts, that a handler panic
is recovered, that `Subscribe` and `SubscribeForRequests` return a subscription only after the server has read its
SUB, and what the callback of a retried `KVStore` update is given and when it
runs again. `Close` stays within the caller's deadline on a connection it dialled, refuses new work once it begins,
and returns nil only after every handler and background goroutine the client owns has returned.
## Requirements
### Requirement: Acknowledged is not durable on a memory stream

A publish acknowledgement from the client SHALL mean only that the server accepted the message into the stream's
storage class; the client SHALL expose the stream's storage class in its stream configuration, and a consumer of a
memory-backed stream SHALL NOT be promised the message across a server restart.

#### Scenario: Memory stream after restart

- **WHEN** a message is published to a memory-backed stream and acknowledged, and the server restarts
- **THEN** the message is absent, and a file-backed stream's message published the same way is present

### Requirement: Settlement follows the decision

A message consumed through the delivery helpers SHALL be acknowledged, negatively acknowledged, or terminated only
after the delivery work returned its decision; a panic in the work SHALL be settled as the result reports and SHALL
not acknowledge the message; a work item that outlasts the acknowledgement wait SHALL be kept from redelivery by the
heartbeat policy while the work runs, and SHALL be redelivered when the heartbeat stops.

#### Scenario: Long work with heartbeat

- **WHEN** the work runs past the consumer's acknowledgement wait with a healthy heartbeat
- **THEN** the message is not redelivered to a second handler while the first runs

#### Scenario: Stopped heartbeat

- **WHEN** the heartbeat stops while the work runs past the acknowledgement wait
- **THEN** the server redelivers the message

#### Scenario: Semantic retry

- **WHEN** the work returns a retry decision
- **THEN** the server redelivers the message durably and the delivery count increases

#### Scenario: Work panics

- **WHEN** the work panics
- **THEN** the result's decision is quarantine with the panic as its cause, the message is not acknowledged, and the
  result reports that the owner must stop

### Requirement: Close and Drain are bounded by the caller's context

`Subscription.Drain(ctx)` SHALL refuse a nil context (kept from the pin). `Client.Close(ctx)` and `Client.Connect(ctx)`
SHALL refuse a nil context without touching the connection (changed behaviour: at the pin `Close(nil)` panics on a
connected client, and `Connect(nil)` dials, then panics, leaking the dialled connection; design P13). The bound Close
puts on its drain is stated in "Close bounds its drain by the caller's context" and "Close ends a connection the server
is not reading".

#### Scenario: Nil Close context

- **WHEN** Close is called with a nil context on a connected client
- **THEN** Close returns an error naming the nil context, the connection stays open, and no drain is attempted

#### Scenario: Nil Connect context

- **WHEN** Connect is called with a nil context
- **THEN** Connect returns an error naming the nil context and no dial is attempted

#### Scenario: Nil Drain context

- **WHEN** Drain is called with a nil context
- **THEN** Drain returns an error naming the nil context and makes no server call

### Requirement: Close bounds its drain by the caller's context

`Close` SHALL bound its drain by the caller's context deadline and return within it, reporting the drain outcome, and a
later `Close` SHALL make no server call, and SHALL return nil once the first `Close`'s cleanup and the join are
complete; none of these SHALL be repeated or retried by the client on the caller's behalf.

#### Scenario: Close under an ended context

- **WHEN** Close is called with a deadline shorter than the drain needs
- **THEN** Close returns within the deadline, the connection is closed, and a second Close with a live context returns
  nil once no handler of the client is still running (see "A nil Close means every handler returned")

### Requirement: Close ends a connection the server is not reading

When the context ends before the drain has finished, `Close` SHALL close the socket of a connection this client dialled
and end any dial nats.go has in progress to reconnect it. That ends at once a write the server is not reading, and
closes the connection; the drain is cut short.

#### Scenario: Close while the server is not reading

- **GIVEN** a connection the client dialled, whose server has stopped reading, with a publish held in the write
- **WHEN** Close is called with a 50 ms deadline
- **THEN** Close returns the context's error within the deadline, the connection is closed, and the held publish
  returns an error

#### Scenario: Close while a reconnect is dialling

- **GIVEN** a client whose connection the server dropped, while nats.go dials a reconnect that is not answered
- **WHEN** Close is called with a 50 ms deadline
- **THEN** Close returns the context's error within the deadline

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

### Requirement: Only Close changes the status once Close begins

While a connection is installed and Close has not begun, a Connect that does not install its own connection SHALL NOT
change `Status()`. Once Close has begun, no writer but Close SHALL change `Status()`. It keeps its value through the
drain and is `Disconnected` for good once Close's cleanup has finished.

#### Scenario: Only Close changes the status once Close begins

- **WHEN** Close has begun and the health monitor, a connection handler or a Connect would change the status
- **THEN** Status keeps the value it had when Close began until Close's cleanup has finished, and reports
  Disconnected from then on

#### Scenario: A losing Connect leaves the winner's status

- **WHEN** two Connect calls overlap, one installs its connection and the other fails or is cancelled
- **THEN** Status is that of the installed connection

### Requirement: A delivery after the recorded end is refused, and counted

A delivery that nats.go hands over after the client has recorded its subscription's or consumer's end SHALL NOT run the
caller's handler. Each refusal SHALL be logged at warn level and counted as `late_delivery_refused` on the JetStream
error metric when the client has JetStream metrics configured.

#### Scenario: Late delivery on an acknowledged consumer

- **WHEN** a delivery reaches the client after it recorded the consumer's end
- **THEN** the handler does not run, the message is not settled, a warn record names the subject and ack policy,
  and the `late_delivery_refused` count rises by one

### Requirement: A late delivery is redelivered or lost by its ack policy

With acknowledgements, a delivery refused as late is redelivered. With `AckPolicy: "none"`, or on a core subscription,
it is lost; the owner accepted this loss (#9 comment 5980769459). A graceful drain SHALL NOT refuse a delivery this way.

#### Scenario: Late delivery without acknowledgements

- **WHEN** a delivery reaches the client after it recorded the end of a consumer with `AckPolicy: "none"`, or of a core
  subscription
- **THEN** the handler does not run and the message is lost

#### Scenario: Graceful drain

- **WHEN** deliveries are still arriving while the client drains gracefully
- **THEN** none of them is refused as a late delivery

### Requirement: A message handler's panic is recovered

A panic in a message handler passed to a Consume method SHALL be recovered by the client, which SHALL Nak the
message, log the panic at error level with the message's subject, and count it as `handler_panic` on the JetStream
error metric when the client has JetStream metrics configured (owner ruling 3, #9 comment 5985697767: only a root
process lets a panic end it). A Nak that fails SHALL be logged with the panic, not discarded.

#### Scenario: A handler panics

- **WHEN** a consumer's message handler panics
- **THEN** the panic does not escape, the message is Nak'd, an error record names the panic and the subject, and
  the `handler_panic` count rises by one

#### Scenario: The Nak after a panic fails

- **WHEN** a handler panics and the Nak that follows returns an error
- **THEN** the error record names the panic, the subject and the Nak error

### Requirement: A setup context does not parent its handlers

`ConsumeStreamWithConfig`, `ConsumeInternalStreamWithConfig`, `Subscribe` and `SubscribeForRequests` SHALL use their
context for setup only and SHALL NOT retain it; a write the server is not reading holds the setup as "A write the server
is not reading holds the connection" states.

#### Scenario: A handler after the setup deadline

- **GIVEN** a consumer, or a `Subscribe` or `SubscribeForRequests` subscription, started with a setup context that then
  ends
- **WHEN** a message or request is delivered after that
- **THEN** its handler receives a context that has not ended

### Requirement: A handler's context descends from one the client owns

Each handler's context SHALL descend from a context the client owns for that consumer or subscription, which the client
SHALL cancel after `Close` has begun, also when the consumer's or subscription's delivery has already ended while a
handler still runs; `Close` SHALL still join every handler invocation (owner ruling, #9 comment 5994720412 item 2).
`ConsumeStreamWithConfigContexts` keeps the caller's `handlerCtx` as the handlers' parent.

#### Scenario: Close begins while a handler runs

- **GIVEN** a consumer, or a `Subscribe` or `SubscribeForRequests` subscription, whose handler is running
- **WHEN** `Close` begins
- **THEN** the handler's context ends, and `Close` returns nil only after the handler has returned

#### Scenario: Close begins after the subscription ended while a handler runs

- **GIVEN** a `Subscribe` or `SubscribeForRequests` subscription unsubscribed while its handler still runs
- **WHEN** `Close` begins
- **THEN** the handler's context ends, and `Close` returns nil only after the handler has returned

### Requirement: A subscription handler's context carries the message's trace

A `Subscribe` or `SubscribeForRequests` handler's context SHALL carry the trace from the message's headers when it has
one and no value of the call's context.

#### Scenario: A handler carries no value of the setup context

- **GIVEN** a `Subscribe` subscription made with a setup context that carries a trace
- **WHEN** a message without trace headers is delivered, and then one with trace headers
- **THEN** the first handler's context carries no trace, and the second carries the trace from its headers

### Requirement: An exported call refuses a nil context before it acts

Every exported `natsclient` call that takes a context and returns an error SHALL refuse a nil context with an
invalid-data error before it touches client state, calls a callback or calls NATS (changed behaviour: at the pin
`Request`, `RequestWithHeaders`, `OpenFrameworkBucket` and their siblings panicked; PR #48, Codex F32).

#### Scenario: Nil context on a request or bucket call

- **WHEN** `RequestClassified`, `RequestWithHeaders`, `OpenFrameworkBucket` or any other exported context-taking,
  error-returning call is given a nil context on a connected client
- **THEN** it returns an invalid-data error, no handler runs and no bucket changes

### Requirement: A retrying request refuses what it cannot attempt

`RequestWithRetryClassified` SHALL take a `retry.Config` and make its attempts as `retry.Do` does: `MaxAttempts` counts
every attempt, and a count below one makes one attempt (a negative count is no longer refused). Changed:
`natsclient.RetryConfig`, whose `MaxRetries` counted the attempts after the first and was refused below zero, is
removed, #222; at the pin a negative count returned success with no reply, Codex F31.

#### Scenario: Negative retry count

- **WHEN** `RequestWithRetryClassified` is called with `MaxAttempts` 0 or -1 and a responder that answers
- **THEN** the responder's handler runs once and the call returns its reply

### Requirement: A retrying request returns a config retry.Do refuses before sending

A config `retry.Do` refuses SHALL be returned by `RequestWithRetryClassified` before anything is sent.

#### Scenario: A config retry.Do refuses

- **WHEN** `RequestWithRetryClassified` is called with a negative `InitialDelay`
- **THEN** it returns an error, whose class is not promised, and the responder's handler never runs

### Requirement: Only an attempt without a reply is retried

Only an attempt that received no reply SHALL be retried; a reply, an error reply included, ends the call.

#### Scenario: An error reply is not retried

- **WHEN** the responder's handler returns an error and `MaxAttempts` is 3
- **THEN** the handler runs once and the call returns the classified error

### Requirement: A retrying request checks its context before every attempt

`RequestWithRetryClassified` SHALL check the context before every attempt, the first included, and SHALL return a
caller's cancellation as an error that matches `context.Canceled` and that `errs.IsTransient` reports transient (Codex
F32).

#### Scenario: Ended context

- **WHEN** a retrying request is called with a context that has already ended
- **THEN** it returns an error matching `context.Canceled` and the responder's handler never runs

### Requirement: A cancellation while an attempt waits for its reply

`RequestWithHeaders`, `RequestClassified` and the retrying call SHALL return a cancellation that arrives while an
attempt waits for its reply as an error that matches `context.Canceled` (early check E1).

#### Scenario: Cancelled while an attempt is in flight

- **WHEN** a caller cancels `RequestWithHeaders`, `RequestClassified` or `RequestWithRetryClassified` while the
  responder holds the request
- **THEN** the call returns an error matching `context.Canceled`

### Requirement: Subscribe returns after the server has read the SUB

SUB and UNSUB are the NATS protocol lines that add and remove a subscription; the server answers a PING with a PONG once
it has read every line sent before it on that connection. `Subscribe` and `SubscribeForRequests` SHALL return a
subscription only after the NATS server has read its SUB (changed behaviour, #144).

#### Scenario: A request from another connection right after the call

- **WHEN** `SubscribeForRequests`, or `Subscribe` with a handler that replies, returns without error on a subject the
  server permits the client to subscribe to, and another connection to the same server sends a request on the subject
  at once
- **THEN** the handler answers it, and the request never fails with "no responders"

This requirement does not say when a responder in another component has subscribed, nor what holds after a reconnect or
across servers in a cluster.

### Requirement: Subscribe sends a PING and waits for the PONG

After subscribing, the client SHALL send a PING on the connection the subscription was made on and wait for the PONG
(one round trip), so a message or request sent from any connection to that server after the call returns finds the
subscription, unless the server refused the SUB. Changed behaviour: at the pin both calls returned while the SUB could
still be in the client's write buffer (`natsclient/client.go:834-849` and `request.go:359` at the pin).

#### Scenario: A PING follows the SUB

- **WHEN** `Subscribe` is called on a connected client
- **THEN** the client sends a PING on the same connection after the SUB, and the call returns after the PONG

### Requirement: The wait for the PONG is bounded

The wait for the PONG SHALL end when the server answers, when the call's context ends, or when `DefaultRequestTimeout`
has passed, whichever comes first. A call whose context has already ended SHALL send no SUB and SHALL return a transient
error matching the context's error.

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

### Requirement: Only the PONG wait of Subscribe is bounded by the call's context

The call's context and `DefaultRequestTimeout` SHALL bound the wait for the PONG only. Each step of the call that takes
nats.go's connection lock (checking the connection, writing the SUB and the PING, and, on a call that fails, clearing
the pending PONG and writing the UNSUB) first waits for any write in progress on that connection.

#### Scenario: Steps behind a write in progress

- **WHEN** another goroutine's write on the connection is in progress when the call needs the connection lock
- **THEN** the call's step waits for that write, whatever the call's context

#### Scenario: A call behind a held write

- **WHEN** the server is not reading, the send buffer is full and a write is held on the connection
- **THEN** the call is held until that write completes or nats.go's write timeout ends it

While the server is not reading and the connection's send buffer is full, a write on that connection, the call's own SUB
or PING or another goroutine's, is held, and so is the call, until that write completes or nats.go's write timeout (one
minute by default) ends it; a call behind more than one held write waits for each in turn. This requirement does not
bound those waits; "A write the server is not reading holds the connection" states them for every call the client makes.

### Requirement: A failed Subscribe ends its subscription

When the call returns no subscription after subscribing (the round trip did not complete, or Close had begun), it SHALL
first end the subscription: it unsubscribes it, so the subscription leaves the client's set of subscriptions and is not
restored on a reconnect, and on a connected client its UNSUB is queued to the server, which drops the subscription when
it reads that line; on a connection Close is draining or has closed, Close's drain or the close ends it instead.

#### Scenario: The subscription is unsubscribed

- **WHEN** the call returns no subscription on a connected client
- **THEN** the subscription is not among those the client restores on a reconnect, and the server reads its UNSUB

### Requirement: A failed Subscribe does not wait for a running handler

The call SHALL NOT wait for a handler invocation of the subscription that is still running; Close joins that invocation
as it joins every other. This differs from a consumer setup refused during Close ("A consumer refused during Close keeps
its claim"), which waits for its handlers.

#### Scenario: A handler is running when the call fails

- **WHEN** a message reaches the subscription's handler while the round trip waits, the handler is still running, and
  `DefaultRequestTimeout` passes
- **THEN** the call returns a transient error matching `context.DeadlineExceeded` while the handler still runs, and a
  Close with a live context returns nil only after that handler returned

### Requirement: Close's join covers every subscription of a failed Subscribe

Close's join SHALL cover every subscription of a failed setup, because a core subscription holds no claim and the
handlers of both `Subscribe` and `SubscribeForRequests` run under a context the client owns ("A handler's context
descends from one the client owns").

#### Scenario: Close joins the handler of a failed setup

- **WHEN** a call returned no subscription while a handler of that subscription still runs, and Close is called with a
  live context
- **THEN** Close returns nil only after that handler has returned

### Requirement: Subscribe returns ErrConnectionClosed once Close has begun

The error SHALL be decided in this order. If Close has begun when the call fails, the call SHALL return
`nats.ErrConnectionClosed`, whatever the round trip's outcome and whether or not the call's context has ended, decided
by Close having begun and not by `Status()`.

#### Scenario: Close begins during the round trip

- **WHEN** a message reaches the subscription's handler while the round trip waits, the handler is still running,
  Close begins, and the server then answers
- **THEN** the call returns an error matching `nats.ErrConnectionClosed` and no subscription while the handler still
  runs, and Close returns nil only after that handler returned

#### Scenario: Close ends the connection during the round trip

- **WHEN** Close begins while the round trip waits, the server never answers, and Close closes the connection when its
  context ends
- **THEN** the call returns an error matching `nats.ErrConnectionClosed` and not `ErrNotConnected`

### Requirement: Subscribe's error when Close has not begun

Otherwise the error SHALL be transient and match `context.Canceled` or `context.DeadlineExceeded` when the context ended
or the bound passed, and SHALL be transient and match `ErrNotConnected`, not `nats.ErrConnectionClosed`, when the
connection was lost or closed by anything other than this client's Close.

#### Scenario: The connection is lost during the round trip

- **WHEN** the connection drops while the round trip waits for the server
- **THEN** the call returns a transient error that matches `ErrNotConnected` and does not match
  `nats.ErrConnectionClosed`, and the subscription is not among those the client restores when it reconnects

### Requirement: Subscribe does not promise to report a refused SUB

The call SHALL NOT be relied on to report a SUB the server refused, for permissions or for the connection's limit on
subscriptions; with nats.go v1.54.0 the call returns the subscription and nil.

#### Scenario: A refused SUB

- **WHEN** the server refuses the SUB for permissions or for the connection's limit on subscriptions
- **THEN** with nats.go v1.54.0 the call returns the subscription and nil

### Requirement: Connect is bounded by its context

`Client.Connect(ctx)` SHALL return a transient error matching the context's error, without dialling, when the context
has already ended, unless Close has begun (Close refuses new work).

#### Scenario: An ended context

- **WHEN** Connect is called with a context that has already ended
- **THEN** Connect returns an error matching the context's error, and the server sees no connection

### Requirement: A pending Connect ends with its context

While Connect dials and while the server's answer to the connection is pending, the context's deadline and its
cancellation SHALL end the attempt: Connect ends the dial or closes the socket it dialled, and returns a transient error
matching the context's error. The context SHALL NOT bound the connection's later reconnects.

#### Scenario: A server that does not answer

- **GIVEN** a server address that accepts the connection and never answers
- **WHEN** Connect is called with a 100 ms deadline
- **THEN** Connect returns an error matching `context.DeadlineExceeded` within the deadline

#### Scenario: A reconnect after Connect's context has ended

- **GIVEN** a client connected with a context that has since ended
- **WHEN** the server drops the connection
- **THEN** the client reconnects, and a request answered by a responder succeeds

### Requirement: A write the server is not reading holds the connection

The client SHALL NOT bound the wait of a call held behind a write the server is not reading (ADR-112).

#### Scenario: A held write holds every call that needs the lock

- **WHEN** the server is not reading, the send buffer is full and a write is held
- **THEN** a call that needs the connection lock, such as Publish or Subscribe, waits for that write whatever its
  context

nats.go takes one lock for every write on a connection. While the server is not reading and the connection's send buffer
is full, a write holds that lock until it completes or nats.go's write timeout (one minute by default,
`WithWriteTimeout`) ends it, and every call that needs the lock waits, whatever its context: `Publish`,
`RequestWithHeaders` (and the classified calls through it), `Subscribe`, the JetStream and key-value calls, and a
consumer's setup. A call behind more than one held write waits for each in turn.

When the write timeout ends a write that was partly sent, the connection can stay connected and carry nothing until
nats.go's stale-connection check closes it and reconnects. That check sends a PING every 30 s by default
(`WithPingInterval`), and nats.go declares the connection stale at the third PING with no answer. The client does not
promise when that happens, nor that the connection is usable sooner.

### Requirement: A held call does not hold Close or Connect

`Close` is the exception: on a connection the client dialled it closes the socket when its context ends ("Close ends a
connection the server is not reading"), and every held call then returns an error. The client SHALL NOT hold its own
lock while it calls into nats.go, so a call held this way does not hold `Close` or `Connect`.

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

### Requirement: Close returns nil only after everything the client owns has finished

`Client.Close` SHALL return nil only after every piece of work the client owns has finished: its background work, every
running invocation of a message handler passed to `Subscribe`, `SubscribeForRequests` or a Consume method, the event
handlers of the connection it closes when this client dialled it, and its async publish error handler as far as nats.go
lets the client observe it (D3 declares the one residual).

#### Scenario: A nil Close means every handler returned

- **WHEN** a message handler passed to Subscribe or a Consume method is running and Close is called, first with an
  ended context and then with a live one
- **THEN** the first Close returns the context's error, and the second returns nil only after the handler returned

### Requirement: Close reports a drain that runs out of time

The join SHALL hold on every Close path, a drain that runs out of time included, which Close SHALL report as a transient
error wrapping `nats.ErrDrainTimeout` whichever timer ran out first.

#### Scenario: A drain that runs out of time is reported

- **WHEN** a message handler is still running when the drain timeout passes, and Close's context is live
- **THEN** the first Close returns an error wrapping the drain timeout, after the handler has returned

### Requirement: Each Close returns within its own context

Each Close SHALL return within its own context and SHALL NOT wait on another caller's drain.

#### Scenario: Each Close observes its own context

- **WHEN** one Close is draining and a second Close is called with an ended context
- **THEN** the second Close returns the context's error without waiting for the drain

### Requirement: Close from inside a client callback returns an error

A Close called from inside one of the client's callbacks SHALL return its context's error and never nil.

#### Scenario: Close from inside a client callback

- **WHEN** a client callback calls Close with a bounded context
- **THEN** Close returns that context's error and never nil

### Requirement: A retried update passes the revision it read

The callback of `natsclient.KVStore.UpdateWithRetryRead` SHALL get the stored bytes and the revision read, 0 for an
absent key. A callback error matching `ErrKVSkipWrite` SHALL make that run write nothing, and the call SHALL return 0
and no error. The returned revision SHALL be the call's own commit, or 0.

#### Scenario: A skip writes nothing

- **WHEN** a key is at revision R and the update callback returns a value with an error wrapping `ErrKVSkipWrite`
- **THEN** the call returns 0 and no error after one run, and the key keeps its value at revision R

#### Scenario: A conflict passes the newer revision

- **WHEN** the callback's first run reads revision R, and the key is written at revision E before that run's value
  commits
- **THEN** the second run gets the bytes written at E and the revision E

### Requirement: An unconfirmed write ends a retried update

`natsclient.KVStore.UpdateWithRetryRead` SHALL run its callback again only after the bucket refused the previous run's
write as a revision conflict, after the write returned `jetstream.ErrNoStreamResponse`, or after its read failed. Any
other error from the create or the update SHALL end the call with that error and revision 0. `UpdateWithRetryRev`,
`UpdateWithRetry` and `UpdateJSON` SHALL behave the same.

#### Scenario: A write that stood but returned an error

- **WHEN** the callback returns a value, the bucket stores it, and the write returns an error that is neither a
  conflict nor `ErrNoStreamResponse`
- **THEN** the call returns that error (`errors.Is`) and revision 0 after one run, and the key holds the value written

#### Scenario: A write no stream answered is retried

- **WHEN** the first run's write returns `ErrNoStreamResponse` and the second run's write is stored
- **THEN** the call returns that write's revision and no error, and the key was written once

### Requirement: An unclassified handler error is sent as fatal

When a `SubscribeForRequests` handler returns an unclassified error (no `*errs.ClassifiedError` in its chain and none of
the sentinels `errs.IsTransient`, `errs.IsFatal` or `errs.IsInvalid` list), the reply SHALL carry
`X-Error-Class: fatal`. A `RequestClassified` caller SHALL receive an error for which `errs.IsFatal` reports true and
`errs.IsTransient` reports false.

#### Scenario: A plain handler error

- **WHEN** a handler returns `errors.New("marshal failed")`
- **THEN** the reply's `X-Error-Class` is `fatal`, and the requester's error reports `errs.IsFatal` true and
  `errs.IsTransient` false

### Requirement: A handler error never arrives as reply data

`RequestClassified`, `RequestWithRetryClassified` and `RequestReadyClassified` SHALL return a reply that carries
`X-Status: error` as an error and no data: a `*errs.ClassifiedError` with the reply's class, code, message and detail.

#### Scenario: A classified call gets the handler's error

- **WHEN** a `SubscribeForRequests` handler returns an error with class invalid and a code, and the caller uses
  `RequestClassified`
- **THEN** the call returns no data and an error for which `errs.IsInvalid` reports true and whose `Code` is the
  handler's code

### Requirement: The raw request call returns an error reply as received

`RequestWithHeaders` SHALL return the reply message as received, headers included, with a nil error, and `ClassifyReply`
on that message SHALL return the same classified error.

#### Scenario: The raw call keeps the headers

- **WHEN** the same handler is called with `RequestWithHeaders`
- **THEN** the call returns a nil error and a message whose `X-Status` header is `error`, and `ClassifyReply` on it
  returns the classified error

### Requirement: No other call returns a reply

No other natsclient call SHALL send a request and return its reply (changed: at the pin `Request` and `RequestWithRetry`
returned an error reply's body as data with a nil error; both are removed, #222).

#### Scenario: Request and RequestWithRetry are gone

- **WHEN** the natsclient package is searched for `Request` and `RequestWithRetry`
- **THEN** neither exists

### Requirement: A failed call refuses no other call

No natsclient call SHALL refuse because other calls failed. A call that fails SHALL return its own error and SHALL leave
`Status()` as `Connect`, the connection's events, the health monitor and `Close` set it.

#### Scenario: Failures on one subject leave other calls working

- **WHEN** a connected client whose health monitor is off makes twenty `PublishToStream` calls to a subject no stream
  captures, and each fails
- **THEN** each error is that publish's own failure, a `PublishToStream` to a subject a stream captures then
  succeeds, and `Status()` reports `Connected`

### Requirement: There is no circuit breaker

Changed: at the pin, fifteen failed calls or dials with no success between them opened a circuit breaker; until its
backoff ended, JetStream and request calls returned `ErrCircuitOpen`, as did `Connect` while no connection was
installed, and the status then read `Disconnected` on a live connection until the health monitor's next tick. The
breaker is removed (#222), and natsclient SHALL NOT refuse a call for it.

#### Scenario: Many failures in a row

- **WHEN** fifteen or more calls fail with no success between them
- **THEN** a later call does not return `ErrCircuitOpen`, and `Status()` is not changed by the failures

### Requirement: The not-connected refusal carries the transient class

`ErrNotConnected` SHALL be transient by `errs.IsTransient` and by `errs.Classify`.

#### Scenario: Not connected

- **WHEN** a call returns `ErrNotConnected`
- **THEN** `errs.IsTransient` reports true and `errs.Classify` returns `ErrorTransient`

### Requirement: A refusal the server reports is counted

When the client was built with `WithMetrics`, it SHALL register a counter `semengine_nats_server_refusals_total` with
label `reason` on that registry through `metric.RegisterOrGet`. A client built without metrics SHALL write the log line
only.

#### Scenario: Metrics on and off

- **WHEN** a client is built with `WithMetrics`, and another without
- **THEN** the first registers `semengine_nats_server_refusals_total` with label `reason` on its registry, and the
  second writes the log line only

### Requirement: Each refusal the server reports adds one to the count

Each refusal nats.go reports to the client's async error handler as `nats.ErrPermissionViolation` SHALL add one to the
counter with reason `permission_violation`. Each reported as `nats.ErrMaxSubscriptionsExceeded` SHALL add one with
reason `max_subscriptions_exceeded`. A refusal SHALL be decided with `errors.Is`, never by the error's text, so a
refused publish and a refused SUB count alike.

#### Scenario: A subscription over the server's limit is counted

- **WHEN** a client with metrics subscribes and the server answers the SUB with "maximum subscriptions exceeded"
- **THEN** the call returns the subscription and nil, and the `max_subscriptions_exceeded` count rises by one

#### Scenario: A refused publish is counted the same way

- **WHEN** the server answers a publish from a client with metrics with a permissions violation
- **THEN** the `permission_violation` count rises by one

This requirement does not say how soon after the call returns the counter rises.

### Requirement: The count is made before the log line

The count SHALL be made before the client's `NATS error` log line for that refusal is written.

#### Scenario: A refused subscription is counted

- **WHEN** a client with metrics subscribes and the server answers the SUB with a permissions violation before the
  PONG
- **THEN** the call returns the subscription and nil, and when the refusal's `NATS error` record is written the
  `permission_violation` count has already risen by one

### Requirement: Subscribe still returns the subscription for a refused SUB

`Subscribe` and `SubscribeForRequests` SHALL still return the subscription and nil for a SUB the server refused (owner
ruling on #169, 2026-10-12; changed: before this change a refusal was only logged).

#### Scenario: A refused SUB returns the subscription

- **WHEN** a client subscribes and the server refuses the SUB
- **THEN** the call returns the subscription and nil

### Requirement: Both filtered key listings complete the same way

`KVStore.KeysByFilter` and `natsclient.FilteredKeys` SHALL list keys through one implementation over the reader's
`WatchFiltered`. They SHALL return every key in the watcher's initial snapshot, and nil with no error when it holds
none.

#### Scenario: No key matches

- **WHEN** no key in a bucket on a NATS server matches the filter
- **THEN** either call returns nil and no error

### Requirement: A filtered listing cut short returns no keys

When the context ends, or the watcher's update channel closes before the snapshot is complete, they SHALL return no keys
and an error, matching the context's error when it ended.

#### Scenario: Cancelled during the snapshot

- **WHEN** either call's context is cancelled after the first key arrives and before the snapshot completes
- **THEN** the call returns no keys and an error matching `context.Canceled`, and the watcher's update channel has
  closed

#### Scenario: The watcher closes before the snapshot completes

- **WHEN** the watcher's update channel closes before the end-of-snapshot marker, with the context live
- **THEN** either call returns no keys and an error

### Requirement: A filtered listing stops its watcher before returning

Before returning, they SHALL stop the watcher and wait at most 5 s, past the call's context, for its update channel to
close.

#### Scenario: The watcher is stopped

- **WHEN** either call returns
- **THEN** the watcher is stopped, and the call waited at most 5 s, past its context, for the update channel to close

### Requirement: The two filtered listings differ in their time bound

`KeysByFilter` SHALL also apply the store's operation timeout; `FilteredKeys` SHALL be bounded by its caller's context.
Changed: `FilteredKeys` read through nats.go's `ListKeysFiltered`, took a reader offering that method, and returned the
keys gathered so far when the lister's channel closed with the context live (#222).

#### Scenario: The bound of each call

- **WHEN** the listing does not complete in time
- **THEN** `KeysByFilter` ends at the store's operation timeout or its caller's context, and `FilteredKeys` ends only
  with its caller's context
