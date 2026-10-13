# remove-natsclient-test-seams

Status: design revision 2, draft for owner review (DESIGN PASS; review r1's findings folded). Issue #192; claim
PR #194. Rests on `inventory.md` (INVENTORY PASS on r2, round 2's citation notes folded as r3) and `design.md`
revision 2.

## Why

`natsclient.Client` carries seven seams that only tests set or replace: `netDial`, `opHook`, `commitHook`,
`connectWith`'s `dial`, `subscribeWith`'s `subscribe`, `newSocketDialer`'s `dial`, and `carriesClientHandlers`, which
serves connections a test builds. 33 tests reach them. The owner's rule from 2026-10-12 (#186) is that production code
carries no hook whose only caller is a test. #192, filed on the owner's word, asks that each hook be removed once
nothing uses it, after each test is driven another way or deleted when its ordering has no real-world trigger (#185).

## What Changes

- **Removed from production** (about 47 lines):
  - `opHook` and `recordOp`;
  - `commitHook` and `commitSeam` (owner question Q1);
  - `connectWith`'s `dial`, with `connectWith` folded into `Connect`;
  - `carriesClientHandlers`;
  - `subscribeWith`'s `subscribe`, with `subscribeFunc` and `nativeSubscribe`, and `subscribeWith` folded into
    `Subscribe`.
- **Kept:** `netDial` and `newSocketDialer`'s `dial`, the only way to hold a TCP dial open (owner question Q2). Five
  seams go and two stay.
- **Tests** (design, "Recommendation"):
  - ten move onto `netDial`, and two onto `silentListener`;
  - one counts failures instead of dials;
  - three drop a dial assertion or the suite's call counts;
  - ten are unchanged;
  - deleted: four tests and two subtests, and, per Q1, two more tests and one subtest;
  - `TestClientCloseDropsLateDisconnect` gains a check that the status is still Disconnected.
  - Tests of two overlapping `Connect` calls are not required work, since nothing in the tree makes them (design
    P2). Rows 1 and 13 stay only because `netDial` drives them at no production cost.
- **Comment:** `client_close_final_test.go:947` names `tableState`, which never existed; it should describe the struct
  `tableClient`.
- **Ledger:** the natsclient row's sentences that name a deleted test or a removed hook record the removal.
- **Unchanged:** the spec, ADR-112, the README and `doc.go`. None names a removed hook, and every scenario keeps a test
  except the health-monitor clause the design names.

## Capabilities

No capability changes. `transport-client`'s requirements stay as written. Each scenario a moved or deleted test
proved keeps a test, except the health-monitor clause of "Only Close changes the status once Close begins", which
becomes review only (design, "What a caller observes"; Q1).

## Impact

- **Code pull request.** Codex reviews it. The developer runs on Opus: the moved tests are concurrency tests of
  Connect's admission and of Close.
- **What callers see.** Nothing: every removed hook is unexported and is nil or its default in production.
- **Order.** After PR #188 (head `4f91838`); inventory §8 maps its line moves. #169 has no pull request or claim. This
  change lands first unless #169 is claimed before task 4.1, which then waits for it.
- **Review stops.** Codex reviews after task 2.3 and again before the archive.
