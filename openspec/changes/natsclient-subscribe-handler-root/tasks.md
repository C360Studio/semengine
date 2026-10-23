# Tasks: natsclient-subscribe-handler-root

Each task names its outcome and the gate that proves it. Each code task lands its tests and its implementation in one
commit; `notes/<task>.md` records how each new test fails on the base commit, or which mutant it detects. A task ends
only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory passes review (`inventory.md`, INVENTORY PASS, `4675084`).
- [ ] 1.2 Design, proposal and spec deltas written; `task spec:check` and `task docs:check` green.
  Hold: the Claude design review and the Codex design review both PASS this design, and the owner accepts it. Section 2
  starts after that.

## 2. Code

- [ ] 2.1 Merge `origin/main` (#376 landed) into the branch, without rebase; re-measure the cites design D6 gives at
  `46d3cd5` and record the new lines in `notes/2.1.md`. Gate: `task verify`.
- [ ] 2.2 `Subscribe` handler root (D1), with T1-T4 and the doc comments at `client.go:997`, `:1036` and
  `stream.go:455-460`, in one commit. Gate: T1-T4 fail on the base (`notes/2.2.md`); `task verify` green.
- [ ] 2.3 Message logger (D2, D4, D5), with T5, the comments at `message_logger.go:205`, `:250-256` and `:274-275`, and
  the test comments design D6 lists, in one commit. Gate: T5 fails on the base (`notes/2.3.md`); `task verify` green.
- [ ] 2.4 Sensitivity: M1-M6 run with `task mutate:check`, each reported as detected or as a survivor (`notes/2.4.md`).
- [ ] 2.5 Code review.
  Hold: the Codex code review PASSes 2.2-2.4 at a named commit with local `task verify`. Section 3 starts after it.

## 3. Docs and archive

- [ ] 3.1 `natsclient/README.md`, `service/README.md`, `docs/admission-ledger.yaml` and the ADR-113 cite with its Status
  amendment line, as design D6 lists. Gate: `task docs:check` and `task ledger:check`.
- [ ] 3.2 Spec deltas synced into `openspec/specs/`; the change archived as the last content commit. Gate:
  `task spec:check`.
