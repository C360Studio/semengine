# Inventory: fix-740-warm-restart-replay

base: 96d047a6 (claim commit on origin/main d1a45753)
Track: design. Issue #740 (milestone v0.1.0-alpha.8; owner ruling 2026-11-24: blocks the alpha.8 tag).
Library versions read: nats.go v1.55.0 (`go.mod:18`), nats-server v2.15.3 source (`go.mod:17`, used by tests).
Test image: `internal/harness/natsfixture/nats-image:6` — `nats:2.15.0-alpine@sha256:ac8f88a6…`. Reporter's broker:
2.14.10. Every library statement below is a source read at the versions named, **not probed**, unless it says so.

## 1. The claimed gap

Claim: on a warm restart every ENTITY_STATES reader replays the whole bucket at once over the one connection,
nothing bounds the total, and a Start that runs while the connection is lost fails the service.

### 1.1 Full replays of ENTITY_STATES in production code

Searched: `git grep -n 'WatchAll\|\.Watch(\|WatchFiltered' -- '*.go' ':!*_test.go'`; callers by
`git grep -n 'follower.Open\|graphview.New'`.

| # | Reader | Watch | Options | What it does with the replay | Readiness tied to it |
|---|---|---|---|---|---|
| R1 | graph-index | `processor/graph-index/component.go:236` — `f, err := follower.Open(ctx, follower.Config{` → `graph/follower/follower.go:236` — `watcher, err := f.reader.WatchAll(ctx)` | none (full values) | Deliver keeps only key and revision; the lane reads the entity again: `processor/graph-index/index.go:211` — `entry, err := c.entities.Get(ctx, entityID)`. `e.Bytes` only feeds `CountInput` | follower QueryReady: `index_not_ready` until the floor reaches the snapshot target |
| R2 | graph-lexical | `processor/graph-lexical/component.go:184` — `f, err := follower.Open(ctx, follower.Config{` | none | applies `Value` (inline text): `processor/graph-lexical/component.go:240` — `read, ok, err := c.apply.deliver(e)` | same follower gate |
| R3 | graph-clustering | `processor/graph-clustering/component.go:193` — `f, err := follower.Open(ctx, follower.Config{` | none | decodes `Value` into the maintainer (`:273-299`) | same follower gate |
| R4 | graph-embedding (tier 1) | `processor/graph-embedding/component.go:324` — `f, err := follower.Open(ctx, follower.Config{` | none | hop 1 reads current truth, not `Value` (`processor/graph-embedding/hop1.go:26-31` doc; `:33` — `c.delivered.Store(e.Revision)`) | same follower gate |
| R5 | websocket | `output/websocket/state.go:113` — `view, err := graphview.New(reader, c.decodeState, graphview.WithHooks(graphview.Hooks{` → `pkg/graphview/view.go:235` — `watcher, err := v.source.WatchAll(watchCtx)` | none | keeps a copy of every value: `output/websocket/state.go:136` — `return storedState{raw: bytes.Clone(value), created: meta.Created}, true, nil` | upgrades refused 503 until caught up (`state.go:157-163` `appliedStuck`); reopens the view after a loss (`superviseView`, `state.go:64-89`) |
| R6 | graph-ingest | `processor/graph-ingest/component.go:454` — `if err := c.startEntityStateGuard(ctx, c.entityBucket); err != nil {` → `processor/graph-ingest/poison_inventory.go:272` — `watcher, err := bucket.Watch(ctx, ">")` | none | synchronous full sweep **inside Start** into the poison inventory, then Stop | Start fails if the watch closes before the end-of-snapshot marker (`poison_inventory.go:293-302`) |

Not a full replay of ENTITY_STATES: `pkg/lifecycle/manager_query.go:223` — `watcher, err := bucket.Watch(ctx,
reg.workflow.EntityIDPattern)` (a workflow pattern, not composed by the stack or by SemSource's list below).

Other full-bucket watches on the same connection at boot (not ENTITY_STATES, still on the one connection):
`graph/embedding/storage.go:570` — `watcher, err := s.index.Watch(ctx, jetstream.AllKeys)` (ScanFailed, inside
graph-embedding's Start); `graph/embedding/worker.go:315` — `watcher, err := w.storage.index.Watch(watchCtx,
jetstream.AllKeys)`; `graph/embedding/vector_cache.go:91` — `return s.index.Watch(ctx, jetstream.AllKeys)` (all
EMBEDDING_INDEX); `processor/graph-query/community.go:87` — `view, err := graphview.New(reader, decodeCommunityEntry)`
(COMMUNITY_INDEX).

Count, the issue's list vs today's tree: the issue names R1, R2, R5 (plus R3, R4 when enabled). R6 is not in the issue.
The tier-0 composed stack runs R1, R2, R3, R5, R6 on one connection; with tier 1, R4 and three EMBEDDING_INDEX watches
are added.

### 1.2 Where the replay sits in Start (ordering facts)

- Each follower component starts `f.Run` **inside its own Start**, before its last Start step:
  graph-index `processor/graph-index/component.go:273` — `if err := f.Run(runCtx); err != nil {` then `:287` —
  `return c.subscribeQueries(ctx)`; graph-lexical `:218` then `:224`; graph-embedding `:375` then `:436`;
  graph-clustering `:238`. Start does not wait for the replay to end; the replay runs on while the component's own
  remaining Start steps, and other components' Starts, use the connection.
- websocket starts its view supervisor under its run context: `output/websocket/websocket.go:209` —
  `c.supervisorErr = c.superviseView(runCtx)`.
- The component manager starts store providers, then every other component **at once**:
  `service/component_manager_lifecycle.go:49` — `if err := cm.startPhase(ctx, providers); err != nil {`,
  `:52` — `if err := cm.startPhase(ctx, consumers); err != nil {`, `:79` — `if err := cm.startComponent(ctx, record);
  err != nil {`. Providers are `component.StoreProvider` (only `storage/objectstore/component.go:136` implements it),
  so graph-ingest and the followers share the second phase and their replays overlap.

### 1.3 What Start does when the connection drops

- natsclient subscribe while reconnecting: `natsclient/client.go:1176` — `return nil, errs.WrapTransient(ErrNotConnected,
  "Client", operation, "register with server")`; `natsclient/client.go:52` — `ErrNotConnected = fmt.Errorf("not
  connected to NATS: %w", errs.ErrConnectionLost)`. ADR-112 `docs/adr/112-natsclient-contract.md:243-244`: Subscribe
  returns `ErrNotConnected` at once while reconnecting (P15); KV calls wait for their context. This matches the issue's
  graph-query and graph-index Start errors and the `INCOMING_INDEX` `context deadline exceeded`.
- A failed component Start: errors joined, all launched components rolled back
  (`service/component_manager_lifecycle.go:27-30` doc). Search for a retry of a transient Start failure:
  `git grep -n 'IsTransient\|StartTimeout\|start_timeout\|startBudget' service/*.go ':!*_test.go'` → only
  `:120`, which wraps a late success, not a retry. **No retry exists.** The errors are already classified transient.
- Only existing bounded retry at boot: the follower's bucket wait, `graph/follower/follower.go:163` — `wait :=
  resource.NewWatcher(graph.BucketEntityStates, func(ctx context.Context) error {` (`internal/resource`).
- After boot, a follower whose watch channel closes declares the feed lost until restart:
  `graph/follower/follower.go:254` — `f.loseFeed("the watch closed")`; `:315`. The composed stack recovers from a
  quick broker restart with no lost feed (`openspec/specs/composed-stack/spec.md:85-99`,
  `TestSmoke_BrokerRestartUnderRunningStack`).

### 1.4 Connections

- One `natsclient.Client` per process reaches every component: `component/dependencies.go:43` —
  `NATSClient      *natsclient.Client        // NATS client for messaging`. One nats.go connection and one JetStream
  context per Client: `natsclient/client.go:506` — `conn, err := nats.Connect(m.urls, opts...)`, `:525` —
  `js, err := jetstream.New(conn, jetstream.WithPublishAsyncErrHandler(m.asyncPublishErrHandler))`. A Client connects
  once; recovery is a new Client (`client.go:446-462` doc).
- Every follower opens its reader **and** a second status reader on the same Client (`graph/follower/follower.go:171`,
  `:175`); both share the connection. The catalog reader passes watch options straight to nats.go:
  `graph/kvcatalog/kvcatalog.go:275` — `WatchAll(ctx context.Context, opts ...jetstream.WatchOpt)
  (jetstream.KeyWatcher, error)`.
- Connection options natsclient sets (`natsclient/client.go:400-424`): reconnects, reconnect wait, ping, flusher
  timeout, timeout, drain timeout, handlers, auth, name. No pending-limit, buffer or consumer setting. ADR-112
  promises nothing about KV watches: `grep -n -i watch docs/adr/112-natsclient-contract.md` finds no promise line
  for watch timing, bounds or throughput.

### 1.5 How nats.go builds a KV watch

Source read at v1.55.0, not probed; not promised by natsclient.

- `jetstream/kv.go:1335` — `subOpts := []nats.SubOpt{nats.BindStream(kv.streamName), nats.OrderedConsumer()}`: an
  ordered **push** consumer through the legacy subscribe. Without `IncludeHistory`: `:1337` —
  `nats.DeliverLastPerSubject()`. `MetaOnly`: `:1343` — `nats.HeadersOnly()`. `ResumeFromRevision`: `:1346` —
  `nats.StartSequence(o.resumeFromRevision)`.
- Delivery: an async callback that blocks on a 256-entry channel, `jetstream/kv.go:1277` —
  `w := &watcher{updates: make(chan KeyValueEntry, 256)}`.
- Ordered consumer config, `js.go:1800-1809`: `FlowControl = true`, `AckNonePolicy`, `MemoryStorage`, heartbeat 5 s.
- Flow-control reply is sent only once the handler has delivered up to the flow-control point:
  `js.go:2489` — `if sub.getJSDelivered() >= jsi.fcd {`.
- Recreated after a gap from the last stream sequence plus one: `js.go:2278` — `sub.resetOrderedConsumer(jsi.sseq + 1)`
  (`resetOrderedConsumer` at `js.go:2313`, `DeliverByStartSequencePolicy`). Whether a reconnected watch replays from
  the start, as the issue says, is **not established**; this source read suggests it resumes.
- Client-side per-subscription pending limits: `nats.go:5945-5949` — 500,000 msgs, 64 MiB.
- Available watch options: `jetstream/kv_options.go:29` `IncludeHistory`, `:41` `UpdatesOnly`, `:52` `IgnoreDeletes`,
  `:61` `MetaOnly`, `:70` `ResumeFromRevision`.

### 1.6 Server slow-consumer behaviour

Source read at nats-server v2.15.3, not probed; 2.14.10 and 2.15.0 not read.

- Per-client outbound limit default: `server/const.go:101-102` — `MAX_PENDING_SIZE = (64 * 1024 * 1024)`; write
  deadline `:132` — `DEFAULT_FLUSH_DEADLINE = 10 * time.Second`.
- Over the limit the server closes the connection: `server/client.go:2690` — `if c.kind == CLIENT && c.out.pb >
  c.out.mp {` … `:2703` — `c.Noticef("Slow Consumer Detected: MaxPending of %d Exceeded", c.out.mp)`, `:2704` —
  `c.markConnAsClosed(SlowConsumerPendingBytes)` (the issue's log line).
- Push consumer flow-control window: `server/consumer.go:607` — `JsFlowControlMaxPending = 32 * 1024 * 1024`; set at
  `:1870`; starts at limit/16 (`:5855`) and doubles on each flow-control reply up to the limit (`:6088-6091`); sending
  stops while unacknowledged bytes exceed the window (`:5632`).
- Read together, not probed: each watch may have up to 32 MiB unacknowledged, and the windows of several watches on
  one connection add up against the single 64 MiB outbound limit. Bytes count against that limit while they wait for a
  slow socket. The issue's "about 690 MB queued" is stream `num_pending`, not outbound bytes.

## 2. Every current spelling of the fact being modeled

Fact: "how a process's ENTITY_STATES readers receive the stored state at boot". Spellings: the follower (R1–R4,
`graph/follower`), graphview (R5, `pkg/graphview`), graph-ingest's guard sweep (R6), each with its own nats.go watch.
The total boot load across watches on one connection is spelled nowhere: no bound, no ordering, no count
(searched `git grep -n -i 'one connection\|per connection\|shared connection\|second connection' docs/adr
openspec/specs` → no hit on this question; `git grep -n -i 'max_pending\|maxpending' -- ':!openspec/changes/archive'`
→ no production hit).

## 3. Adjacent claims

Specs: `openspec/specs/entity-state-follower/spec.md` (Requirements at `:10`, `:22`, `:34`, `:64` initial build at
snapshot revision, `:75` lost feed, `:193` Entry carries the delivered value); `openspec/specs/graph-view-subscription/spec.md:12`
(Run holds one watch), `:145` (a view opens only its own watch), `:153`; `openspec/specs/service-lifecycle/spec.md:358`,
`:374`, `:393` (failed Start rolls back), `:490`; `openspec/specs/composed-stack/spec.md:12`, `:44`, `:73`, `:85`;
`openspec/specs/output-websocket/`, `openspec/specs/transport-client/`.

ADRs: ADR-081 (graph view: one watch per view, slow-consumer motive `docs/adr/081-graph-view-subscription.md:52-58`);
ADR-108 D4 (`docs/adr/108-processor-shell.md:119-120`, one `WatchAll` per follower) and D7 (one projection per producer,
`:153`); ADR-109 (ready after boot, `index_not_ready` as retry); ADR-112 (natsclient promises); ADR-113 (Start bounds
Start); ADR-058 (boot phases).

Open PRs (`gh pr list --state open --json number,title,changedFiles,files`): only #747 (draft, this claim, 0 files).

Open issues (searches: follower, graphview, natsclient, "slow consumer", WatchAll, replay, ENTITY_STATES, websocket,
max_pending, reconnect):

- Names the territory: #108 (graph-index rebuild on restart; measured 5,000-entity restart; resume point undecided);
  #741 (docs: `graph/follower/follower.go:23` names nats.go v1.54.0, the same doc comment this change may touch);
  #477 (graphview adoption, ADR-081); #109 (follower and processor shell; appears implemented — status to check);
  #602 (ENTITY_STATES generation next to indexed_revision); #554 (follower integration test waits); #110 (readiness
  legacy fields).
- Brief-named, not on this territory: #734 (graph-embedding failed-state ordering, `failed.go`), #712 (graph-query
  15 s wait), #659 (fusion seed read `response_too_large`).
- Names the word, not the package: #195, #196, #284, #641, #502, #83, #141, #300, #68, #60, #24, #134, #111, #702,
  #704, #730, #742, #9, #11.
- Open `class:flake` issues: none. alpha.8 milestone open: #740, #11.

Consumer ask: C360Studio/semsource#351 (`max_pending: 536870912` workaround).

## 4. Who it is for

No new surface is proposed at the inventory gate. Any new exported symbol, option or config key the design adds must
name its caller here.

## 5. The problem shape

Shape: bounded concurrent startup load on a shared resource, plus retry of a transient failure within a start bound.
Closest instances: `internal/resource` bounded wait (`graph/follower/follower.go:163`); the component manager's phase
order (`service/component_manager_lifecycle.go:49-53`); ADR-081's one shared watch per view (`pkg/graphview`); the
meta-only narrow watch already used for key listing (`natsclient/kv.go:656` — `watcher, constructErr :=
reader.WatchFiltered(ctx, []string{pattern}, jetstream.IgnoreDeletes(), jetstream.MetaOnly())`).

## 6. Pin decision records

This change carries no package from the pin; category 6 is not applicable. The local ADRs that govern the touched
packages are listed in §3.

## Facts asked for

- Shared follower: none exists. A follower is per producer (`Config.Key`, one watermark, one Deliver, one damage
  record; ADR-108 D7). `graphview.View` shares one watch among the subscribers of one component; each component builds
  its own view (R5, graph-query, the vector cache).
- Narrower watch: R1 and R4 do not apply `Value` (they Get again); R2, R3 and R5 use it. `MetaOnly` sets
  `HeadersOnly` (`kv.go:1343`) and would change `Entry.Bytes`, which feeds `CountInput`
  (`entity-state-follower` spec `:193` requires Value to be the delivered value). `IncludeHistory` on ENTITY_STATES
  replays the same set, since `graph/kvcatalog/kvcatalog.go:86` — `entityStates.History = 1`. `ResumeFromRevision`
  exists; no stored resume point exists (#108). The KV watch is always nats.go's legacy ordered push consumer (§1.5);
  the jetstream package's pull-based ordered consumer is not used by KV watches.

## Tests

- Largest replay in a test: 500 entities (`processor/graph-index/lifecycle_integration_test.go:145`, `:201`;
  `query_integration_test.go:351`); follower `follower_test.go:400` 100.
- Slow consumer: only the client-side kind, `natsclient/client_async_error_integration_test.go:25` —
  `func TestIntegration_ClientHandleErrorLogsObservedSlowConsumerDropCount(t *testing.T) {`. No test of the server
  outbound limit, a multi-watch replay or a large store.
- Composed stack: `internal/smoke/stack_integration_test.go:122` — `components: []string{"graph-clustering",
  "graph-gateway", "graph-index", "graph-ingest", "graph-lexical",` (+ graph-query, websocket); one Client
  `:250` — `client, err := natsclient.NewClient(strings.Join(cfg.NATS.URLs, ","), natsclient.WithLogger(logger))`;
  warm restarts at `internal/smoke/smoke_integration_test.go:844`, `:983`, `:1077` with small stores.

## Same-class collision table (startup-load / replay coordination)

| Dimension | Evidence |
|---|---|
| Semantic class | boot-time delivery of stored state to ENTITY_STATES readers on one connection |
| Owners | `graph/follower` (R1–R4), `pkg/graphview` (R5), graph-ingest guard (R6) |
| Catalogs | `graph/kvcatalog/kvcatalog.go:78-86` (ENTITY_STATES spec, History 1) |
| Status | follower envelope / QueryReady; websocket appliedStuck; graph-ingest Start error |
| Lifecycle | R1–R4 and R5 started in Start, run under the run context; R6 inside Start; no reopen for followers, reopen for websocket |
| Ownership | one watch per reader; no cross-reader coordination (searched §2) |
| Readers | the six above; harness `internal/harness/scorecard/measure.go:429` (test only) |
| Writers | graph-ingest only (AGENTS.md §2) |
| Recovery | nats.go ordered-consumer reset (§1.5); follower lost feed latches; websocket reopens; no Start retry |

## Intent check

No boundary moves (no port-set, tier or exclusion change). Not applicable.

## Adopter seam inventory

A product `main` composing several graph readers on one `natsclient.Client`. SemSource at 52e3f67a: one Client from
`connectNATS` (`cmd/semsource/run.go:463-477`), composes graph-embedding, graph-index, graph-lexical,
graph-clustering, websocket (`run.go:305-313`); `configs/nats.conf:10` — `max_pending: 536870912`.

1. What must they know: that boot replay load grows with store size × reader count on one connection; that the
   server's `max_pending` must exceed it; that a Start failure from a lost connection is not retried.
   That is three items, so by the contract it is a design finding.
2. If they do nothing: warm restart over a large store fails Start every time (issue: 5 of 5 at 230 MB).
3. Where they find out: a server log line and a boot error naming `not connected to NATS: connection lost` or a
   KV `context deadline exceeded`, which names framework internals, not the cause.
4. What they should have to know: nothing. Gap: the framework asks the operator to predict a buffer size that the
   framework could observe.

## Residual / skipped

- No probe was run. Every nats.go and nats-server statement is a source read at v1.55.0 / v2.15.3; 2.14.10 and
  2.15.0 server source were not read.
- Not reproduced. Not established: whether it reproduces inside the Docker network, and the store size where it starts.
- Not traced: per-component Start deadlines and the `INCOMING_INDEX` timeout's own bound; whether #109 is closable.
- SemSource was not read beyond its imports, component list and nats.conf (inventory-scope).
