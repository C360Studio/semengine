# service-lifecycle

## ADDED Requirements

### Requirement: The message logger's construction refuses an incomplete composition

`NewMessageLoggerService` SHALL refuse, as invalid data and naming the input: an unknown configuration key or a key in
another letter case, a nil NATS client, a nil payload registry, `"*"` in `monitor_subjects` (the default) without a
component registry, `max_entries` below 0 or above 100000, and `sample_rate` below 0. Defaults SHALL have one home: 0
`max_entries` means 10000, 0 `sample_rate` means every message, and an empty `monitor_subjects` means `["*"]`.
`RegisterAll` SHALL register the constructor under `message-logger`, and `ResolveServiceConfigs` SHALL NOT add it: the
service runs only when the configuration enables it.

#### Scenario: A refused construction

- **WHEN** the constructor is given a nil NATS client, a nil payload registry, `"*"` with no component registry, an
  out-of-range `max_entries`, a negative `sample_rate` or an unknown key
- **THEN** it returns an invalid-class error naming the input and no service
- **AND** the test that verifies this is `TestMessageLoggerConstructionRefusals`

### Requirement: The message logger's Start bounds Start and Stop is the only stop

`MessageLogger.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second `Start` and a `Start`
after any `Stop` (`errs.ErrAlreadyStarted`), and a refused `Start` SHALL hold nothing. `Start` SHALL check its context
before each subscribe and, if it has ended, return that error and release what it took. Ending `Start`'s context after
`Start` returned SHALL stop nothing: later messages are stored. `Stop(ctx)` SHALL refuse a nil context and, given an
ended context, return its error and stop nothing. Otherwise it SHALL end the subscribe retry and wait for it, drain
every subscription so that a message being handled when `Stop` begins is stored, end every open KV watch stream and wait
for its KV watcher to stop, and stop the base service, returning nil only once all are released. If its context ends
first, or a drain fails, `Stop` SHALL return that error and leave what is not yet released for the next `Stop`, which
runs the same steps. A `Stop` after one that returned nil SHALL return nil; a `Stop` while another runs SHALL be refused
as transient. A message published after `Stop` SHALL NOT be stored. How long `Start` can run past its context's end is
not promised.

#### Scenario: The lifecycle suite

- **WHEN** the message logger is run through the `lifecycle-suite` checks
- **THEN** every check passes, including that a failed `Start` holds nothing and that ending `Start`'s context stops
  nothing
- **AND** the test that verifies this is `TestMessageLoggerPassesTheLifecycleSuite`

#### Scenario: Messages are still stored after the Start context ends

- **WHEN** the context given to `Start` is cancelled after `Start` returned and a message is published
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

Not covered by a test, and reviewed only: a context that ends between two subscribes of one `Start` (it needs a hook
this repository does not keep in production code), and a `Stop` that must wait for a subscribe retry still running
(`TestMessageLoggerRetriesSubscribeUntilConnected` stops the logger after the retry has ended, so it cannot tell a
joined retry from an unjoined one).

### Requirement: The logger is unhealthy while a subject has failed to subscribe

A subscribe that fails, with any error, SHALL NOT fail `Start`: the logger SHALL keep running and retry each failed
subject every 100 ms until it subscribes. While any subject is unsubscribed, `/stats` SHALL report
`subject_reconciliation_degraded` as true with `subject_reconciliation_error` naming each subject and its last error,
and the logger's health check SHALL return an error naming those subjects, so that `IsHealthy` is false and `/readyz`
answers not ready. The base service runs its health check every 30 seconds, so each change in health SHALL show within
one interval. The first check, which runs before any subscribe, SHALL find a normal boot healthy.

#### Scenario: Healthy at the first check of a normal boot

- **WHEN** the logger starts with a connected NATS client
- **THEN** no subject is pending, so `/stats` does not report the logger degraded and the health check, which reads
  the same pending set, passes
- **AND** the test that verifies the statistics is `TestMessageLoggerMonitorsOnlyConfiguredSubjects`; no test runs the
  health check at the first interval of a normal boot, so that half is reviewed from the code only

#### Scenario: Unhealthy after a failed subscribe, healthy after the retry succeeds

- **WHEN** the logger starts on a NATS client that is not yet connected, so every subscribe fails, and the client is
  then connected
- **THEN** while unconnected the statistics report the degraded state and the health check fails; once connected the
  retry subscribes, both clear, and a published message is stored
- **AND** the test that verifies this is `TestMessageLoggerRetriesSubscribeUntilConnected`

### Requirement: The logger subscribes to a fixed subject set, covered ones once

`Start` SHALL compute the subject set once: the explicit `monitor_subjects` and, with `"*"`, the NATS, NATS-request and
JetStream port subjects of every component declaration the component registry holds, skipping `*`, `>` and `_INBOX`
subjects. After duplicates are removed, a subject that another subject in the set covers (`flowgraph.SubjectCovers`)
SHALL NOT be subscribed and SHALL be reported in `/stats` `subject_overlaps` as `broader`, `covered` and `resolution`
`covered subscription omitted`. Every subject in the set SHALL be covered by at least one subscribed subject, and no
subscribed subject SHALL cover another. Two subscribed subjects that overlap without either covering the other MAY
capture one message twice.

#### Scenario: Covered subjects are subscribed once

- **WHEN** a generated set of subjects is resolved
- **THEN** every subject is covered by a subscribed one and no subscribed subject covers another
- **AND** the tests that verify this are `TestPropCoveredSubjectsSubscribedOnce` and
  `TestResolveLoggerSubjectsOmitsCoveredSubjects`

#### Scenario: A covered subject's message is stored once

- **WHEN** `monitor_subjects` holds a subject and one it covers, and a message is published on the covered one
- **THEN** the covered subject is not subscribed, `subject_overlaps` lists it, and the message is stored once
- **AND** the test that verifies this is `TestMessageLoggerCapturesCoveredSubjectOnce`

#### Scenario: Component subjects are discovered with "*"

- **WHEN** `monitor_subjects` holds `"*"` and the component registry holds an admitted component
- **THEN** the logger subscribes to that component's declared subjects and stores a message published on one
- **AND** the test that verifies this is `TestMessageLoggerDiscoversAdmittedComponentSubjects`

### Requirement: A malformed monitored subject fails the logger's Start

`Start` SHALL check every subject of the set with `flowgraph.ValidSubject` before it removes duplicates or covered
subjects, before it starts the base service and before any NATS call. The first subject that fails SHALL refuse `Start`
as invalid data, with an error naming the subject and its source: `monitor_subjects`, or the component and port that
declare it. A refused `Start` SHALL hold nothing, and the refusal SHALL be the same whether or not the NATS client is
connected. The grammar is stricter than NATS on a wildcard inside a token (`a*`) and on form feed.

#### Scenario: Refused while disconnected, holding nothing

- **WHEN** `Start` is given `a..b`, `a.foo bar`, `a.>.b`, `a*` or an empty subject over a client that is not connected
- **THEN** it returns an error naming the subject and `monitor_subjects`, and the base service is not started and no
  subscription is held
- **AND** the test that verifies this is `TestMessageLoggerRefusesMalformedSubject`

#### Scenario: Refused when a broader subject covers it

- **WHEN** `monitor_subjects` is `["a.>", "a.foo bar"]`
- **THEN** `Start` is refused, naming `a.foo bar`
- **AND** the test that verifies this is `TestMessageLoggerRefusesMalformedSubjectCoveredByBroader`

Not covered by a test: the error text that names the component and port of a malformed subject a component declares. It
is the same `flowgraph.ValidSubject` call as the `monitor_subjects` check; only its wording is untested.
