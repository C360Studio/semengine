# Design: setup-02-isolated-harness

This design is the architect's revision 2 of SETUP 02 (epic #6), issued against the inventory in `inventory.md` after
it received `INVENTORY PASS`, with the owner's rulings of 2026-09-30 applied. Section references (A0–A8) point into
`inventory.md`. SemStreams paths are at `5457b3458936f668b71d2fea061f67f8d7d01e67`.

## Context

SemEngine has zero Go packages (A0). SETUP 02 adds the first ones: a harness that owns disposable real-NATS fixtures,
bounded waits, checked cleanup, failure evidence, lifecycle-ownership proofs, and Docker admission shared with
SemStreams on one daemon. It admits no SemStreams package and must not recreate SemStreams' harness defects: the #1411
lifecycle copy-paste, the #1417 unbounded cleanup roots, the substring-filtered volume destroyers, and the fixed
host-port bands.

Adjacent claims (A3): the plan asks for real brokers, bounded waits, and owner-level lifecycle proofs;
`scripts/verify.sh:8-9` reserves the `test:integration` slot; the shared host carries a live Docker daemon (`29.8.0
linux/arm64`, context `desktop-linux`), a SemSource container on a Docker-assigned port, and nine SemStreams
worktrees. The owner's rulings (below) settle the ledger home, the lock, the substrate, coverage timing, the dev
broker, the image pin, and the production lifecycle guard.

## Goals and non-goals

Goals: an admitted integration lane in `task verify`; a NATS fixture whose success means observed absence; a portable
lifecycle floor with probes; machine-checked boundaries that keep test code out of production, keep SemEngine's Docker
names out of SemStreams' destroyers, and keep every reused SemStreams file in the ledger.

Non-goals: the production owner-lifecycle guard, `pkg/errs`, graph fixtures, a Compose lane, a SemEngine dev NATS, an
embedded nats-server, testify, the AST cleanup guard, a SemStreams lock rename.

## Decisions

### Options considered and rulings

Revision 1's options stand as the record; the owner or orchestrator ruled each.

- **O1 NATS fixture substrate — ruled (a) testcontainers only.** (a) one testcontainers `nats` container per test
  (plan preference `setup-plan.md:242`; SemStreams' 168-file idiom); costs Docker, admission, and about 1–3s per
  start, and proves every Docker-ownership pass-evidence item. Rejected: (b) in-process `nats-server/v2` (SemStreams'
  20-file idiom, N2) — cannot produce #6's Docker pass evidence, enlarges the `go.mod`/govulncheck surface, shares the
  race-instrumented process, carries the version drift in A2.1(b), and a server panic kills the test binary; may be
  reopened at 04A with its ledger row fixing the drift. (c) Compose project per run — no consumer stack exists. (d)
  mocks — ruled out by the plan.
- **O2 Docker admission — ruled (a), exclusive, no SemStreams issue.** (a) adopt SemStreams' lock at its exact path
  and owner-file protocol, path held in one runner variable so a later joint rename is one line per repo; zero
  SemStreams change, coordinated from day one, exclusive is the most conservative bound. Rejected: (b) a neutral
  counting semaphore — needs a SemStreams change and CPU/memory evidence that does not exist yet
  (`01-testing.md:296-297`), and until SemStreams migrates SemEngine would still need SemStreams' lock; (c) `flock(2)`
  — kernel-released on death but invisible to SemStreams' `mkdir` protocol, and macOS has no `flock` CLI; (d) a token
  on the dev broker — tests would depend on a persistent service; (e) a Docker-native lock — the right shape if a
  remote daemon ever appears, but novel and requires a SemStreams change.
- **O3 Runner implementation — ruled (a) bash.** Adapted from SemStreams' script with process-group forwarding added,
  consistent with SETUP 01's `scripts/*.sh`, with four focused contract tests rather than SemStreams' 1566 lines.
  Rejected: (b) a Go tool (`exec.Cmd` with `Setpgid`, `Cancel`, `WaitDelay`) — cleaner signals but a second language
  for one protocol and a compiled tool in the gate path.
- **O4 Unbounded-cleanup guard — ruled (b) grep guard.** A `git grep` guard in `verify` for the exact #1417 shape
  (`Stop`, `Close`, `Terminate` with `context.Background()` or `context.TODO()` in `*_test.go`): cheap, zero
  baseline. No allowlist: the sanctioned cleanup roots (`natsfixture.New`'s bounded cleanup Stop and the rollback
  helper) live in non-test files the guard does not scan, so an allowlist would have no entry (corrected after
  implementation; accepted on review of PR #13). Rejected: (a) SemStreams' 4,000-line AST guard (revisit at 04A when
  ported tests arrive); (c) review only.
- **O5 Assertion library — ruled stdlib `testing`.** testify becomes a 04A ledger question when ported tests carry it.
- **O6 Image pin — ruled digest.** `nats:2.14.<x>-alpine@sha256:<digest>` in `.nats-image`, tag kept as a comment; the
  runner exports it and Go reads the env, so the pin is spelled once. A `TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX` mirror
  must serve the same digest or the pull fails loudly. Rejected: the mutable tag SemStreams uses.
- **O7 Coverage lane — ruled enforce 80% now** on `natsfixture`, `lifecycletest`, and `probe`, since every later proof
  rests on them. Rejected: report-only until 03B names critical packages.
- **Ledger check implementation (architect's choice, reversible; owner question Q9).** Recommended (a) a Go contract
  test with `gopkg.in/yaml.v3` (test-only dependency, strict schema, clear errors), run by `task test:unit` inside
  `task verify`, with `task ledger:check` as the focused alias. Rejected: (b) awk over a constrained YAML layout — no
  dependency, but brittle and silent on malformed input.
- **Name-prefix enforcement (architect's choice, reversible).** Recommended (a) a Go contract test over
  `scripts/*.sh`, `Taskfile.yml`, and `docker/**/*.yml`, plus a unit test on `natsfixture.Name()`. Rejected: (b) grep
  in `verify` — it needs regex over YAML keys and cannot see generated names.

### Package list

All Go code lives under `internal/harness/`. "Test-only" means imported only from `_test.go` files or other
`internal/harness/*` packages, enforced by T-B1. `go.mod` gains `github.com/nats-io/nats.go`,
`github.com/testcontainers/testcontainers-go`, and `gopkg.in/yaml.v3` (ledger check). No testify, no nats-server.

- **`internal/harness/natsfixture`** (test-only; ledger L1 adapt, L2 defer-exclude for the knobs, L3 adapt for the
  unexported rollback helper). One disposable NATS container per test: admission check, phased start, run-unique
  `Name(base)` (charset `[a-z0-9-]`, prefix from the sanitized test name, hex suffix), ownership record, checked
  cleanup, evidence, typed failure.
- **`internal/harness/probe`** (test-only; L5 adapt). `Callback`, `ObservedContext`, `Await`.
- **`internal/harness/lifecycletest`** (test-only; L4 adapt). Floor checks, `Run`, and the failpoint double.
- **`internal/harness/runner`** (test-only; L6 adapt for the script, L8 pattern only). `doc.go` plus four runner
  contract tests with a fake toolchain and a temp lock dir.
- **`internal/harness/contract`** (test-only; L7 adapt for T-B2). T-B1 import graph; T-B2 no retained context; T-B3
  one image pin; T-B4 no prune, no `--filter name=`, no foreign `down`; T-B5 SemEngine-assigned names; T-B6 no fixed
  NATS or host address literal in tests (`nats://localhost:`, `127.0.0.1:4222`, `:4222"`); T-B7 ledger schema.
- **`scripts/test-integration.sh`** (script; L6 adapt). Lock (byte-compatible), preflight, image pin export, evidence
  dir, process group and forwarding, canonical `go test`, leak check by session, token-checked release.
- **`scripts/lint-test-ports.sh` and `scripts/lint-test-ports_fixture_test.sh`** (scripts; L9 and L10 carry). Carried
  unchanged; wired into `task lint` and CI.
- **`scripts/cleanup-roots-check.sh`** (script; new). The O4(b) grep guard.
- **`.nats-image`** (file; new). `nats:2.14.<x>-alpine@sha256:<digest>`, the one image spelling.
- **`docs/admission-ledger.yaml`** (data; ruled). The ledger; schema under "Provenance".
- **Task** (extends SETUP 01). `test:integration`, `cover:check`, `ledger:check` (alias of the Go test), and `doctor`
  extended to report the lock owner, Docker host and context, Ryuk env, whether `~/.testcontainers.properties` is
  present, and whether the image digest and the ryuk image are cached. `verify` becomes the existing steps plus
  `test:integration` and `cover:check`.

Left out: the production lifecycle guard (ruled), `pkg/errs`, graph fixtures (03B), the Compose lane, dev NATS (ruled;
reinforced by P13), a TestMain constructor, fixture knobs, the embedded server, testify, the AST cleanup guard, and
`nats_version_contract_test.go`.

### Docker admission mechanism

- **Lock.** `scripts/test-integration.sh` acquires `/tmp/semstreams-integration.lock` with the exact SemStreams
  protocol: atomic `mkdir`; an owner file with the six keys `host,pid,started,identity,token,command`; the same stale
  rule and quarantine name; token-checked release. The path is a script constant overridable only by
  `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR` for contract tests (mirroring `SEMSTREAMS_INTEGRATION_LOCK_DIR`).
  `command=semengine <abs worktree>/scripts/test-integration.sh`. Either repository's runner sees the other as an
  ordinary owner, reports it, waits or fails fast, and quarantines only a provably dead same-host holder. Exclusive is
  the initial bound (ruled). A SemStreams protocol change after the pin enters as its own ledger row; R4 pins the
  owner-file format.
- **Go-side refusal.** `natsfixture.Start` requires `SEMENGINE_DOCKER_ADMISSION_TOKEN` and
  `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR` in the environment and requires `token=<value>` to be present in
  `<lockdir>/owner`; otherwise it returns `ErrNotAdmitted` naming `task test:integration -- <pkg>`, with zero Docker
  calls. The environment variable alone is never trusted.
- **Run identity and leak check.** The fixture writes its testcontainers session label, as `key=value`, to
  `$SEMENGINE_EVIDENCE_DIR/testcontainers-session`, taking the key from `testcontainers.GenericLabels()` (the entry
  whose value is `testcontainers.SessionID()`). After reaping the child group the runner lists `docker ps -aq
  --filter label=<that line>`, waits at most 15s for it to be empty (Ryuk's grace is 10s), removes survivors **by
  ID**, records them, and fails the run. No name filters anywhere; no second label. Implementation correction: this
  design first named the key `org.testcontainers.golang.sessionId`, which testcontainers-go v0.40.0 keeps only as a
  deprecated constant; it labels containers `org.testcontainers.sessionId` (`internal/core/labels.go:27`), and a
  filter on the old key matched nothing, so the leak check passed while Ryuk was still running. The key is now
  observed, not spelled, and an integration test requires the recorded label to select the fixture's container.
- **Interruption.** `go test` runs in its own process group; INT and TERM are forwarded to the group; after a 20s
  grace the group is killed, reaped, leak-checked, and the lock released; the runner exits 130 or 143. If the runner
  itself is SIGKILLed, the dead-pid lock is quarantined by the next same-host acquirer (either repository) and Ryuk
  reaps the containers. A different host's stale lock is a manual recovery, surfaced by `doctor`.
- **In-run bound.** `-p 2` is inherited from SemStreams gh#736 and unmeasured here; the runner records per-package
  wall time and container-start phase latency so the bound can be measured.
- **Environment namespace (M2).** SemEngine reads only `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR` (tests),
  `SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS` (0–3600, default 0), `SEMENGINE_EVIDENCE_DIR`, `SEMENGINE_NATS_IMAGE`
  together with a non-empty `SEMENGINE_NATS_IMAGE_OVERRIDE_REASON` (a digest-reference replacement for one run, for
  the forced-failure protocol; warned and recorded as `image_override` and `image_override_reason`; refused without
  the reason, ruling A4), and `SEMENGINE_TEST_SIGNAL_GRACE_SECONDS` (1–20, the runner contract tests only), and
  exports to Go `SEMENGINE_DOCKER_ADMISSION_TOKEN`,
  `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR`, `SEMENGINE_EVIDENCE_DIR`, and `SEMENGINE_NATS_IMAGE`; it sets
  `TESTCONTAINERS_RYUK_DISABLED=false` unconditionally. It never reads `SEMSTREAMS_*` or `GRAPH_INDEX_LATENCY_LOG`.
  There is no refresh knob: the digest pin makes tag re-pointing irrelevant, and the cache is checked with `docker
  image inspect nats@sha256:<digest>`.
- **Evidence directory.** `${SEMENGINE_EVIDENCE_DIR:-<worktree>/.evidence/<token>}`, gitignored. It records `docker
  version`, image digests (nats, ryuk), `docker info` latency, the lock token and owner, the session id, per-fixture
  JSON (phases with elapsed time, container ID, mapped port, owned names, stop phases, leak check), container logs on
  failure, cancellation, or partial start, the `go test` exit status, the signal received, the effective Docker host
  and context, Ryuk env, properties-file presence, and the before/after listings used by the pass-evidence protocol.
  CI uploads it as an artifact with retention; raw logs live in artifacts, not commits.
- **Known limits (H1).** The lock serialises SemEngine's integration lane against SemStreams' `task test:integration*`
  and its CI `Test` job only. SemStreams' `task e2e:*` stacks, `task services:*`, `task dev:nats:*`, tiered runs from
  `Taskfile.yml:207`, and `release.yml` Docker steps start without admission, so CPU and memory contention with them
  is unbounded, and SemStreams' `task e2e:clean` or `task clean:docker` can run mid-run. That is harmless to SemEngine
  only because of invariant I10 and because the fixture creates no volumes.

## Premises

- P1 No harness exists in SemEngine — A1 (all zero except prose).
- P2 SemStreams has no cross-repo admission; its lock is exclusive, `mkdir`-based, at a fixed path — A2.3
  `run-integration-tests.sh:8,208-242`.
- P3 Signalling only the `go` pid orphans test binaries; `t.Cleanup` does not run on SIGINT —
  `cmd/go/internal/base/signal.go:17-24`, `signal_unix.go:14`, `test.go:1449`.
- P4 testcontainers already owns run identity (session label) and exports `SessionID()`; Ryuk reaps by that label
  after about 10s — `bootstrap.go:41-95`, `reaper.go:545-552`, `testcontainers.go:52`, `config.go:72`.
- P5 `GenericContainer` can return a live container together with an error — `generic.go:89,94`.
- P6 SemStreams' production packages import test libraries (`natsclient/test_client.go:10,16`;
  `component/lifecycle_test_suite.go:11,14-15`), so the harness lives outside production packages behind an
  import-graph test.
- P7 Aggregate goroutine counts are SemStreams' leak proof (`lifecycle_test_suite.go:297,314`) and the plan rejects
  them, so they are dropped from the floor; owner joins are proven by probes.
- P8 The per-owner probe shapes are copied 26 times (observed context) and 9 times (entered/release), and the
  embedded-server helper appears in 20 files (A2.1(b), A2.2), so one `probe` home has consumers at birth.
- P9 No SemStreams package can be imported now; `natsclient`, `component`, and `pkg/errs` are tier-1 port-set
  packages, so patterns are adapted over `nats.go` and the source files get ledger rows.
- P10 Graph types are absent in SemEngine, so graph fixtures wait for 03B.
- P11 SemStreams destroys volumes by substring and prunes the build cache from routine tasks — `common.yml:77-79`,
  `ops.yml:80`, `clean.yml:17-19`.
- P12 Only `run-integration-tests.sh` takes the lock; e2e, services, dev, and release start Docker without it —
  callers versus `up`/`run` sites (A2.3).
- P13 SemStreams tests dial `nats://localhost:4222` in 38 files; any broker SemEngine put on 4222 would be reached by
  them (A2.1(b)).
- P14 testcontainers reads `~/.testcontainers.properties` then env; the runner's explicit
  `TESTCONTAINERS_RYUK_DISABLED=false` wins — `config.go:92-95,114`.

## Lifecycle ownership proof

Subjects: (S1) `natsfixture.Fixture` over real Docker and NATS; (S2) the runner's process-group and lock ownership;
(S3) `lifecycletest`'s own sensitivity via a failpoint double. Every check inspects retained state, not the returned
error alone.

**S3 — failpoint double `refowner`** (in `lifecycletest/*_test.go`). Failpoints: `acceptNilCtx`, `ignorePreCancelled`,
`startTwiceAllowed`, `stopBeforeStartPanics`, `stopIgnoresCallerDeadline` (never returns; the check must observe the
deadline and report), `stopReturnsNilWithWorkerRunning` (retained `workerDone` channel still open), `secondStopReruns`
(cleanup counter above 1), `restartPromisedButRefused`, `abortStopDropsCause`. Checks, each returning an error:
`CheckNilContextsRefused`, `CheckPreCancelledStartRefused`, `CheckStopBeforeStartSafe`,
`CheckControlledStopUnderLiveStartAuthority`, `CheckAbortStopPreservesCause`, `CheckRepeatedStopIsNoOp`,
`CheckSecondStartRefusedOrRestartCycle`. Each sensitivity test asserts a non-nil error for its failpoint and nil for
the clean double. `Run(t, factory, Promise{Restart bool})` wraps them in `t.Run`.

**S1 — fixture owner tests** (`natsfixture/*_integration_test.go`, `//go:build integration`). Failpoints go through
the unexported `deps` seam (`start`, `host`, `mappedPort`, `connect`, `jsReady`, `createStream`, `createKV`,
`terminate`, `drain`), each either `fail(err)` or `block(entered, release)`.

1. Partial-startup cleanup — `fail` at each phase after a container exists (`host`, `mappedPort`, `connect`,
   `jsReady`, `createStream`): the error's phase matches; `docker inspect <recorded id>` reports not found; the
   evidence file has phase, elapsed time, container id, and logs; `deps.terminate` is called once; a later `Stop`
   returns nil with zero Docker calls.
2. Cancellation during I/O — `block` at `start` and at `connect`, cancel the setup context while blocked, release: the
   error satisfies `errors.Is(err, context.Canceled)`, parent state is recorded, the container is absent, and no retry
   is attempted (replacement only for the mapped-port phase with a live parent).
3. Blocked callback — a real `jetstream` consumer with `probe.Callback`; `Stop(ctx)` under live Start authority. While
   blocked: the callback context's `Err()` is nil, the consumer still exists (`stream.ConsumerInfo`), the connection
   status is `CONNECTED`. After release: `Joined` closes before `Stop` returns; then consumer, stream, connection, and
   container are absent — order asserted by channel states, not timing.
4. Graceful finalisation — N `PublishAsync` in flight; `Stop` waits on `PublishAsyncComplete()` bounded and records
   `StreamInfo.State.Msgs == N` before deletion.
5. Deadline expiry — callback blocked, `Stop` with a 200ms context returns an error satisfying `errors.Is(err,
   context.DeadlineExceeded)`; `f.remaining()` lists the consumer, stream, connection, and container as unresolved;
   the container still exists; after release a second bounded `Stop` returns nil with everything absent. A timeout
   never counts as a completed join, and cleanup is retryable.
6. Repeated Stop — after success, a second `Stop` returns nil and `deps` call counters are unchanged.
7. Restart — with `Promise{Restart:false}`, a second `Start` returns `ErrAlreadyUsed` and leaves the first fixture's
   state untouched; `lifecycletest.Run` is applied to a factory adapter over the fixture.
8. Isolation — two fixtures in one test have disjoint container IDs, host ports, and `Name("x")` values; each `Stop`
   removes only its own.
9. Admission refusal — without a token, or with a token absent from the owner file: `ErrNotAdmitted` and zero Docker
   API calls (`deps` counter).

**S2 — runner contract tests** (`internal/harness/runner`, fake `docker` and `go` on `PATH`, temp lock dir). R1 busy
lock: exit 1, owner line printed, fake docker never invoked. R2 a dead-pid lock is quarantined; a live-pid lock is
not. R3 SIGTERM to the runner: the fake `go`, which spawns a grandchild, receives TERM in its group, the grandchild is
reaped, the lock is released only after the reap, exit status 143, evidence records the signal. R4 argv and env pinned
(`-race -failfast -tags=integration -count=1 -p 2 -timeout 10m -coverprofile=<evidence>/integration.coverprofile`,
ruling A5; `TESTCONTAINERS_RYUK_DISABLED=false`, `SEMENGINE_NATS_IMAGE` from `.nats-image`), and the owner-file key set
equals the recorded SemStreams format. Added after review: a signal during the pull reaps the pull (H2); a terminal
interrupt keeps the log (M3); SIGINT ignored on entry is detected and recorded (M4); a TERM-ignoring group is killed
after the grace (M10); an image override without a reason is refused (A4).

**Contract tests** (`internal/harness/contract`).

- T-B1 import graph: no non-`_test.go` file outside `internal/harness/` imports `internal/harness/...`,
  `testcontainers-go`, `testing`, or `gopkg.in/yaml.v3`.
- T-B2 no retained `context.Context` in any non-test struct (direct, embedded, aliased, in a container, as a generic
  type argument, or as a provider result); `probe.ObservedContext` is the spec's one exemption by exact name (ruling
  A2); untyped holders and closures are out of a static check's reach.
- T-B3 a `nats:` image literal (variable tags included) appears only in `.nats-image`, across every tracked file that
  configures or runs Docker; Markdown may cite the tag (ruling A3).
- T-B4 scripts and Taskfile contain no `docker … prune`, no `docker volume ls … --filter name=` (or `-f`), no `xargs …
  docker volume rm`, no Compose invocation without `-p semengine-<project>`, and no `docker ps … --filter name=`;
  backslash-continued commands are checked whole.
- T-B5 every literal Docker name in `scripts/*.sh`, `Taskfile.yml`, and `docker/**/*.yml` (`--name`,
  `container_name:`, `name:`, `-p`/`--project-name`, `COMPOSE_PROJECT_NAME`, volume and network names) starts with
  `semengine-` and contains none of
  `semstreams|nats-semstreams|semembed|agentic|crud-tools|deep-research|ops|research-graph` as a substring anywhere,
  so a lane segment such as `stops` or `loops` fails on `ops` (N3). A unit test checks that `natsfixture.Name()`
  output matches `^[a-z0-9-]+$`, starts with `semengine-`, and is destroyer-free for adversarial test names: a test
  named `TestOps…` is sanitized to a safe segment, for example by hashing any segment that contains a destroyer.
- T-B6 no `nats://localhost:`, `nats://127.0.0.1:`, `:4222"`, or `:8222"` literal in any `*_test.go`, and
  `scripts/lint-test-ports.sh` passes.
- T-B7 `docs/admission-ledger.yaml` parses; every entry has all ten fields non-empty; `source_sha` matches
  `^[0-9a-f]{40}$`; `disposition` is one of `carry`, `adapt`, `repair-before-port`, `defer-exclude`; `source_path` is
  unique.

## Pass-evidence protocol

Items 1, 2, and 4 are manual protocols recorded on PR #13 with the evidence directories attached as artifacts, not
committed; item 3's Go-level half, S1-8, S1-9, and the runner tests are permanent gates.

1. **Two worktrees without collisions.** Worktree A runs `task test:integration`; worktree B runs it concurrently with
   `SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS=900`; permanent test S1-8. Recorded: both green; B waited on A's owner
   line; container IDs, host ports, and session IDs disjoint. The lock serialised them (coordination); S1-8 proves
   isolation.
2. **Interrupting one leaves the other intact.** As item 1, then `kill -INT <A runner>` mid-run, with before/after
   `docker ps -a`, `docker volume ls`, and `docker network ls`. Recorded: A's signal, group TERM/KILL timings, leak
   check, and token-checked release; B completes; listings identical for every row not carrying A's session label
   (`semsource-setup03a-embed` and any `semstreams-nats` included).
3. **Forced startup failure.** `SEMENGINE_NATS_IMAGE=nats@sha256:<64 zeros>
   SEMENGINE_NATS_IMAGE_OVERRIDE_REASON=<why> task test:integration -- ./internal/harness/natsfixture/`; S1-1 and
   S1-2. Recorded: bounded pull failure, lock released, nothing left by session; per-test phase records, container
   logs, and `docker inspect` not-found assertions.
4. **Persistent data and unrelated containers survive, both directions.** Forward: with `semsource-setup03a-embed`
   running, SemStreams' `task dev:nats:start` broker holding a KV entry, and a sentinel `docker run -d --name
   semengine-evidence-sentinel -v semengine-evidence-sentinel-data:/data <pinned nats> -js -sd /data` holding a KV
   entry, run items 1–3; KV entries read back identical and dev/sentinel IDs, status, and volumes are unchanged in the
   diff. Reverse (B2): with the sentinel and SemEngine fixtures present, run read-only exactly SemStreams' destroyer
   listings — `docker volume ls -q --filter name=` for each of `semstreams`, `semembed`, `nats-semstreams`, `agentic`,
   `crud-tools`, `deep-research`, `ops`, `research-graph`, and `docker images --filter
   reference=semstreams-semstreams -q`; every listing is empty of SemEngine resources, proving `task e2e:clean` and
   `task clean:docker` would remove nothing of SemEngine's without running the destructive commands. T-B5 keeps it
   true.
5. **Reviewer sign-off.** An independent reviewer on PR #13 after archive and spec sync.

## Invariants and their spec homes

- I1 A fixture never starts a container without an admission token observed in the live lock owner file —
  `nats-fixture` › "Admission before Docker".
- I2 Every container the harness creates is found by the testcontainers session label written to the evidence dir; the
  runner removes only IDs matching it — `integration-test-runner` › "Leak check by session".
- I3 Setup contexts derive from the caller's; cleanup contexts are fresh, finite, and per operation; nothing retains a
  `context.Context` — `nats-fixture` › "Phased start with typed failure" and "Checked cleanup"; `harness-boundaries` ›
  "No retained context".
- I4 A Stop that returns nil has observed every owned resource absent; a Stop that hits its bound returns the context
  error and retains its handles for a later Stop — `nats-fixture` › "Checked cleanup"; `lifecycle-suite` › "Bound is
  not a join".
- I5 Controlled Stop runs with Start authority live; abort Stop preserves the caller's cause; repeated Stop is a
  no-op; second Start is refused unless restart is promised — `lifecycle-suite` › "Portable floor".
- I6 Lock acquisition is atomic; release is token-checked; only a provably dead same-host owner is quarantined —
  `integration-test-runner` › "Shared host lock".
- I7 Signals to the runner reach the whole child process group; the lock is released only after the group is reaped —
  `integration-test-runner` › "Process group ownership".
- I8 The NATS image is spelled once — `harness-boundaries` › "One image pin".
- I9 Production packages import neither the harness nor test libraries — `harness-boundaries` › "Import graph".
- I10 Every SemEngine-assigned Docker name (container, volume, network, Compose project) starts with `semengine-`,
  uses `[a-z0-9-]`, and contains no SemStreams destroyer substring; run-unique suffixes are lowercase hex —
  `harness-boundaries` › "SemEngine-assigned names".
- I11 SemEngine never selects Docker resources by name substring, only by observed ID or exact label —
  `integration-test-runner` › "Leak check by session"; `harness-boundaries` › "No broad Docker cleanup".
- I12 SemEngine tests never dial a fixed broker address or bind a fixed host port — `harness-boundaries` › "No fixed
  addresses in tests".
- I13 Every reused SemStreams file has a ledger entry with a 40-hex SHA and a known disposition, checked in `task
  verify` — `harness-boundaries` › "Admission ledger".

## Risks and declared costs

- `-p 2` is inherited from SemStreams gh#736 and unmeasured on this host; the runner records the timings that would
  measure it.
- E1: testcontainers behaviour on the `desktop-linux` context is read from source, not run.
- E2: CI pull time for the `nats` and `ryuk:0.13.0` images against the 15-minute `verify` job timeout is unmeasured.
- E4: Docker assigns SemEngine's host ports from the daemon's dynamic range, which on Linux daemons overlaps
  SemStreams' fixed 34xxx–59xxx e2e bands. A SemEngine container could hold a port a SemStreams e2e tier is about to
  publish; SemStreams' `e2e-check-ports.sh` then reports the holder and their run fails fast. Low probability and
  short-lived; not designed around in SETUP 02. SemStreams' own note (`e2e-reserve-ports.sh:27-31`) says renumbering
  out of the range is the fix, and it is theirs. Whether the daemon honours `ip_local_reserved_ports` is unverified.
- A stale lock held from another host is a manual recovery.
- The `/tmp` lock does not follow `DOCKER_HOST` to a remote daemon.
- SemStreams' `docker builder prune -f` empties the shared build cache; SemEngine builds no images in SETUP 02, and a
  later cache miss is slower, not incorrect.
- Embedded `nats-server` v2.12.4 (SemStreams `go.mod`) and the 2.14 image may differ behaviourally; recorded as drift,
  not tested.

## Provenance

Every SemStreams file this change reads for reuse has an entry in `docs/admission-ledger.yaml` at
`5457b3458936f668b71d2fea061f67f8d7d01e67`, re-checked at SETUP 03A. The ledger's entry for each file carries the full
field set; this is the index.

| Row | SemStreams source | Disposition | Destination |
| --- | --- | --- | --- |
| L1 | `natsclient/test_client.go` | adapt | `internal/harness/natsfixture` |
| L2 | `natsclient/test_options.go` | defer-exclude | none (knobs omitted) |
| L3 | `internal/lifecyclecleanup/lifecyclecleanup.go` | adapt | `natsfixture` (unexported) |
| L4 | `component/lifecycle_test_suite.go` | adapt | `internal/harness/lifecycletest` |
| L5 | `processor/graph-query/lifecycle_owner_test.go` | adapt | `internal/harness/probe` |
| L6 | `scripts/run-integration-tests.sh` | adapt | `scripts/test-integration.sh` |
| L7 | `test/contract/context_ownership_contract_test.go` | adapt | `internal/harness/contract` (T-B2) |
| L8 | `test/testinfra/integration_runner_contract_test.go` | defer-exclude | none (pattern only) |
| L9 | `scripts/lint-test-ports.sh` | carry | `scripts/lint-test-ports.sh` |
| L10 | `scripts/lint-test-ports_fixture_test.sh` | carry | same path |
| L11 | `testutil/nats.go` | defer-exclude | none |
| L12 | `test/testinfra/cleanup_guard_test.go` | defer-exclude | none (grep guard instead) |

The architect chose twelve entries rather than the eight named when the ledger was ruled: the four additions are the
two `lint-test-ports` carries and two explicit exclusions, so a later reader sees they were considered.

## Rulings applied

Owner rulings of 2026-09-30, recorded on [issue #6][setup-02-rulings]; earlier plan
rulings are in the [rulings log on PR #1](https://github.com/C360Studio/semengine/pull/1#issuecomment-5917717356).

- Q1: adopt `/tmp/semstreams-integration.lock` as-is, exclusive as the initial bound, no SemStreams rename issue now.
- Q2: testcontainers only; no embedded nats-server lane.
- Q3: enforce 80% coverage now on `natsfixture`, `lifecycletest`, and `probe`.
- Q4: no SemEngine dev NATS task in SETUP 02; pass-evidence item 4 uses daemon residents and a `semengine-` sentinel.
- Q5: the ledger is `docs/admission-ledger.yaml`, machine-checked in `task verify`.
- Q6: the NATS image is pinned by digest in `.nats-image`.
- Q7: the production owner-lifecycle guard (semstreams#1411) is out of scope and becomes `component`'s
  repair-before-port row at 03B/04A.

Decided by the architect within the brief, reversible by the owner: stdlib `testing` only; `internal/harness/`
placement; the grep guard instead of the AST guard; `-p 2`, `-failfast`, and `-timeout 10m`; Go-side admission refusal
by observing the owner file; run identity via the testcontainers session label; `semengine-` as the mandatory name
prefix; hashing destroyer-containing test-name segments in `Name()`; T-B5 and T-B6 as Go contract tests; twelve ledger
entries.

## Decided Questions

Ruled 2026-09-30 on [issue #6][setup-02-rulings], items
8–10. No open question remains for this change.

- **Q8 (E4).** The dynamic-port overlap with SemStreams' e2e bands is a declared cost for SETUP 02. Constraining
  Docker's mapping range would need daemon configuration or fixed ports, which the plan forbids.
- **Q9.** The ledger check is a `gopkg.in/yaml.v3` Go contract test (test-only dependency), not awk.
- **Q10.** The ledger holds code provenance only; documents whose rules were adopted (for example
  `docs/contributing/01-testing.md`) are cited in the specs and get no ledger row.

[setup-02-rulings]: https://github.com/C360Studio/semengine/issues/6#issuecomment-5921046663
