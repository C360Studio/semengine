# Inventory: natsclient's contract over nats.go, and the library-probe rule (#170)

- base: `0a4d6d5738fb74898f4f1de26f8d9a76bbf708fe`, the empty claim commit on `claude/natsclient-contract-adr`
  (draft PR #177) over `origin/main` `c713840e3cc7991e0bd8b81297a899a15d43ec3b`; every `main` pin below holds on both.
  PR #93's head is `f3d5ae56ef62b7caacf0a1bf7f63f0869ab2b047` (merge base with `main` `0e11824`, one commit behind:
  #175, which touches no file this inventory pins). Pins marked "#93" are at that head.
- revision 3 (2026-10-11): rows changed by probes P17 to P25. Design revision 2 ran P17 to P22 after the owner's
  ruling on #170 (comment 6109833136), and design revision 3 ran P23 to P25 after its review; see "Revision 3" in §2.
  Nothing else changed. Revision 2 passed review
  (`INVENTORY PASS`, `/tmp/pr177/review/design-r1.md`, sha256 `68df2d7b…`).
- revision 2 (2026-10-11): the fixes for inventory review r1 (`/tmp/pr177/review/inventory-r1.md`: 2 HIGH, 6 MEDIUM,
  1 NIT), made in one pass with the design under the owner's time-box. No new probe was run. The reviewer's p15b
  (source `/tmp/pr177/review/trees/c713840/rvprobe/p15b/main.go`, the reviewer's runs, 3 of 3) is cited as the
  reviewer's. N-P1 to N-P19 stay listed as the record of what is unmeasured; the time-box drops those probes, and the
  design promises nothing they would have measured. Revision 1 was the inventory only (workflow step 2).
- question: what does natsclient say, on `main` and at #93's head, about how nats.go v1.54.0 and nats-server v2.14.7
  time, order, bound and fail, and what natsclient promises in those terms; which of those statements a probe at the
  pinned versions backs, contradicts or leaves unmeasured; where the existing probe-citation rule lives and where a
  library-probe rule would go; and how a `transport-client` delta here meets #93's at archive.
- repositories read: this one (worktree, the `git archive` copies below, GitHub issues and PR comments through `gh`);
  SemStreams at the pin `8b99efe9` through `gh api 'repos/C360Studio/semstreams/contents/docs/adr?ref=8b99efe9'` only
  (`docs/inventory-scope.md:31-34`, rule 4). No sister checkout was read or touched; no consumer repository was read
  (the adopter seam inventory below is answered from this repository's surface; symbol-level consumer use of the
  calls named is not admitted for this question and is a declared limit, as #156's Q2 was).
- trees: `git archive c713840 | tar -x -C /tmp/pr177/trees/c713840`, `git archive f3d5ae56 | tar -x -C
  /tmp/pr177/trees/f3d5ae56` (local only). Probes are copied into `<tree>/pr177probe/pN/` and built there, as #156's
  P6 was.
- pins: `go.mod:12` — `github.com/nats-io/nats-server/v2 v2.14.7`; `go.mod:13` — `github.com/nats-io/nats.go v1.54.0`;
  `.nats-image:5` — `nats:2.14.7-alpine@sha256:4063edae…` (the container the integration lane runs; same server
  version as the embedded one). The pin used nats.go v1.52.0 and nats-server v2.12.4
  (`docs/admission-ledger.yaml:1618-1619` — `regression evidence: nats.go v1.52.0 at the pin (go.mod:12) is v1.54.0
  here; the embedded test` / `broker nats-server/v2 v2.12.4 at the pin`). Open Dependabot PR #118 would move
  nats-server to 2.15.0 (`go.mod`, `go.sum`; head `d933fe32`); PR #117 would move OpenSpec 1.13.2 → 1.14.0.
- system for every probe: darwin/arm64, Apple M3 Pro (12 CPUs), macOS (Darwin 25.5.0), `go version go1.26.9
  darwin/arm64`, nats.go v1.54.0, nats-server v2.14.7 (embedded, from the module cache), OpenSpec 1.13.2.

## 0. A correction to recorded evidence (found while re-measuring; read first)

PR #156's P6 (comment 6098409783) and #168's table read "after resume: returned at 59.7 s" as the call returning
about a minute after the fake server started reading again. P6 did not resume the server at 1.5 s. Its source prints
`nc.IsConnected()` and `nc.LastError()` before `close(p.resume)`:

- P6 source (comment 6098409783): `fmt.Printf("  before resume: connected=%v last_error=%v async_errors=%d\n",
  nc.IsConnected(), nc.LastError(), asyncErrs.Load())` then `close(p.resume)`.
- nats.go v1.54.0 `nats.go:6285-6288` — `func (nc *Conn) IsConnected() bool {` / `nc.mu.RLock()`;
  `nats.go:4355-4361` — `func (nc *Conn) LastError() error {` … `nc.mu.RLock()`: both wait for the connection lock the
  held write holds.

So in every P6 run that "returned at 59.7 s", the server resumed only when the probe's own `IsConnected` got the lock,
which happened when a held write's one-minute write timeout ended it. #168's caveat ("What happened once the server
read again is not explained") has this explanation. Measured here:

- P11 (the P6 design, kept on purpose: diagnostics before the resume), 16 runs on `main`: 15 of 16 held calls
  returned within 1 ms of the resume, and the resume came at 59.59 s to 1 m 1.9 s (one write timeout), at 1 m 59.6 s
  once (two), and at 1.7 s and 1.9 s in two runs where `IsConnected` got the lock early. The sixteenth, `RTT`,
  returned 10 s after the resume (`P11/out/c713840-full-rtt-1.out` — `proxy resumed at 59.597s; returned at 1m9.598s
  err=nats: timeout`): 10 s is `RTT`'s own flush timeout (`nats.go:6066` — `if err := nc.FlushTimeout(10 *
  time.Second); err != nil {`), so the server did not answer the PING sent after a write timeout had dropped bytes.
  That is the unexplained state of N-P3 (§2, area 1).
- P13 (the same, but the proxy resumes at 1.5 s before this program makes any nats.go call), 16 runs on `main`: all
  16 were held at 1.5 s, and all returned 0 to 29 ms after the resume (§2).
- Codex's blocked-write probe (PR #156 comment 6097783327) makes no nats.go call before resuming: returned 0.3 ms
  after its resume (252.65 ms), consistent with P13.

The #168 figures still show that a call is held past its context while the server does not read; they do not show
that a call stays held for a minute after the server reads again.

The sentences this corrects (revision 2; each reads a P6 time after 1.5 s as a time after the server read again):

- `openspec/changes/archive/2026-10-10-subscribe-registers-interest/design.md:97-99` — `P6 saw writes last to the
  timeout after the server read again (still held 1.45 s past a 50 ms context, returned at 59.7 s` / `and once at
  1m4.9s; …)`.
- same file `:322-325` — `after the server began` / `reading again, some returned 3.3 s to 3.5 s later and others
  only once the write timeout had ended the held write` / `(at 59.7 s, once at 1m4.9s), for a reason not explained.`
- same file `:404-406` — `P6 saw writes last to the timeout after the server read again`, with `returned at 59.7 s
  and once at` / `1m4.9s`.
- #168's body (read 2026-10-11 with `gh issue view 168 --json body`, saved at `/tmp/pr177/gh/issue168.md`; the line
  numbers are that file's): `:7`, `:9`, `:10` and the table rows `:25-29` (returns at 59.7 s and at 4.8-4.9 s, read
  as after the resume); the caveat paragraph `:38-45` (`What happened once the server read again is not explained`,
  `In other runs the call returned about 3.3 s` / `to 3.5 s after the server resumed`); the mechanism paragraph `:75`
  (`the probe saw writes last to the timeout after the server read again`).

The 3.3-3.5 s figure has the same flaw: P6 recorded no time for its resume, which came only when its own
`IsConnected` got the lock, so "3.3 s after the server resumed" counts from 1.5 s, when the server was not yet reading.
What stays true in each sentence: the call is held past its context while the server is not reading, and a held write
ends at nats.go's write timeout. Nothing in this inventory has edited #168 or the archived design.

## 1. The claims table

Statuses. **probe**: a probe at the pinned versions backs it (P1 to P16, or Codex's two probes in PR #156 comment
6097783327, here "C-bw" blocked-write and "C-ds" denied-subscribe). **test**: no posted probe, but a test in the tree
drives the real library at the pinned version (embedded nats-server v2.14.7 through `embeddedServerURL`,
`natsclient/client_lifecycle_test.go:108`, or the `nats:2.14.7` container through `startFixture`) and asserts it.
**contradicted**: a measurement shows it false in a condition the sentence does not exclude. **unmeasured**: neither;
"fake only" marks a test that drives natsclient's logic over a fake, which measures natsclient, not the library.
"own" marks a promise about natsclient's own code with no library behaviour in it (listed because #170 asks for
natsclient's promises in time, order, bound and failure terms). Where a claim holds on an idle connection and fails
while the server is not reading, both are given; the status is the worse one.

Where P1 to P5 live: their outputs are in the archived inventory
(`openspec/changes/archive/2026-10-10-subscribe-registers-interest/inventory.md:16-18`, `:42-45`, `:262-281`); their
source is on one machine only (`/tmp/pr156/probe/probe_test.go`, sha256 `f1d05318…`; `probe2_test.go`, sha256
`5280b81e…`), never posted. P6 to P9 are posted with source and output (PR #156 comments 6098409783, 6098410018). P10
to P16 are this inventory's (§2, "Probes run").

### 1a. `main` (`c713840`)

**`openspec/specs/transport-client/spec.md`** (13 requirements; from PR #48 `deaafd4` and PR #156 `58f0252`)

- M1 `:7-8` — `that \`Subscribe\` and \`SubscribeForRequests\` return a subscription only after the server` / `has
  read its SUB` — **probe**: P1 (831/2000 no-responders without a round trip, 0/2000 with), P7 (the server sends -ERR
  before PONG, PONG still comes: 1000/1000), P13 idle subscribe 0 s; test `TestSubscribeRegistrationAnswersAnotherConnection`
  (`natsclient/subscribe_registration_test.go:240`).
- M2 `:8` — "`Close` stays within the caller's deadline" — **contradicted**: P6 (`main-tcp-full-close`, held at
  1.5 s), P11 (2/2 held, 59.59 s and 1 m 1.9 s), P13 (2/2 held until the server read again at 1.5 s), P14 (3 m 1.6 s;
  still held at 3 m 20 s, the probe's cap), #168. Holds on an idle connection: P13 idle close 11 ms, P6 tcp-idle 51 ms.
- M3 `:15-17` — memory stream: an acknowledged publish is not kept across a restart — **test**:
  `TestIntegration_AcknowledgedIsNotDurableOnMemoryStream` (`natsclient/integration_test.go:109`, container).
- M4 `:26-29` — settled only after the decision; a heartbeat keeps a long delivery from redelivery; redelivered when
  it stops — **test**: `natsclient/delivery_settlement_integration_test.go:18`, `:140`, `:237` (container).
- M5 `:54-56`, `:226-230` — a nil context is refused before any server call (Drain, Close, Connect, every
  context-taking call) — **test**, own: `TestClientCloseRefusesNilContext` (`client_lifecycle_test.go:233`),
  `TestClientConnectRefusesNilContext` (`:255`), `natsclient/nil_context_siblings_test.go`.
- M6 `:57-58` — `\`Close\` SHALL bound its drain by the caller's context deadline and return within it` —
  **contradicted** as M2; on an idle connection **test** `TestClientCloseUnderShortDeadlineClosesConnection`
  (`client_lifecycle_test.go:530`, embedded server).
- M7 `:58-60` — a later Close makes no server call and returns nil once the first Close's cleanup and join are done;
  nothing is retried — **test**, own: `TestClientConcurrentClosesEachHonourTheirContext` (`client_lifecycle_test.go:682`).
- M8 `:64-66` (scenario) — `Close returns within the deadline, the connection is closed` — **contradicted** as M2.
- M9 `:85-88` — a nil Close means every handler, background goroutine and event handler has returned — **test**:
  `TestClientCloseJoinsSubscribeHandlerAfterForcedClose` (`client_close_final_test.go:227`),
  `TestClientCloseJoinsConnectionEventHandlers` (`:835`), `TestClientCloseJoinsRunningAsyncPublishErrorHandler`
  (`:1328`), embedded server. The residual, `:87-88` `its async publish error handler as far as nats.go lets the client
  observe it`, rests on a code read (`natsclient/client.go:1630` — `It runs on nats.go goroutines that have no end
  signal (jetstream/publish.go:408,`): **unmeasured**.
- M10 `:89-90` — a drain that runs out of time is a transient error wrapping `nats.ErrDrainTimeout` whichever timer
  ran out first — **test**: `TestClientCloseReportsDrainTimeout` (`client_close_final_test.go:1355`); class **probe**
  P10 (`errs.WrapTransient(ErrDrainTimeout)` is transient on both trees).
- M11 `:90-91` — `Each Close SHALL return within its own context and SHALL NOT wait on another caller's drain` — idle
  **test** `TestClientCloseHonoursItsContextDuringAnotherDrain` (`client_close_final_test.go:404`); the first Close
  under a held write is **contradicted** (M2). A second Close while the first is held: **unmeasured** (no probe; the
  second Close takes `m.mu` at `client.go:911`, which `subscribeOwned` holds across `conn.IsConnected()` at `:1342-1355`,
  #168 "read, not probed").
- M12 `:91-92` — a Close from inside a client callback returns its context's error, never nil — **test**, own:
  `TestClientCloseFromInsideCallbackReportsContext` (`client_lifecycle_test.go:734`).
- M13 `:92-96` — subscriptions and consumers on a connection `SetConnection` replaced are ended and joined; the
  replaced connection stays open — **test**: `TestClientCloseEndsSubscriptionOnReplacedConnection`
  (`client_close_final_test.go:1399`), `TestClientCloseEndsConsumerOnReplacedConnection` (`:1442`).
- M14 `:135-138` — once Close has begun, Connect, Subscribe, SubscribeForRequests and the Consume methods return
  `nats.ErrConnectionClosed`, decided by Close having begun — **test**, own: `TestClientConnectAfterCloseRefusesBeforeDial`
  (`client_close_final_test.go:716`), `TestClientLifecycleOperationTable` (`:960`). Its class: **probe** P10 —
  `errs.Wrap(nats.ErrConnectionClosed, …)` is transient on `main` (by its text) and not transient at #93 (§1b).
- M15 `:159-161` — only Close changes the status once it has begun — **test**, own: `TestClientHealthMonitorCannotOverwriteClosedStatus`
  (`client_close_final_test.go:587`), `TestClientLosingConnectLeavesWinnerStatus` (`:465`).
- M16 `:177-178` — a delivery nats.go hands over after the recorded end does not run the handler — **unmeasured**
  against nats.go (fake only: `TestClientRefusesLateDeliveryAfterRecordedEnd`, `client_close_final_test.go:1186`,
  drives `newKeptHandlerConsumer`, a fake); that nats.go hands over such a delivery is a code read
  (`natsclient/owned_delivery.go:95` — `same moment are (nats.go:3801-3803, :3817-3828; jetstream/pull.go:58-61)`).
- M17 `:179-181` — `With acknowledgements the message is redelivered. With \`AckPolicy: "none"\`, or on a core
  subscription, it is lost` — **unmeasured** (the scenario test asserts only that the message is not settled).
- M18 `:191-194` — a handler panic is recovered and the message Nak'd (README `:50` adds "so the server redelivers
  it") — **test**: Nak redelivery by `TestIntegration_ConsumeStreamWithConfig_Nak` (`natsclient/stream_integration_test.go:455`,
  container); the panic path itself is own, fake only.
- M19 `:209-212` — the setup context of `ConsumeStreamWithConfig` and `ConsumeInternalStreamWithConfig` bounds setup
  only — "not retained" **test**, own (`natsclient/consume_context_test.go:79`, fake); "bounds setup" **contradicted**
  under a held write: P11 consume held to 59.6 s, P13 consume held to 1.5 s, each with a 50 ms setup context.
- M20 `:240-247` — retrying requests refuse a negative count, check the context before each attempt, and do not count
  a caller's cancellation — **test**, own: `TestReviewerNegativeRequestRetryRefuses`
  (`natsclient/reviewer_full_nats_probe_test.go:12`, embedded server), `natsclient/request_cancel_breaker_test.go`.
- M21 `:268-274` — the storage report view changes at the watch's sync marker — **unmeasured** against nats.go (fake
  only: `fakeKeyWatcher`, `natsclient/storage_report_consumer_test.go:23`); that a nats.go KV watch ends its initial
  values with one nil entry is a library fact no probe here measures.
- M22 `:295-298` — Subscribe returns only after a PING/PONG on the subscription's connection, so a request from any
  connection finds it, unless refused — **probe**: P1, P5 (median 21 µs, p99 96 µs on loopback), P7; test T1
  (`subscribe_registration_test.go:240`).
- M23 `:300-301` — the wait for the PONG ends when the server answers, when the context ends, or at
  `DefaultRequestTimeout` — **probe**: P3 (5 s on the synctest clock), P9 (301 ms for a 300 ms context with no held
  write); tests `TestSubscribeRegistrationBoundedByDefaultRequestTimeout` (`:296`), `…CancelledDuringRoundTrip` (`:318`).
- M24 `:301-302` — an ended context sends no SUB and returns a transient error matching it — **test**:
  `TestSubscribeRegistrationEndedContextSendsNoSub` (`:345`); P2 shows nats.go alone would race (20/20 cancelled, not
  guaranteed).
- M25 `:304-310` — the context and `DefaultRequestTimeout` bound the PONG wait only; a held write holds the call
  `until that write completes or nats.go's write timeout (one minute by default) ends it; a call behind more than
  one held write waits for each in turn` — **probe**: P9 (3.0 s with a 3 s write timeout), P6 (`ft=1s` 1.703 s, two
  timeouts), P13 (released 0-29 ms after the server reads again, 16/16), P14 on `main` (`kvput` 1 m 59.6 s twice,
  `close` 3 m 1.6 s, `subscribe` and `close` still held at 3 m 20 s): consistent with "each in turn"; no ceiling.
- M26 `:312-320` — a failed call unsubscribes, is not restored on reconnect, and does not wait for a running handler;
  Close joins it — **probe**: P2 (`Unsubscribe` while reconnecting is not resent), P3 (the server reads `UNSUB 1`);
  tests `…ReturnsWhileHandlerHeld` (`:390`), `…CloseBeginsDuringRoundTrip` (`:434`).
- M27 `:322-327` — error order: `nats.ErrConnectionClosed` once Close has begun; transient context errors; transient
  `ErrNotConnected` (not `ErrConnectionClosed`) on a lost connection — **probe**: P4 (a pending flush returns
  `nats.ErrConnectionClosed` at once while `RECONNECTING`); tests `…ConnectionLostDuringRoundTrip` (`:364`),
  `…CloseEndsConnectionDuringRoundTrip` (`:479`). The class of the first branch differs by tree (M14).
- M28 `:327-328` — `with nats.go v1.54.0 the call returns the subscription and nil` for a refused SUB — **probe**: P8
  (base: 0 of 1,300 denied subscribes refused), C-ds (`nonnil_handle=true err=<nil>`), P7.
- M29 `:336-337` (scenario) — `the handler answers it, and the request never fails with "no responders"` on a
  permitted subject — **probe**: P1 (0/2000 after the round trip); test T1.
- M30 `:341-344`, `:348-351` (scenarios) — `DefaultRequestTimeout` and cancellation end the round trip and the server
  reads an UNSUB — **test** (synctest fake server; real nats.go over `net.Pipe`): T2, T3 (`:296`, `:318`); P3.
- M31 `:358-362` (scenario) — a dropped connection gives `ErrNotConnected`, not `ErrConnectionClosed`, and the
  subscription is not restored — **probe**: P4; test T6 (`:364`).

**Other `openspec/specs/*/spec.md`** (searched: `git grep -n -i -E 'nats|jetstream|natsclient' -- 'openspec/specs/*/spec.md'`;
hits outside `transport-client`: `harness-boundaries` 15, `integration-test-runner` 6, `nats-fixture` 9; only
`nats-fixture` states nats.go or nats-server behaviour; the other two name images, paths and variables)

- M32 `openspec/specs/nats-fixture/spec.md:98-101` — `Stop(ctx) SHALL, under the caller's context and in order, …
  drain and close the connection, …; When ctx ends first, Stop SHALL return ctx.Err()` — idle **test** (fixture's
  own tests); the drain is nats.go's `Drain` on the fixture's own connection, the same lock (#168's class):
  **unmeasured** under a held write.
- M33 `nats-fixture/spec.md:103-106`, `:114-118` — `Every consumer's delivery SHALL be stopped, its running handlers
  joined, … even when the connection is already closed` — **test** (fixture integration tests); the nats.go part
  (handlers after a closed connection) is not separately probed.
- M34 `nats-fixture/spec.md:146-149`, `:150-155` — `a file-backed stream keeps its messages and a memory-backed
  stream does not` across a Restart — **test** (fixture integration test, container); M3 is the natsclient twin.
- M35 `nats-fixture/spec.md:220-227` — a broker `max_payload` is enforced: `the broker refuses it` — **test**
  (`TestMaxPayloadIsSettable`, `internal/harness/natsfixture/fixture_integration_test.go:1029`, container). Which side
  refuses (nats.go's client check against the server's INFO, or the server) is not stated and **unmeasured**.

**`natsclient/README.md`**

- M36 `:3`, `:9-11` — `automatic reconnection`; `proper context propagation throughout all operations` —
  reconnection **probe** P15 (status `reconnecting` 11 ms after the server stopped, `connected` 1.6 s after it
  restarted, a responder made before the outage answered after it); "context propagation" as a bound:
  **contradicted** by P12 (Connect's dial ignores the context) and P13/P14 (held writes).
- M37 `:28-30` — circuit opens after 15 consecutive failures, then tests the connection with exponential backoff —
  own (`client.go:348-349`, `circuitThreshold: 15`, `maxBackoff: time.Minute`); **test**
  `TestIntegration_CircuitBreakerWithRealConnection` (`integration_test.go:163`, container).
- M38 `:34-35` — `Disconnected → Connecting → Connected → Reconnecting (on failure) → Connected. The client handles
  all transitions automatically` — **probe** P15 (Reconnecting then Connected, 2/2 runs); test
  `TestIntegration_ReconnectionIsRedial` (`integration_test.go:52`) proves re-dial with `WithMaxReconnects(0)`, not
  nats.go's automatic reconnect (`docs/admission-ledger.yaml:2058-2059` — `name and comment say it proves re-dial, not
  nats.go's automatic reconnect`).
- M39 `:49-52` — panic → Nak'd `so the server redelivers it`; an unsettled message is redelivered `when its AckWait
  expires` — Nak redelivery **test** (M18); AckWait-expiry redelivery **test** by the heartbeat-stopped case
  (`delivery_settlement_integration_test.go:140`); for a plain Consume handler that returns unsettled: **unmeasured**.
- M40 `:92-93` — `When \`Subscribe\` returns, the server has read the subscription, … unless the server refused it. A
  refusal is not returned as an error; a client that connected with \`Connect\` logs it.` — **probe** P1, P7, P8, C-ds;
  the log line is own, untested by a probe (C-ds used `SetConnection`, whose caller's handler logged).
- M41 `:173-178` — `KVOptions.Timeout … // Operation timeout (default: 5s)` — **contradicted** as a bound: P14 `kvput`
  returned at 1 m 59.6 s (2/2) with the default 5 s and a 50 ms context; P11 `kvget`/`kvput` held to 1 m 1.7-1.9 s.
  Holds idle: P13 idle `kvget` 0 s.
- M42 `:220-221` — `Every I/O operation requires context.Context as first parameter for proper cancellation and timeout
  support` — **contradicted**: P12 (a 50 ms context; Connect returned at 5.001 s, the dial timeout), P13/P14 (every
  measured call held past its context while the server does not read). Also `Client.RTT` takes no context
  (`client.go:1152` — `func (m *Client) RTT() (time.Duration, error) {`).
- M43 `:237-239` — `WithMaxReconnects(-1) … (default: -1)`, `WithReconnectWait(2*time.Second) … (default: 2s)`,
  `WithTimeout(10*time.Second) // Connection timeout (default: 5s)` — defaults own (`client.go:344-350`); the timeout's
  effect **probe** P12 (5.001 s default; 1.001 s with 1 s; 2.003 s for two URLs at 1 s each: one timeout per URL).
- M44 `:249-273` — the error list and `errors.Is` / `IsKVNotFoundError` patterns — **probe** P10 (class of each; §2,
  area 3). `ErrNotConnected` and `ErrCircuitOpen` are bare `errors.New` values (`client.go:53-54`) that no class
  check reports transient (P10, P15).
- M45 `:307` — `Circuit Breaker: Adds minimal overhead, fails fast when open` — own; not measured for cost.
- M46 `:340-352` (example "Resilient Publisher") — `// Publish with automatic reconnection handling` and a branch on
  `ErrCircuitOpen` — **contradicted** for the reconnect half: P15, `Publish` while reconnecting returns bare
  `ErrNotConnected` at once (2/2), not buffered; `Publish` has no circuit check (`client.go:1447-1468`, read), so the
  example's branch is never taken.

**`natsclient/doc.go`** (package doc; shown by `go doc`)

- M47 `:4-6` — `exponential backoff for reconnection, and proper context propagation throughout all operations` — as
  M36 and M53.
- M48 `:15-17` — the status lifecycle with Reconnecting — **probe** P15, as M38.
- M49 `:47` — `// Handle message with context (30s timeout per message)` — own (`client.go:1313` —
  `msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)`, a child of Subscribe's own context).
- M50 `:75-80` — `On a clustered deployment it is equally what a node that has not applied the meta assignment says
  about a stream that exists` — **unmeasured** (no cluster probe; every test and probe here runs one server).
- M51 `:82-88` — the consumer setups `wait out the propagation window` and return `ErrStreamNotVisible` only after the
  whole budget — own logic, **fake only** for the window (`natsclient/stream_visibility_test.go`); the cluster
  behaviour it waits out is **unmeasured** (M50).
- M52 `:108-110` — `0 and -1 both mean unlimited to JetStream` — **probe** P16 (MaxBytes 0 and -1 both stored as
  -1; MaxAge 0 stored as 0, unlimited). MaxAge -1: **unmeasured**.
- M53 `:111-113` — `at a finite MaxBytes it decides whether the oldest messages are evicted or the newest are refused`
  — **probe** P16 (DiscardNew: 5 of 20 publishes stored, then `err_code=10077 maximum bytes exceeded`; DiscardOld: 20
  of 20 returned nil, stream holds the last 5, FirstSeq 16).
- M54 `:143-147` — `Closed reports the end of a drain that completed, but it can report the end early once delivery
  was stopped without a drain or the connection was force-closed (nats.go jetstream/pull.go:822-837)` — **test**:
  `TestClientCloseJoinsConsumerHandlerWhenClosedReportsEarly` (`client_close_final_test.go:654`, embedded JetStream
  server); tracked upstream-side by #83 ("re-check on each nats.go bump").
- M55 `:159-160` — `client.Close stops and joins the consumer if the caller has not, and its nil return proves no
  handler still runs` — **test** (M9).
- M56 `:336-338` — `Only the first Close() drains; a later Close() waits for the same completion` — own; consistent
  with M11 only when read as "within its own context"; **test** M7.
- M57 `:352-353` — `Reconnection uses exponential backoff to avoid overwhelming the server during failures. Maximum
  backoff is one minute.` — **unmeasured**. Read: natsclient passes `nats.ReconnectWait(m.reconnectWait)` (2 s,
  `client.go:637`, `:345`) and no `CustomReconnectDelay`; nats.go v1.54.0's default reconnect wait is fixed with
  jitter (`nats.go:57-58` — `DefaultReconnectWait = 2 * time.Second` / `DefaultReconnectJitter = 100 *
  time.Millisecond`); the one-minute exponential backoff is the circuit breaker's (`client.go:349`). A probe of the
  reconnect pacing is listed in §2.
- M58 `:392-430` — the unified RPC error contract (ADR-060 at the pin): a reply with no `X-Status` is success;
  `RequestClassified` surfaces transport and handler failures `uniformly` — handler-class round trip **test**
  (`natsclient/errors_integration_test.go:24`, `:174`, container); the transport half is M63.
- M59 `:448-449` — `Context-First API: Every I/O operation requires context.Context … for proper cancellation and
  timeout support` — **contradicted**, as M42.

**Exported doc comments** (listed with `/tmp/pr177/tools/doclist/doclist` over the tree's non-test files, then
filtered for time, order, bound and failure words; 2,289 doc lines on `main`, 640 keyword hits, read in full for
`client.go`, `request.go`, `errors.go`, `kv.go`, `stream.go`, `options.go`; the storage files' hits name natsclient's
own report logic and are covered by M21 except where listed; revision 2: wrong for three of them, now M101-M103)

- M60 `natsclient/client.go:684-691` (Connect) — refuses nil, a second Connect and Connect after Close `before any
  dial`; a failed Connect leaves no event handler running — **test**, own: `natsclient/client_connect_test.go:13`, `:48`,
  `:84` (embedded server). What bounds the dial is not in the doc comment; the unexported comment `client.go:697-698`
  — `production uses` / `nats.Connect synchronously with the timeout in buildConnectionOptions` — **probe** P12
  (5.001 s for a 50 ms context; a context that had already ended still dialled: listener accepts=1, then
  `context canceled` at 1 ms).
- M61 `client.go:884-902` (Close) — `When ctx ends first, or has already ended, Close returns ctx.Err() at once, even
  while another Close is draining` — **contradicted** (M2); idle **test** M6, M11. `A drain that runs out of time is
  reported by the first Close as a transient error wrapping nats.ErrDrainTimeout` — **test** M10.
- M62 `client.go:1214-1220` (Subscription.Drain) — returns once the delivery goroutine has exited; `If the connection
  closes before or during the drain, Drain still returns nil …; messages queued but not yet delivered are discarded,
  because core NATS is at-most-once`; a later call rejoins the same native drain — **fake only**
  (`natsclient/subscription_test.go:19` — `type fakeNativeSubscription struct {`; e.g. `:146`
  `TestSubscriptionDrainConnectionClosesMidDrain`): **unmeasured** against nats.go.
- M63 `client.go:1254-1271`, `request.go:364-381` (Subscribe, SubscribeForRequests) — as M22-M28: the round trip
  **probe**; `ctx does not end a wait for the connection itself … until that write completes or nats.go's write
  timeout (one minute by default) ends it, so it can be held a minute or more` — **probe** P6, P9, P13, P14
  (`subscribe` held past 3 m 20 s in one run of two).
- M64 `client.go:1273-1285`, `request.go:383-395` — the error order — **probe** P4, tests (M27); class by tree (M14).
- M65 `client.go:1446` (Publish) — `Publish publishes a message to a NATS subject`: no bound or nil meaning stated.
  Measured: held past a 50 ms context while the server does not read (P13 1/1, P11 1/1), returned nil at
  1 m 59.6 s and 2 m 1.7 s with the server never reading (P14 2/2): a nil Publish means "in nats.go's buffer", not
  "sent"; while reconnecting it returns bare `ErrNotConnected` at once (P15 2/2).
- M66 `client.go:1552-1568` (PublishToStreamWithMsgID) — `the server's duplicate-detection window collapses
  re-publishes`; `the NATS server default is 2m when unset` — **probe** P16 (Duplicates=2m0s reported; two publishes
  with one ID store 1 message).
- M67 `client.go:1718-1733` (PublishBatchToStream) — `Per-subject ordering … is preserved (jetstream-go async is
  in-order per connection, absent a NoResponders retry …)` — **test** `TestIntegration_PublishToStreamAsync_Ordering`
  (`natsclient/publish_async_integration_test.go:79`, container); `waits for all acks (bounded by ctx)` / `If ctx is
  cancelled before all acks arrive, it returns the context error rather than hanging` — idle **test**
  `TestIntegration_PublishBatchToStream_CtxCancel` (`:308`); under a held write **unmeasured** (`PublishMsgAsync`
  writes under the same lock).
- M68 `client.go:1799-1805` (GetStream) — a clustered node `that has not applied the meta assignment answers it for a
  stream that exists` — **unmeasured** (M50).
- M69 `client.go:1954-1956` (WaitForBucket) — `retrying until the timeout expires or the context is cancelled` — own;
  **unmeasured** under a held write (each attempt is a JetStream request: P13 `getbucket` was held).
- M70 `request.go:33-41` (DefaultRequestHandlerTimeout) — `caps how long a handler may run before its context is
  cancelled` — own; **test** `TestIntegration_SubscribeForRequests_HandlerTimeoutConfigurable`
  (`request_integration_test.go:251`). It cancels the context; it does not end the handler.
- M71 `request.go:81-86` (readiness probe timeout and budget) — `bounds the TOTAL wall-clock spent waiting` — own;
  **unmeasured** under a held write (each attempt is a request: M72).
- M72 `request.go:174-177` (Request) — `The timeout parameter controls how long to wait for a response` — idle
  **test** `TestIntegration_Request_Timeout` (`request_integration_test.go:50`) and P13 idle (51 ms); **contradicted**
  under a held write: P6 (59.7 s on `a85947d`), P11 59.6 s, P13 held to 1.5 s, P14 59.6 s (and 1.7 s in one run).
- M73 `request.go:182-184`, `:502-504`, `errors.go:298-300` — "no responders" is `transient` at startup or after a
  reconnect — prose; `errs.IsTransient(nats.ErrNoResponders)` is false on both trees (**probe** P10, P15). At nats-server
  2.14.7's defaults a request with no subscriber returns `nats.ErrNoResponders` at once (P15, 0 s, 2/2).
- M74 `errors.go:216-217` (RequestClassified), `:251-253` (RequestWithRetryClassified) — `Transport failures (no
  responders, timeout) are returned as the underlying error and classify as ErrorTransient via pkg/errs.IsTransient`
  — **contradicted** for no responders on both trees (P10, P15: `IsTransient=false`); for `nats.ErrTimeout` true on
  `main`, false at #93 (P10); the deadline path returns `context.DeadlineExceeded`, true on both (P10, P13). The
  not-connected path (bare `ErrNotConnected`, P15) is false on both and not listed by the comment.
- M75 `errors.go:297-304` (IsNoResponders) — `whether an absent responder surfaces as ErrNoResponders vs a plain
  timeout is server-config dependent` — default config **probe** P15 (no responders, at once); the configuration
  dependence **unmeasured**. The cited test accepts either by comparing error text (`request_integration_test.go:70-74`,
  `:429-434`), so it measures neither.
- M76 `kv.go:29` (KVOptions.Timeout `Operation timeout`) — as M41.
- M77 `kv.go:99-109` (BucketLastSeq) — `LastSeq is monotonic even under History=1 — a purge raises FirstSeq but never
  lowers LastSeq`; same sequence space as revisions — **probe** P16 (2 → 3 after Delete → 4 after Purge → 4 after
  PurgeDeletes; FirstSeq 5).
- M78 `kv.go:206`, `:226`, `:249` (Put last-writer-wins; Create errors if the key exists; Update CAS) — **test**
  (`natsclient/kv_integration_test.go:273`, container); Create's refusal is what ADR-111 Decision 2 relies on (M94).
- M79 `kv.go:323` (`UpdateWithRetryRev`) — `On any non-nil error the returned revision is 0 — nothing committed.` —
  **unmeasured** (revision 2; revision 1 had it contradicted, on the wrong call). The call reads first: in the
  reviewer's p15b, with nats.go reconnecting and a 200 ms context, `UpdateWithRetryRev` returned `retry cancelled after
  attempt 1 failed: context deadline exceeded` and nothing committed (`rmw = "v1" at revision 2` after the reconnect,
  3 of 3), because its `Get` failed before any write. The case the sentence would fail in, a connection lost between
  the read and the write, is not measured. What P15 and p15b do measure is that the writes it is built on, `Put`,
  `Create` and `Update`, can return an error and commit (M94; §2, area 2). #93 rewrites this sentence (§1b, N3).
- M80 `kv.go:506-507` (DeleteAtRevision) — one fenced attempt, never reads or retries — own; **test**
  `natsclient/kv_delete_revision_test.go`.
- M81 `kv.go:560-561`, `:583-584` (KeysByPrefix server-side filtering; KeysByFilter `A returned native watcher owns
  delivery until its Updates channel closes`) — filtering **test** `TestKVStore_KeysByPrefix` (`kv_integration_test.go:467`);
  the watcher ownership **unmeasured** against nats.go.
- M82 `kv.go:714` (Watch) — `does not apply timeout as it creates a long-lived watcher` — own.
- M83 `kv.go:733-734` — `The NATS Go SDK's public Get already maps ErrKeyDeleted → ErrKeyNotFound` — **probe** P16
  (Get after Delete: `ErrKeyNotFound` true, `ErrKeyDeleted` false).
- M84 `kv.go:728-731`, `:754` (IsKVNotFoundError, IsKVConflictError) — the classes — **probe** P10 (main reads the
  text: an error whose text holds `10071` is a conflict and `10037` a not-found; `APIError 10058 stream name in use`
  is a conflict on both trees).
- M85 `stream.go:307-326` (ErrStreamNotVisible) — produced only after the budget, never when the caller's context
  ended the wait — own, **fake only** (`stream_visibility_test.go`); its cluster premise **unmeasured** (M50).
- M86 `stream.go:427-437`, `:467-474`, `:723-730` (the three Consume methods) — `Closed can report the end early …`
  **test** (M54); `ctx bounds the setup only` **contradicted** under a held write (M19).
- M87 `delivery_settlement.go:273-276` — `Retry here is a bare Nak, which JetStream redelivers as fast as it can` —
  redelivery **test** (`delivery_settlement_integration_test.go:237`); "as fast as it can" **unmeasured**.
- M88 `options.go:57-68` (WithConnectionLossTimeout) — `if the connection has not recovered within grace, the callback
  runs once` — **test** `TestIntegration_ReconnectionIsRedial` (with `WithMaxReconnects(0)` only).
- M89 `options.go:93` (WithTimeout `sets the connection timeout`) — **probe** P12.

**Unexported comments that state nats.go behaviour by source line** (`grep -n 'nats\.go v1\.\|nats\.go:[0-9]\|
jetstream/[a-z_]*\.go:[0-9]\|context\.go:[0-9]' natsclient/*.go`, non-test: 20 lines in `client.go`, `doc.go`,
`owned_delivery.go`, `stream.go`). Each is a code read of v1.54.0, not a probe. Listed as one row each where it carries
a promise:

- M90 `client.go:759-760` — `nats.go v1.54.0 fails jetstream.New only when an option does` — **unmeasured** (read;
  the ledger says no failing-first test exists for it, `docs/admission-ledger.yaml:1577`).
- M91 `client.go:951`, `:2232` — nats.go ends a timed-out drain and closes the connection itself (`nats.go:6349-6390`,
  `:6374-6375`) — **test** M10 covers the reported error, not the mechanism.
- M92 `client.go:1053-1054`, `:1066-1067` — the unsynchronized write at the end of `Consumer.Info` `STILL PRESENT
  UPSTREAM in nats.go v1.54.0` — **unmeasured** by a probe (a read; guarded regardless).
- (`client.go:182`, `:1387`, `:1405`, `:1419`, `:1630`, `:2209`; `owned_delivery.go:16`, `:18`, `:95`, `:164`;
  `stream.go:429`, `:469`, `:725`; `doc.go:145`: mechanism notes behind M9, M16, M26, M27, M54, already counted.)

**`pkg/errs/doc.go`** (the classes natsclient's errors carry)

- M93 `pkg/errs/doc.go:15` — `Errors are automatically classified based on their type or content:` and `:192-193` —
  `falls back to` / `pattern matching for unknown errors` — **probe** P10: on `main` the class of eight raw nats.go
  sentinels comes from their text (`nats.ErrConnectionClosed`, `…Draining`, `…Reconnecting`, `…DrainTimeout`,
  `ErrTimeout`, `ErrStaleConnection`, `ErrNoServers`, `ErrInvalidConnection` are transient; an error reading
  `refused: connection label on entity` is transient too). Ruled out by ruling N (#91 comment 6080973822); #93 changes
  it (§1b).

**ADRs in `docs/adr`** (`git grep -n -i -E 'nats|jetstream|natsclient|timeout|deadline|within|bounded|reconnect' --
docs/adr/`; 102 and 104 hit only "bounded" in the identity-length sense; 108 hits a bounded wait for a bucket, a
SemEngine behaviour, not nats.go's)

- M94 `docs/adr/111-durable-execution-primitive.md:71-72` — an outcome is written with `the store's create-once write
  (natsclient/kv.go:211-212, ErrKVKeyExists :219); a refused create means an outcome already exists` — **test** (M78)
  for a refusal (`ErrKVKeyExists`), and **probe** (revision 2: the reviewer's p15b, 3 of 3) for the case the ADR does
  not address: `Create(new)` returned `kv create new: context deadline exceeded` with revision 0 while nats.go
  reconnected, and after the reconnect `new = "v1" at revision 3`. An error from the create-once write other than
  `ErrKVKeyExists` does not mean no outcome was written. Counted as probe.
  The ADR's pins read Put's body on `main` and at #93 (`natsclient/kv.go:213` — `rev, err := kv.bucket.Put(ctx, key,
  value)`); Create is `kv.go:226-247`; the ADR does not say which tree its lines are on.
- M95 `docs/adr/111-durable-execution-primitive.md:108-109` — `The fence on a NATS write is the store's own:
  Nats-Msg-Id within a duplicates window for a stream publish` — **probe** P16 (window 2 m; dedup inside it).
- M96 `docs/adr/111-durable-execution-primitive.md:133-136` — settlement `settled to the broker as ack, nak or term,
  with a progress heartbeat` — **test** (M4).
- M97 `docs/adr/110-write-seam.md:76-78` — `UpdateWithRetryRead` (`kv.go:344` on #93): the callback gets the revision it
  read; `ErrKVSkipWrite` declines; the result is only its own commit — at #93 **test**
  (`natsclient/kv_update_revision_integration_test.go`, #93 only, container); on `main` the symbol does not exist.

**Rows added in revision 2** (review r1: HIGH 1, HIGH 2, and the doc comments the scan passed over; not measured, by
the owner's time-box):

- M98 `docs/adr/110-write-seam.md:192` — `A watcher sees one revision per applied write and none for a declined one;
  that unit is available to ADR-109.` — **unmeasured**. A KV watcher in nats.go v1.54.0 is an ordered consumer with
  `DeliverLastPerSubject` (`jetstream/kv.go:1334-1336`, the reviewer's read) on a bucket ADR-110 keeps at `History` 1
  (`:92`). The pin's ADR-066, as #176 carries it, says the opposite for a watcher that lags: `History=1 +
  DeliverLastPerSubject means most` / `revisions ≤ LastSeq are purged and never delivered`
  (`docs/adr/066-honest-graph-index-readiness.md:408-409` at #176's head `e5475855`), reviewed against nats.go v1.48.0
  (`:69`).
- M99 `docs/adr/111-durable-execution-primitive.md:104-105`, `:116-117` — `a crash after the guard is written and
  before the acknowledgement redelivers`; `After a restart: (1) the transport redelivers unacknowledged work on a
  file-backed stream` — **unmeasured**: M4's tests show redelivery when the heartbeat stops
  (`delivery_settlement_integration_test.go:237`), not across a process or broker restart.
- M100 `natsclient/client.go:516-519` — `unless the server's typed PubAck says only that the target` / `stream reached
  one of its configured admission ceilings` — **unmeasured**. The code behind it reads the error's text as well as its
  type (`client.go:528-541`: `apiErr.ErrorCode != jetstream.ErrorCode(10077)`, then `switch apiErr.Description {` /
  `case "maximum bytes exceeded",` / `"maximum messages exceeded",` / `"maximum messages per subject exceeded":`), the
  same on both trees (§2, area 3).
- M101 `natsclient/storage_inventory.go:21-24` — `The server moves any` / `stream carrying an offlineReason out of the
  info listing and into` / `Missing/Offline, which the Go client drops on the floor; the NAME listing` / `does not
  filter them.` — **unmeasured**.
- M102 `natsclient/storage_inventory.go:421-422` — `The server does NOT exclude offline` / `streams here` —
  **unmeasured**.
- M103 `natsclient/storage_inventory.go:455-460` — the client's page offset and the server's cursor diverge, `so an
  account` / `with more than one page plus one offline stream serves overlapping pages` — **unmeasured**.
- M104 `natsclient/stream.go:100-103` — `Duplicates` … `Must be <= MaxAge or the server rejects creation` —
  **unmeasured**.

M100-M104 stand at #93's head on the same lines (`client.go` is unchanged there; `storage_inventory.go:23`, `:421`,
`:457`).

**`docs/admission-ledger.yaml`, the `natsclient` row** (`:1424`-`:2242`, 819 lines; `grep -c 'nats\.go'` in the row:
17). It restates M9, M16, M26-M28, M54, M90 and the version difference (`:1618-1619`); it cites no ADR (searched
`grep -o -i 'ADR-[0-9]*'` over `:1424-2242`: none). Not counted again.

### 1b. At #93's head (`f3d5ae56`): what differs

PR #93 leaves `client.go`, `request.go`, `stream.go`, `errors.go`, `delivery_settlement.go`, `owned_delivery.go` and
`doc.go` unchanged (`git diff --name-only c713840 f3d5ae56 -- natsclient pkg/errs`: `README.md`,
`backing_stream_prefix.go`, `kv.go`, `kvspec.go`, `storage_inventory.go`, `storage_report.go`,
`storage_report_consumer.go`, six test files, and `pkg/errs/{doc.go,errs.go}` with two test files). So every time
bound, nil meaning, ownership and reconnect row above (and P11 to P15, run on `main`) holds at #93's head as it does on
`main`, except for `KVStore`'s update loop, whose body moves (`kv.go:331-...` at #93) without a change to the calls it
makes.

Statements whose truth depends on #93 landing (each measured by P10 on both trees; the full table is in §2, area 3):

- **D1** — the class of what Close's refusal returns (M14, M27, M64): `errs.Wrap(nats.ErrConnectionClosed, …)`, built
  at 11 sites (`client.go:710`, `:730`, `:786`, `:818`, `:1342`, `:1397`; `owned_delivery.go:204`; `stream.go:496`,
  `:577`, `:692`, `:777`; `grep -n 'errs.Wrap(nats.ErrConnectionClosed' natsclient/*.go`), is transient by
  `errs.IsTransient` on `main`
  (its text holds "connection") and not transient at #93. `errs.Classify` calls it transient on both (#167).
- **D2** — M74's "timeout" half: raw `nats.ErrTimeout` is transient on `main`, not at #93; the no-responders half is
  false on both.
- **D3** — M84: an error whose text holds `10071` is a conflict, and `10037` a not-found, on `main` only; at #93 the
  classifiers read `jetstream.APIError` codes (`kv.go` at #93 — `func hasJetStreamErrorCode(err error, codes
  ...jetstream.ErrorCode) bool {`, `:788`). `APIError 10058` (stream name in use) is a conflict on both.
- **D4** — M93, replaced at #93 by N7.
- **D5** — M79, replaced at #93 by N3.
- **D6** — M97 (ADR-110 Decision 2's `UpdateWithRetryRead`) has no symbol on `main`.
- **D7** — eight raw nats.go sentinels lose `IsTransient` at #93 (P10: `ErrConnectionClosed`, `ErrConnectionDraining`,
  `ErrConnectionReconnecting`, `ErrDrainTimeout`, `ErrTimeout`, `ErrStaleConnection`, `ErrNoServers`,
  `ErrInvalidConnection`). natsclient returns all but `ErrDrainTimeout` raw from some path (Publish, Request, the
  native subscribe and unsubscribe); any caller that retries on `errs.IsTransient` changes behaviour when #93 lands.

Rows new or changed at #93's head:

- N1 `openspec/changes/setup-04a-02-ingest-kernel/specs/transport-client/spec.md:7-9` — `The callback of
  natsclient.KVStore.UpdateWithRetryRead SHALL get the stored bytes and the revision read, 0 for an absent key. …
  The returned revision SHALL be the call's own commit, or 0.` — **probe** P16 (revisions start at 1: two puts, LastSeq
  2) and **test** `natsclient/kv_update_revision_integration_test.go` (#93, container).
- N2 same file `:16-20` — `A conflict passes the newer revision` — **test** (same file).
- N3 `natsclient/kv.go:323-326` (#93) — `the returned` / `revision is 0 — the call attributes no commit to itself. That
  does not mean nothing committed: a write cut short by an infrastructure error may have committed, and the next run
  then reads it.` — **probe** P15 for the first half (a Put that returned `context deadline exceeded` committed); "the
  next run then reads it" **unmeasured** for `UpdateWithRetryRead`.
- N4 `kv.go:339-341` (#93) — `0 when the key is absent (never written, deleted or purged); no commit is revision 0` —
  **probe** P16 (Get after Delete is not found; Delete and Purge take revisions 3 and 4).
- N5 `kv.go:764-765`, `:774-776` (#93) — classes from type, never text — **probe** P10. `A replicated stream reports a
  wrong last sequence as 10164 where a single replica reports 10071` — **unmeasured** (no replicated stream here; P10
  shows only that the classifier accepts 10164).
- N6 `natsclient/README.md:279-282` (#93) — `No write ever commits at revision 0, so (0, nil) means the callback
  skipped` — **probe** P16; **test** N1.
- N7 `pkg/errs/doc.go:15-16` (#93) — `classified by their class (a ClassifiedError) or a wrapped sentinel, never by
  their text: an error with neither is unclassified, and Classify gives it ErrorTransient` — **probe** P10 (no #93 row
  changes class with its text; `Classify` is transient for all 52 rows, permission violation, maximum payload, bad
  subject and authorization included).
- N8 `openspec/changes/setup-04a-02-ingest-kernel/specs/nats-fixture/spec.md:7-13` — the helper opens `under a bounded
  context` and registers a close that runs `under a context with a deadline` — when the close is a natsclient `Close`,
  M2 applies: **unmeasured** for the helper (its fixture server keeps reading).
- G1-G3 (revision 2, review r1 HIGH 1): #93's `openspec/changes/setup-04a-02-ingest-kernel/specs/graph-ingest-recovery/
  spec.md`, graph-ingest's promises with a transport fact under each (revision 1 counted the deltas other than
  `transport-client` and `nats-fixture` in §3 and did not read them):
  - G1 `:5-15` `Acknowledged is not durable` (a memory stream loses an accepted, unapplied message at broker restart):
    the transport half is M3's, **test**.
  - G2 `:17-26` `Recovery on a file stream is redelivery` (`delivered again after a process` / `or broker restart`):
    **unmeasured**, as M99.
  - G3 `:28-41` `Settlement order` (`not delivered again while it is still being applied` past the acknowledgement
    wait): the transport half is M4's heartbeat, **test**.
  Whether #93's own tests drive G1-G3 against a real server was not read.
- Unchanged at #93 and still text-based: `natsclient/client.go:2335-2342` — `func isAlreadyExistsError(err error) bool
  {` … `strings.Contains(errStr, "bucket name already in use") ||` / `strings.Contains(errStr, "already exists") ||` /
  `strings.Contains(errStr, "stream name already in use")`, read by `CreateKeyValueBucket` (`client.go:1887`) — the
  #146 class on both trees; not a doc claim, so not counted, but area 3 needs it.

### 1c. Counts

| Tree | Rows | probe | test only | contradicted | unmeasured |
| --- | --- | --- | --- | --- | --- |
| `main` `c713840` (M1-M96, M98-M104; M97 has no symbol on `main`) | 103 | 29 | 28 | 16 | 30 |
| #93 `f3d5ae56` (M79 → N3, M93 → N7, M97 counted, N1-N6, N8 and G1-G3 added) | 114 | 34 | 32 | 16 | 32 |

Revision 2 moved M79 from contradicted to unmeasured and M94 from unmeasured to probe (the reviewer's p15b), and added
M98-M104 (unmeasured) and G1-G3 (#93: two test, one unmeasured). Revision 1's counts were 96 (28/28/17/23) and 104
(33/30/16/25).

Contradicted on `main`: M2, M6, M8, M11, M19, M36, M41, M42, M46, M47, M59, M61, M72, M74, M76, M86. Every one
but M46 and M74 is the held-write class (the caller's context does not bound a call while the server is not reading)
or Connect's dial (P12); M46 is the reconnect half of a README example (P15), M74 the class of transport failures
(P10, P15). Of the 28 "test only" rows, checked test by test: M24 and M30 drive real nats.go against a scripted
server over `net.Pipe` (`ownProcessBubble`); M7 and M56 rest on `TestClientConcurrentClosesEachHonourTheirContext`
(`client_lifecycle_test.go:682`), which never dials (`NewClient("nats://unused", …)` in a synctest bubble); M80 on a
fake bucket (`kv_delete_revision_test.go:18` — `func (b *deleteAtRevisionBucket) Delete(`); the other 23 drive an
embedded server or the container (M54's test, `client_close_final_test.go:654`, also builds a `fakeConsumerClient`). A
"test only" row has no posted probe: whether an in-tree test counts as the
probe the new rule asks for is open question Q2.

## 2. The ADR's five areas: what is measured, what is not

### Probes run for this inventory (local only until posted on PR #177)

Each directory holds `main.go`, a `README.md` with the question, system, versions, build and run commands, every
output whole and the source's sha256, and `out/` with one file per run (P10's two outputs are `out-c713840.txt` and
`out-f3d5ae56.txt` beside its source). Built in the `git archive` trees named. The reviewer reproduced P10, P12 and
P13, and ran P6 as posted beside P6b (its diagnostic moved after the resume) and p15b (review r1, preamble).

Every `/tmp/pr177/...` path in this file is local only. The probes are posted on PR #177, each README whole (source,
command, output): P10 comment 6109196968, P11 6109197143, P12 6109197294, P13 6109197438, P14 6109197611, P15
6109197772, P16 6109197927, p15b 6109378036. Added by the orchestrator after INVENTORY PASS.

| Probe | Directory | Tree | Runs | sha256 (source) | What it measures |
| --- | --- | --- | --- | --- | --- |
| P10 | `/tmp/pr177/probes/P10` | `main`, #93 | 1 each | `db78e351…` | class of 52 errors under `pkg/errs` and natsclient's classifiers |
| P11 | `/tmp/pr177/probes/P11` | `main` | 13 idle, 16 full | `76101802…` (run.sh `9428a7f4…`) | 13 calls behind a held write; diagnostics before the resume (P6's design) |
| P12 | `/tmp/pr177/probes/P12` | `main` | 4 | `dcf5b833…` | what bounds Connect |
| P13 | `/tmp/pr177/probes/P13` | `main` | 13 idle, 16 full | `ea88dcf4…` (run.sh `0a8dda8b…`) | the same 13 calls, server reads again at 1.5 s |
| P14 | `/tmp/pr177/probes/P14` | `main` | 10 | `79946bc3…` (run.sh `421b2faa…`) | 5 calls, server never reads again (cap 200 s) |
| P15 | `/tmp/pr177/probes/P15` | `main` | 2 | `d99d8170…` | 8 calls while reconnecting; no responder; restore after reconnect |
| P16 | `/tmp/pr177/probes/P16` | `main` | 1 | `b32a6a69…` | duplicate window, Get after Delete, LastSeq, Discard, unlimited bounds |

Held-write setup (P11, P13, P14): embedded nats-server v2.14.7 with JetStream on loopback; the client dials a proxy
that forwards both ways and can stop reading the client's side; a goroutine publishes 32 KB messages through the same
`natsclient.Client` until the count stops rising for 300 ms; then the call runs with a 50 ms context. Calls:
`Subscribe`, `SubscribeForRequests`, `Request` (a responder that never answers), `Publish`, `PublishToStream`,
`KVStore.Get`, `KVStore.Put`, `KVStore.UpdateWithRetry`, `GetKeyValueBucket`, `GetStream`,
`ConsumeInternalStreamWithConfig`, `RTT`, `Close`. P6's caveat applies: a fake or proxied server on loopback, darwin
socket buffers (autotuned); P14's two short holds show the buffer is not always full when the count stops.

### Revision 3: probes P17 to P25

Source, outputs and commands for P17 to P22 and P25 are in `/tmp/pr177/probes/P17/README.md` (one program, sha256
`3f789247…`; earlier runs used `74a4f3e0…` and `e2436bf1…`, which differ only in the case each later probe added). P23
and P24 are in `/tmp/pr177/probes/P23/README.md`. Trees: `0e13a73`, the branch merged with `main`, whose `natsclient/`
is `main`'s; `fix`, design revision 2's prototype; and `fix-r3-probe`, design revision 3's,
`/tmp/pr177/design-r2/fix-prototype.diff`. Every wait is capped at 5 s, or 8 s for P23.

| Row or item | Changed by | Result |
| --- | --- | --- |
| M2, M6, M8, M61 (`Close` within its context) | P17, P23, P25 | `0e13a73`: a 50 ms `Close` behind a held write is still held at the 5 s cap (4 of 4 runs). `fix`: it returns at 51-52 ms, and the native connection is closed (4 of 4). Revision 3 (P25): 51-52 ms, 4 of 4. During a reconnect dial to an address that does not answer (P23): `0e13a73` 4.69 s and revision 2 4.69 s, 3 of 3 each; revision 3, with `net.Dialer.Cancel`, 51-52 ms, 3 of 3. |
| M42, M59, M60 (`Connect` and its context) | P18, P25 | `0e13a73`: 5.0 s against a silent server, whether by deadline or by cancel; 10.0 s for two URLs; 5.0 s to an unroutable address; an ended context still dials (1 accept). `fix`: 300 ms or 301 ms in each case; an ended context returns at once, with 0 accepts. A reconnect after Connect's context has ended works on both trees. Revision 3 (P25): every case 301-302 ms, an ended context 0 s with no accept, and a reconnect after Connect works. A cancel with no deadline during the dial to the unroutable address: `0e13a73` 5.0 s, revision 2 5.0 s, revision 3 302 ms. |
| N-P2 (`Close` while a `Subscribe` is held) | P17, P21 | `fix`: 51 ms (2 of 2). `fix-nosub`, which is `fix` without the change that stops `subscribeOwned` holding natsclient's lock across `IsConnected`: past the 5 s cap twice, and 1.4 s and 1.5 s twice, when darwin's buffers took the write. |
| N-P3 (the connection after a write timeout) | P19, P22 | With a 1 s write timeout and the server reading again at 3 s, a request failed in 1 of 2 runs; with `ReconnectOnFlusherError` it worked in 2 of 2, after a reconnect that lost 19 of 71 and 20 of 48 accepted publishes (P19). With 300 ms and the server reading again at 1.5 s, the timeout ended the publisher's own write, and the connection stayed connected but carried no request for 5 s: 3 of 3 with no other change, and 3 of 3 with `ReconnectOnFlusherError`, which acts only when nats.go's background flusher fails. With the stale-connection check at 300 ms x 2 it reconnected and carried a request within 7 ms, 3 of 3 (P22, echo on the native connection; the first `ReconnectOnFlusherError` runs did not set it and are not evidence). |
| N-P4 (the stale-connection check while a write is held) | P20, P24 | It never fires while a write is held (2 of 2 at 5 s). With no write held, status goes to reconnecting at 1.21 s (2 of 2). Through `WithPingInterval(300ms)` with no write held (P24 idle): 1.209 s, 1.209 s, 1.205 s. The probes' proxy finishes a read in progress when it stops, so the first PING after the stop is answered; nats.go declares the connection stale at the third unanswered tick (`nats.go:5993-6013`, read). At the default 30 s: not measured. |
| Area 1 option list (`FlusherTimeout`, `ReconnectOnFlusherError`) | P19, P21 | A 1 s write timeout ends `Publish`, `Request` and `KVStore.Put` at about 0.6 s, and `Subscribe` at 1.6-2.0 s. With `ReconnectOnFlusherError`, `Subscribe` was held past 5 s in 1 of 2 runs, because the reconnect handshake through the stopped proxy held the lock. On a paced link, shorter timeouts fail more writes: 16 KB/s gave 5 per run at 1 s, 1 at 5 s, 0 at the default; 8 KB/s gave 3 per run at 2 s, 1 at 10 s, and 0 or 1 at the default (the 1 came after 60 s). No delivered payload was damaged. The pacing proxy's stalls on darwin exceed its rate, so the counts show the direction only. |
| Options `WithWriteTimeout` and `WithPingInterval` (design revision 3) | P24 | Both at 300 ms: the held publish failed with "i/o timeout", and the echo succeeded 3 ms after the server read again, after one reconnect (3 of 3). The write timeout alone: the same failure, then no echo within 5 s, `CONNECTED`, 0 reconnects (3 of 3). Each option refuses 0 and -1 s with an invalid-class error. |

### Area 1 — time bounds per call

Measured today:

- Every one of the 13 calls is held past a 50 ms context while the server is not reading and the send buffer is full
  (P13 16/16 at 1.5 s; P11 16/16; P6 on `a85947d` and `9d1dd87` for Subscribe, Request, Publish, Close; P9 for
  Subscribe after its PING was read; C-bw).
- A held call returns 0-29 ms after the server reads again (P13 16/16; C-bw 0.3 ms).
- With the server never reading again, a held call returns when the held writes ahead of it end by nats.go's write
  timeout, one minute each (`nats.go:68` — `DefaultFlusherTimeout     = time.Minute`; natsclient sets none,
  `client.go:634-657`): P14 `request` 59.6 s; `kvput` 1 m 59.6 s ×2; `publish` 1 m 59.6 s and 2 m 1.7 s (nil);
  `close` 3 m 1.6 s; `close` and `subscribe` still held at the 200 s cap. No ceiling was observed.
- With a short `FlusherTimeout` the hold ends at that timeout per held write: P6 (`ft=1s`: Subscribe 1.703 s, Close
  700 ms; `ft=300ms`: 51 ms and 602 ms), P9 (3.0 s with 3 s).
- Idle (server reading): every call returns at once or at its context (P13 idle: 0-11 ms; Request 51 ms by its
  context because its responder never answers; P6 tcp-idle).
- Connect: the caller's context does not bound the dial; nats.go's `Timeout` does, once per URL (P12: 5.001 s default,
  1.001 s with `WithTimeout(1s)`, 2.003 s for two URLs); a context that has already ended still dials (P12 `ended`:
  one accept, then `context canceled`).
- Subscribe's PONG wait: P1-P5, P9; tests T1-T8.
- After a write timeout the connection reports connected with no last error (P14 `request-2`, `subscribe-2`; P11's
  "before resume" lines), and a cleanup `Close` with a 10 s context then returned `context deadline exceeded` in 10 of
  16 P11 runs and 6 of 10 P14 runs (0 of 16 in P13, where no write timed out), and P11's `RTT` got no answer to its
  PING for 10 s after the resume (§0). Not explained; N-P3.

nats.go v1.54.0 options that bear on a held write and that natsclient does not set (revision 2, review r1 MEDIUM 4;
read, not measured; `git grep -n 'ReconnectOnFlusherError\|ForceReconnect\|MaxPingsOutstanding\|FlusherTimeout' --
natsclient`: no match):

- `FlusherTimeout(t)` (`nats.go:1382`): the write timeout itself, one minute by default (`:68`). Measured by P6 at 1 s
  and 300 ms and by P9 at 3 s: the hold ends at that timeout per held write. That the timed-out bytes are dropped while
  the connection stays connected is read, not measured (`nats.go:2369-2372`; #168 body `:104-106`); P6's posted 1 s
  runs lost nothing (43 of 43, 40 of 40). Corrected by the orchestrator after INVENTORY PASS (design review r1).
  Revision 3: after the timeout the connection can stay connected and unusable (P22: 0 of 3 runs answered a request
  within 5 s of the server reading again).
- `ReconnectOnFlusherError()` (`nats.go:1393`): when the background flusher's write fails, reconnect, or close if
  reconnecting is not allowed (`:4209-4215`). A publish that flushes inline because the buffer reached its limit
  returns the error to the caller instead (`:2346-2347`).
- `Conn.ForceReconnect()` (`nats.go:2622`).
- `PingInterval` and `MaxPingsOutstanding` (`nats.go:1333`, `:1343`): natsclient sets the interval to 30 s
  (`client.go:346`) and leaves the outstanding count at 2 (`:62`); whether the ping timer, which takes the same lock,
  declares a stalled server while a write is held is N-P4.

Not measured (new probes needed, one line each):

- N-P1 — `Close` called a second time, with its own 50 ms context, while the first is held (spec `:90-91`); `main`.
- N-P2 — `Close` (and `Publish`, `Request`) while a `Subscribe` is held at `conn.IsConnected()` under natsclient's
  own lock (`client.go:1339-1358`; #168 "read, not probed"); `main`.
- N-P3 — the connection after a write timeout: whether the dropped bytes tear a protocol frame, what the server does,
  what `IsConnected`, `LastError`, `Status()` report, and whether `Close` can complete (the P11/P14 cleanup failures,
  10 s `Close` failing in 16 of 26 runs, and P11's unanswered `RTT`; revision 1's Q5, moved here); `main`.
- N-P4 — stale-connection detection (`PingInterval` 30 s, `client.go:346`; nats.go `DefaultMaxPingOut = 2`,
  `nats.go:62`) while a write is held: whether the ping timer, which takes the same lock, is itself held (P14's
  ≥ 200 s holds suggest it is); `main`.
- N-P5 — `PublishBatchToStream` / `PublishToStreamAsync`, `WaitForBucket`, `RequestReadyClassified`,
  `Subscription.Drain(ctx)`, `KVStore.Watch` / `KeysByFilter` and a consumer's handler context, behind a held write;
  `main`.
- N-P6 — `natsfixture` Stop's drain behind a held write (M32); `main` (fixture code is unchanged by #93 except
  `open.go`).
- N-P7 — the same held-write matrix on Linux (CI's runner): darwin's socket-buffer autotuning shaped P14's two short
  holds; `main` (revision 1's Q6, first half, moved here).

### Area 2 — what a nil return means

Measured today:

- `Subscribe` / `SubscribeForRequests`: the server read the SUB (P1, P7), and may have refused it (P8 0/1,300
  refused, C-ds, P7: -ERR comes before the PONG, 1,000/1,000).
- `Publish`: the bytes are in nats.go's buffer, not sent (P14: nil at 1 m 59.6 s and 2 m 1.7 s with the server never
  reading; P13 nil after the hold); a publish with no subscriber is dropped without an error (core NATS; P1's
  inventory states it for publishes, measured for requests only).
- `PublishToStream`: the server stored it in the stream's storage class (test M3); `PublishToStreamWithMsgID` nil for
  a duplicate inside the window stores one message (P16).
- `KVStore.Put`, `Create` and `Update`: non-nil does not mean "not committed" (P15 for `Put`, 2/2; the reviewer's
  p15b for `Create` and `Update`, 3/3: each returned `context deadline exceeded` with revision 0 while nats.go
  reconnected, and its value stood after the reconnect, at revisions 3 and 4). `UpdateWithRetryRev` committed nothing
  in p15b because its first step, a `Get`, failed (M79).
- `Close`: everything owned has finished (tests, M9).
- `Connect`: connected and admitted (tests, M60).
- `RequestClassified` / `Request`: a reply arrived; a plain `Request` returns a handler's error body with a nil error
  (doc footgun, test `TestIntegration_LegacyRequest_SuccessBodyUnchanged`, `errors_integration_test.go:114`).

Not measured:

- N-P8 — `Publish` nil when the server denies publish permission (async -ERR); `UpdateWithRetry` (and at #93
  `UpdateWithRetryRead`) losing the connection between its read and its write (`Create` and `Update` alone are
  measured by p15b, revision 2); `main` and #93.
- N-P9 — a refused SUB for the subscription limit through natsclient (P7 measured nats.go; P8 measured permissions
  only); `main`.
- N-P10 — `Subscription.Drain` nil against nats.go (M62 is fake only), and a core subscription's queued messages at
  Close; `main`.

### Area 3 — error classes (by type and sentinel only: ruling N; #146)

Measured today (P10, both trees; P15):

- `errs.Classify` returns transient for all 52 rows on both trees, permission violation, maximum payload, bad
  subject, authorization and maximum subscriptions included (#167).
- `errs.IsTransient` is true on both trees only for the two context errors and what natsclient wraps with
  `errs.WrapTransient` (Subscribe's failures, Close's drain timeout, Connect's dial errors). It is false on both for
  `nats.ErrNoResponders`, `natsclient.ErrNotConnected`, `natsclient.ErrCircuitOpen` (all three returned bare: P15 for
  the first two), `nats.ErrSlowConsumer`, `ErrMaxPayload`, `ErrPermissionViolation`, `ErrAuthorization`,
  `ErrMaxSubscriptionsExceeded`, `ErrReconnectBufExceeded`, every jetstream sentinel and API error, and
  `ErrKVMaxRetriesExceeded`.
- Eight raw nats.go sentinels and Close's refusal change from transient to not transient when #93 lands (§1b D1, D7).
- `natsclient.ErrCircuitOpen` (`client.go:54` — `ErrCircuitOpen  = stderrors.New("circuit breaker is open")`) and
  `errs.ErrCircuitOpen` (`pkg/errs/errs.go:79` — `ErrCircuitOpen        = errors.New("circuit breaker open")`) are two
  sentinels for one fact; `errs.IsTransient` knows only the second, natsclient returns only the first (P10).
- Text classification left after #93, on both trees (complete by the reviewer's sweep of `.Error()` and `Description`
  reads in non-test natsclient files at `f3d5ae56`): `client.go:2335-2342` (`isAlreadyExistsError`, §1b) and
  `client.go:528-541` (`isCircuitNeutralStreamCapacityError`: code 10077, then `apiErr.Description` matched against
  three strings; it decides whether a failed JetStream publish counts toward the circuit; M100). Revision 1 listed
  only the first.
- Wire classes (ADR-060 at the pin): `classForHeader` stamps `errs.Classify`'s answer (`natsclient/errors.go:311-322`),
  so an unclassified handler error goes out as `transient` on both trees (read; P10 shows `Classify`'s default).

Not measured:

- N-P11 — an end-to-end table: drive natsclient to each refusal (closed client, oversize payload, bad subject, denied
  publish and subscribe, DiscardNew's 10077 (which reaches `client.go:528-541`), circuit open, reconnecting, drain
  timeout) and class the value it
  actually returns, on `main` and #93 (P10 classes shapes built from a code read, not returned values).
- N-P12 — `IsKVConflictError` on a replicated (R3) stream's wrong-last-sequence error (10164, N5); needs a three-node
  cluster; #93.

### Area 4 — ownership, and what Close joins

Measured today: tests only (M9-M13, M54-M55; PR #48 design D3, archived at
`openspec/changes/archive/2026-10-05-setup-04a-01-floor/design.md:286-296`, and #156's D4,
`…/2026-10-10-subscribe-registers-interest/design.md:112`): a nil Close joins every handler invocation, the event
handlers of the connection it dialled, the async publish error handler as far as nats.go lets it; a Subscribe that
fails does not wait for its running handler and Close joins it (T7, T8); `SetConnection`'s connection stays the
caller's. The nats.go facts behind them are code reads ("orderings inside nats.go itself, which the nats.go v1.54.0
cites in this section stand in for", `openspec/changes/archive/2026-10-05-setup-04a-01-floor/design.md:470-471`).

Not measured:

- N-P13 — the residual: the async publish error handler's goroutines have no end signal (`client.go:1630`); whether
  one can run after a nil Close; `main`.
- N-P14 — a delivery nats.go hands over after the recorded end (M16) and its redelivery or loss by ack policy (M17),
  against a real server; `main`.
- (N-P1 and N-P2 above also bear on ownership: who holds Close up.)

### Area 5 — reconnect

Measured today:

- nats.go alone (P2, P4): a round trip started while reconnecting waits for its deadline, or returns nil once
  reconnected; `Conn.Subscribe` while reconnecting succeeds and writes no SUB; `Unsubscribe` while reconnecting is not
  resent; a pending flush returns `ErrConnectionClosed` at once when the connection drops; `RTT` is refused.
- natsclient (P15, 2/2): status `reconnecting` 11 ms after the server stopped; while reconnecting `Publish`,
  `Request`, `RequestClassified`, `Subscribe`, `PublishToStream` and `GetStream` return bare `ErrNotConnected` at once
  (not transient by `errs.IsTransient`), while `KVStore.Get` and `KVStore.Put` wait for their 200 ms context
  (`context deadline exceeded`, transient) and the Put's bytes, held in nats.go's reconnect buffer, commit after the
  reconnect (revision 2); a responder made before the outage answers a request from another connection after it;
  status `connected` 1.6 s after the server restarted.
- `TestIntegration_ReconnectionIsRedial` (`integration_test.go:52`): with `WithMaxReconnects(0)`, the loss callback
  fires and a new client re-dials; not nats.go's automatic reconnect.

Not measured:

- N-P15 — reconnect pacing (M57's "exponential backoff … one minute"): time between attempts with the server down;
  `main`.
- N-P16 — the reconnect buffer's limit (`nats.go:64` — `DefaultReconnectBufSize   = 8 * 1024 * 1024 // 8MB`) through
  natsclient's KV and JetStream paths, which do not check `IsConnected`: what a KV write returns past 8 MB, and
  whether earlier buffered writes still commit; `main`.
- N-P17 — consumers across a reconnect: `ConsumeStreamWithConfig` delivery, heartbeat settlement and Close's join
  when the connection drops and returns; `main`.
- N-P18 — failover across a multi-URL client and a cluster (M50, M68); needs a cluster; `main`.
- N-P19 — server-config dependence of no responders (M75: a server or client without headers support); `main`.

### Per-area summary

| Area | Measured by | Probes still needed |
| --- | --- | --- |
| 1 Time bounds | P1-P6, P9, P11-P14, C-bw; tests | N-P1 to N-P7 |
| 2 Nil meaning | P1, P7, P8, P13-P16, C-ds; tests | N-P8 to N-P10 |
| 3 Error classes | P10, P15 | N-P11, N-P12 |
| 4 Ownership | tests (embedded server) | N-P13, N-P14 (and N-P1, N-P2) |
| 5 Reconnect | P2, P4, P15; one test | N-P15 to N-P19 |

## 3. The rule

### Where the existing probe-citation rule lives

- `.agents/contracts/semengine-architect.md:54-61` — `- **Probe the pin before designing its port.** A pin probe is a
  run of this repository's checks against a copy of` … `Every design statement about how the pin behaves cites a
  probe result,` / `as step 5 of the workflow above requires of any premise.`; `:63-65` its source (issue #53) and
  scope (`They bind changes 2 to 7 of Slice 04A and later` / `work`). It is under "Extraction slices": porting designs
  only, and about the SemStreams pin, not a library.
- `.agents/contracts/semengine-architect.md:32-34` — `5. State every premise a design rests on as a measurable claim
  with the measurement attached` — the general premise rule the pin-probe rule cites.
- `.agents/contracts/semengine-reviewer.md:114-116` — `Four further checks, scoped as the architect contract
  (Extraction slices) states:` / `- A design statement about how the SemStreams pin behaves, with no pin-probe result
  behind it, is a finding.`
- `AGENTS.md:99` — `| A porting design cites a pin probe (this repository's checks run on a copy of the SemStreams pin)
  for every statement about how the pin behaves | architect contract § Extraction slices; reviewer contract §
  Pre-owner design review | review only |`.
- Adjacent, not about libraries: `.agents/contracts/semengine-reviewer.md:150-151` — `**Reject artifact-free
  evidence.** A gate/measurement claim with no in-tree or CI artifact is recorded UNVERIFIED`; `AGENTS.md:111` —
  evidence `can be opened on another machine; a path git ignores is named as local only`; `.agents/protocol.md:47-49`
  — `This rule adds no pin experiment per finding. Existing pin-probe obligations and evidence requirements still apply
  to claims about pin behavior.`; `.agents/README.md:68-69` — `The architect contract's` / `rules on pin probes, open
  pull requests and behaviour-not-mechanism remove the causes of most of them.`
- Library-behaviour precedents without a general rule: `docs/setup-plan.md:235-236` — `Document behavioral pins,
  especially NATS client regressions. Upgrades need relevant regression evidence; avoid mixing` / `blanket upgrades
  with extraction.`; `docs/testing.md:474-481` (the synctest bubble rule, measured by PR #156: "3 of 3", "2 to 7 of
  20", `AGENTS.md:82`); issue #83 (`re-check on each nats.go bump`).

Searches that found no library-probe rule: `git grep -n -i 'library' -- AGENTS.md .agents/ docs/testing.md
docs/provenance.md` (one hit, `semengine-developer.md:6`, "a generic Go library", unrelated); `git grep -n -i
'probe' -- .agents/ AGENTS.md` (only the pin-probe lines above).

### Where the new rule's text would go (places, not a choice)

- `AGENTS.md` rule table, `:71-121`: beside `:99` (the pin-probe row) or after `:121` (the last row). The table's own
  rule (`AGENTS.md:66-67`, "A change that adds a repository-wide rule or a rule of agent conduct … adds its row here")
  requires the row.
- Architect contract: workflow step 5 (`:32-34`), § Extraction slices first bullet (`:54-65`, port-only today), or
  § Design discipline (`:323-366`). #170's rule covers "a spec requirement, design statement or doc comment" in any
  change, which is wider than § Extraction slices' scope.
- Reviewer contract: `:114-116` (pre-owner design review; scoped to Extraction slices), § Contract and task-truth
  review (`:137-160`, next to "Reject artifact-free evidence" at `:150-151`), or § Guarantee, signal, and revision
  review (`:310-327`).
- Not named by #170, but the rule covers their artifacts: the developer contract writes doc comments
  (`.agents/contracts/semengine-developer.md:28` lists `doc comments` among a change's outer layers; § Guarantee,
  signal, and revision contracts `:138-162`); the technical writer owns README and docs
  (`.agents/contracts/semengine-technical-writer.md:30`, rule 7, Go doc comments). Q3.

### Could a check be built: measured

Script: `/tmp/pr177/tools/shallcount/count.py` (local only), run as `python3 -I count.py /tmp/pr177/trees/c713840`.
It splits each `openspec/specs/*/spec.md` into sentences and keeps those holding `SHALL`.

- `main`: 282 SHALL sentences; 18 match `\b(bound|bounds|bounded|within)\b`; 33 match a time word (deadline, timeout,
  at once, wait, until, seconds, minutes); none names a probe (the one regex hit, `process-host:31`, is "an alive
  probe", the process host's own operation).
- The 18, by what bounds what: `transport-client` 4 — `:54-58` (Close, M6), `:85-91` (Each Close, M11), `:209`
  (setup, M19), `:322-327` (Subscribe's bound) — nats.go claims, three of them contradicted under a held write;
  `nats-fixture:59`, `integration-test-runner:122`, `:135` — testcontainers and Docker; the other 11 bound this
  repository's own code (`background-work:51`, `harness-boundaries:310` ×2, `integration-test-runner:91` ×2,
  `lifecycle-suite:11`, `mutation-check:157` ×2, `nats-fixture:181` (MaxAge/MaxBytes bounds), `process-host:31`,
  `:49`).
- #93's deltas (`count_dir.py` over `openspec/changes/setup-04a-02-ingest-kernel/specs`): 90 SHALL sentences, 5
  matches, none about nats.go timing ("Within one write", "Within one generation", the lifecycle budget, and N8's
  "bounded context").
- What such a check cannot see: the 68 rows on `main` outside a spec (M36-M96 and M98-M104: README, doc comments, ADRs;
  revision 1 said 59, a miscount of M36-M96's 61), the spec's Purpose and scenario lines (M2, M8, M29-M31); held-
  write claims phrased without "bound" or "within" (M25, M63, M72); and the difference between a library's bound and
  the repository's own. Precision on `main`: 7 of 18 hits are about a library or tool, 4 about nats.go.

## 4. The spec collision with #93: measured

Headings this change's delta would touch, from #170's text and the contradicted rows (which ones is the design's):
`### Requirement: Close and Drain are bounded by the caller's context` (`transport-client/spec.md:52`; M6, M8), `###
Requirement: A nil Close means everything the client owns has finished` (`:83`; M11), `### Requirement: A consumer's
setup context does not parent its handlers` (`:207`; M19), `### Requirement: Subscribe returns after the server has
read the SUB` (`:292`; M25's limit text names #168). The Purpose (`:3-9`, M2) is not a requirement. #93's delta
(`openspec/changes/setup-04a-02-ingest-kernel/specs/transport-client/spec.md:3-5`) has only `## ADDED Requirements` /
`### Requirement: A retried update passes the revision it read`; it modifies no existing heading.

Experiment (OpenSpec 1.13.2 from this worktree's `node_modules/.bin/openspec`; inputs under
`/tmp/pr177/scratch-src/`, runner `run.sh` there; each run copies `main`'s `openspec/` to a fresh
`/tmp/pr177/scratch-*`, empties its archive, adds the named changes, validates each with `--strict`, archives them in
the order given with `-y`, and diffs the resulting `transport-client` spec against `main`'s; `OPENSPEC_TELEMETRY=0`):

- `chg93` — #93's delta file, byte for byte (sha256 `16bbbe59…`).
- `chg177` — MODIFIED `Close and Drain are bounded by the caller's context`, `main`'s text with `and return within
  it,` replaced by a marker (`EXPERIMENT-177 MODIFIED TEXT`; sha256 `819f917f…`).
- `chg177b` — the same heading, a second marker (sha256 `f29c76a3…`).
- `chg177c` — MODIFIED `A retried update passes the revision it read` (#93's added heading) with a marker (sha256
  `c1f0d163…`).

Results (outputs: `/tmp/pr177/scratch-src/out-{AB,BA,same,Cfirst,Cafter}.txt`):

1. Different headings, either order (`run.sh scratch-AB chg93 chg177`; `run.sh scratch-BA chg177 chg93`): both
   validate (`Change 'chg93' is valid`, exit 0), both archive (`+ 1 added`, then `~ 1 modified`; exit 0), and the two
   resulting specs are byte-identical (`diff` empty): the Close text carries the marker and the new requirement is
   appended at the end (`:384` in the result).
2. Two changes MODIFYING one heading (`run.sh scratch-same chg177 chg177b`): both archive with `~ 1 modified`, exit 0,
   no warning; the result holds only the second change's text. The first change's text is replaced silently.
3. MODIFYING a heading that does not exist yet (`chg177c` before #93): `openspec validate chg177c --strict` passes
   with an INFO line (`Archive would refuse this delta: transport-client MODIFIED failed for header "### Requirement:
   A retried update passes the revision it read" - not found`, exit 0), and so does `task spec:check`'s own command
   (`npx --no-install openspec validate --all --strict --no-interactive`, `Taskfile.yml:113`; rerun on a scratch tree
   holding only `chg177c`: `Totals: 13 passed, 0 failed`, exit 0); `openspec archive chg177c -y` then aborts:
   `transport-client MODIFIED failed for header … - not found` / `Aborted. No files were changed.`, exit 1. After #93
   archives, the same delta archives (`~ 1 modified`, exit 0).
4. The archive never changes the Purpose: line 8 still reads `\`Close\` stays within the caller's deadline` after
   every run. A Purpose change is a hand edit in the change's archive/spec sync commit.

Measured on OpenSpec 1.13.2 only; open PR #117 moves it to 1.14.0.

## 5. Surface inventory, the six categories

### Category 1 — the claimed gap

Issue #170 claims three absences; each was searched.

- No rule that library-behaviour claims cite a probe: confirmed (§3, searches listed there). The existing rule is
  port-only and pin-only (`semengine-architect.md:54-61`, `AGENTS.md:99`).
- No record of natsclient's contract over nats.go: `ls docs/adr` — `102-entity-id-segment-semantics.md`,
  `104-unique-platform-authority.md`, `108-processor-shell.md`, `110-write-seam.md`,
  `111-durable-execution-primitive.md`; `git grep -n -i natsclient -- docs/adr` hits only ADR-110 and ADR-111 (M94-M97),
  each about one call. The statements exist, scattered across nine homes (category 2); none is a decision record.
- "Nobody measured nats.go when those sentences were written": for PR #48's sentences, its design says code reads
  stood in (`openspec/changes/archive/2026-10-05-setup-04a-01-floor/design.md:470-471` — `orderings inside nats.go
  itself, which the nats.go` / `v1.54.0 cites in this section stand in for.`); its probes were pin probes (`:990` —
  `probe, PR #48 comment 5942307713`, a nil-context panic at the pin). For #156's, P1-P9 measured nats.go; the
  correction in §0 applies to P6's post-resume figures.

### Category 2 — every current spelling of "what natsclient promises over nats.go"

1. `openspec/specs/transport-client/spec.md` — 13 requirements (M1-M31).
2. `natsclient/doc.go` and `natsclient/README.md` — M36-M59.
3. Exported doc comments — M60-M89 and M100-M104 (revision 2); unexported comments citing nats.go source lines — 20
lines (M90-M92 and the
   mechanism notes listed there).
4. `docs/admission-ledger.yaml:1424-2242`, the natsclient row — 17 lines naming nats.go; restates M9, M16, M26-M28,
   M54, M90.
5. Archived designs: `…/2026-10-05-setup-04a-01-floor/design.md` D3 (`:286-296`, `:470-471`); `…/2026-10-10-
   subscribe-registers-interest/design.md` "Declared limits" (`:300-336`) and `inventory.md` "nats.go behaviour by
   connection state" (`:262-281`).
6. Issues: #168 (held writes, with candidate fixes), #169 (refusals, with the options measured), #83 (Closed() fires
   early; re-check per nats.go bump), #146 and #167 (classes).
7. `docs/adr/110-write-seam.md:76-80`, `docs/adr/111-durable-execution-primitive.md:71-72`, `:108-109`, `:133-136`.
8. Contract sections carried with natsclient: developer § NATS RPC (`semengine-developer.md:206-216`), § Storage and
   retention (`:218-237`); reviewer § NATS RPC error contract (`semengine-reviewer.md:235-247`), § Storage and
   retention review (`:248-261`); `docs/testing.md:474-481` (the nats.go timer pool).
9. The pin's `docs/operations/07-nats-request-retry.md`, the request-retry rule natsclient cites and does not carry:
   `natsclient/request.go:71` — `SemStreams' docs/operations/07-nats-request-retry.md at the pin; not carried`;
   `natsclient/errors.go:228`, `:261`, `:286` cite `docs/operations/07-nats-request-retry.md` as a path in this
   repository, where `docs/` has no `operations` directory (`ls docs`).
10. Consumer-side spellings of the transport's promises (revision 2, review r1 HIGH 1): `docs/adr/110-write-seam.md:192`
   (M98); `docs/adr/111-durable-execution-primitive.md:104-105`, `:116-117` (M99); #93's `graph-ingest-recovery`
   delta `:5-41` (G1-G3); and, at #176's head, the pin's ADR-066 with nats.go v1.48.0 statements (`:69`, `:183`,
   `:216-217`, `:265-266`, `:408-409`, `:484-485`).

Within these, two spellings of one fact: `natsclient.ErrCircuitOpen` and `errs.ErrCircuitOpen` (§2, area 3); and the
class of "no responders", "transient" in prose (M73) and not transient by `errs.IsTransient` (P10).

### Category 3 — adjacent claims on the territory

Open pull requests (`gh pr list --state open --limit 100 --json
number,title,isDraft,changedFiles,headRefName,files`; the 308 files of #93 read in full with
`gh api --paginate repos/C360Studio/semengine/pulls/93/files --jq '.[].filename'`):

| PR | State | Overlapping files | Overlap |
| --- | --- | --- | --- |
| #93 | draft, 308 files, head `f3d5ae56` | `natsclient/` (13 files), `pkg/errs/` (4), `openspec/changes/setup-04a-02-ingest-kernel/specs/transport-client/spec.md`, `…/specs/nats-fixture/spec.md`, `internal/harness/natsfixture/{open.go,open_test.go,fixture_integration_test.go}`, `go.mod` (`golang.org/x/net v0.60.0`) | §1b, §4; frozen per #167's text ("Not in PR #93, which is frozen") |
| #176 | draft, head `e5475855` (updated 2026-10-11T12:51Z), 7 files | `docs/adr/066`, `083`, `084`, `085`, `088` (new), `docs/admission-ledger.yaml` (+97), `docs/repository-map.md` (+2 -2) | the map's `docs/adr/` row (`docs/repository-map.md:29`), the line an ADR-112 entry edits: a textual conflict, so whichever merges second merges `main` first; ADR-066 brings nats.go v1.48.0 statements into `docs/adr` (`:69`, `:183`, `:216-217`, `:408-409`, `:484-485`), which the new rule reaches and one of which contradicts M98 (revision 2: the row read "0 files") |
| #118 | ready (Dependabot) | `go.mod`, `go.sum`: nats-server 2.14.7 → 2.15.0 | every probe here measures 2.14.7; the embedded server would move, `.nats-image` would not |
| #117 | ready (Dependabot) | `package.json`, `package-lock.json`: OpenSpec 1.13.2 → 1.14.0 | §4's results are for 1.13.2 |
| #116, #119, #120 | ready (Dependabot) | `go.mod` | textual only; no NATS module |

Open issues on the territory (`gh issue list --state open --limit 200` filtered for natsclient, nats, transport,
errs, Close, probe, library): #168, #169, #167, #146, #83, #75, #85, #74 (NATS TLS, token and credentials parsed but
never reaching the client; config/boot port), #15 (graph recovery across transport stream generations; blocked). Out
of scope by the brief: fixing #168, #169, #167, #75, #85.

Admission ledger: the natsclient row (`docs/admission-ledger.yaml:1424`, disposition `adapt`, owner
`setup-04a-01-floor developer`) carries adapt items (1)-(13); item (13) is subscribe-registers-interest (#144, PR #156).

Specs: `transport-client` (current truth), `nats-fixture` (M32-M35). Rulings: ruling N (#91 comment 6080973822); O3
and O4 (PR #156 comment 6098388350); the owner's ruling creating #170 (epic #9 comment 6098388541); O1 (PR #156 comment
6091034659, the synctest rule).

### Category 4 — the consumer at birth

This change, as #170 states it, adds an ADR file and a rule (an `AGENTS.md` row and contract text): no exported
symbol, subject, bucket or config field. The ADR's readers in the tree: on `main` no production package outside
`natsclient` imports it (`grep -rl '"github.com/c360studio/semengine/natsclient"' --include='*.go'`, non-test: only
`natsclient/doc.go`'s comment); at #93's head 16 non-test files outside `natsclient` in `component`, `graph/inference`,
`graph/kvcatalog`,
`graph/readiness`, `internal/graphmutation`, `pkg/lifecycle`, `pkg/projection` and `processor/graph-ingest`; outside,
SemSource imports `natsclient` (`docs/admission-ledger.yaml:1426-1430`). Change 3 (boot, service, config) is the next
importer #170 names.

### Category 5 — the problem shape

Shape: a measured contract record over a dependency, with the rule that keeps statements about the dependency
measured. Closest instances: the pin-probe rule (`semengine-architect.md:54-61`) for SemStreams; the synctest rule
(`docs/testing.md:474-481`, a nats.go fact measured and recorded with its numbers); `.nats-image:1-4` (one spelling of
the server version, with `TestOneImagePin` behind it, `AGENTS.md:88`); ADR-102 and ADR-104's port notes
(`semengine-architect.md:97-99`), the form a carried record takes. Not found: a record that pins a library version to
measured behaviour and is re-checked on a version bump (#83 tracks one fact that way, by hand).

### Category 6 — the pin's decision records that govern natsclient

This change carries no package from the pin (natsclient came with PR #48). Asked for here because the ADR records
natsclient's contract. All 85 records fetched at the pin into `/tmp/pr177/pin-adr/` (`gh api
'repos/C360Studio/semstreams/contents/docs/adr?ref=8b99efe9' --jq '.[].name'`, then each with `-H 'Accept:
application/vnd.github.raw'`); `grep -l -i natsclient` hits 21; titles read for the rest. Those whose decision
natsclient's code implements, or that decide something about natsclient:

| ADR (pin) | Decides, for natsclient | Lines naming it in `natsclient/` (tests included) | In `docs/adr` |
| --- | --- | --- | --- |
| 060 Unified RPC Error Contract (+ 060 PR-D summary) | one typed, classified error over the wire: `X-Status`, `X-Error-Class`, `RespondError`, `ClassifyReply` (ADR-060 `:14-17`, `:48-50`); amended by ADR-091 (`:5-6`) | 18 | no; not in #176's list |
| 055 Graph Write-Intent Taxonomy | "T1": `PublishToStreamWithMsgID` and the duplicate window (ADR-055 `:390-392`, `:489-494`) | 4 (`client.go:1557`, `:1566`) | no |
| 066 Honest graph-index readiness | `BucketLastSeq` as the readiness target (`:406`); reviewed "against `nats.go` v1.48.0" (ADR-066 `:52`, `:466`) | 1 (`kv.go:102`) | coming with #176 |
| 068 Graph retention, deletion and GC | D1: live graph buckets carry no binding TTL, MaxBytes or MaxAge, checked at boot (ADR-068 `:190-193`, `:417-418`); natsclient's KV facts it cites (`:93-98`, `:299`) | 12 (`kv.go:166-167` names D1) | no |
| 073 Graph ingestion retention contract | retention fail-closed at boot (`kv.go:158-166` at the pin, ADR-073 `:63`) | 3, spelled "ADR-068/073" (`backing_stream_prefix.go:90`, `:122`, `:128`) | no |
| 070 Gated-DAG durable dispatch | the durable substrate: `PublishToStreamWithAck`, `EnsureStream`, `ConsumeStreamWithConfig`, `PortConsumerContext` (ADR-070 `:46`, `:55`, `:185`, `:196`) | 0 | no |
| 072 Keyed-concurrent entity ingest | keyed mode stays out of natsclient (ADR-072 `:141-145`) | 0 | no |
| 077 Bounded owner discovery | relies on `the framework-enforced natsclient KV deadline` as a ceiling (ADR-077 `:157`); M41 shows a KV call held past it | 0 | no |
| 032 Policy DSL, tenancy, cluster | "`natsclient` package already handles reconnects; cluster-aware defaults" (ADR-032 `:349-351`); ADR "Accepted — In Progress" (`:5`), its track "JetStream cluster docs + reconnect defaults" Pending (`:18`) | 0 | no |
| 102 Entity-ID segment semantics | decision 7: the identity bucket takes no lifecycle retention (`kv.go:166-167`) | 4 | yes |

The natsclient ledger row cites no ADR (`grep -o -i 'ADR-[0-9]*'` over `docs/admission-ledger.yaml:1424-2242`: no
match). Whether ADR-060 (whose decision `natsclient/errors.go` implements) is ported, and how the others are cited,
belongs to the design (architect contract § Extraction slices, "Cite the pin's decision records"; owner decision
on #157).

ADR number: `docs/adr` holds 102, 104, 108, 110, 111; 109 is reserved for readiness (#110), named in
`docs/adr/108-processor-shell.md:130`, `:169`, `:187`, `docs/adr/110-write-seam.md:149`, `:192`, `:197`,
`docs/adr/111-durable-execution-primitive.md:129`, `:208`; #176 adds pin numbers below 102. `git grep -n 'ADR-112'`
finds nothing; `gh search issues --repo C360Studio/semengine 'ADR-112'` finds nothing; no open PR has a `docs/adr`
file. The next free number is **112**.

## 6. Same-class collision table (natsclient, the communication primitive)

The change proposes no new primitive; the table records who already states or owns natsclient's behaviour.

| Dimension | Evidence |
| --- | --- |
| Semantic class | what a natsclient call promises about time, nil, errors, ownership and reconnect over nats.go v1.54.0 and nats-server v2.14.7 |
| Owners | `natsclient` (code and doc comments); `transport-client` spec; the ledger row; ADR-110 and ADR-111 for two calls; pin ADR-060 for wire errors (category 6) |
| Catalogs | the nine homes of category 2; no generated registry (`git grep -n -i 'nats.go' -- '*.yaml' '*.json'` hits only the ledger) |
| Status | `Client.Status()` / `GetStatus()` (`client.go:380`, `:661`); `natsfixture` phase records |
| Lifecycle | `Connect` / `Close` (`client.go:692`, `:903`); `natsfixture` Start/Stop/Restart (`nats-fixture` spec) |
| Ownership | `transport-client` "A nil Close means …" (`:83-96`); PR #48 D3; #156 D4 |
| Readers | at #93, 16 production files outside `natsclient` (category 4); #93's `graph-ingest-recovery` delta (G1-G3); ADR-109 through ADR-110:192 (M98); SemSource; every natsclient test |
| Writers | the natsclient developer (doc comments), spec deltas (#93's), the ledger row's owner |
| Recovery | nats.go's automatic reconnect (P15); `WithConnectionLossTimeout` (`options.go:59-69`); no natsclient re-dial |

## 7. Adopter seam inventory

The surface is reached from outside: SemSource imports natsclient; at #93 eight packages do. Answered for a component
author who has never opened `natsclient/`.

1. What must they know today (each measured above)?
   (a) A context does not bound any natsclient call while the server is not reading and the send buffer is full; the
   hold ends when the server reads again or as each held write reaches nats.go's one-minute timeout, with no ceiling
   observed (P13, P14). (b) `Connect`'s dial is bounded by `WithTimeout` once per URL, not by the context, and an
   ended context still dials (P12). (c) A nil `Publish` means "buffered" (P14). (d) A KV write that returned an error
   may have committed (P15 for `Put`; p15b for `Create` and `Update`). (e) A nil `Subscribe` may be a refused SUB (P8).
   (f) `errs.IsTransient` is false for
   `ErrNotConnected`, `ErrCircuitOpen` and `ErrNoResponders`, which natsclient returns bare, and `errs.Classify` says
   transient for everything (P10); eight more sentinels change class when #93 lands. (g) While reconnecting, core
   calls refuse at once with `ErrNotConnected` while KV calls wait for their context and may commit later (P15). That
   is seven facts; the contract's threshold is two.
2. What happens if they do nothing? A `Stop` that closes its client with a 5 s budget can be held for minutes (P14:
   3 m 1.6 s and over 200 s for a 50 ms Close); a retry loop keyed on `errs.IsTransient` does not retry a lost
   connection (`ErrNotConnected`) and, after #93, does not retry `nats.ErrTimeout`; a KV write retried after an error
   may apply twice where the write is not a CAS (`Put`); a refused responder looks registered until a request gets "no
   responders".
3. Where do they find out? (a) doc comment for `Subscribe` and `SubscribeForRequests` only (M63), and nowhere for the
   other eleven calls; the README and `doc.go` say the opposite (M42, M59). (b) nowhere. (c) nowhere. (d) nowhere on
   `main`; a doc comment at #93 (N3). (e) doc comment (M63) and a log line. (f) nowhere; the doc says the opposite for
   no responders (M74). (g) nowhere. Each is at "doc" or below; four are "nowhere".
4. What should they have to know? The gap between 1 and 4 is the seven facts above; recorded as the gap, not answered
   (inventory only).

## 8. Intent check

Not required: the change sets or moves no boundary (no port set, tier, exclusion or deferral). It records a contract
and adds a review rule.

## 9. Open evidence questions (for the reviewer and the owner)

- Q1. The §0 correction: P6's and #168's readings of a return after 1.5 s as a return after the server read again
  (59.7 s, 1m4.9s, and 3.3-3.5 s) measured the probe's own `IsConnected` waiting for the lock. Who corrects the
  sentences §0 pins: #168's body (`:7`, `:9`, `:10`, `:25-29`, `:38-45`, `:75` of the saved copy) and the archived
  design (`…/2026-10-10-subscribe-registers-interest/design.md:97-99`, `:322-325`, `:404-406`)?
- Q2. Does an in-tree test that drives the pinned library count as "a probe" for the rule (28 "test only" rows), or
  must a probe be posted with source and output as P6-P9 were? P1-P5 were never posted (local only, §1).
- Q3. #170 names the architect and reviewer contracts as the rule's home; doc comments are the developer's and READMEs
  the technical writer's (§3). Whether those contracts carry it is the owner's.
- Q4. The rule's reach backwards: 16 contradicted and 30 unmeasured statements on `main` today (revision 2), and
  ADR-066's nats.go v1.48.0 statements when #176 lands. Whether the ADR change
  corrects them, deletes them, or leaves them to #168/#169 and later changes is the design's question, and some are
  in #93's files (frozen).
- Q5. Moved to §2, N-P3 (revision 2): a measurement, not an owner question.
- Q6. Dependabot #118 (nats-server 2.15.0) would change the version every probe measured; whether a version bump must
  re-run the ADR's probes is the rule's question. (Revision 1's first half, darwin against Linux, is N-P7.)
- Q7. ADR-060 governs `natsclient/errors.go` and is neither in `docs/adr` nor in #176's list; ADR-077 relies on the KV
  deadline that M41 contradicts.
