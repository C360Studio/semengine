# Tasks: natsfixture-restart-port

The developer runs on Opus. Read `design.md` (one page) and `proposal.md`. Run `task test:unit` after each code task,
`task test:integration` with `SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS` set after 2.2, and `task verify` on a clean tree
before the review stop. Choices the design does not spell out go in `notes/<task>.md`.

## 1. Record (orchestrator)

- [x] 1.1 Owner ruling and acceptance: #180 comment 6129968019 (the simple fix; no further design rounds).

## 2. Code (developer)

- [x] 2.1 Unit tests first, with the fixture's fakes (design table rows 1, 2, 3 and 5); show each fails on `main`'s
  code, then make them pass: Restart's one more stop, start and port read on a mapped-port failure with a live
  context, and the inspect detail on the mapped-port error (Start and Restart).
- [x] 2.2 The integration test (design table row 4): hold the next port on all addresses, Restart, dial `URL()`.
  Record its result on `main` and on the fix, five runs each, with the command, in `notes/2.2.md`.
- [x] 2.3 Text: the doc comments in `restart.go` and `fixture.go` (`maxAttempts`), and any line in `docs/testing.md`
  or the natsfixture row of `docs/admission-ledger.yaml` that says Restart never tries again. Check: `task
  docs:check`, `task ledger:check`.
- [x] 2.4 `task verify` on a clean tree at the last commit; record the commit and result in a PR comment.

## 3. Review and close

- [ ] 3.1 Hold: Claude's early check, then Codex's review of record on the 2.4 commit; fix findings and re-request
  after any content commit.
- [ ] 3.2 Archive the change and sync `openspec/specs/nats-fixture/spec.md` as the last content commit. Check: `task
  spec:check`.

Task 3.2 is completed by the archive commit. The archive check, CI before merging and `task merge:check -- 200` are
recorded on PR #200 after it; no task claims them.
