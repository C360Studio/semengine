# Tasks: fix-422-census-in-contract

Light process, owner 2026-10-27: proposal, tasks and one spec delta; no design document; Codex reviews the code.
Tests come first: 2.1 is shown failing before its code, with the failure recorded in notes, and committed with the
code of 3.1, so every commit passes `task verify` on a clean tree (#171).

## 1. Proposal

- [x] 1.1 Proposal, tasks and the `harness-boundaries` delta committed. Gate: `task spec:check` and `task docs:check`.

## 2. Tests

- [x] 2.1 The allowlist guards and their sensitivity tests written in `internal/harness/contract`, reading files
  through `repoFiles` and planting violations with `writeTree`. Gate: with the allowlist emptied, the guard fails
  naming `processor/graph-ingest/consume.go` and `natsclient/stream.go`; the planted-caller cases fail when the check
  returns nil; the output of both is in `notes/2.1.md`. Committed with 3.1.

## 3. Code

- [x] 3.1 `natsclient/consumer_policy_callsite_test.go` leaves natsclient; every test it held is in `contract`
  (`go test -list '.*'` of the two packages together names each test the file held, except
  `TestParseProductionGoFilesIgnoresClaudeWorktrees`, which has no walker to test). Gate: `task test:unit` passes
  from a checkout with a populated `.worktrees/` (the primary checkout) and from this worktree; `task verify` passes
  on a clean tree.

## 4. Docs and rules

- [ ] 4.1 `AGENTS.md` and `.agents/rules-index.md`, one new row each in domain 3, row for row; `AGENTS.md` stays
  under 24,000 bytes. The `natsclient` row of `docs/admission-ledger.yaml` describes the allowlist. Gate:
  `TestAgentSetupParity` in `task verify`, `task ledger:check`, `task docs:check`.

## 5. Code review

- [ ] 5.1 Codex code review of record on the PR; `implemented-by:` and `reviewed-by:` in the PR body.

## 6. Archive

- [ ] 6.1 Spec sync and archive as the last content commit. Gate: `task spec:check`.
