# Tasks: remove-setconnection

The developer runs on Opus: the change edits Close's join and ownership paths and removes exported surface. The change
adds no test. Each task names the existing test that proves what it leaves behind (design, "What a caller observes").
Run `task test:unit` after each code task, and `task verify` on a clean tree at the commit that ends section 3. Notes
on choices the design does not spell out go in `notes/<task>.md`, with a one-line PR comment linking it.

## 1. Record (orchestrator)

- [x] 1.1 Copy `inventory.md` (INVENTORY PASS r1, corrections folded), `design.md`, `proposal.md`, `tasks.md` and
  `specs/transport-client/spec.md` into `openspec/changes/remove-setconnection/`. Run `task spec:check`.
- [x] 1.2 Post probe P-184-1 on PR #188: `README.md`, `main.go`, `go.mod` and the output. Record the comment id in
  design premise P6.
- [x] 1.3 Hold: the owner's acceptance of this design and rulings on Q1 and Q2, on #184. Section 2 starts after them.
  Accepted 2026-10-12, Q1 delete, Q2 filed as #192 (#184 comment 6125883814).

## 2. Code (developer)

- [x] 2.1 Delete the tests that prove only the swap (inventory §4): `TestClientCloseEndsSubscriptionOnReplacedConnection`,
  `TestClientCloseEndsConsumerOnReplacedConnection`, `TestClientCloseAttributesConsumerToItsHandlesConnection`
  and `gatedStream`. Also delete the `SetConnection` column of `TestClientLifecycleOperationTable` and the scaffolding
  that serves only it: `installs`, the `other` connection, `runTableOp`'s parameter and case, `tableClient.logs` with
  its recording handler, and the comment at `:1063`. Delete the unread `tableClient.conn` and `tableClient.url`.
  Drop `t.Cleanup(dialled.Close)` (`:1065`): without the swap, the client's own Close (the test's first Close at `:1078`,
  or `closeInCleanup`'s) closes the dialled connection and cuts it when its budget ends; record that in
  `notes/2.1.md`. Test: `TestClientLifecycleOperationTable` runs 28 cells and passes.
- [x] 2.2 Delete `TestIntegration_SubscribeForRequestsUsesSubscriptionConnectionForOversizeDiagnostic`
  (`request_response_bounds_integration_test.go:144`; owner Q1). Test:
  `TestIntegration_SubscribeForRequestsPublishesBeforeClassifyingResponseLimit` passes under `task test:integration`.
- [x] 2.3 Remove `SetConnection` and the code that exists only for it (design, "What goes"). Test: `task build`, and
  every test in the design's table of observable behaviour passes under `task test:unit` and `task test:integration`.
- [x] 2.4 Rewrite the comments that name the swap (inventory §2d, and the `closedSignals` sentence at
  `client.go:195`-`:199`) so each states what the code does now. Check: `git grep -n SetConnection -- '*.go'` prints
  nothing.
- [x] 2.5 Show that the remaining tests guard the simplified paths. Run `task mutate:check` once per mutant, with
  `-test` naming the test and `-expect` or `-expect-text` naming the failure line it should print. Record each
  verdict in `notes/2.5.md`. Report a survivor or an inconclusive run as
  such.
  - (a) The subscription watcher counts its work finished before the delivery ends:
    `TestClientCloseJoinsSubscribeHandlerAfterForcedClose`.
  - (b) The consumer goroutine's closing arm does not cancel the handler context:
    `TestConsumeHandlerContextEndsWhenCloseBegins`.
  - (c) Cleanup skips the join of the dialled connection's event handlers:
    `TestClientCloseJoinsConnectionEventHandlers`.
  - (d) Close does not cut the dialled connection's socket: `TestCloseEndsAHeldWrite`.

  The design review ran (a) to (d) against the code before this change; each was a detection. Its wrong changes and
  expected text, to re-point at the simplified lines:
  - (a) `client.go:1505` `defer finish()` → `finish()`, `-test
    TestClientCloseJoinsSubscribeHandlerAfterForcedClose/Subscribe`, `-expect-text "Close's join completed before the
    handler returned"`.
  - (b) `owned_delivery.go:154` `cancelHandlers()` deleted, `-expect-text "the handler's context ending once Close
    began"`.
  - (c) `client.go:1040` `awaitHandlers := false && …`, `-expect-text "Close's join completed before the error handler
    returned"`.
  - (d) `client.go:971` `socket = nil`, `-expect-text "Close did not return within"`.

## 3. Text (developer)

- [x] 3.1 ADR-112: the dated note and the struck sentence in §4 (draft `docs/adr/112-natsclient-contract.md.diff`).
  Check: `task docs:check`.
- [x] 3.2 Ledger, natsclient row (draft `docs/admission-ledger.yaml.diff`):
  - item (9) of `contract` becomes a one-line removal record;
  - `SetConnection` leaves the IsConnected sentence at `:1706`;
  - a "pin → deleted" entry is added after `:1772`;
  - the `request_response_bounds` line map at `:2208` records that test's removal.
  Check: `task ledger:check`.
- [x] 3.3 Run `task verify` on a clean tree at this commit. `cover:check` keeps natsclient at 80% or more merged
  (design P7: about 89%). Record the commit and the run's result in a PR comment.

## 4. Review stop (Codex)

- [x] 4.1 Request Codex's review of record at the section 3 commit, naming that commit and its
  `task verify` run. The ledger is not Markdown, so the review covers the code, the ledger and the text together. Fix
  findings, and re-request after any content commit. Stop for the owner after three rounds with findings open.
  Codex: APPROVE at `4f91838` (PR #188 comment 6127066190), one NIT on this task's wording, fixed in the next commit.

## 5. Close

- [ ] 5.1 Archive the change and sync `openspec/specs/transport-client/spec.md` as the last content commit. Run
  `task spec:check`.

Task 5 is completed by the archive commit. The archive review, the CI check before merging and `task merge:check --
188` are recorded on PR #188 after the archive; no task claims them. The PR body carries `implemented-by: claude
(opus)` and `reviewed-by: codex`.
