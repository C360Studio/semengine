# service-lifecycle

## MODIFIED Requirements

### Requirement: The message logger's Start bounds Start and Stop is the only stop

`MessageLogger.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second `Start` and a `Start`
after any `Stop` (`errs.ErrAlreadyStarted`), and a refused `Start` SHALL hold nothing. `Start` SHALL make each subscribe
under its context, check that context before each subscribe and once more after the last subscribe, before it logs that
it started, and, if it has ended at any of these checks, return that error and release everything the logger holds
(ADR-113). A subscribe that `Start`'s context ends records its subject as failed, and the next check fails `Start`.

One exception, accepted by the owner: when `Start`'s context cuts a subscribe after its SUB was sent, the NATS client
removes that subscription and returns none, so the logger never holds it. A message delivered to it before its removal
can still be stored after `Start` failed. The NATS client owns that callback, and its `Close` waits for it.

Ending `Start`'s context after `Start` returned SHALL stop nothing: later messages are stored. `Stop(ctx)` SHALL refuse
a nil context and, given an ended context, return its error and stop nothing. Otherwise it SHALL end the subscribe retry
and wait for it, drain every subscription so that a message being handled when `Stop` begins is stored, end every open
KV watch stream and wait for its KV watcher to stop, and stop the base service, returning nil only once all are
released. If its context ends first, or a drain fails, `Stop` SHALL return that error and leave what is not yet
released for the next `Stop`, which runs the same steps. A `Stop` after one that returned nil SHALL return nil; a `Stop`
while another runs SHALL be refused as transient. A message published after `Stop` SHALL NOT be stored.

The logger SHALL NOT store a message whose handler context has ended. It SHALL log each such message at Warn, naming
the subject, and count it in `dropped_messages`, not in `total_messages`. The NATS client ends a handler's context
after its `Close` has begun (transport-client, "A setup context does not parent its handlers"). Which messages arrive
after that is not promised (ADR-112); a composition root that stops the logger before closing its client drops none.

How long `Start` can run past its context's end is not promised: its context ends a subscribe's wait for the server's
answer, but not the waits for the connection that transport-client's "A write the server is not reading holds the
connection" states.

#### Scenario: The lifecycle suite

- **WHEN** the message logger is run through the `lifecycle-suite` checks
- **THEN** every check passes, including that a failed `Start` holds nothing and that ending `Start`'s context stops
  nothing
- **AND** the test that verifies this is `TestMessageLoggerPassesTheLifecycleSuite`

#### Scenario: A Start context that ends after the last subscribe

- **WHEN** the context given to `Start` is cancelled after its last subscribe succeeded, while `Start` still logs that
  subscribe
- **THEN** `Start` returns an error matching `context.Canceled`, logs no "MessageLogger started" record, holds nothing,
  and a later `Stop` returns nil
- **AND** the test that verifies this is `TestMessageLoggerStartFailsWhenItsContextEndsAfterSubscribing`

#### Scenario: A message whose handler context has ended

- **WHEN** the logger's subscription callback is handed a message with a context that has ended
- **THEN** the message is not stored and not counted in `total_messages`, `dropped_messages` counts it, and a Warn
  record names its subject
- **AND** the test that verifies this is `TestMessageLoggerCountsAndLogsADroppedMessage`

#### Scenario: Messages are still stored after the Start context ends

- **WHEN** the context given to `Start`, under which the logger subscribed, is cancelled after `Start` returned and a
  message is published
- **THEN** the message is stored
- **AND** the test that verifies this is `TestMessageLoggerKeepsCapturingAfterStartContextEnds`

#### Scenario: Stop stores the message being handled

- **WHEN** `Stop` begins while one message's handler is still running
- **THEN** `Stop` returns nil and that message and the one published after it are stored
- **AND** the test that verifies this is `TestMessageLoggerStopDrainsInFlightMessage`

#### Scenario: A failed Stop is retried

- **WHEN** the first `Stop` returns its context's error while a handler is held
- **THEN** a second `Stop` does not return nil until the handler is released, and a third `Stop` returns nil
- **AND** the test that verifies this is `TestMessageLoggerFailedStopIsRetried`

#### Scenario: Stop ends open watch streams

- **WHEN** `Stop` runs while a KV watch stream is open
- **THEN** `Stop` returns nil, the stream's body reaches its end and no stream is left open
- **AND** the test that verifies this is `TestMessageLoggerStopEndsOpenWatchStreams`

#### Scenario: Nothing is stored after Stop

- **WHEN** a message is published after `Stop` returned
- **THEN** it is not stored
- **AND** the test that verifies this is `TestMessageLoggerIgnoresPublishesAfterStop`

Not covered by a test, and reviewed only:

- that `Start`'s subscribes run under its context rather than the run context, and a context that ends between two
  subscribes of one `Start` or during a subscribe's wait for the server's answer (these need a hook this repository
  does not keep in production code, or a server that withholds its answer, which the logger's tests on the shared
  server do not have);
- a message stored by the handler of a subscribe that a failed `Start` cut;
- a `Stop` that must wait for a subscribe retry still running (`TestMessageLoggerRetriesSubscribeUntilConnected` stops
  the logger after the retry has ended, so it cannot tell a joined retry from an unjoined one);
- which messages reach the logger with an ended context during the NATS client's `Close` (the drop's signal is tested;
  its timing is not promised).
