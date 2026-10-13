# Design: remove-natsclient-test-seams, revision 2

Base: `main` `a5020db` (claim commit `b28c1f2`, PR #194, issue #192). Rests on `inventory.md` (INVENTORY PASS on r2;
r3 folds round 2's two notes). Line numbers are on `a5020db`; inventory §8 maps those PR #188 moves. "Row n" is
inventory §3's row. Revision 2 folds design review r1 (F1 to F7, N1, N2).

## The problem

`natsclient.Client` carries seven seams only tests use (inventory §2): the fields `netDial`, `opHook` and `commitHook`;
the parameters `connectWith`'s `dial`, `subscribeWith`'s `subscribe` and `newSocketDialer`'s `dial`; and
`carriesClientHandlers`, needed only for connections a test builds. 33 tests reach them. Since #186 (owner,
2026-10-12), production code carries no hook whose only caller is a test (`.agents/contracts/semengine-developer.md:110`-`:114`).
Under #185 ("edge cases that are mostly make believe irl"), a race counts only with a routine trigger. One that needs a
library's timing, or a call sequence only a test makes, is not required work.

## Recommendation

Remove five seams: `opHook`, `commitHook` (Q1), `connectWith`'s `dial`, `subscribeWith`'s `subscribe` and
`carriesClientHandlers` (P8). That is about 47 production lines (inventory §2), and `connectWith` and `subscribeWith`
fold into `Connect` and `Subscribe`. Keep two: `netDial` and `newSocketDialer`'s `dial` (Q2). `nativeSubscription`,
carried from the pin and optional under the protocol's Bounded port scope, is not touched.

| What happens | Rows |
| --- | --- |
| Unchanged: the lifecycle adapter stops counting operations | 4, 27-32 |
| Unchanged: `netDial` stays | 8, 9, 16; `socket_test.go:16` |
| `silentListener`'s `accepted` channel shows that no dial was made, as `client_connect_test.go:141`-`:158` does | 3, 14 |
| The dial count goes: 15 and 25 keep their error and connection checks; 17 checks `Failures()` stays 15; 26's adapter reports no `Calls`, so the suite compares nil with nil, as its doc comment says | 15, 17, 25, 26 |
| `netDial` holds the first Connect's dial while a second Connect dials the embedded server. Then it fails that dial or ends its context (row 1's `dial-error`, `canceled-dial`), or dials for real, and the first returns `ErrAlreadyStarted` (row 13). Row 13 then holds the loser after its Connecting write, not before, so it no longer proves that write's guard (`client.go:762`). It also stops waiting for the rejected connection's closed handler | 1, 13 |
| `netDial` holds the dial until Close has returned, then dials the embedded server; Connect returns `nats.ErrConnectionClosed` and installs nothing (P5) | 11 |
| `connectScripted` sets `netDial` to hand over the pipe's client end; T2 to T7 keep their bubble, and T8 stays on real time (P7) | 18-24 |
| Deleted | 5, 7, 10, 12; row 1's `held-before-connecting`; 33's `core` subtest; per Q1, 2, 6 and row 1's `held-before-failure-write` |

**Overlapping Connects** (rows 1, 5, 7, 13). Nothing in this tree runs two overlapping `Connect` calls (P2), so no
test of them is required work.

- Rows 1 (its two dial subtests) and 13 stay, not because of a trigger. `netDial`, kept for Q2's reasons, drives them
  at no production cost, and they keep "A losing Connect leaves the winner's status" (`:187`) tested without a spec
  delta.
- Rows 5 and 7, and row 1's `held-before-connecting`, go. `netDial` cannot hold the points 7 and that subtest hold:
  inside a discarded connection's closed handler, and before the Connecting write. No scenario names 5's property.

**Other deletions.** 10 needs a dial to succeed in the instant after the context ended, and a socket dialled after
the cut is closed (P6). Every Connect to an embedded server proves 12 (e.g. `client_lifecycle_test.go:288`). 33's
`core` runs on a fake subscription; the refusal is shared code (`owned_delivery.go:41`-`:48`), still proved by
`ack-explicit` and `ack-none`.

**Left to review, not required work:** the Connecting write's guard (`client.go:762`); `discardCandidate`'s wait for a
discarded connection's closed handler (`:893`-`:897`), which only row 7 observes and row 11's path reaches unobserved
(a late run writes no status once Close has begun, `ownsStatusLocked`, `:2184`); and, per Q1, the one-step check and
write and the monitor's ownership check.

Test helpers whose last user goes are deleted. The comment at `client_close_final_test.go:947` names `tableState`,
which never existed (from `deaafd4`); it should describe `tableClient`, the struct below it.

## What a caller observes, and the test that proves it

Production behaviour does not change: in production each removed hook is nil or its default (P1). Every
`transport-client` scenario that a moved or deleted test proved keeps a test:

| Scenario (`openspec/specs/transport-client/spec.md`) | Test after this change |
| --- | --- |
| Nil Connect context (`:90`) | row 14, on `silentListener` |
| Connect after Close (`:157`): no dial | row 3, on `silentListener` |
| Only Close changes the status once Close begins (`:180`) | the table's `closing` and `closed` cells; row 4; row 28, which gains a check that `Status()` is still Disconnected after a disconnect handled once Close's cleanup has finished (`:176`-`:177`). Its health-monitor clause loses its only test (row 2), per Q1 |
| A losing Connect leaves the winner's status (`:187`) | rows 1 and 13, on `netDial` |
| Subscribe returns after the server has read the SUB (`:357`-`:400`) | rows 18-24 |
| Close while a reconnect is dialling (`:79`); Connect is bounded by its context (`:402`) | rows 8 and 16, unchanged |
| Late delivery on an acknowledged consumer (`:200`) | row 33's `ack-explicit` and `ack-none` |

There is no spec delta (`.openspec.yaml`: `skip_specs: true`). `grep -rn -E
'opHook|commitHook|netDial|connectWith|carriesClientHandlers|subscribeWith' openspec/specs` finds nothing. The design
adds no statement about how nats.go behaves: P5 to P8 are about natsclient's own code, and a moved test shows each.

## Premises

- **P1.** Only tests set or replace a hook: #192's 24 sites, re-measured (inventory, base paragraph and §2).
- **P2.** No production code in this tree calls `Client.Connect`. `git grep -n -E '\.Connect\(' -- '*.go'
  ':!*_test.go'` finds 7 lines: doc examples calling it once (`natsclient/doc.go:40`, `:212`, `:322`, `:492`),
  `nats.Connect` (`internal/harness/natsfixture/deps.go:105`), and other types' doc examples (`pkg/errs/doc.go:285`,
  `pkg/retry/doc.go:24`). Neither consumer repositories (`docs/inventory-scope.md`) nor the pin's callers were read.
- **P3.** On correct code `runIfCommitFree` never runs its function, and a change that splits the check from the write
  fails both of its tests, 3 of 3 (inventory §4).
- **P4.** No test address portably holds a TCP dial open. The archived design says why
  (`openspec/changes/archive/2026-10-11-natsclient-contract/design.md:254`-`:255`: "whether an address is unroutable
  depends on the host"), and probe P25 held a dial open only at a blackhole address on darwin (PR #177 comment
  6110634199).
- **P5.** Close cuts only the installed connection's dialer (`client.go:966`-`:972`). `m.socket` is set at admission
  (`:837`), so a Close during a Connect's dial does not end that dial. Read, not run; row 11's move shows it.
- **P6.** A socket dialled after the cut is closed and refused (`socket.go:41`-`:42`).
- **P7.** `socketDialer.Dial` hands its address to `netDial` and keeps what it returns (`socket.go:31`-`:45`), and
  `Connect` sets `nats.SkipHostLookup` itself (`client.go:776`). So the pipe becomes `socketDialer`'s connection, which
  today `connectScripted`'s own `SetCustomDialer` bypasses. Connect's cut (`:777`) and Close's cut (`:1029`,
  `socket.go:62`-`:72`) then close the pipe, so T7 and T8 end through the cut, as production does. T8's comment
  (`subscribe_registration_test.go:474`-`:478`) is reworded to match. Read, not run; rows 18-24 passing shows it.
- **P8.** Every connection the client builds carries its closed handler (`client.go:665`). Once rows 10-12 stop
  handing `Connect` a prebuilt connection, every connection that reaches `discardCandidate` or `cleanup` has one.

## Order

PR #188 (head `4f91838`) lands first, and task 1.2 holds on it. #169 has no pull request or claim (2026-10-12). It
edits `subscribeOwned`, the code task 4.1 changes.

This change lands first unless #169 is claimed before task 4.1 starts; then 4.1 waits for #169's merge (its
`Hold:`), and any `connectScripted` caller #169 adds moves onto `netDial` when the branch merges it. The developer
runs on Opus: the moved tests are concurrency tests of Connect's admission and of Close.

## Owner questions

**Q1. Remove `commitHook`, and delete its three tests (rows 2 and 6, row 1's `held-before-failure-write`)?**
Recommendation: **yes.**

- **What they do.** Each holds a status writer between "do I still own the status?" and its write, and lets Close run
  there. Row 6 and the row 1 subtest fail only when a change splits the check from the write (P3). Row 2 holds the
  health monitor after its health check while Close finishes.
- **Why delete.** Each needs a broker event or a health tick to land within a few instructions of Close starting, and
  would catch only a wrong `Status()` after Close (#185: a library's timing). Each check and write sit in one locked
  block of about ten lines (`client.go:868`-`:879`, `:2190`-`:2199`, `:2207`-`:2215`, `:2396`-`:2404`). The hook is
  14 production lines that exist only for tests (#186).
- **The cost.** A change that splits one of those blocks, or drops the monitor's ownership check, is caught by review
  only. **The other answer, keep:** 14 lines and `runIfCommitFree` stay, under a #186 waiver for one field.

**Q2. Keep `netDial`, a field only tests set?** Recommendation: **keep it, and waive #186 for this one field.**

- **Why.** It is the only way to test that Close and Connect end a dial in progress (T4, T8; P4), which has a routine
  trigger: NATS unreachable at boot or shutdown, so the dial gets no answer. It also keeps the six bubble tests (T2
  to T7) in fake time once `connectWith`'s `dial` goes.
- **The cost of keeping.** 9 lines (5 in `client.go`, 4 for `newSocketDialer`'s `dial`): a test-only field, against
  #186.
- **The cost of removing.** T8, row 9, T4's two dial subtests, and rows 1 and 13 go, with a spec delta removing `:187`.
  The six bubble tests move to real time, where T2 and T6 each wait 5 s (untried).

If the owner answers either question otherwise, the architect revises the Recommendation and tasks 2.2 and 2.3 before
section 2 starts.
