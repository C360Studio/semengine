# Tasks: remove-natsclient-test-seams

The developer runs on Opus. Rows are `inventory.md` §3 rows; line numbers are on `a5020db`, mapped through #188 by
inventory §8. Run `task test:unit` after each code task, and `task test:integration` after 2.1. Run `task verify` on a
clean tree before each review stop. Notes on choices the design does not spell out go in `notes/<task>.md`, with a
one-line PR comment linking them. If the owner answers Q1 or Q2 otherwise than the design recommends, the architect
revises tasks 2.2 and 2.3 before section 2 starts.

## 1. Record (orchestrator)

- [x] 1.1 Copy `.openspec.yaml` (`skip_specs: true`), `inventory.md`, `design.md`, `proposal.md` and `tasks.md` into
  `openspec/changes/remove-natsclient-test-seams/`. Run `task spec:check`.
- [x] 1.2 Hold: the owner's acceptance of this design and rulings on Q1 and Q2, on #192; and PR #188 merged into
  `main`. Section 2 starts after them, on a branch that has merged `origin/main`. Accepted with Q1 remove `commitHook`
  and Q2 keep `netDial` (#192 comment 6127721456); #188 merged as `993fd1f` (2026-10-12); `d231fc7` merges
  `origin/main` at `993fd1f`. Line numbers below stay on `a5020db`: the developer maps them on the merged head.

## 2. Code, part 1 (developer)

- [x] 2.1 Remove `opHook`.
  - Rows 3 and 14 assert no dial through `silentListener`'s `accepted` channel.
  - Row 17 asserts `Failures()` stays 15.
  - Rows 15 and 25 drop their dial assertions.
  - The lifecycle adapter stops counting; its doc comment says the suite's call counts do not cover Client (row 26).
  - Delete rows 5 and 7, and row 1's `held-before-connecting`.
  - Remove `opHook`, `recordOp` and its calls.

  Test: the rows above pass; row 17 passes under `task test:integration`.
- [x] 2.2 Remove `commitHook` (Q1). Delete rows 2 and 6, row 1's `held-before-failure-write`, and `runIfCommitFree`.
  Add a check to `TestClientCloseDropsLateDisconnect`: after the late disconnect, `Status()` is still Disconnected,
  because Close's cleanup has finished (spec `:176`-`:177`). Remove `commitHook`, `commitSeam` and its calls.
  - Test: `TestClientCloseDropsLateDisconnect` passes.
  - Show it can fail: run `task mutate:check` with `-expect-text` on `handleDisconnect` writing the status even once
    Close has begun (`if !m.closing`, `client.go:2195`). Record the verdict in `notes/2.2.md`.
- [x] 2.3 Remove `connectWith`'s `dial` and `carriesClientHandlers` (Q2: `netDial` stays).
  - Move rows 1 (`dial-error`, `canceled-dial`), 11 and 13 onto `netDial` (design, "Recommendation"). Row 13's
    comment no longer claims the Connecting-write window it stops testing.
  - Move `connectScripted` onto `netDial`, so T2 to T7 keep their bubble. Reword T8's comment
    (`subscribe_registration_test.go:474`-`:478`) for the path it now takes (design P7).
  - Delete rows 10 and 12, `newConnectCandidate`, and the loser-dial wrappers.
  - Fold `connectWith` into `Connect`. `discardCandidate` and `cleanup` always wait for the closed handler.

  Test: rows 1, 11, 13 and 18-24 pass. Show they can fail: run `task mutate:check` with `-expect-text` once per wrong
  change, and record each verdict in `notes/2.3.md`:
  - Connect's admission stops refusing once Close has begun (`case m.closing:`, `client.go:822`): row 11;
  - `connectFailed` stops checking for an installed connection (`|| m.conn != nil`, `client.go:869`): row 1's
    `dial-error`.

  Report a survivor or an inconclusive run as such.

## 3. Review stop (Codex), part 1

- [x] 3.1 Run `task verify` on a clean tree at the commit that ends 2.3. Request Codex's review of that commit on PR
  #194, still a draft, naming the commit and the run. Fix findings and re-request after any content commit. Section 4
  starts once Codex records no open finding; stop for the owner after three rounds with findings open. Codex APPROVE
  at `1b75749`, no findings (PR #194 comment 6135230181); request 6135124209.

## 4. Code, part 2 (developer)

- [x] 4.1 Remove `subscribeWith`'s `subscribe`. Delete row 33's `core` subtest and `fakeInvalidSubscription`. Fold
  `subscribeWith` into `Subscribe`; remove `subscribeFunc` and `nativeSubscribe`. Leave `nativeSubscription` as it
  is. Test: `TestClientRefusesLateDeliveryAfterRecordedEnd` passes.
  - Hold: #169's merge, only if #169 has a claim pull request when this task starts (design, "Order"). It did not
    apply: when this task started, #169 had no claim pull request (only #194 mentions it).
- [x] 4.2 Comments.
  - The comment at `client_close_final_test.go:947` describes the struct `tableClient`; `buildTableClient` builds it.
  - Rewrite every comment that names a removed hook or `connectWith`, including `netDial`'s own
    (`client.go:100`-`:102`) if it does.

  Check: this prints nothing:

  ```bash
  git grep -n -E 'opHook|commitHook|commitSeam|recordOp|connectWith|carriesClientHandlers' -- '*.go'
  git grep -n -E 'subscribeWith|subscribeFunc|nativeSubscribe\b|runIfCommitFree|tableState' -- '*.go'
  ```

  Its first check also matches `metric/`'s own `opHook` and `recordOp` (#207, outside this change); limited to
  `natsclient/`, both checks print nothing (`notes/4.2.md`).

## 5. Text (developer)

- [x] 5.1 Ledger, natsclient row. Each sentence that names a deleted test or a removed hook records the removal
  (`docs/admission-ledger.yaml:1625`-`:1638`, `:2001`, `:2222`, `:2271`, `:2296` on `a5020db`). Check:
  `task ledger:check`.
- [ ] 5.2 Run `task verify` on a clean tree at this commit. `cover:check` keeps natsclient at its floor. Record the
  commit and the run's result in a PR comment.

## 6. Review stop (Codex), part 2

- [ ] 6.1 Mark PR #194 ready and request Codex's review of record at the section 5 commit, naming that commit and its
  `task verify` run. Fix findings, and re-request after any content commit. Stop for the owner after three rounds with
  findings open.

## 7. Close

- [ ] 7.1 Archive the change (no spec to sync) as the last content commit. Run `task spec:check`.
