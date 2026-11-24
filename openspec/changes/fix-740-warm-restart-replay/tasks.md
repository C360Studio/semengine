# Tasks: fix-740-warm-restart-replay

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`. One pull request (#747): `Closes #740`, `Closes #741`.

## 1. Proposal and design

- [ ] 1.1 Inventory and design: INVENTORY PASS, then pre-owner design review passes; the owner fills `Approved-by:`
  and rules on design questions 1 to 6.
  Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 Post probe740 on PR #747: `main.go`, `run.sh`, `probes.sh`, the commands, the versions (nats-server 2.15.0
  from `internal/harness/natsfixture/nats-image`, nats.go v1.55.0, Docker Desktop 29.9.3) and every output the design
  cites as P1 to P6. Accept: the comment opens on another machine and each P-number in `design.md` names its run.

## 2. The failing test

- [ ] 2.1 `TestSmoke_WarmRestartOverALargeStore` in `internal/smoke`: seed the composed stack
  (`internal/smoke/stack_integration_test.go:122`) through graph-ingest with at least 66 MB in `ENTITY_STATES`
  (8,000 entities of about 8 KiB; P2 measured 2 closes there), stop it, start it again on the same server with its
  default `max_pending`, and assert: `Start` returns nil; every follower envelope and the websocket view report the
  snapshot applied; the server lists no connection of the process closed as a slow consumer (the account
  connection listing `natsfixture` already reads, `internal/harness/natsfixture/shared.go:34`), or, if closed
  connections are not listed there, the main Client reports no connection loss from `Connect` to ready. Record the
  run time and which observation was used.
  Premises: `internal/smoke/smoke_integration_test.go:824` (the restart shape), inventory § 1.2, § 1.3.
  Accept: shown to fail on the base commit for the right reason (a slow-consumer close or a failed `Start`). If 66 MB
  passes on the base, raise the size along P2 (132 MB, then 230 MB) and record each run.

## 3. natsclient: a sibling Client

- [ ] 3.1 Tests first, shown failing, in `natsclient`: `NewSibling` returns a Client that is not connected; after
  `Connect` it reaches a password-protected fixture server with the parent's credentials and carries the parent's
  name; it registers nothing on the parent's metrics registry; a connection loss on it never runs the parent's
  connection-lost callback; it is refused, with `nats.ErrConnectionClosed`, once the parent's `Close` has begun.
  Premises: `natsclient/client.go:398-424` (the settings), `natsclient/options.go:43-62`, `:143-157`,
  `docs/admission-ledger.yaml:1175-1178`.
- [ ] 3.2 Implement `NewSibling` and its doc comment, which promises no timing. Accept: 3.1 passes; `task verify`.

## 4. Each whole-bucket watch on its own connection

- [ ] 4.1 `graph/follower`: tests first, shown failing: while `Run` runs, the server lists one connection more than
  before `Open`; after `Run` returns, it lists none more; a connection that cannot be opened within the startup wait
  declares the feed lost. Implement: `Run` opens the sibling inside the wait of `follower.go:163`, watches on it,
  closes it before returning; status reads stay on `Config.Client`. Update the package doc's Limits
  (`follower.go:14-23`), naming nats.go v1.55.0 and saying the stall time is nats.go's, not promised (#741 item 2).
  Accept: `task verify`; `TestFollowerRunReturnsAndLeavesNothing` still passes.
- [ ] 4.2 `output/websocket` (`state.go:113`) and `processor/graph-query` (`community.go:87`): each view's reader is
  opened on a sibling that the view's run closes when it ends, a reopened view included. Tests: the connection is
  gone after `Stop`. Accept: `task verify`.
- [ ] 4.3 `processor/graph-ingest`: the boot sweep (`poison_inventory.go:272`) runs on a sibling closed before
  `Start` returns, on success and on every failure path. Accept: `task verify`; the lifecycle suite still passes.
- [ ] 4.4 `graph/embedding`: `ScanFailed` (`storage.go:570`), the Worker (`worker.go:315`) and the vector cache
  (`vector_cache.go:91`) each watch on a sibling closed when the watch ends. Accept: `task verify`.
- [ ] 4.5 2.1 passes. Accept: `task verify` and `task test:integration` for `internal/smoke`.

## 5. Docs and spec

- [ ] 5.1 `docs/boot.md`: how many connections a tier 0 and a tier 1 stack open and when; that no `max_pending`
  setting is needed for a warm restart; that a product's own whole-bucket watch should run on a sibling. Accept:
  `task docs:check`.
- [ ] 5.2 `openspec/specs/composed-stack/spec.md:8`: state what is covered, without the issue reference (#741 item 1).
- [ ] 5.3 ADR-108 Decision 4 gains the amendment line, if the owner rules yes on question 6.
- [ ] 5.4 File the follow-up for two simultaneous message-logger KV watches on `ENTITY_STATES` on the shared
  connection (`service/message_logger_kv_watch.go:138`), linked from the PR without a closing keyword.
- [ ] 5.5 The deltas under `specs/` match the code and the test names. Accept: `task spec:check`.

## 6. Close

- [ ] 6.1 Archive the change and sync the specs as the last content commit. Accept: `task spec:check`.
