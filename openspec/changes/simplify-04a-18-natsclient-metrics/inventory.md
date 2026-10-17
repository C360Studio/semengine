# Inventory: simplify-04a-18-natsclient-metrics

base: 594a29716da8834bee1ee403dabe13579016db85 (claim head 747a076, no content)

Question: what #222's last bullet touches, as the owner named 6d (#223 comment 6234160898): #75 (JetStream metric
series ownership), #169 (a refused subscription), and one each of `publishToStream` / `PublishToStreamWithAck` and
`KeysByFilter` / `FilteredKeys`. Repositories read (docs/inventory-scope.md): this one; SemStreams at the pin
`8b99efe9` (`gh api repos/C360Studio/semstreams/tarball/8b99efe9`, file grep); semsource `4093d3c`, semconnect
`dff1265`, semteams `9a04328`, semboids `37dbdb0` at symbol level. The consumer SHAs are the local checkouts' HEAD
files, read without git; semconnect and semboids are on branches, so the reviewer re-checks them with `gh api`.

Callers were measured with `gopls references` under `GOFLAGS=-tags=integration`. Known positive: `KeysByFilter`
returned `processor/graph-ingest/query.go:270`.

## 1. The claimed gaps

Issue #75, shared series with no owner:

- `natsclient/jetstream_metrics.go:145` —
  `if m.streamState, err = metric.RegisterOrGet(registry, "jetstream", "stream_state", m.streamState); err != nil {`.
  A second client on the same registry gets the same collector (design D9), so the premise holds in the API.
- `natsclient/jetstream_metrics.go:291` — `m.streamState.WithLabelValues(name).Set(0)`. Any failed stream `Info` sets
  the series to 0 (the issue's `:278`).
- `natsclient/jetstream_metrics.go:244` — `func (m *jetstreamMetrics) forgetConsumer(streamName, consumerName string) {`.
  It deletes the four consumer series at `:251-254` (the issue's `:232-243`).
- `natsclient/jetstream_metrics.go:310` — `m.consumerPending.DeleteLabelValues(consumer.stream, consumer.name)`. A
  failed consumer `Info` deletes the series too (`:308-320`). This was added after the pin, and is the same class.
- Inside one client nothing collides. `natsclient/stream.go:622` —
  `"Client", operation, "duplicate local durable identity")`: a durable consumer has one local owner per client.
- Real-world trigger: none found. Production `natsclient.WithMetrics` callers, each one client per registry:
  - pin `internal/bootstrapobservability/bootstrap.go:123` (one per process, from `internal/boot/run.go:436`);
  - semconnect `cmd/cs-graph-backend/main.go:186`, on a registry built at `:181`;
  - semboids `cmd/semboids/main.go:338`.

  In this repo there are none: `git grep -n -w WithMetrics -- ':!*_test.go' ':!internal/cache'` lists only
  `natsclient/options.go:145` and `doc.go:412`. Two clients share one registry only in
  `natsclient/jetstream_metrics_test.go:22` (`TestJetStreamMetricsShareCanonicalCollectorsAcrossOwners`).

Issue #169, a refused subscription:

- `natsclient/client.go:409` — `nats.ErrorHandler(m.handleError),` is set on every connection `Connect` dials.
  `SetConnection` is gone (#184, closed).
- `natsclient/client.go:1867` — `m.logger.Error("NATS error", attrs...)`. This is the only action `handleError` takes
  on a refusal (`:1840-1869`). There is no metric. Confirmed.
- nats.go v1.54.0 (source read; the scripted-server tests in task 2.1 drive it), in `nats.go`:
  - `:4400-4401`: a server `-ERR` starting "permissions violation" goes to
    `processTransientError(fmt.Errorf("%w: %s", ErrPermissionViolation, ne))`;
  - `:4402-4403`: a limit refusal goes to `processTransientError(ErrMaxSubscriptionsExceeded)`;
  - `:4145-4146`: `asyncErrorCB(nc, nil, err)`, with no subscription argument.

  nats-server v2.15.0 sends "Permissions Violation for Publish to" (`server/client.go:5805`) and "... for
  Subscription to" (`:5814`): both match one prefix, so the typed error does not tell publish from subscribe.
- Behaviour kept by ruling: `docs/adr/112-natsclient-contract.md:149` —
  "**`Subscribe`, `SubscribeForRequests`:** the server has read the SUB, not that it accepted it. A refused SUB
  returns …".
  Also `openspec/specs/transport-client/spec.md:277-278` and `natsclient/README.md:102`.

The two duplicates:

- Publish: `natsclient/client.go:1285` —
  `func (m *Client) publishToStream(ctx context.Context, subject string, data []byte, msgID string) error {`
  (`:1285-1316`), and `natsclient/stream.go:1048` — `func (c *Client) PublishToStreamWithAck(` (`:1048-1086`).
  - Both check the status, get JetStream, add a trace, build the `nats.Msg`, `InjectTrace` and `js.PublishMsg`.
  - Only the second counts `publish_to_stream` (`stream.go:1080`) and wraps the error as transient (`:1081`).
  - A third copy builds the same message: `natsclient/client.go:1355` —
    `func (m *Client) publishToStreamAsync(ctx context.Context, subject string, data []byte) (jetstream.PubAckFuture,
    error) {` (`:1371-1381`).
  - `natsclient/stream.go:1046` — `// If AutoCreate is true and the stream doesn't exist, it will be created.` The
    call has no config and creates nothing: advertised but absent.
- Keys: `natsclient/kv.go:618` —
  `func (kv *KVStore) KeysByFilter(ctx context.Context, pattern string) ([]string, error) {`
  - `:625`: it reads through `kv.bucket.WatchFiltered`, owns the watcher until `Updates` closes, refuses a snapshot
    that ended early (`errFilteredWatcherSnapshotIncomplete`), and drains for at most 5 s (`:604`).
  - `natsclient/kv.go:701` — `func FilteredKeys(ctx context.Context, kv interface {`. It reads through
    `ListKeysFiltered` (`:707`) and `collectFilteredKeys` (`:726`). That returns the keys gathered so far when the
    lister's channel closes with the context live (`:735-739`).
  - It maps `ErrNoKeysFound` and `ErrKeyNotFound` to empty. Source read of nats.go v1.54.0: neither `WatchFiltered`
    nor `ListKeysFiltered` returns either error (`jetstream/kv.go:1244-1300`, `:1461-1488`; only `Keys` returns
    `ErrNoKeysFound`, `:1418`). Not probed, and not promised.

## 2. Every spelling

- Refusal signals: `handleError` log (`client.go:1867`). The JetStream error counter's operation values:
  `create_stream`, `get_stream`, `handler_panic`, `late_delivery_refused`, `publish_async`, `publish_to_stream`
  (`git grep -n 'recordError("' -- natsclient`).
- Building a stream message: three copies (§ 1). Core `Publish` (`client.go:1188`) is out of scope.
- Listing keys by filter: two (§ 1). `KeysByPrefix` (`kv.go:594`) calls `KeysByFilter` (`:600`).
  `kvcatalog.CatalogReader` exposes `ListKeysFiltered` (`graph/kvcatalog/kvcatalog.go:255`) but not `WatchFiltered`.

## 3. Adjacent claims

- Specs:
  - `openspec/specs/transport-client/spec.md:242` (Subscribe; refusal not reported, `:277-278`) and `:519`;
  - `openspec/specs/graph-transport-boundary/spec.md:116` (graph-ingest lists via `KVStore.KeysByFilter`);
  - `openspec/specs/metric-registry/spec.md` (one collector per key; `RegisterOrGet` shares).
- ADR-112 §2 (`:149`) and §3 (`:165`, "Read a class with `errors.Is`").
- Ledger natsclient row:
  - `docs/admission-ledger.yaml:839` contract — "Client, its options and every exported operation as at the pin";
  - `:1011-1012` (refusal logged by `handleError`);
  - `:1088-1091` known_risks (#75).
- Archived design: `openspec/changes/archive/2026-10-05-setup-04a-01-floor/design.md:957` — "No client label. The
  `stream_state` and `forgetConsumer` sharing".
- Rulings: #169 comment of 2026-10-12 (counter, keep nil); #9 comment 5969776736 item 5 (#75 tracked); #223 comments
  6214503370 (#190 kinds) and 6234160898 (6d).
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`):
  - #292 (8d) changes `natsclient/kvspec.go` (a comment) and `docs/admission-ledger.yaml` near `:1975`
    (kvcatalog). 6d edits neither.
  - #289 (8e): no natsclient file.
  - #243 bumps nats-server to 2.15.1 (go.mod).
  - #296 is this claim.
- Open issues (`gh issue list --state open --search natsclient`):
  - retired here: #75, #169, #222;
  - stay, being other packages' work: #291 (graph-ingest), #195 (natsfixture adoption), #74 (config/boot), #83
    (re-check on nats.go bump), #141 (`UpdateWithRetryRead` adoption), #68 (metric `sync.Once`; its natsclient line
    is done), #162 (text-classification check; this design classifies by `errors.Is`), #133 (#292);
  - name the word, not the package: #281, #216, #174, #196, #139, #280, #24, #9, #135, #60, #103, #134.

## 4. Readers (who it is for)

| Symbol | This repo, production | Pin, production | Consumers, production |
| --- | --- | --- | --- |
| `WithMetrics` | none | `internal/bootstrapobservability/bootstrap.go:123` | semconnect `main.go:186`; semboids `main.go:338` |
| `PublishToStream` / `WithMsgID` | none | `processor/agentic-loop/component.go:3036` (MsgID) | semsource `internal/entitypub/publisher.go:346` |
| `PublishToStreamWithAck` | none | none (tests only) | semteams `cmd/semteams/commands/teamhint/command.go:97` |
| `KVStore.KeysByFilter` | `processor/graph-ingest/query.go:270`; `kv.go:600` | `processor/graph-index/query.go:428`, `:567`, `owner_reconcile.go:47`, `component.go:2099` | none |
| `FilteredKeys` | none | `graph/clustering/storage.go:244`; `graph/inference/storage.go:312`, `:560`; graph-clustering `query.go:321`, `anomaly.go:123`, `component.go:2189`; agentic-loop `trajectory_reader.go:80`; agentic-tools `register_graph_query.go:111`; graph-index-temporal `query.go:75`; `test/e2e/client/nats.go:947` | semteams `cmd/semteams/tools/emitartifact/derivation.go:914` (a `jetstream.KeyValue`, `:898`) |

The pin passes narrow readers that implement `ListKeysFiltered` only: graph-clustering `incomingBucketReader`
(`reader_capabilities_test.go:27`) and agentic-loop `trajectoryFactBucket`. In-tree tests that call `FilteredKeys`:
`kv_filter_test.go:82`, `kv_key_contract_integration_test.go:384`, `:689`, `nil_context_siblings_test.go:255`. The
design adds no new exported symbol.

## 5. Problem shape

- Counted refusal with a log line: `natsclient/owned_delivery.go:101-102` (`late_delivery_refused`). Adopted.
- One private path behind several exported calls: `natsclient/client.go:1255`, `:1279` (`publishToStream`). Extended.

## 6. Pin decision records

The change carries no package from the pin, so this category does not apply. ADR-112 (in tree) governs.

## Shape sweep (today's tree)

- A second spelling: three message builders and two key listings (§ 2).
- Classification by error text: none added. The refusal is read with `errors.Is`.
- Advertised but absent: `stream.go:1046` (AutoCreate).

The collision table is not triggered: no new durable, communication or coordination primitive. The intent check is
not triggered: no boundary moves.

## Adopter seam inventory (a product calling natsclient)

- Refusal counter:
  - What they must know: nothing new; the call still returns nil.
  - If they do nothing: the refusal shows as a log line plus a counter.
  - Where they find out: log or metric, the rank the owner ruled for #169.
- One client per registry (#75):
  - What they must know: two clients on one registry share series and the last write stands.
  - Where they find out: a doc comment, which counts as a finding at that rank. No measured product builds two
    clients on one registry.
- `FilteredKeys` reader:
  - What they must know: the reader needs `WatchFiltered`. A narrow reader gets a compile error, the best rank.
  - If they do nothing: an early-closing listing now returns an error instead of a short list.
