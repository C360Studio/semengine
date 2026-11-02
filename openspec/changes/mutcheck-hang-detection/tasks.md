# Tasks: mutcheck-hang-detection

Each step ends when `task verify` passes on a clean tree at its commit (#171). One PR; closes #503.

## 1. Design

- [ ] 1.1 Design track (`design.md`); owner ruling on #503 (comment 6587993650).

## 2. Harness

- [ ] 2.1 `internal/harness/mutcheck`: a timed-out mutant run reads as a detection by hang under the design's conditions;
  a bounded run stays inconclusive; the report names the kind. Replayed event logs for the new reading and the refused
  cases, each shown able to fail.

## 3. Spec and docs

- [ ] 3.1 `openspec/specs/mutation-check/spec.md` delta applied; `docs/testing.md` verdict description updated.
