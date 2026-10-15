# Tasks: simplify-04a-05-prochost-helper-exit

Each task names the outcome and the gate that proves it. Tests come first and are shown failing on the base commit
before any production line changes; record the failing output in `notes/<task>.md`.

## 1. Design

- [ ] 1.1 Inventory, proposal, design and spec delta reviewed (inventory review, pre-owner design review) and accepted
  by the owner on #208. Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 The probes the inventory cites (P1-P3: source, commands, go1.26.9 darwin/arm64, output) are posted as a
  comment on PR #264, and `inventory.md` cites the comment's URL. Gate: the URL opens.

## 2. Failing tests

- [ ] 2.1 `internal/harness/prochost/prochost_test.go`: the table-driven test of design D1, with four rows (killed,
  interrupted, returned, panicked):
  - A helper-parent starts a grandchild helper and records its pid, start identity and stderr file.
  - The test confirms the grandchild is running before it ends the helper-parent.
  - It then asserts within `failureBound` that the grandchild's pid is gone or names a process with another start
    identity, and that its stderr names the reason.
  - The helper-parent's temporary files live under the outer test's temporary directory, and the outer test's
    cleanup kills a grandchild whose start identity still matches, so no run leaves a process or a directory behind.
  - No `time.Sleep`; waits use `probe.Await`.

  Gate: every row fails on the base commit, the grandchild still running. After that run, `ps` shows no
  `TestHelperProcess` child left. Record both in `notes/2.1.md`.
- [ ] 2.2 The early-exit guard (design D1) is shown able to fail. Use `task mutate:check` with a mutant of the
  implementation from 3.1 that leaves the helper's stdin as the null device. It should fail
  `TestStartedHelperHasItsOwnGroupAndCapturedOutput` on its exit-status check. Gate: the mutant is detected; record
  the command and verdict in `notes/2.2.md`.

## 3. Implementation

- [ ] 3.1 `prochost.Start` and `prochost.Helper`: a helper exits on its own once its test process has ended, writing
  the reason to its stderr and exiting non-zero. The process group, `Signal`, `Pause`, `Resume`, `Kill`, `Wait`,
  `Alive` and the cleanup are unchanged. Gate: 2.1 passes; `task test:unit` and `task test:repeat`; `go test -tags
  integration -race -run Settlement ./processor/graph-ingest/` passes unchanged.

## 4. Documentation

- [ ] 4.1 The `prochost` doc comment (`prochost.go:1-12`) and `docs/testing.md` (the `prochost` paragraph, `:114-119`):
  - a helper ends when its test process ends, naming #208 as the failure this code exists for;
  - stdin is reserved;
  - a paused helper is not promised;
  - the SIGTERM park is explained without Go's deadlock detector (design D4).

  Gate: `task docs:check`.
- [ ] 4.2 File every defect this change found and did not fix (protocol File step), linked from the PR body without a
  closing keyword. Gate: every such issue number appears in the PR body's follow-up line, or the body says none.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-05-prochost-helper-exit`) as the last content commit. Gate:
  `task spec:check`.
