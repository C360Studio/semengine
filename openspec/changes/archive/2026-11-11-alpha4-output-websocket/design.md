# Design: `output/websocket` over applied state

base: b7de1e0 (INVENTORY PASS). Pin: SemStreams `8b99efe9`. Inventory: `inventory.md` ("inv § n"). Rulings: #34
comments 6797028887 and 6797377552; #9 comment 6797029453; 03B D14/D15/D4a
(`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:301-331,609-645`, matrix row `:721`).

## Rulings and where they land

| Ruling | Lands in |
| --- | --- |
| ACME deferred; file TLS only (6797028887 item 1; #9 6797029453) | D6 `design.md:134`; D10 `design.md:170`; ADR-115 `:76` |
| One PR, reviewed steps (6797028887 item 2) | "Size, steps and merge order" `design.md:201`; `tasks.md` review stops |
| Three HTTP surface roles (6797377552 items 1-3) | ADR-115 `:36-49` |
| Debug routes stay supported for people, tests, agents (owner, 2026-11-11) | ADR-115 `:40-43` |
| 03B D14 governs; ADR-090 decision 6 not carried (6797377552) | D9 `design.md:161`; ADR-115 `:56-57` |
| Two inputs: shared graphview watch; core subject at most once (6797377552) | D1 `design.md:46`; `specs/output-websocket/spec.md:5`, `:55` |
| JetStream input dropped (6797377552) | D1 `design.md:46`; D5 `design.md:126`; `specs/output-websocket/spec.md:5` |
| Shed a slow client's data, never slow the rest (6797377552 item 3) | D2 `design.md:60`; `specs/output-websocket/spec.md:67` |
| App SSE = second wire format here; no GraphQL subscriptions (6797377552) | ADR-115 `:59-60` |
| `adapt` row naming `class:port-refactor`; matrix proving test (03B D4a, `:721`) | D10 `design.md:170`; "Tests" `design.md:182`; `specs/output-websocket/spec.md:17`, `:29`; `specs/composed-stack/spec.md:5` |

Owner decisions (2026-11-11, #34 comment 6798408792), each as recommended:

1. `COMMUNITY_INDEX` is not carried; the websocket streams `ENTITY_STATES` only.
2. #588's citation of pin ADR-090 decision 6 stays; ADR-115 drops decision 6 only for streaming.
3. The new key `ping_interval` is accepted.
4. #477's SSE item stays, as optional.

## Options considered

- **Input model.** (a) One input per instance (chosen): one wire stream per socket, two instances for both. (b) Both on
  one instance: mixed frames, two readiness stories, no consumer asking. (c) Keep the pin's relay: against D14.
- **Applied-state reader.** (a) One `graphview.View` per instance shared by its clients (chosen; ADR-081;
  `.agents/skills/kv-or-stream/SKILL.md:61`). (b) A `WatchAll` per client: the trap ADR-081 retires. (c) Share
  graph-query's view: needs a process-level view registry, a new primitive for one reader.
- **Applied-state contract.** (a) Current-state convergence (chosen): what graphview gives, merging per key in each
  250 ms window and per slow subscriber (`pkg/graphview/view.go:17`, `:628-654`, `:737-774`). (b) Every revision: needs
  a JetStream consumer per instance or client, the at-least-once machinery the pin never finished; no consumer asks.
- **`COMMUNITY_INDEX`.** (a) Not carried (chosen, owner decision 1): no consumer streams communities; state frames
  carry `bucket`, so it can be added later without a wire change. (b) Carried: a second decode path, and a second
  watch of the bucket beside graph-query's (`processor/graph-query/community.go:84-88`), since components share no
  view.
- **Fast keep-alive test.** (a) Key `ping_interval` (chosen): pin `doc.go:57-80` advertises it, proxies with idle
  timeouts need it. (b) Fixed 30 s/60 s: a test waits over a minute. (c) A test hook: a back door (#186).

## D1. Inputs

Exactly one input port, resolved at construction. Anything else is refused naming the port; a `jetstream` input names
its replacement.

- **Applied state:** a kv-watch of every `ENTITY_STATES` key, no history (the default; as
  `processor/graph-clustering/config.go:86-104`). One view per instance over `kvcatalog.OpenCatalogReader`. Its decode
  checks each value with `graph.UnmarshalEntityState` (`graph/entity_predicate_contract.go:288`) and keeps the stored
  bytes and write time, so a frame carries the bytes graph-ingest stored; a value that does not decode is poison
  (graphview G6). The open/run/reopen loop follows `processor/graph-query/community.go:40-66`. Cost: the instance holds
  every entity's stored bytes in memory, as graph-embedding's vector cache holds vectors.
- **Core subject:** one `natsclient.Client.Subscribe` (`natsclient/client.go:1057`) in `Start`. At most once, best
  effort; nothing is acknowledged or replayed (ADR-095's at-most-once declaration).

## D2. What a client sees

| Situation | Observable | Test |
| --- | --- | --- |
| Upgrade before the view caught up, or while lost | 503 naming why; `Health` degraded; `/readyz` not ready (ADR-109) | `TestNotReadyRefusesUpgrade` |
| Connect | a `put` per current entity, any `poisoned`, `synced{revision}`, then changes (G1: no seam gap at attach) | `TestAppliedStateSnapshotThenChanges` |
| Changes after `synced` | converge: per key, revisions increase and the latest write always arrives; values in between may be merged away, in the view's window or for a slow client | `TestAppliedStateConverges` |
| Value that does not decode | `poisoned{key,error}`; the client drops its copy; a later valid write heals | `TestAppliedStateConverges`, `TestPoisonedValueIsAnnounced` |
| Watch lost (bucket deleted, as `pkg/graphview/integration_test.go:323`) | close 1013; 503 until a new view catches up; reconnect gets a fresh snapshot | `TestWatchLossClosesAndRecovers` |
| Subject message, valid JSON | every client gets `data{subject,timestamp,payload}`, payload byte-identical | `TestSubjectFanOut` |
| Subject message, not JSON | not sent; counted and logged (`not_json`) | `TestSubjectFanOut` |
| A client stops reading | others unaffected. Subject: its 256-frame queue drops oldest (`client_queue_full`). Applied state: its changes merge per key (`client_behind`). Both counted and logged | `TestStalledClientDoesNotSlowOthers` |
| Client answers pings, sends nothing; or sends frames | stays connected (pong extends the read deadline to 2 × `ping_interval`; probe P1); its frames are discarded | `TestQuietClientStaysConnected` |
| Client answers no pings | closed; `client_disconnections_total{reason="read_timeout"}` | `TestSilentClientIsClosed` |
| Write blocked 10 s (pin `websocket.go:1743`) | client closed; what gorilla does at the deadline is not promised (ADR-112) | uncovered |
| Any Origin | accepted, as pin `:433-436`; no authentication (#84); documented | `TestSubjectFanOut` |

**Declared drops.** Each drop or merge is a counter (`messages_dropped_total{reason}` for `not_json` and
`client_queue_full`; `changes_coalesced_total` for `client_behind`, fed by graphview's `Hooks.OnFanOut` overwritten
count, `pkg/graphview/view.go:74-80`) and a warning naming the reason, the listener and why continuing is safe (other
clients and later frames are unaffected; on applied state the latest write still arrives). The first warning per
reason logs at once; later ones at most once a minute per reason, with the count since the last line. Merging inside
the view's own window is the contract, not a drop, and graphview does not count it. The tests read the counter and a
captured `deps.Logger`; removing either emit fails them.

Probe P1 (PR #592): at gorilla v1.5.3 a server that sets a read deadline per `ReadMessage` and records pongs without
extending it gets `i/o timeout` at the deadline with a pong received; extending it in the pong handler kept the
connection past 8 deadlines with 15 pongs.

## D3. Lifecycle (ADR-108, ADR-113, ADR-095)

On `component.Lifecycle`; the pin's state machine (`websocket.go:155-170`) is not carried. `Start(ctx)` binds the
listener, loads TLS when enabled, and subscribes or launches the view loop; a failure returns the error and holds
nothing (the port dials refused). Ending `Start`'s context afterwards stops nothing; connections and the view run under
the one run context `Stop` ends (ADR-113; replaces pin `:642`). `Stop(ctx)` refuses new upgrades with 503, sends close
1001, ends the subscription and the view, and returns when every client handler and the server have returned, or with
`ctx`'s error, retried by a later `Stop`. After nil, the port is released and `clients_connected` is 0. Running upgrades
are counted by `internal/httpadmission`. `Health` adds the stuck states "applied-state view catching up" and
"applied-state watch lost, reopening". Tests: the lifecycle suite through an adapter (as
`processor/graph-clustering/component_integration_test.go`), `TestStartFailureReleasesPort`,
`TestStartContextEndingStopsNothing`.

## D4. Wire format and consumer migration

One JSON text message per frame; `timestamp` is Unix ms (receive time for `data`, KV write time for state frames).

```json
{"type":"data","subject":"boids.frames","timestamp":1731330000123,"payload":{"...":"message, unchanged"}}
{"type":"put","bucket":"ENTITY_STATES","key":"<entity id>","revision":42,"timestamp":0,"payload":{"...":"stored"}}
{"type":"delete","bucket":"ENTITY_STATES","key":"<entity id>","revision":43,"timestamp":0}
{"type":"poisoned","bucket":"ENTITY_STATES","key":"<entity id>","revision":44,"error":"graph_state_reset_required"}
{"type":"synced","bucket":"ENTITY_STATES","revision":44}
```

The frame type is unexported; the spec and README are its contract. It replaces the pin's `MessageEnvelope`
(`websocket.go:209-214`): `id` is dropped (its reader was the ack path), and the payload is no longer rewritten with
`timestamp`/`subject` (pin `:1298-1329`).

- **semsource** (`cmd/semsource/run.go:1058-1110`): drop the JetStream input (the default is the kv-watch) and
  `delivery_mode`. `/ws` now carries current entity state (`put`, `delete`, `poisoned`, `synced`), not every
  `graph.ingest.*` message: no manifest, status or predicate payloads, and no promise of every revision. A client keeps
  a map by `key` and treats the latest frame as truth. Its ADR-0001 is superseded on its side. Doing nothing fails boot
  naming `delivery_mode`.
- **semboids** (`configs/flock.json:206-238`): drop `delivery_mode`; read `frame.payload` (unchanged bytes) and
  `frame.subject`/`frame.timestamp` from the frame.

## D5. Config

Strict decode (`component/validation.go:183`) naming unknown keys. Kept: `ports` (D1; one `network` output, port
1024-65535 as pin `:612`, 0 refused), `path` (default `/ws`, validated once; the pin did it four times). New:
`ping_interval` (> 0, default `30s`). Refused with a hint: `delivery_mode`, `ack_timeout`, `passthrough`. A nil
`NATSClient` is refused. Default (`{}`): kv-watch of `ENTITY_STATES`, `0.0.0.0:8081`, `/ws`; the declarer and
constructor share one function. Tests: `TestConfigKeysAreRead`, `TestConfigRefusals` (one case per key).

## D6. TLS and metrics

TLS: `internal/tlsutil.LoadServerTLSConfig(deps.Security.TLS.Server)` (`internal/tlsutil/tlsutil.go:19`), mTLS
applied; ACME not ported, and an `acme` mode is refused at `Start` by tlsutil, naming the key (`TestTLSServesWSS`,
`TestACMEModeRefusedAtStart`). Metrics `semengine_websocket_*` via `metric.RegisterOrGet`, using the returned collector
(nil registry: unregistered, as graph-clustering), each labelled `listener` (host:port, unique by the registry's
network claims, `component/registry.go:631-669`): `messages_received_total`, `messages_sent_total`,
`bytes_sent_total`, `message_size_bytes`, `clients_connected`, `client_connections_total`,
`client_disconnections_total{reason}`, `errors_total{type}`, new `messages_dropped_total{reason}` and
`changes_coalesced_total`. Dropped: `broadcast_duration_seconds` (no broadcast wait), `server_uptime_seconds`
(`Health` has `Uptime`), the `subject` label (unbounded under wildcards). Test: `TestMetricsCountTraffic`.

## D7. Shape sweep (inv § 7)

Required: S5 (`component.Lifecycle`, ADR-108), S7 (no hooks, #186), S9/S11 (at-least-once, `PendingMessage`, the
pending ring and `delivery_mode` dropped; at most once declared), S10 (`slow` signal dropped; client frames discarded),
S12 (ADR-113 run context), S13 (nil client refused), S14 (`RegisterOrGet`), S18 (pong extends the deadline; P1).
Adopted though optional: S1 (`internal/httpadmission`), S4 (one validation). Gone with removed code: S2, S3, S6, S15
(metrics use `listener`), S17 (dead state). S8: only `Register` is exported. S16: every origin kept and declared; #84
owns authentication. Pin `doc.go` and `README.md` are rewritten against the code (inv § 4 c).

## D8. Open issues (inv § 3)

Retired: #34 (`Closes #34`). Websocket items retired, issues stay for others, with a PR comment: #196 (task 3.1), #195
(tests use `natsfixture`). Stay: #284 (products using `tlsutil`; optional), #285 (until ACME), #477 (new adopter noted;
owner decision 4: stays, optional), #84 (no authentication, declared).

## D9. Governing records

03B D14 and the matrix row govern; pin ADR-090 decision 6 is not carried for streaming, recorded in ADR-115 (owner
decision 2: #588's citation stays). ADR-090 is not ported: only this decision touches the package. ADR-081 adopted,
including its merge per key, which is why D2 promises convergence, not every revision. ADR-095 kept through the shell;
its JetStream mechanics leave with the input. ADR-108 Decision 1 adopted (`docs/adr/108-processor-shell.md:181` names
this site). ADR-113 applied. ADR-112: probe P1. Pin ADR-092 (`pkg/lifecycle`) is not binding; a close frame is
terminal and a client reconnects for a fresh snapshot.

## D10. Ledger and docs

New row `output/websocket` (pin SHA, destination `output/websocket`, `adapt`): `contract` cites the matrix row and the
`output-websocket` spec; `known_risks` names `class:port-refactor`, at-most-once subject delivery, convergence (not
every revision) on applied state, no authentication or origin check, and the view's memory; `proving_tests` lists D2
and D3's tests; dependencies include gorilla/websocket v1.5.3. Edits: `internal/tlsutil`
(`docs/admission-ledger.yaml:716-733`): "deferred to change 5" becomes "deferred until a consumer needs ACME (#9
comment 6797029453)", and `consumer_purpose` adds `output/websocket`. `pkg/buffer` (`:4875-4878`): the websocket ring
is no longer a reader. New `pkg/acme` row, `defer-exclude`, `destination: none` (`scripts/port-status.sh:142`).
`docs/repository-map.md`: the package and ADR-115. `.agents/skills/query-pattern/SKILL.md`: an application streams
applied state through an output component (ADR-115), never the debug SSE.

## Tests

Unit: config, declarer parity, refusals. Integration (`-tags integration`, `natsfixture` shared server through its
reset, no `t.Parallel`): D2, D3, the lifecycle suite. Ports: a `127.0.0.1:0` pick, released, host `127.0.0.1`, as
`internal/smoke/stack_integration_test.go:429`; clients dial `127.0.0.1`. No sleeps, skips, or test-owned deadlines:
waits are on frames, close errors, client-side ping counts and `Health`. Oracles: the test's own frame struct; payloads
compared to what was published or read back; applied-state clients fold frames into a map compared to the bucket read
back. Shown able to fail (`task mutate:check`): the pong extension, drop-oldest, the synced marker, each warning emit.
Matrix proving test: `TestAppliedStateConverges` and the composed-stack step (graph-ingest applies; a client receives).

**Generated checks (`docs/testing.md:435-446`): not added; examples suffice.** The order-dependent law (a snapshot
plus coalesced deltas converges to the bucket, no seam gap, poison and heal) belongs to graphview and is already a
generated history against a reference model (`pkg/graphview/history_prop_test.go:81`,
`TestViewHistoryMatchesModel`). This component adds a per-batch translation from deltas to frames with no state across
batches except the client's queue, so its own combinations are the frame kinds and their order within a batch. The
named boundary cases are: a key put twice, put then deleted, and put, poisoned, healed in one burst; a client attaching
mid-burst; a stalled client beside a live one; the bucket deleted and recreated. `TestAppliedStateConverges` checks
the fold against the read-back whichever window each write falls in, so the test does not depend on timing.

## Size, steps and merge order

Pin: 2,220 production, 3,659 test. Not carried: JetStream input (about 260), at-least-once (about 150), ACME wiring
(about 30), the state machine (about 370). Estimate: about 1,000 production, 1,450 test, 150 elsewhere. One PR, four
steps, each ending at a clean-tree `task verify` and a review stop: (1) config and `Register`; (2) server, subject
input, keep-alive, TLS, metrics, lifecycle; (3) applied state; (4) smoke, docs, ledger, archive. PR #588 merges first
(shared `go.mod`, `go.sum`, ledger, repository-map, `internal/smoke/*`, `composed-stack` spec). The `composed-stack`
delta is written against the text #588 lands, and #592 merges `origin/main` before step 1 and before archive.

## Guidance carried

No pin guidance names this package (inv § 6); `kv-or-stream` and `query-pattern` are here; D10 adds one line.
