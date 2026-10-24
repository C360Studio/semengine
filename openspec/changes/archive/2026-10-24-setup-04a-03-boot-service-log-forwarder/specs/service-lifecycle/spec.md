# service-lifecycle delta: setup-04a-03-boot-service-log-forwarder

## ADDED Requirements

### Requirement: The log forwarder refuses an invalid configuration

`NewLogForwarderService` SHALL refuse, as invalid data and naming the input: nil dependencies, a document that does not
parse or holds more than one JSON value, an unknown configuration key or a key in another letter case, and a
`min_level` other than `DEBUG`, `INFO`, `WARN` or `ERROR` after it is trimmed and upper-cased. It SHALL NOT require a
NATS client and SHALL do no I/O. An empty, whitespace-only or `null` configuration, and an omitted or empty
`min_level`, SHALL give `INFO`. Its exclusion list SHALL be the configured `exclude_sources`, each trimmed, with empty
entries dropped and duplicates removed in first-seen order, and nothing added. `RegisterAll` SHALL register the
constructor under `log-forwarder`, and `ResolveServiceConfigs` SHALL NOT add it: the service runs only when the
configuration enables it.

#### Scenario: A refused construction

- **WHEN** the constructor is given nil dependencies, `{"min_level":`, two JSON values, `{"min_level":"TRACE"}`,
  `{"min_level":"INVALID"}`, `{"min_levl":"debug"}`, `{"Min_Level":"debug"}` or `{"Exclude_Sources":["a"]}`
- **THEN** it returns an invalid-class error naming the input and no service
- **AND** the test that verifies this is `TestLogForwarderConstructionRefusals`

#### Scenario: Defaults and normalisation

- **WHEN** the policy resolves `{}`, `{"exclude_sources":[]}`, a lower-case and an upper-case level, and a list
  with repeated or padded entries
- **THEN** the level defaults to `INFO`, the list is a non-nil slice holding the configured entries once each, and no
  entry is added
- **AND** whitespace-only input, an explicit empty level and a padded level (`" warn "`) give the defaults or the
  trimmed level, as `TestResolveOwnsDefaultsNormalizationAndValidation` and the "padded level" row of
  `TestLogForwarderSaysItForwardsNothing` check
- **AND** the tests that verify this are `TestResolveOwnsDefaultsNormalizationAndValidation` and
  `TestValidateLevelAcceptsOnlyTheFourUpperCaseLevels`

#### Scenario: The constructor is registered and runs only when enabled

- **WHEN** `RegisterAll` has filled a registry, and separately a document enabling `log-forwarder` and one naming no
  services are composed
- **THEN** the registry offers a constructor under `log-forwarder` that builds a `*LogForwarder`, the first
  composition holds a log forwarder and the second holds none
- **AND** the tests that verify this are `TestLogForwarderIsRegistered` and `TestLogForwarderComposedWhenEnabled`

### Requirement: The log forwarder's Start bounds Start and Stop is the only stop

`LogForwarder.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second `Start` and a
`Start` after any `Stop` (`errs.ErrAlreadyStarted`), and a refused `Start` SHALL hold nothing and write no record.
Otherwise it SHALL start its base service and return nil; it SHALL start no goroutine of its own. Ending `Start`'s
context after `Start` returned SHALL stop nothing. `Stop(ctx)` SHALL be the base service's: it SHALL refuse a nil
context, return an ended context's error and stop nothing, and otherwise return nil once the base service has
stopped; a `Stop` after one that returned nil SHALL return nil.

#### Scenario: The lifecycle suite

- **WHEN** the service is run through the `lifecycle-suite` checks
- **THEN** every check passes, including that a failed `Start` holds nothing and that ending `Start`'s context stops
  nothing
- **AND** the test that verifies this is `TestLogForwarderPassesTheLifecycleSuite`

### Requirement: The log forwarder forwards nothing and says so

The log forwarder SHALL publish nothing, subscribe to nothing and hold no NATS connection: no log handler in this
module publishes logs to NATS. When it starts it SHALL write one record at level Warn, through the logger in its
dependencies (`slog.Default()` when none is given) carrying `service` set to `log-forwarder`, with the message
`log-forwarder forwards nothing: no log handler in this module publishes logs to NATS` and the attributes `service`
(`log-forwarder`, once), `min_level` (the resolved level's name) and `exclude_sources` (the resolved list, `[]` when
empty), and SHALL write no other record while it runs. It SHALL record its status on the injected metrics registry as
`semengine_service_status{service="log-forwarder"}`, as every service built on `BaseService` does.

#### Scenario: The record states the applied configuration

- **WHEN** the service is built from `{"min_level":"warn","exclude_sources":[" a ","a","","b"]}` and started, then
  stopped, then started again
- **THEN** exactly one record is written, at Warn, with `min_level` `WARN` and `exclude_sources` `["a","b"]`, and the
  second `Start` is refused
- **AND** the test that verifies this is `TestLogForwarderSaysItForwardsNothing`

#### Scenario: The status is recorded

- **WHEN** the service is built with a metrics registry, started and stopped
- **THEN** `semengine_service_status{service="log-forwarder"}` reads 0, then 2, then 0
- **AND** the test that verifies this is `TestLogForwarderReportsItsStatus`

### Requirement: The service package reaches no agentic package

`service` SHALL NOT import, directly or through its dependencies, any package of the agentic domain: no import path
with an element named `agentic` or beginning `agentic-`. SemStreams' `service/milestone_service.go`, the pin's only
`service` to `agentic` edge, is not ported (SETUP 03B design D4a, #30). A sensitivity test SHALL require the contract
test's failure for a temporary module whose `service` package reaches such a path.

#### Scenario: Dependency listing

- **WHEN** a contract test lists the dependencies of `./service`
- **THEN** no listed path has an `agentic` element
- **AND** the tests that verify this are `TestServiceReachesNoAgenticPackage` and
  `TestServiceReachesNoAgenticPackageSensitivity`

## MODIFIED Requirements

### Requirement: A service configuration key in another letter case is refused

A service payload decoded strictly (the service manager's, the metrics service's, the heartbeat service's, the
metrics forwarder's and the log forwarder's) SHALL be refused when a key differs only in letter case from the name of
the field it decodes into, in any struct field, pointer or map value the payload type reaches, with an error naming
the key as written and as declared. This SHALL be the same check `config` applies to its own document. A guard test
SHALL fail if `config.Config`, `service.ManagerConfig`, `service.MetricsConfig`, `service.HeartbeatConfig`,
`service.MetricsForwarderConfig` or `service.LogForwarderConfig` reaches a shape the check does not walk: an embedded
struct, a slice or array of structs or maps, or a type that decodes itself.

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

#### Scenario: A log forwarder key in another letter case

- **WHEN** `NewLogForwarderService` is given `{"Min_Level":"debug"}` or `{"Exclude_Sources":["a"]}`
- **THEN** it returns an invalid-class error naming the key as written and no service
- **AND** the test that verifies this is `TestLogForwarderConstructionRefusals`

#### Scenario: A payload type grows a shape the check does not walk

- **WHEN** the guard runs over the six payload types, and over a type with each unwalked shape
- **THEN** it reports nothing for the six, and names each unwalked shape
- **AND** the tests that verify this are `TestPayloadTypesReachOnlyShapesTheLetterCaseCheckWalks` and
  `TestLetterCaseWalkerGapsDetectsEachShape`
