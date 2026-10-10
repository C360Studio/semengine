# graph-ingest-recovery

## ADDED Requirements

### Requirement: Acknowledged is not durable

A transport acknowledgement SHALL mean that the transport accepted a message; durability SHALL mean that graph-ingest
committed the entity state and its applied-sequence record to their key-value buckets. graph-ingest SHALL NOT report
an input as applied that it did not commit.

#### Scenario: Memory transport lost at broker restart

- **WHEN** a message accepted on a memory-backed input stream has not been applied, and the broker restarts
- **THEN** the entity state committed before the restart is still readable, the lost message is not reported as
  applied, and publishing it again from the source applies it

### Requirement: Recovery on a file stream is redelivery

On a file-backed input stream, an input that graph-ingest did not acknowledge SHALL be delivered again after a process
or broker restart, and applying it again SHALL leave the entity state equal to the state after one application.

#### Scenario: Process killed between apply and acknowledgement

- **WHEN** the process running graph-ingest is killed after applying an input and before acknowledging it, and a new
  process starts graph-ingest on the same stream and buckets
- **THEN** the input is delivered again, the entity state equals the state after one application, and no input is lost

### Requirement: Settlement order

graph-ingest SHALL acknowledge an input only after the input's effect and its applied-sequence record are committed,
and SHALL tell the transport that an input is still in progress while its application outlasts the consumer's
acknowledgement wait.

#### Scenario: Durable record fails

- **WHEN** the entity write succeeds and the write of its applied-sequence record fails
- **THEN** the input is not acknowledged and is delivered again

#### Scenario: Long apply

- **WHEN** applying one input takes longer than the consumer's acknowledgement wait
- **THEN** the input is not delivered again while it is still being applied

### Requirement: Replay protection is generation-aware

The applied-sequence record SHALL be keyed by stream generation, learned from the server and never from
configuration. Within one generation, an input whose sequence is not newer than the last applied SHALL be dropped
as a redelivery and acknowledged without changing state. An input from a newer generation SHALL never be dropped
because of a sequence recorded in an older one. An input with no stored record SHALL be applied as first seen; when
graph-ingest has no applied-sequence bucket at all, it SHALL log and count that it is treating the input as first
seen. A stored record graph-ingest cannot decode SHALL be refused: the input SHALL NOT be applied or acknowledged, the
refusal SHALL be counted and logged once per key with the key, and the input SHALL be delivered again.

#### Scenario: Stream recreated with lower sequences

- **WHEN** the input stream is recreated and re-ingestion publishes the current source at sequences lower than the
  recorded ones
- **THEN** the current relationships and content are applied and the entity queries return them

#### Scenario: Redelivery within one generation

- **WHEN** an input already applied in the current generation is delivered again
- **THEN** it is acknowledged and the entity state does not change

#### Scenario: A record that cannot be decoded

- **WHEN** the stored applied-sequence record for an input's entity and stream is three bytes long
- **THEN** the input is neither applied nor acknowledged and the refusal count rises by one, and once the record is
  deleted the redelivered input is applied

### Requirement: A failing payload is poison, not a redelivery loop

On the Graphable lane, a message whose envelope fails validation, or whose payload code panics while it is decoded,
validated or asked for its entity, SHALL be counted, logged with its payload type and subject, and terminated; it
SHALL NOT be returned for redelivery and SHALL NOT stop the consumer.

#### Scenario: Payload panics while giving its entity ID

- **WHEN** a registered payload type's entity method panics on a delivered message
- **THEN** the message is terminated, the poison count rises by one, the panic is reported as a classified error, and
  the next message on the stream is applied
