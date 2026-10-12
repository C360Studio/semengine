# Inventory: remove `natsclient.Client.SetConnection` (#184, PR #188)

Accepted: INVENTORY PASS on `/tmp/pr188/inventory.md` (sha256
`fc35bf7d010de33c218a44a843442efc4d0bbe82f6b7265ba919c7af05faae44`),
review `/tmp/pr188/inventory-review.r1.md`. This copy folds in that review's six corrections (M1, M2, N1-N4), each
marked
`[r1 Mx]` or `[r1 Nx]` where it lands; nothing else changed.

base: a5020db (main) + 02f679a (empty claim commit), worktree
`/Users/coby/Code/c360/semengine-wt/claude/remove-setconnection`.
Every file this inventory pins is unchanged since 5f0e453 (`git diff --stat 5f0e453 HEAD -- natsclient
openspec/specs/transport-client docs/adr/112-natsclient-contract.md docs/admission-ledger.yaml` is empty), so the
issue's line numbers hold here.

Phase: inventory only (architect contract, Required workflow step 2). No target state, options, recommendation or
tasks.

Question: what exists on this tree that removing `SetConnection` touches, and what each piece is there for.

Read: issue #184 (body, comment 6124681941), PR #188 body, the architect contract, developer contract `:110-114`,
reviewer contract `:313-321` and `:399-410`, `docs/inventory-scope.md` rule 4. Repositories: this tree, plus
SemStreams at the pin `8b99efe9` through `gh api` only (one `contents` call, and the pin tarball extracted to
`/tmp/pr188/pinsnap`, grepped, nothing run). The consumer check is not repeated (PR #188 body).

Legend for the "swap-only" column: **S** = exists only because a connection can be swapped under a running client;
**B** = also guards something else (named); **D** = dead on this tree with or without the swap.

## 0. Base facts every row below rests on

- Writers of `Client.conn` (gopls references on `natsclient/client.go:77`): `:434` (`SetConnection`), `:834`
  (Connect's admission), `:1048` (Close's cleanup). Admission refuses once Close has begun
  (`natsclient/client.go:822` — `case m.closing:`) and while a connection is installed
  (`natsclient/client.go:826` — `case m.conn != nil:`). Cleanup runs only after `closing` is set.
- `natsclient/client.go:834`-`:837` write `m.conn = conn`, `m.js = js`, `m.dialled = conn`, `m.socket = socket` in one
  critical section; `:1048`-`:1050` clear `m.conn` and `m.js` together. `dialled` is written nowhere else (gopls
  references on `:178`: write `:836`; reads `:970`, `:1044`, `:1571`).
- So, without `SetConnection`: while Close has not begun, `m.conn` is nil or the one connection Connect admitted, and
  that is `m.dialled`; a client admits at most one connection in its life; `m.draining`
  (`natsclient/client.go:973` — `m.draining = conn`, set from `m.conn` at the first Close) is nil or `m.dialled`.
- `SetConnection` writes only `m.conn` (`natsclient/client.go:434` — `m.conn = conn`). It does not write `m.js`,
  `m.dialled` or `m.socket`; after a swap the JetStream handle and `jetStreamWithConn`'s attribution still name the
  dialled connection.
- One other way the tree makes `conn != dialled`: `natsclient/kv_key_contract_integration_test.go:73` —
  `wrapperClient.conn = recordedConn` (carried from the pin, pin `:82`). That test never calls Close or a consume
  method on `wrapperClient` (grep of the file for `wrapperClient.Close` and `Close(ctx`: none), so it reaches none of
  the branches in section 2.

## 1. Category 1: the issue's claims, measured

| Claim (#184) | Measurement | Result |
| --- | --- | --- |
| `SetConnection` at `client.go:421-438` | `natsclient/client.go:421` — `// SetConnection sets the NATS connection (for testing). The connection it` | holds |
| Only natsclient's own tests call it | `gopls references natsclient/client.go:424:18` (default tags): `client_close_final_test.go:1136`, `:1423`, `:1476`, `:1577`. gopls does not load the `integration` tag; `git grep -n SetConnection -- '*.go'` adds `request_response_bounds_integration_test.go:167`, `:189` | holds; no production caller |
| 39 Go lines, 48 Markdown lines | `git grep -c SetConnection`: Go 11+24+2+2 = 39; Markdown 7 (spec) + 1 (ADR-112) + 40 (archives) = 48; plus 4 lines in `docs/admission-ledger.yaml` | holds |
| Cited special cases `:183, :197, :935, :1043, :1454, :1496-1497, :1560`; `owned_delivery.go:120, :125` | each is a comment line naming `SetConnection`; the code each comment describes is in section 2 | holds; code sites listed below |
| #93 touches none of the four Go files | PR #93 file list (334 files, `gh api --paginate .../pulls/93/files`) | holds; overlaps elsewhere, section 7 |

## 2. Go production code that names or exists because of a swap

### 2a. The method

- `natsclient/client.go:421`-`:438`, `natsclient/client.go:424` — `func (m *Client) SetConnection(conn *nats.Conn) {`.
  Doc `:421`-`:423`; IsConnected before the lock, citing T2 and P21 (`natsclient/client.go:427` —
  `connected := conn != nil && conn.IsConnected()`); refusal once Close began with a warn log
  (`natsclient/client.go:430` — `if m.closing {`, `natsclient/client.go:431` —
  `m.logger.Warn("NATS client closing; SetConnection ignored")`); status Connected when given a connected
  connection (`natsclient/client.go:436` — `m.setStatus(StatusConnected)`). **S.**

### 2b. Code sites whose only job is the swap

| Site | What it does | Swap-only | Evidence |
| --- | --- | --- | --- |
| `natsclient/client.go:970` — `if conn != nil && conn == m.dialled {` | first Close picks the socket to cut | **S** for the `conn == m.dialled` operand; `conn != nil` covers a Close before Connect | section 0: `conn` is nil or `m.dialled` |
| `natsclient/client.go:1044` — `if conn != m.dialled {` (`:1044`-`:1046`, comment `:1042`-`:1043`) | cleanup skips the closed-handler join for an installed connection the client did not dial | **S** | section 0; unreached in the unit run once the swap tests are skipped (section 9) |
| `natsclient/client.go:1512` — `replaced := conn != m.draining` and `natsclient/client.go:1515` — `_ = sub.Unsubscribe()` (`:1509`-`:1516`) | subscription watcher unsubscribes a subscription left on a replaced connection | **S** for the `case <-closing:` arm, the `replaced` read and the Unsubscribe. The goroutine itself is **B**: it holds the subscription's owned-work count until delivery ends (`natsclient/client.go:1505` — `defer finish()`, `:1517` — `<-d.done`), which Close joins | subscription's `conn` is captured from `m.conn` under `mu` at `:1413`; section 0 gives `conn == m.draining`; block `:1514`-`:1516` unreached without swap tests |
| `natsclient/owned_delivery.go:156` — `replaced := conn != c.draining` and `natsclient/owned_delivery.go:159` — `h.Stop()` (`:155`-`:160`) | consumer ownership goroutine stops a consumer whose handle was made on a replaced connection | **S** for the `replaced` read and Stop. The enclosing `case <-closing:` (`natsclient/owned_delivery.go:153`) also calls `cancelHandlers()` (`:154`): **B**, owner ruling #9 comment 5994720412 item 2 (`owned_delivery.go:127`-`:129`) | block `:158`-`:160` unreached without swap tests |
| `natsclient/owned_delivery.go:131` — `conn *nats.Conn, d *ownedDelivery, cancelHandlers context.CancelFunc, release func(started bool),` | `ownConsumer`'s `conn` parameter | **S** | its one reader is `:156` |
| `natsclient/client.go:1124` — `conn *nats.Conn` (`guardedConsumer.conn`, doc `:1122`-`:1123`) | carries the connection a consumer's JetStream handle was made on | **S** | gopls references: written `natsclient/stream.go:547` — `guarded := &guardedConsumer{Consumer: consumer, conn: jsConn}` and `natsclient/stream.go:825` — `&guardedConsumer{Consumer: consumer, conn: jsConn}, identity, claim, handler,`; read only to call `ownConsumer` (`natsclient/stream.go:577` — `owned, ok := c.ownConsumer(guarded.conn, d, cancelHandlers, func(bool) {`, `natsclient/stream.go:689` — `owned, ok := c.ownConsumer(consumer.conn, d, cancelHandlers, func(started bool) {`) |
| `natsclient/client.go:1571` — `return m.js, m.dialled, nil` (`jetStreamWithConn`, `:1558`-`:1572`) | returns the handle and the connection it was made on, read together | **S** for the second result. Doc `natsclient/client.go:1560` — `// attributed to the connection it runs on even if SetConnection and a new Connect run meanwhile.` | callers `natsclient/stream.go:512`, `:795` (into `guardedConsumer.conn`); `natsclient/client.go:1554` — `js, _, err := m.jetStreamWithConn()` discards it. A second Connect admits only after `SetConnection(nil)` empties `m.conn` (`:826`) |
| `natsclient/client.go:178` — `dialled        *nats.Conn` (doc `:170`-`:175`) | names the connection Connect installed, apart from `conn` | **S** | its three reads are the three rows above (`:970`, `:1044`, `:1571`); each differs from `m.conn` only after a swap (section 0) |

### 2c. Code next to the swap that also guards something else

| Site | Other job | Evidence |
| --- | --- | --- |
| `natsclient/client.go:185` — `closingSignal chan struct{}` | **B.** Its doc names only the swap (`natsclient/client.go:183` — `// subscription's watcher can end one left on a connection SetConnection`), but it also decides `Subscribe`'s refusal class once Close has begun (`natsclient/client.go:1427` — `case <-closing:`, `natsclient/client.go:1444` — `case <-closing:`) and drives `cancelHandlers` in `ownConsumer` (`natsclient/owned_delivery.go:138` — `closing := c.closingSignalLocked()`, `:145`, `:153`, `:170`) | gopls references on `:185`: `:1522`-`:1525` (`closingSignalLocked`); callers `:974`, `:1420`, `owned_delivery.go:138` |
| `natsclient/client.go:176` — `draining       *nats.Conn` | **B.** Besides the two `replaced` reads, `handleError` uses it to count a drain timeout only for the connection Close drains (`natsclient/client.go:2330` — `if nc == m.draining {`); without a swap this still excludes a drain someone else started before Close, for example through the exported `GetConnection` (`natsclient/client.go:402`) | gopls references on `:176`: `:973`, `:1512`, `:2330`, `owned_delivery.go:156` |
| `natsclient/client.go:201` — `closedSignals   map[*nats.Conn]chan struct{}` | **B.** Keyed per connection because a losing Connect's candidate is awaited too (`natsclient/client.go:893` — `await := carriesClientHandlers(conn)` in `discardCandidate`, `:889`), not only the dialled connection (`join`, `:1070`). Only the comment clause is swap-only: `natsclient/client.go:197` — `// ever waits: a connection replaced through SetConnection (a test hook), or`; its sentence runs `:195`-`:199`, which is the range to edit `[r1 N2]` | `grep closedSignals client.go` |
| `natsclient/client.go:343` — `func carriesClientHandlers(conn *nats.Conn) bool {` | **B**, but its other job is a test seam `[r1 M1]`. In production `connectWith` has one caller, `natsclient/client.go:720` — `return m.connectWith(ctx, nats.Connect)`, which dials with `buildConnectionOptions`, and that always sets `natsclient/client.go:665` — `nats.ClosedHandler(m.handleClosed),`. So every non-nil connection Connect installs carries the handlers. The only handler-less connections are a swapped one, or a candidate from a test `dial` that ignores the options (`natsclient/client_connect_test.go:34`, `:68`, `:96`; `newConnectCandidate` `:111`-`:128`). It must stay with the swap gone: without it Close would wait forever for the candidate `TestConnectCommitsCandidateWhenAdmissionRemainsOpen` admits (`natsclient/client_connect_test.go:90`). Used by `discardCandidate` `:893` and cleanup `:1040` | gopls references on `:343` and on `connectWith` (`:726`): one production caller, twelve test callers |
| `natsclient/client.go:2185` — `return !m.closing && nc == m.conn` and `natsclient/client.go:2191` — `if nc != m.conn {` | **B.** Ignore events of a rejected or not-yet-admitted Connect candidate (doc `:2179`-`:2183`) | read |
| `natsclient/client.go:179` — `socket         *socketDialer` | **B.** The dialer Close cuts (T1, T8); only the operand that selects it (`:970`) is swap-only | gopls: written `:837`, read `:971` |
| `natsclient/client.go:1047` — `if m.conn == conn {` | **D.** True on every path on this tree: every writer of `m.conn` refuses once `closing` is set (`:430`, `:822`), and only the first Close reaches cleanup. Historically it guarded a `SetConnection` during Close, before owner ruling 3 made `SetConnection` refuse then (archived `2026-10-05-setup-04a-01-floor/design.md:340`) | section 0 |
| `natsclient/client.go:1423` (IsConnected outside the lock in `subscribeOwned`) and `natsclient/client.go:415` — `if conn == nil \|\| !conn.IsConnected() {` (`MaxPayload`) | **B.** The same T2/P21 ordering as `SetConnection`'s `:425`-`:427`; still cited by `client_close_held_test.go:56` and ADR-112 `:158` | `git grep -n P21 -- natsclient/*.go` |

### 2d. Comment-only lines (no code of their own)

`natsclient/client.go:183`-`:184`, `:197`, `:935` — `// installed through SetConnection is not cut. Once Close has begun
the client`,
`:1043`, `:1453`-`:1454` — `// SetConnection installed since.` (the round trip uses the local `conn`, `:1455`, not
swap-specific code), `:1496` — `// a connection SetConnection replaced is unsubscribed here once Close begins:`,
`:1497`,
`:1560`; `natsclient/owned_delivery.go:119`-`:121` (`:120` — `// whose connection SetConnection has since replaced is
the client's but its connection is the`),
`:124`-`:125` (`:125` — `// handle when the setup began (jetStreamWithConn), whatever SetConnection has installed
since.`).

Qualifiers that draw the line `SetConnection` makes ("a connection the client dialled" against one installed through
`SetConnection`; spec `:63` states the contrast): `natsclient/client.go:931` —
`// the same completion. On a connection the client dialled, an ended ctx also`; `natsclient/doc.go:9`-`:10`
(`:10` — `// connection the client dialled, and Connect are the exception (T1, T4; ADR-112). It`). Not swap-related:
`natsclient/client.go:714`-`:716` (Connect's doc, "the connection it dialled" is the losing candidate).

## 3. Category 2: every current spelling of "which connection is the client's"

| Spelling | Where | Differs from `m.conn` without a swap? |
| --- | --- | --- |
| `Client.conn` (installed connection) | `natsclient/client.go:77` | reference |
| `Client.dialled` (the one Connect installed) | `:178` | no, while `m.conn != nil` (section 0) |
| `Client.draining` (the one the first Close drains) | `:176` | no: set from `m.conn` at `:973` |
| `Client.js` (JetStream made on the dialled connection) | `:78` | made from the same connection at `:835`; `SetConnection` does not replace it |
| captured `conn` in `subscribeOwned` | `:1413`, passed to the watcher `:1451` and the request callback (`natsclient/request.go:412` — `return c.requestCallback(ctx, conn, subject, handler)`) | no |
| `guardedConsumer.conn` / `jetStreamWithConn` second result | `:1124`, `:1571` | no |
| `ownsStatusLocked`, `handleDisconnect` (`nc == m.conn`) | `:2185`, `:2191` | compares event connections to `m.conn`; guards candidates |
| `Client.awaitingClosed` (the connection whose closed handler the joiner awaits) `[r1 N4]` | `natsclient/client.go:180` — `awaitingClosed *nats.Conn` | no; it is never set for a swapped connection, because cleanup skips the join for one |
| test-side: `tableClient.conn` | `natsclient/client_close_final_test.go:955`, written `:1084` — `tc.conn = c.GetConnection()` | never read (grep `tc.conn`: one write) |

## 4. Tests

| Test | What it proves | Uses the swap for | Goes with the method, or proves something else |
| --- | --- | --- | --- |
| `natsclient/client_close_final_test.go:960` — `func TestClientLifecycleOperationTable(t *testing.T) {` | return and status of 8 operations in 4 lifecycle states (32 cells) | one column of 4 cells (`want` at `:975`, `:981`, `:987`, `:994`) | The column proves only swap behaviour (install before Close; ignore and warn after; mutation M13, archived floor `tasks.md:645`-`:646`). The other 28 cells do not use a swap: with the column skipped (`go test -run '^TestClientLifecycleOperationTable$' -skip '^TestClientLifecycleOperationTable$/^[a-z]+/SetConnection$' ./natsclient/`) 28 subtests pass. |
| `natsclient/client_close_final_test.go:1399` — `func TestClientCloseEndsSubscriptionOnReplacedConnection(t *testing.T) {` (two subtests) | Close unsubscribes and joins a subscription left on a replaced connection, leaves that connection open, and tolerates its Unsubscribe error | setup `natsclient/client_close_final_test.go:1423` — `c.SetConnection(second)` | swap-only. Close joining a running subscription handler without a swap: `:227` `TestClientCloseJoinsSubscribeHandlerAfterForcedClose` |
| `natsclient/client_close_final_test.go:1442` — `func TestClientCloseEndsConsumerOnReplacedConnection(t *testing.T) {` (internal, port) | Close stops an idle consumer on a replaced connection and leaves that connection open | setup `natsclient/client_close_final_test.go:1476` — `c.SetConnection(replacement)` | swap-only. Consumer join without a swap: `:654`, `:303` |
| `natsclient/client_close_final_test.go:1529` — `func TestClientCloseAttributesConsumerToItsHandlesConnection(t *testing.T) {` (internal, port) | a consumer is attributed to the connection its handle was made on when a second connection is installed during its setup | `natsclient/client_close_final_test.go:1577` — `c.SetConnection(nil)`, then `:1578` a second Connect | swap-only: a second Connect admits only after `SetConnection(nil)` (`client.go:826`) |
| `natsclient/request_response_bounds_integration_test.go:144` — `func TestIntegration_SubscribeForRequestsUsesSubscriptionConnectionForOversizeDiagnostic(t *testing.T) {` (tag `integration`) | an oversize response's classified refusal reads `max_payload` from the subscription's own connection (`natsclient/request.go:471` — `"max_payload": conn.MaxPayload(),`), not from `Client.MaxPayload`, which refuses when the client's connection is not connected (`client.go:415`) | `natsclient/request_response_bounds_integration_test.go:189` — `client.SetConnection(nil)`, "Model the reconnect window" (`:186`-`:188`); restore in cleanup at `:167` — `t.Cleanup(func() { client.SetConnection(subscriptionConn) })` so the fixture client's Close drains it | **Proves something else; the setup is a swap.** Without a swap the subscription's connection and the client's are one `*nats.Conn` (section 3), so the two readings differ only in `MaxPayload`'s connected gate. Carried from the pin (pin `:165`, `:187`). The oversize refusal itself is also asserted by `:76` `TestIntegration_SubscribeForRequestsPublishesBeforeClassifyingResponseLimit` (`:136`, `:138`). |

Test-only scaffolding that exists for the `SetConnection` column of the table test:
`natsclient/client_close_final_test.go:997`-`:999` (`:999` — `installs := map[string]bool{"new": true, "connected":
true, "closing": false, "closed": false}`),
`:1005` — `"ConsumeStreamWithConfig", "ConsumeInternalStreamWithConfig", "SetConnection"}`, `:1015`-`:1019` (dial of
`other`), `:1033`-`:1042` (assertions), `natsclient/client_close_final_test.go:1098` —
`func runTableOp(t *testing.T, tc *tableClient, op string, id int, other *nats.Conn) error {` and its case
`:1135`-`:1137`,
`natsclient/client_close_final_test.go:951` — `logs       *recordingHandler` (one reader, `:1038`, the warn check) with
its
handler wiring `:1054`-`:1055`, and `natsclient/client_close_final_test.go:1063` —
`// The test owns the connection the client dialled once SetConnection may replace it.` above `:1065`
`t.Cleanup(dialled.Close)` (the local `dialled` at `:1064` is also used by the closing state). Dead test fields, not
swap-specific: `tableClient.conn` (`:955`, written `:1084`, never read), `tableClient.url` (`:952`, set `:1057`, never
read).

Tests that build a client state by writing fields, with no swap reached: `fakeConsumerClient`
(`natsclient/client_close_final_test.go:640` — `func fakeConsumerClient(t *testing.T, native jetstream.Consumer, opts
...ClientOption) *Client {`),
`newConnectedClientWithFakeJS` (`natsclient/getstream_circuit_breaker_test.go:253`), and `c.js =` writes at
`client_close_final_test.go:324`, `:644`, `:1547`, `client_lifecycle_test.go:659`, `consume_context_test.go:59`,
`:205`. Their `conn`, `dialled` and `draining` are all nil (or all equal on a connected client), so `replaced` is
false on their paths now and without the swap code.

What this tree offers a test that needs a client on a server: unit — `natsclient/client_lifecycle_test.go:108` —
`func embeddedServerURL(t *testing.T) string {` and `natsclient/client_close_final_test.go:183` —
`func embeddedJetStreamURL(t *testing.T) string {`, then `NewClient` and `Connect`; integration —
`natsclient/test_helpers_integration_test.go:48` —
`func newFixtureClient(t *testing.T, opts ...natsfixture.Option) *fixtureClient {` over `natsfixture.New`, `Start`,
`internal/harness/natsfixture/fixture.go:418` — `func (f *Fixture) URL() string {`, `:83` `WithMaxPayload`, and
`internal/harness/natsfixture/restart.go:40` — `func (f *Fixture) Restart(ctx context.Context) error {` (a real
disconnect; used at `natsclient/integration_test.go:75` — `require.NoError(t, f.Restart(restartCtx))`). PR #93 at
`6f1d6b1` adds `natsfixture.Open` (`internal/harness/natsfixture/open.go:29`), not on this tree. **None of these hands
an existing `*nats.Conn` to a `Client`; each builds a client that dials.** The three swap-only Close tests need a
client whose subscription or consumer runs on a connection other than the one Close drains, a state Connect alone
does not produce (section 0).

## 5. Spec, ADR, README, doc, ledger

### `openspec/specs/transport-client/spec.md` (481 lines)

Lines naming `SetConnection` (7):

- `openspec/specs/transport-client/spec.md:63` — ``A connection installed through `SetConnection` was not dialled by the
  client, and a write held on it holds `Close` as`` (sentence ends `:64`), in Requirement "Close and Drain are bounded
  by the caller's context" (`:52`).
- `openspec/specs/transport-client/spec.md:109` — ``error and never nil. A client-owned subscription left on a
  connection that `SetConnection` replaced SHALL be``; `:110` — ``unsubscribed and its running handler joined; the
  replaced connection itself belongs to the `SetConnection` caller.``; `:112` — ``` `SetConnection` SHALL be stopped
  when Close begins and its handlers joined, and its claim SHALL be released only after``` (sentence `:111`-`:113`), in
  Requirement "A nil Close means everything the client owns has finished" (`:100`).
- Scenarios in the same requirement: "A subscription on a replaced connection" (`:136`-`:141`; `:138` — `- **WHEN** a
  client subscription's connection has been replaced through SetConnection and its handler is running`) and "A consumer
  on a replaced connection" (`:143`-`:148`; `:146` — `SetConnection, and Close is called with a live context`).
- `openspec/specs/transport-client/spec.md:155` — ``returned. `SetConnection` SHALL change nothing once Close has
  begun.``, in Requirement "Close refuses new work" (`:150`).

Qualifier lines (no literal; they draw the dialled / installed-through-`SetConnection` line): `:8` (Purpose),
`:61` — ``` `Close` SHALL close the socket of a connection this client dialled and end any dial nats.go has in progress
to```,
`:74` (scenario GIVEN), `:104` — `the event handlers of the connection it closes when this client dialled it, and its
async publish error handler as`,
`:433`-`:434` in Requirement "A write the server is not reading holds the connection" (`:427`).

### `docs/adr/112-natsclient-contract.md` (accepted 2026-10-11)

- `docs/adr/112-natsclient-contract.md:153` — ``includes one on a connection that `SetConnection` replaced. The replaced
  connection stays the caller's, and `Close` `` (bullet `:152`-`:154`, citing tests `:1399`, `:1442`).
- `docs/adr/112-natsclient-contract.md:127` — ``connection it dialled (tests `client_close_final_test.go:227`, `:835`,
  `:1328`, `:1399`, `:1442`). The async`` — two of its five backing tests are the swap tests.
- Qualifier `docs/adr/112-natsclient-contract.md:79` — ``- **`Close` returns within its context** on a connection the
  client dialled. That holds even when the server has``.
- Precedent for editing this accepted ADR in place: PR #178 (`5f0e453`) rewrote `:216` and kept the old reading ("it
  was for ADR-109 (#110) when this record was drafted"). Precedent for an amendment in a Status line:
  `docs/adr/102-entity-id-segment-semantics.md:22` — `**Accepted (2026-08-26); decision 4 amended 2026-08-28.**`.

### `natsclient/README.md`, `natsclient/doc.go`

- No `SetConnection` literal in the README (`grep -n -i 'replaced|dialled|not cut|installed|test hook|for
  testing|swap|GetConnection'`: two hits, one unrelated). Qualifier: `natsclient/README.md:16` — ``returns within its
  context, and so does `Close` on a connection the client dialled (`TestConnectEndsWithItsContext`,``.
- `natsclient/doc.go:9`-`:10`, section 2d.

### `docs/admission-ledger.yaml`

- Row `docs/admission-ledger.yaml:1521` — `- source_path: natsclient`, disposition `docs/admission-ledger.yaml:2361` —
  `disposition: adapt`.
- Its `contract` defines the surface: `docs/admission-ledger.yaml:1530` — `Client, its options and every exported
  operation as at the pin, less the surface named under` (continues: "known_risks as dropped"). The dropped-surface list
  and its form ("each an adapt item, pin → deleted: … client.go ErrConnectionTimeout :55, …") is `known_risks`
  `:1759`-`:1772`.
- The 4 `SetConnection` lines: `:1642` — `Close unsubscribes a client-owned subscription on a connection replaced
  through SetConnection` (item (9) of the contract, `:1642`-`:1652`, naming
  `TestClientCloseEndsConsumerOnReplacedConnection` and `TestClientCloseAttributesConsumerToItsHandlesConnection`
  `:1647`-`:1648`, `TestClientLifecycleOperationTable` and `TestClientCloseEndsSubscriptionOnReplacedConnection`
  `:1651`-`:1652`); `:1649`, `:1650`; `:1706` — `TestCloseEndsAReconnectDial). subscribeOwned, MaxPayload and
  SetConnection call nats.go's`. Qualifier `:1704`-`:1705`.
- `proving_tests` maps pin to SemEngine lines inside the integration test file: `docs/admission-ledger.yaml:2208` —
  `request_response_bounds :122 → :124, :169 → :171, :176 → :178; request :43 → :45, :128 →`.
- `task ledger:check` compares `carry` rows only (`internal/harness/pindiff/main.go:86` — `// the pin"). Other
  dispositions are not compared. All its output goes to standard error.`, `:90` — `if e.Disposition == "carry" {`);
  natsclient is `adapt`, so removing a pin method changes nothing it reports beyond the row schema
  (`TestAdmissionLedger*`). `task ledger:diff -- natsclient` prints per-file diffs and the counts "files, differ, only
  at the pin, only in the tree" (`main.go:216`-`:223`); of the touched files, `client.go` and
  `request_response_bounds_integration_test.go` exist at the pin, `owned_delivery.go` and `client_close_final_test.go`
  do not (pin snapshot). Removing code changes the printed diff, not the counts, unless a file is added or removed.
  (Read from code; `ledger:diff` not run.)

### Archived changes (history: listed, not edited)

40 lines in three archives (`git grep -n SetConnection -- openspec/changes/archive/`):
`2026-10-05-setup-04a-01-floor` — `design.md:319`, `:321`-`:324`, `:340`, `:460`; `proposal.md:72`;
`specs/transport-client/spec.md:84`, `:85`, `:87`, `:113`, `:121`, `:130`; `tasks.md:645`, `:646`, `:653`, `:663`,
`:667`.
`2026-10-10-subscribe-registers-interest` — `design.md:64`, `:206`, `:334`, `:422`, `:439`.
`2026-10-11-natsclient-contract` — `cleanup.md:25`; `design.md:96`, `:125`; `inventory.md:152`, `:245`, `:748`;
`notes/3.2.md:11`, `:16`; `notes/3.5.md:6`, `:11`, `:32`, `:65`; `proposal.md:31`; `specs/transport-client/spec.md:16`;
`tasks.md:46`, `:102`. The owner ruling that made `SetConnection` refuse after Close began: ruling 3, #9 comment
5980296112 (archived floor `design.md:340`).

## 6. Category 6 and the pin (`8b99efe9`)

- This change carries no package from the pin; it edits `natsclient`, an `adapt` package already ported.
- `[r1 N1]` Every special case in section 2b was added by SemEngine; none came from the pin. That covers `dialled`,
  `draining`, `closingSignal`, the watcher's `replaced` branch, `owned_delivery.go` (absent at the pin),
  `jetStreamWithConn`, `guardedConsumer.conn` (the pin's struct, pin `client.go:646`-`:649`, has no `conn`) and
  `carriesClientHandlers`. The pin carries only the setter. So Bounded port scope (the rule for inherited structure)
  does not cover the special cases. The ledger's changed-behaviour item (9) (`docs/admission-ledger.yaml:1642`-`:1652`)
  is what goes with them, and `SetConnection` joins the "pin → deleted" list (`:1759`-`:1772`).
- The pin has it: pin `natsclient/client.go:223` — `// SetConnection sets the NATS connection (for testing)`,
  `:224`-`:231`: writes `m.conn`, sets Connected if `conn.IsConnected()` under the lock; no Close check, no
  lock-ordering.
- Pin callers (`grep -rn SetConnection --include='*.go'` on the pin snapshot):
  `natsclient/request_response_bounds_integration_test.go:165`, `:187`;
  `processor/agentic-tools/startup_atomic_integration_test.go:29`, `:30`, `:51` (makes Start fail with ErrNotConnected,
  then restores); `internal/boot/root_resources_test.go:31` (puts a client on `nats.Connect("",
  nats.InProcessServer(server))`,
  `:25`). No production caller. Neither package has a ledger row (`grep -n -E 'source_path:
  (internal/boot|processor/agentic-tools)'`: none)
  or appears in Slice 04A's D2 package lists (archived `2026-10-01-setup-04a-foundation/design.md:155`-`:178`), and
  `git grep internal/boot -- openspec/specs docs/setup-plan.md docs/repository-map.md` is empty.
- Pin ADRs: none records `SetConnection` or a connection setter. `grep -n -i -E 'SetConnection|set the
  connection|replace(d)? (the )?connection|swap(ped)? (the )?connection|connection setter|for testing\)' docs/adr/*.md`
  over the pin's 85 records: none. `grep -n -E 'GetConnection|Client\.Close|NewTestClient|\*nats\.Conn|native
  connection' docs/adr/*.md`: none.
  20 records name `natsclient` at all (030, 032, 039, 046, 047, 048, 049, 055, 056, 058, 060, 060-pr-d, 065, 066, 068,
  070, 072, 073, 077, 081); none of those hits is about the client's connection. ADR-112 `Related` (`:223`)
  already lists the pin ADRs it rests on (ADR-060, ADR-068).
- Not ADRs, but the pin's own archived inventories flagged it, and the pin did not act on it:
  `openspec/changes/archive/2026-08-21-require-restart-for-config-activation/native-surface-inventory.md:39`
  (`**RETIRE**`),
  `.../inventory.md:78` (no production consumer) and `:228` (rewrite the integration test "avoid exported
  `SetConnection`; use a narrow package-private fixture seam without teaching runtime mutation"),
  `2026-08-21-simplify-one-shot-lifecycle-ownership/next-tag-closeout-inventory.md:150`-`:151`, `:501`-`:502`.

## 7. Category 3: adjacent claims

- Open PRs (`gh pr list --state open`): #188 (this; draft; 0 files) and #93 (draft, head `6f1d6b1`, 334 files; full list
  by `gh api --paginate`). #93's overlaps with what this removal touches:
  - `docs/admission-ledger.yaml`, the same `natsclient` row: #93's hunks are `@@ -1716,6 +1734,32 @@` (after the
    contract's tail, `:1716`-`:1718`) and `@@ -2356,7 +2400,23 @@` (`proving_tests` tail), plus `@@ -493`. This
    removal's lines are `:1642`-`:1652`, `:1704`-`:1706`, and possibly `known_risks` `:1759`-`:1772` and `:2208`.
    Different hunks, one row.
  - `natsclient/README.md`: #93 adds 24 lines at `@@ -272` and `@@ -294`; this removal's line is `:16`.
  - `transport-client` capability: #93's delta
    `openspec/changes/setup-04a-02-ingest-kernel/specs/transport-client/spec.md`
    (38 lines) has only `## ADDED Requirements` ("A retried update passes the revision it read", "An unconfirmed write
    ends a retried update"); `grep -n -i 'SetConnection|replaced|dialled'` on it: none. This removal would touch
    existing requirements (`:52`, `:100`, `:150`, `:427`, Purpose). Which merges first is for the design.
  - #93 adds `internal/harness/natsfixture/open.go` (`Open`, section 4).
  - #93 touches none of `natsclient/client.go`, `owned_delivery.go`, `stream.go`, `doc.go`,
    `client_close_final_test.go`,
    `request_response_bounds_integration_test.go`, `docs/adr/112-natsclient-contract.md` (file list).
- #169 (open, bug): body says "a `SetConnection` connection logs only through its caller's handler". Owner ruling
  #169 comment 6124053574 (2026-10-12): add a refusal counter from the async error handler, and "The `SetConnection`
  case … goes with `SetConnection`: #184". The handler is `handleError` (`natsclient/client.go:2325`), beside the
  drain-timeout check `:2326`-`:2334` (section 2c, `draining`). #184 comment 6124681941: land one after the other.
- #168 closed 2026-10-12; #184 says its last open note (Close does not cut a `SetConnection` connection) exists only
  because of the method.
- Rules binding this change: `.agents/contracts/semengine-reviewer.md:313` — ``- Production code whose only caller is a
  test, a `_test.go` file or `internal/harness` (a method, field, option,``
  (`:313`-`:321`: exported and carried from the pin stays `BLOCKING`); `:399` — `**Severity needs a real-world trigger**
  (owner ruling of 2026-10-12, issue #185). Some failures happen only under a`;
  `.agents/contracts/semengine-developer.md:112` — `carries no method, field, option, constructor or hook, exported or
  not, whose only caller is a test. A test that`.
- No active OpenSpec change on this tree (`ls openspec/changes` without `archive`: empty). #184 has no milestone.

## 8. Category 4: the consumer at birth

The issue proposes no new symbol, subject, bucket or config field, and this inventory names none. Any test helper
or fixture surface a design adds is new surface and needs its consumer named then.

## 9. Category 5: the problem shape, and coverage

Shape: remove a test-only entry point from a production type, and move the tests that used it onto calls production
code makes. Closest existing instances:

- Pin surface removed with a ledger record: task 3.7a's dropped surface, `docs/admission-ledger.yaml:1759`-`:1772`
  ("each an adapt item, pin → deleted").
- Tests that put a client in a state without an exported setter: in-package field writes (section 4,
  `fakeConsumerClient`, `newConnectedClientWithFakeJS`, `kv_key_contract_integration_test.go:73`-`:76`).
- Tests that drive a real lost connection instead of simulating one: `natsclient/integration_test.go:52` —
  `func TestIntegration_ReconnectionIsRedial(t *testing.T) {` (Restart at `:75`), `client_close_held_test.go:85`
  `TestCloseEndsAReconnectDial`.
- Same rule, other package: #135 (open; graph/readiness Watcher test hooks).

Coverage. `natsclient` is a `task cover:check` target on the merged unit and integration profile, floor 80%
(`scripts/cover-check.sh:16` — `readonly threshold=80`, `:27` — `"natsclient merged"`). Measured:

- Unit profile, `go test -count=1 -coverprofile=/tmp/pr188/unit.coverprofile` over the five unit targets (all pass):
  natsclient 79.0%.
- Integration profile from the last passing run on a tree whose `natsclient`, `internal/harness/natsfixture`,
  (`[r1 N3]` local only, git ignores it:
  `/Users/coby/Code/c360/semengine-wt/claude/adr-109-readiness/.evidence/Cobys-MBP.localdomain-14084-1791763891-27085/integration.coverprofile`,
  sha256 `e365b372744bbd149548d96a04b954dab691ae1d5e1500820ff542e87db6c45d`)
  `go.mod`, `go.sum` equal this one (worktree `adr-109-readiness`, `git_head=0f47445`, `go_test_status=0`, 2026-10-11
  19:14; `git diff --stat 0f47445 HEAD -- natsclient internal/harness/natsfixture go.mod go.sum` empty).
- `scripts/cover-check.sh <unit> <integration>`: natsclient **89.2% merged, 3071 of 3444 statements**; every target ok.
- Unit run with the swap tests skipped (`-skip` the three swap tests and the table's `SetConnection` cells): natsclient
  78.7% unit, 89.1% merged. The integration run is `go test -tags=integration ./...`, so it also runs the untagged
  swap tests; 89.1% still counts them.
- Statements only the swap unit tests reach (covered in the full unit run, not in the skipped one): 12 — the method
  (`client.go:424`-`:437`, 9) and the three swap branches (`:1044`-`:1046`, `:1514`-`:1516`,
  `owned_delivery.go:158`-`:160`). One more block (`client.go:596`-`:598`, `armCircuitTimer` stopping an earlier
  round's timer) differed between the two runs, but no swap test reaches it when run alone: timing variance.
- Bound: if those 12 stay uncovered and the 9 method statements leave, about 3059 of 3435 = 89.05%. Headroom to the
  floor is about 9 points (about 300 statements). The integration swap test's own reach is not measured.

## 10. Triggers checked and not triggered

- Same-class collision table: not triggered. No durable, communication or runtime-coordination primitive is proposed;
  a test entry into an existing one (the client's connection) is removed.
- Intent check: not triggered. No capability boundary is set or moved; no capability in `AGENTS.md` "What this is
  for" is touched.
- Shape sweep: applies to porting pull requests from Slice 04A change 3 on; this is a port-refactor of ported surface.
  Its finding 4 ("A test is not a reader", architect contract `:157`-`:159`) is this change's subject.
- `docs/setup-plan.md`: `grep -n -i -E 'natsclient|test hook|setter|test-only|dead surface'` gives one unrelated line
  (`:378`).

## 11. Adopter seam inventory

Surface reached from outside: `natsclient.Client.SetConnection` (exported; natsclient is public, ledger `:1523`-`:1527`
"SemSource imports it, so it stays public at its pin path"), and Close's documented behaviour, which names the
`SetConnection` case (`client.go:935`; spec `:63`; ADR-112 `:153`).

1. **What a caller of `SetConnection` must know today** (measured from code): (a) the replaced connection is theirs
   to close (`client.go:421`-`:423`); (b) the installed connection carries none of the client's event handlers, so its
   status changes, errors and refusals reach only the caller's own handlers (archived subscribe-registers-interest
   `design.md:206`, `:422`; #169 body), not stated in the method's doc; (c) Close does not cut its socket when Close's
   context ends (`:935`; spec `:63`); (d) once Close has begun the call is ignored with a warn log (`:423`,
   `:430`-`:433`); (e) the JetStream handle is not replaced: `SetConnection` writes only `m.conn` (`:434`), `m.js` is
   written only at `:835` and cleared at `:1050`, so JetStream, KV and consume calls keep using the dialled
   connection; not stated anywhere. Five items.
2. **If they do nothing once it is removed:** a call site stops compiling. Measured call sites: none in SemEngine
   production, none in the four consumers (PR #188 body), at the pin only tests in packages with no ledger row
   (section 6).
3. **Where they find out:** a compile error. Today, (e) is found nowhere and (b) only in archived design text and #169.
4. **What they should have to know**, written as the gap: today items (a)-(e). With the method gone, the setter's
   items go with it, and the qualifier "on a connection the client dialled" in Close's contract (`client.go:931`,
   `doc.go:9`-`:10`, README `:16`, spec `:8`, `:61`, `:74`, `:104`, `:433`-`:434`, ADR-112 `:79`, `:127`) names a
   distinction with one case left.

Prefer observation to prediction: nothing here asks a caller to predict a value; not applicable.

## 12. Searches that closed empty

- `gopls references natsclient/client.go:424:18` (default tags): four sites, all in `client_close_final_test.go`.
- Pin snapshot `grep -rn SetConnection --include='*.go'`: 7 lines in 4 files, no production caller beyond the
  definition. Pin `docs/adr` greps (section 6): none.
- `grep -n -E 'source_path: (internal/boot|processor/agentic-tools)' docs/admission-ledger.yaml`: none.
- `git grep -n -E 'internal/boot' -- openspec/specs docs/setup-plan.md docs/repository-map.md`: none.
- #93's transport-client delta, `grep -n -i 'SetConnection|replaced|dialled'`: none.
- `gh issue list --state open --search 'opHook OR commitHook OR netDial in:body'`: none.
- `ls openspec/changes | grep -v archive`: none.

## 13. Open evidence questions

1. The integration test's property (section 4, last row): does a real disconnect make `Client.MaxPayload` refuse
   while an oversize response on the same connection still fails with `nats.ErrMaxPayload`? A source read of nats.go
   v1.54.0 (not a probe) shows `publish` checking max payload after its closed and draining checks with no
   connected-state check (`nats.go:4668` — `if !nc.initc && msgSize > nc.info.MaxPayload {`) and `(*Conn).MaxPayload`
   ungated (`nats.go:6518`-`:6522`). Not measured. Also not measured: whether the property needs a test once the two
   connections are always one.
2. Merged coverage with the integration-lane swap test gone is not measured (needs a Docker run); section 9 gives a
   bound.
3. ADR-112 `:126`-`:127` cites five tests for "the event handlers of a connection it dialled"; two are swap tests.
   Whether the other three (`:227`, `:835`, `:1328`) carry the claim alone was not read in full.
4. Whether `t.Cleanup(dialled.Close)` (`client_close_final_test.go:1065`) is needed for the closing state without a
   swap: its comment names only the swap; not measured.
5. `[r1 M2]` Other test seams in `natsclient`, measured by the review. Each one's only non-production user is a test.
   They are outside #184's text:
   - `netDial` (`natsclient/client.go:103`): set to `(*net.Dialer).Dial` in `NewClient` (`:364`), read at `:775`, and
     replaced only in tests (`client_close_held_test.go:166`, `client_connect_test.go:224`, `:253`).
   - `opHook` (`:204`): written only in tests (`client_close_final_test.go:518`, `:756`, `:869`;
     `client_lifecycle_test.go:46`, `:333`; `integration_test.go:171`).
   - `commitHook` (`:210`): written only in tests (`client_close_final_test.go:563`, `:592`, `:809`).
   - Production reaches the two hooks only through `recordOp` (`:754`, `:1172`) and `commitSeam` (`:873`, `:2196`,
     `:2212`, `:2392`). None of the three is at the pin.
   - `connectWith`'s `dial` parameter: `natsclient/client.go:724` — `// admission against terminal Close. dial is a test
     seam; production uses`.
     It is at the pin (pin `client.go:478`-`:480`); its only production argument is `nats.Connect`; it has twelve test
     callers in four files.
   - `carriesClientHandlers`: section 2c (M1).
   Whether these ride #184 under the #186 rule or get their own issue is an owner question for the design.
6. For a later port of `internal/boot`: its pin test puts a client on `nats.InProcessServer` through `SetConnection`.
   Whether `Connect` can reach an in-process server is not measured; `connectWith` always adds
   `nats.SetCustomDialer(socket)` and `nats.SkipHostLookup()` (`natsclient/client.go:776`).
7. `task ledger:diff -- natsclient` was not run (it fetches the pin); section 5's statement about its counts comes
   from reading `internal/harness/pindiff/main.go`.
