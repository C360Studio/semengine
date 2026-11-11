# output-websocket Specification

## Purpose
The `websocket` output component serves graph data to browser and other websocket clients from one input per
instance: either the current state of every entity in `ENTITY_STATES` (a snapshot, a `synced` marker, then changes), or
the JSON messages of a core NATS subject. Delivery is at most once. A slow client never slows another, and a client
that falls behind gets the latest change per key. Tests live in `output/websocket` and `internal/smoke`.

## Requirements

### Requirement: One input per instance

The `websocket` component SHALL take exactly one input: a kv-watch of every `ENTITY_STATES` key with no history, or a
core NATS subject. It SHALL refuse any other input at construction, naming the port, and SHALL name a kv-watch of
`ENTITY_STATES` as the replacement for a `jetstream` input. It SHALL refuse the keys `delivery_mode`, `ack_timeout` and
`passthrough`, naming each.

#### Scenario: Removed surface refused

- **WHEN** a configuration carries `delivery_mode`, or a `jetstream` input
- **THEN** construction fails with an error naming that key or port (`TestConfigRefusals`)

### Requirement: Applied state is a snapshot, a marker, then changes

For a kv-watch input, a client SHALL receive one `put` per current entity and one `poisoned` per poisoned key, then
one `synced` frame carrying the revision of that snapshot, then changes after it. Each `put` payload SHALL be the stored
bytes of its revision.

#### Scenario: Late client

- **WHEN** an entity is written, then a client connects, then a second entity is written and the first deleted
- **THEN** the client receives `put` of the first, `synced`, then a `put` of the second and a `delete` of the first
  (`TestAppliedStateSnapshotThenChanges`)

### Requirement: Applied state converges to the current state

Folding a client's frames (`put` sets the key, `delete` and `poisoned` remove it) SHALL give each key's current state
once the client holds that key's latest revision. A key's frames SHALL carry strictly increasing revisions above its
snapshot revision, and its latest write SHALL never be omitted. Intermediate revisions MAY be omitted: within one
coalescing window only the last write reaches a client, and a client that falls behind gets the latest change per key.

#### Scenario: Repeated writes to one key

- **WHEN** a client is connected and, back to back, one key is put twice, a second key is put then deleted, and a third
  key is put, poisoned and put again
- **THEN** each key's revisions in the frames increase, the last frame of each key is at its last write's revision,
  and the client's folded state equals the bucket read back after the writes: the first key at its second value, the
  second absent, the third at its healed value (`TestAppliedStateConverges`)

### Requirement: No stream before the view is caught up

Until the instance's view has caught up, and while its watch is lost, an upgrade SHALL be refused with 503 and
`Health` SHALL be degraded naming why. A lost watch SHALL close every client with code 1013.

#### Scenario: Not caught up

- **WHEN** `ENTITY_STATES` does not exist
- **THEN** an upgrade gets 503 and `Health` is degraded; once the bucket exists and the view catches up, an upgrade
  succeeds (`TestNotReadyRefusesUpgrade`)

### Requirement: A subject is delivered at most once, unchanged

For a subject input, each message that is valid JSON SHALL be sent to every connected client as a `data` frame whose
`payload` is the message's bytes unchanged. A message that is not JSON SHALL NOT be sent; the drop SHALL be counted and
logged. Nothing SHALL be acknowledged or redelivered.

#### Scenario: Fan-out

- **WHEN** two clients are connected and a JSON message and a non-JSON message are published
- **THEN** both receive one `data` frame with the JSON bytes unchanged, `messages_dropped_total{reason="not_json"}`
  rises by one, and one warning names the reason, the subject and that later messages still flow (`TestSubjectFanOut`)

### Requirement: A slow client never slows another

A client that stops reading SHALL NOT delay frames to other clients. On a subject input its queue SHALL discard its
oldest frame when full; on applied state its pending changes SHALL merge per key. Each SHALL be counted and logged:
the first time per reason at once, after that at most once a minute per reason with the count since the last line.

#### Scenario: Stalled client

- **WHEN** one client stops reading while frames continue
- **THEN** another client receives the last frame, and the shed or merge is counted and logged
  (`TestStalledClientDoesNotSlowOthers`)

### Requirement: Keep-alive

The component SHALL ping each client every `ping_interval`, and a pong SHALL extend the read deadline to twice that. A
client that answers pings and sends nothing SHALL stay connected, and a client that answers none SHALL be closed.

#### Scenario: Quiet client

- **WHEN** a client answers pings and sends nothing for several read deadlines
- **THEN** it still receives the next frame (`TestQuietClientStaysConnected`)

### Requirement: Start bounds start; Stop is the only stop

A failed `Start` SHALL release the listener. Ending `Start`'s context after it returned SHALL stop nothing. `Stop` SHALL
close every client with code 1001, and SHALL release the port and leave no client counted once it returns nil.

#### Scenario: Stop

- **WHEN** a running instance with connected clients is stopped
- **THEN** each client sees close 1001, a dial of the port is refused, and `clients_connected` is 0 (lifecycle suite,
  `TestStartContextEndingStopsNothing`)
