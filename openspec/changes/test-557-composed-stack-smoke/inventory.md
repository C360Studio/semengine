# Inventory: composed smoke test with restart safety (#557)

base: d15e6c4646a3c26ff1af907b5631f4c01def07ab (origin/main; claim worktree `.worktrees/claude-composed-smoke-test`,
branch `claude/composed-smoke-test`). Revision 2: applies review-1 (INVENTORY CHANGES REQUESTED). Owner rulings read:
issue #557, comments 6729015840 (use case A, restart safety a main goal) and 6730981995 (four restart cases
required to pass;
one-component restart out; search checks after #558). Pin: no package is carried from SemStreams (test code only).

## 1. The claimed gap: nothing starts the real components together from one config

- `processor/graph-query/composed_integration_test.go:66` —
  `// startIngestAndIndex starts graph-ingest and graph-index on a fresh client, without graph-query.`
  The only composed test wires by hand (`CreateGraphIngest`, `CreateGraphIndex`, `startLifecycle`, :67-107); no
  graph-embedding, no `config.Manager`, no `ServiceManager`. Writes go through `graph.mutation.*` (`:138`).
- `service/config_boot_process_restart_integration_test.go:175` — `deps.Manager = configManager`
  The one process-restart boot test uses a stand-in component (`:152-170`) and calls `NewComponentManager` directly
  (`:178`), skipping `ServiceManager`.
- No production composition root: `git grep -n "EnsureStreams(" -- ':!*_test.go'` finds only
  `config/streams.go:328` — `func (sm *StreamsManager) EnsureStreams(ctx context.Context, cfg *Config) error {`
  and `config/README.md:301`. `git grep -n "natsclient.NewClient" -- ':!*_test.go' ':!*.md'` finds only doc comments.
  There is no `main` package (`go list ./... | grep -c /cmd/` = 0). The test is the composition root.
- Boot path pieces: `config/config.go:389` — `func (l *Loader) LoadFromBytes(data []byte) (*Config, error) {`;
  `config/manager.go:115` —
  `func NewConfigManager(cfg *Config, natsClient *natsclient.Client, logger *slog.Logger,` (line continues
  `opts ...ManagerOption) (*Manager, error) {`);
  `service/register.go:19` — `func RegisterAll(registry *Registry) error {`;
  `service/service_manager.go:150` —
  `func (m *Manager) ConfigureFromServices(services types.ServiceConfigs, deps *Dependencies) error {`;
  `service/service_manager.go:342` — `func (m *Manager) StartAll(ctx context.Context) error {`.
  Component `Register` funcs: `processor/graph-ingest/component.go:331`, `processor/graph-index/component.go:168`,
  `processor/graph-embedding/component.go:174`, `processor/graph-query/component.go:120`.
- Payload registry is the composition root's job: `processor/graph-ingest/component.go:274` —
  `if deps.PayloadRegistry == nil {`, plumbed by `service/component_manager.go:215` — `payloadRegistry:   deps.PayloadRegistry,`.
  Test type registration: `internal/harness/payloadfixture/testing.go:33` (`NewWithSubset`), `:54` (`RegisterTestType`).
- Input path: `processor/graph-ingest/config.go:93` — `Name: "entity_stream", Config: component.JetStreamPort{...}`
  (stream `ENTITY`, subjects `entity.>`, deliver policy `all`); the JSON config declares the ENTITY stream with
  bounds and file storage (`EnsureStreams` refuses an unbounded one,
  `config/streams.go:328`; file is the default,
  `config/streams.go:28` — `Storage string json:"storage,omitempty" // "file" or "memory" (default: file)`,
  whitespace collapsed).

## 2. Every current spelling of the facts the test checks

Verbs served on main with graph-ingest + graph-index + graph-embedding + graph-query (`processor/graph-query/query.go:27-73`):
`entity`, `batch`, `prefix` (forward to graph-ingest), `byName` (graph-index), `entityByAlias`, `relationships`,
`pathSearch`, `hierarchyStats`, `summary`, `localSearch` (composed in graph-query). Not served today:

- `processor/graph-query/query.go:32` — `"semantic": {graph.ResponderGraphEmbedding, "search"},` and `:33`
  `"similar":  {graph.ResponderGraphEmbedding, "similar"},` forward to `graph.embedding.query.*`, which has no
  responder on main; draft PR #558 adds them (`processor/graph-embedding/query.go` is new there).
- `globalSearch`/`searchGraph` (`:52-62`) need the spatial and temporal responders, which are not ported.
Subjects are spelled in `graph/query_verbs.go:95-121` (`graph.query.<verb>`).

Readiness: `graph/readiness/watcher.go:50` `KeyGraphIndex = "graph-index"`, `:52` `KeyGraphEmbedding = "graph-embedding"`,
`:57` `KeyGraphIngest = "graph-ingest"` in `GRAPH_STATUS`; component health via `ComponentManager.GetComponentHealth`
(`service/component_manager.go:527`) and the HTTP health routes.

Embedding records: no public read verb until #558. The only read is the `EMBEDDING_INDEX` bucket (a read, never a
write), declared as such. BM25 only, no provider: `processor/graph-embedding/config.go:13-14`. BM25 statistics are
process-local and order-dependent (`graph/embedding/bm25_embedder.go:119-123`): after a restart check record presence
and status, never vectors or rank (owner, 6730981995).

## 3. Adjacent claims

- Open PRs (`gh pr list --state open`, 2026-11-08): only #558 (draft, 32 files). File overlap with this change: none
  for test files. It edits `graph/follower/follower.go` and `graph/embedding/worker.go`, the files a case-4 fix would
  touch. Merge order: see design.
- No draft PR claims `claude/composed-smoke-test` yet (no claim until one is opened).
- Issues: #557 (this); #15 open (memory-stream generation and the community index marker; file-storage cases here do
  not retire it); #108 (graph-index rebuild cost: recorded, not timed); #195 (`natsfixture.Open` adoption: the test
  uses it); #109, #110 name the word, not this work. SemStreams #821 (restart acceptance never landed), #1161, #811,
  #1426: reference only.
- Specs touched by observation only: `graph-ingest-recovery` (`openspec/specs/graph-ingest-recovery/spec.md:303-306`
  file stream across a broker restart; "Process killed between apply and acknowledgement"), `entity-state-follower`
  (`:86-91` lost feed holds until the process restarts), `graph-embedding` (`:24` lost Worker watch until the process
  restarts), `nats-fixture`, `process-host`. No spec states a composed-stack or restart promise: `git grep -n -i
  "composed\|whole stack" -- openspec/specs` finds only graph-query's composed verbs, operator-surface's composed
  services and two other uses of the word.

## 4. Who it is for

No new exported symbol, subject, bucket or config field (test code only). New spec requirements are the owner's
restart-safety promise (6730981995), consumers SemSource and SemTeams at the v0.1.0-alpha.3 tag.

## 5. Problem shape and closest instances

Shape: a black-box composition test with process restart. Closest instances:

- Process kill and redelivery: `processor/graph-ingest/settlement_integration_test.go:217-260` (prochost child, held
  on the "Entity ingested" log record, `processor/graph-ingest/ingest.go:104`; consumer `AckPending` read at `:243-247`).
- Broker restart under a running component: `processor/graph-ingest/file_restart_health_integration_test.go:23-58`
  (`// Own server:`, reconnecting client, `restartKeepingURL`).
- Restart helper: `internal/harness/natsfixture/restart.go:39` — `func (f *Fixture) Restart(ctx context.Context) error {`
  (drains the fixture's own connection; URL kept in practice, not promised by Docker, `:25-29`).
- Child process host: `internal/harness/prochost/prochost.go:88` —
  `func Start(t testing.TB, name string, env ...string) (*Process, error) {`; `Kill` `:199`, `Alive` `:211`;
  wait for SIGTERM, never bare `select{}` (`:12-13`); confirm alive first (`docs/testing.md:578-580`).

## 6. Pin decision records

None: this change carries no package from the pin.

## Seams the design must face (review-1 corrections)

- `service/service_manager.go:165` — `if cfg.HTTPPort == 0 {` / `:166` `cfg.HTTPPort = 8080`: a JSON config cannot
  ask for a kernel-picked port; absent or 0 means 8080.
- `service/service_manager_http.go:75` — `listener, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.HTTPPort))`: every address.
- `service/service_configs.go:90` — `materializeServiceConfig(resolved, "metrics", json.RawMessage(`{"path":"/metrics","port":9090}`))`:
  metrics is on by default on a fixed port.
- The bound address is private: tests in package `service` read `manager.httpListener`
  (`service/service_manager_test.go:97-104`) or write `manager.config.HTTPPort = 0` (`service/component_startup_test.go:247`).
- `service/service_manager.go:329-331` — `StartAll ... a StartAll after StopAll (errs.ErrAlreadyStarted)`: a restart
  needs a fresh `config.Manager`, `ServiceManager` and connection, or a new process.
- `graph/follower/follower.go:9-19`: a lost feed is never repaired; `:214` `watcher, err := f.reader.WatchAll(ctx)`;
  `:232` `f.loseFeed("the watch closed")`; `:421` `target, created, err := readStream(ctx, f.status)` (a changed
  stream creation time is a lost feed). graph-embedding's Worker and VectorCache hold their own `EMBEDDING_INDEX`
  watches (`graph/embedding/worker.go:300`, `graph/embedding/vector_cache.go:72`).

## Probe: KV watch across a broker restart (case 4), run 2026-11-08

nats.go v1.55.0, nats-server image nats:2.15.0-alpine (`internal/harness/natsfixture/nats-image`), Docker 29.9.2.
Source `/tmp/smoke-design/probe/restart_probe_integration_test.go`, run in a scratch copy at package
`graph/follower`: `go test -tags integration -count=1|2 -run TestProbeWatchAcrossBrokerRestart -v`. Output
`/tmp/smoke-design/probe/run1.txt`, `run2.txt`. Three runs, all PASS:

- ENTITY_STATES storage=File; stream creation time equal before and after (`sameCreated=true`), so `:421` does not fire.
- The raw `WatchAll` did not close; puts after the restart arrived 19.53 s, 19.56 s, 19.46 s after it.
- Every key delivered once (no redelivery); the follower read ready again (`Indexed=5 Stuck=[]`), never lost the feed.
- Not covered: graph-ingest's consumer (covered by the file-restart test above), graph-embedding's two watches
  (same nats.go watch, not run), graph-query's subscriptions, `config.Manager`'s watches, a long outage.

## Adopter seam inventory (a product writing its own main)

1. Must know: call `EnsureStreams` before `StartAll`; build the payload registry and register its types; set
   `http_port` and disable or port `metrics`, or get 8080/9090; a `ServiceManager` is not restartable; the client
   must reconnect (defaults do: `natsclient/client.go:317`); a watch stalls about 20 s after a broker restart. Six
   facts: a design finding, recorded, not fixed here.
2. Doing nothing: fixed ports 8080 and 9090 bound on every address; a missing `EnsureStreams` (where it surfaces: not
   measured, open question).
3. Found out: port clash is a boot error; restart refusal a typed error; the stall nowhere but a doc comment.
4. Should know: nothing beyond the config. The gap is recorded for a follow-up issue, not this change.

Collision table: not triggered (no new durable, communication or coordination primitive). Intent check: not
triggered (no boundary moves).
