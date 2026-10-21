# Tasks: setup-04a-03-boot-service-component-manager

Each task names its outcome and the gate that proves it. This change ports the component manager of `service` (PR 3
of 7 in the split the owner accepted, #9 comment 6296023694). Tests come first. Record in `notes/<task>.md` how each
test fails on the base commit, or which mutant it detects. Review stops (Codex) come after the port and after the
docs. Each task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Shape sweep (`sweep.md`) and design (`design.md`) pass review and are accepted by the owner, including the
  owner's reading of the ADR-113 question (per-component cancel). Gate: `task spec:check` and `task docs:check`; each
  comment read back.

## 2. Port

- [ ] 2.1 Tests first: the pin test files the design lists, adapted as the design says, plus the PR 3 tests cut from
  PR 2 (archive `notes/2.1.md` parts 3 and 5). Gate: the tests fail on the base (compile errors noted in
  `notes/2.1.md`).
- [ ] 2.2 The five pin files with the module path rewritten, the design's surface and lifecycle, `RegisterAll`
  updated, `projection.PredicateReconciler` restored. Gate: `go test -race ./service/ ./pkg/projection/` green and
  `task verify` green.
- [ ] 2.3 Sensitivity: the mutants the design names, each detected or reported as a survivor (`notes/2.3.md`).

## 3. Docs, ledger and follow-ups

- [ ] 3.1 `service/README.md`, `service/doc.go` and the admission ledger rows updated; adoption note on #196.
- [ ] 3.2 Spec deltas synced into `openspec/specs/` and `skip_specs` removed; archive as the last content commit.
