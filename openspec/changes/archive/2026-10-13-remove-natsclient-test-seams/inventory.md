# Delta inventory: the tests behind natsclient's test-only hooks (#192, PR #194)

base: a5020db (main) + b28c1f2 (empty claim commit), worktree `/Users/coby/Code/c360/semengine-wt/claude/natsclient-test-seams`.
Phase: inventory only. No options, recommendation, real-world-trigger judgment or tasks.

Question (the coordinator's brief for #194): for each test that uses `netDial`, `opHook`, `commitHook`, `connectWith`'s
`dial` parameter or `carriesClientHandlers`, including every caller of `newReconnectDialClient`, `newClientOwner` and
`connectScripted`: (1) what it proves; (2) what the hook does for it; (3) a hook-free way already in this tree to produce
the same situation; (4) another test that proves the same with no hook; (5) the production code that would go with the
hook.

Revision r2 (review `/tmp/pr194/inventory-review.r1.md`): adds a seventh seam, `subscribeWith`'s `subscribe`
parameter, and its test (F1, row 33); says what the two `runIfCommitFree` tests guard (F2, rows 1 and 6, §4, §5);
corrects `newSocketDialer`'s count (N1). r1 is kept at `/tmp/pr194/inventory.r1.md`. Revision r3 (INVENTORY PASS on
r2, sha256 `34e526a5…`, kept at `/tmp/pr194/inventory.r2.md`) folds round 2's two citation notes: §4's quoted sentence
is at `design.md:374`, and §8 maps the lines PR #188 moves.

Base: #192's site table (24 sites in 6 files; comment 6125947321 read too) is the site map, re-measured here:
`git grep -n -E '\.(netDial|opHook|commitHook) *=|connectWith\(' -- 'natsclient/*_test.go'` gives the same 24 lines,
and the three helpers have 2, 11 and 7 call sites. Every line number is on `a5020db`. PR #188 edits `client.go` and
`client_close_final_test.go`, so these numbers move once it merges; its design (`/tmp/pr188/design.md`) removes no
test listed here.

## 0. PR #191 (#190)

PR #191 has no content commit yet (0 files at `34fff5a`). #190 sorts **exported** symbols into framework API, plumbing and
back door ("Every exported symbol in a production package is one of three kinds", #190 body). The seven seams here are
unexported (three fields, three function parameters, one function), so #190's sorting does not reach them. The rule
that does is rule #186, which covers production code "exported or not" whose only caller is a test
(`.agents/contracts/semengine-developer.md:110`-`:114`; reviewer contract `:313`-`:321`).

## 1. Hook-free ways to set up a situation, already in this tree

| Technique | Where | What it produces |
| --- | --- | --- |
| Embedded server, ephemeral port | `natsclient/client_lifecycle_test.go:108` `embeddedServerURL`; `client_close_final_test.go:183` `embeddedJetStreamURL` | a real server and a real Connect |
| `silentListener` | `natsclient/client_connect_test.go:365` | a TCP listener that accepts and never answers. Its `accepted` channel shows whether Connect dialled (used at `:141`-`:158`, "no dial"). It holds Connect in the handshake, after the TCP dial |
| `refusedNATSURL` | `natsclient/test_helpers_test.go:31` | a dial that fails at once |
| `server.DisconnectClientByID` | `natsclient/client_close_held_test.go:189`, `client_connect_test.go:315` | a dropped connection with the server still up; nats.go reconnects |
| `heldProxy` / `newHeldClient` / `holdPublish` | `natsclient/client_close_held_test.go:279`, `:207`, `:234` | a TCP proxy that stops reading after the first bytes of a publish on `heldSubject`, so every later write waits behind it (T1, T2) |
| Logger hold: `newRecordingHandler(msg)` through the production option `WithLogger` | `natsclient/client_lifecycle_test.go:140`; used by `client_close_final_test.go:836` (test 11) and `client_lifecycle_test.go:765` (`TestClientCloseDuringConnectStartsNothing`, held on "Successfully connected to NATS") | holds the first goroutine that logs a given message. Connect's log lines: "Connecting to NATS" `client.go:766`, after the started check (`:734`) and the Connecting write (`:757`-`:763`); "Successfully connected to NATS" `:842`, after admission. No log line falls between the health monitor's RTT and its commit (`:2388`-`:2396`), inside the event handlers' check-and-write (`:2190`-`:2197`, `:2207`-`:2213`), or in `handleClosed` (`:2307`-`:2322`) |
| Context cancel or deadline | e.g. `client_connect_test.go:189`-`:204` | ends Connect's wait in the handshake |
| Failure count | `Client.Failures()` (`client.go:452`) and `GetStatus().FailureCount`; the second asserted zero at `client_connect_test.go:153` as "Connect made no attempt" | a failed Connect raises it; an attempt that never runs leaves it |
| NATS fixture `Restart` (integration lane) | `internal/harness/natsfixture/restart.go:40` | stop and start the broker; it may come back on another port (`:28`-`:30`) |

Searched and not found in tests: server-side connection counting (`NumClients`, `Connz`, `Varz`); a portable address
that holds a TCP dial open (the field's own comment says there is none, `client.go:100`-`:102`); a scripted server on a
real TCP socket (`serveScripted`, `subscribe_registration_test.go:48`, runs only on a `net.Pipe`).

## 2. Production code each hook keeps (question 5)

| Hook | Production lines today (on `a5020db`) | Count |
| --- | --- | --- |
| `netDial` | field and comment `client.go:100`-`:103`; default in `NewClient` `:364`; passed at `:775` (`newSocketDialer(m.timeout, m.netDial)`) | 5 lines, plus the `:775` argument |
| `newSocketDialer`'s `dial` parameter (a sixth seam, not in #192) | `socket.go:16` field goes; the constructor `:23`, `:24` and the call `:35` change. Its only argument other than `netDial` is a test's: `socket_test.go:17` (`TestSocketDialerRefusesADialAfterTheCut`). With `netDial` gone, production always passes `(*net.Dialer).Dial` | 4 lines (1 goes, 3 change) |
| `opHook` | field and comment `:203`-`:205`; `recordOp` `:303`-`:307`; calls `:752`-`:754` (with their comment) and `:1172` | 12 lines |
| `commitHook` | field and comment `:207`-`:211`; `commitSeam` `:309`-`:313`; calls `:873`, `:2196`, `:2212`, `:2392` | 14 lines |
| `connectWith`'s `dial` | parameter `:728`, doc `:724`-`:725`, call `:778`; `Connect` `:719`-`:721` passes `nats.Connect` | 4 lines; `connectWith` could fold into `Connect` |
| `subscribeWith`'s `subscribe` parameter (a seventh seam, not in #192; review F1) | goes: the `subscribeFunc` type and comment `:1357`-`:1360` ("it is a seam the way connectWith's dial is"), the `nativeSubscribe` wrapper `:1362`-`:1368`, which exists only to be passed, and `subscribeOwned`'s parameter `:1400`. Changes: `subscribeWith`'s parameter `:1371`, the arguments at `:1354`, `:1386` and `request.go:413`, and the call `:1435`, which would call `conn.Subscribe`. `subscribeWith` (`:1370`-`:1387`) could then fold into `Subscribe`, as `connectWith` into `Connect`. Production always passes `nativeSubscribe`; the only other argument is `client_close_final_test.go:1237`-`:1240`. Not at the pin: the pin's `client.go` has no `subscribeFunc`, `subscribeWith` or `nativeSubscribe` (`gh api` read, `grep`) | 12 lines go, 5 change |
| `carriesClientHandlers` | `:341`-`:345`; callers `:893`-`:897` (`discardCandidate`) and `:1040` (cleanup). `buildConnectionOptions` always sets `nats.ClosedHandler(m.handleClosed)` (`:665`), so without the `dial` seam it reduces to `conn != nil` | 5 lines, plus 2 call sites |

Adjacent, carried from the pin, optional under `.agents/protocol.md` § Bounded port scope (inherited structure, not
required work): the `nativeSubscription` interface, `client.go:1226`-`:1231` (pin `natsclient/client.go:746`). Its
implementers are `*nats.Subscription` and two test fakes: `fakeInvalidSubscription` (`client_close_final_test.go:1176`)
and `fakeNativeSubscription` (`subscription_test.go:19`, passed to `newSubscription` at 11 sites in that file). Removing
the `subscribe` seam does not remove it: `newSubscription` (`client.go:1245`) still takes it. On `a5020db`
`watchOwnedSubscription` (`:1503`) takes it too; at PR #188's head `4f91838` it no longer does
(`watchOwnedSubscription(d, finish)`, `client.go:1461` there).

Test side, not production: the lifecycle adapter's `Calls` (`client_lifecycle_test.go:36`-`:50`) depends on `opHook`.
The harness field `lifecycletest.Observation.Calls` stays, because other adapters fill it
(`internal/harness/natsfixture/fixture_integration_test.go:647`, `metric/lifecycle_test.go:70`).

## 3. One row per test

Hook letters: N `netDial`, O `opHook`, C `commitHook`, D `connectWith`'s `dial`, H `carriesClientHandlers`, S
`subscribeWith`'s `subscribe`.
Kind, the answer to question 2: **hold** (holds one call at a point while another runs), **count** (observes that a call
happened), **fake** (fakes a connection, a subscription or the transport), **guard** (offers another call a gap between
an ownership check and its status write: on correct code the check and the write are one step under `mu`, so there is no
gap and the test runs its fallback; on a change that splits them, the other call runs inside the gap and the test
fails; §4), **none** (installs the adapter and never reads its count).

| # | Test (`natsclient/`) | Hook | (1) Proves | (2) Kind: what the hook does | (3) Hook-free way in this tree | (4) Proved elsewhere with no hook |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `client_close_final_test.go:465` `TestClientLosingConnectLeavesWinnerStatus` (4 subtests) | D, O, C | When two Connects race, the one that loses never changes the winner's status, failure count or circuit | **hold.** D holds the loser's dial open, then fails it (`:482`, `:531`, `:571`). O holds the loser after its started check and before its Connecting write, while the winner connects (`:518`). The `held-before-failure-write` subtest is a **guard** (§4): C offers the winner the gap between the loser's ownership check and its failure write (`:563`); on this code there is none, so the winner connects after the write; on a split the winner runs inside it and the subtest fails ("the loser's failure"; mutation-detected 3 of 3) | none found. For the guard subtest, as row 6. One URL serves both Connects, so `silentListener` would hold the winner too. A logger hold on "Connecting to NATS" comes after the Connecting write, so it misses the `held-before-connecting` window | none: row 13 also uses hooks |
| 2 | `client_close_final_test.go:587` `TestClientHealthMonitorCannotOverwriteClosedStatus` | C | Once Close has finished, a health check that was in flight cannot set the status back | **hold.** It holds the monitor between its RTT (`client.go:2389`) and its commit (`:2396`) while Close finishes and writes Disconnected, then releases it | none found: no log line in that window (§1). Holding the RTT itself would need a withheld PONG, which only the scripted server gives (it needs D) | partly: the table's `closing` and `closed` rows (row 25) check status after Close, but not with a monitor in flight |
| 3 | `client_close_final_test.go:716` `TestClientConnectAfterCloseRefusesBeforeDial` | D, O (adapter) | A Connect after Close returns `ErrConnectionClosed`, never dials, and writes no status | **count.** D records any dial; O counts "dial" (`:729`) | `silentListener`'s `accepted` channel, as `client_connect_test.go:141`-`:158` does for an ended context | partly: `client_connect_test.go:163`-`:173` (`TestConnectRefusesAnEndedContext/after Close`) checks the error on `nats://unused`, not the absence of a dial |
| 4 | `client_close_final_test.go:735` `TestClientFailuresAfterCloseLeaveStatusDisconnected` | O (adapter) | Failures recorded after Close leave the status Disconnected and arm nothing | **none.** It reads `Unresolved` only (`:744`) | n/a | n/a |
| 5 | `client_close_final_test.go:749` `TestClientConnectDuringCloseDrainReturnsPromptly` | O | A Connect that passed its started check before another Connect won returns `ErrConnectionClosed` at once while Close drains, rather than waiting for the drain | **hold.** It holds the late Connect at its "dial" report (`client.go:754`), after its started check, while the winner connects and Close begins a held drain | candidate, not measured: a logger hold on "Connecting to NATS" (`client.go:766`) is also after the started check. The hold is once only (`client_lifecycle_test.go:157`-`:162`), so the winner would pass | none: the table's `closing/Connect` cell (row 25) starts after Close began and is refused at entry (`client.go:737`-`:738`) |
| 6 | `client_close_final_test.go:795` `TestClientEventHandlerCannotCommitAfterClose` (2 subtests) | C | A connection event racing Close cannot leave a status after Close: the handler's ownership check and its status write are one step | **guard** (§4). C offers Close the gap between `handleReconnect`'s or `handleDisconnect`'s check and its write (`client.go:2190`-`:2197`, `:2207`-`:2213`). On this code `mu` is held there, so `runIfCommitFree` (`client_close_final_test.go:210`-`:221`) leaves Close for afterwards, as its comment says; on a split, Close runs inside the gap and the test fails ("the handler committed a status after Close"; mutation-detected 3 of 3) | none found: a gap between check and write exists only on a wrong change, and C is the only thing in the tree that runs another call inside it | none found for "one step". The direct handler calls in `client_test.go:582`-`:727` and `client_lifecycle_test.go:390`-`:746`, and the table's `closing` and `closed` rows (row 25), run the handler and Close one after the other, so a split would pass them (reasoned, not mutation-tested). `-race` would not catch a split either: the lock is released and taken again, so there is no unsynchronized access |
| 7 | `client_close_final_test.go:862` `TestClientLosingConnectLeavesNoCandidateHandler` | O, D | A Connect that loses does not return until its discarded connection's event handlers have finished | **hold.** O holds the loser after its started check (`:869`). D wraps a real dial that keeps the client's options (`:879`-`:888`) and adds a closed handler that holds | partly: a logger hold on "Connecting to NATS" for the ordering (as row 5). Holding the candidate's closed handler: none found (`handleClosed` logs nothing) | none |
| 8 | `client_close_held_test.go:85` `TestCloseEndsAReconnectDial` (via `newReconnectDialClient` `:150`) | N | Close returns within its context while nats.go is dialling a reconnect, by ending the dial (T8) | **hold.** N lets the first dial through and holds every reconnect dial until the dialer's Cancel (`:166`-`:180`). `DisconnectClientByID` (`:189`) makes the reconnect start | partly: `silentListener` as the reconnect target would hold the handshake, not the TCP dial; whether that holds nats.go's lock as the dial does (P23) is not measured. The Cancel path itself: none (`client.go:100`-`:102`) | partly: T1 `TestCloseEndsAHeldWrite` (`client_close_held_test.go:24`, `heldProxy`) for the socket-close path; `socket_test.go:16` for the Cancel path, through `newSocketDialer`'s seam |
| 9 | `client_close_held_test.go:101` `TestSubscribeEndedByCloseIsConnectionClosed` (via `newReconnectDialClient`) | N | A Subscribe admitted before Close, whose connection check fails after Close began, returns `ErrConnectionClosed`, not `ErrNotConnected` | **hold.** The held reconnect dial keeps nats.go's lock, so the Subscribe's check runs only after Close began | candidate: `heldProxy` (§1) holds a Subscribe behind a write instead | partly: T2 `TestCloseIsNotHeldByAHeldSubscribe` (`:57`, `heldProxy`) asserts `ErrConnectionClosed` for a Subscribe held across Close's start, whichever step fails it (`:74`-`:77`) |
| 10 | `client_connect_test.go:19` `TestConnectRejectsCandidateWhenContextCanceledDuringDial` | D, H | If Connect's context ends while it dials and the dial then succeeds, Connect returns the context error, closes that connection and installs nothing | **hold + fake.** D holds the dial and then returns a connection the test made on an in-process server, without the client's options (`newConnectCandidate` `:111`-`:130`). H is what keeps Close from waiting for that connection's handlers | partly: `silentListener` with a cancel (`TestConnectEndsWithItsContext/cancel once accepted`, `:189`-`:204`) ends Connect in the handshake, so no connection is returned. "Dial returned, then the context ended": none found | partly: that subtest checks the error only |
| 11 | `client_connect_test.go:54` `TestConnectRejectsCandidateWhenCloseWinsAdmission` | D, H | If Close begins while Connect dials and the dial then succeeds, Connect returns `ErrConnectionClosed`, closes the connection and installs nothing | **hold + fake**, as row 10 | none found. Close does not cut an uninstalled Connect's socket (`client.go:964`-`:972` takes the installed connection), so with `silentListener` Connect would wait for its own deadline and return no connection | partly: `client_connect_test.go:163`-`:173` (Connect after Close, refused at entry) |
| 12 | `client_connect_test.go:90` `TestConnectCommitsCandidateWhenAdmissionRemainsOpen` | D, H | A Connect whose dial returns a connection installs it: status Connected and JetStream ready | **fake.** D returns the in-process connection; H keeps the later Close from waiting for handlers it lacks | yes: Connect to `embeddedServerURL` | yes: every Connect to an embedded server, e.g. `client_lifecycle_test.go:288`, and the table's `new/Connect` cell (row 25) |
| 13 | `client_lifecycle_test.go:322` `TestClientConnectThatLosesAdmissionLeavesStatus` | O, D | Two Connects pass the started check; the one that resumes after the other connected is refused with `ErrAlreadyStarted` and leaves the winner's connection and status | **hold.** O holds the first Connect at its "dial" report (`:333`). D wraps a real dial (options kept) to observe the rejected connection's closed handler (`:346`-`:351`) | candidate, not measured: logger hold on "Connecting to NATS", as row 5 | none |
| 14 | `client_lifecycle_test.go:255` `TestClientConnectRefusesNilContext` | D | `Connect(nil)` returns an error and never dials | **count.** D counts dials (`:261`-`:266`) | `silentListener`'s `accepted` channel, as `client_connect_test.go:141`-`:158` | none found (`nil_context_siblings_test.go` and `reviewer_nil_context_probe_test.go` have no `Connect` case: `grep -n Connect`) |
| 15 | `client_lifecycle_test.go:283` `TestClientSecondConnectRefused` | D | A second Connect on a connected client returns an error, never dials, and leaves the first connection | **count.** D counts dials (`:301`-`:308`) | partly: the connection staying the same and connected is already asserted with no hook (`:311`-`:313`). A dial count against a real server: none found (no server-side counting) | partly: the table's `connected/Connect` cell (row 25), whose dial check also uses O |
| 16 | `client_connect_test.go:177` `TestConnectEndsWithItsContext`, subtests `deadline while dialling` (`:221`) and `host name resolved inside the dial` (`:245`) | N | Connect returns when its context ends while the TCP dial itself is pending, and a host name reaches the dialer unresolved (T4) | **hold.** N replaces the network dial with one that waits for Cancel or its timeout; the second subtest also captures the address | none: no portable address holds a dial open (`client.go:100`-`:102`), and the lookup cannot be observed without the dialer | partly: the same test's subtests `deadline`, `cancel once accepted` and `two addresses` (`:178`-`:216`, `silentListener`, no hook) cover the handshake phase |
| 17 | `integration_test.go:163` `TestIntegration_CircuitBreakerWithRealConnection` (tag `integration`) | O | After 15 failed Connects the circuit opens, and a Connect with the circuit open fails at once without dialling | **count.** O counts "dial" (`:171`-`:175`, `:194`-`:200`) | candidate, not measured: `Failures()`, already asserted 15 at `:190`, staying 15 after the circuit-open Connect (a failed dial adds a failure, `client.go:867`-`:876`); or `silentListener`, though it never answers, so 15 Connects would each wait for their timeout | none found |
| 18-24 | `subscribe_registration_test.go`: T2 `:296` `…BoundedByDefaultRequestTimeout`, T3 `:318` `…CancelledDuringRoundTrip`, T4 `:345` `…EndedContextSendsNoSub`, T5 `:364` `…ConnectionLostDuringRoundTrip`, T6 `:390` `…ReturnsWhileHandlerHeld`, T7 `:434` `…CloseBeginsDuringRoundTrip`, T8 `:479` `…CloseEndsConnectionDuringRoundTrip` (via `connectScripted` `:177`) | D | How Subscribe ends while the server has not answered its registration PING: bounded by `DefaultRequestTimeout` (5 s, `request.go:18`), by a cancel, by an ended context, by a lost connection, by Close, and with a handler held | **fake.** D routes the connection through a `net.Pipe` to a scripted server (`:191`-`:195`), which bypasses the client's own `socketDialer`. T2 to T7 then run in a `synctest` bubble in a process of their own, so a withheld PONG costs no wall time (`:28`-`:31`, `:505`-`:510`); T8 runs on real time | partly: `heldProxy` withholds every write after a held publish, not one PONG; a scripted server on a real TCP listener does not exist. Whether a `synctest` bubble can run over a TCP socket is not measured. On real time, T2 and T6 would each wait 5 s | partly: T1 `:240` (embedded server, no hook) for registration; `client_close_held_test.go:57` (T2 there) for Close; `client_close_final_test.go:227` for Close joining a held Subscribe handler. By name, none of the 11 Subscribe tests outside this file (`grep -n '^func Test.*Subscribe'`) covers the timeout bound, cancel, ended context or lost connection during the round trip; their bodies were not read |
| 25 | `client_close_final_test.go:960` `TestClientLifecycleOperationTable` (via `buildTableClient` `:1057`) | O (adapter) | Each operation's return and status in each lifecycle state, and whether Connect dialled | **count.** It reads `Calls["dial"]` (`:1013`, `:1030`) | none found for a dial count on the JetStream server the table uses | the returns and statuses need no hook; the dial column only through O |
| 26 | `client_lifecycle_test.go:859` `TestClientLifecycleSuite` (2 adapter sites) | O (adapter) | The lifecycle suite on Client, including that Stop after a failed Start makes no call | **count.** The suite compares `Calls` before and after Stop (`internal/harness/lifecycletest/lifecycletest.go:299`-`:305`). Without O, `Calls` is nil on both sides and the comparison passes trivially | the same check's `requireNothingRetained` (`:296`) runs with no hook | partly: as (3) |
| 27 | `client_close_final_test.go:1145` `TestClientLifecycleAdapterListsHeldHandler` | O (adapter) | The adapter lists a running handler after Close cleared the connection, and nothing after a nil Close | **none** (`Unresolved` only, `:1160`, `:1164`) | n/a | n/a |
| 28 | `client_lifecycle_test.go:420` `TestClientCloseDropsLateDisconnect` | O (adapter) | Once Close has begun, a disconnect arms no connection-loss timer and the drop is logged | **none** (`:444`-`:464`) | n/a | n/a |
| 29 | `client_lifecycle_test.go:470` `TestClientCloseStopsCircuitTimer` | O (adapter) | Close stops a pending circuit-test timer | **none** (`:482`-`:490`) | n/a | n/a |
| 30 | `client_lifecycle_test.go:643` `TestClientCloseJoinsInternalClaimRelease` | O (adapter) | Close joins an internal consumer's claim release | **none** (`:667`, `:676`) | n/a | n/a |
| 31 | `client_lifecycle_test.go:765` `TestClientCloseDuringConnectStartsNothing` | O (adapter) | Close during Connect's last steps leaves no monitor or poller running (the ordering is forced by a logger hold, not a hook) | **none** (`:792`) | n/a | n/a |
| 32 | `client_lifecycle_test.go:802` `TestClientCloseJoinsClaimRelease` | O (adapter) | Close joins a port consumer's claim release; a consumer started after Close began is refused and its claim released | **none** (`:834`-`:854`) | n/a | n/a |
| 33 | `client_close_final_test.go:1227` `TestClientRefusesLateDeliveryAfterRecordedEnd/core` (test at `:1186`) | S | A message handed to a core subscription's callback after the client recorded that subscription's end does not run the handler; it is logged at warn level with the subject and ack policy `core`, and counted as `late_delivery_refused` | **fake.** S returns `fakeInvalidSubscription` (`:1176`-`:1181`), invalid from creation, so `newSubscription` records the end at once (`client.go:1250`-`:1253`); S also keeps the native callback, so the test can deliver through it after the end (`:1236`-`:1243`) | none found for a core subscription: no test reaches a core subscription's callback after its end except through S (`git grep -n -E 'late_delivery_refused\|lateRefusals\(' -- 'natsclient/*_test.go'`) | partly: the same test's JetStream subtests `ack-explicit` and `ack-none` (`:1192`-`:1225`) drive the same refusal (`ownedDelivery.run`, `owned_delivery.go:41`-`:48`) through a fake consumer that `fakeConsumerClient` (`:640`) installs in the client's `js` field, a production field, not through S. There is no direct test of `ownedDelivery` (no `owned_delivery_test.go`) |
| extra | `socket_test.go:16` `TestSocketDialerRefusesADialAfterTheCut` | `newSocketDialer`'s `dial` | After a cut, a dial never reaches the network, and a second cut returns | **fake**: a dial function that fails the test if called | n/a: it tests the dialer alone | none |

The seven scripted tests share rows 18-24. The rows cover 33 tests: 14 with a hook site in #192's map (rows 1-3, 5-7,
10-17), 2 through `newReconnectDialClient` (8, 9), 9 through `newClientOwner` alone (4, 25-32), 7 through
`connectScripted` (18-24), and 1 through S (33), plus the extra seam. Row 3 also builds its client with `newClientOwner`.

## 4. What the two `runIfCommitFree` tests guard

Applies to row 6 and to row 1's `held-before-failure-write` subtest. They guard one property: the ownership check and
the status write are one step. The property is from `natsclient-status-ownership`
(`openspec/changes/archive/2026-10-05-setup-04a-01-floor/design.md:369`; the sentence quoted is at `:374`): "Once
`Close` has begun, nothing but `Close` changes `Status()`", and a Connect that loses changes neither the status,
nor `Failures()`, nor the circuit.

**By design, on correct code these hooks find the section held.**

- The "connect failure" and "event handler" seams are called by the goroutine that already holds `mu`:
  `connectFailed` (`client.go:868`-`:873`), `handleDisconnect` (`:2190`-`:2196`) and `handleReconnect`
  (`:2207`-`:2212`). The field's comment says so (`client.go:207`-`:209`: "inside the commit's critical section").
- So `runIfCommitFree`'s `TryLock` (`client_close_final_test.go:214`-`:219`) fails. The other call is left for the test
  to run afterwards, as the helper's comment states (`:210`-`:213`).
- Measured on a copy of the tree (`git archive HEAD` into `/tmp/pr194/copy`, nothing written in the worktree): with a
  counter added to `runIfCommitFree`, the helper ran its function 0 of 20 runs. This is certain from the code:

```bash
go test -count=20 -race -run \
  '^(TestClientLosingConnectLeavesWinnerStatus|TestClientEventHandlerCannotCommitAfterClose|TestZZCommitFreeNeverRan)$' \
  ./natsclient/
```

**On a wrong change they fail.**

- If a change splits the check from the write (unlock, seam, relock), `TryLock` succeeds and the other call runs inside
  the gap.
- The reviewer measured this with the program behind `task mutate:check`: a wrong change in a copy outside the
  repository, 3 baseline runs, 3 mutant runs, then an after-run. Both gave **detection**, 3 of 3:
  - `handleReconnect` split before `setStatus`, against `TestClientEventHandlerCannotCommitAfterClose/handleReconnect`
    ("the handler committed a status after Close"). Report: `/tmp/pr188-review/m194/e.log`.
  - `connectFailed` split before `recordFailureLocked`, against
    `TestClientLosingConnectLeavesWinnerStatus/held-before-failure-write` ("the loser's failure"). Report:
    `/tmp/pr188-review/m194/f.log`.
  - Both reports are local only.

**Question (4) for this property:** no hook-free test found that proves the check and the write are one step. The other
tests that call these handlers run them one after the other (row 6). The "health monitor" seam (`:2392`) is outside
`mu` by design, and row 2 is a hold, not a guard.

## 5. Counts by what the hook does (question 2)

| Kind | Tests | Rows |
| --- | --- | --- |
| hold | 10 | 1, 2, 5, 7, 8, 9, 10, 11, 13, 16. Of these, 3 hold a dial open with N (8, 9, 16), 2 hold a dial and return a faked connection (10, 11), and 5 hold a point with O or C (1, 2, 5, 7, 13). Row 1's `held-before-failure-write` subtest is a guard (§4) |
| guard | 1 | 6 (and row 1's `held-before-failure-write` subtest), §4 |
| count | 6 | 3, 14, 15, 17, 25, 26 |
| fake | 9 | 12, the 7 scripted tests (18-24), and 33 |
| none | 7 | 4, 27, 28, 29, 30, 31, 32 |

Plus the extra seam: one test, **fake**.

Total: 33 tests.

## 6. Searches that closed empty

- Server-side connection counts in tests: `git grep -n -E 'NumClients\(\)|NumConns|Connz\(|server\.Varz' -- '*_test.go'`:
  none.
- Hook-free `Connect(nil)` tests: `grep -n Connect nil_context_siblings_test.go reviewer_nil_context_probe_test.go`: none.
- A log line inside the health monitor's RTT-to-commit window, the event handlers' check-and-write, or `handleClosed`:
  `awk` over `client.go:730`-`:860`, `:2186`-`:2340`, `:2370`-`:2405` for `logger.`: none in those windows (§1).
- Other `subscribeWith` or `subscribeFunc` arguments: `git grep -n -E 'subscribeWith\(|subscribeFunc|nativeSubscribe\b'`
  gives production (`client.go:1354`, `:1386`, `request.go:413`) and `client_close_final_test.go:1236` only.
- A hook-free test that the handlers' check and write are one step: every test call of `handleReconnect` or
  `handleDisconnect` (`git grep -n -E 'handleReconnect\(|handleDisconnect\(' -- 'natsclient/*_test.go'`, 15 lines) runs
  it one after the other with Close, not inside its critical section; no test calls `connectFailed`.
- Other `newSocketDialer` callers: `grep -n newSocketDialer *.go` gives `client.go:775` and `socket_test.go:17`.

## 7. Open evidence questions

1. Logger hold as a stand-in for the `opHook` "dial" hold (rows 5, 7, 13): "Connecting to NATS" is logged after the
   started check and the Connecting write. Whether holding there reproduces each test's ordering, given the hold fires
   for the first Connect to log it, is not measured. Row 1's `held-before-connecting` needs the point before the write,
   which no log line marks.
2. `silentListener` as a reconnect target (row 8): it would hold nats.go's handshake rather than its TCP dial. Whether
   nats.go holds the connection lock there, as P23 measured for the dial, is not measured.
3. `Failures()` as a hook-free "no dial" observable (row 17): reasoned from `connectFailed` (`client.go:867`-`:876`),
   not run.
4. Whether a `synctest` bubble can drive the scripted server over a real TCP socket instead of a `net.Pipe` (rows
   18-24): not measured. Without a bubble, T2 and T6 each wait `DefaultRequestTimeout`, 5 s.
5. The pin has `connectWith` (SemStreams `8b99efe9` `natsclient/client.go:472`-`:478`, read with `gh api`); the
   same grep finds no `opHook`, `commitHook`, `netDial` or `carriesClientHandlers` there. Which of these tests came
   from the pin: not measured.
6. The real-world trigger for each forced ordering is the design's question (#185), not answered here.

## 8. Lines PR #188 moves (its head `4f91838`, read with `gh api …/contents/…?ref=4f91838`)

`client_lifecycle_test.go`, `client_connect_test.go`, `socket.go` and `request.go` are unchanged there. In
`client_close_final_test.go` every line before `:948` is unchanged, including `runIfCommitFree` (`:210`) and the stale
`tableState` comment (`:947`).

| Item | `a5020db` | `4f91838` |
| --- | --- | --- |
| `netDial` field, `NewClient` default | `client.go:103`, `:364` | `:103`, `:362` |
| `opHook`, `commitHook` fields | `:205`, `:211` | `:203`, `:209` |
| `recordOp`, `commitSeam`, `carriesClientHandlers` | `:303`, `:309`, `:343` | `:301`, `:307`, `:341` |
| `Connect` → `connectWith`; its `dial` parameter; the dial call | `:720`, `:728`, `:778` | `:699`, `:707`, `:757` |
| `recordOp("dial")`, `newSocketDialer(…, m.netDial)` | `:754`, `:775` | `:733`, `:754` |
| `commitSeam("connect failure")`; `discardCandidate`'s await | `:873`, `:893` | `:851`, `:871` |
| `cleanup`'s `carriesClientHandlers`; `recordOp("drain")` | `:1040`, `:1172` | `:1016`, `:1138` |
| `nativeSubscription`, `newSubscription` | `:1226`, `:1245` | `:1192`, `:1211` |
| `Subscribe`'s argument, `subscribeFunc`, `nativeSubscribe`, `subscribeWith` | `:1354`, `:1360`, `:1362`, `:1370` | `:1320`, `:1326`, `:1328`, `:1336` |
| `subscribeOwned`'s parameter, the `subscribe` call | `:1400`, `:1435` | `:1366`, `:1401` |
| `commitSeam("event handler")` ×2, `commitSeam("health monitor")` | `:2196`, `:2212`, `:2392` | `:2134`, `:2150`, `:2330` |
| `TestClientLifecycleOperationTable`; its `Calls["dial"]` reads | `client_close_final_test.go:960`; `:1013`, `:1030` | `:957`; `:1006`, `:1016` |
| `TestClientLifecycleAdapterListsHeldHandler`; `…RefusesLateDeliveryAfterRecordedEnd`; its `core` `subscribeWith` call | `:1145`; `:1186`; `:1236` | `:1114`; `:1155`; `:1205` |
