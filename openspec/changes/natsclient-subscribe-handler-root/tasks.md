# Tasks: natsclient-subscribe-handler-root

Each task names its outcome and the gate that proves it. Each code task lands its tests and its implementation in one
commit; `notes/<task>.md` records how each new test fails on the base commit, or which mutant it detects. A task ends
only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory passes review (`inventory.md`, INVENTORY PASS, `4675084`).
- [x] 1.2 Design, proposal and spec deltas written; `task spec:check` and `task docs:check` green. Claude DESIGN REVIEW
  PASS at `6566fd4` (round 3), Codex DESIGN REVIEW PASS at `98b2a95` (PR #382 comment, 2026-10-23), owner go-ahead
  2026-10-23.

## 2. Code

- [ ] 2.1 Merge `origin/main` (#376 landed) into the branch, without rebase; re-measure the cites design D6 gives at
  `46d3cd5` and record the new lines in `notes/2.1.md`. Gate: `task verify`.
- [ ] 2.2 `Subscribe` handler root (D1), with T1-T4 and the doc comments at `client.go:997`, `:1036` and
  `stream.go:455-460`, in one commit. T4 publishes on the raw connection (design, T4 note). Gate: T1-T4 fail on the
  base (`notes/2.2.md`); `task verify` green.
- [ ] 2.3 Message logger (D2-D5), in one commit:
  - Start subscribes under its context, and the final check sits after the loop, before the retry and the
    "MessageLogger started" record.
  - The ended-context branch of `handleMessage` writes the Warn record and counts `dropped_messages` (served in
    `GetStatistics`).
  - It lands with T5 and T6, the comments at `message_logger.go:205`, `:250-256`, `:274-275` and `:800-803`,
    `service/doc.go:351`, and the test comments design D6 lists.
  - Gate: T5 and T6 fail on the base (`notes/2.3.md`); `task verify` green.
- [ ] 2.4 Sensitivity: M1-M8 run with `task mutate:check`, each reported as detected or as a survivor (`notes/2.4.md`).
- [ ] 2.5 Code review.
  Hold: the Codex code review PASSes 2.2-2.4 at a named commit with local `task verify`. Section 3 starts after it.

## 3. Docs and archive

- [ ] 3.1 `natsclient/README.md`, `service/README.md` (the Start text and the `/stats` table), `docs/admission-ledger.yaml`,
  and the ADR-113 `:47` cite (corrected in place, no Status line), as design D6 lists. Gate: `task docs:check` and
  `task ledger:check`.
- [ ] 3.2 Spec deltas (`transport-client`, `service-lifecycle`, `operator-surface`) synced into `openspec/specs/`; the
  change archived as the last content commit. Gate: `task spec:check`.
