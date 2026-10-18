# graph-ingest-recovery

## MODIFIED Requirements

### Requirement: An interrupted application is redelivered, not a stop

When applying an input panics, its in-progress signal cannot be sent, or the settlement helper finds the delivery's
metadata or decision invalid, graph-ingest SHALL leave the input unsettled, so that it is delivered again after the
acknowledgement wait; SHALL log it, and count it once in an interrupted-work count labelled by its cause (`panic`,
`heartbeat_failed` or `invalid`); and SHALL keep taking input on the same lane. The interruption SHALL NOT add to the
error count `Health()` reports: a write the interruption cut counts there once, at its own site, and an interruption
anywhere else leaves that count unchanged (owner ruling, #91 comment 6124130342). A panic in a payload's own code while
the message is decoded, validated or asked for its entity is poison instead (the requirement above).

#### Scenario: Application panics

- **WHEN** applying an input panics
- **THEN** the input is neither acknowledged, negatively acknowledged nor terminated, the interrupted-work count for
  `panic` rises by one, the `ErrorCount` `Health()` reports is unchanged, and the next input on the same lane is
  acknowledged

#### Scenario: In-progress signal fails

- **WHEN** sending `InProgress` fails while an input is being applied
- **THEN** the application is cancelled, the input is not settled, and the interrupted-work count for
  `heartbeat_failed` rises by one

## ADDED Requirements

### Requirement: graph-ingest's health reads its stuck states

graph-ingest's `Health()` SHALL report `Healthy` false while it holds a poisoned entity record or while its latest
status tick could not read the backlog of a bound consumer, and true again once neither holds, without a restart. A
count of past errors SHALL NOT lower `Healthy`; it is reported as `ErrorCount`.

#### Scenario: The durable consumer is lost

- **WHEN** graph-ingest's durable consumer is deleted while graph-ingest runs
- **THEN** after the next status tick `Healthy` is false and `LastError` names the backlog observation

#### Scenario: The observation succeeds again

- **WHEN** a status tick's backlog observation fails and a later tick's succeeds
- **THEN** `Healthy` is false after the first and true after the second; this says nothing about whether graph-ingest
  is consuming (a deleted consumer is not rebound, #216)

#### Scenario: A processing error

- **WHEN** graph-ingest terminates an undecodable message
- **THEN** `ErrorCount` rises and `Healthy` stays true
