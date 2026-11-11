# Design: `output/websocket` over applied state

base: b7de1e0 (inventory: INVENTORY PASS at b7de1e0). Pin: SemStreams `8b99efe9`. Inventory: `inventory.md` (cited
as "inv § n"). Rulings: #34 comments 6797028887 and 6797377552; #9 comment 6797029453; 03B D14/D15/D4a
(`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:301-331,609-645`, matrix row `:721`).

## Options considered

**Input model.** (a) One input per instance, either applied state or a core subject (chosen): one wire stream per
socket, and a deployment wanting both composes two instances on two ports. (b) Both inputs on one instance: mixed
frames on one socket, plus two readiness stories in one `Health`, with no consumer asking for it. (c) Do nothing
(keep the pin's subject/JetStream relay): contradicts D14 and the 2026-11-11 ruling.

**Applied-state reader.** (a) One `graphview.View` per instance, shared by all its clients (chosen; ADR-081;
`.agents/skills/kv-or-stream/SKILL.md:61`). (b) One `WatchAll` per client: the trap ADR-081 retires. (c) Share
graph-query's view across components: needs a process-level view registry that does not exist, a new primitive for
one reader.

**`COMMUNITY_INDEX`.** (a) Not carried (recommended, owner question 1): no consumer streams communities; every
state frame carries `bucket`, so adding it later changes no frame. (b) Carry it as a second allowed bucket: a
second decode path and test, and a second watch of that bucket in a process that runs graph-query
(`processor/graph-query/community.go:84-88`), because components share no view.

**Fast keep-alive test.** (a) Config key `ping_interval` (chosen, owner question 3): the pin's `doc.go:57-80`
advertises it; operators behind proxies with idle timeouts need it; tests set it to milliseconds. (b) Fixed 30 s/60 s:
the proving test would wait over 60 s of wall clock. (c) A test hook: a back door (#186), refused.

## D1. Inputs

Exactly one input port, resolved at construction:

- `kv-watch` of `ENTITY_STATES`, every key, no history: the default, as graph-clustering accepts it
  (`processor/graph-clustering/config.go:86-104`). One view per instance over `kvcatalog.OpenCatalogReader`
  (`graph/kvcatalog/kvcatalog.go:295`). The decode checks each value with `graph.UnmarshalEntityState`
  (`graph/entity_predicate_contract.go:288`) and keeps the stored bytes and write time, so each frame sends the bytes
  graph-ingest stored, never a re-encoding. A value that does not decode is poison (graphview G6). The view's
  open/run/reopen loop follows graph-query's `runCommunityView` (`processor/graph-query/community.go:40-66`): a missing
  bucket or a lost watch is retried at a fixed recheck. Known cost: the instance holds every entity's stored bytes in
  memory, as graph-embedding's vector cache holds vectors.
- `nats` subject: one `natsclient.Client.Subscribe` (`natsclient/client.go:1057`) in `Start`. At most once, best
  effort: a message nats.go does not deliver, or a client's queue sheds, is gone. Nothing is acknowledged or replayed
  (ADR-095: "external lanes that cannot do so declare explicit at-most-once limits").
- Anything else is refused naming the port. A `jetstream` input names its replacement (a kv-watch of
  `ENTITY_STATES`), and a kv-watch of another bucket, with keys, or with history names what is accepted.

## D2. What a client sees (observable behaviour)

| Situation | Observable | Proving test |
| --- | --- | --- |
| Upgrade on `path` while running and (applied state) the view is caught up | 101; a client is counted | `TestAppliedStateSnapshotThenChanges` |
| Upgrade before the view caught up, or while it is lost | 503, body names the reason; `Health` degraded with that stuck state; process `/readyz` not ready (ADR-109) | `TestNotReadyRefusesUpgrade` |
| Connect after writes | one `put` per current entity, any `poisoned`, then `synced{revision}`, then changes in revision order, no gap or duplicate (graphview G1) | `TestAppliedStateSnapshotThenChanges` |
| Value that does not decode | `poisoned{key,error}`; the client drops its copy | `TestPoisonedValueIsAnnounced` |
| Watch lost (bucket deleted, as `pkg/graphview/integration_test.go:323`) | each client gets close 1013; new upgrades 503 until a new view catches up; a reconnect gets a fresh snapshot | `TestWatchLossClosesAndRecovers` |
| Subject message, valid JSON | every client gets `data{subject,timestamp,payload}` with `payload` byte-identical to the message | `TestSubjectFanOut` |
| Subject message, not JSON | not sent; `messages_dropped_total{reason="not_json"}` +1 | `TestSubjectFanOut` |
| One client stops reading | other clients keep receiving. Subject: its 256-message queue drops oldest, counted `client_queue_full`. Applied state: its changes merge per key. | `TestStalledClientDoesNotSlowOthers` |
| Client answers pings, sends nothing | stays connected (pong extends the read deadline to 2 × `ping_interval`; probe P1) | `TestQuietClientStaysConnected` |
| Client answers no pings | closed after 2 × `ping_interval`; `client_disconnections_total{reason="read_timeout"}` | `TestSilentClientIsClosed` |
| Client sends text or binary | read and discarded; never answered | `TestQuietClientStaysConnected` |
| A write blocked 10 s (pin `websocket.go:1743`) | the client is closed. What gorilla does at the deadline is not promised here (ADR-112); uncovered | none (uncovered) |
| Origin header | every origin accepted, as the pin (`:433-436`); no authentication (#84); documented as a limit | `TestSubjectFanOut` (cross-origin dial) |

Probe P1 (PR #592): at gorilla v1.5.3, a server that sets a read deadline before each `ReadMessage` and records pongs
without extending it returns `i/o timeout` at the deadline with a pong received. Extending the deadline in the pong
handler kept the connection alive past 8 deadlines with 15 pongs.

## D3. Lifecycle (ADR-108, ADR-113, ADR-095)

On `component.Lifecycle`. The pin's own state machine (`websocket.go:155-170`, S5) is not carried.

- `Start(ctx)` binds the listener on the output `NetworkPort`, loads TLS when `Security.TLS.Server.Enabled`, and
  subscribes the subject or launches the view loop. A failed bind, TLS load or subscribe returns the error and holds
  nothing: the port is free, which a test checks by dialling it. A failed rollback is retained for `Stop` (the shell).
- Ending `Start`'s context after it returned stops nothing. Connections and the view run under the one run context
  `Stop` ends (ADR-113; this replaces the pin's `websocket.go:642`, S12).
- `Stop(ctx)` refuses new upgrades with 503, sends close 1001 to each client, ends the subscription and the view, and
  returns once every client handler and the server have returned, or with `ctx`'s error, in which case a later `Stop`
  retries. After `Stop` returns nil the port is released and `clients_connected` is 0.
- Upgrades admitted and still running are counted through `internal/httpadmission` (S1, adopting the one home
  `metric.Server` and the service manager use).
- `Health`: running, plus stuck states "applied-state view catching up" or "applied-state watch lost, reopening".
  `DataFlow` from `Lifecycle.CountInput`/`CountError`.
- Proving: the lifecycle suite through an adapter, as `processor/graph-clustering/component_integration_test.go`;
  `TestStartFailureReleasesPort`; `TestStartContextEndingStopsNothing`.

## D4. Wire format and consumer migration

One JSON text message per frame. `timestamp` is Unix milliseconds: receive time for `data`, the KV write time for
state frames (omitted when the view does not carry it).

```json
{"type":"data","subject":"boids.frames","timestamp":1731330000123,"payload":{...message, unchanged...}}
{"type":"put","bucket":"ENTITY_STATES","key":"<entity id>","revision":42,"timestamp":...,"payload":{...stored state...}}
{"type":"delete","bucket":"ENTITY_STATES","key":"<entity id>","revision":43,"timestamp":...}
{"type":"poisoned","bucket":"ENTITY_STATES","key":"<entity id>","revision":44,"error":"graph_state_reset_required: ..."}
{"type":"synced","bucket":"ENTITY_STATES","revision":44}
```

The frame type stays unexported, so the spec and README are its contract. It replaces the pin's `MessageEnvelope`
(`websocket.go:209-214`): `id` is dropped (its only reader was the ack path), and `timestamp`/`subject` are no longer
injected into the payload (pin `:1298-1329`). The payload is sent exactly as the pin's `passthrough` did.

- **semsource** (`cmd/semsource/run.go:1058-1110`): drop the `graph_entities` JetStream input (or set a kv-watch of
  `ENTITY_STATES`; the default is that) and `delivery_mode`. `/ws` clients receive applied entities (`put`, `delete`,
  `poisoned`, `synced`) instead of every `graph.ingest.*` kind, so manifest, status and predicate payloads no longer
  arrive on `/ws`. Its ADR-0001 ("WebSocket output consumes the raw GRAPH stream") is superseded on its side. If it
  does nothing, boot refuses with an error naming `delivery_mode`, then the `jetstream` port.
- **semboids** (`configs/flock.json:206-238`): drop `delivery_mode`; the subject input stays. Browser code reads
  `frame.payload` (unchanged bytes) and `frame.subject`/`frame.timestamp` from the frame, not the payload.

## D5. Config

Strict decode (`component/validation.go:183`), refusing unknown keys by name. Kept: `ports` (one input per D1; one
`network` output, host plus port 1024-65535 as the pin's `:612`, port 0 refused), `path` (default `/ws`, validated
once at construction; the pin did it in four places, S4). New: `ping_interval` (duration > 0, default `30s`; read
deadline 2×). Removed and refused, with a hint: `delivery_mode` ("delivery is at most once"), `ack_timeout`,
`passthrough` ("the payload is always sent unchanged"). Factory refuses a nil `NATSClient` (S13). Each kept key has a
test that fails when the key is ignored (`TestConfigKeysAreRead`; refusals in `TestConfigRefusals`, one case per key).
The default config (`{}`) is a kv-watch of `ENTITY_STATES`, `0.0.0.0:8081`, `/ws`. The declarer and constructor share
one function (component-registration).

## D6. TLS and metrics

TLS: `internal/tlsutil.LoadServerTLSConfig(deps.Security.TLS.Server)` (`internal/tlsutil/tlsutil.go:19`), mTLS block
applied, as the platform's other listeners. ACME is not ported (ruling 1). An `acme` mode or `acme.enabled` is refused
at `Start` by tlsutil's existing check, naming the key (`TestACMEModeRefusedAtStart`, `TestTLSServesWSS`).

Metrics (`semengine_websocket_*`, registered with `metric.RegisterOrGet` and using the returned collector; a nil
registry leaves them unregistered, as graph-clustering). Each has a `listener` label (host:port, unique per instance
by the registry's network-claim admission, `component/registry.go:631-669`): `messages_received_total`,
`messages_sent_total`, `bytes_sent_total`, `message_size_bytes`, `clients_connected`, `client_connections_total`,
`client_disconnections_total{reason}`, `errors_total{type}`, and new `messages_dropped_total{reason}` (the declared
drops of D2). Dropped: `broadcast_duration_seconds` (no broadcast waits now), `server_uptime_seconds` (`Health` has
`Uptime`; it needed a ticking goroutine), and the `subject` label (unbounded under wildcards). Proved by
`TestMetricsCountTraffic`.

## D7. Shape sweep dispositions (inv § 7)

| # | Disposition |
| --- | --- |
| S1 | Adopted `internal/httpadmission`; the pin copy is not carried (rewritten with S5) |
| S2, S6 | Gone with the JetStream input and the test fakes |
| S3 | Gone: no broadcast snapshot; each client has its own writer |
| S4 | One validation at construction |
| S5 | Required (ADR-108; AGENTS row): `component.Lifecycle` |
| S7 | Required (#186, #190): no hooks; tests drive real NATS and real sockets |
| S8 | Only `Register` exported; everything else unexported or dropped (consumers compose by JSON, inv § 4) |
| S9, S11 | Required (silent-loss gate): at-least-once, `PendingMessage`, the pending ring and `delivery_mode` dropped; declared at most once |
| S10 | Required (declare or drop): the `slow` signal is dropped; client frames are discarded (D2) |
| S12 | Required (ADR-113): the one run context, ended in `Stop` |
| S13 | Required: nil NATS client refused |
| S14 | Required (build; metric-registry spec): `RegisterOrGet` |
| S15 | Gone with the JetStream durable; metrics use the `listener` label |
| S16 | Kept as the pin (every origin), declared in the README and spec; optional, #84 owns authentication |
| S17 | Dropped (dead state) |
| S18 | Required (a ported defect; probe P1): a pong extends the read deadline |

Also gone: pin `doc.go`/`README.md` text describing behaviour that does not exist (inv § 4 c); both are rewritten
against the code.

## D8. Open issues (inv § 3)

Issue #34: retired (`Closes #34`, tasks 2-5). #196: its websocket item is retired by task 3.1; the issue stays for its other
packages, and the PR comments on it. #195: its 18 `output/websocket` `NewTestClient` sites are not carried (tests use
`natsfixture`); the issue stays for others, and the PR comments. #284: stays (about products using `tlsutil`;
optional). #285: stays until ACME is ported (ruling 1). #477: stays. The websocket is a new adopter, noted in a PR
comment; whether the SSE item still applies is owner question 4. And #84 stays; the websocket's lack of authentication
is declared.

## D9. Governing records

- 03B D14 and the matrix row govern. **Pin ADR-090 decision 6 is not carried** (ruling 6797377552): applications may
  stream applied state through an output component. ADR-115 records it. Request/response reads keep going through
  the gateway (#588 cites decision 6 for that; owner question 2). ADR-090 is not ported: it records state ownership
  across the pin, and only this decision touches this package.
- ADR-081 (on main) adopted: one watch per instance, snapshot then changes, merging per key, fail-closed loss.
- ADR-095 (on main) kept through the shell's failed-start retention; its JetStream mechanics leave with the input,
  and its at-most-once declaration rule applies to the subject input.
- ADR-108 Decision 1 adopted (the shell; `docs/adr/108-processor-shell.md:181` names this site). ADR-113 applied.
  ADR-112: probe P1.
- ADR-092 (pin, `pkg/lifecycle` watches) is not binding here. A close frame is terminal as it says, and a client
  reconnects for a fresh snapshot.
- New ADR-115: the three HTTP surface roles.

## D10. Ledger and docs

New row: `source_path: output/websocket`, pin SHA, `destination: output/websocket`, `disposition: adapt`.
`contract` cites the 03B matrix row (`design.md:721`) and the `output-websocket` spec. `known_risks` names
`class:port-refactor` (D4a), at-most-once on the subject input, no authentication or origin check, and the
memory held by the view. `proving_tests` lists D2/D3's tests. `dependencies_and_side_effects` lists `component`,
`graph`, `graph/kvcatalog`, `pkg/graphview`, `natsclient`, `metric`, `internal/tlsutil`, `internal/httpadmission`,
`pkg/security`, gorilla/websocket v1.5.3; a listener; one goroutine group joined by `Stop`.

Edits: `internal/tlsutil` (`docs/admission-ledger.yaml:716-733`): "deferred to change 5" and "until change 5" become
"deferred until a consumer needs ACME (owner, 2026-11-11, #9 comment 6797029453)", and `consumer_purpose` adds
`output/websocket`. `pkg/buffer` (`:4875-4878`): the websocket pending-ack ring is removed as a reader (at-least-once
not ported). New `pkg/acme` row, `disposition: defer-exclude`, `destination: none`, citing the same ruling, so
`task status` counts it excluded (`scripts/port-status.sh:142`). `docs/repository-map.md` gains `output/websocket` and
ADR-115. `.agents/skills/query-pattern/SKILL.md` gains one line: an application streams applied state through an
output component (ADR-115), never the debug SSE.

## Tests and test rules

Unit (no NATS): config, declarer parity, refusals. Integration (`-tags integration`, `natsfixture` shared server through
its reset, no `t.Parallel`): everything in D2/D3, plus the lifecycle suite. Ports: a `127.0.0.1:0` pick, released and
configured with host `127.0.0.1`, as `internal/smoke/stack_integration_test.go:429` does. Clients dial `127.0.0.1`,
never `localhost`. No `time.Sleep`, no skips, no test-owned deadlines: waits are on frames, close errors, client-side
ping counts, and `Health`. Oracles: frames are decoded with the test's own struct; payload bytes are compared to what
was published or read back from the bucket. Shown able to fail: `task mutate:check` on the pong extension (S18), the
drop-oldest shed, and the synced marker. The matrix proving test is `TestAppliedStateSnapshotThenChanges` plus the
smoke step: graph-ingest applies an entity, and a websocket client in the composed stack receives its `put`.
Uncovered: the write-deadline close (D2).

## Size, steps and merge order

Pin: 2,220 production (doc.go 294, websocket.go 1,926), 3,659 test. Not carried: JetStream input about 260, at-least-once
about 150, ACME wiring about 30, the lifecycle state machine about 370. Estimate: about 1,000 production, 1,400 test, 150
elsewhere. One PR (ruling 2), four reviewed steps, each ending at a clean-tree local `task verify` and a review stop:
(1) config, ports, `Register`; (2) server, subject input, keep-alive, TLS, metrics, lifecycle; (3) applied-state input;
(4) smoke, docs, ledger, archive.

Merge order: #588 (graph-gateway, ready) first. It shares `go.mod`, `go.sum`, `docs/admission-ledger.yaml`,
`docs/repository-map.md`, `internal/smoke/*` and `openspec/specs/composed-stack/spec.md`. #592 merges `origin/main`
before step 1 if #588 has landed, and again before archive.

## Guidance carried

The pin carries no guidance naming this package (inv § 6). `kv-or-stream` and `query-pattern` are already here. The
`query-pattern` line in D10 is the adapted addition.

## Owner questions

1. `COMMUNITY_INDEX`: carry it now, or defer it (recommended)?
2. ADR-090 decision 6 and #588: request/response reads keep going through the gateway as #588 cites, and only the
   "raw KV is not an application fallback" part is not carried for streaming (ADR-115's reading). Or should #588's
   citation point to ADR-115?
3. New key `ping_interval`: accept it as new surface (no consumer caller; offered through the spec and README)?
4. #477's SSE item: keep it on #477, or drop it there now that ADR-115 says debug routes are not built for many
   watchers?
