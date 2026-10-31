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

- **WHEN** a message accepted on a memory-backed input stream has not been applied, the broker restarts, and the
  producer creates the stream again while graph-ingest keeps running
- **THEN** the entity state committed before the restart is still readable and the lost message is not reported as
  applied
- **AND** while no consumer is created again under graph-ingest's durable name, graph-ingest's `Health()` reads
  unhealthy until its process is restarted; once it is, publishing the message again from the source applies it

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
acknowledgement wait. The signal starts when the application starts. An input that waits in its lane for half the
acknowledgement wait or longer before its application starts MAY be delivered again; replay protection (below) keeps
that copy from applying twice.

#### Scenario: Durable record fails

- **WHEN** the entity write succeeds and the write of its applied-sequence record fails
- **THEN** the input is not acknowledged and is delivered again

#### Scenario: Long apply

- **WHEN** an input's application starts before half the consumer's acknowledgement wait has passed since delivery,
  and takes longer than the consumer's acknowledgement wait
- **THEN** the input is not delivered again while it is still being applied

#### Scenario: A long wait before the application starts

- **WHEN** an input waits in its lane for half the acknowledgement wait or longer before its application starts, and is
  delivered again
- **THEN** the second copy is dropped as a redelivery and the entity state is applied once

### Requirement: An input submitted after a failed Start is negatively acknowledged

An input the callback accepts (it decodes and passes the callback's checks) and submits to the ingest pool after a
failed `Start`'s rollback has returned SHALL be negatively acknowledged, never acknowledged or terminated. An input the
callback settles before that submit keeps its own settlement.

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
`heartbeat_failed` or `invalid`); and SHALL keep taking input on the same lane.

#### Scenario: Application panics

- **WHEN** applying an input panics
- **THEN** the input is neither acknowledged, negatively acknowledged nor terminated, the interrupted-work count for
  `panic` rises by one, the `ErrorCount` `Health()` reports is unchanged, and the next input on the same lane is
  acknowledged

#### Scenario: In-progress signal fails

- **WHEN** sending `InProgress` fails while an input is being applied
- **THEN** the application is cancelled, the input is not settled, and the interrupted-work count for
  `heartbeat_failed` rises by one

### Requirement: A panic in payload code is poison, not an interruption

A panic in a payload's own code while the message is decoded, validated or asked for its entity SHALL be poison instead
(the requirement "A failing payload is poison, not a redelivery loop").

#### Scenario: A payload panics while it is decoded

- **WHEN** a payload's own code panics while its message is decoded
- **THEN** the message is terminated as poison and the interrupted-work count does not rise

### Requirement: An interruption does not add to the health error count

An interruption (panic, failed heartbeat, invalid decision) SHALL NOT add to the error count `Health()` reports: a
write the interruption cut counts there once, at its own site, and an interruption anywhere else leaves that count
unchanged (owner ruling, #91 comment 6124130342).

#### Scenario: A write cut by an interruption

- **WHEN** an interruption cuts a write
- **THEN** the `ErrorCount` `Health()` reports rises once, at the write's own site, and not again for the
  interruption

### Requirement: Replay protection uses the generation read before the bind

The applied-sequence record SHALL be keyed by stream generation, learned from the server and never from
configuration. `Start` SHALL read each input stream's generation before it binds the stream's consumer, and every
input that consumer delivers SHALL carry that generation.

#### Scenario: The generation is read before the bind

- **WHEN** `Start` binds an input stream's consumer
- **THEN** it has already read the stream's generation from the server, and every input the consumer delivers carries
  that generation

### Requirement: A redelivery within one generation is dropped

Within one generation, an input whose sequence is not newer than the last applied SHALL be dropped as a redelivery and
acknowledged without changing state. An input that carries a generation other than the record's SHALL NOT be dropped
because of the record's sequence.

#### Scenario: Redelivery within one generation

- **WHEN** an input already applied in the current generation is delivered again
- **THEN** it is acknowledged and the entity state does not change

#### Scenario: A different generation is not dropped by sequence

- **WHEN** an input carries a generation other than the stored record's, at a sequence the record covers
- **THEN** it is not dropped because of the record's sequence

### Requirement: A recreated or restored stream can be mistaken for a redelivery

Replay protection SHALL NOT be relied on to tell stream generations apart. An input from a stream that was deleted and
created again, or restored, can carry the generation an older record holds; such an input, at a sequence that record
covers, is acknowledged without being applied and counted in `semengine_graph_ingest_redeliveries_dropped_total`, as a
redelivery is. These cases are declared limitations, not promises.

#### Scenario: Recreated between the generation read and the consumer's creation

- **WHEN** the input stream is deleted and created again after `Start` reads its generation and before the stream's
  consumer is created (owner ruling, #214 comment 6157056960)
- **THEN** an input at a sequence the older record covers is acknowledged without being applied and counted as a
  dropped redelivery
- **AND** a broker restart at that moment, after which a producer creates a memory input stream again, may be such a
  case; that is not measured

#### Scenario: Restored from a backup

- **WHEN** the input stream is restored from a backup (owner ruling, #91 comment 6112550268)
- **THEN** an input at a sequence the older record covers is acknowledged without being applied and counted as a
  dropped redelivery

#### Scenario: Stream and consumer created again while disconnected

- **WHEN** the input stream and graph-ingest's durable consumer are both created again while graph-ingest is
  disconnected from the server (owner ruling, #91 comment 6113959050)
- **THEN** an input at a sequence the older record covers is acknowledged without being applied and counted as a
  dropped redelivery

### Requirement: An input with no stored record is applied as first seen

An input with no stored record SHALL be applied as first seen; when graph-ingest has no applied-sequence bucket at all,
it SHALL log and count that it is treating the input as first seen.

#### Scenario: No applied-sequence bucket

- **WHEN** graph-ingest has no applied-sequence bucket and an input is delivered
- **THEN** the input is applied as first seen, and graph-ingest logs and counts that it is treating the input as first
  seen

### Requirement: A stored record that cannot be decoded is refused

A stored record graph-ingest cannot decode SHALL be refused: the input SHALL NOT be applied or acknowledged, the
refusal SHALL be counted and logged once per key with the key, and the input SHALL be delivered again.

#### Scenario: A record that cannot be decoded

- **WHEN** the stored applied-sequence record for an input's entity and stream is three bytes long
- **THEN** the input is neither applied nor acknowledged and the refusal count rises by one, and once the record is
  deleted the redelivered input is applied

### Requirement: A later Start recovers a recreated stream except after a restore

Except after a restore, a later `Start` SHALL read the recreated stream's generation, and an input its producer
publishes again after that `Start` SHALL be applied. After a restore, recovery is not promised.

#### Scenario: Stream recreated with lower sequences

- **WHEN** the input stream is recreated and re-ingestion publishes the current source at sequences lower than the
  recorded ones
- **THEN** the current relationships and content are applied and the entity queries return them

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

### Requirement: graph-ingest's data flow counts input messages

graph-ingest's `DataFlow()` SHALL count each input message once: each delivery on a stream input, redeliveries
included, and each mutation request that reaches a handler. `BytesPerSecond` SHALL count those messages' payload
bytes. Query requests, in-process creates and the containers hierarchy inference creates SHALL NOT count. `ErrorRate`
SHALL be the processing errors counted since start divided by the input messages counted.

#### Scenario: A stream message that creates containers

- **WHEN** one stream message is applied and hierarchy inference creates two containers for it
- **THEN** `DataFlow()` counts one message

#### Scenario: An append to three subjects

- **WHEN** one append request writes three entities
- **THEN** `DataFlow()` counts one message

### Requirement: The entity cache stays usable through Stop

A graph-ingest query, `CreateEntity` call or write that reads or updates the entity query cache while `Stop` runs, or
after it, SHALL neither panic nor race with `Stop` clearing the cache: it uses the cache it found, which stays usable
after it is closed.

#### Scenario: Cache use during Stop

- **WHEN** cache reads, invalidations and repopulations run on goroutines while `Stop` runs, under the race detector
- **THEN** none panics and the race detector reports nothing
- **AND** the test that verifies this is `TestEntityCacheUseDuringStop`

#### Scenario: A cache after Close

- **WHEN** `Get`, `Set` and `Delete` are called on each `internal/cache` implementation after `Close`
- **THEN** each works as before `Close`
- **AND** the test that verifies this is `TestCacheUsableAfterClose`

### Requirement: A lost input binding is stuck until the process restarts

graph-ingest SHALL record a lost input binding when a status tick, run while the component is running and its
`Stop` has not begun, observes that a consume loop it started has ended. Once
recorded, `Health()` SHALL report `Healthy` false with a `LastError` that names the stream and consumer and says the
process must be restarted, and the readiness envelope SHALL be degraded, on every later read until the component
stops.

#### Scenario: Stream deleted and created again while the broker stays up

- **WHEN** graph-ingest's input stream is deleted and created again while the broker stays up
- **THEN** after the next status tick `Healthy` is false, `LastError` names the consumer and the restart, and the
  readiness envelope is degraded
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
recovery, and it reads the generation again. Once `Stop` has begun, no tick SHALL record or log a lost input binding.

#### Scenario: Stop ends the loop

- **WHEN** graph-ingest's `Stop` has begun and drains its consumers, and a status tick runs after a drain closed a
  loop
- **THEN** that tick records and logs no lost input binding

### Requirement: A declared consumer name is refused

graph-ingest names the durable consumer of each JetStream input from the input's subject. Its configuration
validation SHALL refuse a JetStream input that declares `consumer_name`, as invalid configuration whose error names
the port and `consumer_name`, so that construction and port declaration both fail rather than drop the value.

#### Scenario: A JetStream input declaring consumer_name

- **WHEN** graph-ingest's configuration has a JetStream input that declares `consumer_name`
- **THEN** `DeclarePorts`, `CreateGraphIngest` and `Config.Validate` each fail, classified invalid, naming the port and
  `consumer_name`
- **AND** the test that verifies this is `TestConfigRefusesDeclaredConsumerName`

### Requirement: The boot sweep waits at most 5 s for its watch to close

After stopping its `ENTITY_STATES` snapshot watch, graph-ingest's boot sweep SHALL wait until the watch's delivery
closes for at most 5 s. When the 5 s run out, the sweep SHALL fail, classified transient, with an error saying the
delivery did not close after `Stop`, so `Start` fails (#456).

#### Scenario: The snapshot watch's delivery never closes

- **WHEN** the sweep stops its watch and the update channel stays open, as at nats.go v1.55.0 when the watch's
  context ends, or its connection closes, after nats.go subscribes and before it installs the handler that closes the
  channel (no goroutine is then left to join; on every other path the wait ends when nats.go's delivery ends)
- **THEN** the sweep returns, after 5 s of its clock, a transient error saying the delivery did not close after
  `Stop`, and records no poison
- **AND** the test that verifies this is `TestEntityStateGuardGivesUpOnADeliveryThatNeverCloses`

### Requirement: Stop returns only after every input callback has returned

graph-ingest's `Stop` SHALL return nil only after every JetStream input's delivery has ended and every consume
callback has returned, through `natsclient`'s `WaitConsumerHandlers`, never on the consume loop's `Closed()` alone. A
`Stop` whose context ends first SHALL return that context's error and keep its inputs for a later `Stop` (#441).

#### Scenario: A callback held after the connection closed for good

- **WHEN** a consume callback is held, the client's connection closes for good and the consume loop's `Closed()` has
  fired, and `Stop` runs under a context that ends before the callback is released
- **THEN** `Stop` returns that context's error and keeps its inputs, and a `Stop` after the callback returns
  returns nil and holds nothing
- **AND** the test that verifies this is `TestStopJoinsCallbackAfterConnectionLoss`
