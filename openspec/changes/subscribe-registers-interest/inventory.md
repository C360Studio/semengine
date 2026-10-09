# Inventory: subscribe-registers-interest

- base: `805ace8bbf70f5f7ead0f145328f6db4a2e0e08b`, the empty claim commit on `claude/subscribe-registers-interest`
  (draft PR #156) over `origin/main` `9d1dd879f182a573c465fd25be10200bfaeacf30`, so every pin below holds on both.
- revision 2, measured 2026-10-09. Revision 1 had inventory review round 1 (changes requested: three MEDIUM, one
  NIT); revision 2 adds the six missed test flushes, `natsclient/README.md` to #93's row, the Consume path to
  category 5, #158's files, and makes Q2 a declared limit. Issue #144 (`class:flake`); owner ruling 2026-10-09,
  relayed in the brief for this change: fix window 1 (defined below) once, in natsclient's subscribe, as its own pull
  request on `main`.
- repositories read: this one; SemStreams at its pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` through
  `gh api 'repos/C360Studio/semstreams/contents/<path>?ref=8b99efe9'` and one tarball of the pin
  (`gh api repos/C360Studio/semstreams/tarball/8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, extracted under
  `/tmp/pr156/pin-src`, local only). No `git` command was run against a sister checkout. semsource and semconnect
  were not read: `docs/inventory-scope.md:15-16` admits their imports and named corpora, not their symbol-level use
  of `natsclient.Subscribe` (Q2, a declared limit).
- probes: P1 to P5, a throwaway Go module under `/tmp/pr156/probe` (local only) built from this repository's
  `go.mod` and `go.sum` (nats.go v1.54.0, nats-server v2.14.7), run with `go test -cpu=1` on darwin/arm64, go1.26.9.
  Nothing in the worktree was written.

Question: where does natsclient make a core subscription and return before the server has it; what in nats.go
v1.54.0 can make the server confirm it, and how does that behave while the connection is reconnecting, draining or
closed; what else already models "the server has this subscription"; and what else claims this territory?

The term **window 1** (the owner's) means: natsclient's subscribe has returned, but the SUB is still in the nats.go
client's write buffer, so a message or request sent from another connection in that moment finds no subscriber. A
**round trip** here means a PING written on the connection and its PONG read back. NATS processes one connection's
protocol lines in order, so a PONG received after a SUB means the server processed the SUB.

## Problem statement

From #144: `TestGraphIngestServesExactlyTheDeclaredVerbs` (on PR #93 only) asks graph-ingest's query subjects from the
fixture's own connection right after graph-ingest's `Start`, and gets "no responders": 149 of 300 runs at `-cpu=1`
locally (PR #93 `f1c8392`, #144 comment), twice in CI (runs 37927038481, 37936839601). The cause read in the code:

- `natsclient/client.go:1267` — `sub, err := conn.Subscribe(subject, cb)`, then `return sub, nil`: no round trip.
- nats.go `nats.go:5170-5173` — `if !nc.isReconnecting() {` /
  `nc.bw.appendString(fmt.Sprintf(subProto, subj, queue, sid))` / `nc.kickFlusher()`: the SUB is appended to a
  buffer and a flusher goroutine is signalled;
  `nats.go:4421-4427` — `kickFlusher` is a non-blocking channel send; `nats.go:4175` — `func (nc *Conn) flusher() {`
  writes later.

P1 measured the window without natsclient: subscribe on connection A, request at once from connection B, 2,000
times. Without a round trip on A: 831/2000 "no responders" at `-cpu=1`, 800/2000 at `-cpu=1 -race`. With
`A.FlushWithContext` between them: 0/2000 in both.

## 1. The claimed gap

Claim: no core subscribe in natsclient makes a round trip before it returns.

- `natsclient/client.go:1253` — `func (m *Client) Subscribe(ctx context.Context, subject string,
  handler func(context.Context, *nats.Msg)) (*Subscription, error) {`
- `natsclient/client.go:1258` — `return m.subscribeWith(ctx, subject, handler, nativeSubscribe)`
- `natsclient/client.go:1264` — `type subscribeFunc func(conn *nats.Conn, subject string, cb nats.MsgHandler)
  (nativeSubscription, error)`
- `natsclient/client.go:1266` — `func nativeSubscribe(conn *nats.Conn, subject string, cb nats.MsgHandler)
  (nativeSubscription, error) {`
- `natsclient/client.go:1267` — `sub, err := conn.Subscribe(subject, cb)`
- `natsclient/client.go:1277` — `return m.subscribeOwned("Subscribe", subject, func(*nats.Conn) nats.MsgHandler {`
- `natsclient/client.go:1298` — `func (m *Client) subscribeOwned(`
- `natsclient/client.go:1302` — `if m.closing {`
- `natsclient/client.go:1304` — `return nil, errs.Wrap(nats.ErrConnectionClosed, "Client", operation, "client closed")`
- `natsclient/client.go:1306` — `conn := m.conn`
- `natsclient/client.go:1307` — `if conn == nil || !conn.IsConnected() {`
- `natsclient/client.go:1309` — `return nil, ErrNotConnected`
- `natsclient/client.go:1312` — `finish := m.admitLocked(workSubscription)`
- `natsclient/client.go:1317` — `sub, err := subscribe(conn, subject, func(msg *nats.Msg) {`
- `natsclient/client.go:1321` — `finish()`
- `natsclient/client.go:1324` — `s := newSubscription(sub, d.end)`
- `natsclient/client.go:1325` — `go m.watchOwnedSubscription(conn, sub, d, closing, finish)`
- `natsclient/client.go:1326` — `return s, nil`
- `natsclient/request.go:363` — `func (c *Client) SubscribeForRequests(`
- `natsclient/request.go:372` — `return c.subscribeOwned("SubscribeForRequests", subject,
  func(conn *nats.Conn) nats.MsgHandler {`
- `natsclient/request.go:374` — `}, nativeSubscribe)`

Every natsclient path that makes a core subscription:

```bash
git grep -n -E '\.(Subscribe|QueueSubscribe|ChanSubscribe|ChanQueueSubscribe|SubscribeSync|QueueSubscribeSync|SubscribeForRequests)\(' -- '*.go' ':!*_test.go'
```

returns one production call into nats.go, `natsclient/client.go:1267`; the other hits are doc comments
(`message/doc.go:346-352`, `natsclient/doc.go:46`, `vocabulary/doc.go:80`). Both exported entries, `Subscribe` and
`SubscribeForRequests`, reach it only through `subscribeOwned` (`git grep -n -E 'nativeSubscribe|subscribeOwned|subscribeWith\('`:
`client.go:1258`, `:1266`, `:1274`, `:1277`, `:1298`, `request.go:372`, `:374`, and the test seam at
`natsclient/client_close_final_test.go:1236`). There is no queue, channel or synchronous subscribe in natsclient.

Not core subscriptions made by natsclient, and unchanged: the Consume methods (`natsclient/stream.go:440` —
`func (c *Client) ConsumeStreamWithConfig(`) and the KV watchers (`natsclient/kv.go:719` —
`watcher, err := kv.bucket.Watch(ctx, pattern)`; `natsclient/storage_report_consumer.go:209` —
`watcher, err := c.store.WatchAll(ctx)`): nats.go's jetstream package makes their subscriptions, and the server
sends to them only after a request on the same connection, which follows the SUB.

Callers in the tree. `gopls references natsclient/client.go:1253:18` and `natsclient/request.go:363:18` return test
files only (`client_close_final_test.go`, `client_lifecycle_test.go`, `client_test.go`, `nil_context_siblings_test.go`,
`request_cancel_breaker_test.go`, `reviewer_full_nats_probe_test.go`, `reviewer_nil_context_probe_test.go`); gopls
does not build `integration` files, so `git grep -n -E '\.(Subscribe|SubscribeForRequests)\(' -- '*_test.go'` adds
`client_integration_test.go`, `errors_integration_test.go`, `integration_test.go`, `readiness_integration_test.go`,
`request_integration_test.go`, `request_response_bounds_integration_test.go`, `subscription_integration_test.go`,
all under `natsclient/`. No production package on `main` calls either. graph-ingest's calls exist on PR #93 only.

## 2. Every current spelling of the fact

The fact: "the server has processed this subscription".

nats.go v1.54.0 (module cache, `go list -m -f '{{.Dir}}' github.com/nats-io/nats.go`):

- `context.go:174` — `func (nc *Conn) FlushWithContext(ctx context.Context) error {`; `:181-184` —
  `_, ok := ctx.Deadline()` / `if !ok {` / `return ErrNoDeadlineContext`; `:187-190` — closed: `ErrConnectionClosed`;
  `:194` — `nc.sendPing(ch)`; `:200-202` — a closed channel becomes `err = ErrConnectionClosed`; `:206-207` —
  `case <-ctx.Done():` / `err = ctx.Err()`. P2: a context with no deadline returns
  `nats: context requires a deadline`.
- `nats.go:5983-5988` — `func (nc *Conn) sendPing(ch chan struct{}) {` / `nc.pongs = append(nc.pongs, ch)` /
  `nc.bw.appendString(pingProto)` / `nc.bw.flush()`: the PING and everything buffered before it, the SUB included,
  are written at once.
- `nats.go:6017` — `func (nc *Conn) FlushTimeout(timeout time.Duration) (err error) {`: the same, bounded by a timer,
  not a context.
- `nats.go:6058-6070` — `func (nc *Conn) RTT() (time.Duration, error) {`; `:6062-6064` — `if nc.IsReconnecting() {` /
  `return 0, ErrDisconnected`; `:6066` — `if err := nc.FlushTimeout(10 * time.Second); err != nil {`.
- `nats.go:6074-6076` — `func (nc *Conn) Flush() error {` / `return nc.FlushTimeout(10 * time.Second)`.

natsclient:

- `natsclient/client.go:1152` — `func (m *Client) RTT() (time.Duration, error) {`; `:1161` — `return conn.RTT()`:
  nats.go's fixed 10 s, whatever the caller's context.
- `natsclient/client.go:389` — `func (m *Client) GetConnection() *nats.Conn {`: a caller can flush the raw
  connection itself, which is what the tests below do.

Tests that wait for the server after a natsclient subscribe, fifteen sites (redundant once this change lands;
decision D8):

- `natsclient/nil_context_siblings_test.go:33`, `:127`, `:164` — `require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))`
- `natsclient/request_cancel_breaker_test.go:73` — `require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))`
- `natsclient/client_close_final_test.go:247` — `require.NoError(t, nc.FlushTimeout(lifecycleBound))` (after
  `SubscribeForRequests` at `:240`)
- `natsclient/client_close_final_test.go:1155`, `:1377` — `require.NoError(t, nc.FlushTimeout(lifecycleBound))`;
  `:1416` — `require.NoError(t, first.FlushTimeout(lifecycleBound))` (after `c.Subscribe` at `:1152`, `:1374`,
  `:1414`)
- `natsclient/client_lifecycle_test.go:547` — `require.NoError(t, nc.FlushTimeout(lifecycleBound))` (after
  `c.Subscribe` at `:544`)
- `natsclient/client_close_final_test.go:1071` — `require.NoError(t, dialled.FlushTimeout(lifecycleBound))` (after
  `c.Subscribe` at `:1069`)
- `natsclient/reviewer_full_nats_probe_test.go:23` — `require.NoError(t, c.GetConnection().Flush())` (after
  `SubscribeForRequests` at `:18`)
- `natsclient/reviewer_nil_context_probe_test.go:23` — `require.NoError(t, c.GetConnection().FlushTimeout(10*time.Second))`
  (after `SubscribeForRequests` at `:21`)
- `natsclient/request_response_bounds_integration_test.go:122` — `require.NoError(t, client.GetConnection().Flush())`;
  `:166` — `require.NoError(t, subscriptionConn.Flush())`; `:225` —
  `require.NoError(t, client.GetConnection().FlushWithContext(flushCtx))` (after `SubscribeForRequests` at `:118`,
  `:158`, `:219`)

Flushes after a raw nats.go subscribe, not natsclient's, and out of scope: `client_close_final_test.go:414`, `:773`,
`client_close_integration_test.go:39`. Searches: `git grep -n -E 'Flush|RTT\(\)' -- '*.go'`, each hit read with the
14 lines before it. Revision 1 listed nine sites: its search output was cut at 50 lines, which hid the five sites in
`reviewer_*_probe_test.go` and `request_response_bounds_integration_test.go`, and `client_close_final_test.go:1071`
was shown but not checked (review round 1).

Planned, not in the tree: PR #93's design D20 and task 3.12l add `natsclient.Client.Flush(ctx)` for graph-ingest's
`Start` (category 3).

A different window, not this fact: the requester-side readiness read, `natsclient/request.go:84` —
`DefaultReadinessProbeTimeout = 2 * time.Second`, `:87` — `DefaultReadinessBudget = 30 * time.Second`, used by
`RequestReadyClassified` for "a not-yet-subscribed responder" at boot. It retries a request until a responder
exists; it does not make a subscription registered. Out of scope by the ruling.

## 3. Adjacent claims

Specs. `openspec/specs/transport-client/spec.md`: "Close refuses new work" (`:132-154`: once Close has begun,
`Subscribe` and `SubscribeForRequests` return `nats.ErrConnectionClosed`); "A nil Close means everything the client
owns has finished" (`:82-130`: every running invocation of a handler passed to `Subscribe` or
`SubscribeForRequests`); "An exported call refuses a nil context before it acts" (`:225-235`). No other spec names
either call (`grep -n "Subscribe" openspec/specs/*/spec.md` outside `transport-client`: none).

Ledger. `docs/admission-ledger.yaml:1424` — `- source_path: natsclient`; `:2197` — `disposition: adapt`. Its changed
behaviours are numbered adapt items inside `contract` (`:1432-1579`, the last `(12) natsclient-consume-handler-context`
at `:1568`) with their tests in `proving_tests` (`:1714-2195`).

The pin (probe of the code, not of the tests: this is not a porting change, so the three runs the architect
contract asks of a porting inventory do not apply). SemStreams at `8b99efe9` does not flush after subscribing:

- pin `natsclient/client.go:834` — `sub, err := m.conn.Subscribe(subject, func(msg *nats.Msg) {`, then `:849` —
  `return newSubscription(sub), nil`.
- pin `natsclient/request.go:359` — `sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {`.
- pin `natsclient/client.go`, `request.go`, `typed.go`, `doc.go`, `README.md`: `grep -n -i "flush"` finds no flush on
  a subscribe path.

The pin's ADRs. `gh api 'repos/C360Studio/semstreams/contents/docs/adr?ref=8b99efe9'` lists 85; all 85 were
fetched and searched with:

```bash
grep -l -i -E "flush|no responders|noresponders|SubscribeForRequests|client\.Subscribe|conn\.Subscribe|registered with the server|interest" *.md
```

None states when a subscription is registered with the server. Those that touch the territory:

- ADR-060 (unified RPC error contract): governs what `SubscribeForRequests`' handler errors send back
  (`060-unified-rpc-error-contract.md:47`); this change does not touch replies.
- ADR-083 (readiness as distributed state) and ADR-088 (readiness per producer, folded by the consumer):
  `088-readiness-is-per-producer-aggregation-is-the-consumers.md:23` — "`Start` returning is not that signal". They
  govern the out-of-scope window (a responder in another component), not window 1.
- ADR-094 (boot-only composition), `:112` — "Connect owns a private five-second native flusher ceiling with no
  adopter-facing knob": a fixed private bound with no knob. The pin's `natsclient/client.go` has no
  `FlusherTimeout` (`grep -n "FlusherTimeout\|Flusher"`: none), so the ADR's sentence describes something the pin
  does not have. Recorded as an observation only.

Open pull requests (`gh pr list --state open --json number,title,changedFiles,files`; #93's 300 files read with
`gh api --paginate repos/C360Studio/semengine/pulls/93/files`):

| PR | Overlap |
| --- | --- |
| #93 (draft, head `5e4c009`) | `docs/admission-ledger.yaml`, natsclient's row: adds an adapt item at the end of `contract` (hunk `@@ -1576`) and tests at the end of `proving_tests` (`@@ -2192`), the places this change adds to. `natsclient/nil_context_siblings_test.go` (`@@ -230`, a table row; this change does not edit the file, D8). A `transport-client` delta that ADDs "A retried update passes the revision it read". Design D20 and task 3.12l plan `natsclient.Client.Flush(ctx)` and a round trip in graph-ingest's `Start`; task 3.12h changes `IsKVConflictError`, `IsKVNotFoundError`, and `errs.IsTransient`/`IsFatal` to class and sentinel only, never text. `natsclient/README.md`: #93 adds 24 lines at `:248` and `:270`; this change adds one sentence at `:85-86` in its archive commit; the file is shared, the lines are not. It cannot merge while #144 is open. |
| #158 (draft) | its four files are `.agents/contracts/semengine-architect.md`, `.agents/contracts/semengine-reviewer.md`, `AGENTS.md` and `docs/inventory-scope.md`; none is this change's. It makes the ADR listing above part of every porting inventory (#157). |
| #118 | `go.mod`, `go.sum`: nats-server 2.14.7 → 2.15.0, the embedded server natsclient's unit tests run on. No shared file. |
| #116, #117, #119, #120 | no shared file; #117 bumps the OpenSpec CLI. |

## 4. The consumer at birth

The design adds no exported symbol, subject, bucket or config field. Searches for a new name it might need:
`DefaultRequestTimeout` exists (`natsclient/request.go:18` — `const DefaultRequestTimeout = 5 * time.Second`), and
`ErrNotConnected` exists (`natsclient/client.go:53` — `ErrNotConnected = stderrors.New("not connected to NATS")`).
PR #93's planned `Client.Flush(ctx)` has graph-ingest as its one consumer; this change removes the need for it.

## 5. The problem shape

Shape: the framework absorbs a server-side window the caller cannot observe, with a bounded wait inside the call.
Closest instance: stream visibility.

- `natsclient/stream.go:290` — "cannot, so the framework absorbs it here rather than exporting a wait-first";
  `:304` — `streamVisibilityBudget = 5 * time.Second`; `:344-345` — "The wait is bounded by the caller's context AND
  the budget, whichever ends first, and completes before returning: no goroutine outlives the call."
- its test runs the budget on a `synctest` clock: `natsclient/stream_visibility_test.go:278-282`.

This design adopts that shape for the bound (D2).

Second shape, for the failure path: a setup whose delivery has started and then fails or meets Close. Owned today by
the Consume path:

- `natsclient/owned_delivery.go:182-183` — "failed hands over a native Consume that returned an error and returns once
  the claim has been released, so a caller that retries does not meet its own claim"; `:184` —
  `func (o *consumerOwnership) failed() {`
- `natsclient/owned_delivery.go:189-192` — "If Close has begun meanwhile, delivery is stopped and the setup returns
  nats.ErrConnectionClosed with no handle once every handler invocation has returned, or ctx's error first if ctx
  ends"; `:193` — `func (c *Client) commitConsumer(`; `:202-208` — the `select` on `<-o.released` and `<-ctx.Done()`.
- `openspec/specs/transport-client/spec.md:149-154` — "A consumer refused during Close keeps its claim".

This design does not take that rule's wait for handlers, and states why core subscriptions differ (D4). It
establishes no new primitive, so no adoption sweep is owed.

Same-class collision table (the job: know that the server has a subscription before relying on it).

| Dimension | Evidence |
| --- | --- |
| Owners | nats.go `FlushWithContext`, `FlushTimeout`, `Flush`, `RTT` (category 2); natsclient `Client.RTT` (`client.go:1152`); callers through `GetConnection` (the fifteen test sites); PR #93's planned `Client.Flush` |
| Catalogs | none: `git grep -n -i "registered\|interest"` in `natsclient/*.go` finds no registry of subscriptions beyond nats.go's own `nc.subs` (`nats.go:5156-5160`) |
| Status | `Client.Status()` (`client.go:380`) says connected or not, nothing per subscription |
| Lifecycle | `subscribeOwned` admits, `watchOwnedSubscription` joins (`client.go:1312`, `:1325`); on reconnect nats.go resends every subscription it holds (`nats.go:6091` — `func (nc *Conn) resendSubscriptions() {`, called at `:3552`) |
| Ownership | the client owns the subscription until its delivery ends (`client.go:1293-1297`) |
| Readers | none in production on `main` (category 1) |
| Writers | `subscribeOwned` only |
| Recovery | nats.go's resend on reconnect; nothing in natsclient |

## nats.go behaviour by connection state (measured)

| State when the round trip starts or while it waits | Measured | Code |
| --- | --- | --- |
| connected, server answers | returns nil; median 21 µs, p99 96 µs, max 179 µs on loopback (P5, 2,000 runs) | `context.go:194-208` |
| context with no deadline | refused: `nats: context requires a deadline` (P2) | `context.go:181-184` |
| context already ended, healthy connection | `context.Canceled` in 20 of 20 runs (P2); Go's `select` picks at random when both cases are ready, so this is not guaranteed | `context.go:199-208` |
| server does not answer | ends at the context's deadline: fake server, 5 s on the `synctest` clock (P3) | `context.go:206-207` |
| connection drops while waiting | returns `nats.ErrConnectionClosed` at once, while `Status()` is `RECONNECTING` (P4) | `nats.go:3625` (`nc.clearPendingFlushCalls()`), `:6129-6136`, `context.go:200-202` |
| started while already reconnecting, server down | waits for its deadline: 701 ms for 700 ms (P2) | `nats.go:2361-2367` (`flush` writes nothing while a pending buffer is set) |
| started while reconnecting, server back in about 300 ms | returns nil after 518 ms, once reconnected (P2) | `nats.go:3552`, `:3555` (resend, then the pending buffer, the PING in it) |
| `Conn.RTT` while reconnecting | refused: `nats: server is disconnected` (P2) | `nats.go:6062-6064` |
| `Conn.Subscribe` while reconnecting | succeeds and writes no SUB (P2); natsclient refuses before it (`client.go:1307-1309`) | `nats.go:5170` |
| `Unsubscribe` while reconnecting | succeeds; after the reconnect a request to the subject gets "no responders" (P2): not resent | `nats.go:5542` |
| draining | round trip returns nil (P2); `Conn.Subscribe` refused: `nats: connection draining` (P2) | `nats.go:5117-5118` |
| closed | `nats: connection closed` (P2) | `context.go:187-190` |
| after an `Unsubscribe` on a connected client | the server reads `UNSUB <sid>` (P3: the fake read `SUB s.x  1`, `PING`, `UNSUB 1`) | `nats.go:5542-5550` |

P3 also shows that nats.go v1.54.0 connects, subscribes and runs a round trip inside a `testing/synctest` bubble over
`net.Pipe`, with a scripted server, and that the bubble ends cleanly after `Close`.

## Intent check

Not required: this change sets or moves no boundary (no port-set, tier, exclusion or deferral change). It changes what
two existing calls promise.

## Adopter seam inventory

The surface: `Client.Subscribe` and `Client.SubscribeForRequests`, called by component authors (graph-ingest on
PR #93) and by consumers (SemSource imports natsclient, `docs/admission-ledger.yaml:1426-1430`).

1. What must they know today? That a nil error from `Subscribe` does not mean another connection can reach the
   subscription yet, and that the fix is a flush on the raw connection (`GetConnection`), with a deadline, since
   nats.go refuses a context without one. Two facts, both about internals.
2. If they do nothing? A request from another connection gets "no responders" about 40% of the time at one CPU (P1),
   and a publish from another connection is lost without an error (core NATS drops a message with no subscriber).
   The second is silent loss.
3. Where do they find out? Nowhere: no doc comment says it (`client.go:1245-1252`, `request.go:356-362`), and the
   failure shows as a flaky test or a lost message.
4. What should they have to know? Nothing: a subscription that `Subscribe` returned is reachable. The caller cannot
   observe the window; natsclient can (a PONG). This is the "prefer observation to prediction" case.

## Open questions

- Q1. Does any consumer call `Subscribe` or `SubscribeForRequests` per request or in a loop, where one round trip
  per call would cost? On `main`, nothing outside natsclient's tests calls either. At the pin, every production call
  runs in a `Start` or a `setupSubscriptions`, once per configured port or declared verb (pin
  `processor/json_map/json_map.go:334` — `for _, port := range m.inputPorts {`, `:349`; pin
  `processor/graph-ingest/query.go:27`, `:34`, `:41`, `:48`); the per-scenario subscribes are in `test/e2e` and
  `internal/e2e*`, which are not admitted.
- Q2. Answering Q1 for semsource and semconnect needs a symbol-level read that `docs/inventory-scope.md` does not
  admit for them. Not read; the design declares it as a limit (D6, L5).
