# Tasks: setup-04a-03-boot-service-message-logger

Each task names its outcome and the gate that proves it. This change ports the message logger of `service` (PR 4 of 7
in the split the owner accepted, #9 comment 6296023694). Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit, or which mutant it detects. Review stops (Codex) come after the design, after the port and
after the docs. Each task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Shape sweep (`sweep.md`) and design (`design.md`) pass review and are accepted by the owner, including the
  #280 item 2 decision. Gate: `task spec:check` and `task docs:check`; each comment read back.

## 2. Port

- [x] 2.1 Tests first: the pin test files the design lists, adapted as the design says, and the subject-grammar tests in
  `internal/flowgraph` (design D3, U-h; #368's reproduction and the `FuzzValidSubject` seed corpus among them). Gate:
  the tests fail on the base (compile errors and #368's failing assertions noted in `notes/2.1.md`).
- [x] 2.2 The three pin files with the module path rewritten, the design's surface and lifecycle, and `RegisterAll`
  updated; `flowgraph.ValidSubject` added and the whitespace rule in the shared grammar (design D3; the pull request
  body carries `Closes #368`). Gate: `go test -race ./service/ ./internal/flowgraph/ ./composition/` green and `task
  verify` green.
- [x] 2.3 Sensitivity: the mutants the design names, each detected or reported as a survivor (`notes/2.3.md`).

## 3. Docs, ledger and follow-ups

- [x] 3.1 `service/README.md`, `service/doc.go`, `internal/flowgraph/README.md`, `internal/flowgraph/doc.go` and the
  admission ledger rows (`service`, `internal/flowgraph`) updated; #280 commented with the item 2 outcome; the D3
  adoption sweep filed as one tracking issue and linked from the PR.
- [x] 3.2 Spec deltas synced into `openspec/specs/` and `skip_specs` removed; archive as the last content commit.
