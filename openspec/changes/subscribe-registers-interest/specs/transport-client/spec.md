# transport-client

## ADDED Requirements

### Requirement: A subscription is registered with the server when Subscribe returns

SUB and UNSUB are the NATS protocol lines that add and remove a subscription; the server answers a PING with a PONG
once it has read every line sent before it on that connection. `Subscribe` and `SubscribeForRequests` SHALL return a
subscription only after the NATS server has read its SUB: after subscribing, the client SHALL send a PING on the
connection the subscription was made on and wait for the PONG (one round trip), so a message or request sent from any
connection to that server after the call returns finds the subscription (changed behaviour: at the pin both returned
while the SUB could still be in the client's write buffer, `natsclient/client.go:834-849` and `request.go:359` at the
pin; #144). The round trip SHALL end when the server answers, when the call's context ends, or when
`DefaultRequestTimeout` has passed, whichever comes first. A call whose context has already ended SHALL send no SUB
and SHALL return a transient error matching the context's error.

When the call returns no subscription after subscribing (the round trip did not complete, or Close had begun), it
SHALL first end the subscription: it unsubscribes it, so the subscription leaves the client's set of subscriptions and
is not restored on a reconnect, and on a connected client its UNSUB is queued to the server, which drops the
subscription when it reads that line; on a connection Close is draining or has closed, Close's drain or the close
ends it instead. The call SHALL then return at once, without waiting for a handler invocation of the subscription that
is still running; Close joins that invocation as it joins every other. This differs from a consumer setup refused
during Close ("A consumer refused during Close keeps its claim"), which waits for its handlers: a core subscription
holds no claim, its handlers' context descends from the call's own context, and Close's join already covers every
subscription of a failed setup.

The error SHALL be decided in this order. If Close has begun when the round trip ends, the call SHALL return
`nats.ErrConnectionClosed`, whatever the round trip's outcome and whether or not the call's context has ended, decided
by Close having begun and not by `Status()`. Otherwise the error SHALL be transient and match `context.Canceled` or
`context.DeadlineExceeded` when the context ended or the bound passed, and SHALL be transient and match
`ErrNotConnected`, not `nats.ErrConnectionClosed`, when the connection was lost or closed by anything other than this
client's Close. This requirement does not say when a responder in another component has subscribed, nor what holds
after a reconnect or across servers in a cluster.

#### Scenario: A request from another connection right after the call

- **WHEN** `SubscribeForRequests`, or `Subscribe` with a handler that replies, returns without error, and another
  connection to the same server sends a request on the subject at once
- **THEN** the handler answers it, and the request never fails with "no responders"

#### Scenario: The server does not answer

- **WHEN** `Subscribe` is called with a context that has no deadline and the server never answers the PING
- **THEN** the call returns once `DefaultRequestTimeout` has passed, with no subscription and a transient error
  matching `context.DeadlineExceeded`, and the server then reads an UNSUB for the subscription

#### Scenario: The caller's context ends during the round trip

- **WHEN** the call's context is cancelled while the round trip waits for the server
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
