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

### Requirement: Only Close changes the status once Close begins

While a connection is installed and Close has not begun, a Connect that does not install its own connection SHALL
change neither `Status()`, nor `Failures()`, nor the circuit. Once Close has begun, no writer but Close SHALL change
`Status()`. It keeps its value through the drain and is `Disconnected` for good once Close's cleanup has finished.

#### Scenario: Only Close changes the status once Close begins

- **WHEN** Close has begun and the health monitor, a connection handler, a failing operation or a Connect would
  change the status
- **THEN** Status keeps the value it had when Close began until Close's cleanup has finished, and reports
  Disconnected from then on

#### Scenario: A losing Connect leaves the winner's status

- **WHEN** two Connect calls overlap, one installs its connection and the other fails or is cancelled
- **THEN** Status, Failures and the circuit are those of the installed connection

### Requirement: A delivery after the recorded end is refused, and counted

A delivery that nats.go hands over after the client has recorded its subscription's or consumer's end SHALL NOT run
the caller's handler. Each refusal SHALL be logged at warn level and counted as `late_delivery_refused` on the
JetStream error metric when the client has JetStream metrics configured. With acknowledgements the message is
redelivered. With `AckPolicy: "none"`, or on a core subscription, it is lost; the owner accepted this loss
(#9 comment 5980769459). A graceful drain SHALL NOT refuse a delivery this way.

#### Scenario: Late delivery on an acknowledged consumer

- **WHEN** a delivery reaches the client after it recorded the consumer's end
- **THEN** the handler does not run, the message is not settled, a warn record names the subject and ack policy,
  and the `late_delivery_refused` count rises by one

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
every attempt, a count below one makes one attempt (a negative count is no longer refused), and a config `retry.Do`
refuses SHALL be returned before anything is sent (changed: `natsclient.RetryConfig`, whose `MaxRetries` counted the
attempts after the first and was refused below zero, is removed, #222; at the pin a negative count returned success with
no reply, Codex F31). Only an attempt that received no reply SHALL be retried; a reply, an error reply included, ends
the call. It SHALL check the context before every attempt, the first included, and SHALL return a caller's cancellation
as an error that matches `context.Canceled` and that `errs.IsTransient` reports transient, without counting it toward
the circuit breaker (Codex F32). No request call that counts failures (`RequestWithHeaders` and `RequestClassified`
through it, and the retrying call) SHALL count a caller's cancellation toward the circuit breaker, whether the context
ended before an attempt or while one waited for its reply; an attempt's own timeout still counts (early check E1).

#### Scenario: Negative retry count

- **WHEN** `RequestWithRetryClassified` is called with `MaxAttempts` 0 or -1 and a responder that answers
- **THEN** the responder's handler runs once and the call returns its reply

#### Scenario: A config retry.Do refuses

- **WHEN** `RequestWithRetryClassified` is called with a negative `InitialDelay`
- **THEN** it returns an error, whose class is not promised, and the responder's handler never runs

#### Scenario: An error reply is not retried

- **WHEN** the responder's handler returns an error and `MaxAttempts` is 3
- **THEN** the handler runs once and the call returns the classified error

#### Scenario: Ended context

- **WHEN** a retrying request is called with a context that has already ended
- **THEN** it returns an error matching `context.Canceled`, the responder's handler never runs, and the client's
  failure count is unchanged

#### Scenario: Cancelled while an attempt is in flight

- **WHEN** a caller cancels `RequestWithHeaders`, `RequestClassified` or `RequestWithRetryClassified` while the
  responder holds the request
- **THEN** the call returns an error matching `context.Canceled` and the client's failure count is unchanged

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
`WithWriteTimeout`) ends it, and every call that needs the lock waits, whatever its context: `Publish`,
`RequestWithHeaders` (and the classified calls through it), `Subscribe`, the JetStream and key-value calls, and a
consumer's setup. A call behind more than one held write waits for each in turn. The client does not bound these waits
(ADR-112). `Close` is the exception: on a connection the client dialled it closes the socket when its context ends
("Close and Drain are bounded by the caller's context"), and every held call then returns an error. The client SHALL NOT
hold its own lock while it calls into nats.go, so a call held this way does not hold `Close` or `Connect`. When the
write timeout ends a write that was partly sent, the connection can stay connected and carry nothing until nats.go's
stale-connection check closes it and reconnects. That check sends a PING every 30 s by default (`WithPingInterval`), and
nats.go declares the connection stale at the third PING with no answer. The client does not promise when that happens,
nor that the connection is usable sooner.

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
`RequestWithHeaders` SHALL return the reply message as received, headers included, with a nil error, and
`ClassifyReply` on that message SHALL return the same classified error. No other natsclient call sends a request and
returns its reply (changed: at the pin `Request` and `RequestWithRetry` returned an error reply's body as data with a
nil error; both are removed, #222).

#### Scenario: A classified call gets the handler's error

- **WHEN** a `SubscribeForRequests` handler returns an error with class invalid and a code, and the caller uses
  `RequestClassified`
- **THEN** the call returns no data and an error for which `errs.IsInvalid` reports true and whose `Code` is the
  handler's code

#### Scenario: The raw call keeps the headers

- **WHEN** the same handler is called with `RequestWithHeaders`
- **THEN** the call returns a nil error and a message whose `X-Status` header is `error`, and `ClassifyReply` on it
  returns the classified error
