# graph-ingest-recovery

## MODIFIED Requirements

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

## ADDED Requirements

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
- The input stream and a consumer with graph-ingest's consumer name are created again while graph-ingest runs, and
  graph-ingest receives the new stream's input through a request its consumer made before the stream was deleted
  (a doc line under the owner's rule of 2026-10-12, #185; not measured).

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

## REMOVED Requirements

### Requirement: Replay protection is generation-aware

**Reason**: Its scenario "Stream recreated while graph-ingest binds its consumer" and its sentences "An input SHALL
carry the generation of the stream its consumer is bound to" and "`Start` SHALL fail" for that window are no longer
true: the owner retired the check that held them (#214 comment 6157056960), and the case is a declared limitation.
OpenSpec's archive refuses a modified requirement that drops a scenario, so the requirement is added again above as
"Replay protection uses the generation read before the bind". It keeps the other three scenarios, states what
generation an input carries, and lists the declared limitations.

**Migration**: None. No caller relies on `Start` failing for a recreated stream: no test other than the ones this
change deletes, and no consumer, which imports SemStreams at the pin, where the check does not exist.
