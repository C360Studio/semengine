# operator-surface

## ADDED Requirements

### Requirement: The message logger's routes are served under /message-logger

The message logger SHALL serve, under `/message-logger/`, `entries`, `stats`, `subjects`, `trace/{traceID}`,
`kv/{bucket}` and `kv/{bucket}/watch`. Each route SHALL answer only `GET` and SHALL answer any other method with 405.
`entries` SHALL return the newest stored entries, newest first; its `limit` SHALL default to 100 and be clamped to
10000, and a missing, non-numeric, zero or negative value SHALL give 100. Its `subject` filter SHALL be a NATS subject
pattern matched with `flowgraph.SubjectCovers` and applied before the limit, and a pattern NATS cannot parse SHALL match
nothing. `stats` SHALL report `total_messages`, `sampled_messages`, `valid_messages` (messages that decoded as typed
envelopes), `invalid_messages` (the rest), `sample_rate`, `max_entries`, `start_time`, `last_message_time`,
`uptime_seconds`, `monitored_subjects`, `subject_overlaps`, `subject_reconciliation_degraded` and
`subject_reconciliation_error`. `trace/{traceID}` SHALL answer 400 unless the ID is 32 hex characters, and otherwise
the entries still held for the trace, oldest first. `entries`, `stats` and `trace` SHALL answer from memory during and
after `Stop`, and `subjects` SHALL answer `[]` after `Stop`. A payload that is not JSON SHALL be stored and served as a
JSON string, so every entry can be served. `RegisterAll` SHALL register the message logger's OpenAPI spec, so that every
`$ref` in `/openapi.json` names a schema it contains; importing the package SHALL register nothing.

#### Scenario: The entries limit

- **WHEN** `entries` is asked with no limit, a limit above 10000, and a zero, negative or non-numeric limit
- **THEN** the limits applied are 100, 10000 and 100
- **AND** the test that verifies this is `TestEntriesLimit`

#### Scenario: The subject filter applies before the limit

- **WHEN** `entries` is asked with a `subject` filter and a `limit` smaller than the number of entries that match
- **THEN** the answer holds that many matching entries, not the matches among the newest `limit` entries
- **AND** the tests that verify this are `TestEntriesFilterBeforeLimit` and `TestEntriesSubjectFilterUsesNATSMatching`

#### Scenario: A message that is not JSON

- **WHEN** a payload that is not JSON has been stored
- **THEN** `entries` and `trace` answer every entry, the payload as a JSON string
- **AND** the test that verifies this is `TestEntriesServeNonJSONPayload`

#### Scenario: Valid and invalid counts

- **WHEN** a typed envelope and a raw payload are stored
- **THEN** `valid_messages` counts the first and `invalid_messages` the second
- **AND** the test that verifies this is `TestStatsCountsOnlyTypedEnvelopesValid`

#### Scenario: A malformed trace ID, or a method other than GET

- **WHEN** `trace/` is asked for an ID that is not 32 hex characters, or any route is asked with `POST`
- **THEN** the answers are 400 and 405
- **AND** the tests that verify this are `TestTraceRouteRefusesMalformedID` and
  `TestMessageLoggerRoutesRefuseOtherMethods`

#### Scenario: Reads during and after Stop

- **WHEN** `entries`, `stats`, `trace` and `subjects` are asked after `Stop`
- **THEN** the first three answer from memory and `subjects` answers `[]`
- **AND** the test that verifies this is `TestMessageLoggerRoutesAfterStop`

#### Scenario: Every schema reference resolves

- **WHEN** a composition built through `RegisterAll` enables `message-logger` and is started
- **THEN** every `$ref` in `/openapi.json` names a schema in that document, `MessageLogEntry` among them
- **AND** the test that verifies this is `TestOpenAPIDocumentResolvesEverySchemaRef`

### Requirement: The KV query reads an existing bucket and never creates one

`kv/{bucket}` SHALL answer 400 for a bucket name that is empty, `.`, `..` or holds `/`, `\`, `*` or `>`, a check shared
with the watch route. It SHALL look the bucket up without creating it and answer 404 when it does not exist, 500 for
another lookup, listing or read failure, and an empty result for a bucket with no keys. It SHALL sort keys before
`limit` applies (default 100, clamped to 1000), and `pattern` SHALL be a key glob: `*` or empty for every key, one `*`
as prefix, suffix, both ends or in the middle (the prefix and suffix not sharing a character), or none for an exact key.
It SHALL answer each value as stored, so large numbers are not rounded, and a value that is not JSON as a JSON string.
`status` SHALL keep only records whose top-level JSON `status` string equals it. A key deleted or purged between the
listing and its read SHALL be left out; any other read error SHALL answer 500 naming the key.

#### Scenario: A missing bucket

- **WHEN** a bucket that does not exist is asked for
- **THEN** the answer is 404 and no bucket was created; a lookup failure other than not-found answers 500
- **AND** the tests that verify this are `TestKVQueryMissingBucketIs404AndCreatesNothing` and
  `TestKVQueryLookupFailureIs500`

#### Scenario: Reads of existing buckets

- **WHEN** an existing bucket is queried, empty or holding entries, with a key glob, a status filter and a limit
- **THEN** the answers hold the matching entries, the stored values unrounded, keys sorted before the limit
- **AND** the tests that verify this are `TestKVQueryReadsExistingBucketsWithoutCreating`,
  `TestKVQueryEmptyBucketReturnsNoEntries`, `TestKVQueryFailedRecordRoundTrips`, `TestKVQuerySortsKeysBeforeLimit`,
  `TestKVQueryPreservesJSONValues`, `TestKVQueryPatternMatchesBeforeLimit`, `TestRecordMatchesStatus` and
  `TestMatchesPatternMiddleWildcard`

#### Scenario: A malformed bucket name

- **WHEN** either KV route is asked for a bucket name that is empty, a path step or holds a separator or wildcard
- **THEN** the answer is 400
- **AND** the tests that verify this are `TestKVRoutesRejectMalformedBucket` and `TestIsValidBucketName`

Not covered by a test: the two read branches that need the bucket to change, or the server to fail, between the key
listing and the read (a key left out because it was deleted; any other error answering 500). Sorting the keys is also
not detected by `TestKVQuerySortsKeysBeforeLimit`: the pinned `nats.go` already returns them sorted, so removing the
sort does not fail the test, and the sort is kept because that order is not promised.

### Requirement: The KV watch ends on client exit, falling behind or Stop

`kv/{bucket}/watch` SHALL answer 503 once `Stop` has begun, 400 for a bad bucket name or `pattern`, 404 for a bucket
that does not exist and 500 for another lookup failure, and none of these SHALL open a stream or create a bucket. An
empty or `*` `pattern` SHALL watch every key; any other SHALL be a NATS KV key filter, a final `>` accepted and a space
refused. Otherwise it SHALL stream Server-Sent Events: `connected` with the bucket, pattern and a message, `retry:
5000`, one `kv_change` for each key's current value, one `kv_change` with operation `initial_sync_complete`, then one
`kv_change` for each later change. A `kv_change` event's `operation` SHALL be what NATS records, `put`, `delete` or
`purge`, and its `value` SHALL be present for a `put` only. The logger SHALL NOT read `Last-Event-ID`. Each stream SHALL
buffer at most 100 events between its KV watcher and the client; the first event that does not fit SHALL stop the
watcher, and the stream SHALL send the events already buffered, then one `error` event with
`{"error":"watch fell behind: events were dropped"}`, and end, so no `kv_change`, and no `initial_sync_complete`,
follows a dropped event. The stream SHALL end when the client leaves or the logger stops, and the KV watcher and its
reader SHALL have ended before the handler returns.

#### Scenario: Initial values, then changes

- **WHEN** a watch is opened on a bucket that holds keys and a key is then written and deleted
- **THEN** the stream carries `connected`, the current values, `initial_sync_complete`, then the `put` and `delete`
- **AND** the test that verifies this is `TestKVWatchStreamsInitialValuesThenChanges`

#### Scenario: Refusals open nothing

- **WHEN** the watch is asked for an invalid filter, a missing bucket, or after `Stop` began
- **THEN** the answers are 400, 404 and 503
- **AND** the tests that verify this are `TestKVWatchRejectsInvalidFilter`, `TestKVWatchMissingBucketIs404` and
  `TestKVWatchAfterStopIs503`

#### Scenario: A client that falls behind

- **WHEN** a client that does not read opens a watch on a bucket holding more keys than the stream buffers
- **THEN** it gets the buffered events, then one `error` event, then the end of the stream, no
  `initial_sync_complete` anywhere, and fewer `kv_change` events than keys
- **AND** the test that verifies this is `TestKVWatchFallingBehindEndsStream`

Review only: when a client falls behind, `nats.go` waits for room when it puts an entry on the watcher's 256-entry
updates channel. If more than 256 entries arrive between the reader's last receive and its stop, a `nats.go` goroutine
could stay blocked until the stop. This was not probed and no real-world trigger is named, so the behaviour is not
promised.
