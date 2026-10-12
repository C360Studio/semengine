# Design: test-infra-audit

base: ecb13d9 (main at 993fd1f: PRs #93, #200 and #188 merged). Facts: `inventory.md` (INVENTORY PASS, round 3,
c5787fe), re-pinned here where merges moved them. Its section 2 experiment and notes R3-N1 to N3 are replaced by
§ Short check. Round 1 of design review: DESIGN CHANGES REQUESTED at d63f0b9; this revision answers it.

## Premises, measured at ecb13d9

- P1. testcontainers v0.44.0 reads its configuration once per process, behind `sync.Once`
  (`tc:internal/config/config.go:19-20,103-110`); `TESTCONTAINERS_RYUK_DISABLED` wins over
  `~/.testcontainers.properties` (`:119-125,165-180`); `SkipReaper` is deprecated (`tc:container.go:157`). No package
  `init` reads it: the module's non-test `init` functions (`internal/core/docker_socket.go:25`,
  `internal/core/bootstrap/bootstrap.go:47`, `log/logger.go:29`) do not call `config.Read`. Every container gets
  `org.testcontainers.sessionId` (`tc:internal/core/labels.go:38-51`, `tc:docker.go:1250-1253`); only
  `org.testcontainers.reap` depends on Ryuk (`labels.go:46-48`). natsfixture is the module's only importer of
  testcontainers (`TestImportGraph`).
- P2. Ryuk fails on this Mac at our versions, the newest: 2 of 395 local run logs (#205 comment 6132700584).
- P3. A test binary stopped by SIGINT or by its `-timeout` runs no `t.Cleanup` (setup-02 P3, archived design
  `:187-188`; `go:src/testing/testing.go:2782-2802`). A killed run leaves containers whatever we build here.
- P4. Docker's container list carries each container's creation time in Unix seconds (`Created int64`, moby api
  v1.55.0 `types/container/container.go:99`) and filters by label (moby client v0.5.0 `container_list.go:40`). How far
  the daemon's clock (Docker Desktop's VM) may drift from the host's is unmeasured.
- P5. One full run, evidence `…-77449-1791838462-7102` (local only): 271 containers; by package natsclient 105,
  graph-ingest 103, natsfixture 37, pkg/lifecycle 12, kvcatalog 11, component 3. Median start 287 ms, stop about
  150 ms (coordinator's count). From the same run's `per-package.txt`: natsclient 93.6 s, graph-ingest 102.7 s,
  natsfixture 47.1 s, the runner's own tests 22.4 s; `go test` 237 s in all (`runner.env`, `go_test_ms`).
- P6. 72 Go files carry `//go:build integration`; none calls `t.Parallel`. Tests in a package run one at a time.
- P7. Product tests that need a server of their own: Restart 4 (`natsclient/integration_test.go:52,109`;
  `processor/graph-ingest/guard_generation_integration_test.go:70,155`); server stopped mid-test 1
  (`natsclient/storage_inventory_integration_test.go:214`); `WithMaxPayload` 3
  (`natsclient/request_response_bounds_integration_test.go:69,77,146`). 8 in all, plus natsfixture's own 21. The four
  account-wide tests (`storage_inventory_integration_test.go:40`; `graph/kvcatalog/storage_report_integration_test.go`
  `:74,229,297`) read stream bounds and per-tier usage and limits, not cumulative counters, so they share (P13).
- P8. Code under test uses fixed names: graph-ingest tests run `DefaultConfig` with stream `ENTITY` on `entity.>`
  (`processor/graph-ingest/fixture_integration_test.go:58,88`); natsclient's integration files spell 105 literal
  stream, bucket and consumer names, 8 of them in more than one test.
- P9. All JetStream data lives in streams: bucket `KV_<bucket>` (nats.go v1.54.0 `jetstream/kv.go:487`), object store
  `OBJ_<bucket>` (`jetstream/object.go:480`); a stream's delete removes its consumers (nats-server v2.15.0
  `server/stream.go:8673,8731-8739`); `AccountInfo` reports `Streams`, `Consumers`, `Memory`, `Store`
  (`jetstream/jetstream.go:273-293`). The container runs 2.15.0 too (`.nats-image:5`).
- P10. Every account imports `$SYS.REQ.ACCOUNT.PING.STATZ` (`server/events.go:2404`); the reply's `conns` counts the
  account's client connections (`events.go:217-223`, `monitor.go:2545-2549`). No mapped port is needed.
- P11. Nothing outside natsfixture reads its `Error`, `Phase`, `ErrNotAdmitted`, `CreateKeyValue` or `Consume`
  (`git grep`); only `internal/harness/contract/docker_test.go:256` reads `CheckName`.
- P12. SemStreams' name-substring cleanup destroys volumes (setup-02 P11, archived design `:202-203`); the fixture
  creates none.
- P13. Probe (below): STATZ answers in the default account and counts the asking connection; deleting every stream
  leaves zero streams, consumers, memory and storage at once.
- P14. prochost re-runs the whole test binary (`internal/harness/prochost/prochost.go:96`,
  `-test.run=^TestHelperProcess$`), so a child runs its package's `TestMain`; graph-ingest's `TestHelperProcess` is in
  an integration-tagged file (`processor/graph-ingest/settlement_integration_test.go:64`), and one child is killed
  (`:283`).

## Probe of the pinned server (P10, P13)

Program `/tmp/ti-audit/probe/main.go` (local only; to be posted on PR #206 with this output): nats-server v2.15.0 in
the probe's own process, `Options{Host: "127.0.0.1", Port: -1, JetStream: true}`, no config file, as the fixture's
container runs `--port 4222 --js`; nats.go v1.54.0; go1.26.9 darwin/arm64. Command: `GOPROXY=off go run .`.

| Question | Output |
| --- | --- |
| Default account gets a STATZ reply; system account exists | `system account: true`; reply decoded |
| `conns` with only the asking connection open | `1` |
| `conns` with a second connection open | `2` |
| After the second connection's `Close` | `conns=1 after 152.917µs and 1 polls` |
| CONNZ names connections | `[<server id> reset test-A-client]` |
| Before the reset: a stream, a bucket with a watcher, an object store, a durable | `streams=3 consumers=2 memory=4421 store=106250` |
| Right after deleting `[ENTITY KV_ENTITY_STATES OBJ_blobs]` | `streams=0 consumers=0 memory=0 store=0` |

Not probed: the same over Docker's port forwarding, where a close may take longer; the reset waits for it, bounded.

## Options

- Machinery. (a) Do nothing: keeps every source of today's flakes. (b) Remove it, one container per test: keeps
  271 allocations a run. (c) Remove it, one server per package: **recommended**, the owner's direction; still exposed
  to #180 and its replacement container (#179's area). (d) NATS in the test process (#204): rejected by the owner.
- Isolation on a shared server. (i) Per-test names: fails on P8. (ii) A reset to a verified-empty server before each
  test: **recommended**; it works with fixed names. (iii) Both: names add nothing while tests run one at a time (P6).
- Connection leaks. (α) Review only. (β) STATZ through the client port: **recommended** (P10, P13). (γ) `/varz` on
  a monitoring port: one more mapped port per server, more #180 exposure.

## Dispositions (inventory layers)

| Layer | Disposition |
| --- | --- |
| L1 Task entry | Keep: `go test` flags only (`Taskfile.yml:95-98`) |
| L2 Host lock | Remove (ruling 6132580576; the short check shows two runs at once pass) |
| L3 Admission token | Remove; plain `go test` runs (#129) |
| L4 Process group, signals, log FIFO | Remove. Ctrl-C reaches the terminal's foreground group; leftovers go to the sweep (P3) |
| L5 Leak check, and Ryuk | Remove both (ruling 6132995614); natsfixture turns Ryuk off and labels and sweeps (B3) |
| L6 Preflight record | Remove from the run; `task doctor` keeps it |
| L7 Image pin | Keep the digest pin; it moves into natsfixture, embedded. Remove pre-pull and override; testcontainers pulls (`tc:docker.go:1220-1244`) |
| L8 Evidence directory | Remove. A failed start puts the container log in the test log; prochost logs a child's output when its test fails |
| L9 `go test` flags | Keep; add `-shuffle=on` (go prints the seed per package, `go:src/testing/testing.go:2426`) |
| L10 Start | Keep the phases (as error text) and the one replacement container for #180; keep one test seam, the mapped-port read; remove 12 seams and the exported `Error` and `Phase` (P11) |
| L11 Observed absence | Remove; `Terminate`'s error is the answer. Removes the error-text classification at `deps.go:179` |
| L12 Owned-resource teardown | Remove; removing the container removes everything in it |
| L13 Restart | Keep, with #200's retry; drop the consumer clause with `Consume` |
| L14 Extras | Keep `FaultKV`, `WithMaxPayload`, `Open` (now takes a URL); stream helpers become functions; remove `CreateKeyValue`, `Consume` (P11) |
| L15 Safe names | Remove the fixture's name generator and the SemStreams substring list (P12); a name a script or Compose file assigns still starts `semengine-` |
| L16 Lifecycle floor on the fixture | Remove (the fixture is not a component) |
| L17 probe, L18 prochost | Keep |
| L19 Coverage coupling | Keep the gate; it reads `coverage/integration.coverprofile` and refuses one whose tree fingerprint differs |

Added: Ryuk off, the label and the sweep (about 35 lines), the shared server with its reset (about 100), two text
checks (about 60 with their sensitivity tests). Removed: about 3,000 lines (an estimate from `wc -l`).

## Behaviour: what a developer observes, and the test that proves it

- B1. `go test -tags integration ./natsclient` runs with no refusal. Proof: § Short check, step 1, the run without
  Task.
- B2. `task test:integration` is `go test -race -failfast -tags=integration -count=1 -p 2 -shuffle=on -timeout 10m
  -coverprofile=coverage/integration.coverprofile` over `./...` or the packages given. Proof:
  `TestUnitInvocationsPinned`, extended to this task, fails on drift.
- B3. Once per process, before its first testcontainers call, natsfixture sets `TESTCONTAINERS_RYUK_DISABLED=true`
  whatever the caller set, then removes, running or not, every container labelled `org.c360studio.semengine.test`
  that Docker created more than 30 minutes earlier. Every container it starts carries that label, valued with its
  package and process ID. Nothing is chosen by name or by whether a process is alive. A removal error never fails a
  test: not-found is ignored, anything else goes to the test output. Proofs: after natsfixture's first container,
  `testcontainers.GenericLabels()` has no `org.testcontainers.reap` key (that a caller's own value is overridden is
  review only, since the configuration is read once per process and the test order is shuffled); the sweep's test,
  over a label value of its own, keeps a young container under a one-hour age, removes it under age zero, and leaves
  an unlabelled one running.
- B4. A package's shared server starts at its first reset, under five minutes so a cold image pull on CI fits, and
  closes its start connection before the reset counts; a process that never resets (a prochost child, P14, or a
  `-run` that selects no NATS test) starts nothing. `TestMain` stops it after `m.Run` if it started, removing the
  container. A failed start fails that test; every later reset in the process fails at once with the same error.
- B5. The reset is the one call that returns the shared server's URL. Under a bounded context it waits until STATZ
  reports one connection, its own, and otherwise fails the test saying a connection from an earlier test is still
  open; deletes every stream; fails the test unless `AccountInfo` reports zero streams, consumers, memory and storage;
  closes its connection; and registers a cleanup, run after the test's own, that fails the test, naming the count,
  while connections other than the asking one remain after a bounded wait. Proofs, in natsfixture's tests: what test A
  creates is gone for test B; a connection test A leaves open fails A's cleanup; a reset while one is open fails.
- B6. A test with its own server calls `natsfixture.New` with `// Own server: <reason>` on the line above.
  `TestOwnServerReason` checks call sites with that text, and the 8 own-server tests call `New` themselves so it
  reads each reason; a call behind a helper, a renamed import or a function value escapes it, and is review only.
- B7. `TestNoParallelIntegrationTests` fails on `t.Parallel()` in an integration-tagged Go file; a call in an untagged
  helper or on a receiver not named `t` escapes it, and is review only.
- B8. Two runs at once from two worktrees both pass. Proof: § Short check, step 1, in pull requests 1 and 3.

A leaked connection fails every later test in its package under plain `go test` (Task's `-failfast` stops at the
first); the reset's message points at the earlier test. What a reset does not touch: server settings such as
`max_payload` and the server's uptime (a test needing another takes its own server); cumulative counters such as
STATZ `total_conns` and `AccountInfo.API`, which no test reads today (`git grep` over the 72 files).

## Short check (the owner's verification, minutes)

Packages: natsclient, graph-ingest and natsfixture (P5: 94, 103 and 47 s).

1. Two worktrees on the pull request's head run `task test:integration -- ./natsclient ./processor/graph-ingest
   ./internal/harness/natsfixture` at the same time, three times. Pass: six green runs. Once, run `go test -tags
   integration ./natsclient` without Task.
2. Start one run and press Ctrl-C after 30 s. `docker ps -a --filter label=org.c360studio.semengine.test` lists what
   it left, every container labelled.
3. Record each run's wall time and its container count (`docker events --since <start> --filter
   label=org.c360studio.semengine.test --filter event=create`).

About 12 minutes (estimate, about 3 min a run). Pull request 1 runs steps 1 to 3; pull request 3 runs steps 1 and 3
again, on shared servers. The check does not show the leftovers removed: that takes 30 minutes, and the sweep's own
test shows the rule (owner question 2). Isolation between tests is shown by natsfixture's tests (B5), not here.

## Pull requests, in order

1. **Plain `go test`.** Removes L2-L8, L15 and the runner's tests; adds B1-B3 and `-shuffle=on`; re-points coverage
   (L19); updates every file that names the runner, the lock, the evidence directory or `SEMENGINE_NATS_IMAGE`
   (tasks 2.6, 2.8); `AGENTS.md` rows 1 and 2. End state: B1-B3 and B8 hold; `task verify` passes.
2. **Smaller natsfixture.** L10-L14, L16. End state: no exported `Error`, `Phase`, `CreateKeyValue`, `Consume`;
   natsfixture's tests and its 80% floor pass.
3. **One server per package.** B4-B7, `TestMain` in the 5 packages, the 8 own-server marks, the guidance page and
   `AGENTS.md` rows 3 to 7; archives the change. End state: a full run starts at most about 50 containers (5 shared,
   8 own, and natsfixture's own tests, 37 today), from 271. If the diff is too long, the package migrations split off
   after natsclient's.

Each pull request syncs the requirements it makes true into `openspec/specs/` as its last content commit. PR #194
(draft; OpenSpec files only) has no overlap today; if its build touches natsclient's test helpers, whichever lands
second merges `main`.

## Specs

Deltas: `integration-test-runner`, `nats-fixture`, `harness-boundaries`, `process-host`. A requirement that loses a
scenario is REMOVED and ADDED under a new name, since OpenSpec refuses a MODIFIED block that drops one.
`merge-gate` and `spec-queue` need none. The archive rewrites the `Purpose` paragraphs that name the lock or
SemStreams' cleanup. The names `Shared` and `Reset` in the guidance page are placeholders pull request 3 settles.

## Owner questions

1. **Separate names.** You called separate streams a good call. The design keeps tests apart by emptying the shared
   server before each test and checking it is empty, because graph-ingest's code uses fixed names a test cannot change.
   Recommendation: no naming rule; a test may use production names.
2. **Seeing the sweep in minutes.** With a 30-minute age, what a killed run leaves is removed by the first run that
   starts 30 minutes later. Recommendation: no setting to shorten it; the sweep's own test proves the rule in seconds,
   the check shows the leftovers carry the label, and one `docker ps` the next day shows them gone.
3. **"Remove after the run."** Recommendation: each test removes its own server, and each package its shared server,
   when it ends; no extra step after `go test`, because that step cannot run when a run is killed, the only time
   containers are left.
