# operator-surface

## ADDED Requirements

### Requirement: The storage report is served at /storage-observability/report

The storage observability service SHALL serve `GET <prefix>/report`, mounted under `/storage-observability`. The route
SHALL answer `GET` and `HEAD` and any other method with 405, and SHALL answer 200 whatever the report says: critical
pressure, an over-committed tier, an unreadable account and a report not yet read each answer 200 with the fact in the
body. The body SHALL carry `report_only` (always true), `synced` (whether the watch has read every current value at
least once), `updated_at` (present only once a change was applied), `summary` (`resources`, `pressure` as an object
that is never null, `not_evaluated`, and `worst_pressure` only when some row was evaluated), `account` (present only
once an account row was read) and `resources`. `resources` SHALL be every published row sorted by resource name, never
null and never filtered; the route SHALL take no filter parameter, so a row with no pressure state is still served. A
row with no bound of its own SHALL carry `pressure.evaluated_against` `account-tier` when its tier has a limit. The
route SHALL read the one consumer that feeds the metrics and the health message and SHALL NOT recompute any value. The
route's OpenAPI document SHALL describe `/report`, and `RegisterAll` SHALL register no spec for the service because its
spec names no schema.

#### Scenario: The shape of the response

- **WHEN** a snapshot with rows, an account and a collection time is served, and again with none of them
- **THEN** the body carries the fields above, `account` and `updated_at` are absent when unread, and `resources` and
  `summary.pressure` are empty, not null
- **AND** the tests that verify this are `TestStorageReportHTTP_ResponseRoundTrips`,
  `TestStorageReportHTTP_UnknownAccountIsAbsentNotZero` and `TestStorageReportHTTP_UnreadReportSaysSo`

#### Scenario: Not-evaluated rows stay visible

- **WHEN** the report holds a row with no pressure state
- **THEN** the route serves it, and `summary.not_evaluated` counts it; `worst_pressure` is absent when nothing was
  evaluated
- **AND** the tests that verify this are `TestStorageReportHTTP_NotEvaluatedRowsStayVisible` and
  `TestStorageReportHTTP_WorstPressureIsEmptyWhenNothingIsEvaluated`

#### Scenario: Critical pressure answers 200

- **WHEN** a row is at critical pressure
- **THEN** the route answers 200
- **AND** the test that verifies this is `TestStorageReportHTTP_CriticalPressureFailsNothing`

#### Scenario: Methods and prefix

- **WHEN** the route is called with `GET`, `HEAD` and another method, and mounted with a prefix with and without a
  trailing slash
- **THEN** `GET` and `HEAD` answer 200, another method answers 405, and both prefixes serve the route
- **AND** the tests that verify this are `TestStorageReportHTTP_AnswersGETAndHEAD`,
  `TestStorageReportHTTP_RejectsNonGET` and `TestStorageReportHTTP_PrefixWithOrWithoutTrailingSlash`

#### Scenario: The route and the metrics read one consumer

- **WHEN** one report consumer feeds both the route and the metrics
- **THEN** the route's rows and the published series agree for every resource
- **AND** the test that verifies this is `TestStorageReportHTTP_CannotDivergeFromTheMetricsSurface`

#### Scenario: The collector's rows reach the route

- **WHEN** a collection runs over a server holding bounded and unbounded streams
- **THEN** their rows, growth and pressure appear on the route and in the registry
- **AND** the test that verifies this is `TestStorageObservabilityCollectsPublishesAndServes`

### Requirement: The storage metrics are published under semengine_storage_

The storage observability service SHALL register 19 collectors on the injected metrics registry under the name
`storage-observability`, each named `semengine_storage_*`: `resource_used_bytes`, `resource_limit_bytes`,
`resource_headroom_bytes`, `resource_headroom_ratio`, `resource_growth_bytes_per_second`,
`resource_time_to_threshold_seconds` and `resource_pressure` (0 normal to 3 critical), labelled `resource`, `owner`,
`kind` and `tier`; `resources` and `pressure_resources`; nine `account_*` families per storage tier; and
`report_collected_timestamp_seconds`. The service SHALL write to the collector `metric.RegisterOrGet` returns, so a
second service built on the same registry writes the series the registry serves. A value that was not measured SHALL
publish no series, never zero. A series SHALL be deleted when its value goes away, when a resource leaves the report
(under the label tuple it was published with) and, for the per-tier series, when the consumer drops the account row.
The count and state families SHALL be emitted in full on every rebuild. The freshness gauge SHALL carry the newest
collection time of a published row, never move backwards, and ignore a zero time. The example alert rules at
`configs/prometheus/rules/storage-pressure.yml` SHALL read only published metric names, SHALL NOT fire on a report row
disappearing, SHALL name unbounded resources, and SHALL state that nothing is enforced.

#### Scenario: Per-resource series and absence

- **WHEN** a row is observed, with and without each optional measurement
- **THEN** its series carry the `semengine_storage_` names and labels, and an unmeasured value has no series
- **AND** the tests that verify this are `TestStorageMetrics_PublishesPerResourceSeries`,
  `TestStorageMetrics_AbsentMeasurementsPublishNoSeries`, `TestStorageMetrics_PressureSeverityEncoding` and
  `TestStorageMetrics_UnboundedResourceCarriesNoPressureSeries`

#### Scenario: Retraction

- **WHEN** a resource changes state or owner, or is forgotten, or the account row is dropped
- **THEN** the old series are deleted, and a later observation publishes afresh
- **AND** the tests that verify this are `TestStorageMetrics_StateTransitionRetractsTheStaleSeries`,
  `TestStorageMetrics_OwnerChangeDoesNotStrandTheOldSeries`, `TestStorageMetrics_ForgetRemovesEveryOneOfItsSeries`,
  `TestStorageAccountMetrics_TierThatDisappearsIsRetracted` and
  `TestStorageAccountMetrics_ForgetAccountRetractsEveryTierSeries`

#### Scenario: Aggregates and tier series

- **WHEN** rows and an account are observed
- **THEN** the count families are emitted in full, and the account families publish the tier's limit, over-commitment
  and own capacity picture, with an unbounded or unreadable limit explicit
- **AND** the tests that verify this are `TestStorageMetrics_AggregateCountsAreFullyEmitted`,
  `TestStorageAccountMetrics_OverCommittedTier`, `TestStorageAccountMetrics_UnboundedLimitIsNotApplicableNotSatisfied`,
  `TestStorageAccountMetrics_UnknownLimitIsDistinctFromUnbounded`,
  `TestAccountTierMetrics_PublishTheCeilingsOwnCapacity` and `TestAccountTierMetrics_AnUnevaluableCeilingIsExplicit`

#### Scenario: A second registration writes the canonical collectors

- **WHEN** a second metrics surface registers on the same registry
- **THEN** its observations appear in the series the registry serves
- **AND** the test that verifies this is `TestStorageMetrics_SecondRegistrationWritesTheCanonicalCollectors`

#### Scenario: The freshness gauge

- **WHEN** rows arrive out of order or with a zero collection time
- **THEN** the gauge carries the newest collection time and never moves backwards
- **AND** the tests that verify this are `TestStorageMetrics_FreshnessGaugeCarriesCollectionTime`,
  `TestStorageMetrics_FreshnessGaugeNeverMovesBackwards` and `TestStorageMetrics_FreshnessGaugeIgnoresAZeroTimestamp`

#### Scenario: The example alert rules read only published names

- **WHEN** the example rules file is parsed
- **THEN** every metric an alert reads is one the service publishes, no alert fires on a series disappearing, unbounded
  storage is named, and each alert says nothing is enforced
- **AND** the tests that verify this are `TestStorageAlertRules_ReferenceOnlyPublishedMetrics`,
  `TestStorageAlertRules_KeyNothingOnRowDisappearance`, `TestStorageAlertRules_MakeUnboundedResourcesVisible` and
  `TestStorageAlertRules_AreAnnotatedAndReportOnly`
