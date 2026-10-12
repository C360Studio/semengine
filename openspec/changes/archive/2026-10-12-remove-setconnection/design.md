# Design: remove-setconnection, revision 1

Status: draft for pre-owner review. Issue #184; claim PR #188. Base `a5020db`.

Rests on `inventory.md`: INVENTORY PASS r1 with its six corrections folded, rewrapped to the 120-column limit. Its
path is `/tmp/pr188/inventory.accepted.md`, sha256 `e3f9b821e7be1dd1a29ff32a706e591716349ee0d86c302dc34b8f6fe4addc05`;
the reviewed original is `fc35bf7d…05faae44`. "§n" below is a section of that inventory.

## The problem

`natsclient.Client.SetConnection` (`natsclient/client.go:421`-`:438`) swaps the connection under a running client.
Its doc says "(for testing)", and nothing but tests calls it, here, in the four consumers, or at the pin (P1).
To cover a swap, SemEngine added code to Close, the subscription watcher, the consumer goroutine and the JetStream
accessor (P3). It also added seven spec lines, an ADR-112 bullet, and four tests. The owner, 2026-10-12: "I have no
idea why we have a setter in Go for SetConnection — this is not Java." Exported production code that only tests call
is `BLOCKING` (reviewer contract `:313`-`:321`, #186).

## Options

| Option | What it costs |
| --- | --- |
| O0. Do nothing | Keeps an exported setter that only tests call, and 12 statements of special cases that only it reaches (P3). Keeps spec and ADR text about a state no caller can produce. #169's `SetConnection` case stays open. |
| O1. Remove the method only | The special cases stay as branches no production path reaches (P2), and the spec keeps requirements about a state nothing produces. A smaller diff that keeps the debt. |
| **O2. Remove the method, the code and tests that exist only for it, and its spec, ADR and ledger text** | About 290 test lines deleted (tests `:1395`-`:1596`, 202 lines; the integration test, 65; the table's column, about 25) and about 60 production lines (the method is 18; the rest are the §2b rows and their comments). These are estimates from the line ranges, not a diff. In the spec, two requirements are modified and one is removed and re-added under a new heading. The ADR gets a dated note, and the ledger gets four edits. It deletes one integration test carried from the pin (Q1). |
| O2b. O2, but keep the integration test's property on a real disconnect | See Q1. |

## Recommendation: O2

**What goes** (P3, §2b): `SetConnection`; the `dialled` field and its three reads (`client.go:970`, the
`conn == m.dialled` operand; `:1044`-`:1046`; `:1571`); the watcher's `<-closing` arm with its `replaced` read and
Unsubscribe (`:1509`-`:1516`); `ownConsumer`'s `replaced` read and `h.Stop()` (`owned_delivery.go:155`-`:160`) and its
`conn` parameter; `guardedConsumer.conn`; `jetStreamWithConn`'s second result; the condition at `client.go:1047`, which
is dead either way (P4); and the comment clauses that name the swap (§2d, the `closedSignals` sentence `:195`-`:199`).
The developer decides the shape of what is left, for example whether `jetStreamWithConn` folds into `JetStream`.

**What stays** (P4, §2c): `closingSignal`, `draining`, `closedSignals`, `carriesClientHandlers`, `ownsStatusLocked`,
`socket`, and the watcher's wait for its subscription's delivery to end.

- The phrase "on a connection the client dialled" stays in the spec, ADR-112, README `:16` and `doc.go`. It is still
  true. Removing it would mean two more modified requirements, the Purpose, the README, `doc.go` and ADR history,
  and no behaviour would change.
- **Spec route.** OpenSpec 1.14.0 refuses a MODIFIED block that drops scenarios: "Copy the omitted scenarios into the
  MODIFIED block (a MODIFIED requirement replaces the whole block, so archive refuses to drop them)"
  (`openspec validate --strict` on a copy). So "A nil Close means everything the client owns has finished" is REMOVED
  and ADDED as "Close returns nil only after everything the client owns has finished", with its text and four
  remaining scenarios kept word for word. The other two requirements are MODIFIED. The new heading is forced, not
  chosen: REMOVED plus ADDED under the same heading is refused ("Requirement present in both ADDED and REMOVED"),
  and RENAMED carries the old scenarios into the same scenario check (`node_modules/@fission-ai/openspec/dist/core/specs-apply.js:330`-`:389`).
  - On a copy of `openspec/`, the delta validates strictly and archives as `+ 1, ~ 2, - 1`. It leaves no
    `SetConnection` in the spec and moves the re-added requirement to the end of the spec.
  - `git grep` finds no other citation of the old heading, and #93's delta names neither heading.
- `request.go:465`-`:467` stays as it is. The comment is carried unchanged from the pin (pin `request.go:403`-`:405`),
  and probe P-184-1 backs it (P6).

**What a caller observes, and the test that proves it.** No new test: the change removes surface. Task 2.5 shows
that the named tests fail on a wrong change to each path the change simplifies.

| Observable | Test |
| --- | --- |
| `Client` has no `SetConnection`; a call site does not compile | `task build`; PR #188 body (consumers) |
| Close returns nil only after a running `Subscribe` or `SubscribeForRequests` handler has returned | `TestClientCloseJoinsSubscribeHandlerAfterForcedClose` (`client_close_final_test.go:227`) |
| Close joins a running consumer handler; a refused setup holds its claim until its handlers return | `TestClientCloseJoinsConsumerHandlerWhenClosedReportsEarly` (`:654`), `TestClientRefusedConsumerKeepsOwnershipUntilHandlersReturn` (`:303`) |
| Close joins the event handlers of the connection the client dialled | `TestClientCloseJoinsConnectionEventHandlers` (`:835`) |
| A consumer's handler context ends when Close begins | `TestConsumeHandlerContextEndsWhenCloseBegins` (`consume_context_test.go:112`) |
| Close cuts the socket when its context ends | T1 `TestCloseEndsAHeldWrite`, T8 `TestCloseEndsAReconnectDial` |
| Each operation's return and status in each lifecycle state | `TestClientLifecycleOperationTable` (`:960`), 28 cells |
| An oversize reply is refused with `response_too_large` and the server's max payload | `TestIntegration_SubscribeForRequestsPublishesBeforeClassifyingResponseLimit` (`request_response_bounds_integration_test.go:76`) |

**Order.**

- This change lands before #169 (#184 comment 6124681941).
- With #93 (P10): the two share no Go file, and they edit the natsclient ledger row in different hunks. For the
  `transport-client` capability, #93 adds two requirements, and this change modifies two others and replaces a third,
  so either order merges. This change is smaller, so it should go first. The second to land merges `origin/main`.
- The developer runs on Opus, because the change edits Close's join and ownership paths and removes exported surface.

## Premises

| # | Premise | Measurement |
| --- | --- | --- |
| P1 | Nothing outside natsclient's tests calls `SetConnection` | `gopls references client.go:424:18`: 4 sites in `client_close_final_test.go`; `git grep` adds `request_response_bounds_integration_test.go:167`, `:189`; 0 in the four consumers (PR #188 body; reviewer re-ran it); at the pin only tests, in packages without ledger rows (§1, §6) |
| P2 | Without it, a client admits at most one connection; until Close, `conn` is nil or `dialled`, and `draining` is nil or `dialled` | writers of `conn` are `:434`, `:834`, `:1048`; admission refuses at `:822` (closing) and `:826` (installed) (§0); reviewer's refutation search found no other path |
| P3 | The code listed under "What goes" exists only for the swap | §2b row by row; with the swap tests skipped, the unit run leaves exactly 12 statements unreached: the method (9) and the three branches (§9; the reviewer reproduced it over two runs) |
| P4 | The code listed under "What stays" has another job; `:1047` is dead either way | §2c; `carriesClientHandlers` serves the test `dial` seam (r1 M1, Q2); `:1047`'s condition is true on every path, because `conn`'s writers refuse once Close has begun (`:430`, `:822`) |
| P5 | The 4 table cells and tests `:1399`, `:1442`, `:1529` prove only the swap; the other 28 cells do not use it | §4; `-skip` of the column runs and passes 28 cells; `gatedStream` has one use (`:1545`, test `:1529`) |
| P6 | During a real reconnect, the client's connection and a request's connection are the same `*nats.Conn`, which still reports its max payload; an oversize reply still fails with `nats.ErrMaxPayload`; `Client.MaxPayload` refuses | P2; probe P-184-1 (PR #188 comment 6125837700): nats.go v1.54.0, nats-server v2.15.0, go1.26.9 darwin/arm64, 50/50 `IsConnected=false MaxPayload=1024 publish=ErrMaxPayload respond=ErrMaxPayload` |
| P7 | natsclient stays above its 80% coverage floor | 89.2% merged now (3071 of 3444 statements); worst case after removal about 89.05% (§9; the integration profile is a local-only path, §9 r1 N3) |
| P8 | Removing a pin method changes nothing `task ledger:check` reports | the natsclient row is `adapt` (`admission-ledger.yaml:2361`); `pindiff` compares only `carry` rows (`main.go:85`-`:93`); `TestAdmissionLedger` checks the schema only (§5, review) |
| P9 | The special cases are SemEngine's; the pin has only the setter | pin `client.go:223`-`:231`; `owned_delivery.go` and `guardedConsumer.conn` are absent at the pin (§6 r1 N1). So Bounded port scope does not protect them |
| P10 | #93 touches none of the code this change removes | #93's file list; its natsclient files at `6f1d6b1` use none of the removed symbols (§7; review) |

## Owner questions

**Q1. Delete `TestIntegration_SubscribeForRequestsUsesSubscriptionConnectionForOversizeDiagnostic`
(`request_response_bounds_integration_test.go:144`)?** Recommendation: **delete it.**

When a request handler's answer is too big to send, natsclient replies with an error that names the server's size
limit. It reads that limit from the connection the request came in on. The test checks this by making the client say
"not connected" (`SetConnection(nil)`) while the request's connection is still up, so that the two disagree. Once
`SetConnection` is gone, a client has one connection, so the client and the request's connection can no longer
disagree. During a real reconnect they are the same connection (P2).

- The code keeps doing the right thing. It reads the request's connection (`request.go:471`), and the probe shows that
  during a real reconnect that connection still reports its limit and the oversize reply still fails the same way
  (P6).
- The refusal itself stays tested (`:76`).
- What the test would still catch is a future edit that reads the limit from the client instead. That edit only
  matters if a reply is too big at the same moment the connection is down: two things at once, not a routine event
  (#185).

Keeping the test on a real disconnect (O2b) needs no server outage and no new fixture: `client_close_held_test.go:189`
already drops a client on a live embedded server with `server.DisconnectClientByID(cid)`, and the client reconnects to
the same URL. What it does cost is ordering. The handler's reply has to happen inside the reconnect window, so the test
needs either a reconnect wait longer than the step from release to `Respond`, which is a timing assumption, or the
`netDial` seam that Q2 proposes to file for removal. Whether nats.go sends a reply published during the reconnect once
the client is back is not measured: P-184-1 sets `ReconnectWait(time.Hour)` and never reconnects. A third answer,
keeping the test by writing `client.conn = nil` directly, tests a state production cannot reach. That is what #186
removes, so I have not offered it.

**Q2. Remove the other test seams in natsclient in this change?** Recommendation: **no. File one port-refactor
issue for them, and keep #184 to `SetConnection`.**

`netDial`, `opHook`, `commitHook` and `connectWith`'s `dial` parameter let tests hold a dial open, pause at a status
write, or hand Connect a fake connection. `carriesClientHandlers`' remaining job only arises from that fake
connection. Like `SetConnection`, only tests use them. Unlike it, none is exported, so no consumer can call one
(accepted inventory §13 Q5, measured by the reviewer).

- Folding them in grows #188 from one removal to reworking about 24 test sites in 6 files. Several are concurrency
  tests that use a hook to force an interleaving, and each needs its own answer: drive it another way, or delete it as
  make-believe.
- #186's severities bind code a change adds (reviewer `:317`-`:319`). This change adds none of these, so they do not
  block #188.
- #169 waits on #188.

Folding them in would cost a larger design, more review rounds on the shutdown path, and a later #169. What it would
buy is one pull request for every test seam in natsclient, and the code loses them sooner.
