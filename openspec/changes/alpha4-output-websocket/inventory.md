# Inventory: alpha4-output-websocket (port `output/websocket` over applied state)

base: fb40de2daa8bd2c46088a0955fbd028534ffb37c (claim commit on main 8ad451bab36f047f5aa793f1773de128d5d22978; empty diff)
pin: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128 (read from `git archive 8b99efe9` into a temp copy)
sister reads: semsource bbfb3c62036fb6c2bddb1e68addcec27be05d7de, semboids 8c03cc53836ced93a5df7064473c63ff144e64f1
(gh api ?ref=)
question: what exists that a port of `output/websocket` from a NATS-subject input to applied state (#34; 03B D14, D4a)
must know about, and who carries the change.
track: design (new package, new spec capability expected); this file is inventory only.

## 0. Pin probe

Measured on the pin copy, go1.26.4 darwin/arm64:

- `go test ./output/websocket/ -count=5 -cpu 1 -shuffle on`: `ok` (8.6 s), `-test.shuffle 1794400503030996000`.
- `go test ./output/websocket/ -count=1 -race`: `ok` (3.1 s); 53 `--- PASS`, 0 `--- FAIL`.
- Unit tests only. Not run: the four `//go:build integration` files (`metrics_integration_test.go`,
  `path_integration_test.go`, `websocket_integration_test.go`, `websocket_nats_integration_test.go`), and SemEngine's
  lifecycle suite, as in every change-4/5 port and graph-clustering
  (`openspec/changes/archive/2026-11-10-setup-04a-07-substrate-seam-graph-clustering/inventory.md:17-23`).
  The pin's own `lifecycle_owner_test.go` and `lifecycle_authority_test.go` ran in the unit runs.

Test-text rules, measured by grep on the pin copy:

| Rule | Count | Where |
| --- | --- | --- |
| `time.Sleep` | 48 | `websocket_test.go` 21, `metrics_integration_test.go` 12, `websocket_integration_test.go` 11, `websocket_nats_integration_test.go` 4 |
| `Skip` | 4 | `websocket_nats_integration_test.go:104`, `:251`; `websocket_test.go:1012`, `:1128` (all `testing.Short`) |
| build tags other than `integration` | 0 | |
| `t.Parallel` | 0 | |
| `natsclient.NewTestClient(` | 18 | 6 each in the three NATS-using integration files (matches #195) |
| fixed ports bound or dialled | 26 lines in `websocket_test.go` (22 bind, 4 dial); 2 more elsewhere | ports 8080-8094, 8901-8904, 18081-18083; e.g. `:290` `mustNewOutput(t, 8901, ...)`, `:320` `Host: "localhost:8901"`; also `passthrough_test.go:26` (8099), `websocket_nats_integration_test.go:24` (8082) |
| pick-then-release ports | 9 | `net.Listen` then close: 8 on `127.0.0.1:0`, e.g. `constructor_test.go:53`, `lifecycle_authority_test.go:30`, `lifecycle_owner_test.go:45`, `metrics_integration_test.go:384`, `websocket_integration_test.go:434`; and `websocket_test.go:1173` — `net.Listen("tcp", ":0")`, an every-address pick (#390) |
| dial `localhost` (dual-stack) | 13 | e.g. `metrics_integration_test.go:90` — `wsURL := fmt.Sprintf("ws://localhost:%d/ws", port)`; 4 in `metrics_integration_test.go`, 4 in `websocket_test.go`, 5 in `websocket_integration_test.go` (`path_test.go:102` holds `localhost` in a config string, not a dial) |
| production hooks set by tests | 6 | `lifecycle_owner_test.go:100,150,203,206,214,224` (`requestHook`, `subscribeCore`, `waitForInput`, `consumeStream`, `waitJSClosed`) |

Also relevant to proving tests: `websocket.go:612` — `if w.port < 1024 || w.port > 65535 {` (Initialize refuses port 0,
so the component cannot bind an ephemeral port itself).

## 1. The claimed gap

- `output/websocket` is absent on main: `ls output` and `ls input` → "No such file or directory"
  (no ACME caller has been ported); `git grep -n 'output/websocket'`
  finds only docs: `docs/admission-ledger.yaml:4877` (pkg/buffer row names it as a consumer),
  `docs/adr/108-processor-shell.md:181`, `docs/tier1-cross-check.md:39,61,125`, `docs/inventory-scope.md:24`,
  `scripts/port-status.sh:99`.
- No ledger row: `grep -n 'source_path: output/websocket\|source_path: pkg/acme\|source_path: input/websocket'
  docs/admission-ledger.yaml` → none.
- `github.com/gorilla/websocket` is absent: `grep -n gorilla go.mod` → none. Pin: `go.mod:10` —
  `github.com/gorilla/websocket v1.5.3`.
- **Issue #34's dependency list, checked:**
  - `pkg/tlsutil` is **ported**, at `internal/tlsutil` (ledger `docs/admission-ledger.yaml:711-739`, disposition
    `adapt`). `internal/tlsutil/tlsutil.go:19` —
    `func LoadServerTLSConfig(cfg security.ServerTLSConfig) (*tls.Config, error) {` applies MTLS. The pin's websocket
    calls two loaders main lacks: `LoadServerTLSConfigWithMTLS` (dropped, #221; ledger `:718`) and
    `LoadServerTLSConfigWithACME` (deferred; ledger `:723-724`, `:729-733`; the ledger text still says "deferred to
    change 5", which shipped as graph-embedding without a websocket, so it is stale).
  - `pkg/security` is ported: `pkg/security/config.go`; `component/dependencies.go:47` —
    `Security        security.Config           // Platform-wide security configuration`.
  - `pkg/lifecyclecleanup` is ported (ledger `:1961-1981`).
  - `pkg/buffer` is ported (PR #587, ledger `:4873-`). Its `consumer_purpose` names "the output/websocket pending-ack
    ring (#34)" as a reader.
- On main nothing publishes an applied entity change on a subject: `git grep -n '\.Publish(\|PublishMsg('` in
  `processor/graph-ingest` non-test → only `processor/graph-ingest/readiness.go:376` —
  `if err := c.statusPublisher.Publish(ctx, status); err != nil {` (readiness status). Applied state exists only as KV.

## 2. Every current spelling of the fact (an applied change delivered to a reader)

| Spelling | Where | Shape |
| --- | --- | --- |
| Per-reader KV watch, operator SSE | `service/message_logger_http.go:45` — `mux.HandleFunc("GET "+prefix+"kv/{bucket}/watch", ml.handleKVWatch)`; `service/message_logger_kv_watch.go:138` — `watcher, err = kv.WatchAll(watchCtx)` | one watch per SSE client (the ADR-081 trap; #477 lists it "Convert"); wire type `KVWatchEvent{Bucket,Key,Operation,Value,Revision,Timestamp}` (`:49-58`) plus an `initial_sync_complete` marker |
| Shared view | `pkg/graphview/view.go:480` — `func (v *View[T]) SnapshotAndSubscribe(ctx context.Context) (Snapshot[T], *Subscription[T], error) {`; `:516` `Subscribe` | one `WatchAll`, decode once, atomic snapshot then coalesced `Delta{Op,Key,Value,Revision,Created,Err}` (`pkg/graphview/delta.go:41-56`), slow subscribers coalesce, never disconnect |
| Projection-owner follower | `graph/follower/follower.go:1-7` (package doc); used at `processor/graph-clustering/component.go:192` — `f, err := follower.Open(ctx, follower.Config{`, and graph-embedding `component.go:256`, graph-index | per-entry `Deliver`, `Complete` watermark, publishes readiness to `GRAPH_STATUS` (ADR-108 D4) |
| Second view over `COMMUNITY_INDEX` | `processor/graph-query/community.go:84-88` — `kvcatalog.OpenCatalogReader(ctx, c.natsClient, graph.BucketCommunityIndex)` then `graphview.New(reader, decodeCommunityEntry)`; the ledger `pkg/graphview` row names it as the reader in this tree (`docs/admission-ledger.yaml:3979-3980`); `processor/graph-embedding/query.go:16,183` imports graphview too (for `ErrNotReady`) | one view of the bucket already runs in graph-query; `.agents/skills/kv-or-stream/SKILL.md:61`: "Many local readers of one bucket share one `graphview.View` ..., never one `WatchAll` each" |
| Raw catalog reader | `graph/kvcatalog/kvcatalog.go:275-281` (`CatalogReader`: `Get`, `Watch`, `WatchAll`, `WatchFiltered`, `Status`); `:295` `OpenCatalogReader` | must-exist bind, read-only |
| Port declaration | `component/port_kv.go:6` — `type KVWatchPort struct {` (`Bucket`, `Keys`, `History`); graph-clustering and graph-embedding accept only all-keys, no-history `ENTITY_STATES` (`processor/graph-clustering/config.go:97`, `processor/graph-embedding/config.go:105`) | |
| Decoders | `graph/entity_predicate_contract.go:288` — `func UnmarshalEntityState(key string, data []byte, entity *EntityState) error {`; `graph/clustering/storage.go:132` — `func DecodeCommunity(key string, value []byte) (*Community, error) {` | refusals are classified (`graph_state_reset_required`) |
| Consumer-side (semboids) | ledger `pkg/graphview` row `:3978` names semboids `internal/api/graphstream*.go` at 8c03cc53 | app-side applied-state stream over graphview |
| Pin websocket (to be ported) | `output/websocket/websocket.go:209-214` `MessageEnvelope{Type,ID,Timestamp,Payload}`; `:1298-1329` `transformPayload` injects `timestamp`/`subject` | relays NATS input, not applied state |

Inherited second spellings inside the pin package (shape sweep 1):

- `websocket.go:1022` — `func (w *Output) admitRequest() bool {` with `releaseRequest`/`fenceRequests` (`:1035-1055`) vs
  main's `internal/httpadmission.Requests` ("metric.Server and the service manager's listeners use it").
- `websocket.go:1238` — `func (w *Output) handleNATSMessage(ctx context.Context, msg *natspkg.Msg) {` duplicates
  `handleNATSMessageData` (`:1216`); its only callers are tests (`passthrough_test.go:202`,
  `websocket_test.go:693,767,888`).
- `websocket.go:1590` — `func (w *Output) buildClientSnapshot() ...` and the inline copy in `pingClients`
  (`:1785-1796`).
- Path validation in four places: `:114` (Validate), `:377` (resolveConstructorPorts), `:618` (Initialize), `:727`
  (setupHTTPServer); and a fifth copy across repos at semsource `config/config.go:364` ("mirrors the semstreams
  websocket output's basic path" check).
- Counts kept twice: atomics `messagesSent`/`bytesSent`/`errors` (`:178-181`) and Prometheus `Metrics` (`:245-256`);
  main's `component.Lifecycle.CountInput`/`CountError` (`component/lifecycle_type.go:147,157`) would be a third.
- Lifecycle state machine: `websocket.go:155-170` (`running`, `lifecycleMu`, `lifecycleUsed`, `terminal`,
  `stopping`, `cleanupPending`, `startDone`, ...) vs `component.Lifecycle` (`component/lifecycle_type.go:27` —
  `type Lifecycle struct {`; ADR-108 D1: "the one way a SemEngine component implements `LifecycleComponent`").
- `waitForStream` (`:1177-1204`), a bounded poll of `js.Stream` (30 tries, 100 ms doubling to 2 s), is the package's
  own wait-for-input.

## 3. Adjacent claims on the territory

**Open issues** (`gh issue list --state open --search <term>`, terms `websocket`, `tlsutil`, `acme`, `gorilla`,
`pkg/buffer`, `graphview`, `applied state`, `change observation`, `passthrough`; hits read):

| Issue | Names the package how | Relevance |
| --- | --- | --- |
| #34 port-refactor: output/websocket — read applied state | title | the deliverable; matrix row, proving test, `adapt` row |
| #196 adoption: internal/lifecycleguard | body: "Change 5: ... `output/websocket/websocket.go:157`" | lifecycle copy (shape 2) |
| #195 adoption: natsfixture.Open | body: "`output/websocket` 18" | test client sites |
| #284 tlsutil: re-judge internal/tlsutil as framework API | body: pin callers `input/websocket`, `output/httppost` | the loaders this package calls; #284 is about products, not in-repo callers |
| #285 acme: renew_before silently falls back to 8h | "recorded for when ACME is ported" | lands if ACME lands (see ruling below) |
| #9 epic | comments 5950752741, 5950822861 (ACME deferral to "the first change that ports a caller ... (`output/websocket`)"); 6772865635 (alpha.4 ruling) | binding owner rulings |
| #477 adoption: pkg/graphview | lists the SSE KV watch and vector cache; not the websocket | the websocket is a candidate adopter not on its list |
| #84 epic: security primitives | surfaced by `acme`; authn/authz for engine HTTP servers | websocket has no auth (`doc.go:226`) |

Names the word, not the package: #15, #24, #109, #110 (from `applied state`).

**Owner rulings in force:**

- 03B D14 (`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:609-631`): the primitive is "a KV
  watch on `ENTITY_STATES` and `COMMUNITY_INDEX`"; `output/websocket` is "a tier-0 output component over applied
  state; it is a transport, not a UI"; SemSource's `graph.ingest.>` relay is "the pattern this row replaces".
- 03B D4a (`:301-331`): `class:port-refactor`; `adapt` row with `contract` citing the matrix row, `proving_tests`,
  `known_risks` naming the class.
- Matrix row (`:721`): "Change observation · `output/websocket` as a transport over applied state, not a UI and not a
  relay of ingest input"; proving test "harness: a websocket client receives applied entity changes".
- D15 (`:633-645`): one primitive, two transports (websocket for consumers, SSE KV watch for operators).
- #9 comment 5950822861 (clarifying 5950752741), in full: "the ACME loaders, their shutdown defect and the `Certificates`
  race land with the first change that ports a caller, change 5 (`output/websocket`)." Comment 5950752741 (2026-10-02)
  adds that the shutdown defect ("the stop can block inside lego's `Certificate.Renew`, which takes no context") "needs
  `Shutdown(ctx)` and a renewal test seam", and that all of it lands "under the background-work rule".
  This ruling binds this change: change 5 shipped as graph-embedding without the websocket, no caller has been ported
  since (`input/` and `output/` are absent on main), and #34's 2026-11-10 note says "No new ruling". The ledger
  `internal/tlsutil` row (`:723`, `:729-731`) still says "deferred to change 5", which is stale.
- #34 comment (2026-11-10): scheduled for alpha.4, "No new ruling"; "the applied-state message shape is a migration
  note".
- Owner 2026-11-02: a consumer break is a migration note.

**Open PRs** (`gh pr list --state open --json number,title,changedFiles,files`):

| PR | Files overlapping | Note |
| --- | --- | --- |
| #592 (draft, this claim) | none yet | |
| #588 (draft) graph-gateway, 50 files | `go.mod`, `go.sum` (adds `vektah/gqlparser/v2`), `docs/admission-ledger.yaml`, `docs/repository-map.md`; `internal/smoke/*` and `openspec/specs/composed-stack/spec.md` if this port adds a smoke step | further along (change archived on its branch); no websocket or subscription code (`gh pr diff 588 \| grep -i 'websocket\|gorilla'` → none) |

**Specs:** no current spec names change observation (`git grep -n -i 'change observation' openspec/specs` → none);
`docs/setup-plan.md:79` puts "change observation" at tier 0. No active OpenSpec change under `openspec/changes/`.

**Ledger rows touching the territory:** `pkg/graphview` (`:3973`, "the view layer of change observation (SETUP 03B
design D14, tier 0)"), `internal/tlsutil` (`:711`), `pkg/security` (`:401`), `pkg/buffer` (`:4873`),
`internal/lifecyclecleanup` (`:1961`).

**Guards that will apply:** the JetStream consumer allowlist `internal/harness/contract/consumerpolicy_test.go:40`
(pin's equivalent lists `output/websocket/websocket.go` at `natsclient/consumer_policy_callsite_test.go:265`);
the metric-registry rule (`metric/registry.go:29` — `func RegisterOrGet[C prometheus.Collector](...)`; the pin's
`registry.RegisterCounterVec` etc., `websocket.go:345-354`, do not exist on main); strict config decode
(`component/validation.go:183` `SafeUnmarshal` refuses unknown keys); `component.Factory` takes no instance name
(`component/registry.go:21`); `composition/catalog.go:39` — `"websocket":      "WebSocket Output",`; the registry's
admission of network port claims (`component/registry.go:631-669`: `checkSelfClash` and
`claimsClash` refuse clashing exclusive network claims, so the `NetworkPort` output is checked against other claims);
`component/port_codec_test.go:150` refuses the legacy `websocket-server` port kind.

## 4. Who it is for

Not applicable at the inventory stage: no new symbol is proposed. Pin surface audit (list a/b/c), by pin-wide search
(`grep -rn 'output/websocket"'` → only `componentregistry/register.go:18`) and consumer symbol use (semsource
`cmd/semsource/run.go:54,314` and semboids `componentregistry/register.go:8` import `Register` only; both compose by
JSON):

(a) Exported, no reader outside the package: `NewOutput` (`:361`), `NewOutputFromConfig` (`:416`),
`ConstructorConfig` (`:60`), `DefaultConstructorConfig` (`:75`), `DefaultConfig` (`:89`), `Config` (`:44`) and
`Config.Validate` (`:113`), `Output` (`:126`; returned by the factory), `MessageEnvelope` (`:209`, the wire contract
in the README), `PendingMessage` (`:217`), `Metrics` (`:245`), `DeliveryMode` and its constants (`:34-41`),
`DeclarePorts` (`:1842`), `CreateOutput` (`:1873`). Read only through `Register`'s registration: `CreateOutput`,
`DeclarePorts`.

(b) Config parsed but not honoured: `delivery_mode` takes any string (`:1879` — `deliveryMode := DeliveryAtMostOnce`,
`:1881` — `deliveryMode = cfg.DeliveryMode`, no check); `at-least-once` is accepted but nothing is redelivered (below).
Dead state: `PendingMessage.Retries`, `.SentAt`, `.Subject`, `.Data` are never read; `clientInfo.messagesSent` is
never written; `clientInfo.lastPing` is written (`:1388`, `:1411`) and never read; `pendingBuffer` is written
(`:1654` — `if err := i.pendingBuffer.Write(pending); err != nil {`) and never read.

(c) Documented, not implemented:

- `README.md:186` "**Use when**: Critical events, alerts, or state updates that must not be lost." vs `:184` "Future
  support for retries", `:534` "Retry logic not yet implemented"; `waitForAck` (`:1697-1720`) counts a nack or timeout
  and drops it; the JetStream message is acked after fan-out whatever the clients did (`:1158-1162`).
- "slow" backpressure (`README.md:221-227`) vs `:1490` — `// For now, just log that we received a slow signal` (it
  logs nothing).
- `doc.go:57-80` describe `WriteTimeout`/`PingInterval` settings; none exist (hard-coded 5 s send `:1615`, 10 s
  write deadline `:1743`, 60 s read `:1423`, 30 s ping `:1771`).
- `doc.go:229` "No SSL/TLS (use reverse proxy or add to component)" vs TLS code at `:752-786`.
- `doc.go:120-123` metric names (`websocket_clients_total` ...) vs real `semstreams_websocket_*` (`:266-336`).
- `doc.go:134` "streamkit/errors"; `MessageEnvelope` comment "matches the protocol defined in input/websocket_input"
  (`:205`; `input/websocket` is not admitted).

## 5. The problem shape

Shapes: (i) serve many local readers one shared KV-bucket view, snapshot plus changes; (ii) a component serving
HTTP and holding hijacked connections across Stop; (iii) fan-out to slow clients.

- (i) Closest instance: `pkg/graphview` (ADR-081), production reader `graph/embedding/vector_cache.go:64` —
  `view, err := graphview.New(indexSource{c.storage.index}, decodeVector)`. `CatalogReader.WatchAll`'s signature
  (`graph/kvcatalog/kvcatalog.go:278`) matches `graphview.WatcherSource` (`pkg/graphview/view.go:42`) as written
  (not compiled here).
- (ii) Two instances on main: own listener (`metric/handler.go:163` —
  `listener, err = net.Listen("tcp", httpServer.Addr)`; `service/pprof.go:49`) with `httpadmission`; and routes
  mounted on the service manager's server by a component's `RegisterHTTPHandlers`
  (`service/service_manager_http.go:280-284`; #588's gateway uses it). The pin binds its own listener from a
  `NetworkPort` output (`websocket.go:789`, `net.Listen`).
- (iii) Pin: per-message goroutine per client, waits for all (`:1525-1562`), slow client dropped after 5 s;
  graphview: coalesce per key, never disconnect (`pkg/graphview/subscription.go:9-16`).

For `COMMUNITY_INDEX`, graph-query already holds the one view (`processor/graph-query/community.go:84-88`); the design
must say whether a websocket watch of that bucket shares that view or is a second `WatchAll` against
`.agents/skills/kv-or-stream/SKILL.md:61`.

Whether the port adopts each is design work.

## 6. The pin's decision records

Fetched from the pin copy `docs/adr` (85 records); `grep -l -i -e 'output/websocket' -e websocket -e MessageEnvelope
-e DeliveryAtLeastOnce -e passthrough` → 036, 044, 047, 049, 056, 071, 092 (mentions only); read by title as well.

| ADR | Decides | Governs this package? |
| --- | --- | --- |
| ADR-081 Graph View Subscription (on main `docs/adr/081-...`) | one shared `WatchAll`, snapshot+delta, per-subscriber at-most-once coalescing, fail-closed loss; per-client watchers are the trap (#579) | yes for an applied-state input |
| ADR-095 One-shot running lifecycle with retained failed-start cleanup (on main) | Stop fences admission, drains native `ConsumeContext`, failed Start retains `cleanupPending` | yes; the pin's Start/Stop (`:631-997`) implement it |
| ADR-090 Authoritative current state and materialized views (pin only; `docs/adr/` on main has no 090, though ADR-099, 110 and 111 cite it, and PR #588 cites decision 6 as governing) | Decision 6: "Remote application reads use the required GraphQL gateway's admitted operations; GS-12 makes that gateway conformant. ... Raw KV is an owner/operator seam, not an application fallback." Decision 2: "SemStreams does not adopt event sourcing or a general CQRS/read-model runtime." Decision 9: "Shared runtime mechanics are introduced only after at least three surviving owners need the same behavior and a prototype reduces total code and adopter knowledge." | yes: a websocket that watches `ENTITY_STATES` or `COMMUNITY_INDEX` is a remote application read of raw KV. 03B D14 (2026-10-01) is the later owner ruling; the design must say whether D14 keeps or changes decision 6, and which of decisions 2 and 9 a new shared read path meets |
| ADR-092 Lifecycle poison localization | "Keep asynchronous value-channel and WebSocket closure as the terminal contract" (`:28`); a subscription closes on matching poison | about `pkg/lifecycle` watches; names websocket closure as terminal |
| ADR-049 (`:393`) | lifecycle harness "WebSocket stream watches ENTITY_STATES" | context only; not admitted |
| ADR-047, 044, 036, 056, 071 | mention websocket in passing | no |

SemEngine records that bind the port (on main): ADR-108 (component shell; `docs/adr/108-processor-shell.md:181` names
`output/websocket/websocket.go:157`), ADR-113 (the Start context bounds Start; one run context ended in Stop; the pin
breaks it at `websocket.go:642` — `runtimeCtx, runtimeCancel := context.WithCancel(ctx)`), ADR-112 (library timing
claims need a probe).

Guidance: the pin's `kv-or-stream` skill and the developer contract line "producer -> graph-ingest -> ENTITY_STATES ->
KV watchers" (`.agents/contracts/semstreams-developer.md:42`) are already carried in this repo at
`.agents/skills/kv-or-stream/SKILL.md:32-38` ("A KV watch has no acknowledgement ... Never use a KV watch as a work
queue") and `.agents/skills/query-pattern/SKILL.md`. No pin guidance names `output/websocket` (`grep -rn -i websocket
.agents` at the pin → only the kv-or-stream table).

## 7. Shape sweep (pin copy)

| # | Shape | Pin `path:line` — text | Binding basis if required |
| --- | --- | --- | --- |
| S1 | second spelling | `websocket.go:1022` — `func (w *Output) admitRequest() bool {` (vs `internal/httpadmission`) | none on its own; optional |
| S2 | second spelling | `websocket.go:1238` — `func (w *Output) handleNATSMessage(ctx context.Context, msg *natspkg.Msg) {` (test-only duplicate) | test-only code (#186); unexported pin code only tests call falls under Bounded port scope, so optional; probably falls away with the NATS input |
| S3 | second spelling | `websocket.go:1590` — `func (w *Output) buildClientSnapshot() ...` vs `pingClients` `:1785-1796` | optional |
| S4 | second spelling | path validated at `:114`, `:377`, `:618`, `:727` | optional |
| S5 | copied shell | `websocket.go:157` — `lifecycleUsed  bool` (own Start/Stop/Health/DataFlow state) | AGENTS row "component.Lifecycle"; ADR-108: required |
| S6 | layered interface, one impl | `websocket.go:198` — `type coreSubscription interface {` (one production implementer, the natsclient subscription; else test fakes) | optional (falls away if the NATS input goes) |
| S7 | back door | `websocket.go:171-175` `requestHook`, `subscribeCore`, `waitForInput`, `consumeStream`, `waitJSClosed`, set only by `lifecycle_owner_test.go` | AGENTS row "no back door" (#186, #190): required |
| S8 | exported, no reader | § 4 (a) list | surface audit: each dropped, internal, or kept with a contract line |
| S9 | advertised absent | at-least-once (`README.md:186`; `:1697-1720`; ack `:1160` — `if ackErr := msg.Ack(); ackErr != nil {`) | admission gate "acknowledged durability / silent-loss prevention": required correction or declared limit |
| S10 | silent no-op | `websocket.go:1490` — `// For now, just log that we received a slow signal` | "any skip, drop, or degrade is declared": required to declare or drop |
| S11 | phantom config | `websocket.go:1881` — `deliveryMode = cfg.DeliveryMode` (unvalidated) | "refuse unknown config ... naming the key" spirit; review |
| S12 | context root | `websocket.go:642` — `runtimeCtx, runtimeCancel := context.WithCancel(ctx)` | ADR-113 (verify+review): required |
| S13 | nil default | `websocket.go:625` — `// NATS client is optional for testing - will skip NATS subscription if nil`; `:1058` — `if w.natsClient == nil {` | "never invent ... nil defaults": required |
| S14 | metric API | `websocket.go:345` `registry.RegisterCounterVec(...)` (absent on main) | metric-registry spec: required (build) |
| S15 | identity | `websocket.go:1902` — `Name:            "websocket-output",` fixed name feeds metric labels and the JetStream durable `ws-output-<subject>` (`:1131` — `consumerName := fmt.Sprintf("ws-output-%s", sanitizedSubject)`); two instances on one subject collide | review; may fall away with the NATS input |
| S16 | origin | `websocket.go:433-436` `CheckOrigin` always `true` ("In production, this should be more restrictive") | review; #84 scope |
| S17 | dead state | `lastPing` (`:231`), `clientInfo.messagesSent`, `PendingMessage.Retries/SentAt/Subject/Data`, `pendingBuffer` write-only | dead surface: drop or keep with reason |
| S18 | hypothesis, unprobed | `websocket.go:1423` — `_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))`; the pong handler (`:1410-1413`) does not extend it, so a client that sends no data may be dropped at 60 s | ADR-112: needs a probe at gorilla v1.5.3 before any design statement |
| — | classification by error text | `grep -n 'strings.Contains\|err.Error()' websocket.go` → none | — |

## 8. Collision table (delivering applied graph-state changes to readers)

| Dimension | Evidence |
| --- | --- |
| Semantic class | change observation over `ENTITY_STATES`/`COMMUNITY_INDEX` (03B D14) |
| Owners | `pkg/graphview` (view; graph-query's `COMMUNITY_INDEX` view is its one live instance on that bucket); `graph/follower` (projection owners); `service` message logger SSE (operator transport, D15); semboids `graphstream` (consumer); pin `output/websocket` (relay of NATS input) |
| Catalogs | `graph/kvcatalog/kvcatalog.go:78` (`ENTITY_STATES`, owner graph-ingest), `:188` (`COMMUNITY_INDEX`, derived, graph-clustering); `component.KVWatchPort` declarations |
| Status | follower publishes `GRAPH_STATUS` readiness; graphview `ErrNotReady` until caught up, `ErrWatcherLost`; SSE sends `initial_sync_complete`; pin websocket `Health` = running && server non-nil (`:551-570`) |
| Lifecycle | graphview `Run(ctx)` once, replace by new view; follower `Open`/`Run`; SSE per-request; pin Start/Stop (ADR-095) |
| Ownership | no singleton claim in any; graphview one watch per view; pin websocket durable JetStream consumer name from subject (`:1131`) |
| Readers | graph-embedding vector cache (graphview); graph-query community view over `COMMUNITY_INDEX` (graphview, `community.go:84-88`); graph-index, graph-clustering, graph-embedding (follower); operators (SSE); SemSource UI and external ws clients (pin websocket) |
| Writers | graph-ingest only (`ENTITY_STATES`); graph-clustering (`COMMUNITY_INDEX`) |
| Recovery | graphview: lost watch fails closed, owner rebuilds; follower: lost feed never repaired until restart (`follower.go:10-14`); SSE: client reconnects; pin websocket: JetStream durable resumes (input side only) |

## 9. Intent check

The change sets no new boundary: `output/websocket`'s membership (tier 0) and its input change are ruled (03B D14, Q11
re-ruled; #34 2026-11-10). One capability effect for the owner to see: the pin's general NATS-subject relay
(semboids' `boids.frames`, § 10) has no ruling either way once the input changes (no ruling found in #8, #9, #34 for a
retained subject input). Also for the owner: the 03B matrix row (`design.md:721`) names semboids as the qualifying
consumer for change observation, yet semboids' websocket use (`boids.frames`) has no applied-state input, and its
applied-state stream is its own graphview-based SSE (`internal/api/graphstream*.go`).

## 10. Adopter seam inventory

**SemSource** (first-wave consumer; `bbfb3c62`):

- Composition: `cmd/semsource/run.go:314` — `"websocket":        websocket.Register,`; `:554-558`
  `components["websocket-output"] = wsCfg`; `:1058-1110` `websocketComponentConfig`: keys `path`, `ports`
  (input `graph_entities`, `component.JetStreamPort{StreamName: "GRAPH", Subjects: []string{"graph.ingest.>"}}` at
  `:1088`), `delivery_mode: "at-most-once"`; output `NetworkPort{Protocol:"http", Host, Port}` from
  `websocket_bind` (default `0.0.0.0:7890`, `config/config.go:232`).
- What its clients get today: every `graph.ingest.*` kind on GRAPH (`run.go:1008-1012`: entity, batch, manifest,
  status, predicates) in `{type:"data",id,timestamp,payload}` with `timestamp`/`subject` injected.
- Its own decision: `docs/adr/0001-websocket-exports-raw-entities.md` — "Keep the current architecture: WebSocket
  output consumes the raw GRAPH stream"; "Sources emit facts, not interpretations". Named downstream readers SemSpec
  and SemDragon are archived or post-MVP (`docs/inventory-scope.md`: none).
- It expects the change: `docs/upstream/semengine-port-ledger.md:71` — "Port-refactor: it reads applied state instead
  of a NATS subject. The raw `/ws` stream contract changes with it".
- What it tests: `test/ui/ui-profile.spec.cjs:322-337` opens `/ws` and closes it; `cmd/semsource/run_test.go:392`
  checks only that the path reaches the component JSON. Caddy proxies `/ws` (`Caddyfile:18-21`); spec
  `openspec/specs/ui-profile/spec.md:13` lists "the raw `/ws` WebSocket".
- Duplicate it maintains: `config/config.go:364-` path validation mirroring the pin's.

1. What must they know? (a) the factory name `websocket`; (b) which config keys survive (today `path`, `ports`,
   `delivery_mode`, `ack_timeout`, `passthrough`); (c) that the input is no longer theirs to name; (d) the new wire
   shape; (e) that manifest/status/predicates no longer arrive on `/ws` if only entity/community state is carried;
   (f) the path rule, which they copy.
2. If they do nothing: their JetStream `graph.ingest.>` input port and `delivery_mode` meet strict decode
   (`SafeUnmarshal`) and the new port rules. Refused means a boot error naming the key; accepted and ignored would be
   a silent change. Which one happens is the design's call.
3. Where they find out: boot error at best (strict decode); the wire-shape change shows up only in a client, and
   their only client test does not read a message.
4. What they should have to know: the bind address and path; nothing about subjects or streams.

**semboids** (verification consumer; `8c03cc53`):

- `configs/flock.json:206-238`: `"frames-websocket"`, factory `websocket`, input
  `{"kind":"nats","subject":"boids.frames"}`, output `0.0.0.0:8081`, `delivery_mode: at-most-once`. Frames are
  simulation output, not `ENTITY_STATES` writes; an applied-state-only input has nothing to offer this use. semboids
  already streams applied state through its own graphview-based `internal/api/graphstream*.go` (ledger `:3978`).

Prefer observation to prediction: the adopter predicts the listen port (refused below 1024 at Initialize, `:612`)
and, today, a subject, stream, and the durable consumer name the component derives; under applied state the bucket
and readiness are framework-owned facts.

## 11. Size and split facts (for the design; no split chosen here)

| Unit | Production lines | Test lines |
| --- | --- | --- |
| `output/websocket` at the pin | 2,220 (doc.go 294, websocket.go 1,926) | 3,659 (10 files) |
| of which NATS/JetStream input | about 260 (`:1057-1204`, `:1216-1268`, `:1270-1329`, input part of `:373-414`) | `websocket_nats_integration_test.go` 425 plus parts of `websocket_test.go` |
| of which at-least-once | about 150 (`:1454-1494`, `:1635-1735`, `PendingMessage`) | parts of `websocket_test.go` |
| of which TLS wiring | about 35 (`:752-786`), plus `runServer` `ServeTLS` (`:1338-1342`) | |
| ACME (by the 2026-10-02 ruling) | `pkg/tlsutil` `:189-340` (about 150) + `pkg/acme` 543 (client.go 402, doc.go 141) + `go-acme/lego/v4 v4.20.4` | `pkg/acme` 215; #285 |
| README | 544 | |

Natural seams the design can choose among: the HTTP/client transport, the applied-state input, the TLS wiring, and
ACME (a separate package, `pkg/acme`, plus two loaders). The ~3,000-line guide (#500) is exceeded by the package with
its tests (5,879); the design states the split and why.

## Skipped and residual risk

- Integration tests and the lifecycle suite were not run on the pin (precedent cited in § 0).
- S18 is an unprobed library-behaviour hypothesis (ADR-112).
- semstreams-ui not re-read (03B settled Q11); semconnect and semteams not read (they do not compose the websocket per
  the 03B matrix row `:721`).
