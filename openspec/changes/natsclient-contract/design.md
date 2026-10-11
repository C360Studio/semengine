# Design: natsclient-contract, revision 3

Status: draft for pre-owner review. Issue #170; claim PR #177; addresses #168. It follows the owner's ruling of
2026-10-11 (#170 comment 6109833136) and his answer to Q1 (comment 6110309758). It rests on `inventory.md` revision
3.

**Probes.** All of them ran on darwin/arm64 against nats.go v1.54.0 and nats-server v2.14.7:

- P17 to P22 and P25: `/tmp/pr177/probes/P17/README.md`;
- P23 and P24: `/tmp/pr177/probes/P23/README.md`.

**Prototype.** `fix-prototype.diff` holds every production change below, with the text-fed unit test it retires.

- It applies to a `git archive` of `0e13a73` (`git apply --check`, exit 0).
- `go vet`, the pinned `revive`, `TestNoRetainedContext` and `TestPublicSignatures` pass on it.
- `go test -race ./natsclient/` passes (`prototype-unit-tests-r3.txt`).

## The problem

nats.go takes one lock for every write on a connection. When the server stops reading, the operating system's send
buffer fills, and a write holds that lock until nats.go's write timeout ends it: one minute, `DefaultFlusherTimeout`
(`nats.go:68`). Every other call on the connection waits for the lock, whatever its context. That includes the
`Drain` and `Close` inside natsclient's `Close`, nats.go's ping timer and `ForceReconnect`. A reconnect holds the
same lock while it dials (`nats.go:3506-3517`).

- **`Close`.** A 50 ms `Close` was held more than 3 minutes in P14. On `main` it was still held at P17's 5 s cap, 4 of
  4 runs. During a reconnect dial to an address that does not answer, it took 4.69 s, 3 of 3 (P23).
- **`Connect`.** It ignores its context while it dials (P18 and P25, `main`):
  - 5.0 s against a server that accepts and never answers, for a deadline or a cancel;
  - 5.0 s to an address that does not answer, for a deadline or a cancel;
  - 10.0 s for two silent URLs;
  - an ended context still dials.

## Decisions

### D1. `Close` cuts the socket, and any reconnect dial, when its context ends

**What the caller sees.** On a connection the client dialled, `Close` returns within its context, even when the
server has stopped reading or nats.go is dialling a reconnect. The connection is then closed, and every call held
behind the write returns an error.

**Evidence.**

- P17 and P25: 51-52 ms, 4 of 4 runs, with the native connection closed and the held publish returning "use of closed
  network connection". The same while a `Subscribe` is held: 51 ms, 4 of 4.
- P23: 51-52 ms, 3 of 3, during a reconnect dial. Revision 2's dialer took 4.69 s there, 3 of 3.

**Mechanism.** natsclient dials through `nats.SetCustomDialer`, which nats.go provides (`nats.go:1657`). The dialer
(`socket.go`) keeps the last socket it dialled, and dials with `net.Dialer{Timeout, Cancel: done}`. When the context
ends, `cut` does three things:

- it closes the socket, which ends the held write at once;
- it closes `done`, which ends a dial in progress;
- it refuses every later dial.

`cleanup` arms the cut with `context.AfterFunc` before it stops anything, and stops and joins it before it returns.

Closing the socket is the only way to end a held write before the write timeout. The ping timer does not fire while
the write is held (P20, 2 of 2), and `ForceReconnect` takes the same lock (`nats.go:2622`).

**`net.Dialer.Cancel`.** The field is deprecated in favour of `DialContext`, but the net package still honours it
(`net/dial.go`), and the Go 1 compatibility promise covers it. The alternatives are worse:

- `DialContext` needs a context, and nats.go's `CustomDialer.Dial(network, address)` passes none.
- Keeping Connect's context in the dialer fails `TestNoRetainedContext`.
- Making one in `Dial` with `context.Background()` invents a root inside a callback, which the developer contract §
  Context ownership forbids.
- Leaving the window open means `Close` waits up to `WithTimeout` (5 s by default) for each dial (P23).

`revive.toml` has no deprecation rule, so `task lint` passes (run on the prototype). No test here shows Go's `net`
package honours `Cancel`: T4 and T8 dial through a stand-in, so they show only that natsclient closes `Cancel` on a
cut. P23 and P25 measured the real behaviour with go1.26.9 on darwin; Go's own `TestDialCancel` covers the field, and
the Go 1 compatibility promise keeps it. After a Go toolchain upgrade, re-run P23.

**Costs.**

- **Unsent bytes are dropped.** nats.go's own `close` flushes its buffer before it closes the socket
  (`nats.go:6190-6191`). The cut can win that race. On a healthy server whose drain outlasts the context, bytes that
  `main` would have flushed are lost. This is ADR-112 §2's "a message still buffered when the connection closes is
  lost".
- **A hostname's addresses are tried in the resolver's order.** Without a custom dialer, nats.go resolves a hostname
  itself, shuffles its addresses and divides `WithTimeout` among them (`nats.go:2484-2511`). With D2's
  `SkipHostLookup`, nats.go hands the dialer the name, and Go's `net.Dialer` resolves it and dials its addresses in
  the resolver's order. nats.go's shuffle of one hostname's addresses is gone; its shuffle of the server list across
  separate URLs (`nats.go:2158-2159`) remains. `net.Dialer` puts the lookup and every address under one `Timeout`,
  giving each address an equal share of what is left, at least 2 s (`net/dial.go:270-290`, `:539`, `:560`). So one
  dial holds the connection's lock for up to `WithTimeout` (5 s by default), the lookup included. That is Go's code,
  read; no test here measures it, and ADR-112 states it as not promised. `Close` and `Connect` cut the dial; other
  calls wait.
- **`SetConnection`.** A connection installed through `SetConnection` was not dialled by the client, so it is not cut.

**Size.** `socket.go`, 67 lines; about 20 lines in `client.go`, where the `netDial` test seam is a field on `Client`.

### D2. `Connect` honours its context

`Connect` refuses an ended context before it dials. While it runs, `context.AfterFunc(ctx, cut)` cuts the dial or the
handshake when the context ends, whether by deadline or by cancel. Once Connect has connected it stops and joins the
`AfterFunc`, so the context no longer touches the connection. Reconnects are then bounded by `WithTimeout` alone.

nats.go looks a hostname up before it calls a custom dialer, and nothing ends that lookup (`nats.go:2484-2485`). In
Codex's stop-1 finding F1 (PR #177 comment 6111641674, reproduced 3 of 3 in comment 6111684071), `Connect` was still
blocked 500 ms after the cancel, until the probe released the lookup. So `Connect` also passes
`nats.SkipHostLookup()`. The name reaches the dialer, and `net.Dialer` resolves it under the dial's `Timeout` and
`Cancel` (T4, "host name resolved inside the dial"). The cost: a hostname that resolves to several addresses is
dialled in the resolver's order, and nats.go's shuffle of those addresses is gone; its shuffle of the server list
across separate URLs remains (D1, "Costs").

| Piece | `main` (P18, P25) | Fix (P18 revision 2, P25 revision 3) |
| --- | --- | --- |
| Ended-context check | 5.0 s, 1 connection accepted | 0 s, none accepted |
| Cut during the dial: no answer to the dial, deadline or cancel | 5.0 s | 301-302 ms |
| Cut during the handshake: silent server, deadline or cancel; two URLs | 5.0 s; 10.0 s | 301-302 ms |
| `AfterFunc` stopped once connected | (not present) | a reconnect after the context ended works |

**Size.** About 10 lines. Revision 2's dial deadline and its reset are gone.

### D3. natsclient holds no lock of its own while it calls nats.go

`subscribeOwned`, `MaxPayload` and `SetConnection` call `conn.IsConnected()` under natsclient's lock
(`client.go:1339-1358`, `:398-405`, `:410-420`). `IsConnected` takes nats.go's lock, so a held write holds
natsclient's lock too, and `Close` blocks before it can arm the cut.

- P21 is the fix without this change to `subscribeOwned`. `Close` was held past 5 s in 2 runs, and took 1.4 s and
  1.5 s in 2 more.
- With the change: 51 ms in 4 of 4 runs.

The fix reads the field under the lock, and calls nats.go after releasing it. When `subscribeOwned`'s check fails
because `Close` began in that gap, it returns `nats.ErrConnectionClosed`, not `ErrNotConnected`. That is what
`transport-client` asks of a call that `Close` ended. **Size:** about 15 lines.

### D4. The write timeout keeps its default, and a caller can set it (owner, Q1)

The owner answered "lean no, but it should be something we can control": the default stays at one minute, and no
test waits for it. Calls other than `Close` and `Connect` still wait behind a write the server is not reading, up to
the write timeout for each write ahead of them (P17: still held at the 5 s cap).

natsclient gains two options. Both refuse a value that is not positive (P24):

- **`WithWriteTimeout(d)`** sets `nats.FlusherTimeout`. Zero would mean that nats.go never times out a write
  (`nats.go:2325-2326`).
- **`WithPingInterval(d)`** sets `nats.PingInterval`. The field already exists (`client.go:92`), 30 s by default.
  Zero would turn the PINGs off (`nats.go:2931`).

**Two options, not one.** Each mirrors one nats.go option, as `WithReconnectWait` and `WithTimeout` already do. A
caller can also want a short ping alone: it detects an idle server that has stopped answering (P24 idle). One option
taking two durations of the same type invites swapped arguments.

**The two belong together.** `WithWriteTimeout`'s doc comment says to set `WithPingInterval` with it. With a 300 ms
write timeout alone, the held write failed in 3 of 3 runs, but the connection stayed `CONNECTED` and carried no
request for 5 s after the server read again: 3 of 3, through the options in P24 and through nats.go's own options in
P22. With a 300 ms ping as well, it reconnected and an echo succeeded 3 ms after the resume, 3 of 3 (P24).

**No option for `MaxPingsOutstanding`.** nats.go's default of 2 stays, because nothing needs another value.

**What the stale-connection check takes.** At 300 ms, the status left "connected" 1.205-1.212 s after the server stopped
reading: 5 of 5 runs (P20 and P24 idle), about four intervals. nats.go declares the connection stale at the third PING
tick with no PONG (`nats.go:5993-6013`, read). The probes' proxy finishes a read already in progress when it stops, so
the first PING after the stop was answered (a read of the probe's code). At the default 30 s that is 60-90 s by the
code, and up to 2 minutes by the probe's count. ADR-112 marks the 30 s timing not promised.

**Measured costs of a shorter write timeout,** for a caller that sets one:

- **A healthy slow link fails more writes.** At 16 KB/s: 5 failed writes at 1 s, 1 at 5 s, none at the default. At
  8 KB/s: 3 at 2 s, 1 at 10 s, and 0 or 1 at the default. Where it was 1, the default fired once, in a run that
  lasted 65 s. (P19, P21; the pacing proxy is approximate.)
- **One deadline covers a whole flush** (`nats.go:6918-6923`, `:2361-2373`), so a 1 MB message needs 100 KB/s at
  10 s.
- **`ReconnectOnFlusherError` is not used.** It acts only when the background flusher fails, healed 0 of 3 (P22),
  and lost 19 of 71 and 20 of 48 accepted publishes on a reconnect (P19).

**Rejected.**

- Cutting the socket when any call's context ends: one caller's deadline would end the connection for all callers.
- Returning early and leaving the call running: that is unowned background work.

**Size.** About 35 lines in `options.go`, and one option line in `buildConnectionOptions`.

### D5. Error classes: two sentinels, and no responders left alone

On `main`, `errs.IsTransient` returns false for both sentinels, and `errs.Classify` calls them transient by its
default (#167).

| Error | Change | Then, on `main` and after #93 |
| --- | --- | --- |
| `ErrNotConnected` | `fmt.Errorf("not connected to NATS: %w", errs.ErrConnectionLost)` | both say transient (sentinel list, `errs.go:168` on `main`, `:169` in #93) |
| `natsclient.ErrCircuitOpen` | `= errs.ErrCircuitOpen`, one sentinel instead of two | both say transient (`errs.go:171`, `:172`) |

**No responders is not changed.** It stays the raw `nats.ErrNoResponders`, matched by `errors.Is` and
`IsNoResponders`. #93 decides commit outcomes on whether an error is a `*errs.ClassifiedError`
(`internal/graphmutation/client.go:153-162`, `pkg/projection/mutation_client.go:441-447`). Wrapping no responders
would turn its "definitely not committed" into "rejected" or "commit-unknown". Its class waits for #93's D7, which
owns the raw nats.go sentinels. Also waiting for #93: #167, `Classify`'s default for unknown errors.

`pkg/errs` is not touched: #93 rewrites it (#146), and the fence (#93 comment 6109839233) keeps it out of here. No
production code compares either sentinel with `==` or by its text, on either tree.

`request_integration_test.go:70-74` and `:429-434` compare error text, and move to `errors.Is`.

### D6. The two text classifiers

- **`isAlreadyExistsError` is replaced by `errors.Is(err, jetstream.ErrBucketExists)`** at its one caller,
  `CreateKeyValueBucket` (`client.go:1887`). nats.go joins `ErrBucketExists` to every create that fails with
  `ErrStreamNameAlreadyInUse`, code 10058 (`jetstream/kv.go:541-544`; `errors.go:105-107`, `:414-416`). The function
  and its text-fed unit test (`client_test.go:560-583`) go.
- **`isCircuitNeutralStreamCapacityError` stays.** It already matches the typed code, 10077. nats-server sends that
  code for every store failure and names the limit only in the description (`JSStreamStoreFailedF`, `{err}`;
  `store.go:49-53`). nats.go has no code for the limit. Its comment (`client.go:516-519`) is corrected, and
  `stream_capacity_circuit_integration_test.go:14` backs it.

### D7. Statements, the ADR and the rule

- **The statements.** `cleanup.md` fixes the 46 statements, and the two this change makes false: the sentinel
  definitions printed in `README.md:249-250` and `doc.go:247-248`.
- **ADR-112** records the promises after the fix, and each line names its probe or test.
- **Spec deltas.** `transport-client` changes three requirements and adds four. `nats-fixture` changes one scenario.
  The archive commit edits `transport-client`'s Purpose to "on a connection it dialled".
- **The rule** goes into the four role contracts, and its row into `AGENTS.md` (`rule.md`).

## Tests that replace the probes

Every test follows `docs/testing.md`:

- the embedded server binds port -1, and a test-owned proxy listens on `127.0.0.1:0`;
- no test sleeps; each waits on a channel or `probe.Await`;
- cleanup contexts have a deadline;
- no test runs in a `synctest` bubble;
- no test waits on the one-minute default.

**The hold.** The proxy stops reading once it has read the first bytes of a 16 MiB publish, and the server's
`max_payload` is 32 MiB. nats.go holds its lock for the whole of that publish's flush (`nats.go:2346-2347`), and the
message cannot fit in the socket buffers.

**Cleanup order.** The proxy registers its cleanup after the client's, so it runs first and closes both of its
sockets. A failing-first run on `main` therefore ends its held publish at once rather than after a minute.

| Test | Shows | Replaces | Fails before the change because |
| --- | --- | --- | --- |
| T1 `TestCloseEndsAHeldWrite` | `Close` with 50 ms returns the context's error, the held publish returns an error, and the connection is closed | P17 close | `Close` waits for the write; the test's own 10 s wait fails |
| T2 `TestCloseIsNotHeldByAHeldSubscribe` | the same while a `Subscribe` is held; `Close` is called once the `Subscribe` is admitted (`probe.Await` on the count of running work) | P17 close+sub, P21 | the `Subscribe` holds natsclient's lock, so it is never admitted |
| T3 `TestConnectRefusesAnEndedContext` | a transient error matching `context.Canceled`, and the listener accepts nothing | P18 ended | `Connect` dials |
| T4 `TestConnectEndsWithItsContext` | against a listener that accepts and never answers, each case returns the context's error before the test's 3 s wait: a 100 ms deadline, a cancel sent once the listener has accepted, and two such URLs | P18 silent, two-silent | they take 5 s and 10 s |
| T5 `TestReconnectAfterConnectContextEnded` | after the server drops the client, a request to an echo responder succeeds | P18 reconnect-after | passes on `main`; a mutant that leaves Connect's `AfterFunc` armed fails it (`task mutate:check`) |
| T6 `TestCreateKeyValueOfAnExistingBucketIsErrBucketExists` | a second create with a different config matches `jetstream.ErrBucketExists` | nats.go read | passes; it pins the library fact for #118 |
| T7 `TestClientSentinelsAreTransient` | `ErrNotConnected` and `ErrCircuitOpen`: `IsTransient` and `Classify` agree; `errors.Is` matches `errs.ErrConnectionLost` and `errs.ErrCircuitOpen` | P10 | `IsTransient` is false |
| T8 `TestCloseEndsAReconnectDial` | `netDial` blocks every dial after the first until its `Cancel` closes, and signals when it is entered; the server drops the client; once the dial is entered, `Close` with 50 ms returns before the test's 3 s wait | P23, P25 blackhole-cancel | the dial is not cancelled |
| T9 `TestWriteTimeoutAndPingIntervalOptions` | with both at 300 ms, the held publish fails, then the connection reconnects and carries an echo after the proxy reads again; with a value that is not positive, each option is refused | P22, P24 | the options do not exist |

The unroutable-address case has no in-tree test: whether an address is unroutable depends on the host. T8 tests the
same cut through the seam.

Before #118's upgrade merges, T1 to T9 re-run under `task verify`, so the probes need not be re-run.

## Order with #93 and other work

Both orders archive cleanly with OpenSpec 1.14.0 (`archive-177-then-93.txt`, `archive-93-then-177.txt`): the two
capabilities come out with the same lines. After #93, five specs fail `validate --all --strict`. They are the five
that #93 creates, and their Purpose is still the placeholder that `openspec archive` writes.

**Code.** #93 changes `README.md`, `kv.go` and `storage_inventory.go`. This change's hunks in those files do not
overlap the hunks of #93, except at `kv.go:323` (M79), which is left to #93. The README sentinel rows (`:249-250`) lie
three unchanged lines from #93's insertion after `:253`, so the merge is clean. Both changes append to the natsclient
ledger row's contract text at `docs/admission-ledger.yaml:1695`, and the second to merge keeps both. Either can merge
first.

**Other work.** #176 has merged, and #118 holds (comment on #118).
