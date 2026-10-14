# graph-ingest-recovery Specification

## Purpose
This capability states what graph-ingest promises about input from a JetStream stream when a process or the broker
stops. An acknowledgement means the transport accepted a message, not that the entity was stored: graph-ingest
acknowledges an input only after the entity and its applied-sequence record (the last stream sequence applied, kept
per stream generation so that a stream deleted and created again does not inherit it) are committed. On a
file-backed stream, an input not acknowledged is delivered again after a restart, and applying it twice leaves the
same state. A message that fails validation, or whose payload code panics, is terminated as poison (ended for good
and counted) rather than redelivered forever; an application that is interrupted, by a panic for instance, is left
unsettled so that it is delivered again.

## Requirements

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
acknowledgement wait. The signal starts when the application starts: an input that waits in its lane for half the
acknowledgement wait or longer before its application starts MAY be delivered again, and replay protection (below)
keeps that copy from applying twice. An input the callback accepts (it decodes and passes the callback's checks) and
submits to the ingest pool after a failed `Start`'s rollback has returned SHALL be negatively acknowledged, never
acknowledged or terminated. An input the callback settles before that submit keeps its own settlement.

#### Scenario: Durable record fails

- **WHEN** the entity write succeeds and the write of its applied-sequence record fails
- **THEN** the input is not acknowledged and is delivered again

#### Scenario: Long apply

- **WHEN** an input's application starts before half the consumer's acknowledgement wait has passed since delivery,
  and takes longer than the consumer's acknowledgement wait
- **THEN** the input is not delivered again while it is still being applied

#### Scenario: A delivery after a failed consumer setup

- **WHEN** a consumer's setup runs its callback on an input that decodes and is accepted, the setup then fails, and
  the callback reaches its submit to the ingest pool after the failed `Start`'s rollback has returned
- **THEN** the input is negatively acknowledged, not acknowledged or terminated, and the callback returns without
  panicking

### Requirement: A failing payload is poison, not a redelivery loop

On the Graphable lane, a message whose envelope fails validation, or whose payload code panics while it is decoded,
validated or asked for its entity, SHALL be counted, logged with its payload type and subject, and terminated; it
SHALL NOT be returned for redelivery and SHALL NOT stop the consumer.

#### Scenario: Payload panics while giving its entity ID

- **WHEN** a registered payload type's entity method panics on a delivered message
- **THEN** the message is terminated, the poison count rises by one, the panic is reported as a classified error, and
  the next message on the stream is applied

### Requirement: An interrupted application is redelivered, not a stop

When applying an input panics, its in-progress signal cannot be sent, or the settlement helper finds the delivery's
metadata or decision invalid, graph-ingest SHALL leave the input unsettled, so that it is delivered again after the
acknowledgement wait; SHALL log it, and count it once in an interrupted-work count labelled by its cause (`panic`,
`heartbeat_failed` or `invalid`); and SHALL keep taking input on the same lane. The interruption SHALL NOT add to the
error count its health reads: a write the interruption cut counts there once, at its own site, and an interruption
anywhere else leaves that count unchanged (owner ruling, #91 comment 6124130342). A panic in a payload's own code while
the message is decoded, validated or asked for its entity is poison instead (the requirement above).

#### Scenario: Application panics

- **WHEN** applying an input panics
- **THEN** the input is neither acknowledged, negatively acknowledged nor terminated, the interrupted-work count for
  `panic` rises by one, the health error count is unchanged, and the next input on the same lane is acknowledged

#### Scenario: In-progress signal fails

- **WHEN** sending `InProgress` fails while an input is being applied
- **THEN** the application is cancelled, the input is not settled, and the interrupted-work count for
  `heartbeat_failed` rises by one

### Requirement: Replay protection uses the generation read before the bind

The applied-sequence record SHALL be keyed by stream generation, learned from the server and never from
configuration. `Start` SHALL read each input stream's generation before it binds the stream's consumer, and every
input that consumer delivers SHALL carry that generation. Within one generation, an input whose sequence is not newer
than the last applied SHALL be dropped as a redelivery and acknowledged without changing state. An input that carries
a generation other than the record's SHALL NOT be dropped because of the record's sequence. An input with no stored
record SHALL be applied as first seen; when graph-ingest has no applied-sequence bucket at all, it SHALL log and count
that it is treating the input as first seen. A stored record graph-ingest cannot decode SHALL be refused: the input
SHALL NOT be applied or acknowledged, the refusal SHALL be counted and logged once per key with the key, and the input
SHALL be delivered again.

The cases below are declared limitations. In each, an input from a stream that was deleted and created again, or
restored, can carry the generation an older record holds. Such an input, at a sequence that record covers, is
acknowledged without being applied and counted in `semengine_graph_ingest_redeliveries_dropped_total`, as a
redelivery is:

- The input stream is deleted and created again after `Start` reads its generation and before the stream's consumer
  is created (owner ruling, #214 comment 6157056960). A broker restart at that moment, after which a producer creates
  a memory input stream again, may be such a case; that is not measured.
- The input stream is restored from a backup (owner ruling, #91 comment 6112550268).
- The input stream and graph-ingest's durable consumer are both created again while graph-ingest is disconnected from
  the server (owner ruling, #91 comment 6113959050).

Except after a restore, a later `Start` reads the recreated stream's generation, and an input its producer
publishes again after that `Start` is applied. After a restore, recovery is not promised.

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
