# operator-surface

## MODIFIED Requirements

### Requirement: The message logger's routes are served under /message-logger

The message logger SHALL serve, under `/message-logger/`, `entries`, `stats`, `subjects`, `trace/{traceID}`,
`kv/{bucket}` and `kv/{bucket}/watch`. Each route SHALL answer `GET` and `HEAD` and SHALL answer any other method with 405.
`entries` SHALL return the newest stored entries, newest first; its `limit` SHALL default to 100 and be clamped to
10000, and a missing, non-numeric, zero or negative value SHALL give 100. Its `subject` filter SHALL be a NATS subject
pattern matched with `flowgraph.SubjectCovers` and applied before the limit, and a pattern NATS cannot parse SHALL match
nothing. `stats` SHALL report `total_messages`, `dropped_messages` (messages not stored because their handler context
had ended; not counted in `total_messages`), `sampled_messages`, `valid_messages` (messages that decoded as typed
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

#### Scenario: Dropped messages are counted

- **WHEN** the logger's callback is handed a message whose context has ended
- **THEN** `stats` reports `dropped_messages` 1 and `total_messages` 0
- **AND** the test that verifies this is `TestMessageLoggerCountsAndLogsADroppedMessage`

#### Scenario: A malformed trace ID, or a method other than GET or HEAD

- **WHEN** `trace/` is asked for an ID that is not 32 hex characters, or any route is asked with `POST`, or
  `stats` with `HEAD`
- **THEN** the answers are 400, 405 and 200
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
