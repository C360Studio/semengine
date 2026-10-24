# operator-surface Specification

## Purpose

The operator surface is what a person or a probe sees of a running SemEngine process over HTTP: the diagnostic routes
the service manager serves from the moment it binds its main listener (`/health`, `/healthz`, `/readyz`, `/services`,
`/services/health`), the full route set it serves once boot commits (each service's routes under its prefix,
`/graph/triples`, `/openapi.json`, `/docs`), the dedicated health listener, and the metrics service's claimed server.
This capability states what each route answers in each phase of the manager's lifecycle, what the API document
contains and in which shape, and what readiness means, so that a deployment's probes and an operator's tools can be
written against promises rather than against the pin's code. It is created by the `service` port (change 3 of epic
#9, service PR 2) and grows as later owners (the component manager, the heartbeat and the other services) add their
routes.
## Requirements
### Requirement: The service manager serves only diagnostics until boot commits

From the moment `StartAll` binds its main listener (`http_port`, default 8080, on every interface) until it commits, the
listener SHALL serve `/health`, `/healthz`, `/readyz`, `/services` and `/services/health`, and the component manager's
read-only routes when it provides them, and SHALL answer any other path with status 503 and the body `NOT READY`.

#### Scenario: Before and after the commit

- **WHEN** a manager has sealed its composition and serves its startup routes, and then every service has started
  and the full route set is committed
- **THEN** before the commit `/readyz` and `/graph/triples` each answer 503 with `NOT READY`; after it a committed
  route answers, and `/readyz` answers 200 with `READY`
- **AND** the test that verifies this is `TestStartupReadinessAndAtomicPromotion`

### Requirement: The full route set is served after boot commits

After the commit the manager SHALL serve the full route set: those five, `/graph/triples`, `/openapi.json`, `/docs` when
`swagger_ui` is true, each service's routes under its prefix, and the routes of components that register their own.

#### Scenario: A committed route answers

- **WHEN** every service has started and the full route set is committed
- **THEN** a route of the full route set, such as `/graph/triples`, answers instead of 503 `NOT READY`

### Requirement: A route set prepared but not committed is never served

A route set prepared but not committed SHALL never be served.

#### Scenario: The health publisher fails to start

- **WHEN** `StartAll` fails after preparing the full route set and before committing it
- **THEN** no request is ever served by the prepared routes
- **AND** the test that verifies this is `TestHealthPublisherFailureNeverCommitsPreparedRoutes`

### Requirement: /readyz says READY only after boot, while all is healthy

`/readyz` SHALL answer 200 with the body `READY` only when boot has committed, every service reports `StatusRunning`
and is healthy, and every component the component manager admitted reports healthy (ADR-109); otherwise it SHALL
answer 503 with `NOT READY`. It SHALL answer `NOT READY` from the moment `StopAll` begins, before any service's
`Stop` returns. `/healthz` SHALL answer 200 with `OK` whenever the listener serves.

#### Scenario: Readiness clears when StopAll begins

- **WHEN** a ready manager's `StopAll` is called and a service's `Stop` blocks
- **THEN** `/readyz` answers 503 with `NOT READY` while that `Stop` blocks, and `StopAll` returns nil once it returns
- **AND** the test that verifies this is `TestReadinessRequiresCommitAndClearsBeforeChildStop`

#### Scenario: An unhealthy service

- **WHEN** a ready manager's `component-manager` service turns unhealthy
- **THEN** `/readyz` answers 503
- **AND** the test that verifies this is `TestStartupReadinessAndAtomicPromotion`

### Requirement: Health and service listings are served as at the pin

`/health` SHALL answer the aggregate (`health.Aggregate`) of every service's `Health()` plus a `nats` sub-status when
the manager has a NATS client, with status 503 when the aggregate is unhealthy and 200 otherwise. The `nats` sub-status
SHALL be healthy with the message `Connected (RTT: <rtt>)` while connected, and unhealthy with a message beginning
`Disconnected:` otherwise.

#### Scenario: NATS disconnected

- **WHEN** `/health` is requested from a manager whose NATS client is not connected
- **THEN** it answers 503, and its `nats` sub-status is unhealthy with a message beginning `Disconnected:`
- **AND** the test that verifies this is `TestSystemHealthReportsNATSDisconnected`

### Requirement: /services and /services/health list the services

`/services` SHALL list each composed service's `name`, `status` and `healthy`, with the `startup` snapshot,
`restart_required` and `pending_service_changes`. `/services/health` SHALL answer each service's health and their
aggregate under `overall`, with status 503 when `overall` is unhealthy.

#### Scenario: Composed services are listed

- **WHEN** `/services` is requested from a manager composing `component-manager` and a service named `probe`
- **THEN** it lists both with their `status` and `healthy`, and `restart_required` is false
- **AND** the test that verifies this is `TestServicesListsComposedServicesWithHealth`

### Requirement: /openapi.json documents the mounted routes

`/openapi.json` SHALL answer an OpenAPI 3.0.0 document whose `info` is `server_info` (defaults `SemEngine API`,
`SemEngine framework API`, `0.0.0`), whose paths are each `HTTPHandler` service's paths under its prefix, and whose
component schemas are those of every registered spec's request and response types.

#### Scenario: A mounted service's routes

- **WHEN** a manager composes a service named `widgets` whose spec has the path `/items`, and `/openapi.json` is
  requested after boot
- **THEN** the body parses as JSON, its `openapi` is `3.0.0`, and its paths include `/widgets/items`
- **AND** the test that verifies this is `TestOpenAPIDocumentListsMountedRoutes`

### Requirement: Schema type names are distinct

`TypeNameFromReflect` SHALL name a pointer by its element, keep a named type's bare name, and give a slice, an array, a
map and an anonymous struct names that differ from their element's and from each other and match `^[A-Za-z0-9._-]+$`.

#### Scenario: Collections are named apart

- **WHEN** `TypeNameFromReflect` names a struct type `X`, `*X`, `[]X`, `[2]X`, `map[string]X` and an anonymous
  struct
- **THEN** `X` and `*X` are both `X`, the other four names differ from `X` and from one another, and every name
  matches `^[A-Za-z0-9._-]+$`
- **AND** the test that verifies this is `TestTypeNameFromReflectNamesEachShapeApart`

### Requirement: /docs serves the Swagger UI only when swagger_ui is true

`/docs` SHALL serve the Swagger UI page, which loads its scripts from `unpkg.com`, only when `swagger_ui` is true.

#### Scenario: Swagger UI switched off

- **WHEN** `swagger_ui` is false
- **THEN** `/docs` does not serve the Swagger UI page

### Requirement: /graph/triples answers a sorted, validated read of ENTITY_STATES

`GET /graph/triples` SHALL read `ENTITY_STATES` through the read-only catalog reader and answer a JSON array, never
`null`, of the triples matching every given filter: `subject` exactly, `predicate` exactly, `object` by its `%v` text. A
method other than `GET` SHALL answer 405. With no NATS client the manager SHALL answer `[]`.

#### Scenario: Read from a live bucket

- **WHEN** entities are written to `ENTITY_STATES` on a NATS server and `/graph/triples` is requested with a
  `subject`
- **THEN** it answers 200 with that entity's triples and no other entity's
- **AND** the test that verifies this is `TestGraphTriplesReadsEntityStates`

### Requirement: /graph/triples limit

`limit` SHALL default to 100, treat 0 as the default, clamp to 1000, and refuse a negative or non-integer value with
400.

#### Scenario: Limit values

- **WHEN** `/graph/triples` is requested with no `limit`, `limit=0`, `limit=5000`, `limit=-1` and `limit=abc`
- **THEN** the limits applied are 100, 100 and 1000, and the last two answer 400

### Requirement: /graph/triples refuses a malformed subject or predicate

A `subject` that is not a literal entity ID (`types.ValidateEntityID`) or a `predicate` that is not three parts
(`vocabulary.ParsePredicate`) SHALL be refused with 400 naming the parameter.

#### Scenario: Malformed filters

- **WHEN** `/graph/triples?subject=a.b` and `/graph/triples?predicate=x` are requested
- **THEN** each answers 400 with a body naming `subject` and `predicate` respectively
- **AND** the test that verifies this is `TestGraphTriplesRefusesMalformedSubjectAndPredicate`

### Requirement: /graph/triples sorts before the limit

The `/graph/triples` answer SHALL be the first `limit` matching triples of all entities taken in sorted key order.

#### Scenario: Sorted before the limit

- **WHEN** generated entities, filters and a limit are scanned through a stub reader that lists keys out of order
- **THEN** the answer equals the first `limit` triples of an oracle that sorts every matching triple by key and
  position
- **AND** the test that verifies this is `TestScanGraphTriplesMatchesSortedOracle`

### Requirement: /graph/triples read errors

A key deleted between listing and reading SHALL be skipped; any other read error, and an entity that
`graph.UnmarshalEntityState` refuses, SHALL answer 500.

#### Scenario: A read error other than a deleted key

- **WHEN** one key's read reports not found and another key's read fails with a timeout
- **THEN** the first is skipped and the answer is 500
- **AND** the test that verifies this is `TestGraphTriplesSkipsOnlyADeletedKey`

### Requirement: Products wrap every route through one middleware seam

`(*service.Manager).UseHTTPMiddleware(mws...)` SHALL append middleware applied to every route the manager serves,
the first given outermost, nil entries skipped. A call after the main listener started SHALL change nothing and log
a Warn (pin ADR-030).

#### Scenario: Middleware order

- **WHEN** two middlewares are registered before `StartAll` and a route is requested
- **THEN** the first registered sees the request first and the response last
- **AND** the test that verifies this is `TestManager_BuildHTTPHandler_AppliesRegisteredMiddleware`

#### Scenario: Too late

- **WHEN** `UseHTTPMiddleware` is called after the main listener started
- **THEN** a request is served without that middleware
- **AND** the test that verifies this is `TestManager_UseHTTPMiddleware_IgnoredAfterServerStarts`

### Requirement: A dedicated health listener serves /health and /healthz

`StartHealthListener(ctx, port)` SHALL do nothing and return nil for port 0. Otherwise it SHALL bind `:port` before
it returns, returning a bind error, and serve `/health` and `/healthz` with the main listener's answers until
`StopAll`, which SHALL stop it whether or not `StartAll` ran. `ctx` SHALL bound the bind only: ending it after the
call returned SHALL stop nothing and SHALL end no request's context. A second call, or a call after `StopAll` began,
SHALL be refused.

#### Scenario: Health listener

- **WHEN** `StartHealthListener` binds a port and `/healthz` is requested on it
- **THEN** it answers 200 with `OK`, and after `StopAll` a second `StartHealthListener` is refused
- **AND** the tests that verify this are `TestStartHealthListener_BindsHealthAndHealthz` and
  `TestHealthListenerCannotRebindAfterCompletedStop`

#### Scenario: The start context ends

- **WHEN** the context given to `StartHealthListener` is cancelled after it returned
- **THEN** `/healthz` on the health listener answers 200 with `OK`, and the context the server gives a new connection
  has not ended
- **AND** the test that verifies this is `TestHealthListenerRequestContextOutlivesStartContext`

### Requirement: The component manager's routes are served under /components

The component manager SHALL serve, under `/components/`, `health`, `list`, `types`, `types/{id}`, `status/{name}` (with
`?debug=true` adding a component's `DebugStatus`), `config/{name}`, `flowgraph` (JSON, or Mermaid with
`format=mermaid`), `validate` and `paths`. Each route SHALL answer only `GET` and SHALL answer any other method with
405.

#### Scenario: A method other than GET

- **WHEN** a route is asked with `POST`
- **THEN** the answer is 405
- **AND** the test that verifies this is `TestAttack_HandleComponentStatus_InvalidHTTPMethods`

### Requirement: Component routes refuse a malformed or unknown name

A name that is empty, `.`, `..` or decodes to a path separator SHALL be answered with 400, and an unknown name with 404.

#### Scenario: A malicious component name

- **WHEN** `/components/status/` is asked for `..` or a name that decodes to a path separator
- **THEN** the answer is 400
- **AND** the tests that verify this are `TestAttack_HandleComponentStatus_MaliciousComponentName` and
  `TestComponentRoutesRejectMalformedName`

### Requirement: Only some component routes are served before boot commits

Before boot commits only `health`, `list` and `status/` SHALL be served under `/components/`.

#### Scenario: A component route before the commit

- **WHEN** boot has not committed and `/components/flowgraph` is requested
- **THEN** it is not served; `health`, `list` and `status/` are

### Requirement: The component manager's OpenAPI spec is registered by RegisterAll

`RegisterAll` SHALL register the component manager's OpenAPI spec, so that every `$ref` in `/openapi.json` names a
schema it contains; importing the package SHALL register nothing.

#### Scenario: Every schema reference resolves

- **WHEN** a composition built through `RegisterAll` is started with `StartAll`
- **THEN** every `$ref` in `/openapi.json` names a schema in that document
- **AND** the test that verifies this is `TestOpenAPIDocumentResolvesEverySchemaRef`

### Requirement: The composition projections serve the result kept at construction

`/components/flowgraph`, `/components/validate` and `/components/paths` SHALL project the composition result the
component manager kept at construction and SHALL compute nothing. The flowgraph answer for a given composition SHALL be
stable: its JSON and Mermaid forms are a tested contract.

#### Scenario: The flowgraph for a fixture composition

- **WHEN** `/components/flowgraph` is asked for a fixture composition, as JSON and with `format=mermaid`
- **THEN** both answers equal the checked-in files under `service/testdata/`
- **AND** the test that verifies this is `TestFlowGraphMatchesGoldenForFixtureComposition`

#### Scenario: Validation projects the kept result

- **WHEN** `/components/validate` is asked after construction
- **THEN** it returns the findings the construction-time analysis produced
- **AND** the test that verifies this is `TestFlowValidationHandlerProjectsLibraryResult`

### Requirement: The message logger's routes are served under /message-logger

The message logger SHALL serve, under `/message-logger/`, `entries`, `stats`, `subjects`, `trace/{traceID}`,
`kv/{bucket}` and `kv/{bucket}/watch`. Each route SHALL answer `GET` and `HEAD` and SHALL answer any other method with
405.

#### Scenario: A malformed trace ID, or a method other than GET or HEAD

- **WHEN** `trace/` is asked for an ID that is not 32 hex characters, or any route is asked with `POST`, or
  `stats` with `HEAD`
- **THEN** the answers are 400, 405 and 200
- **AND** the tests that verify this are `TestTraceRouteRefusesMalformedID` and
  `TestMessageLoggerRoutesRefuseOtherMethods`

### Requirement: The message logger entries route returns the newest entries

`entries` SHALL return the newest stored entries, newest first; its `limit` SHALL default to 100 and be clamped to
10000, and a missing, non-numeric, zero or negative value SHALL give 100.

#### Scenario: The entries limit

- **WHEN** `entries` is asked with no limit, a limit above 10000, and a zero, negative or non-numeric limit
- **THEN** the limits applied are 100, 10000 and 100
- **AND** the test that verifies this is `TestEntriesLimit`

### Requirement: The message logger entries subject filter

The `subject` filter of `entries` SHALL be a NATS subject pattern matched with `flowgraph.SubjectCovers` and applied
before the limit, and a pattern NATS cannot parse SHALL match nothing.

#### Scenario: The subject filter applies before the limit

- **WHEN** `entries` is asked with a `subject` filter and a `limit` smaller than the number of entries that match
- **THEN** the answer holds that many matching entries, not the matches among the newest `limit` entries
- **AND** the tests that verify this are `TestEntriesFilterBeforeLimit` and `TestEntriesSubjectFilterUsesNATSMatching`

### Requirement: The message logger stats route

`stats` SHALL report `total_messages`, `dropped_messages` (messages not stored because their handler context had ended;
not counted in `total_messages`), `sampled_messages`, `valid_messages` (messages that decoded as typed envelopes),
`invalid_messages` (the rest), `sample_rate`, `max_entries`, `start_time`, `last_message_time`, `uptime_seconds`,
`monitored_subjects`, `subject_overlaps`, `subject_reconciliation_degraded` and `subject_reconciliation_error`.

#### Scenario: Valid and invalid counts

- **WHEN** a typed envelope and a raw payload are stored
- **THEN** `valid_messages` counts the first and `invalid_messages` the second
- **AND** the test that verifies this is `TestStatsCountsOnlyTypedEnvelopesValid`

#### Scenario: Dropped messages are counted

- **WHEN** the logger's callback is handed a message whose context has ended
- **THEN** `stats` reports `dropped_messages` 1 and `total_messages` 0
- **AND** the test that verifies this is `TestMessageLoggerCountsAndLogsADroppedMessage`

### Requirement: The message logger trace route

`trace/{traceID}` SHALL answer 400 unless the ID is 32 hex characters, and otherwise the entries still held for the
trace, oldest first.

#### Scenario: A well-formed trace ID

- **WHEN** `trace/` is asked for an ID of 32 hex characters
- **THEN** the answer holds the entries still held for that trace, oldest first

### Requirement: The message logger answers during and after Stop

`entries`, `stats` and `trace` SHALL answer from memory during and after `Stop`, and `subjects` SHALL answer `[]` after
`Stop`.

#### Scenario: Reads during and after Stop

- **WHEN** `entries`, `stats`, `trace` and `subjects` are asked after `Stop`
- **THEN** the first three answer from memory and `subjects` answers `[]`
- **AND** the test that verifies this is `TestMessageLoggerRoutesAfterStop`

### Requirement: The message logger serves a payload that is not JSON

A payload that is not JSON SHALL be stored and served as a JSON string, so every entry can be served.

#### Scenario: A message that is not JSON

- **WHEN** a payload that is not JSON has been stored
- **THEN** `entries` and `trace` answer every entry, the payload as a JSON string
- **AND** the test that verifies this is `TestEntriesServeNonJSONPayload`

### Requirement: The message logger's OpenAPI spec is registered by RegisterAll

`RegisterAll` SHALL register the message logger's OpenAPI spec, so that every `$ref` in `/openapi.json` names a schema
it contains; importing the package SHALL register nothing.

#### Scenario: Every schema reference resolves

- **WHEN** a composition built through `RegisterAll` enables `message-logger` and is started
- **THEN** every `$ref` in `/openapi.json` names a schema in that document, `MessageLogEntry` among them
- **AND** the test that verifies this is `TestOpenAPIDocumentResolvesEverySchemaRef`

### Requirement: The KV query reads an existing bucket and never creates one

`kv/{bucket}` SHALL answer 400 for a bucket name that is empty, `.`, `..` or holds `/`, `\`, `*` or `>`, a check shared
with the watch route.

#### Scenario: A malformed bucket name

- **WHEN** either KV route is asked for a bucket name that is empty, a path step or holds a separator or wildcard
- **THEN** the answer is 400
- **AND** the tests that verify this are `TestKVRoutesRejectMalformedBucket` and `TestIsValidBucketName`

### Requirement: The KV query looks the bucket up without creating it

The KV query SHALL look the bucket up without creating it and answer 404 when it does not exist, 500 for another lookup,
listing or read failure, and an empty result for a bucket with no keys.

#### Scenario: A missing bucket

- **WHEN** a bucket that does not exist is asked for
- **THEN** the answer is 404 and no bucket was created; a lookup failure other than not-found answers 500
- **AND** the tests that verify this are `TestKVQueryMissingBucketIs404AndCreatesNothing` and
  `TestKVQueryLookupFailureIs500`

### Requirement: The KV query sorts keys before the limit and matches a key glob

The KV query SHALL sort keys before `limit` applies (default 100, clamped to 1000), and `pattern` SHALL be a key glob:
`*` or empty for every key, one `*` as prefix, suffix, both ends or in the middle (the prefix and suffix not sharing a
character), or none for an exact key.

#### Scenario: Reads of existing buckets

- **WHEN** an existing bucket is queried, empty or holding entries, with a key glob, a status filter and a limit
- **THEN** the answers hold the matching entries, the stored values unrounded, keys sorted before the limit
- **AND** the tests that verify this are `TestKVQueryReadsExistingBucketsWithoutCreating`,
  `TestKVQueryEmptyBucketReturnsNoEntries`, `TestKVQueryFailedRecordRoundTrips`, `TestKVQuerySortsKeysBeforeLimit`,
  `TestKVQueryPreservesJSONValues`, `TestKVQueryPatternMatchesBeforeLimit`, `TestRecordMatchesStatus` and
  `TestMatchesPatternMiddleWildcard`

### Requirement: The KV query answers values as stored

The KV query SHALL answer each value as stored, so large numbers are not rounded, and a value that is not JSON as a JSON
string. `status` SHALL keep only records whose top-level JSON `status` string equals it.

#### Scenario: Values and status filter

- **WHEN** a record holds a large number, another value is not JSON, and `status` is given
- **THEN** the large number is not rounded, the value that is not JSON is a JSON string, and only records whose
  top-level `status` equals the filter are kept

### Requirement: The KV query leaves out a key deleted before its read

A key deleted or purged between the listing and its read SHALL be left out by the KV query; any other read error SHALL
answer 500 naming the key.

#### Scenario: A key deleted between listing and read

- **WHEN** a key is deleted or purged between the listing and its read
- **THEN** the key is left out; any other read error answers 500 naming the key

Not covered by a test: the two read branches that need the bucket to change, or the server to fail, between the key
listing and the read (a key left out because it was deleted; any other error answering 500). Sorting the keys is also
not detected by `TestKVQuerySortsKeysBeforeLimit`: the pinned `nats.go` already returns them sorted, so removing the
sort does not fail the test, and the sort is kept because that order is not promised.

### Requirement: The KV watch ends on client exit, falling behind or Stop

`kv/{bucket}/watch` SHALL answer 503 once `Stop` has begun, 400 for a bad bucket name or `pattern`, 404 for a bucket
that does not exist and 500 for another lookup failure, and none of these SHALL open a stream or create a bucket.

#### Scenario: Refusals open nothing

- **WHEN** the watch is asked for an invalid filter, a missing bucket, or after `Stop` began
- **THEN** the answers are 400, 404 and 503
- **AND** the tests that verify this are `TestKVWatchRejectsInvalidFilter`, `TestKVWatchMissingBucketIs404` and
  `TestKVWatchAfterStopIs503`

### Requirement: The KV watch pattern

An empty or `*` `pattern` SHALL watch every key; any other SHALL be a NATS KV key filter, a final `>` accepted and a
space refused.

#### Scenario: A watch filter

- **WHEN** the watch is asked for a `pattern` that is empty or `*`, one with a final `>`, and one with a space
- **THEN** the first watches every key, the second is accepted as a NATS KV key filter, and the third is refused with
  400

### Requirement: The KV watch streams Server-Sent Events in order

Otherwise the watch SHALL stream Server-Sent Events: `connected` with the bucket, pattern and a message, `retry: 5000`,
one `kv_change` for each key's current value, one `kv_change` with operation `initial_sync_complete`, then one
`kv_change` for each later change.

#### Scenario: Initial values, then changes

- **WHEN** a watch is opened on a bucket that holds keys and a key is then written and deleted
- **THEN** the stream carries `connected`, the current values, `initial_sync_complete`, then the `put` and `delete`
- **AND** the test that verifies this is `TestKVWatchStreamsInitialValuesThenChanges`

### Requirement: A kv_change event carries what NATS records

A `kv_change` event's `operation` SHALL be what NATS records, `put`, `delete` or `purge`, and its `value` SHALL be
present for a `put` only. The logger SHALL NOT read `Last-Event-ID`.

#### Scenario: Operation and value

- **WHEN** a key is put, deleted and purged under a watch
- **THEN** the `kv_change` events carry `put`, `delete` and `purge`, and only the `put` event carries a `value`

### Requirement: A KV watch stream buffers at most 100 events

Each stream SHALL buffer at most 100 events between its KV watcher and the client; the first event that does not fit
SHALL stop the watcher.

#### Scenario: The buffer is full

- **WHEN** a client does not read and more than 100 events arrive
- **THEN** the first event that does not fit stops the KV watcher

### Requirement: A dropped event ends the stream with an error event

After the watcher stops for a dropped event, the stream SHALL send the events already buffered, then one `error` event
with `{"error":"watch fell behind: events were dropped"}`, and end, so no `kv_change`, and no `initial_sync_complete`,
follows a dropped event.

#### Scenario: A client that falls behind

- **WHEN** a client that does not read opens a watch on a bucket holding more keys than the stream buffers
- **THEN** it gets the buffered events, then one `error` event, then the end of the stream, no
  `initial_sync_complete` anywhere, and fewer `kv_change` events than keys
- **AND** the test that verifies this is `TestKVWatchFallingBehindEndsStream`

Review only: when a client falls behind, `nats.go` waits for room when it puts an entry on the watcher's 256-entry
updates channel. If more than 256 entries arrive between the reader's last receive and its stop, a `nats.go` goroutine
could stay blocked until the stop. This was not probed and no real-world trigger is named, so the behaviour is not
promised.

### Requirement: The KV watch ends cleanly

The stream SHALL end when the client leaves or the logger stops, and the KV watcher and its reader SHALL have ended
before the handler returns.

#### Scenario: Client leaves

- **WHEN** a client closes a watch stream
- **THEN** the stream ends, and the KV watcher and its reader have ended before the handler returns

### Requirement: The storage report is served at /storage-observability/report

The storage observability service SHALL serve `GET <prefix>/report`, mounted under `/storage-observability`. The route
SHALL answer `GET` and `HEAD` and any other method with 405.

#### Scenario: Methods and prefix

- **WHEN** the route is called with `GET`, `HEAD` and another method, and mounted with a prefix with and without a
  trailing slash
- **THEN** `GET` and `HEAD` answer 200, another method answers 405, and both prefixes serve the route
- **AND** the tests that verify this are `TestStorageReportHTTP_AnswersGETAndHEAD`,
  `TestStorageReportHTTP_RejectsNonGET` and `TestStorageReportHTTP_PrefixWithOrWithoutTrailingSlash`

### Requirement: The storage report answers 200 whatever the report says

The route SHALL answer 200 whatever the report says: critical pressure, an over-committed tier, an unreadable account
and a report not yet read each answer 200 with the fact in the body.

#### Scenario: Critical pressure answers 200

- **WHEN** a row is at critical pressure
- **THEN** the route answers 200
- **AND** the test that verifies this is `TestStorageObservabilityCriticalPressureKeepsReadiness`, which reads the
  critical row through the route; `TestStorageReportHTTP_CriticalPressureFailsNothing` checks the body it serves

### Requirement: The storage report body fields

The body SHALL carry `report_only` (always true), `synced` (whether the watch has read every current value at least
once), `updated_at` (present only once a change was applied), `summary` (`resources`, `pressure` as an object that is
never null, `not_evaluated`, and `worst_pressure` only when some row was evaluated), `account` (present only once an
account row was read) and `resources`.

#### Scenario: The shape of the response

- **WHEN** a snapshot with rows, an account and a collection time is served, and again with none of them
- **THEN** the body carries the fields above, `account` and `updated_at` are absent when unread, and `resources` and
  `summary.pressure` are empty, not null
- **AND** the tests that verify this are `TestStorageReportHTTP_ResponseRoundTrips`,
  `TestStorageReportHTTP_UnknownAccountIsAbsentNotZero` and `TestStorageReportHTTP_UnreadReportSaysSo`

### Requirement: The storage report lists every published row

`resources` SHALL be every published row sorted by resource name, never null and never filtered; the route SHALL take no
filter parameter, so a row with no pressure state is still served. A row with no bound of its own SHALL carry
`pressure.evaluated_against` `account-tier` when its tier has a limit.

#### Scenario: Not-evaluated rows stay visible

- **WHEN** the report holds a row with no pressure state
- **THEN** the route serves it, and `summary.not_evaluated` counts it; `worst_pressure` is absent when nothing was
  evaluated
- **AND** the tests that verify this are `TestStorageReportHTTP_NotEvaluatedRowsStayVisible` and
  `TestStorageReportHTTP_WorstPressureIsEmptyWhenNothingIsEvaluated`

### Requirement: The storage report and the metrics read one consumer

The route SHALL read the one consumer that feeds the metrics and the health message and SHALL NOT recompute any value.

#### Scenario: The route and the metrics read one consumer

- **WHEN** one report consumer feeds both the route and the metrics
- **THEN** the route's rows and the published series agree for every resource
- **AND** the test that verifies this is `TestStorageReportHTTP_CannotDivergeFromTheMetricsSurface`

#### Scenario: The collector's rows reach the route

- **WHEN** a collection runs over a server holding bounded and unbounded streams
- **THEN** their rows, growth and pressure appear on the route and in the registry
- **AND** the test that verifies this is `TestStorageObservabilityCollectsPublishesAndServes`

### Requirement: The storage report's OpenAPI document

The route's OpenAPI document SHALL describe `/report`, and `RegisterAll` SHALL register no spec for the service because
its spec names no schema.

#### Scenario: OpenAPI document and RegisterAll

- **WHEN** a composition is built through `RegisterAll` with the storage observability service
- **THEN** `/openapi.json` describes `/report`, and no spec is registered for the service

### Requirement: The storage metrics are published under semengine_storage_

The storage observability service SHALL register 19 collectors on the injected metrics registry under the name
`storage-observability`, each named `semengine_storage_*`. Seven are labelled `resource`, `owner`, `kind` and `tier`:
`resource_used_bytes`, `resource_limit_bytes`, `resource_headroom_bytes`, `resource_headroom_ratio`,
`resource_growth_bytes_per_second`, `resource_time_to_threshold_seconds` and `resource_pressure` (0 normal to 3
critical).

#### Scenario: Per-resource series and absence

- **WHEN** a row is observed, with and without each optional measurement
- **THEN** its series carry the `semengine_storage_` names and labels, and an unmeasured value has no series
- **AND** the tests that verify this are `TestStorageMetrics_PublishesPerResourceSeries`,
  `TestStorageMetrics_AbsentMeasurementsPublishNoSeries`, `TestStorageMetrics_PressureSeverityEncoding` and
  `TestStorageMetrics_UnboundedResourceCarriesNoPressureSeries`

### Requirement: The other twelve storage collectors

The other twelve collectors SHALL be `resources` and `pressure_resources`; nine `account_*` families per storage tier;
and `report_collected_timestamp_seconds`.

#### Scenario: The registered collectors

- **WHEN** the service registers on a metrics registry
- **THEN** 19 collectors named `semengine_storage_*` are registered under `storage-observability`

### Requirement: The storage metrics write the canonical collectors

The service SHALL write to the collector `metric.RegisterOrGet` returns, so a second service built on the same registry
writes the series the registry serves.

#### Scenario: A second registration writes the canonical collectors

- **WHEN** a second metrics surface registers on the same registry
- **THEN** its observations appear in the series the registry serves
- **AND** the test that verifies this is `TestStorageMetrics_SecondRegistrationWritesTheCanonicalCollectors`

### Requirement: A storage value that was not measured publishes no series

A value that was not measured SHALL publish no series, never zero.

#### Scenario: Unmeasured value

- **WHEN** a row has no measurement of one value
- **THEN** no series is published for that value, and no zero

### Requirement: Storage series are deleted when their value goes away

A series SHALL be deleted when its value goes away, when a resource leaves the report (under the label tuple it was
published with) and, for the per-tier series, when the consumer drops the account row.

#### Scenario: Retraction

- **WHEN** a resource changes state or owner, or is forgotten, or the account row is dropped
- **THEN** the old series are deleted, and a later observation publishes afresh
- **AND** the tests that verify this are `TestStorageMetrics_StateTransitionRetractsTheStaleSeries`,
  `TestStorageMetrics_OwnerChangeDoesNotStrandTheOldSeries`, `TestStorageMetrics_ForgetRemovesEveryOneOfItsSeries`,
  `TestStorageAccountMetrics_TierThatDisappearsIsRetracted` and
  `TestStorageAccountMetrics_ForgetAccountRetractsEveryTierSeries`

### Requirement: The storage count and state families are emitted in full

The count and state families SHALL be emitted in full on every rebuild.

#### Scenario: Aggregates and tier series

- **WHEN** rows and an account are observed
- **THEN** the count families are emitted in full, and the account families publish the tier's limit, over-commitment
  and own capacity picture, with an unbounded or unreadable limit explicit
- **AND** the tests that verify this are `TestStorageMetrics_AggregateCountsAreFullyEmitted`,
  `TestStorageAccountMetrics_OverCommittedTier`, `TestStorageAccountMetrics_UnboundedLimitIsNotApplicableNotSatisfied`,
  `TestStorageAccountMetrics_UnknownLimitIsDistinctFromUnbounded`,
  `TestAccountTierMetrics_PublishTheCeilingsOwnCapacity` and `TestAccountTierMetrics_AnUnevaluableCeilingIsExplicit`

### Requirement: The storage freshness gauge

The freshness gauge SHALL carry the newest collection time of a published row, never move backwards, and ignore a zero
time.

#### Scenario: The freshness gauge

- **WHEN** rows arrive out of order or with a zero collection time
- **THEN** the gauge carries the newest collection time and never moves backwards
- **AND** the tests that verify this are `TestStorageMetrics_FreshnessGaugeCarriesCollectionTime`,
  `TestStorageMetrics_FreshnessGaugeNeverMovesBackwards` and `TestStorageMetrics_FreshnessGaugeIgnoresAZeroTimestamp`

### Requirement: The example storage alert rules

The example alert rules at `configs/prometheus/rules/storage-pressure.yml` SHALL read only published metric names, SHALL
NOT fire on a report row disappearing, SHALL name unbounded resources, and SHALL state that nothing is enforced.

#### Scenario: The example alert rules read only published names

- **WHEN** the example rules file is parsed
- **THEN** every `semengine_storage_*` name an alert reads is one the service publishes (the migration alert reads
  `semengine_streams_migration_override_expired`, which the component manager publishes), no alert fires on a series
  disappearing, unbounded storage is named, every alert has a summary, a description and a `for` window, and at least
  one description states that the report is report-only
- **AND** the tests that verify this are `TestStorageAlertRules_ReferenceOnlyPublishedMetrics`,
  `TestStorageAlertRules_KeyNothingOnRowDisappearance`, `TestStorageAlertRules_MakeUnboundedResourcesVisible` and
  `TestStorageAlertRules_AreAnnotatedAndReportOnly`
