# service-lifecycle

## ADDED Requirements

### Requirement: The heartbeat service refuses an invalid configuration

`NewHeartbeatService` SHALL refuse, as invalid data and naming the input: an unknown configuration key or a key in
another letter case, nil dependencies, a nil service manager, and an `interval` that does not parse, is not positive or
is under 1 second. It SHALL look nothing up and do no I/O. An omitted or empty `interval` SHALL be 30 seconds.
`RegisterAll` SHALL register the constructor under `heartbeat`, and a composition built from a loaded document that
names no services SHALL construct the heartbeat, because `config`'s defaults enable it, and no metrics forwarder.

#### Scenario: A refused construction

- **WHEN** the constructor is given nil dependencies, a nil service manager, a document that does not parse, an
  interval that does not parse, is zero, negative, 500 ms or 999 ms, an unknown key or a key in another letter case
- **THEN** it returns an invalid-class error naming the input and no service
- **AND** the test that verifies this is `TestHeartbeatConstructionRefusals`

#### Scenario: A miscased key beside the default

- **WHEN** a loaded document enables `heartbeat` with the config `{"Interval":"5s"}`, which the loader merges beside
  the default `{"interval":"30s"}`, and the service manager composes it
- **THEN** composition fails with an error containing `config key Interval must be written interval`, and the
  service manager holds no heartbeat
- **AND** the test that verifies this is `TestHeartbeatRefusesAMiscasedKeyBesideTheDefault`

#### Scenario: The constructor is registered and the defaults compose it

- **WHEN** `RegisterAll` has filled a registry, and separately a document with only a `platform` block is composed
- **THEN** the registry offers constructors under `heartbeat` and `metrics-forwarder`, each building its own type,
  and the composition holds a heartbeat and no metrics forwarder
- **AND** the tests that verify this are `TestHeartbeatAndMetricsForwarderAreRegistered` and
  `TestHeartbeatComposedFromTheConfigDefaults`

### Requirement: The heartbeat's Start bounds Start; Stop is the only stop

`HeartbeatService.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second `Start` and a
`Start` after any `Stop` (`errs.ErrAlreadyStarted`), and a refused `Start` SHALL hold nothing. It SHALL look up the
service named `component-manager` through the service manager it was built with and refuse, naming
`component-manager`, when there is none or it is not the framework's `*ComponentManager`; that manager SHALL need to
be composed, not started. Otherwise it SHALL start the base service, then the heartbeat loop, and return nil. Ending
`Start`'s context after `Start` returned SHALL stop nothing. `Stop(ctx)` SHALL refuse a nil context and, given an ended
context, return its error and stop nothing. Otherwise it SHALL end the loop, wait for it and stop the base service,
returning nil only once both are done. If its context ends first, `Stop` SHALL return that error and the next `Stop`
SHALL wait again. A `Stop` after one that returned nil SHALL return nil, and a `Stop` while another runs SHALL be
refused as transient.

#### Scenario: The lifecycle suite

- **WHEN** the service is run through the `lifecycle-suite` checks
- **THEN** every check passes, including that a failed `Start` holds nothing and that ending `Start`'s context stops
  nothing
- **AND** the test that verifies this is `TestHeartbeatPassesTheLifecycleSuite`

#### Scenario: Start refuses without the framework's component manager

- **WHEN** `Start` is called on a heartbeat whose service manager holds no `component-manager`, or holds a stand-in
  that is not a `*ComponentManager`
- **THEN** `Start` returns an error naming `component-manager`, the status stays stopped and no loop is held
- **AND** the test that verifies this is `TestHeartbeatStartRefusesWithoutTheComponentManager`

#### Scenario: A failed Stop is retried

- **WHEN** the first `Stop` returns its context's error while the loop is held inside a log call
- **THEN** a second `Stop` has not returned while the loop is held, and after release a later `Stop` returns nil
- **AND** the test that verifies this is `TestHeartbeatFailedStopIsRetried`

### Requirement: The heartbeat logs system health at Start and every interval

The heartbeat service SHALL log one record, with the message `System heartbeat`, at level Info, once when it starts
and then once per `interval` until `Stop`, through the logger in its dependencies. The record SHALL carry `uptime` (the
base service's uptime rounded to the second, as a duration string), `goroutines` (the process's goroutine count),
`components_healthy` and `components_total` (how many of the component manager's components report healthy, out of how
many it holds). No record SHALL be written between ticks.

#### Scenario: A record states component health at Info

- **WHEN** the heartbeat starts over a component manager holding two healthy components and one unhealthy one
- **THEN** exactly one record is written at Info, with `components_healthy` 2, `components_total` 3, `uptime` `0s` and
  a `goroutines` attribute
- **AND** the test that verifies this is `TestHeartbeatRecordsComponentHealth`

#### Scenario: One record at Start and one per interval

- **WHEN** the heartbeat runs with intervals of 1 s, 3 s, the default and an empty value
- **THEN** one record is written at `Start`, none before the interval has passed, and one per interval after it, each
  carrying the uptime of its round; the default and the empty value both give 30 s
- **AND** the test that verifies this is `TestHeartbeatCadence`

### Requirement: The metrics forwarder refuses an invalid configuration

`NewMetricsForwarderService` SHALL refuse, as invalid data and naming the input: an unknown configuration key or a key
in another letter case, nil dependencies, a nil NATS client, a nil metrics registry, and a `push_interval` that does
not parse or is not positive. It SHALL do no I/O. An omitted or empty `push_interval` SHALL be 5 seconds, and
`include_go_metrics` and `include_proc_metrics` SHALL default to false. `RegisterAll` SHALL register the constructor
under `metrics-forwarder`, and `ResolveServiceConfigs` SHALL NOT add the service: it runs only when the configuration
enables it.

#### Scenario: A refused construction

- **WHEN** the constructor is given nil dependencies, a nil NATS client, a nil metrics registry, a document that does
  not parse, an interval with no unit, an unknown unit, a negative or zero value, an unknown key, or a key in another
  letter case (a flag key included)
- **THEN** it returns an invalid-class error naming the input and no service
- **AND** the test that verifies this is `TestMetricsForwarderConstructionRefusals`

#### Scenario: Omitted fields take the documented defaults

- **WHEN** the configuration is `{}`, absent or has an empty `push_interval`
- **THEN** the applied interval is 5 seconds and both include flags are false; set flags and intervals are applied as
  written
- **AND** the test that verifies this is `TestMetricsForwarderConstructionRefusals`

### Requirement: The forwarder's Start bounds Start; Stop is the only stop

`MetricsForwarder.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second `Start` and a
`Start` after any `Stop` (`errs.ErrAlreadyStarted`), and a refused `Start` SHALL hold nothing. Otherwise it SHALL start
the base service, then the publish loop, and return nil; it SHALL do no I/O and SHALL NOT fail on account of the NATS
connection. Ending `Start`'s context after `Start` returned SHALL stop nothing: later rounds still publish. `Stop(ctx)`
SHALL behave as the heartbeat's does: it ends the loop, waits for it and stops the base service; if its context ends
first it returns that error and the next `Stop` waits again. The forwarder SHALL record its state on
`semengine_service_status{service="metrics-forwarder"}`: stopped once built, running after `Start` and stopped after
`Stop`.

#### Scenario: The lifecycle suite

- **WHEN** the service is run through the `lifecycle-suite` checks
- **THEN** every check passes, including that a failed `Start` holds nothing and that ending `Start`'s context stops
  nothing
- **AND** the test that verifies this is `TestMetricsForwarderPassesTheLifecycleSuite`

#### Scenario: A failed Stop is retried

- **WHEN** the first `Stop` returns its context's error while the loop is held inside a log call
- **THEN** a second `Stop` has not returned while the loop is held, and after release a later `Stop` returns nil
- **AND** the test that verifies this is `TestMetricsForwarderFailedStopIsRetried`

#### Scenario: Publishing continues after the Start context ends

- **WHEN** the context given to `Start` is cancelled after `Start` returned and a counter is then raised
- **THEN** a later round publishes the raised value on the counter's subject
- **AND** the test that verifies this is `TestMetricsForwarderKeepsPublishingAfterStartContextEnds`

#### Scenario: The status is recorded

- **WHEN** the forwarder is built, started and stopped
- **THEN** `semengine_service_status{service="metrics-forwarder"}` reads 0, 2 and 0
- **AND** the test that verifies this is `TestMetricsForwarderReportsItsStatus`

### Requirement: The metrics forwarder publishes every sample and counts its drops

At `Start` and then every `push_interval`, the forwarder SHALL gather the metrics registry it was given and publish,
for each sample of each family, one JSON object with the fields `timestamp`, `name`, `component`, `type`, `value` and
`labels` on the subject `metrics.<component>.<name>`, through the NATS client's JetStream publish under its run
context. `component` SHALL be the sample's `component` label, else `system`. `type` SHALL be the family's type in lower
case, `untyped` when absent or of another kind. `value` SHALL be the counter, gauge or untyped value, or the sample
count of a histogram or summary. `labels` SHALL be an object, `{}` when the sample has none. The samples of one round
SHALL share one timestamp in Unix milliseconds. Families named `go_*` SHALL be left out unless `include_go_metrics` is
true, and families named `process_*` unless `include_proc_metrics` is true. A sample that does not encode or does not
publish SHALL increment `semengine_errors_total{service="metrics-forwarder",type="metrics_publish"}`; a gather that
fails SHALL increment `type="metrics_gather"` and skip that round; each SHALL be logged at Debug, and the loop SHALL go
on. The forwarder SHALL create no stream: a stream that captures `metrics.>` is the deployment's.

#### Scenario: One round per interval

- **WHEN** the forwarder runs with a `push_interval` of 100 ms over a client that never connects, for 350 ms
- **THEN** the probe counter's subject is attempted four times: once at `Start` and once per tick
- **AND** the test that verifies this is `TestMetricsForwarderCadence`

#### Scenario: The include flags select the families

- **WHEN** a `go_` counter, a `process_` counter and a plain counter are registered and the flags are varied
- **THEN** the plain counter is always attempted, the `go_` counter only with `include_go_metrics`, and the
  `process_` counter only with `include_proc_metrics`
- **AND** the test that verifies this is `TestMetricsForwarderIncludeFlagsReachThePublisher`

#### Scenario: Each sample is published with its subject, type, value and labels

- **WHEN** a counter labelled `component=probe` with two routes and an unlabelled gauge are registered and the
  forwarder starts over a real server with a stream on `metrics.>`
- **THEN** the stream holds the two route samples on `metrics.probe.probe_requests_total` and the gauge on
  `metrics.system.probe_free_bytes`, each carrying exactly the six payload fields, the type and value, the labels as
  set (`{}` for the gauge) and a timestamp between the call to `Start` and the read
- **AND** the test that verifies this is `TestMetricsForwarderPublishesEachSample`

#### Scenario: A failed publish is counted and the loop goes on

- **WHEN** every publish fails over a client that never connects
- **THEN** each failed publish is counted once on `metrics_publish` and logged at Debug naming its subject, later
  rounds still attempt the subject, and nothing is counted on `metrics_gather`
- **AND** the test that verifies this is `TestMetricsForwarderPublishFailureIsCounted`

#### Scenario: A failed gather is counted and skips the round

- **WHEN** a collector that reports an invalid metric is registered on the registry
- **THEN** each round's failed gather is logged and counted once on `metrics_gather`, nothing is published, and
  nothing is counted on `metrics_publish`
- **AND** the test that verifies this is `TestMetricsForwarderGatherFailureIsCounted`

#### Scenario: A sample that does not encode is counted and the round goes on

- **WHEN** a gauge holding NaN sorts before a counter in the same round
- **THEN** the counter is delivered, the NaN sample is not, and the failure is counted once on `metrics_publish`
- **AND** the test that verifies this is `TestMetricsForwarderCountsAnUnencodableSample`

Not covered by a test, and reviewed only: the construction's "no I/O" (no test observes the constructors for I/O; the
code makes none); the heartbeat's `goroutines` value (the test asserts the attribute exists, not its value); and a
label value spliced into a subject, whose publish outcome is counted like any other and is not tested. The sample
mapping, including a histogram's and a summary's sample count and the `untyped` fallback, is covered by
`TestMetricSamples*` in `service/metrics_forwarder_samples_test.go`.

## MODIFIED Requirements

### Requirement: A service configuration key in another letter case is refused

A service payload decoded strictly (the service manager's, the metrics service's, the heartbeat service's and the
metrics forwarder's) SHALL be refused when a key differs only in letter case from the name of the field it decodes
into, in any struct field, pointer or map value the payload type reaches, with an error naming the key as written and
as declared. This SHALL be the same check `config` applies to its own document. A guard test SHALL fail if
`config.Config`, `service.ManagerConfig`, `service.MetricsConfig`, `service.HeartbeatConfig` or
`service.MetricsForwarderConfig` reaches a shape the check does not walk: an embedded struct, a slice or array of
structs or maps, or a type that decodes itself.

#### Scenario: A metrics key in another letter case

- **WHEN** `service.NewMetrics` is given `{"Port":9091}`
- **THEN** it returns an error containing `config key Port must be written port` and no service
- **AND** the test that verifies this is `TestServiceConfigRefusesKeyInAnotherCase`

#### Scenario: A heartbeat or forwarder key in another letter case

- **WHEN** `NewHeartbeatService` is given `{"Interval":"30s"}`, or `NewMetricsForwarderService` is given
  `{"Push_Interval":"5s"}` or `{"Include_Go_Metrics":true}`
- **THEN** each returns an invalid-class error naming the key as written and no service
- **AND** the tests that verify this are `TestHeartbeatConstructionRefusals` and
  `TestMetricsForwarderConstructionRefusals`

#### Scenario: A payload type grows a shape the check does not walk

- **WHEN** the guard runs over the five payload types, and over a type with each unwalked shape
- **THEN** it reports nothing for the five, and names each unwalked shape
- **AND** the tests that verify this are `TestPayloadTypesReachOnlyShapesTheLetterCaseCheckWalks` and
  `TestLetterCaseWalkerGapsDetectsEachShape`
