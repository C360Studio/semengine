# Tasks: setup-04a-03-boot-service-storage-observability

Each task names its outcome and the gate that proves it. This change ports the storage observability service of
`service` (PR 5 of 7 in the split the owner accepted, #9 comment 6296023694). Tests come first. Record in
`notes/<task>.md` how each test fails on the base commit, or which mutant it detects. Review stops (Codex) come after
the design, after the port and after the docs. Each task ends only when `task verify` passes on a clean tree at its
commit (#171).

## 1. Design

- [x] 1.1 Shape sweep (`sweep.md`) and design (`design.md`) pass review and are accepted by the owner. Gate:
  `task spec:check` and `task docs:check`; each comment read back.

## 2. Port

- [x] 2.1 Tests first: the pin test files the design lists, adapted as the design says. Gate: the tests fail on the
  base (compile errors noted in `notes/2.1.md`).
- [x] 2.2 The three pin files with the module path rewritten, the design's surface and lifecycle, and `RegisterAll`
  updated. Gate: `go test -race ./service/` green and `task verify` green.
- [ ] 2.3 Sensitivity: the mutants the design names, each detected or reported as a survivor (`notes/2.3.md`).

## 3. Docs, ledger and follow-ups

- [ ] 3.1 `service/README.md`, `service/doc.go` and the admission ledger rows updated.
- [ ] 3.2 Spec deltas synced into `openspec/specs/` and `skip_specs` removed; archive as the last content commit.
