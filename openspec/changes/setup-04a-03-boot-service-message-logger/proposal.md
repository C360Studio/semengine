# Proposal: setup-04a-03-boot-service-message-logger

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). `service` is split
by lifecycle owner into seven pull requests (owner, 2026-10-20, #9 comment 6296023694, option B; order in
`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md` § 2.3). PRs 1-3 merged: `BaseService`
and the shared files as `3453d9b` (#343), `Manager` and `Metrics` as `7c4dd2c` (#353), and `ComponentManager` as
`ad9fda0` (#359). This change is PR 4: the message logger (`MessageLogger`), the lifecycle owner that subscribes to the
message subjects, keeps a bounded log of recent messages, and serves it and KV watches over HTTP. One lifecycle owner,
2,245 non-test pin lines at SemStreams `8b99efe9`.

## What Changes

- Port `message_logger.go`, `message_logger_http.go` and `message_logger_kv_watch.go` with their tests (2,613 pin test
  lines). `RegisterAll` registers the message logger's constructor.
- Apply ADR-113 (settled) to the pin's run context (`message_logger.go:714`), retry goroutine (`:801`) and per-request
  SSE watch goroutine (`message_logger_kv_watch.go:230`).
- Resolve the shape-sweep findings that name the message logger (inventory § 3): S1 run context, S3 local lifecycle
  state (`message_logger.go:234-239`, toward `internal/lifecycleguard`, #196), S7 error-text classification
  (`message_logger_http.go:443`, `:497`), and the dropped access token and `Snapshots` result shape
  (`message_logger.go:347-349`).
- Decide #280 item 2: whether `component.Declaration` keeps its parallel port and port-facts lists, with its readers
  updated if it changes.

## Capabilities

- `service-lifecycle`: the message logger's construction refusals, its Start and Stop under ADR-113, its health
  while a subject has failed to subscribe, the fixed subject set, and the malformed-subject refusal.
- `operator-surface`: the routes under `/message-logger`, the KV query, and the KV watch.
- `composition-validation`: one subject grammar for validity, cover and overlap.

## Non-goals

`StorageObservabilityService`, `MetricsForwarder`, `HeartbeatService` and `LogForwarder` (PRs 5-7). #280 items 1 and 3.
The follow-ups from PR 3 (#361-#364).
