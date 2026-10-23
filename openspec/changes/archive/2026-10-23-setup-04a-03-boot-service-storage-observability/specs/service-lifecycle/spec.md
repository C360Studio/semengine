# service-lifecycle

## ADDED Requirements

### Requirement: Storage observability's construction refuses an unusable configuration

`NewStorageObservabilityService` SHALL refuse, as invalid data and naming the input: an unknown configuration key
(keys nested under `pressure_thresholds` included) or a key in another letter case, nil dependencies, a nil NATS
client, a nil metrics registry, an `interval` or `timeout` that does not parse or is not positive, a `timeout` above
the `interval`, and pressure thresholds that do not resolve. It SHALL do no I/O. Defaults SHALL have one home: an
omitted `interval` is 1 minute, an omitted `timeout` is 15 seconds and omitted thresholds are the storage report
package's defaults. `RegisterAll` SHALL register the constructor under `storage-observability`, and
`ResolveServiceConfigs` SHALL NOT add the service: it runs only when the configuration enables it.

#### Scenario: A refused construction

- **WHEN** the constructor is given nil dependencies, a nil NATS client, a nil metrics registry, an interval or timeout
  that does not parse or is not positive, a timeout above the interval, thresholds that are inverted, out of range or
  carry an unparseable horizon, an unknown key, an unknown nested key or a key in another letter case
- **THEN** it returns an invalid-class error naming the input and no service
- **AND** the test that verifies this is `TestStorageObservabilityConstructionRefusals`

#### Scenario: Omitted fields take the documented defaults

- **WHEN** the configuration is `{}`
- **THEN** the applied interval is 1 minute, the applied timeout is 15 seconds and the thresholds equal the storage
  report package's resolved defaults
- **AND** the test that verifies this is `TestStorageObservabilityConfig_OmittedFieldsTakeDocumentedDefaults`

#### Scenario: The constructor is registered

- **WHEN** `RegisterAll` has filled a registry
- **THEN** the registry offers a constructor under `storage-observability`
- **AND** the test that verifies this is `TestStorageObservabilityService_IsRegistered`

`ResolveServiceConfigs` adds no `storage-observability` entry, so the service runs only when a configuration enables
it; the test that verifies this is `TestResolveServiceConfigsIsPureAndCanonical`.

### Requirement: Storage observability's Start bounds Start; Stop is the only stop

`StorageObservabilityService.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second `Start`
and a `Start` after any `Stop` (`errs.ErrAlreadyStarted`), and a refused `Start` SHALL hold nothing. `Start` SHALL start
the base service, then the collection loop and the report-watch loop, and return nil. It SHALL do no I/O and SHALL NOT
fail on account or bucket state: an account that cannot be read, or a report bucket that cannot be acquired, SHALL be
retried by the loops, each on its own schedule. Ending `Start`'s context after `Start` returned SHALL stop nothing:
later collections still publish. `Stop(ctx)` SHALL refuse a nil context and, given an ended context, return its error
and stop nothing. Otherwise it SHALL end both loops, wait for them and stop the base service, returning nil only once
all three are done. If its context ends first, `Stop` SHALL return that error and the next `Stop` SHALL wait again. A
`Stop` after one that returned nil SHALL return nil, and a `Stop` while another runs SHALL be refused as transient.
`Health` and the report route SHALL answer before `Start`, while running and after `Stop`, from the last report read.
How soon a collection already inside the account listing sees `Stop` begin is not promised: it depends on nats.go.

#### Scenario: The lifecycle suite

- **WHEN** the service is run through the `lifecycle-suite` checks over a client that never connects
- **THEN** every check passes, including that a failed `Start` holds nothing and that ending `Start`'s context stops
  nothing
- **AND** the test that verifies this is `TestStorageObservabilityPassesTheLifecycleSuite`

#### Scenario: Start succeeds with the account unreadable

- **WHEN** `Start` is given a service built over a client that never connects
- **THEN** `Start` returns nil, both loops report their retry warning, `Health` is healthy and says the storage report
  has not been read yet, and the route answers `synced` false
- **AND** the test that verifies this is `TestStorageObservabilityBootsWithoutAReachableAccount`

#### Scenario: Collection continues after the Start context ends

- **WHEN** the context given to `Start` is cancelled after `Start` returned and a stream is then created
- **THEN** the stream's row appears on the report route
- **AND** the test that verifies this is `TestStorageObservabilityKeepsCollectingAfterStartContextEnds`

#### Scenario: A failed Stop is retried

- **WHEN** the first `Stop` returns its context's error while the collection loop is held inside a log call
- **THEN** a second `Stop` has not returned while the loop is held, and after release the second and third `Stop`
  return nil
- **AND** the test that verifies this is `TestStorageObservabilityFailedStopIsRetried`

#### Scenario: Reads answer before Start, while running and after Stop

- **WHEN** the report route is read before `Start`, while running and after `Stop`
- **THEN** each read answers 200 with `report_only` true, and after `Stop` the health verdict is the base service's own
  with the last storage picture appended
- **AND** the test that verifies this is `TestStorageObservabilityRoutesAfterStop`

Not covered by a test, and reviewed only: a collection already inside the JetStream account listing when `Stop` begins
(no test holds a loop inside the listing), and the report bucket's lock, which `lazyReportBucket.resolve` holds while
the bucket is created or reconciled, so a slow server holds a second caller behind the first.

### Requirement: Storage pressure is reported and gates nothing

`StorageObservabilityService.Health` SHALL return the base service's `Healthy`, `Status` and sub-statuses unchanged for
every report, and SHALL only append the storage picture to the base message (joined with a semicolon, the base message
first when it is not empty). A critical pressure state, an over-committed tier, a report not yet read and a failed collection
SHALL NOT make the service unhealthy, degraded or not ready. The service SHALL set no health check of its own. The
health message SHALL name the pressure counts, up to five worst resources (own-bound findings before ones inherited
from the account tier, at equal severity), the over-committed or unknown-limit tiers, and the freshness of the last
collection.

#### Scenario: Health keeps the base verdict for any report

- **WHEN** a base status of any health, message and sub-statuses is combined with any snapshot and inventory
- **THEN** `Healthy`, `Status` and the sub-statuses equal the base's, and the message begins with the base message when
  that is not empty
- **AND** the tests that verify this are `TestPropStorageHealthKeepsTheBaseVerdict`,
  `TestStorageObservabilityHealth_CriticalPressureFailsNoGate`,
  `TestStorageObservabilityHealth_NeverCarriesANonHealthySubStatus` and
  `TestStorageObservabilityHealth_PreservesTheBaseVerdictsReason`

#### Scenario: Critical pressure leaves readiness and health green

- **WHEN** a manager composes the service over a server holding a stream whose pressure is critical, and the report
  route has served that critical row
- **THEN** `/readyz` answers `READY` and `/health` answers 200 with the critical count in the service's message
- **AND** the test that verifies this is `TestStorageObservabilityCriticalPressureKeepsReadiness`

#### Scenario: The message names what an operator should look at

- **WHEN** resources are not evaluated, a tier is over-committed, a collection failed or the report has not been read
- **THEN** the message says so, and a state inherited from the account tier names its tier and does not push an
  own-bound finding out of the five named resources
- **AND** the tests that verify this are `TestStorageObservabilityHealth_NamesUnevaluatedResources`,
  `TestStorageObservabilityHealth_ReportsAnOverCommittedTier`,
  `TestStorageObservabilityHealth_FailedCollectionIsReportedNotGated`,
  `TestStorageObservabilityHealth_ReportsWhenNothingHasBeenReadYet`,
  `TestInheritedPressure_HealthMessageNamesTheCeilingItCameFrom` and
  `TestInheritedPressure_DoesNotEvictOwnBoundFindingsFromTheHealthMessage`
