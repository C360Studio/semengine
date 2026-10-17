# transport-client

## ADDED Requirements

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

## MODIFIED Requirements

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
