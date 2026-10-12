# Design: test-infra-audit

base: 7070c84 (main at d77c597: PR #93 and PR #200 merged). Facts: `inventory.md` (INVENTORY PASS, round 3, c5787fe),
re-pinned here where the merges moved them. Its section 2 experiment and notes R3-N1 to N3 are replaced by
§ Short check.

## Premises, measured at 7070c84

- P1. testcontainers v0.44.0 turns Ryuk off only per process, from `TESTCONTAINERS_RYUK_DISABLED` or `ryuk.disabled`
  in `~/.testcontainers.properties`, read once (`tc:internal/config/config.go:70,118-125`); `SkipReaper` is
  deprecated (`tc:container.go:157`). With Ryuk off it adds no session label (`tc:generic.go:100-105`).
- P2. Ryuk fails on this Mac at our versions, the newest: 2 of 395 local run logs (#205 comment 6132700584).
- P3. A test binary stopped by SIGINT or by its `-timeout` runs no `t.Cleanup` (setup-02 P3, archived design
  `:187-188`; `go:src/testing/testing.go:2782-2802`). A killed run leaves containers whatever we build here.
- P4. Docker's container list carries each container's creation time in Unix seconds (`Created int64`, moby api
  v1.55.0 `types/container/container.go:99`) and filters by label (moby client v0.5.0 `container_list.go:40`).
- P5. One full run (coordinator's count, evidence `…-77449-1791838462-7102`, local only): 271 containers. By package:
  natsclient 105, graph-ingest 103, natsfixture 37, pkg/lifecycle 12, kvcatalog 11, component 3. Median start 287 ms,
  stop about 150 ms; `go test` 237 s; the runner's own contract tests 22.4 s of package time.
- P6. No integration-tagged file calls `t.Parallel` (0 of 71 files): tests in a package already run one at a time.
- P7. Tests that need a server of their own, counted: Restart 4 (`natsclient/integration_test.go:52,109`;
  `processor/graph-ingest/guard_generation_integration_test.go:70,155`); server stopped mid-test 1
  (`natsclient/storage_inventory_integration_test.go:214`); `WithMaxPayload` 4
  (`natsclient/request_response_bounds_integration_test.go:68,76,144,210`; PR #188 deletes one); account-wide usage 4
  (`storage_inventory_integration_test.go:40`; `graph/kvcatalog/storage_report_integration_test.go:74,229,297`).
  13 product tests, plus natsfixture's own 21.
- P8. Code under test uses fixed names: graph-ingest tests run `DefaultConfig` with stream `ENTITY` on `entity.>`
  (`processor/graph-ingest/fixture_integration_test.go:58,88`). natsclient's integration files spell 105 literal
  stream, bucket and consumer names, 8 of them in more than one test.
- P9. All JetStream data lives in streams: a key-value bucket is stream `KV_<bucket>` (nats.go v1.54.0
  `jetstream/kv.go:487`), an object store `OBJ_<bucket>` (`jetstream/object.go:480`); deleting a stream deletes its
  consumers (nats-server v2.15.0 `server/stream.go:8673,8731-8739`); `AccountInfo` reports `Streams` and `Consumers`
  (`jetstream/jetstream.go:288,293`). The container runs the same server version (`.nats-image:5`).
- P10. Subscriptions live on connections, which a stream reset does not touch. The server counts client connections on
  `/varz` (`connections`, `server/monitor.go:1270`) when started with `-m` (`server/opts.go:6261`).
- P11. Nothing outside natsfixture reads its `Error`, `Phase`, `ErrNotAdmitted`, `CreateKeyValue` or `Consume`
  (`git grep`); only `internal/harness/contract/docker_test.go:256` reads `CheckName`.
- P12. SemStreams' name-substring cleanup destroys volumes (setup-02 P11, archived design `:202-203`); the fixture
  creates none (`nats-fixture` spec, "Run-unique ownership").

## Options

- Machinery. (a) Do nothing: keeps the sources of 4 of 6 flake issues. (b) Remove it, one container per test: 271
  allocations a run stay. (c) Remove it, one server per package: **recommended**, the owner's direction. (d) NATS in
  the test process (#204): rejected by the owner's premise.
- Isolation on a shared server. (i) Per-test names: fails on P8. (ii) Reset to a verified-empty server before each test:
  **recommended**; it works with fixed names. (iii) Both: names add nothing while tests run one at a time (P6).
- Connection leaks. (α) Review only: the kind of rule that drifted in SemStreams. (β) The reset reads `/varz`:
  **recommended**; it costs one more mapped port per shared server (5 a run) and about 20 lines.

## Dispositions (inventory layers)

| Layer | Disposition |
| --- | --- |
| L1 Task entry | Keep: `go test` flags only (`Taskfile.yml:95-98`) |
| L2 Host lock | Remove (ruling 6132580576; the short check shows two runs at once pass) |
| L3 Admission token | Remove; plain `go test` runs (#129) |
| L4 Process group, signals, log FIFO | Remove. Ctrl-C reaches the terminal's foreground group; leftovers go to the sweep (P3) |
| L5 Leak check | Remove; replaced by label and sweep (ruling 6132995614) |
| L6 Preflight record | Remove from the run; `task doctor` keeps it |
| L7 Image pin | Keep the digest pin; it moves into natsfixture, embedded. Remove pre-pull and override; testcontainers pulls (`tc:docker.go:1220-1244`) |
| L8 Evidence directory | Remove. A failed start puts the container log in the test log |
| L9 `go test` flags | Keep; add `-shuffle=on` (go prints the seed, `go:src/testing/testing.go:2426`) |
| L10 Start | Keep the phases (as error text) and the one replacement container for #180; keep one test seam, the mapped-port read; remove 12 seams and the exported `Error` and `Phase` (P11) |
| L11 Observed absence | Remove; `Terminate`'s error is the answer. Removes the error-text classification at `deps.go:179` |
| L12 Owned-resource teardown | Remove; removing the container removes everything in it |
| L13 Restart | Keep, with #200's retry; drop the consumer clause with `Consume` |
| L14 Extras | Keep `FaultKV`, `WithMaxPayload`, `Open` (now takes a URL); stream helpers become functions; remove `CreateKeyValue`, `Consume` (P11) |
| L15 Safe names | Remove the fixture's name generator and the SemStreams substring list (P12); a name a script or Compose file assigns still starts `semengine-` |
| L16 Lifecycle floor on the fixture | Remove (the fixture is not a component) |
| L17 probe, L18 prochost | Keep |
| L19 Coverage coupling | Keep the gate; it reads `coverage/integration.coverprofile` and refuses one whose tree fingerprint differs |

Added: the label and sweep (about 30 lines), the shared server with its reset (about 120), two text checks (about
60 with their sensitivity tests). Removed: about 3,000 non-test and test lines (an estimate from `wc -l` of the files
listed).

## Behaviour: what a developer observes, and the test that proves it

- B1. `go test -tags integration ./natsclient` runs with no refusal. Proof: § Short check, step 1, the run without
  Task.
- B2. `task test:integration` is `go test -race -failfast -tags=integration -count=1 -p 2 -shuffle=on -timeout 10m
  -coverprofile=coverage/integration.coverprofile` over `./...` or the packages given, with Ryuk off. Proof:
  `TestUnitInvocationsPinned`, extended to this task, fails on drift.
- B3. Every container natsfixture starts has the label `org.c360studio.semengine.test`, valued with its package and
  process id. Before a test process starts its first container it removes, running or not, every container with that
  label that Docker created more than 30 minutes earlier. Not-found is ignored (`errdefs.IsNotFound`, as
  `tc:testing.go:160`); any other error is logged and does not fail the test (a declared degrade). Nothing is chosen by
  name or by whether a process is alive. Proof: an integration test labels a container with its own value, sweeps that
  value with age zero, and sees it gone while an unlabelled container stays.
- B4. A package's `TestMain` starts one shared server, runs `m.Run`, then stops it. Stop removes the container and
  returns an error, which fails the package, if a client connection is still open.
- B5. The per-test reset is the only way to the shared server's URL. It waits, bounded, until the server reports no
  client connection, deletes every stream, and fails the test unless `AccountInfo` reports zero streams and zero
  consumers. It registers a cleanup, run after the test's own, that fails the test, naming the count, while a client
  connection remains after a bounded wait. Proofs: a stream test A leaves is gone for test B; a connection test A leaves
  open fails A's cleanup; a reset while a connection is open fails.
- B6. A test with its own server calls `natsfixture.New` with `// Own server: <reason>` on the line above.
  `TestOwnServerReason` fails on a call without it.
- B7. `TestNoParallelIntegrationTests` fails on `t.Parallel` in an integration-tagged file.
- B8. Two runs at once from two worktrees both pass; a killed run leaves only labelled containers. Proof:
  § Short check.

What a reset does not cover, and who does: subscriptions and connections (B5's connection checks); server settings
such as `max_payload` (fixed per server; a test needing another takes its own, B6); server restarts (own server);
cumulative account usage and API counters (account-wide tests take their own server, P7).

Ryuk with plain `go test` follows the developer's own setting, which defaults to on (P1). The guidance page says how to
turn it off. Setting it from inside the test process was rejected: a process-wide environment change made by a library.

## Short check (the owner's verification, minutes)

Packages: natsclient, graph-ingest and natsfixture, measured at 94, 103 and 47 s in a full run (P5).

1. Two worktrees on the pull request's head run `task test:integration -- ./natsclient ./processor/graph-ingest
   ./internal/harness/natsfixture` at the same time, three times. Pass: six green runs. Once, run `go test -tags
   integration ./natsclient` without Task.
2. Start one run and press Ctrl-C after 30 s. `docker ps -a --filter label=org.c360studio.semengine.test` lists what it
   left; every container it lists carries the label. Run the sweep's test (B3).
3. Record each run's wall time and its container count (`docker events --since <start> --filter
   label=org.c360studio.semengine.test --filter event=create`). About 12 minutes in all (estimate: about 3 min a run).

## Pull requests, in order

1. **Plain `go test`.** Removes L2-L8 and the runner's tests; adds B1-B3 and `-shuffle=on`; re-points coverage
   (L19); edits `ci.yml`, `doctor.sh` and `docs/setup-plan.md`. End state: B1-B3 and B8 hold, `task verify` passes.
2. **Smaller natsfixture.** L10-L16. End state: no exported `Error`, `Phase`, `CreateKeyValue`, `Consume`; natsfixture's
   tests and its 80% floor pass.
3. **One server per package.** B4-B7, `TestMain` in the 5 packages, the 13 own-server marks, the guidance page and
   the `AGENTS.md` rows; archives the change. End state: a full run starts at most about 60 containers (from
   271), counted as in step 3. If the diff is too long, the package migrations split off after natsclient's.

Each pull request syncs the requirements it makes true into `openspec/specs/` as its last content commit. PR #188
(ready; deletes one `WithMaxPayload` test) merges before the third. PR #194 (draft; OpenSpec files only) has no
overlap today; if its build touches natsclient's test helpers, whichever lands second merges `main`.

## Risks

- A test that resets twice while holding a connection (a lifecycle suite run with a factory that resets) fails loudly
  at the second reset; it resets once per test or takes its own server.
- `/varz` adds a mapped port to each shared server; the replacement container covers both ports.
- Whether account usage falls to zero at once after a stream delete is unmeasured; the reset does not depend on it.

## Specs

Deltas: `integration-test-runner`, `nats-fixture`, `harness-boundaries`, `process-host`. `merge-gate` needs none: its
unit-run requirement still holds with the new command, and its CI job limit and coverage requirement do not name the
runner. `spec-queue` needs none. The `Purpose` paragraphs of `integration-test-runner`, `nats-fixture` and
`harness-boundaries` name the lock or SemStreams' cleanup; the archive rewrites them. The names `StartShared`, `Reset`
and `Shared` in the guidance page are placeholders that pull request 3 settles.

## Owner questions

1. **Separate names.** You called separate streams a good call. The design keeps tests apart by emptying the shared
   server before each test and checking it is empty, because graph-ingest's code uses fixed names a test cannot change.
   Recommendation: no naming rule; a test may use production names.
2. **Seeing the sweep in minutes.** With a 30-minute age, what a killed run leaves is removed by the first run that
   starts 30 minutes later. Recommendation: no setting to shorten it; the sweep's own test proves it in seconds, the
   check shows the leftovers carry the label, and one `docker ps` the next day shows them gone.
3. **"Remove after the run."** Recommendation: each test removes its own server, and each package its shared server,
   when it ends; no extra step after `go test`, because that step cannot run when a run is killed, the only time
   containers are left.
