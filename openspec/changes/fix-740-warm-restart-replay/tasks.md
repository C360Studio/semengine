# Tasks: fix-740-warm-restart-replay

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`. One pull request (#747): `Closes #740`, `Closes #741`. Every stop test below checks the same
three things: the stop returns what it returns today, nothing is logged at Error, and no natsclient goroutine is
left: `natsfixture.AwaitNoStacks` over the sibling's frames (`(*Client).join`, `awaitClosedHandler`, `runPoller`),
compared with the count before the sibling was opened, since the test's own parent Client runs the same frames. Each
task's notes record the context the sibling is
closed under and why it meets the contract's rule on terminal, bounded detachment.

## 1. Proposal and design

- [x] 1.1 Inventory and design: INVENTORY PASS, then pre-owner design review passes; the owner fills `Approved-by:`.
  The owner's rulings on the six design questions are recorded on #740 (comment 7136495793).
- [x] 1.2 Post probe740 on PR #747: `main.go`, `run.sh`, `runnet.sh`, `probes.sh` and every file under `out/`
  (one per run, named by the design's P-numbers, with `out/env.txt` for the versions). Accept: the comment opens on
  another machine, and each P-number in `design.md` names a file there.
  Posted: PR #747 comments 7137819740 (scripts) and 7137820382 (outputs).

## 2. The failing test

- [x] 2.1 `TestSmoke_WarmRestartOverALargeStore` in `internal/smoke`: seed the composed stack
  (`internal/smoke/stack_integration_test.go:122`) through graph-ingest with at least 66 MB in `ENTITY_STATES`
  (8,000 entities of about 8 KiB), stop it, start it again on the same server with its default `max_pending`, and
  assert: `Start` returns nil; every follower envelope and the websocket view report the snapshot applied; and the
  account's `slow_consumers` count in `$SYS.REQ.ACCOUNT.PING.STATZ` (the subject `natsfixture` already reads,
  `internal/harness/natsfixture/shared.go:31`, `:205`) is the same after the restart reached ready as just before
  it began. P7 showed that count rising once per server close at 2.15.0, and showed the account connection listing
  carries no closed connections, so that listing is not used. The count is the account's, so the test reads it
  before and after the restart rather than against zero: a shared server keeps earlier tests' counts.
  Premises: `internal/smoke/smoke_integration_test.go:824` (the restart shape), inventory § 1.2, § 1.3.
  Accept: shown to fail for the right reason (a rise in the count, or a failed `Start`) both locally and on Linux
  CI: push the test alone, before any fix, and record the failed `Integration` run's URL in `notes/2.1.md`. P2net
  measured 1 close at 66 MB inside the Docker network against 2 to 3 through the macOS port forward; if 66 MB passes
  on CI, raise the size (132 MB, then 230 MB), push again, and record each run.

## 3. natsclient: a sibling Client

- [ ] 3.1 Tests first, shown failing, in `natsclient`: `NewSibling() (*Client, error)` returns a Client that is not
  connected; after `Connect` it reaches a password-protected fixture server with the parent's credentials, and the
  server lists it under the parent's name. In-package, after the sibling connected: no metrics poller runs for it
  (`AwaitNoStacks` over `startMetricsPoller`'s goroutine, `natsclient/client.go:617`), it holds no metrics object, no
  connection-lost callback and no connection-loss timeout, and the parent's registry holds only the parent's
  series. The parent's `Close` leaves the sibling connected. `NewSibling` after the parent's `Close` began returns
  an error matching `nats.ErrConnectionClosed`. A permission refusal on the sibling is logged and not counted
  (`natsclient/client.go:1903-1906`).
  Premises: `natsclient/client.go:398-424` (the settings), `natsclient/options.go:43-62`, `:143-157`,
  `docs/admission-ledger.yaml:1175-1178`.
- [ ] 3.2 Implement `NewSibling` and its doc comment, which promises no timing and says the parent's `Close` does
  not close a sibling. Accept: 3.1 passes; `task verify`.

## 4. Each boot-time whole-bucket reader on its own connection

- [ ] 4.1 `graph/follower`: tests first, shown failing: while `Run` runs, the server lists one connection more than
  before `Open`; a clean stop (the stop checks above) leaves none more and `Run` returns nil, so graph-index logs no
  Warn (`processor/graph-index/component.go:273-275`); a broker stopped after `Open` and before `Run` (an own
  server, `// Own server: the test stops the broker`) ends `Run`'s connection wait in a lost feed, asserted by the
  lost-feed refusal (`follower.go:413`), not the
  initial-build one (`:433`). Implement: `Run`
  waits for the sibling to connect with `Config.StartupAttempts` and `Config.StartupInterval`, a wait of its own
  beside `Open`'s bucket wait at `follower.go:163`; watches on it; closes it before returning. Status reads stay on
  `Config.Client`. Update the package doc's Limits (`follower.go:14-23`), naming nats.go v1.55.0 and saying the
  stall time is nats.go's, not promised (#741 item 2).
  Accept: `task verify`; `TestFollowerRunReturnsAndLeavesNothing` still passes.
- [ ] 4.2 `output/websocket` (`state.go:113`, `runView`) and `processor/graph-query` (`community.go:82-94`,
  `runCommunityView`): each run opens a sibling, opens its reader on it, and closes it when the view's run ends, a
  reopened view included (both supervisors reopen: `state.go:64-89`, `community.go:41`). Tests
  `TestWebsocketViewClosesItsConnection` and `TestCommunityViewClosesItsConnection`, with the stop checks. Accept:
  `task verify`.
- [ ] 4.3 `processor/graph-ingest`: the boot sweep (`poison_inventory.go:272`) runs on a sibling closed before
  `Start` returns, on success and on every failure path, an ended Start context included; a failed `Start` returns
  its cause and nothing from the close. Test `TestEntityStateGuardClosesItsConnection`. Accept: `task verify`; the
  lifecycle suite still passes.
- [ ] 4.4 `processor/graph-embedding`: for the failed-record scan (`component.go:346`), the Worker (`:389`) and the
  vector cache (`:420`), the component opens a sibling, opens `EMBEDDING_INDEX` on it, and builds that user's
  `embedding.NewStorage` over it (dedup stays on the main Client); the scan's sibling is closed before `Start`
  returns; the Worker's and the cache's are connected in `Start`, where they are built, and closed when their run ends.
  No exported signature in `graph/embedding` changes, so
  its unit tests over in-memory buckets keep running without a server. Test
  `TestGraphEmbeddingWatchesCloseTheirConnections`, with the stop checks. Accept: `task verify`.
- [ ] 4.5 2.1 passes, locally and on CI. Accept: `task verify`; the URL of a green `Integration` run on a
  named commit recorded in `notes/4.5.md`.

## 5. Docs and spec

- [ ] 5.1 `docs/boot.md`: how many connections a tier 0 and a tier 1 stack open and when, all under the main
  Client's name; that no `max_pending` setting is needed for a warm restart; that a product's own whole-bucket watch
  should run on a sibling. Accept: `task docs:check`.
- [ ] 5.2 `openspec/specs/composed-stack/spec.md:8`: state what is covered, without the issue reference (#741 item 1).
- [ ] 5.3 ADR-108 Decision 4 (`docs/adr/108-processor-shell.md:119`) gains the clause "the connection its watch runs
  on", with the date and the ruling's comment, as the owner ruled.
- [ ] 5.4 File the follow-up for two simultaneous message-logger KV watches on `ENTITY_STATES` on the main
  connection (`service/message_logger_kv_watch.go:138`), linked from the PR without a closing keyword.
- [ ] 5.5 The deltas under `specs/` match the code and the test names. Accept: `task spec:check`.

## 6. Close

- [ ] 6.1 Archive the change and sync the specs as the last content commit. Accept: `task spec:check`.
