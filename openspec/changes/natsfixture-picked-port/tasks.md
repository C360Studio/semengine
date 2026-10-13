# Tasks: natsfixture-picked-port

The developer runs on Opus. The design is #209's diagnosis (comment 6151740502). Choices the design does not spell
out go in `notes/1.md`.

## 1. Code (developer)

- [x] 1.1 Failing test first: `TestStartPastHeldPorts` holds the ports after a running fixture's own on all addresses
  and starts a second fixture; `TestRestartURLDials` asserts `URL()` is unchanged across `Restart`. Both fail on
  `main`'s code (records in `notes/1.md`).
- [x] 1.2 The fix: the start phase gives Docker a host port the kernel picked (`freePort` in `fixture.go`). Both tests
  pass.
- [x] 1.3 Text: the comments in `fixture.go` and `restart.go`, `restart_test.go`, `docs/testing.md`, and
  `natsclient/integration_test.go` that said the restart port may move or cited `TestRestartPastAHeldPort`, which is
  removed. The header of `scripts/lint-test-ports.sh` says how `freePort` applies its rule. Check: `task lint`.
- [ ] 1.4 `task verify` passes on a clean tree at the last commit of section 1. At `2b36ba7` every step passed but
  `test:unit`, which failed only #202's `TestEscapedWriterDoesNotHoldTheLock` (`notes/1.md`).

## 2. Review and close

- [ ] 2.1 Hold: Codex's review of record on the commit whose `task verify` run is recorded on PR #210; fix findings,
  and re-request after any content commit.
- [ ] 2.2 Archive the change and sync `openspec/specs/nats-fixture/spec.md` as the last content commit. Check:
  `task spec:check`.

Task 2.2 is completed by the archive commit. The archive check, CI before merging and `task merge:check -- 210` are
recorded on PR #210 after it; no task claims them.
