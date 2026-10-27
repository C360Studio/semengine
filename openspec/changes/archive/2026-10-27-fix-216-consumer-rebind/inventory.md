# Inventory for #216: graph-ingest stops when its input stream is recreated

base: `ef037ea7a3116026222eaeee9ebf829ce57f155a` (main; the claim branch `claude/fix-216-consumer-rebind` head
`62e623686b9af502ad25792fb96ab06f22cfcb4a` adds an empty claim commit only). Draft PR #431.
Read in `/Users/coby/Code/c360/semengine/.worktrees/claude-fix-216-consumer-rebind`.

This file is the inventory only (architect contract, Required workflow step 2). It holds no target state. The
options the brief asked for are in a separate file, `options-preview.md`, for the caller to hold until the inventory
passes review.

Terms used below:

- **Consume loop**: the delivery loop nats.go's `jetstream.Consumer.Consume(handler)` runs. It sends pull requests
  to one named consumer on the server and calls the handler for each message the server returns.
- **Durable consumer**: a consumer the server keeps by name (`graph-ingest-<subject>` here) until it is deleted.
- **Zombie loop**: a consume loop that keeps running and pulling, but whose consumer no longer exists, so it never
  delivers anything again.
- **Generation**: graph-ingest's name for a stream's creation time on the server, read once in `Start`; the
  redelivery guard compares it to tell two lives of the same stream name apart.
- **Readiness tick**: graph-ingest's loop that, every 5 s, reads each bound consumer's outstanding work from the
  server and publishes the result; `Health()` reports its latest failure (`processor/graph-ingest/readiness.go:72-101`).
- **Claim**: natsclient's local ownership record for one (stream, durable name) pair on one client; a second
  local owner of the same pair is refused (`natsclient/stream.go:609-631`), and the claim is released when the
  consume loop has ended and its handlers have returned.
- **Policy entry point**: natsclient's consume functions that apply the consumer policy (`ConsumeStreamWithConfig`
  and its siblings), plus the port reader `GetConsumerConfig` that feeds them; only allowlisted files may call them
  (#422).
- **Adopter seam**: the point where code or an operator outside this repository uses the surface: what they must
  know, and what happens when they do not (architect contract, "The adopter seam inventory").

## 0. Answer first

- **#216 is real.** At the pinned nats.go, a consume loop does not recover by itself when its stream comes back
  empty after a broker restart (probe case a), and stops for good when its stream is deleted while the server is up
  (case d). nats.go does not cover this, and natsclient's ADR says it does not promise consumers across a reconnect
  (`docs/adr/112-natsclient-contract.md:216`: "consumers across a reconnect").
- **One case the issue did not list is worse than the stall.** If anyone creates the consumer again under the same
  name after the loop stopped, `Health()` goes back to healthy while nothing is delivered (cases e and f). PR #299's
  test `TestIntegration_HealthClearsWhenObservationSucceeds_ConsumptionNotChecked` builds exactly that end state.
- **Even where nats.go does resume (case b), graph-ingest would mis-handle what it delivers**: every input carries
  the generation read in the first `Start`, so on a recreated stream an input at a sequence already recorded for its
  entity is acknowledged without being applied (a code reading, below; not run through graph-ingest).

## 1. The probe

### 1.1 Setup

- Source: `/tmp/fix-216-stage/probe/main.go`, module `/tmp/fix-216-stage/probe/go.mod`; raw output
  `/tmp/fix-216-stage/probe/out-{a,b,c,d,e,f}.txt`. Every output file starts with the versions the binary was built
  with, read from its own build info: `github.com/nats-io/nats.go v1.54.0`, `github.com/nats-io/nats-server/v2
  v2.15.1` (the versions `go.mod:16-17` pins). Go 1.26.10, darwin/arm64. The server is the go.mod pin, not the
  integration-test image, which is `nats:2.15.0-alpine` (`internal/harness/natsfixture/nats-image:2`, `:6`); the probe
  was not repeated on 2.15.0.
- Command: `cd /tmp/fix-216-stage/probe && go build -o probe . && ./probe <a|b|c|d|e|f>`. Run once per scenario on
  2026-10-27 (a to e in parallel, each with its own server and port; f afterwards). Scenario f was added to `main.go`
  after a to e ran; the edit adds the `f` branch only, so a to e run the same code from the current source.
- Server: in-process `server.NewServer`, JetStream on, one store directory reused across the restart, shut down with
  `Shutdown`/`WaitForShutdown` and started again on the same port. In-process rather than Docker because the restart
  is then a real server restart (memory streams are gone, file streams are reloaded from the store directory) with no
  container port remapping, so the client reconnects to the same URL as a production client would.
- Client, mirroring natsclient: `MaxReconnects(-1)`, `ReconnectWait(2s)`, `PingInterval(30s)`
  (`natsclient/client.go:308-310`, `:399-407`). The consumer is created with `CreateOrUpdateConsumer` (durable name,
  filter subject, deliver all, explicit ack, 30 s ack wait: what `natsclient/stream.go:854-917` builds from
  graph-ingest's port, `processor/graph-ingest/consume.go:126-136`), then `Consume(handler)` with **no** pull options,
  as `natsclient/stream.go:579` and `:691` call it (neither passes a `PullConsumeOpt`). The producer has its own
  connection.
- The one addition is a `jetstream.ConsumeErrHandler` that logs what nats.go reports. It does not change control
  flow: on a terminal status nats.go calls `Stop` whether or not a handler is set
  (`jetstream/pull.go:273-281`), and for other errors it only calls the handler (`:270-271`, `:411-416`).
- Default timings this inherits from nats.go (not promised; read at v1.54.0): pull expiry 30 s
  (`jetstream/pull.go:192`), heartbeat half of that, 15 s (`:1210-1213`), and a missed-heartbeat timer of twice the
  heartbeat, 30 s (`:1090`).

### 1.2 Results

Times are seconds from the probe's start, from the raw output files.

**(a) Memory stream, broker restart, producer recreates the stream, nobody recreates the consumer**
(`out-a.txt`). The handler never sees the new message.

| t (s) | What happened (verbatim) |
| --- | --- |
| 0.010 | `ConsumeErrHandler: *jetstream.jsError nats: server shutdown` (the server answers the open pull as it stops) |
| 2.083 | `consumer conn: RECONNECTED`, then `ConsumeErrHandler: *errors.errorString nats: no responders available for request` |
| 2.106 | `after restart: stream lookup: nats: API error: code=404 err_code=10059 description=stream not found` |
| 2.106 | producer recreates the stream (new creation time, `last_seq=0`) |
| 2.107 | `consumer lookup: ... code=404 err_code=10014 description=consumer not found`; `published "after" stream_seq=1` |
| 32.084, 62.086 | `nats: no heartbeat received`, then `nats: no responders available for request`, every 30 s |
| 77.112 | `RESULT NOT delivered "after" within 1m15s`; `END: ConsumeContext closed=false` |

`Consume` does not stop itself: `Closed()` fired only when the probe closed its connection at the end. It is a
zombie loop: it re-pulls every 30 s against a consumer that does not exist and reports each attempt only through
the error handler natsclient does not set.

Source lines: on reconnect the loop sends a fresh pull (`jetstream/pull.go:358-381`); the server answers a pull for
a missing consumer with a 503 status, which `checkMsg` maps to `nats.ErrNoResponders`
(`jetstream/message.go:483-484`); `handleStatusMsg` treats it as not terminal (`jetstream/pull.go:739-752`), so it
only reaches `ErrHandler` (`:270-271`); the missed-heartbeat timer then re-pulls (`:389-409`), forever. The 409
"server shutdown" status is not terminal either (`message.go:507-508`; `pull.go:740` lists only consumer deleted and
bad request).

**(b) As (a), then someone else recreates the consumer under the same durable name** (`out-b.txt`). The existing
`Consume` resumes by itself, about 30 s after the reconnect.

- 2.149 `other party: consumer recreated under durable "graph-ingest-probe-in"`; `published "after" stream_seq=1`.
- 32.073 `ConsumeErrHandler: *jetstream.jsError nats: no heartbeat received`; 32.074 `HANDLER got "after"
  stream_seq=1`.
- The delay is the missed-heartbeat timer reset at reconnect (`jetstream/pull.go:377-378`, 30 s by `:1090` and
  `:1213`); the re-pull it fires (`:389-404`) reaches the new consumer because the pull subject is built from the
  stream and consumer names only (`:225`). Not promised.
- The recreated stream's creation time differs (`2026-10-27T13:22:06.922903Z` before, `13:22:09.06322Z` after) and
  its sequences restart at 1. See section 2.4 for what graph-ingest does with that.

**(c) File stream, broker restart, nothing recreated** (`out-c.txt`). Delivery resumes without any action.

- 2.159 `after restart: stream exists: created=2026-10-27T13:22:06.923135Z last_seq=1` (same creation time as before
  the restart) and `consumer exists ... delivered.stream_seq=1`.
- 2.160 `HANDLER got "after" stream_seq=2`, about 40 ms after the reconnect (the reconnect-time pull,
  `jetstream/pull.go:366`). No error reached the handler after the reconnect.

**(d) Memory stream, server stays up, stream deleted and recreated, consumer not recreated** (`out-d.txt`).
`Consume` stops itself at once.

- 0.010 `ConsumeErrHandler: *jetstream.jsError nats: consumer deleted` and `ConsumeContext.Closed() fired`.
- 75.011 `RESULT NOT delivered "after"`; `consumer not found` throughout.
- Source: a 409 "Consumer Deleted" status (`jetstream/message.go:501-502`) is terminal in `handleStatusMsg`
  (`jetstream/pull.go:740-741`), and the loop calls `Stop` (`:273-281`).

**(e) As (d), then someone else recreates the consumer** (`out-e.txt`) and **(f) server up, only the consumer
deleted, then recreated** (`out-f.txt`, the shape of PR #299's health test). Nothing is delivered: the loop already
stopped (as in d), and nats.go does not start a new one.

- e, 75.012: `end: consumer exists, num_pending=1 num_ack_pending=0 delivered.stream_seq=0`; `closed=true`.
- f, 75.008: `end: consumer exists, num_pending=2 num_ack_pending=0 delivered.stream_seq=0`; `closed=true`.

### 1.3 What the probe does not show

- One run per scenario; the 2 s reconnect, the 30 s re-pull and the 40 ms resume in (c) are this machine's numbers
  and nats.go's defaults, not promises (ADR-112 §1).
- It does not run graph-ingest. Section 2 maps each case onto graph-ingest from its code.
- It does not cover a clustered server, where a consumer can move between servers (409 leadership change is not
  terminal, `jetstream/pull.go:748-751`), nor a client that stays disconnected longer than the server is down.

## 2. Today's tree

### 2.1 Who creates graph-ingest's input consumer

- `processor/graph-ingest/consume.go:22` `setupSubscriptions` runs `setupJetStreamConsumer`
  (`consume.go:64`) once per JetStream input port, inside `Start`.
- `consume.go:89` — `created, err := waitForStream(ctx, streamName)`; `consume.go:96` —
  `generation := uint64(created.UnixNano())`: the generation is read once, here.
- `consume.go:102` — `consumerName := fmt.Sprintf("graph-ingest-%s", sanitizedSubject)`: the durable name comes from
  the subject alone, so two graph-ingest processes on one subject share one durable consumer on the server.
- `consume.go:156` — `handle, err := consumeStream(ctx, natsclient.PortConsumerContext{...}, cfg, func(...) {`:
  `consumeStream` is `c.natsClient.ConsumeStreamWithConfig` (`:152`) unless a test set the field (`:153-155`, one of
  #301's four test-only fields). Every input the callback submits carries `generation` (`:217`).
- `consume.go:231` — `c.consumers = append(c.consumers, graphIngestConsumerBinding{handle: handle})`; `:236`
  `c.registerBoundConsumer(streamName, consumerName)` for the readiness tick.
- natsclient path: `ConsumeStreamWithConfig` (`natsclient/stream.go:439`) →
  `consumePortStreamWithConfigContexts` (`:745`) → `stream.CreateOrUpdateConsumer` (`:800`) →
  `startPortConsumer` (`:634`) → `consumer.Consume(func(msg jetstream.Msg) {` (`:691`), no options.
  The internal path is the same shape: `CreateOrUpdateConsumer` (`:533`), `guarded.Consume(` (`:579`), no options.
- natsclient's ownership goroutine waits on the handle's `Closed()` and then releases the claim and forgets the
  consumer's metrics (`natsclient/owned_delivery.go:124-169`). It logs nothing and tells no one when `Closed()` fires
  without a `Stop` (case d).
- natsclient's reconnect handler only sets status (`natsclient/client.go:1762-1771`, `handleReconnect`); the issue's
  `client.go:2144-2155` citation was at `0bbd78f` and has moved.
- Stop: `processor/graph-ingest/component.go:574-587` drains each retained handle and waits on its `Closed()`.

### 2.1a Who creates the stream again after a broker restart

Case (a) needs a producer that creates the memory stream again. Nothing in this tree does that after boot:

- `config/streams.go:328` — `func (sm *StreamsManager) EnsureStreams(ctx context.Context, cfg *Config) error {`:
  boot-time framework API; `git grep -n "EnsureStreams(" -- '*.go' ':!*_test.go'` finds only its declaration, no
  production caller here.
- `natsclient/stream.go:147` `EnsureStream` creates a missing stream (`:184` `jetstream.ErrStreamNotFound`, `:195`
  `js.CreateStream`) only when someone calls it; nothing calls it on reconnect.
- graph-ingest never creates its input stream: `processor/graph-ingest/consume.go:133` — `AutoCreate:    false,`;
  it reads the stream only in `Start` (`consume.go:89`).
- Memory is opt-in: an omitted `storage` means file (`config/stream_drift.go:91-98`, `declaredStorage` returns
  `jetstream.FileStorage` unless `"memory"` is declared).
- Evidence that SemSource does create the stream again: #15's body, "Memory `GRAPH` is recreated with lower sequence
  numbers" (#15 body, line 4).

So the routine trigger needs two things: a stream declared `storage: memory`, and a producer outside graph-ingest
that creates it again after the restart (SemSource does). With file storage the case is (c) and recovers. With
memory storage and no producer that recreates the stream, publishing fails at the producer (no stream answers the
subject) rather than being accepted and left unapplied.

### 2.2 The consumer-creation allowlist (#422)

- `internal/harness/contract/consumerpolicy_test.go:40-42` — `policyEntryPointAllowlist`:
  `"processor/graph-ingest/consume.go": true` is the only production file allowed to call a policy entry point.
- `consumerpolicy_test.go:46-48` — `directConsumerCreationAllowlist`: `"natsclient/stream.go": true` is the only
  production file allowed to call `CreateOrUpdateConsumer`/`CreateConsumer`/`OrderedConsumer` (`:30-34`).
- How often a listed file calls is not checked (`:37-38`), so another call from `consume.go` passes the guard; a call
  from any other graph-ingest file fails it.

### 2.3 What Health and readiness do when the consumer is lost

- The readiness tick runs every `statusMetricsInterval` = `readiness.DefaultHeartbeat`
  (`processor/graph-ingest/readiness.go:19`; `graph/readiness/watcher.go:71` — `DefaultHeartbeat = 5 * time.Second`).
- `readiness.go:204-226` `observeOutstandingWork` looks the stream up, then the consumer by name, and fails with
  `"look up stream "+streamName` (`:211`) or `"look up consumer "+consumerName` (`:215`).
- `readiness.go:142-149` stores `"backlog observation failed for consumer ..."` as the fault; `Health()`
  (`processor/graph-ingest/component.go:392-401`) reports it, so `Healthy` is false. `/readyz` answers not ready while
  any component is not healthy (ADR-109, `docs/adr/109-ready-after-boot.md:119-120`).
- Per probe case, from that code:

| Case | Consume loop | Observation | Health |
| --- | --- | --- | --- |
| a (restart, memory, consumer gone) | zombie, runs forever | fails, stream then consumer not found | false, until something recreates the consumer |
| b (as a, consumer recreated) | resumes after ~30 s | succeeds | true; delivers, with the old generation (2.4) |
| c (restart, file) | resumes | succeeds | true |
| d (in place, consumer gone) | stopped | fails, consumer not found | false |
| e, f (stopped, consumer recreated) | stopped | succeeds | **true while nothing is delivered** |

- Tests: `processor/graph-ingest/health_integration_test.go:57` `TestIntegration_HealthReadsLostConsumer` deletes the
  consumer in place and asserts unhealthy. `:80` `TestIntegration_HealthClearsWhenObservationSucceeds_ConsumptionNotChecked`
  deletes it, recreates it (`:94` `js.CreateConsumer`) and asserts healthy (`:96-98`); by probe case f its end state
  is a stopped loop with a green `Health()`. Its comment says so (`:78-79`: "Consumption recovery is not checked or
  promised by this test").
- Nothing restarts a component in-process: `component/lifecycle.go:59-60` — "A second Start is refused. An instance
  never restarts; a host that needs the component again creates a new one." Recovery today is a process restart
  by whatever watches `/readyz`. `/healthz` answers 200 while the listener serves (`service/doc.go:177`), so a
  supervisor that restarts on `/healthz` never restarts it.

### 2.4 The generation after a recreated stream (code reading, not probed)

- The guard drops an input when `r.generation == work.generation && work.seq <= r.seq`
  (`processor/graph-ingest/keyed_ingest.go:88`).
- A running graph-ingest stamps every input with the generation read at `consume.go:96`. If the loop resumes on a
  recreated stream (case b), inputs carry the old generation and the new stream's low sequences, so an entity already
  applied at a higher sequence has its re-ingested input acknowledged and not applied. That is #15's SemSource symptom
  ("Current source re-ingestion is rejected as already applied", #15 body).
- The spec declares this a limitation for a stream and consumer both recreated while graph-ingest is disconnected
  (`openspec/specs/graph-ingest-recovery/spec.md:180-185`, owner ruling #91 comment 6113959050). The archived notes
  name the case b mechanism (`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/notes/4.3.md:56-63`).
- #15's proving test stops graph-ingest before the restart and starts a new instance after it
  (`processor/graph-ingest/guard_generation_integration_test.go:111-121`), so the "graph-ingest keeps running" path
  has no test.

### 2.5 The text #216 says is contradicted (current lines)

- `openspec/specs/graph-ingest-recovery/spec.md:21-25`, scenario "Memory transport lost at broker restart": "the lost
  message is not reported as applied, and publishing it again from the source applies it". Under case a it is not
  applied while the same graph-ingest runs.
- `docs/contract.md:79` — `| NATS stream in memory | accepted messages that were not yet applied are lost; stored
  entities survive | re-publish from your source (semsource does this) |` (the issue's `:75` was at an older commit).
- Text that already tells the truth: `openspec/specs/graph-ingest-recovery/spec.md:232-235` ("this says nothing about
  whether graph-ingest is consuming (a deleted consumer is not rebound, #216)"); `processor/graph-ingest/README.md:188`
  ("A lost durable consumer reads unhealthy this way; graph-ingest does not bind a new one (#216).");
  `docs/adr/112-natsclient-contract.md:216-217` (not promised: consumers across a reconnect; redelivery after a broker
  restart).

### 2.6 Other production callers of the same consume paths

- `git grep -n "ConsumeStreamWithConfig\|ConsumeInternalStreamWithConfig\|ConsumeStreamWithConfigContexts" -- '*.go'
  ':!*_test.go'`: the only production caller outside natsclient is `processor/graph-ingest/consume.go:152`.
  `ConsumeInternalStreamWithConfig` has no production caller (hits are its own file and `natsclient/doc.go:83-84`,
  `:163`).
- `git grep -n "\.Consume(\|\.Messages(\|\.Fetch(\|CreateOrUpdateConsumer\|CreateConsumer(" -- '*.go' ':!*_test.go'
  ':!internal/harness/**'`: only `natsclient/stream.go:533`, `:579`, `:691`, `:800`. No other production code shares
  the defect today. Consumers outside this repo that call these entry points do (framework API; adopter seam
  below).

## 3. Overlaps, issues and decision records

### 3.1 Open pull requests

`gh pr list --state open` on 2026-10-27: two.

- #431 (draft, this claim): no files yet.
- #414 (draft, `claude/port-graph-index`, 65 files; full list via `gh api --paginate .../pulls/414/files`): no file
  under `processor/graph-ingest/`, `natsclient/`, `internal/harness/contract/consumerpolicy_test.go`,
  `docs/contract.md` or `openspec/specs/graph-ingest-recovery/`. **Adjacent:** it edits
  `graph/readiness/index_status.go` and `graph/readiness/gauges.go` (graph-ingest's readiness tick calls
  `readiness.ComputeBacklogStatus`, `processor/graph-ingest/readiness.go:152`), and `AGENTS.md` and
  `.agents/rules-index.md` (conflict only if #216 adds a rule row). Its OpenSpec deltas are `entity-state-follower`,
  `graph-index`, `graph-readiness` and `graph-transport-boundary`, none of which #216 touches. Merge order is the
  design's to state.

### 3.2 Open issues

Searches (`gh issue list --state open --search <q> --limit 200`) for `graph-ingest`, `natsclient`, `consumer`,
`reconnect`, `Consume`, `restart`; hits read. Kept:

- **#15** "repair: preserve graph recovery across transport stream generations" (open). Its symptom is the case b
  path of 2.4 when graph-ingest keeps running. A fix for #216 that resumes delivery without reading the generation
  again would reproduce #15 for a running graph-ingest; a fix that re-reads it (or a process restart) would not.
  Whether #216's fix retires #15 is a design question; #15's own scope is wider (its body).
- **#301** "retire the four test-only NATS fault fields on Component": `consumeStream` and `waitForStreamInput` sit on
  the path a fix touches (`consume.go:81-83`, `:152-155`). Stays unless the design chooses otherwise; a fix must not
  add a fifth such field (no-back-door rule).
- **#83** "nats.go ConsumeContext.Closed() fires early for an invalid subscription": any design that waits on
  `Closed()` to learn the loop ended inherits this caveat (natsclient doc comments `stream.go:423`, `:468`, `:718`). Stays.
- **#300** (optional graph-ingest items) and **#109** (processor shell / follower): name graph-ingest, not its
  consumer binding; stay.
- Names the word, not the subject: #405, #270, #141, #106, #195, #428, #107, #196, #33, #111, #134, #110, #103, #20,
  #198, #108, #16, #9, #350, #96, #95, #94, #7, #24, #420, #281, #68, #162, #280, #418, #60, #361, #36, #161, #27,
  #26, #284, #10, #429, #11, #105, #415, #114, #203.
- **#291** (graph-ingest `Start` during a reconnect), which #216 was to pair with, is **closed** (fixed by PR #316).
  **#420** (KeysByFilter watcher drain) is about KV watches, not consumers; unrelated.

### 3.3 Decision records that bind

- **ADR-112**, `docs/adr/112-natsclient-contract.md:109` — "natsclient uses nats.go's connection as nats.go provides
  it"; `:216-217` — not promised: "consumers across a reconnect", "redelivery after a broker restart". With the owner's
  #170 ruling (natsclient passes nats.go through; no added mechanism) and the ADR's probe rule (a statement about
  how nats.go times, orders, bounds or fails cites a probe at the pinned version, or says it is not promised).
- **ADR-095**, `docs/adr/095-one-shot-running-lifecycle-with-retained-failed-start-cleanup-authority.md:51` — "Running
  owners retain the exact native `jetstream.ConsumeContext` returned at the delivery commit point."; `:63` — "Client
  does not catalog, replace, rediscover, drain, or delete component-owned children." The consumer is graph-ingest's,
  not natsclient's.
- **ADR-094**, `docs/adr/094-boot-only-composition-and-observable-rule-activation.md:128-131` — "The owner that starts
  a managed JetStream consumer or core NATS subscription retains its exact returned handle, invokes native Drain during
  Stop, and waits for authoritative closure before canceling Start-owned work. Graceful paths do not substitute
  `ConsumeContext.Stop` or `Unsubscribe`, both of which can discard queued delivery." Any fix that retires a zombie
  loop (case a) or replaces a handle is bound by it.
- **ADR-113**, `docs/adr/113-start-context-bounds-start.md:62-64`: work after `Start` returns runs under the one run
  context `Start` creates and `Stop` ends.
- **ADR-109**, `docs/adr/109-ready-after-boot.md:119-120`: `Healthy` is false while a named stuck state holds; `/readyz`
  reads it.
- **ADR-088** (`docs/adr/088-readiness-is-per-producer-aggregation-is-the-consumers.md:117-126`): `/readyz` reads
  `Healthy`, not the readiness envelope.
- `component/lifecycle.go:59-60` (one-shot instance; no in-process restart).

## 4. Same-class collision table (consumer liveness)

| Dimension | Evidence |
| --- | --- |
| Semantic class | "Is graph-ingest's input consumer delivering?" |
| Owners | graph-ingest (`consume.go:64-243`, owns the handle per ADR-095:51); natsclient's ownership goroutine (`owned_delivery.go:124-169`, waits on `Closed()` for joins only) |
| Catalogs | `c.consumers` (`consume.go:231`); `boundConsumers` (`readiness.go:34-66`); natsclient `internalClaims` (`stream.go:609-631`) and metrics tracking (`stream.go:711`) |
| Status | `backlogObservationFault` → `Health()` (`readiness.go:142-149`, `component.go:392-401`); readiness envelope (`readiness.go:93-101`); natsclient's metrics poll: on a failed `consumer.handle.Info` it deletes the consumer's gauges, counts a `consumer_info` error, and logs Warn once per transition, "JetStream consumer state unavailable; its gauges are removed until it answers" (`natsclient/jetstream_metrics.go:317-335`); none reads `Closed()` |
| Lifecycle | bound once in `Start`; drained in `Stop` (`component.go:574-587`); no consumer rebind anywhere (`git grep -n -i "rebind" -- '*.go' ':!*_test.go'`: only `graph/readiness/watcher.go:84-87`, `:191`, a KV watcher, not a consumer) |
| Ownership | durable name per subject, shared across processes (`consume.go:102`); one local owner per client (`stream.go:609-631`) |
| Readers | readiness tick (5 s); `Health()`; `/readyz`; natsclient's metrics poll (`jetstream_metrics.go:317`), which runs for as long as the consumer is tracked: in case (a) the loop never closes, so it keeps polling and failing; in d, e and f the ownership goroutine forgets the consumer's metrics when `Closed()` fires (`owned_delivery.go:124-169`, release at `stream.go:678-683`) |
| Writers | `setupJetStreamConsumer` only (allowlist `consumerpolicy_test.go:40-48`) |
| Recovery | consumer: none in-process; process restart; file streams by nats.go (probe c). Stream: created at boot by `config/streams.go:328` `EnsureStreams` (no production caller here) or on demand by `natsclient/stream.go:147` `EnsureStream`; graph-ingest never (`consume.go:133`); after a broker restart only an outside producer recreates it (SemSource, #15 body) |

Closest existing instance of the shape "detect a lost subscription, report it, and re-establish it": the message
logger retries a failed subscribe every 100 ms and reports `subject_reconciliation_degraded` through its health
(`service/message_logger.go:254`, `:822-847`; `service/doc.go:345-350`). A second instance: the readiness KV
watcher keeps trying to bind, "rebinding after any failure" (`graph/readiness/watcher.go:188-191`, delay `:84-87`).
Whether a design adopts either is the design's to say.

## 5. Adopter seam inventory

Who carries it: whoever runs graph-ingest with a memory input stream (SemSource is the named one), and any product
that calls `ConsumeStreamWithConfig` for its own component.

1. **What they must know today:** (i) after a broker restart, a running graph-ingest on a memory stream stops
   applying input; (ii) only a process restart recovers it, so their supervisor must restart on `/readyz`, not
   `/healthz`; (iii) if anything recreates the durable consumer first, even that signal goes green while nothing is
   applied. Three items: a design finding by the contract's rule.
2. **If they do nothing** (memory stream recreated by their producer, section 2.1a): published input is accepted by
   the stream and never applied; `/readyz` stays 503 and the
   log carries `graph-ingest readiness: outstanding-work read failed` every 5 s (`readiness.go:133-137`). In cases e
   and f, nothing at all.
3. **Where they find out:** `/readyz` and graph-ingest's readiness log line in cases a and d; in case a also
   natsclient's Warn "JetStream consumer state unavailable; its gauges are removed until it answers", once, and a
   `consumer_info` error count rising each poll (`natsclient/jetstream_metrics.go:317-335`); nowhere in e and f.
   The docs say the opposite (`docs/contract.md:79`).
4. **What they should have to know:** that a memory stream loses unapplied input and they re-publish, which is what
   the contract already tells them. The gap between 1 and 4 is items (i) to (iii).

## 6. Open evidence questions

1. Does SemSource's deployment restart the process on `/readyz` failure? Not in `docs/inventory-scope.md`'s listed
   questions for semsource; an owner question, not read.
2. Is more than one graph-ingest process on one subject (sharing the durable consumer, `consume.go:102`) a supported
   deployment? Partly answered: it is already outside the guard's one-writer-per-key assumption
   (`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/notes/4.3.md:65-66`), and natsclient refuses a
   second local owner of the same consumer on one client (`natsclient/stream.go:621-626`), so two processes is the
   only way to share it. The other real-world trigger for cases e and f is an operator recreating the consumer by hand
   (`nats consumer add`).
3. Does any producer delete and recreate its stream while the server stays up (case d)? Not measured.
4. Clustered JetStream behaviour (consumer moves, leadership change) is not probed.
