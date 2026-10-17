# ADR-112: What natsclient Promises Over nats.go

## Status

**Accepted (2026-10-11); §4 amended 2026-10-12 (#184); §3 amended 2026-10-15 (#167); §2 amended 2026-10-17
(#169).** The owner accepted design revision 3, and this record with it, on PR #177 (comment 6111038987). Drafted
on PR #177, for #170 and #168, under the owner's rulings:

- 2026-10-10, epic #9 comment 6098388541: a record of natsclient's contract over nats.go;
- 2026-10-11, #170 comment 6109833136: fix natsclient so its calls do not sit for minutes, without rewriting nats.go;
- 2026-10-11, #170 comment 6110309758: keep nats.go's write timeout, but make it settable, so no test waits a minute.

The record was measured at nats.go v1.54.0 and nats-server v2.15.0, re-run on 2026-10-11 (`go.mod:12-13`,
`.nats-image:5`). The first measurement was at nats-server v2.14.7. #118 moved the pin to v2.15.0 on 2026-10-11, and
the quick probes (P10, P12, P13, P15, p15b, P17, P18, P20, and P23 to P25) were run again there with the same results
(PR #177 comment 6111119623). P11, P14, P16, P19, P21 and P22 were measured at v2.14.7 only.

**Amendment, 2026-10-12 (#184).** `Client.SetConnection` is removed, with the code and tests that served it (owner,
2026-10-12: "I have no idea why we have a setter in Go for SetConnection — this is not Java"). §4's sentence about a
connection it replaced is struck where it stands. §2's `Close` line still cites `:1399` and `:1442`, tests removed
with it. Line numbers in `client_close_final_test.go` are as of `d71df33`.

**Amendment, 2026-10-15 (#167).** Two "not promised" lines in §3 are now promised (owner ruling 4 on #167, comment
6191970843). The original bullets stay where they are; this note replaces what they leave open.

**Amendment, 2026-10-17 (simplify-04a-16-natsclient-circuit, #222).** The connection circuit breaker is removed, so
`natsclient.ErrCircuitOpen`, the circuit-neutral stream-capacity line in §3 and the circuit refusal in the retry
line are struck where they stand. No library statement is added.

**Amendment, 2026-10-17 (simplify-04a-18-natsclient-metrics, #169).** §2's `Subscribe` line is unchanged in what it
returns: a refused SUB still returns the subscription and nil (owner ruling on #169, 2026-10-12). What is added is
that, when the client was built with `WithMetrics`, the refusal is also counted on
`semengine_nats_server_refusals_total{reason}`, with `permission_violation` or `max_subscriptions_exceeded`. The
count is made before the client's `NATS error` log line. nats.go reports a refused PUB the same way, so a refused
publish is counted too. Backed by `TestServerRefusalCountedBeforeLog` (`subscribe_registration_test.go`). It is not
promised how soon after the call returns the count rises. Without metrics only the log line is written.

- **The unknown-error default.** `errs.Classify` returns fatal for an unclassified error: one with no
  `*errs.ClassifiedError` in its chain and none of the sentinels `errs.IsTransient`, `errs.IsFatal` or `errs.IsInvalid`
  list, whatever its text says. Those three checks stay strict and report false for it. The
  `error-classification` spec holds this. `natsclient` sends an unclassified handler error as fatal.
- **No responders' class.** The raw `nats.ErrNoResponders` is unclassified, so `errs.Classify` returns fatal for it
  and `errs.IsTransient` reports false. The `projection.MutationError` that `pkg/projection` and `pkg/lifecycle` return
  for it is *not committed* and carries `Class` transient in its field; `errs.IsTransient` on that error reads through
  to the raw error and is also false. The `projection-mutation` spec holds the field.
- **Still not promised:** which other raw nats.go errors become classified sentinels. Until one does, it is
  unclassified and falls under the default above.

**Amendment, 2026-10-17 (change `simplify-04a-14-natsclient-request`).** `Client.Request` is removed, so it no longer
appears in the lists of calls a held write holds (§2) and of calls that return `ErrNotConnected` while reconnecting (§3).
`RequestWithHeaders` takes its place in the held-write list, with the classified calls through it: P13's `request`
case ran the removed `Request`, which made the same nats.go call (`conn.RequestMsgWithContext`) that
`RequestWithHeaders` makes. P13 was not re-run on `RequestWithHeaders`.

**How each line is backed.**

- **Probe.** A probe is a program posted on PR #177 with its source, command, versions and output. Probes ran on
  darwin/arm64, and a line backed by a probe alone describes those versions. The PR #177 comments that hold them:
  - P10 to P16, one comment each, in order: 6109196968, 6109197143, 6109197294, 6109197438, 6109197611, 6109197772,
    6109197927;
  - the reviewer's p15b: 6109378036;
  - P17 to P22, and P25: 6110634199;
  - P23 and P24: 6110634423;
  - the re-run at nats-server v2.15.0: 6111119623.

  P7 and P8, cited in section 2, are PR #156's (comment 6098410018). The experiment that archives this change's
  `transport-client` delta and #93's in both orders is PR #177 comment 6109434169.
- **Test.** A test is in the tree and drives the pinned nats.go against the pinned server: embedded, or the
  `.nats-image` container. T1 to T9 are the tests this change adds, all in `natsclient/`:

  | Test | Name | File |
  | --- | --- | --- |
  | T1 | `TestCloseEndsAHeldWrite` | `client_close_held_test.go` |
  | T2 | `TestCloseIsNotHeldByAHeldSubscribe` | `client_close_held_test.go` |
  | T3 | `TestConnectRefusesAnEndedContext` | `client_connect_test.go` |
  | T4 | `TestConnectEndsWithItsContext` | `client_connect_test.go` |
  | T5 | `TestReconnectAfterConnectContextEnded` | `client_connect_test.go` |
  | T6 | `TestCreateKeyValueOfAnExistingBucketIsErrBucketExists` | `kv_bucket_exists_test.go` |
  | T7 | `TestClientSentinelsAreTransient` | `client_sentinels_test.go` |
  | T8 | `TestCloseEndsAReconnectDial` | `client_close_held_test.go` |
  | T9 | `TestWriteTimeoutAndPingIntervalOptions` | `client_write_timeout_test.go` |

  Some depart from that definition in part. T3 never reaches a server, and T4's cases "deadline", "cancel once
  accepted" and "two addresses" run against a test listener that accepts and never answers, in place of a server. T8,
  and T4's cases "deadline while dialling" and "host name resolved
  inside the dial", dial through a stand-in for the network dial (see Alternatives). T7 checks natsclient's own two
  sentinels and drives no server.

## Context

`transport-client` promised that `Close` returns within its context. Nobody measured nats.go when that was written,
and it was false:

- nats.go takes one lock for every write on a connection.
- A write the server is not reading holds that lock until nats.go's write timeout ends it (one minute,
  `nats.go:68`). A reconnect holds it while it dials (`nats.go:3506-3517`).
- `Close`'s `Drain` and `Close` wait for the lock. A 50 ms `Close` took more than 3 minutes (P14), and 4.69 s during
  a reconnect dial (P23).
- `Connect` ignored its context while it dialled (P18, P25).

The owner ruled that natsclient exists to make the framework's life easier, so it fixes this with plain code. It does
not take over what nats.go does.

## Decision

natsclient uses nats.go's connection as nats.go provides it. It adds four things:

- a dialer it can cut (`nats.SetCustomDialer`), so that `Close` and `Connect` end with their context;
- no natsclient lock held across a nats.go call;
- two options, `WithWriteTimeout` and `WithPingInterval`, which set nats.go's own settings;
- two sentinels that carry their class.

Each line below names its backing. Anything not backed is not promised.

### 1. Time

- **`Close` returns within its context** on a connection the client dialled. That holds even when the server has
  stopped reading, and while nats.go dials a reconnect. When the context ends, `Close` closes the socket, ends the
  dial, and refuses later dials; the connection closes, and every held call returns an error. Backed by T1, T2 and T8,
  and by P17, P23 and P25: 50-52 ms in every run, at both server versions.
- **`Connect` returns within its context.** An ended context is refused before any dial. A deadline or a cancellation
  ends the dial or the handshake. Backed by T3 and T4, and by P18 and P25: 301-302 ms, where `main` took 5 s or 10 s.
  A hostname reaches natsclient's dialer unresolved (`nats.SkipHostLookup`), so its lookup ends with the context too
  (T4; PR #177 comments 6111641674 and 6111684071). A hostname's addresses are then dialled in the resolver's order,
  within each address family: nats.go's shuffle of them is gone, and its shuffle of separate URLs remains (read, not measured:
  `nats.go:2484-2513`, `net/dial.go:539-557`). The context
  does not bound later reconnects (T5).
- **Every other call does not end with its context while a write is held.** This covers `Publish`, `RequestWithHeaders`
  (and the classified calls through it), `Subscribe`, the JetStream and key-value calls, and a consumer's setup.
  - The context ends their wait for the server's answer (P13 idle; tests `subscribe_registration_test.go:296`,
    `:318`), not their wait for nats.go's lock.
  - Behind a write the server is not reading, each call waits for every write ahead of it, up to the write timeout
    each, and `Close` ends the wait. In P17 each held call was held until the write ended: past the 5 s cap, or
    1.6-2.0 s when darwin's buffers took it; never at the call's deadline. In P13 each returned 0 to 29 ms after the
    server read again.
- **The write timeout and the PING interval are the caller's to set.**
  - `WithWriteTimeout` and `WithPingInterval` default to one minute and 30 s, and refuse a value that is not positive
    (T9; P24).
  - With both at 300 ms, a held publish failed at the write timeout, and the connection reconnected and carried a
    request 1 to 3 ms after the server read again (P24, 3 of 3 at each server version).
  - With the write timeout alone, it stayed connected and carried nothing for 5 s (P22, P24: 6 of 6). So set them
    together.
- **Not promised:**
  - **That the connection works after a write timeout.** It recovers when nats.go's stale-connection check closes
    it. That check declares the connection stale at the third PING with no answer (`nats.go:5993-6013`, read). At
    300 ms it took 1.21 s (P20, P24 idle, 5 of 5). At the default 30 s it is not measured: 60 to 90 s by the code.
  - **How long a reconnect attempt holds the connection.** Go's `net.Dialer` puts a hostname's lookup and all its
    addresses under one `WithTimeout` (5 s by default). Each address gets an equal share of what is left, raised to
    2 s only while 2 s remain: with the default 5 s, a first and a second address get 2 s each, a third about 1 s, and
    a fourth none in that attempt. IPv4 and IPv6 addresses are dialled as two lists, the second starting 300 ms after
    the first (`net/dial.go:270-290`, `:655-725`). Read from Go's source, not measured; no test here measures it.
  - **Timing on Linux.**
  - **How soon a dead server is detected while a write is held.** The ping timer does not fire then (P20).

### 2. What a nil return means

- **`Subscribe`, `SubscribeForRequests`:** the server has read the SUB, not that it accepted it. A refused SUB returns
  the subscription and nil, and is counted when metrics are configured (amended 2026-10-17, see Status).
  Backed by the test at `subscribe_registration_test.go:240`, and by P7, P8 and #169.
- **`Publish`:** the message is in nats.go's buffer, not sent: nil came back after 1 m 59.6 s with the server never
  reading (P14). Delivery is not promised: a message still buffered when the connection closes is lost, including
  when `Close`'s context ends before its drain does.
- **`PublishToStream`:** the server stored it, in the stream's storage class. On a memory stream it does not survive
  a restart (test `integration_test.go:109`).
- **`Close`:** everything the client owns has finished: handlers, background work, and the event handlers of a
  connection it dialled (tests `client_close_final_test.go:227`, `:835`, `:1328`, `:1399`, `:1442`). The async
  publish error handler is joined only as far as nats.go lets the client see it. That limit is declared, not
  promised.
- **A KV write that returned an error may still have committed.** `Put` (P15), and `Create` and `Update` (p15b),
  returned `context deadline exceeded` during a reconnect, and their values stood afterwards.

### 3. Error classes

- **Read a class with `errors.Is` and `errors.As`** (ruling N, #91 comment 6080973822).
- **`ErrNotConnected` is transient** by `errs.IsTransient` and `errs.Classify`, because it wraps
  `errs.ErrConnectionLost` (T7).
- ~~**`natsclient.ErrCircuitOpen` is `errs.ErrCircuitOpen`** (T7).~~ Removed with the circuit breaker (#222).
- **No responders** comes back as nats.go's `nats.ErrNoResponders`, matched by `errors.Is` or `IsNoResponders`. On the
  default server configuration it comes at once (P15). Not promised:
  - its class, which waits for #93's handling of raw nats.go errors (amended 2026-10-15, #167; see Status);
  - other server configurations.
- **`CreateKeyValueBucket`** treats `jetstream.ErrBucketExists` as "the bucket exists" (T6).
- ~~**A stream at its byte or message limit** does not count against the connection circuit
  (`stream_capacity_circuit_integration_test.go:14`).~~ Struck with the circuit breaker (#222). nats-server names the
  limit only in the description of error 10077, so natsclient reads that text; nats.go has no code for it.
- **Not promised here:** `Classify`'s default for unknown errors (#167; amended 2026-10-15, see Status), and the raw
  nats.go errors that #93's `pkg/errs` rewrite reclassifies. Both wait for #93.

### 4. Ownership

- **A subscription or consumer the client created is the client's.** `Close` stops it and joins its handlers. ~~That
  includes one on a connection that `SetConnection` replaced. The replaced connection stays the caller's, and `Close`
  does not cut its socket (tests `client_close_final_test.go:1399`, `:1442`).~~ Struck 2026-10-12 (#184; see Status):
  the sentence before rests on tests `client_close_final_test.go:227` and `:654`.
- **Once `Close` begins, new work is refused** (test `client_close_final_test.go:960`). A `Subscribe` that `Close`
  overtakes returns `nats.ErrConnectionClosed` (T2; `TestSubscribeEndedByCloseIsConnectionClosed`,
  `client_close_held_test.go`).
- **natsclient holds no lock of its own while it calls nats.go**, so a held call cannot hold `Close` (T2, P21).

### 5. Reconnect

- **natsclient keeps nats.go's automatic reconnect,** with natsclient's settings: unlimited attempts, and
  `ReconnectWait` of 2 s (`client.go:355-356`). A reconnect after `Connect`'s context has ended works (T5).
- **While reconnecting,** `Publish`, `RequestClassified`, `Subscribe`, `PublishToStream` and `GetStream`
  return `ErrNotConnected` at once (P15). KV calls wait for their context.
- **Not promised:**
  - reconnect pacing;
  - nats.go's reconnect buffer;
  - consumers across a reconnect;
  - redelivery after a broker restart;
  - failover between servers.

### 6. Precedence

Where this record and other text disagree, this record holds. The `natsclient-contract` change brings
`transport-client`, the doc comments and the README into line with it.

## Alternatives

Each alternative that needed a timeout was measured with a short one, so no probe waited a minute.

- **A shorter default write timeout.**
  - It bounds every held call: 1 s ended `Publish` at 0.6 s (P19).
  - On a healthy slow link it fails writes that a minute would complete (P19, P21).
  - A write it ends partway leaves the connection unusable until the stale-connection check fires (P22).

  The owner kept the default and asked for an option instead.
- **One option for both settings.** Two durations of one type invite swapped arguments. A short ping is also useful
  alone.
- **`ReconnectOnFlusherError`.** It acts only when nats.go's background flusher fails. It healed 0 of 3 (P22) and lost
  accepted publishes on reconnect (P19).
- **Ending the reconnect dial some other way.**
  - `DialContext` needs a context that nats.go's dialer interface does not pass.
  - Keeping one fails `TestNoRetainedContext`.
  - `context.Background()` in the dialer invents a root inside a callback.
  - Leaving the dial alone holds `Close` up to `WithTimeout` per dial, 5 s by default (P23: 4.69 s).

  So the dialer uses `net.Dialer.Cancel`. It is deprecated in favour of `DialContext`, but still honoured (P23, P25,
  go1.26.9 on darwin; Go's own `TestDialCancel`). T4 and T8 use a stand-in, so no test here would catch a change;
  P23 is re-run after a Go toolchain upgrade.
- **Closing the socket when any call's context ends.** One caller's deadline would end the connection for all.
- **Returning early and leaving the call running.** That is unowned background work.
- **Promising nothing and changing no code** (revision 1). The owner declined it on 2026-10-11.

## Consequences

- **A component's `Stop` that closes its client is bounded by its context.** Its other calls are not, while a server
  is not reading. A caller that needs a shorter bound sets `WithWriteTimeout` and `WithPingInterval` together.
- **A retry keyed on `errs.IsTransient` now retries `ErrNotConnected`.** (It also
  retried a circuit refusal; struck with the breaker, #222.) No responders is unchanged. A KV write's error is
  commit-unknown, so re-read before retrying a write that is not compare-and-set.
- **Re-check after a nats.go or nats-server upgrade.** T1 to T9 run under `task verify` and pin the library facts this
  record rests on. A probe line is re-run before an upgrade merges. #118 (nats-server v2.15.0) merged on 2026-10-11
  at 14:37Z, while this record was in review, and the re-run after it found no change (comment 6111119623). After a
  Go toolchain upgrade, P23 is re-run (the dialer's `Cancel`).
- **Covered elsewhere:**
  - ADR-110:192 (a watcher sees every revision) is for #103, by ADR-109's ruling (#9 comment 6111608867); it was for
    ADR-109 (#110) when this record was drafted.
  - ADR-111's redelivery after a restart is for the change that relies on it.

## Related

Issues #170, #168, #169, #167, #146, #118 · #9 comment 6098388541 · #170 comments 6109833136, 6110309758 · PR #156 ·
ADR-110, ADR-111 · SemStreams at `8b99efe9`: ADR-060 (the wire error contract), ADR-068 (no lifecycle retention)
