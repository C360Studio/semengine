# websocket

The output component that streams the graph to browsers and other WebSocket clients.

## Overview

`websocket` runs an HTTP server and upgrades each request on its path (default `/ws`) to a WebSocket. It has exactly
one input, chosen in configuration, so one instance sends one kind of frame:

- **Applied state** (the default): a kv-watch of every `ENTITY_STATES` key, the bucket graph-ingest writes. A client
  sees the entities as they are now, then each change.
- **A core NATS subject**: every JSON message published on it.

To stream both, run two instances on two ports.

A *frame* is one WebSocket text message holding one JSON object. A *revision* is the number NATS gives each write to
a bucket.

## Configuration

Applied state, which is also what `{}` gives:

```json
{
  "type": "output",
  "name": "websocket",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "config": {"kind": "kv-watch", "bucket": "ENTITY_STATES"}}
      ],
      "outputs": [
        {"name": "websocket_server", "config": {"kind": "network", "protocol": "http", "host": "0.0.0.0", "port": 8081}}
      ]
    },
    "path": "/ws",
    "ping_interval": "30s"
  }
}
```

A subject instead streams one subject (a core NATS subject, wildcards as NATS defines them, no queue group):

```json
{"name": "boids_frames", "config": {"kind": "nats", "subject": "boids.frames"}}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the ports above | A `ports` block that is present is used as written; one that is absent takes the default |
| `path` | string | `/ws` | Path the clients upgrade on; must begin with `/` and be a valid HTTP ServeMux pattern |
| `ping_interval` | string | `30s` | Time between pings to each client, a Go duration; a value that does not parse or is not above zero is refused |

Exactly one input is required: a kv-watch of every `ENTITY_STATES` key with no history, or a core NATS subject with no
queue group (a queue group would split messages between instances). Any other input is refused naming the port; a
`jetstream` input is refused with the replacement. Exactly one output is required, an `http` network port from 1024
to 65535 (port 0 is refused: the address clients dial must be known before `Start`). The configuration is decoded
strictly: an unknown key is refused at construction, naming it. A missing NATS client is refused too.

## Wire format

Every frame is a JSON object with a `type`. `timestamp` is Unix milliseconds.

```json
{"type":"data","subject":"boids.frames","timestamp":1731330000123,"payload":{"...":"the message, unchanged"}}
{"type":"put","bucket":"ENTITY_STATES","key":"<entity id>","revision":42,"timestamp":1731330000100,"payload":{"...":"stored"}}
{"type":"delete","bucket":"ENTITY_STATES","key":"<entity id>","revision":43,"timestamp":1731330000200}
{"type":"poisoned","bucket":"ENTITY_STATES","key":"<entity id>","revision":44,"error":"<why the value does not decode>"}
{"type":"synced","bucket":"ENTITY_STATES","revision":44}
```

| Type | Instance | Meaning |
| --- | --- | --- |
| `data` | subject | One message. `payload` is the message's bytes unchanged; `timestamp` is when the component received it |
| `put` | applied state | The entity `key` was written. `payload` is the bytes graph-ingest stored; `timestamp` is the write time in the bucket |
| `delete` | applied state | The entity `key` was deleted or purged |
| `poisoned` | applied state | The value stored at `key` does not decode as an entity state. Drop your copy of `key`; a later valid write heals it |
| `synced` | applied state | Every frame before it is the state at `revision`; every frame after it is a change above it |

Frames a client sends are read and discarded.

### What an applied-state client sees

1. On connect: one `put` per current entity, one `poisoned` per poisoned key, then `synced`. Entities and poisoned keys
   are each in key order.
2. After `synced`: the changes, as they apply.

Keep a map by `key`: `put` sets it, `delete` and `poisoned` remove it. The map then equals the bucket's current state
once the client holds each key's latest revision. A key's frames carry increasing revisions and its latest write is
never omitted, but intermediate revisions can be: the shared view merges writes inside one 250 ms window, and a client
that is behind receives only the latest change per key. Do not count frames, and do not expect every revision.

### What a subject client sees

Each message that is valid JSON is sent to every connected client as a `data` frame. A message that is not JSON is
not sent; it is counted (`messages_dropped_total{reason="not_json"}`) and logged. Delivery is at most once, best
effort: the component acknowledges nothing, replays nothing, and a client that was not connected misses the message.

## Readiness and closing

- **Applied state before it is ready.** Until the instance's view of `ENTITY_STATES` has caught up (a bucket that does
  not exist yet counts), and while its watch is lost, an upgrade is answered `503` with the reason and `Health` reports
  it as degraded: `applied-state view catching up` or `applied-state watch lost, reopening`. A watch lost on a running
  instance closes every client with code `1013` (try again later); reconnect after a `503` clears to get a fresh
  snapshot.
- **Stop.** New upgrades get `503`, every client is sent close code `1001` (going away), and `Stop` returns once the
  server and every client handler have returned. After `Stop` returns nil the port is released.
- **Start.** `Start` binds the listener, loads TLS when enabled, and subscribes or starts the view. A failure returns
  the error and holds nothing. Ending `Start`'s context afterwards stops nothing; only `Stop` does (ADR-113).

## Slow and silent clients

A client that stops reading never slows another.

- **Subject:** each client has a queue of 256 frames; when it is full the client's oldest frame is dropped
  (`messages_dropped_total{reason="client_queue_full"}`).
- **Applied state:** the client's pending changes merge per key, so it gets the latest change per key when it
  catches up (`changes_coalesced_total`).

Each drop is counted at once. It is logged as a warning naming the reason, the listener and why continuing is safe: the
first of a reason immediately, after that at most once a minute per reason, with the count since the last line.
Merging inside the view's 250 ms window is part of the applied-state contract and is not counted.

Keep-alive: the component pings each client every `ping_interval`, and a pong extends the client's read deadline to
twice that. A client that answers pings but sends nothing stays connected; one that answers none is closed
(`client_disconnections_total{reason="read_timeout"}`). Probe P1 (PR #592): at gorilla/websocket v1.5.3, a pong
that does not extend the read deadline lets the read time out with pongs received, so the handler extends it. A write
to a client that does not complete in 10 seconds closes that client; what gorilla/websocket does when that deadline
passes is not promised.

## Origins and authentication

Every `Origin` is accepted, as in the SemStreams original, and the output has no authentication (#84). Anything that
can reach the port can read the stream. Bind it to an interface the deployment trusts, or put a proxy in front that
checks origin and credentials.

## TLS

TLS comes from the deployment's server TLS configuration (`component.Dependencies.Security`), loaded in `Start`
through `internal/tlsutil`: certificate and key files, with client certificates (mTLS) applied when that
configuration asks. An `acme` mode is refused at `Start` naming the key; ACME is not carried until a consumer needs it.
With TLS enabled the server speaks HTTP/1.1 only, which the WebSocket upgrade needs, and clients dial `wss://`.

## Metrics

Collectors register on the injected `metric.MetricsRegistry`, never on Prometheus' global registry; with no registry
they are not registered. Every metric is labelled `listener` (`host:port`), so two instances on one registry stay
apart.

| Metric (`semengine_websocket_` prefix) | Type | Meaning |
| --- | --- | --- |
| `messages_received_total` | counter | Subject messages received, or applied-state writes the view decoded |
| `messages_sent_total` | counter | Data and state frames written to a client, once per client |
| `bytes_sent_total` | counter | Bytes of those frames |
| `message_size_bytes` | histogram | Size of each such frame |
| `clients_connected` | gauge | Clients connected now |
| `client_connections_total` | counter | Requests upgraded to a WebSocket |
| `client_disconnections_total{reason}` | counter | `read_timeout`, `client_closed`, `write_failed`, `stopped`, `watch_lost` |
| `errors_total{type}` | counter | `upgrade` (a request that could not be upgraded), `serve` (the server stopped before Stop) |
| `messages_dropped_total{reason}` | counter | `not_json`, `client_queue_full` |
| `changes_coalesced_total` | counter | Applied-state changes merged into a later one for a client that was behind |

## Migration from SemStreams

The wire format is new and the SemStreams `MessageEnvelope` is gone: its `id` is dropped (it served the
acknowledgement path), and the payload is no longer rewritten with `timestamp` and `subject`. These keys are refused
at construction with a line saying what replaces them: `delivery_mode`, `ack_timeout`, `passthrough`.

- **semsource:** drop the JetStream input (the default is the kv-watch) and `delivery_mode`. `/ws` now carries current
  entity state (`put`, `delete`, `poisoned`, `synced`), not every `graph.ingest.*` message: no manifest, status or
  predicate payloads, and no promise of every revision. Keep a map by `key` and treat the latest frame as truth.
  A configuration that still has `delivery_mode` fails boot naming it.
- **semboids:** drop `delivery_mode`. Read the message from `frame.payload` (its bytes are unchanged), and the subject
  and time from `frame.subject` and `frame.timestamp`.

## Known limits

- **At most once on a subject.** A message sent while no client is connected, or dropped from a full queue, is gone.
- **Convergence, not history, on applied state.** Intermediate revisions can be merged away.
- **Memory.** The instance holds every entity's stored bytes in memory, so its footprint grows with `ENTITY_STATES`.
- **No authentication, any origin** (#84).
- **`COMMUNITY_INDEX` is not streamed.** State frames carry `bucket`, so another bucket could be added without a
  wire change; none is present.

## Dependencies

- Upstream: graph-ingest writes `ENTITY_STATES`; a subject input needs a NATS client.
- Reads: `pkg/graphview` (one shared view per instance), `graph/kvcatalog`, `natsclient`, `internal/tlsutil`,
  `internal/httpadmission`, `gorilla/websocket` v1.5.3.
