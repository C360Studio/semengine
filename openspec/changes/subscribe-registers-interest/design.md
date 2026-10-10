# Design: subscribe-registers-interest

Status: revision 5, draft, after Codex's implementation review; revision 4 was accepted by the owner (PR #156 comment
6091034659). Review rounds 1 and 2 asked for changes (round 2: one MEDIUM, on D4's wait for running handlers); revision
3 answered each finding, passed review and was accepted by the owner (PR #156 comment 6085722854). Revision 4 changes
D7's test plan, adds L6 and L7, and adds one rule, in `docs/testing.md` and `AGENTS.md` (O1), after task 2.1 found that
T2 to T8 cannot share a test process (PR #156 comment 6086018979). The owner answered both of its questions ("Owner
decisions"). It rests on `inventory.md` revision 2, which passed inventory review. Issue #144 (`class:flake`); owner
ruling 2026-10-09 (fix it once, in natsclient's subscribe, as its own pull request on `main`); claim PR #156. Pins are
at base `805ace8`, revision 4's at `c4b61ec`, and revision 5's at `a85947d`; probes P1 to P5 are in `inventory.md`.
Revision 5 answers Codex's implementation review (PR #156 comment 6097783327). D2's bound is narrowed to the wait for
the server's PONG, and the waits for nats.go's connection lock are declared as L8 (owner question O3); the guarantee is
narrowed to a SUB the server has read, which it may have refused, declared as L9 (O4). Revision 5's probes, P6 to P9,
are posted with their sources and outputs on this pull request (comments 6098409783 and 6098410018; task 1.6).

## Context

The NATS protocol adds a subscription with a SUB line and removes it with an UNSUB line; a PING line is answered by
the server with a PONG. `Client.Subscribe` and `Client.SubscribeForRequests` return as soon as nats.go has put the SUB
in its write buffer (`natsclient/client.go:1267`; nats.go `nats.go:5170-5173`). The owner calls the moment between
that return and the server reading the SUB **window 1**. A request sent from another connection in window 1 gets "no
responders"; a publish sent then is dropped without an error. P1: 831 of 2,000 requests from a second connection,
sent right after the subscribe, found no responder at `-cpu=1`; 0 of 2,000 did after a round trip. A **round trip**
means a PING written on the connection and its PONG read back; the server handles one connection's lines in order, so
the PONG proves it has read the SUB. It does not prove the server accepted it: a SUB refused for permissions or for the
connection's limit on subscriptions gets a `-ERR` line, sent before the PONG, and the PONG still comes (P7; L9).

This change closes window 1 for every core subscription natsclient makes. It does not say when a component may rely
on another component's responder (not started yet, after a reconnect, across a cluster): the pin's ADR-083 and
ADR-088 answer that with readiness each producer publishes and each consumer folds, and ADR-088:23 says "`Start`
returning is not that signal". No requester-side retry is added.

## Options

1. **Do nothing in natsclient; each caller waits.** graph-ingest's `Start` makes the round trip, through
   `GetConnection().FlushWithContext` or through a new `Client.Flush(ctx)` (PR #93, D20 and task 3.12l). Cost: every
   developer who calls `Subscribe` must know a fact about nats.go's buffering and make the call; one who does not
   loses messages silently or answers "no responders" (`inventory.md`, "Adopter seam inventory": what a developer
   calling these methods must know, and what happens if they do not). PR #93's version adds one exported method.
   The owner ruled against keeping it per caller.
2. **The round trip inside natsclient's core subscribe** (recommended; the ruling). Every `Subscribe` and
   `SubscribeForRequests` makes one round trip before it returns. Cost: one round trip per call (D6); a subscribe can
   now fail for a reason it could not before (D4), and waits at most `DefaultRequestTimeout` for a server that reads
   but does not answer; a server that stops reading can hold it longer, as it holds every call (L8).
3. **Only `SubscribeForRequests`.** Covers "no responders" but leaves `Subscribe`'s window, where the loss is
   silent: a message published from another connection in the window is dropped with no error. Rejected for that
   reason.
4. **Use nats.go's `Conn.RTT` or `Conn.Flush`.** Both wait a fixed 10 s whatever the caller's context says
   (`nats.go:6066`, `:6074-6076`), and `RTT` refuses while reconnecting (`nats.go:6062-6064`). Rejected: a cancelled
   caller would wait up to 10 s.
5. **Fix the test only** (#144's option 1): the test flushes graph-ingest's connection before it asks. Rejected by
   the owner: a `Subscribe` that returned would still not be reachable.

## Decisions

### D1. Where the round trip goes

In `subscribeOwned` (`client.go:1298`), after the native subscribe returns without error and before the handle is
returned. Both exported calls reach the native subscribe only through it (`client.go:1277`, `request.go:372`;
inventory category 1), so every core subscribe passes through the round trip once, and a future core subscribe that
goes through `subscribeOwned` gets it too. The round trip runs on the connection the subscription was made on (the
one read under the lock at `client.go:1306`), never on whatever connection the client holds later: a round trip on a
connection that `SetConnection` installed meanwhile proves nothing about this SUB. It runs on the caller's goroutine;
no goroutine is added. The bound of D2 is a context derived from the call's own context and cancelled before the call
returns; nothing keeps it, and no root context is created (architect contract, "Design discipline"). The nil-context
refusals stay where they are, at the two exported calls (`client.go:1255-1257`, `request.go:369-371`).

### D2. The bound: the caller's context or 5 s, whichever ends first

The wait for the server's PONG ends when the server answers, when the call's context ends, or when
`DefaultRequestTimeout` (`request.go:18`, 5 s) has passed since the round trip began, whichever comes first. The bound
is for that wait only: the waits for nats.go's connection lock are L8's (last bullet below). This follows the
repository's own shape for a bounded wait inside a call, `stream.go:344-345`: "bounded by the caller's context AND the
budget, whichever ends first". PR #93's D20 rule ("the caller's deadline when there is one, otherwise 5 s") was a
mechanism note on #144, not an owner ruling, and this change replaces it.

- A bound is needed because the context a caller passes to `Subscribe` is the parent of every handler's context
  (`client.go:1280`, `request.go:387`, `:396`), so callers pass a lifecycle context, which usually has no deadline,
  and nats.go refuses a round trip under a context with no deadline (`context.go:181-184`; P2).
- 5 s, through `DefaultRequestTimeout`, because it is natsclient's existing answer to "how long may one round trip to
  the server take when the caller named no limit" (`request.go:204`, `:263`, `:541`), and `stream.go:304` uses the
  same value. A healthy round trip took 21 µs at the median and 179 µs at most on loopback (P5). nats.go's own 10 s
  (`nats.go:6074-6076`) would double the wait of a boot that will fail anyway. No new name and no setting.
- A deadline on `Subscribe`'s context is a deadline for its handlers; "whichever ends first" keeps a long one from
  holding a boot longer than `DefaultRequestTimeout` on a server that reads but does not answer. A server that stops
  reading can hold the call longer (next bullet).
- The bound does not cover waits for nats.go's connection lock. nats.go takes one lock for every write on a
  connection, and each step of the call takes it without looking at the context: the connection check
  (`client.go:1340`; `nats.go:6286`), made while natsclient's own lock is held (`client.go:1328`; read, not probed);
  the native subscribe that writes the SUB (`nats.go:5097`); the PING (`context.go:186-195`, written in place by
  `sendPing`, `nats.go:5983-5988`); and, on a call that fails, clearing the pending PONG once the context has ended
  (`removeFlushEntry`, `context.go:211`, `nats.go:5968`) and the UNSUB (`client.go:1379`; `nats.go:5512`). While the
  server is not reading and the send buffer is full, a write on that connection (the call's own SUB or PING, or
  another goroutine's) holds that lock until the held write completes or nats.go's write timeout (`FlusherTimeout`,
  one minute by default, `nats.go:68`) ends it, and the call waits for each held write ahead of it in turn. P6 saw
  writes last to the timeout after the server read again (still held 1.45 s past a 50 ms context, returned at 59.7 s
  and once at 1m4.9s; with a 1 s timeout, two timeouts back to back); P9: held after the server had read the PING,
  returned at the write timeout. `Close`, `Request`, `Publish` and `Subscribe` are held the same way on `main` (P6),
  which breaks `Close`'s accepted bound (#168). This change sets no write timeout: it would apply to every write and
  drop a timed-out write's bytes while the connection reports connected (L8; owner question O3).

### D3. A context that has already ended returns its error without acting

When the context has already ended on a connected client that Close has not begun, the call returns a transient
error matching the context's error and sends no SUB. With both a PONG and an ended context ready, Go's `select` picks
one at random (`context.go:199-208`), so without this check the result would depend on luck (P2 saw the context win
20 of 20 times, which does not make it certain); and a subscription whose handler parent has ended would hand every
handler an ended context. The nil check stays first (spec, "An exported call refuses a nil context before it acts"),
and the Close check stays before this one (spec, "Close refuses new work").

### D4. A failed round trip fails closed and does not wait for handlers

When the call returns no subscription after the native subscribe (the round trip did not complete, or Close had
begun), it first ends the subscription, then returns without waiting for a running handler (ending the subscription
takes nats.go's connection lock, so it can wait behind a held write, L8):

- it unsubscribes it, so the subscription leaves the client's set of subscriptions and nats.go does not restore it on
  a reconnect (P2), and on a connected client its UNSUB is queued to the server (`nats.go:5544-5552`); the server
  drops the subscription when it reads that line (P3: the scripted server read `UNSUB 1`), which may be after the call
  has returned. When Close is draining or has closed the connection, nats.go refuses the unsubscribe
  (`nats.go:5400-5407`) and Close's drain or the close ends the subscription instead, as `watchOwnedSubscription`
  already assumes (`client.go:1328-1338`).
- it does not wait for a handler invocation of the subscription that is still running. The subscription stays owned
  by the client until its delivery has ended, so Close joins that invocation, and a nil Close still means every
  handler returned (spec, "A nil Close means everything the client owns has finished").

**How this relates to the Consume rule, and why core differs.** natsclient has a rule for a consumer setup whose
delivery has started when Close begins: `commitConsumer` (`natsclient/owned_delivery.go:189-209`) "returns
nats.ErrConnectionClosed with no handle once every handler invocation has returned, or ctx's error first if ctx
ends", and `failed` (`:182-187`) returns once the claim is released; the spec states it as "A consumer refused during
Close keeps its claim" (`openspec/specs/transport-client/spec.md:149-154`). A core subscription does not wait, for
three reasons:

1. **No claim to release.** The consumer waits so that its claim is held until its handlers return, and so that "a
   caller that retries does not meet its own claim" (`owned_delivery.go:182-183`). A core subscription holds no claim;
   a retried `Subscribe` on the same subject meets nothing.
2. **The join is Close's.** A `Start` that fails here has usually made other subscriptions before this one, and their
   handlers are still running while it tears down; a wait inside this one call would protect it from one
   subscription's handlers only. Close joins them all, as it already does for every subscription of a failed `Start`.
3. **A wait could block without limit.** With a context that has no deadline, which is the normal case (D2), a wait
   for a handler that never returns never ends: for example a handler waiting on a lock its caller holds across
   `Subscribe`, a pattern that was safe before this change. Not waiting keeps a handler from holding the call: the
   call takes D2's bound plus the lock waits L8 declares, and no handler adds to that.

The core subscription's handlers also differ in their context: it descends from the call's context (`client.go:1280`,
`request.go:396`), where a consumer's handlers do not descend from its setup context (spec, "A consumer's setup
context does not parent its handlers"). A caller that wants them stopped cancels its own context.

**The error.** Decided by Close having begun, never by `Status()`, as the spec decides Close's refusal ("Close refuses
new work", `spec.md:134-135`), and classified the way `Connect` classifies its own (`client.go:745`, `:752`). The rows
are checked in this order, and the first that holds decides:

| Case | Returned error |
| --- | --- |
| Close has begun when the round trip ends, whatever the round trip's outcome and whether or not the call's context has ended | `nats.ErrConnectionClosed`, wrapped as at `client.go:1304` |
| the call's context ended | transient (`errs.WrapTransient`), matching `context.Canceled` or `context.DeadlineExceeded` |
| `DefaultRequestTimeout` passed | transient, matching `context.DeadlineExceeded` |
| nats.go ended the round trip with `nats.ErrConnectionClosed` (the connection was lost, P4, or closed by anything but this client's Close) | transient, matching `ErrNotConnected` (`client.go:53`) and not `nats.ErrConnectionClosed`: the error `Subscribe` already returns when called while not connected (`client.go:1307-1309`) |

The Close check comes first because Close's drain timeout or its context ending makes it close the connection
(`client.go:1136-1138`, `:1143-1145`), which ends a pending round trip with nats.go's `ErrConnectionClosed`
(`nats.go:6154`, `:6174`), the same error a lost connection gives. `nats.ErrConnectionClosed` keeps the one meaning
the spec gives it for these calls, "Close has begun", so a caller that tests for it to recognise shutdown does not
misread a reconnect. The transient class is set explicitly, not left to `errs.IsTransient`'s text match, which PR #93
task 3.12h removes. When Close begins during the round trip, the call returns Close's refusal even if the server
answers: the connection is draining and nats.go drains this subscription with it.

### D5. While reconnecting

Measured in nats.go v1.54.0 (inventory, "nats.go behaviour by connection state"):

- A call made while the client is not connected is refused before any subscribe, with `ErrNotConnected`, as today
  (`client.go:1307-1309`). Unchanged.
- A connection lost while the round trip waits ends the round trip at once with nats.go's `ErrConnectionClosed` (P4);
  unless Close has begun, the call reports `ErrNotConnected` (D4) and removes the subscription, which nats.go then
  does not resend (P2). natsclient does not wait for the reconnect: the caller decides whether to subscribe again.
- A subscription that was registered and later lives through a reconnect is resent by nats.go (`nats.go:3552`,
  `:6091`). Whether it is registered again before some other component asks is the out-of-scope window (L2).

### D6. The cost

One round trip per `Subscribe` or `SubscribeForRequests` call: 21 µs median, 96 µs p99 on loopback (P5); one network
round trip elsewhere. Nothing in the tree calls either outside natsclient's tests (inventory category 1). At the pin
every production call runs at start-up, once per configured port or declared verb, never per request or per message
(inventory Q1). semsource and semconnect are not read for this: `docs/inventory-scope.md` does not admit a
symbol-level read of them for this question, and the cost is at most one round trip per call (L5). Requests
(`Request`, `RequestClassified` and the others) do not subscribe through `subscribeOwned`, and the Consume methods and
KV watchers are not core subscriptions natsclient makes, so none of them pays.

### D7. What a caller can observe, and the test that proves it

All are unit tests in `natsclient`, so `task test:unit` (race detector, one CPU) and `task test:repeat` (five runs,
one CPU, shuffled) run them, and `task mutate:check` (one CPU, race detector) can check them; #129 limits it for
integration tests only. `task mutate:check` matches a failure line that T2 to T7 print from their own process (below)
like any other: in design round 4, M2 to M10 were each a detection against a sketch of D1 to D4.

- T1 uses the embedded server natsclient's unit tests already start (`client_lifecycle_test.go:108`), on an ephemeral
  port. It can fail only at one CPU: P1 missed 831 and 800 of 2,000 at `-cpu=1` (without and with `-race`), and #144's
  test failed 0 of 500 at the default CPU count. Its comment says so. On the base code natsclient missed 104 to 124 of
  200 per call at one CPU (task 2.1, PR #156 comment 6086018979).
- T2 to T7 use a scripted server on `net.Pipe` inside a `testing/synctest` bubble (goroutines on a fake clock that
  moves only when all of them are blocked): it answers the handshake, records every line the client sends, delivers a
  message when told, and answers or withholds each later PONG as the test says. The client reaches it through
  `connectWith`'s dial seam (`client.go:699-701`), with nats.go's `SetCustomDialer` and `SkipHostLookup` options and a
  URL that names no port; a connection given through `SetConnection` carries none of the client's handlers, so it is
  not used (task 2.1). The pipe has no address, so "Tests bind no fixed address or port" holds, and a withheld PONG
  costs no wall time. No test sleeps.
- Each of T2 to T7 runs in a test process of its own: the test runs the test binary again for itself alone and, when
  that run fails, reports its output. nats.go v1.54.0 keeps one pool of timers for the whole process (`timer.go:22`;
  `FlushTimeout`, `nats.go:6030`, which Close's drain calls at `:5430` and `:6384`). A timer made inside a bubble goes
  back to that pool, and when code outside the bubble, or in another bubble, takes it, the Go runtime ends the process
  (`fatal error: reset of synctest timer from outside bubble`), or a bubble waits on a timer its clock does not drive
  and the run hangs. Any test that flushes, requests or drains can be that code, in whatever order `task test:repeat`
  shuffles. Round 4's probes: with two `runtime.GC()` calls around each bubble instead, and T8 on real time, 5 of 40
  runs of the package ended with that `fatal error`, and 4 of the reviewer's 20 ended or hung, 2 of them hung in T7's
  bubble; with a process of its own, none of 122, 112 of them of the whole package (L6). natsclient's other bubbles
  run no nats.go connection, so they share the process as before. The run of its own:
  - is started only for a top-level test: called from a subtest (a name with `/`), the helper fails the test before
    any run starts, because `-test.run` splits its pattern at each slash and a repeated subtest name gains a `#01`
    suffix, so such a run could select nothing;
  - passes only when it exits 0 and prints the test's `--- PASS: <name> (` line; a run that selects no test exits 0
    without that line, and the test fails;
  - keeps the parent's CPU count, so `-cpu 1` still applies;
  - gets nine tenths of the parent's remaining time limit, so a run that hangs ends first, and its own timeout panic
    and goroutine dump appear in the test's failure message; with no time left the test fails without starting it;
  - writes its coverage where `go test` collects the parent's (`-test.gocoverdir`), so `task cover:check` and
    `task mutate:check`'s reach run count what it runs; without that, T2's lines read as never run;
  - turns off the race detector's one-second sleep at exit (`GORACE` option `atexit_sleep_ms=0`), so a run of its own
    costs about 10 ms, not one second.
- T8 uses the same scripted server on real time, outside a bubble. When Close closes the connection itself, nats.go's
  drain goroutine runs for up to 5 s more and nothing signals its end (L7); a bubble whose test returns while that
  goroutine runs panics (`deadlock: main bubble goroutine has exited but blocked goroutines remain`). On real time it
  ends on its own, with an ordinary timer. T8 begins Close once the server has read the round trip's PING, or reports
  the call's result if the call returns first (on the base code no PING comes), and Close's context has already ended,
  so nothing in T8 waits for time to pass. T8 rests on one wall-clock limit: less than the call's own bound
  (`DefaultRequestTimeout`, 5 s) passes between the server's read of the PING and Close closing the connection, a
  window in which the test cancels a context and calls Close, and Close closes the connection as soon as it sees the
  ended context (`client.go:1141-1145`). If a stalled machine lets 5 s pass, the call returns the bound's error first,
  and T8 fails at its `nats.ErrConnectionClosed` assertion, printing the error it got; it cannot pass wrongly.

| Test | Observable |
| --- | --- |
| T1 | For each of `Subscribe` (a handler that replies) and `SubscribeForRequests`: right after the call returns nil, a request from a second, raw nats.go connection to the same server is answered, never "no responders". 200 times per call, each on a new subject. |
| T2 | With a context that has no deadline and the PONG withheld: the call returns when exactly `DefaultRequestTimeout` has passed on the bubble's clock, with no subscription and a transient error matching `context.DeadlineExceeded`; the server then reads `UNSUB` for the subscription's sid. |
| T3 | With the PONG withheld, the caller cancels the context: the call returns with no bubble time passed, with no subscription and a transient error matching `context.Canceled`; the server reads `UNSUB`. |
| T4 | With a context that has already ended, on a connected client: a transient error matching `context.Canceled`, and the server reads no `SUB`. |
| T5 | With the PONG withheld, the server closes its end of the pipe: the call returns a transient error that matches `ErrNotConnected` and does not match `nats.ErrConnectionClosed`; the connection's subscription count (`Conn.NumSubscriptions`) is back to its value before the call. |
| T6 | The server delivers a message after the `SUB` and withholds the PONG; the handler starts and is held; `DefaultRequestTimeout` passes: the call returns a transient error matching `context.DeadlineExceeded` while the handler is still held, and a `Close` with a live context returns nil only after the handler has been released and returned. |
| T7 | As T6 up to the held handler; then `Close` begins with a live context and the server answers every PING: the call returns an error matching `nats.ErrConnectionClosed` and no subscription, although its round trip completed, while the handler is still held; `Close` returns nil only after the handler has been released and returned. |
| T8 | On real time. With the PONG withheld and the server's read of the PING seen, `Close` begins with a context that has already ended, so Close closes the connection at once, which ends the round trip with nats.go's `ErrConnectionClosed`: the call returns an error matching `nats.ErrConnectionClosed` and not `ErrNotConnected`, and no subscription. |

Each is written first and run on the base code, where every one fails (T1 by "no responders", the others because the
call returns a subscription and no error; T4 also because the server reads a `SUB`). The reproduction the protocol
asks of a flake fix ("Known flakes") is task 2.5: the command recorded on issue #144, run against PR #93's head before
and with this change.

Wrong changes, one at a time, with `task mutate:check`; each must be a detection by the named assertion:

| Mutant | Change | Expected to fail |
| --- | --- | --- |
| M1 | the round trip removed | T1's "answered" assertion (P1: about 40% of iterations miss at `-cpu=1`) |
| M2 | `DefaultRequestTimeout` in the bound replaced by `50 * DefaultRequestTimeout` | T2's assertion that the bubble's elapsed time equals `DefaultRequestTimeout` (round 4 measured 90 s: with every PONG withheld, nats.go ends the connection after two unanswered pings 30 s apart, `client.go:346` and nats.go's `DefaultMaxPingOut`, `nats.go:62`) |
| M3 | on a failed round trip, the subscription is left subscribed | T2's and T3's `UNSUB` assertion; T5's count |
| M4 | on a failed round trip, the client stops owning the subscription before its delivery ends | T6's assertion that `Close` has not returned nil while the handler is held |
| M5 | the ended-context check removed | T4's "no `SUB`" assertion |
| M6 | nats.go's `ErrConnectionClosed` passed through when Close has not begun | T5's "does not match `nats.ErrConnectionClosed`" assertion |
| M7 | Close beginning during the round trip not checked after it ends | T7's error match (a handle is returned) |
| M8 | the Close check made after the mapping of nats.go's `ErrConnectionClosed` to `ErrNotConnected` | T8's error match |
| M9 | a failed round trip waits for a running handler before it returns | T6's assertion that the call returned while the handler is held |
| M10 | a call refused because Close began waits for a running handler before it returns (Consume's rule) | T7's assertion that the call returned while the handler is held |

### D8. Tests that flush after a natsclient subscribe stay as they are

Fifteen test sites flush the raw connection after `Subscribe` or `SubscribeForRequests` (inventory category 2). After
this change each flush is a no-op. They stay: each is one line in a test whose subject is something else, removing
them changes no behaviour, and `nil_context_siblings_test.go` is also edited by PR #93. T1 carries the guarantee.

### Generated checks

The outcome depends on the order of events (a PONG, the context ending, the connection dropping, Close beginning, a
delivery arriving), so `docs/testing.md`, "Decide whether generated checks are needed", applies. Examples suffice:
each ending is a separate branch, and T2 to T8 reach each one deterministically through the scripted server, which
fixes the order. The orderings that matter combine one ending with "a handler is running", and every ending takes
the same end-the-subscription-and-return path, which T6 (the bound) and T7 (Close) check with a handler held, with
M4, M9 and M10 showing each assertion can fail. The success path's race against another connection is the one
ordering the test cannot fix, and T1 repeats it 200 times per call under every unit run. A generated check over
event orders would explore the same few endings.

## Invariants and their spec homes

| Invariant | Spec home |
| --- | --- |
| I1. A call that returned a subscription returned it after the server read its SUB; the server may have refused it (L9). | delta, "Subscribe returns after the server has read the SUB", first sentence; scenario "A request from another connection right after the call", on a subject the server permits |
| I2. A call that returned an error after the native subscribe has removed the subscription from the connection's set, and on a connected client has queued its UNSUB; the server drops it when it reads that line. | delta, same requirement; scenarios "The server does not answer", "The caller's context ends during the round trip", "The connection is lost during the round trip" |
| I3. The call's wait for the server's PONG ends by the time its context ends or `DefaultRequestTimeout` has passed, whichever is first; the call never waits for a handler. Its waits for nats.go's connection lock are bounded by neither (L8). | delta, same requirement; scenarios "The server does not answer", "The caller's context ends during the round trip", "A handler is running when the call fails" |
| I4. A handler still running when a failed call returns is joined by Close; a nil Close means every handler returned. | delta, same requirement; scenarios "A handler is running when the call fails", "Close begins during the round trip"; existing "A nil Close means everything the client owns has finished" |
| I5. A call whose round trip ends after Close began returns `nats.ErrConnectionClosed`, decided by Close having begun. | delta, same requirement; scenarios "Close begins during the round trip", "Close ends the connection during the round trip"; existing "Close refuses new work" |

## Declared limits

- L1. One server. In a cluster the server the client is connected to passes interest to the others on its own
  schedule; its PONG says nothing about them. Out of scope by the ruling.
- L2. After a reconnect nats.go resends every subscription, and no natsclient call waits for that (D5). Out of scope:
  ADR-083 and ADR-088.
- L3. A responder in another component that has not subscribed yet. Out of scope: readiness (ADR-083, ADR-088);
  `RequestReadyClassified` is unchanged.
- L4. A handler may still be running, for a message that arrived during a round trip that then failed, when the call
  returns its error; Close joins it (D4).
- L5. Whether semsource or semconnect subscribe per request is not measured (D6).
- L6. nats.go v1.54.0 keeps one pool of timers for the whole process (`timer.go:22`), so a `synctest` bubble that runs
  a nats.go connection cannot share a test process with other tests; T2 to T7 each run in a process of their own
  (D7). Out of scope: upstream; nats.go's newest release and its main branch both keep the pool.
- L7. When Close closes the connection itself (its context ended, or the drain timed out), nats.go's drain goroutine
  runs for up to 5 s more (`nats.go:6355`, `:6380-6390`) and Close does not wait for it; nothing signals its end.
  Unchanged by this change, and the reason T8 runs on real time (D7). Out of scope: upstream.
- L8. The call's context and `DefaultRequestTimeout` bound the wait for the server's PONG, not the waits for nats.go's
  connection lock (D2, last bullet): checking the connection, writing the SUB and the PING, and on a failed call
  clearing the pending PONG and writing the UNSUB. While the server is not reading and the connection's send buffer is
  full, a write on the connection, the call's own SUB or PING or another goroutine's, holds that lock until it
  completes or nats.go's write timeout (`FlusherTimeout`, one minute by default) ends it, and the call waits for each
  such write ahead of it. In P6 every such call was still held 1.45 s past its 50 ms deadline; after the server began
  reading again, some returned 3.3 s to 3.5 s later and others only once the write timeout had ended the held write
  (at 59.7 s, once at 1m4.9s), for a reason not explained. Every natsclient call that writes on the connection shares
  this limit, and on `main` it breaks `Close`'s accepted bound (`openspec/specs/transport-client/spec.md:7`, `:51-65`,
  `:89`). Out of scope: the connection-wide fix, #168 (owner question O3).
- L9. A SUB the server refuses (for permissions, or for the connection's limit on subscriptions) is not reported by the
  call: it returns the subscription and nil, and the server holds no subscription for it. nats.go v1.54.0 records the
  refusal only as the connection's last error, which another error can replace before the PONG (P8: 433 of 800 missed
  with 16 subscribers at once), and a limit refusal names no subject. On a client that connected with `Connect`,
  natsclient's error handler (`handleError`) logs the refusal at error level (`NATS error`, the error naming the
  subject); a limit refusal's record does not say which subscription it was, and no metric counts refusals. That log
  line is `main`'s behaviour and is not shown by a test in this change (#169). A connection given through
  `SetConnection` reports a refusal only to the error handler its caller gave it. Out of scope: making a refusal an
  error, and a metric for it, #169 (owner question O4).

## Files

- `natsclient/client.go` (`subscribeOwned`, its doc comment and the doc comment of `Subscribe`),
  `natsclient/request.go` (the doc comment of `SubscribeForRequests`).
- New tests in `natsclient` (T1 to T8) and the unexported helper that runs each of T2 to T7 in a test process of its
  own, in a new `_test.go` file; no integration file.
- `docs/testing.md`, "How reach is judged" (`:295-296`): the sentence that says a child process the test starts
  writes no coverage, since D7's helper gives its run the parent's coverage directory (and a `prochost` helper
  writes it through the `GOCOVERDIR` it inherits).
- The rule of O1: one bullet in `docs/testing.md`, "Concurrency and cleanup", after `:471`, and its row in
  `AGENTS.md`'s rule table, after `:81`.
- `docs/admission-ledger.yaml`, natsclient's row: adapt item `(13) natsclient-subscribe-registers-interest` at the end
  of `contract`, and T1 to T8 with the mutation record at the end of `proving_tests`.
- At the archive: `openspec/specs/transport-client/spec.md` (the new requirement and one clause in the Purpose),
  `natsclient/README.md` (one sentence beside the Subscribe example, `:85-86`), `docs/repository-map.md` (a row for
  the archived change).

## Not in this change

When a component may rely on another's responder; anything after a reconnect or across a cluster; a requester-side
retry; a new exported method (PR #93's `Client.Flush` is not needed); the fifteen test flushes (D8); anything in
SemStreams; the ADR-094 observation in the inventory; a bound on waits for nats.go's connection lock, which every call
shares (L8, #168); reporting a refused SUB as an error (L9).

## Overlaps and order

- **This change merges first.** It closes #144, the open `class:flake` issue, so `merge-check` stops failing every
  other pull request (`.agents/protocol.md:129-131`).
- **PR #93** then merges `origin/main` and: (a) drops task 3.12l's `natsclient.Client.Flush(ctx)` and the round trip in
  graph-ingest's `Start`, and rewrites D20's round-trip sentences to cite this requirement; (b) resolves the textual
  conflict in natsclient's ledger row, where both add at the end of `contract` and of `proving_tests`, by keeping
  both; (c) settles its own `graph-transport-boundary` requirement "Routable when Start returns" with the #91
  re-check, since this change provides natsclient's half only; (d) runs the #144 command on the merged tree. Both
  change `natsclient/README.md`: #93 adds lines at `:248` and `:270`, this change one sentence at `:85-86` in the
  archive commit; no textual conflict. #93's task 3.12h changes `errs.IsTransient` to class and sentinel only; this
  change sets the class explicitly (D4), so it is unaffected.
- **#158** and **#166** (drafts) change `AGENTS.md`'s rule table, as this change does (O1): #158's hunk starts at
  `:96`, and #166 adds three rows at the end of the table. This change's row goes after `:81`, so no hunk is shared.
  This change merges first (it closes #144); each of them then merges `origin/main`. Their other files are not shared.
- **#118** (nats-server 2.15.0, the embedded server T1 runs on) cannot merge while #144 is open, so it merges after
  this change, and its CI runs T1 on the new server.
- **#116, #117, #119, #120**: no shared file; each also waits for #144 to close. #117's OpenSpec CLI then runs
  `task spec:check` on a tree that holds this change.

## Owner decisions

Revision 3 left none open. Round 1's four questions are decided above: the bound (D2), the ended context (D3), the
lost-connection error (D4), and no read of semsource or semconnect (D6, L5). D4 does not adopt the Consume rule's wait
for handlers and states why core subscriptions differ; the spec delta states the rule and its relation to Consume's.

Revision 4 asked two, answered by the owner at task 1.5 (PR #156 comment 6091034659):

- O1. Is "a test that runs a nats.go connection inside a `testing/synctest` bubble runs in a test process of its own" a
  rule for the whole repository, added by this pull request to `docs/testing.md` and `AGENTS.md`? Recommended: yes. The
  crash or hang it prevents shows only in some shuffle orders, the class #144 belongs to, and nothing else tells the
  next author. If no, the helper's doc comment is its only record. Answered: yes, in this pull request (comment
  6091034659).
- O2. Revision 4 replaces D7's test plan, which the owner accepted in revision 3. Does the owner accept revision 4,
  after its pre-owner review (task 1.4)? Answered: accepted at `a88b190` (comment 6091034659).

Revision 5 asks two (PR #156 comment 6097783327's findings):

- O3. Should #156 promise only that `Subscribe`'s context ends its wait for the server's answer, and leave the waits for
  nats.go's connection lock, which no natsclient call's context ends today, to #168? Recommended: yes. Why: every step
  of a subscribe (the connection check, the SUB, the PING, and on failure the cleanup and the UNSUB) takes nats.go's
  one connection lock without looking at the context, and a write held by a server that stopped reading, the call's
  own SUB or PING or another goroutine's, keeps that lock until it completes or nats.go's one-minute write timeout ends
  it; P6 saw writes last to the timeout after the server read again. `Close`, `Request`, `Publish` and `Subscribe` are
  held this way on `main` already (P6: still held 1.45 s past a 50 ms deadline, returned at 59.7 s and once at
  1m4.9s), so fixing `Subscribe` alone leaves the caller's next call, usually `Close`, held. And #156 closes the flake
  every other pull request waits on. If no: #156 must change how every connection writes, with a shorter write timeout
  that drops a timed-out write's bytes while the connection reports connected and still returns after the timeout, not
  at the context (1.7 s for a 50 ms context with 1 s set: two timeouts back to back); or run each subscribe in a
  goroutine `Close` joins, which reverses D1, which you accepted, and moves the wait into `Close`. Closing the socket
  ends a held write too, but only by ending the connection: that suits `Close`, not one `Subscribe`. Your cost: this
  answer now, and later a ruling on #168, which also covers `Close` breaking its accepted bound on `main`. Answered:
  yes (PR #156 comment 6098388350).
- O4. When the server refuses a subscription (permissions, or its limit on subscriptions), should #156 keep `main`'s
  behaviour, the subscription and nil returned and the refusal logged, and leave making it an error to a follow-up
  issue? Recommended: yes. Why: nats.go v1.54.0 reports a refusal only as the connection's last error, one slot any
  later error overwrites; a check of it caught 500 of 500 refusals one at a time but missed 433 of 800 with 16
  subscribers at once, and a limit refusal names no subject (P7, P8), so the check cannot be promised. Nothing gets
  worse than on `main`, and the spec's promise is narrowed to subjects the server permits. What yes leaves: a refusal
  gets one error-level log line and no metric, where the reviewer contract asks for both
  (`.agents/contracts/semengine-reviewer.md:296-299`); a limit refusal's line does not say which subscription; a
  `SetConnection` connection logs only through its caller's handler; a caller who reads no logs sees "no responders".
  If no: #156 adds the check now, about 25 lines that parse nats.go's error text, a new error row in D4, two tests, a
  declared limit for the misses, and another review round on the pull request every other one waits for. Your cost:
  this answer now, and later a ruling on the follow-up issue, filed only if you answer yes. Answered: yes (PR #156
  comment 6098388350); no T9 (the planned test of the refusal's log line) or new log requirement in this change. The
  follow-up issue is #169.

## Conformance

Filled at task 2.7: each decision mapped to the `file:line` that carries it out and to the test or record that shows
it. Lines are at `987a33f`, task 2.6's commit (CI run 38009721793 passed on it). `client.go`, `request.go` and the
test files are in `natsclient/`; a bare `:line` is in the file last named in the same cell. Comment ids are PR #156's;
`T1` to `T8` and `M1` to `M10` are D7's tests and wrong changes.

| Decision | Carried out at | Shown by |
| --- | --- | --- |
| D1 | `client.go:1360-1364`: the round trip (`conn.FlushWithContext`) in `subscribeOwned` (`:1324-1397`), after the native subscribe returns without error (`:1350-1356`) and before the handle is returned (`:1369`), on `conn`, read under the lock at `:1339`. `Subscribe` reaches it through `subscribeWith` (`:1282`, `:1301`), `SubscribeForRequests` at `request.go:396-398`. The nil-context refusals stay at `client.go:1279-1281` and `request.go:393-395`. The fix adds no `go` statement (`git diff e68da6d 987a33f -- natsclient`); the watcher started at `client.go:1358` was there before. | T1 (`TestSubscribeRegistrationAnswersAnotherConnection`), for both calls: on the base code `e68da6d` 134 and 110 of 200 requests got "no responders" at `subscribe_registration_test.go:287` (task 2.1, comment 6091149322). M1 (round trip removed) is a detection at `:287` (task 2.3, comment 6091577433). Not shown by a test: that the round trip runs on the subscribing connection and not on one `SetConnection` installed during the call; T1 to T8 do not call `SetConnection`. |
| D2 | `client.go:1362`: `context.WithTimeout(ctx, DefaultRequestTimeout)`, derived from the call's context and cancelled at `:1364` before the call returns; `DefaultRequestTimeout` is `request.go:18` (5 s). The bound's error is `client.go:1392-1395`. | T2 (`TestSubscribeRegistrationBoundedByDefaultRequestTimeout`): with a context that has no deadline, the call returns after exactly `DefaultRequestTimeout` on the bubble's clock (`subscribe_registration_test.go:306`), matching `context.DeadlineExceeded` (`:307`). T3 (`TestSubscribeRegistrationCancelledDuringRoundTrip`): the caller's context ending first returns with no bubble time passed (`:335`). M2 (`50 * DefaultRequestTimeout`) is a detection at `:306`: 5 s expected, 1m30s seen (comment 6091577433). |
| D3 | `client.go:1333-1338`: `ctx.Err()` checked under the lock, after the Close check (`:1329-1332`) and before the native subscribe (`:1350`); the nil checks stay first (`:1279-1281`, `request.go:393-395`). | T4 (`TestSubscribeRegistrationEndedContextSendsNoSub`): a transient error matching `context.Canceled` and no subscription (`subscribe_registration_test.go:353-356`), and the server reads no `SUB` (`:358`). M5 (the check removed) is a detection at `:358` (comment 6091577433; its report is in 6091577987). Not shown by T1 to T8: that the Close check comes before this one. The check also sits before the not-connected check (`client.go:1339-1343`), so an ended context on a client that is not connected returns the context's error; no test covers that case (task 2.2, comment 6091459854). |
| D4 | `client.go:1365-1370`: Close's state is read after the round trip, and the handle is returned only when the round trip succeeded and Close has not begun. `:1379`: the unsubscribe. `:1380-1396`: the error, in the order of D4's table: Close begun (`:1381-1386`), the call's context ended (`:1387-1388`), nats.go's `ErrConnectionClosed` reported as `ErrNotConnected` (`:1389-1391`), the bound (`:1392-1395`). The failed path waits for nothing; the watcher started at `:1358` (`watchOwnedSubscription`, `:1409-1425`) keeps the subscription owned until its delivery ends, so Close joins its handler. | T2 and T3: the server reads `UNSUB` (`subscribe_registration_test.go:312`, `:339`). T5: the subscription count is back to its value before the call (`:384`); the error matches `ErrNotConnected` and not `nats.ErrConnectionClosed` (`:381-382`). T6 (`TestSubscribeRegistrationReturnsWhileHandlerHeld`): the call returns while the handler is held (`:406`), and Close returns nil only after the handler has returned (`:413-427`). T7 (`TestSubscribeRegistrationCloseBeginsDuringRoundTrip`): `nats.ErrConnectionClosed` and no subscription although the round trip completed (`:454-457`); Close joins the handler (`:458-470`). T8 (`TestSubscribeRegistrationCloseEndsConnectionDuringRoundTrip`): `nats.ErrConnectionClosed`, not `ErrNotConnected`, no subscription (`:496-498`). Detections (comment 6091577433): M3 at `:312`, `:339` and `:384`; M4 at `:418`; M6 at `:381`; M7 at `:456`; M8 at `:496`; M9 at `:406`; M10 at `:454`. |
| D5 | Not connected when called: `client.go:1339-1343`, unchanged by the fix. Lost while the round trip waits: `:1389-1391` reports `ErrNotConnected` unless Close has begun, and `:1379` removes the subscription so nats.go does not resend it; nothing waits for a reconnect. A registered subscription resent after a reconnect is nats.go's (`nats.go:3552`, `:6091`): no natsclient line, outside this change (L2). | Not connected: `TestClientLifecycleOperationTable` (`client_close_final_test.go:960`, rows `:971-972`), an existing test, not one of T1 to T8. Lost: T5 (`TestSubscribeRegistrationConnectionLostDuringRoundTrip`, `subscribe_registration_test.go:380-384`), whose dialer refuses every later dial (`:160-172`); M6 at `:381` and M3 at `:384` are detections (comment 6091577433). That the call does not wait for a reconnect has no wrong change of its own. The resend after a reconnect: no test in this change (L2). |
| D7 (#144) | `subscribe_registration_test.go`: T1 `:237-292`, T2 `:294-314`, T3 `:316-341`, T4 `:343-360`, T5 `:362-386`, T6 `:388-429`, T7 `:431-472`, T8 `:474-500`. The scripted server `:33-158` and its dialer `:160-172`, reached through `connectWith`'s dial seam (`connectScripted`, `:174-197`; `client.go:699-701`). The helper `ownProcessBubble`, `subscribe_registration_test.go:502-549`: the subtest refusal `:517-521`, the CPU count `:524-525`, `-test.gocoverdir` `:526-528`, nine tenths of the time limit `:529-538`, `GORACE` `:540-541`, the `--- PASS: <name> (` line `:546-548`. | On the base code `e68da6d` all eight fail, with no panic or hang (task 2.1, comment 6091149322). At `570e8ae`, 20 runs of the package (`go test -race -cpu=1 -count=5 -shuffle=on -v ./natsclient/`) all pass, each seed recorded (task 2.2, comment 6091459854). M1 to M10 are each a detection, and the helper's two refusals fail as written, R1 at `subscribe_registration_test.go:520` and R2 at `:547` (task 2.3, comments 6091577433 and 6091577987). The reproduction: the command recorded on #144 ran PR #93's two graph-ingest tests 300 times each at `-cpu=1`; `TestGraphIngestServesExactlyTheDeclaredVerbs` failed 116 of 300 at PR #93's head `2799632` and 0 of 300 with `570e8ae` merged in, and `TestGraphIngestProvisionsNoSuffixIndex` 0 of 300 in both (task 2.5, comment 6091567435). `natsclient/` is the same at `570e8ae`, `718f700` and `987a33f`. Task 2.6: CI run 38009721793 on `987a33f`, success. Not checked with a wrong change: the helper's CPU count, its share of the time limit and its no-time-left refusal (`:535`), `-test.gocoverdir`, and `GORACE` (comment 6091577433, "Not covered"). |
