# Inventory: simplify-04a-16-natsclient-circuit

base: 3c1c1e6328888f3b3356a8339a87a555e709ca73 (claim head 11c8bde, no content)

Question: what #222 part c touches. Part c asks for three things: `testCircuit` resets the count; the health monitor
stops tripping the breaker on `AccountInfo` errors; typed atomics instead of `atomic.Value`. Also asked: whether each
defect exists on today's tree, and who reads the circuit and status surface. Repositories read
(docs/inventory-scope.md): this one; SemStreams at the pin `8b99efe9` (`gh api …/tarball/8b99efe9`); semsource
`e4febc0`, semconnect `d0d06e0`, semteams `0f1a20b`, semboids `8c03cc5`. The consumers were read at symbol level from
tarballs with a file grep only, at the same SHAs as simplify-04a-14's inventory. Callers were measured with
`gopls references` under `GOFLAGS=-tags=integration`. Word searches use `git grep -n -w`; the known positive is
`recordFailure`, which returns 50+ hits.

## 1. The claimed gap

- **`testCircuit` leaves the count non-zero, so one failure reopens the circuit.** This does not hold as stated.
  - `natsclient/client.go:463` — `m.circuitFailures.Store(0)`: the round's count is zeroed when the circuit opens.
  - Only failures counted while the circuit is open survive the test. Those come from async acks and calls already in
    flight, and they are fewer than the 15 the threshold needs (`:480` zeroes them at the threshold).
  - `natsclient/client.go:575` — `func (m *Client) testCircuit() {` sets `:582` — `m.setStatus(StatusDisconnected)` on
    the installed, live connection.
  - The stream seams then refuse:
    - `natsclient/stream.go:167` — `if c.Status() != StatusConnected {` (also `:508`, `:791`, `:1080`; `client.go:1468`,
      `:1535`, `:1621`, `:1774`, `:1823`, `:1883`, `:1971`, `:2001`). They return `ErrNotConnected` until the health
      monitor sets `Connected` (`client.go:2262` — `if healthy && m.Status() != StatusConnected {`).
    - That wait is up to one `healthInterval`: 10 s by default (`:330`) and 30 s in semsource (`run.go:451`). With
      `WithHealthInterval(0)` it lasts until a reconnect event (`:2073` `handleReconnect`).
  - `resetCircuitLocked` (`:525-534`) would also set `backoff` to 1 s, defeating `TestCircuitBreaker_ExponentialBackoff`
    (`client_test.go:73`).
  - No test calls `testCircuit`: gopls finds one reference, `client.go:568`.
- **The health monitor trips the breaker on `AccountInfo`.** Absent.
  - `git grep -n -w AccountInfo -- natsclient` finds only `storagereport/storage_account.go` and a test fake
    (`getstream_circuit_breaker_test.go:50`).
  - `git log -S AccountInfo -- natsclient/client.go` finds no commit.
  - Pin `natsclient/client.go:1616-1667` has none either.
  - The monitor (`client.go:2224-2280`) calls `conn.IsConnected()` and `conn.RTT()` (`:2255`) and never calls
    `recordFailure`.
- **Six `atomic.Value` fields, and `subsMu`/`jsMu`/`healthMu`.** Three fields; the mutexes do not exist.
  - `natsclient/client.go:72` — `status   atomic.Value // stores ConnectionStatus`
  - `natsclient/client.go:86` — `lastFailure      atomic.Value // stores time.Time`
  - `natsclient/client.go:87` — `backoff          atomic.Value // stores time.Duration`
  - `git grep -n 'atomic.Value' -- natsclient` finds only these three.
  - `git grep -n -w -e lastError -e disconnectTime -e lastSpinAttempt -e serverInfo -e healthMu -e jsMu -e subsMu --
    natsclient` finds nothing.
  - The same holds at #222's base `a230405` (`git show a230405:natsclient/client.go`).
- **Panics from untyped loads.**
  - `client.go:410` — `return m.backoff.Load().(time.Duration)` and `:636` —
    `lastFailure := m.lastFailure.Load().(time.Time)` panic on a `Client` not built by `NewClient`.
  - No production code builds one (gopls finds no `&Client{}` outside tests). There is no real-world trigger (#185).
  - Every write already holds `mu` (`:345` `setStatus` doc; `recordFailureLocked` `:432`; `resetCircuitLocked` `:525`).

## 2. Every spelling of "is the connection usable"

- **nats.go's own state:** `client.go:1413` — `if conn == nil || !conn.IsConnected() {` (`Publish`);
  `request.go:116-121`, `:198-204` (requests).
- **The stored status:** `Status() != StatusConnected` at the stream seams listed in § 1.
- **The breaker:** `Status() == StatusCircuitOpen` at `client.go:689`, `:1464`, `:1531`, `:1617`, `:1770`, `:1819`,
  `:1879`, `:1967`, `:1997`; `stream.go:163`, `:505`, `:788`, `:1076`; `request.go:121`, `:203`; `errors.go:279`.
- **A fourth, dead spelling:** `client.go:2036` — `func connStatus(conn *nats.Conn) ConnectionStatus {` (gopls: 0
  references).
- **Calls that neither check nor count the breaker:** core `Publish` (`:1405-1426`), `Subscribe`, and `KVStore` calls
  (`kv.go` has no `recordFailure`).
- **Exceptions written to keep application errors out of the count:**
  - not found (`client.go:1788`, `:1898`);
  - stream capacity (`:494-512`, by error text);
  - cancellation (transport-client "A retrying request…");
  - readiness (`request.go:155-160`: "trip the shared per-client circuit breaker and fast-fail UNRELATED calls").
- **Calls still counted:** a `CreateStream` error (`:1474`), a publish to a subject no stream captures (`stream.go:1103`
  via `recordStreamPublishFailure`), and a consumer `Info` error (`consumer_policy.go:53`).
- **The pin is the same:** `natsclient/client.go:254-361`, with `CompareAndSwap` in place of `mu`.

## 3. Adjacent claims

- **Specs:**
  - transport-client "Only Close changes the status once Close begins" (`spec.md:123-139`; names `Failures()` and the
    circuit).
  - "A retrying request refuses what it cannot attempt" (`:205-243`; circuit clause, "failure count is unchanged" ×2).
  - "The client's refusals carry the transient class" (`:404-417`; scenario "Circuit open").
  - projection-mutation "One mapping from a mutation request's error to its commit state" (`spec.md:119-170`; names
    `natsclient.ErrCircuitOpen`, scenario "natsclient refused the request before sending").
- **ADR-112:** `:164` (`ErrCircuitOpen` is `errs.ErrCircuitOpen`); `:170-172` (stream capacity is circuit-neutral);
  `:236` (a retry keyed on `IsTransient` retries a circuit refusal).
- **Ledger row `natsclient`:**
  - `docs/admission-ledger.yaml:831` — `the NATS and JetStream client: connection lifecycle with circuit breaker and
    health status,`
  - `:864` (threshold and max backoff defaults), `:873`, `:879`, `:925`, `:1030`, `:1137-1139` (audit item (c) on
    `testCircuit`'s comment), `:1323-1327`, `:1398-1411`, `:1578-1579`, `:1641-1647`.
  - `:2028` (graphmutation row: `ErrCircuitOpen` is `TransportUnavailable`).
- **Docs:** `natsclient/README.md:3`, `:8`, `:37-41`, `:164`, `:205`, `:231-233`, `:245`, `:271`, `:284-285`, `:346`,
  `:353`; `natsclient/doc.go:1`, `:5`, `:15-16`, `:24`, `:203-218`, `:231-232`, `:254`, `:333`, `:352-359`, `:446-451`.
- **Open PRs** (`gh pr list --state open`):
  - #286 (8c, 100 files, full list read with `gh api --paginate`) changes `internal/graphmutation/client.go`,
    `natsclient/kvspec.go`, `processor/graph-ingest/component.go` and `docs/admission-ledger.yaml`. Its hunks hold no
    circuit or status symbol.
  - #289 (8e) has no overlap. #290 is this claim.
  - #242-#248 are dependency bumps; none touches these files.
- **Open issues** (`gh issue list --state open --search natsclient`, and `circuit`, `"health monitor"`):
  - #222: names this work (part c).
  - #223: the epic.
  - #75 and #169: 6d.
  - #114: the `model/breaker.go` breaker, a different package.
  - Names the word, not this surface: #195, #74, #83, #141, #162, #133, #139 (graph-ingest's `atomic.Value`), #132,
    #216, #281, #280, #68, #24, #9, #135, #60, #103, #134, #174, #28, #95.

## 4. Readers (who it is for)

| Symbol | Repo production | Pin outside natsclient (production) | Consumers (production) |
| --- | --- | --- | --- |
| `Client.Status`, `StatusConnected` | `graph-ingest/component.go:982` | graph-query `component.go:356`; graph-gateway `:61` (excluded) | semconnect `cs-api/component.go:290-291` |
| `StatusCircuitOpen` | natsclient only | none (doc.go only) | none; semsource test `workbench_capabilities_integration_test.go:58` |
| `ErrCircuitOpen` | `internal/graphmutation/client.go:171` | agentic-dispatch `command_effect.go:139` (not ported) | semsource `code-context/http_error.go:69`, `internal/entitypub/classify.go:54` |
| `Client.Failures`, `Backoff` | none (tests: 12 files) | none | none |
| `Client.GetStatus` | none (tests) | `service/service_manager.go:1745` | none |
| `Status.FailureCount` | none (tests) | `service/service_manager.go:1752` | none |
| `Status.LastFailureTime` | none (`client_test.go:459`) | none | none |
| `Status.Reconnects` | none (never written) | none | none |
| `Client.IsHealthy` | `client.go:597` (WaitForConnection) | `service/base.go:434`, rule `processor.go:1041` | none |
| `Client.RTT`, `Status.RTT` | none / `client.go:651` | `service_manager.go:1748` | none |
| `WithHealthInterval` | none (tests) | none | semsource `cmd/semsource/run.go:451` |

`service`, graph-query and rule are admitted (`docs/tier1-cross-check.md:64-65`). agentic-dispatch is not ported
(`:97`).

## 5. Problem shape

The problem is a predicted refusal against an observed outcome. The breaker predicts that the connection is bad from
a count of other calls' errors. The closest existing instance of the observed form is ADR-112 § 5: while
reconnecting, calls return `ErrNotConnected` at once, which is nats.go's own state (P15). The same-class breaker
`model/breaker.go:108` (`NewRollingWindowBreaker`, per LLM endpoint, #114) is not shared across callers and is not
adopted here.

## 6. Pin decision records

None governs the breaker or the health monitor:

- `grep -l -i -e circuit -e IsHealthy -e 'health monitor'` over the pin's `docs/adr/*.md` matches only "short-circuit"
  prose.
- `grep -n -E 'IsHealthy|natsclient\.Status|GetStatus|StatusConnected|WithHealth'` matches only ADR-058:42, which is
  `service.Service`'s own interface.
- SemEngine's ADR-112 governs (§ 3).

## Shape sweep (today's tree; the pin is the same unless noted)

1. **Second spelling:** § 2, four spellings of "usable".
2. **Exported surface with no reader:**
   - `Backoff`, `Failures` and `Status.LastFailureTime` are read by tests only.
   - `Status.Reconnects` is read and written by nothing.
   - `connStatus` is unexported and dead.
3. **Classification by text:** `client.go:501-512` reads `APIError.Description`. ADR-112 `:170-172` accepts this,
   because nats.go has no code for it.
4. **Advertised but absent:**
   - `doc.go:207-211`: "Half-Open: Testing if system recovered". There is no half-open state.
   - `doc.go:213-218`: `time.Sleep(client.Backoff())` sleeps the next round's backoff, not the current one
     (`:450-455`).
   - `README.md:39-41`: "gradually tests the connection". `testCircuit` tests nothing; the ledger fixed only its code
     comment (`:1137-1139`).

Collision table: not triggered, because no primitive is proposed. Intent check: not a boundary change.

## Adopter seam inventory (a product calling natsclient, today)

1. **What they must know:**
   - Any caller's errors count toward one shared breaker.
   - `ErrCircuitOpen` can come from a call that never reached the server.
   - After the circuit test, a live client reports `Disconnected` and stream calls refuse until the health tick.
   - `WithHealthInterval(0)` makes that refusal last until a reconnect.
   - Which application errors count and which do not.

   That is five facts, more than two: a design finding.
2. **If they do nothing:**
   - One component's bad stream name refuses every component's JetStream, request and Connect calls.
   - Health reads unhealthy (semconnect `:291`, pin `service/base.go:434`).
   - graph-ingest `Start` fails with "already connected" (`component.go:982-984`).
3. **Where they find out:** an Info log "Circuit breaker opened" (`client.go:457`), and otherwise nowhere.
4. **What they should have to know:** nothing. Each call returns its own error.
