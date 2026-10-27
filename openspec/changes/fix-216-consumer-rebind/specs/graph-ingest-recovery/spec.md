# graph-ingest-recovery

## MODIFIED Requirements

### Requirement: Acknowledged is not durable

A transport acknowledgement SHALL mean that the transport accepted a message; durability SHALL mean that graph-ingest
committed the entity state and its applied-sequence record to their key-value buckets. graph-ingest SHALL NOT report
an input as applied that it did not commit.

#### Scenario: Memory transport lost at broker restart

- **WHEN** a message accepted on a memory-backed input stream has not been applied, the broker restarts, and the
  producer creates the stream again while graph-ingest keeps running
- **THEN** the entity state committed before the restart is still readable and the lost message is not reported as
  applied
- **AND** graph-ingest's `Health()` reads unhealthy until its process is restarted, and once it is, publishing the
  message again from the source applies it

### Requirement: graph-ingest's health reads its stuck states

graph-ingest's `Health()` SHALL report `Healthy` false while it holds a poisoned entity record, while its latest
status tick could not read the backlog of a bound consumer, or once it has recorded a lost input binding (the
requirement "A lost input binding is stuck until the process restarts"). The first two SHALL clear without a restart
once they no longer hold; a lost input binding SHALL NOT clear. A count of past errors SHALL NOT lower `Healthy`; it
is reported as `ErrorCount`.

#### Scenario: The durable consumer is lost

- **WHEN** graph-ingest's durable consumer is deleted while graph-ingest runs
- **THEN** after the next status tick `Healthy` is false and `LastError` names the backlog observation

#### Scenario: The observation succeeds again

- **WHEN** a status tick's backlog observation fails while no input binding is lost, and a later tick's succeeds
- **THEN** `Healthy` is false after the first and true after the second

#### Scenario: A processing error

- **WHEN** graph-ingest terminates an undecodable message
- **THEN** `ErrorCount` rises and `Healthy` stays true

## ADDED Requirements

### Requirement: A lost input binding is stuck until the process restarts

graph-ingest SHALL record a lost input binding when a consume loop it started has ended and its own `Stop` did not end
it, or when a status tick reads a bound input stream whose creation time is not the generation read at bind. Once
recorded, `Health()` SHALL report `Healthy` false with a `LastError` that names the stream and consumer and says the
process must be restarted, and the readiness envelope SHALL be degraded, on every later read until the component
stops.

#### Scenario: Memory stream created again after a broker restart

- **WHEN** graph-ingest runs on a memory-backed input stream, the broker restarts, and the producer creates the stream
  again
- **THEN** after the next status tick `Healthy` is false and `LastError` names the consumer and the restart
- **AND** when a consumer is then created again under graph-ingest's durable name, a later tick still reads `Healthy`
  false

#### Scenario: Stream deleted and created again while the broker stays up

- **WHEN** graph-ingest's input stream is deleted and created again while the broker stays up
- **THEN** after the next status tick `Healthy` is false and the readiness envelope is degraded
- **AND** when a consumer is then created again under graph-ingest's durable name, a later tick still reads `Healthy`
  false

#### Scenario: Consumer created again after it was deleted

- **WHEN** graph-ingest's durable consumer is deleted while graph-ingest runs, and a consumer with the same stream,
  name and configuration is then created
- **THEN** a tick after the creation still reads `Healthy` false

#### Scenario: File stream across a broker restart

- **WHEN** graph-ingest runs on a file-backed input stream and the broker restarts
- **THEN** a tick after the restart reads `Healthy` true, and a message published after the restart is applied

### Requirement: A process restart recovers a lost input binding

graph-ingest SHALL NOT rebind, stop or delete the consumer of a lost input binding: a process restart is the
recovery, and it reads the generation again. `Stop` ending a loop SHALL NOT record a lost input binding.

#### Scenario: Stop ends the loop

- **WHEN** graph-ingest's `Stop` drains its consumers
- **THEN** no lost input binding is recorded or logged
