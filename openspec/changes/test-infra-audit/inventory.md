# Inventory: test-infra-audit (#205)

base: 841bf4d (origin/main). Also read: PR #93 head fd9463c0116c4872a414da51a3f2f0759bb0875f, PR #200 head
5ae562575ac358cbcd9f4891bf2cd70438183a40.

**Question.** Between `task test:integration` and a test talking to NATS, which layers exist? What failure was each
added to prevent? Does testcontainers, Ryuk, `go test` or Docker already handle that failure? What has each layer cost
in flakes and bugs?

**Read from:**

- this tree;
- SemStreams at the pin `8b99efe9`, through `gh api` only (`scripts/run-integration-tests.sh` and `docs/adr`), as
  `docs/inventory-scope.md` row "semstreams" allows;
- testcontainers-go v0.44.0 from the module cache (`go.mod:17`), paths below written as `tc:`;
- Go 1.26.9 from GOROOT (`go.mod:3`), paths written as `go:`.

**Not done:** no Docker run, at the owner's word for this pass (a sister session holds the daemon and the lock).
Item 2's experiment is written in section 2, not run.

## 0. Premises in #205 and #204, checked

| Claim | Measured on 841bf4d | Verdict |
|---|---|---|
| testcontainers-go v0.44.0 | `go.mod:17` — `github.com/testcontainers/testcontainers-go v0.44.0` | true |
| lock at `/tmp/semstreams-integration.lock` | `scripts/admission-lock.sh:8` — `readonly admission_lock_default="/tmp/semstreams-integration.lock"` | true |
| runner 572 lines; contract tests 1,203 lines | `wc -l`: 572 and 1203 | true |
| natsfixture ~1,970 non-test / ~2,340 test lines | `wc -l`: 1,971 non-test, 2,341 test | true |
| "a stop-and-start retry when the mapped port is missing, and `Restart` with its own retry (PR #200 adds one more)" | On main, Start's retry makes a **replacement container**, not a stop-and-start: `fixture.go:149-160`, `maxAttempts = 2` at `:31`. On main, Restart has **no** retry: `restart.go:32-34` — "no replacement container is created"; one mapped-port read at `:145-152`. PR #200 adds Restart's first retry, a stop-and-start of the same container. | wrong as worded |
| "4 of 6 `class:flake` are in this machinery (#49, #62, #180, #201)" | #180's cause is Docker Desktop for Mac, not code we wrote. PR #200's diagnosis (comment 6128094108) reproduced it with a standalone program using plain testcontainers and natsfixture's request shape. `tc:docker.go:187-228` (`MappedPort`) reads the empty binding Docker Desktop's API proxy returns. Removing our machinery does not remove #180. #49 is the probe package's own timing test, not Docker machinery. #62 is a test that exists only because the runner exists. | partly wrong |
| Ryuk "is forced on by the runner" | `scripts/test-integration.sh:362` — `export TESTCONTAINERS_RYUK_DISABLED=false`. testcontainers' default is already on: `tc:internal/config/config.go:70` — ``RyukDisabled bool `properties:"ryuk.disabled,default=false"` ``. The export only overrides a user's env or `~/.testcontainers.properties` (`:123-125`). | true; the default is the stock feature |
| #204: "73 test files run under the `integration` tag" on fd9463c | `git grep -l '^//go:build integration' fd9463c` gives **71**: 30 on main plus 41 added by #93. #204 counted 2 files in `internal/harness/contract` whose string literals contain the tag (`boundaries_test.go:62`, `testtext_test.go:146`). | wrong (71) |
| #204: "Every one of them goes through the runner, the host lock and a Docker container" | 2 files start nothing: `internal/tlsutil/mtls_integration_test.go` (httptest only, e.g. `:69-71`) and, on #93, `component/schema_integration_test.go` (config validation only, `:13-30`) | wrong |
| #205 omits: the approved plan requires cross-repository admission | `docs/setup-plan.md:266-268` — "Dynamic ports prevent collisions but do not prevent CPU or memory saturation. During extraction, admission control for a shared Docker daemon must coordinate SemEngine and SemStreams"; `:362-363` — "Bound Docker concurrency and coordinate admission across the repos sharing the daemon." So the lock's stated job is **saturation and coordination**, not correctness. Removing it amends the approved plan. | missing premise |
| #205 item 2: "if parallel runs are safe, the lock … [has] no job left" | That holds only for correctness. The plan's stated job is CPU and memory saturation (row above), so item 2 must also measure start latency and wall time. The setup-02 pass evidence never ran two runs at once: "The lock serialised them (coordination)" (`openspec/changes/archive/2026-09-30-setup-02-isolated-harness/design.md:297`). | incomplete premise |

## 1. Layers from `task test:integration` to NATS (#205 item 1)

Size is non-test lines unless marked. "Stock" means what testcontainers, Ryuk, `go test` or Docker already do, cited
to source.

### L1. Task entry

- `Taskfile.yml:95-98` — `scripts/test-integration.sh {{.CLI_ARGS}}`, desc "(host lock, process group, leak check)".
- `scripts/verify.sh:13` puts `test:integration` inside `task verify`.
- CI: `.github/workflows/ci.yml:25-28,52-56`. One `verify` job runs `task doctor` then `task verify` on
  `ubuntu-24.04` with `timeout-minutes: 15`. There is no separate integration job.

### L2. Host lock shared with SemStreams

- **Code.**
  - `scripts/admission-lock.sh:8`.
  - Owner fields: `scripts/test-integration.sh:99-107` — `owner_host=$(hostname)` …
    `owner_identity_utc=$(TZ=UTC LC_ALL=C ps -o lstart= …)`.
  - Lock section `:137-277`: liveness `owner_is_stale` `:175-187`, `publish_owner` `:210`, `acquire_lock` `:230`,
    `release_lock` `:265`.
  - Wait budget `:49-54`.
  - doctor reads it: `scripts/doctor.sh:107-122`.
  - About 160 runner lines.
- **Spec.** `openspec/specs/integration-test-runner/spec.md:9-87`. One requirement, 31 lines of SHALL text, 7
  scenarios.
- **Tests.** About 480 of the runner tests' 1,203 lines: `internal/harness/runner/runner_test.go:244-337` (identity
  helpers), `:343-712` (R1 ×5, R2), `:849-862`.
- **Added for.**
  - Plan mandate: `docs/setup-plan.md:266-268,362-363`.
  - setup-02 ruling O2(a): "adopt SemStreams' lock at its exact path … exclusive is the most conservative bound"
    (`…/2026-09-30-setup-02-isolated-harness/design.md:43-50`).
  - Ruled 2026-09-30 (Q1): `scripts/admission-lock.sh:3-4`.
  - Adapted from SemStreams `scripts/run-integration-tests.sh` at 5457b345 (`docs/admission-ledger.yaml:196-212`, row
    L6). At the pin the lock is at `run-integration-tests.sh:8` and `:208-242`. Its header gives no reason beyond "Lock
    contention fails immediately by default" (`:4-6`).
- **Known limit, by design.** It serialises only SemStreams' `task test:integration*` and CI `Test`. SemStreams' e2e,
  services, dev and release start Docker without it (`design.md:176-180`, premise P12 at `:204-205`).
- **Stock.** Nothing stock serialises runs. testcontainers keeps runs apart rather than serialising them:
  - session id = sha256(parent pid, parent create time): `tc:internal/core/bootstrap/bootstrap.go:41-94` (`:50`,
    `:86-93`). So every `go test` invocation is its own session.
  - one Ryuk per session, named and looked up by that session: `tc:reaper.go:82-87` (`"reaper_" + sessionID`) and
    `:158-180` (filter `label=<LabelSessionID>=<sessionID>` and `name=reaper_<id>`).
  - Docker assigns each container its own host port: `HostPort "0"`, as PR #200's diagnosis cites
    (`tc:lifecycle.go:626-650`). On Docker Desktop the allocator keeps one port sequence per binding address (PR #200
    comment 6128508133, run 1 note).
  - Whether two runs at once fail or saturate the host is **unmeasured** (section 2).
- **On CI.** The lock is in the job VM's own `/tmp`. Only the `verify` job runs integration (`ci.yml:25-56`), and
  GitHub gives each hosted job a fresh VM (GitHub's documentation; not probed). So the lock never contends on CI.
- **Bugs and flakes it caused.**
  - #50: pid reuse in a test helper (closed).
  - #124: liveness by pid alone (closed; archived change `2026-10-07-admission-owner-identity`).
  - #126: time zone and locale (closed; `2026-10-08-lock-identity-locale`).
  - #201 (`class:flake`, open): macOS host name follows the network. PR #200 removes the host comparison
    (`openspec/changes/lock-owner-same-machine/` on 5ae5625).
  - #202 (bug, open): `TestEscapedWriterDoesNotHoldTheLock` watchdog under load. Its stated harm is "the lock is held
    by an escaped writer" (`runner_test.go:1168`).
  - Ledger accepted risk: a runner in another time zone still misreads a SemStreams owner
    (`admission-ledger.yaml:210-212`).

### L3. Admission token (Go-side refusal)

- **Code.** `internal/harness/natsfixture/admission.go` (143 lines):
  - `admit` `:30-77` reads `SEMENGINE_DOCKER_ADMISSION_TOKEN`, `…_LOCK_DIR`, `SEMENGINE_EVIDENCE_DIR`,
    `SEMENGINE_NATS_IMAGE`;
  - `ownerLive` `:92-133`;
  - `fixture.go:129` — `adm, err := admit(ctx)`;
  - the runner exports it at `scripts/test-integration.sh:363-366`.
- **Spec.** `nats-fixture/spec.md:9-55` (one requirement, 6 scenarios).
- **Tests.** `admission_test.go:96-312`.
- **Added for.** `design.md:141-144`: "The environment variable alone is never trusted."
- **Second spelling.** `ownerLive` restates the runner's `owner_is_stale` (`test-integration.sh:175-187`) in Go.
  Archived change `2026-10-07-admission-owner-identity` exists because the two drifted (#124).
- **Cost.** #129, open: `go test -tags integration ./natsclient` and `task mutate:check` cannot run an integration
  test. `errors.go:11` — ``ErrNotAdmitted = errors.New("natsfixture: not admitted to Docker; run it through
  `task test:integration -- <package>`")``.
- **Stock.** None, and none is needed without L2.

### L4. Process group, signal forwarding, KILL escalation, log FIFO

- **Code.**
  - `scripts/test-integration.sh:279-311` (jobs);
  - `:313-353` (traps; SIGINT-ignored probe `:329-341`);
  - `:441-506` (set -m group, tee on a FIFO ignoring INT/TERM `:449`, TERM→KILL after grace, reap, bounded tee wait
    `:491-501`);
  - grace knob `:29-43`.
  - About 150 lines.
- **Spec.** `integration-test-runner/spec.md:89-118`.
- **Tests.** `runner_test.go:713`, `:979`, `:1009`, `:1032`, `:1062`, `:1141`, `:1183`.
- **Added for.** Premise P3: "Signalling only the `go` pid orphans test binaries; `t.Cleanup` does not run on SIGINT"
  (`design.md:187-188`). Ledger: "upstream traps do not forward INT/TERM to go test (repaired: process group)"
  (`admission-ledger.yaml:209`).
- **Stock.**
  - `go test` notes SIGINT and stops starting new tests, but forwards nothing
    (`go:src/cmd/go/internal/base/signal.go:13-24`).
  - SIGTERM is not in `signalsToIgnore` (`go:…/base/signal_unix.go:14` — `os.Interrupt, syscall.SIGQUIT`), so
    SIGTERM ends `go` and leaves its test binaries running.
  - Each test binary has its own `-test.timeout`, and `go` kills it at timeout plus up to one minute
    (`go:src/cmd/go/internal/test/test.go:833-843`).
  - A terminal Ctrl-C reaches the whole foreground group without help.
  - Ryuk removes an orphan's containers once that binary exits and the 10 s reconnection timeout passes
    (`tc:internal/config/config.go:81`).
- **Residual failure without L2.** A scripted SIGTERM sent to the runner's pid alone leaves test binaries and their
  containers running for up to the 10-minute timeout. Nothing stock prevents that.
- **Flakes and bugs.** #62 (closed, `class:flake`: the `TestSignalDuringPullReapsThePull` fake), #202 (open), #128
  (open: `now_ms` under a comma-decimal locale with bash 5).

### L5. Leak check by testcontainers session

- **Code.** `scripts/test-integration.sh:509-555`: waits up to 15 s (`:44`), removes survivors by id, fails the run.
  `natsfixture/evidence.go:59-85` (`recordSession`) writes the label first (`fixture.go:138-146`).
- **Spec.** `integration-test-runner/spec.md:120-131`.
- **Test.** `runner_test.go:884`.
- **Added for.** `design.md:145-153`. It is a check that nothing survived, beside Ryuk. The first version used the
  wrong label key and passed silently (`design.md:149-153`).
- **Stock.** Ryuk removes every container carrying the session label once the session's connections drop:
  `tc:reaper.go:158-180`; reconnection timeout 10 s, `tc:internal/config/config.go:81`. Ryuk image
  `testcontainers/ryuk:0.14.0` (`config.go:16`).
- **Survivors ever found** in a recorded run: **unmeasured**. Evidence directories are local only and were not read.
- **Flakes.** None recorded.

### L6. Docker preflight and environment record

- **Code.** `scripts/test-integration.sh:368-386` (`docker info` latency, version, context, endpoint, `DOCKER_HOST`,
  Ryuk env, properties file); `scripts/doctor.sh:56-105`.
- **Added for.** The evidence list at `design.md:170-175`.
- **Stock.** Nothing stock records this. How testcontainers reports an unreachable daemon at the first
  `GenericContainer` call is **unmeasured**.
- **Note.** In PR #200's run A, Docker Desktop's resource saver made the preflight take 40,092 ms (comment
  6128094108, "Side observation").

### L7. Image pin, override and pre-pull

- **Code.**
  - `.nats-image`; `scripts/test-integration.sh:59-83` (digest regex, override needs a reason);
  - `:388-418` (cache check by digest; pull under the lock with a 300 s budget `:28`);
  - Go-side digest check `natsfixture/fixture.go:261-268`.
- **Spec.** `integration-test-runner/spec.md:133-158`; `harness-boundaries/spec.md:68-78` ("One image pin";
  `TestOneImagePin`).
- **Added for.** O6 digest pin (`design.md:62-64`). The override with a reason exists for the forced-failure protocol
  (`design.md:303-306`), a setup-02 pass-evidence item recorded on PR #13.
- **Stock.** testcontainers pulls a missing image itself (`tc:docker.go:1220-1244`), with exponential backoff under
  the caller's ctx (`tc:docker.go:1524-1555`, `attemptToPullImage`). A digest reference passed as `Image` is honoured
  by Docker; testcontainers' handling of it is unmeasured.
- **Flakes.** None. #62 is in the pull-reaping test (L4).

### L8. Evidence directory

- **Code.** `scripts/test-integration.sh:109-126` (`runner.env`, token, git head, `tree_state` `:125`), `:420-432`
  (container, volume and network listings before and after), `:507` (per-package), `:557-561` (`.evidence/last-run`).
  Fixture records: `natsfixture/evidence.go:15-108`, `fixture.go:653-670`; container logs on a failed attempt:
  `fixture.go:386-399`. CI uploads it: `ci.yml:61-69`.
- **Readers.**
  - `scripts/cover-check.sh:51-65` reads `.evidence/last-run`, `go_test_status=0` and `tree_state=` for the
    natsfixture 80% gate (`:24`).
  - Humans: PR #200's diagnosis used "16 integration-run evidence directories … 19 fixture records" (comment
    6128094108, "Prior local evidence").
  - The before and after listings belong to setup-02's pass-evidence protocol items 1, 2 and 4
    (`design.md:290-316`), manual and one-time.
- **Stock.** testcontainers has log consumers (`tc:logconsumer.go`) but writes nothing to disk on failure.
- **Bugs.** #203 (open): Restart's PR #200 retry leaves no evidence record.

### L9. `go test` invocation

- **Code.** `scripts/test-integration.sh:434-437` — `test -race -failfast -tags=integration -count=1 -p 2 -timeout
  10m -coverprofile=…`.
- **Why `-p 2`.** `:439-440` — "inherited from SemStreams gh#736 … unmeasured here". The pin explains it at
  `run-integration-tests.sh:336-341`: "container starts queue behind each other and blow their own start budgets
  (gh#736)".
- **Stock.** These are plain `go test` flags; a Task command could pass them directly.

### L10. natsfixture Start: phases, typed errors, replacement retry, deps layer

- **Code.**
  - `fixture.go:108-166` (Start) and `:224-369` (attempt: image, start, host, mapped-port, connect, JetStream). The
    request at `:285-295` waits with `WaitingFor: wait.ForLog("Server is ready")` (`:292`) only.
  - Replacement retry: `:148-160`, once, only after a mapped-port failure with a live parent and a clean rollback
    (`:157`). It exists for "the one transient SemStreams observed under Docker API pressure (gh#736)" (`:28-30`).
  - `deps.go:27-44`: 13 replaceable calls, each counted. The only replacements are in the fixture's own tests:
    `fixture_test.go` 20, `fixture_integration_test.go` 7, `admission_test.go` 3
    (`git grep -c '\.deps\.[a-zA-Z]+ *='`).
  - Our own NATS dial: `deps.go:90-121`.
  - Errors: `errors.go` (92 lines).
  - Operation slot: `fixture.go:53-56,171-221`.
- **Spec.** `nats-fixture/spec.md:57-77`.
- **Stock.**
  - `wait.ForListeningPort` / `wait.ForMappedPort` (`tc:wait/host_port.go:67,79-81`) re-poll `MappedPort` until it is
    non-zero, within the strategy timeout, default 60 s (`:205-223`).
  - Composed with `wait.ForAll` (`tc:wait/all.go:46`).
  - On Docker Desktop, while a Mac process holds the port on the wildcard address, the binding stays empty and
    appears only when the holder lets go (PR #200 comment 6128094108, h5b). So a stock wait strategy waits out its
    timeout where the replacement container succeeds at once.
  - Loopback publishing avoids wildcard holders but fails the same way with loopback holders (comment 6128508133,
    verdicts 1 and 4).
- **Flakes.** #179 (closed): the fixture's own test misread the replacement retry. #180 (open, `class:flake`): Docker
  Desktop, see section 4.

### L11. Rollback and observed absence

- **Code.** `fixture.go:375-415` (logs, then terminate, then observe absent, each under a fresh 15 s context:
  `rollback.go:9-25`). `stop.go:240-250` (`observeAbsent`). `deps.go:145-167` (`containerAbsent` opens a second
  Docker client).
- **Shape finding: classification by error text.** `deps.go:163` — `if strings.Contains(err.Error(), "No such
  container") {`.
- **Stock.**
  - `Terminate` removes with `Force: true, RemoveVolumes: true` (`tc:docker.go:351-354`) and returns its error.
  - testcontainers classifies not-found through containerd's errdefs (`tc:testing.go:153-160`, `errdefs.IsNotFound`).
  - `tc:testing.go:99-105` (`CleanupContainer`) calls `TerminateContainer`, which uses `context.Background()`
    (`tc:cleanup.go:104`). That is an unbounded cleanup root, against the intent of `harness-boundaries` "Bounded
    cleanup roots" (`spec.md:125-135`). Calling `c.Terminate(boundedCtx)` directly is the stock path that stays
    bounded.
- **Unmeasured.** Whether a nil from Docker's `ContainerRemove` already means the container is absent.

### L12. Stop: owned-resource teardown

- **Code.** `stop.go:27-148`: in order, wait for publishes, delete consumers, streams and buckets while observing
  their absence, drain, terminate, observe the container absent.
- **Spec.** `nats-fixture/spec.md:96-135`.
- **Added for.** The plan's ownership rule "also covers NATS streams, buckets, and consumers when sharing a broker"
  (`docs/setup-plan.md:260-262`). The fixture never shares a broker: "one test one disposable … NATS server"
  (`nats-fixture/spec.md:4-5`). The bounds come from SemStreams (`fixture.go:35-38`, citing
  `test_client.go:911-927`).
- **Stock.** Removing the container removes everything inside it (L11).

### L13. Restart

- **Code.** `restart.go` (193 lines): ready-line count `:125-143`, single mapped-port read `:145-152`. PR #200 adds one
  stop-and-start retry and `restart_port_integration_test.go`.
- **Spec.** `nats-fixture/spec.md:137-177`.
- **Users.** Section 3.
- **Stock.** `wait.ForLog(...).WithOccurrence(n)` exists (`tc:wait/log.go:102`). That `c.Start` re-runs the request's
  original `WaitingFor`, which matches the previous boot's line, is PR #200's reading of `tc:lifecycle.go:211-238` and
  `tc:wait/log.go:142-206` (comment 6128094108). Not re-read here.

### L14. Fixture extras

- `FaultKV` (`faultkv.go`, 144 lines): a `jetstream.KeyValue` wrapper with no Docker dependency.
- `WithMaxPayload` (`fixture.go:80-85`).
- `CreateMemoryStream`, `CreateKeyValue`, `Consume` (`fixture.go:478-616`).
- `Open` (#93 only: `natsfixture/open.go:29`).
- Users: section 3.

### L15. Safe names

- **Code.** `names.go:13-134`: the `semengine-` prefix and SemStreams "destroyer" substrings, listed at `:15-22` with
  their pin sources. `fixture.go:288` names the container. `harness-boundaries/spec.md:80-101`;
  `TestSemEngineAssignedNames`; `CheckName` is used by `internal/harness/contract/docker_test.go`.
- **Added for.** Premise P11: SemStreams removes volumes by name substring (`design.md:202-203`). The rule exists
  whatever happens to the lock.
- **Unmeasured.** Whether Docker's own generated names (no `Name` in the request) can contain a destroyer substring.

### L16. Lifecycle floor on the fixture

- `nats-fixture/spec.md:207-216`; `fixture_integration_test.go:178-709` (S1-1 to S1-8).
- At setup-02 the fixture was the lifecycle suite's first subject (`docs/setup-plan.md:286-298`). Product subjects now
  call `lifecycletest.Run` (`internal/harness/lifecycletest/lifecycletest.go:77`):
  - on main: `natsclient/client_lifecycle_test.go`, `metric/lifecycle_test.go`,
    `internal/lifecycleguard/guard_test.go`;
  - on #93, also `processor/graph-ingest/lifecycle_integration_test.go` (gopls references on fd9463c).

### L17. probe

- `internal/harness/probe` (161 lines). `probe.Await` has 23 calling files on main (gopls). It is a general wait
  helper, not Docker machinery.
- Flake #49 (closed by #44) was its own test.

### L18. prochost

- `internal/harness/prochost/prochost.go` (225 lines): re-executes the test binary in its own process group.
- Users: none outside its own tests on main; one on #93 (`processor/graph-ingest/settlement_integration_test.go`).
- No flakes recorded.

### L19. Coverage coupling

- `scripts/cover-check.sh:19-28,51-65`: natsfixture's 80% floor is measured from the last integration run's profile,
  which the runner writes (`:435`).
- Setup-02 ruling O7 (`design.md:65-66`).
- Any change to the runner or the fixture changes what this gate reads and measures.

## 2. Two runs on one daemon (#205 item 2): facts and the experiment, not run

**Facts.**

- testcontainers separates runs by session and Ryuk (L2 stock).
- The fixture creates no volume or network (`nats-fixture/spec.md:83-84`).
- Container names carry a random suffix (`names.go:121-134`).
- Host ports come from Docker.
- No same-daemon concurrent run has ever been recorded: the setup-02 pass evidence serialised them
  (`design.md:295-298`). `TestS1_8Isolation` (`fixture_integration_test.go:651`) proves two fixtures in one process
  are disjoint, not two runs.

**Bypass without a code change.** The runner reads `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR` at
`scripts/test-integration.sh:49`. It refuses only the signal-grace knob on the shared lock (`:37-42`). The fixture
admits against whatever lock directory it is handed (`admission.go:40,45`). So a private lock directory per run
bypasses the shared lock. Nothing in the repository changes; the copies keep runs out of the worktrees' `.evidence`.

**Experiment.** Run it only at a time the owner picks, when no other session runs integration:

```bash
# Preconditions, before every pair:
test ! -e /tmp/semstreams-integration.lock
docker ps -q --filter label=org.testcontainers=true   # must print nothing
BASE=<main after #93 and #200 merge>
X=$(mktemp -d /tmp/semengine-par.XXXX); mkdir -p "$X/a" "$X/b" "$X/ev"
git -C /Users/coby/Code/c360/semengine archive "$BASE" | tar -x -C "$X/a"
git -C /Users/coby/Code/c360/semengine archive "$BASE" | tar -x -C "$X/b"
run() { ( cd "$X/$1" && SEMENGINE_DOCKER_ADMISSION_LOCK_DIR="$X/$1-$2-$3.lock" \
          SEMENGINE_EVIDENCE_DIR="$X/ev/$2-$1-$3" scripts/test-integration.sh ./... ); echo $? > "$X/ev/$2-$1-$3.rc"; }
for i in $(seq 1 10); do run a serial "$i"; done                              # baseline
for i in $(seq 1 30); do run a pair "$i" & run b pair "$i" & wait; done       # 30 pairs
```

- **Run count.** 10 serial runs, then 30 pairs (60 runs). If none of F1 to F4 appears in 30 pairs, the per-pair
  failure rate is bounded below about 10% at 95% (rule of three).
- **Failures.**
  - F1: a test fails in a paired run but passed in all 10 serial runs.
  - F2: `leak_check` in `runner.env` is not `clean`.
  - F3: a fixture record shows a start or stop phase error. `port "4222/tcp" not found` is tallied apart as #180's
    class, and its paired rate is compared with its serial rate.
  - F4: one run's container vanishes while that run still uses it (`No such container`, or the connection closed, in
    its `go-test.log`).
- **Measured, not failures** (the plan's saturation question, `setup-plan.md:266-267`): per-package wall time
  (`per-package.txt`), fixture phase `elapsed_ms`, `docker_info_ms`, serial against paired.
- **Linux.** The same commands as two background runners inside one throwaway CI job, 10 pairs. That needs a branch
  and a workflow, so it is an owner question.
- **SemEngine beside SemStreams.** SemStreams' runner has the same bypass (`SEMSTREAMS_INTEGRATION_LOCK_DIR`, pin
  `run-integration-tests.sh:9-11`). Running SemStreams' suite goes beyond its row in `docs/inventory-scope.md`, so it
  needs the owner's word.
- **Runner notes.**
  - A runner started as an `&` job from a script starts with SIGINT ignored. It warns and records
    `int_ignored_on_entry=yes` (`:16-19`). Expected here.
  - In a git-less copy, `tree_state` and `git_head` record `unknown` (`:124-125`). `cover:check` is not part of the
    experiment.

## 3. Which tests need what (#205 item 3)

Found with `gopls references` on each natsfixture API and on each package helper (`GOFLAGS=-tags=integration`): main
in the worktree, #93 in the `git archive` copy. Files listed: `git grep -l '^//go:build integration'`.

### Main, 30 files

- `internal/harness/natsfixture/fixture_integration_test.go`: the fixture's own tests. It uses every feature, and it
  also runs the `docker` CLI directly at `:38,:155,:836,:948,:960`.
- `internal/tlsutil/mtls_integration_test.go`: **no container, no NATS** (httptest).
- `natsclient`: 27 test files and 1 helper. The helpers are `startFixture` (`test_helpers_integration_test.go:26-35`),
  called from 17 files, and `newFixtureClient` (`:48-75`), called from 12; `subscription` uses both. Every one needs a
  container. Beyond a plain server:
  - Restart: `natsclient/integration_test.go:75` (TestIntegration_ReconnectionIsRedial `:52`) and `:141`
    (TestIntegration_AcknowledgedIsNotDurableOnMemoryStream `:109`). The same file also uses `CreateMemoryStream`,
    `CreateStream` and `Name`.
  - Server goes away: `natsclient/storage_inventory_integration_test.go:249` — `fixture.Stop(stopCtx)` (test at
    `:214`).
  - Max payload: `natsclient/request_response_bounds_integration_test.go` (`WithMaxPayload` ×4).
  - `CreateStream`: `stream_visibility_integration_test.go`, `test_helpers_integration_test.go`.

### PR #93 adds 41 files (fd9463c)

- `component/registry_integration_test.go`: container via `Open` (`fixtureClient` at `:27`).
- `component/schema_integration_test.go`: **no container, no NATS**.
- `graph/kvcatalog/*` (4 files): container (`fixture_client_integration_test.go:23`).
- `pkg/lifecycle/*` (4 files): container (`harness_gate_integration_test.go:71`, which also uses `CreateStream`;
  `manager_integration_test.go:39`).
- `processor/graph-ingest/*` (31 files): all start a container, through `newFixtureClient` `:64`,
  `startBatchTestComponent` `:84`, `startPrefixTestComponent` `:116`, `createTestComponentForLifecycle`
  (`lifecycle_integration_test.go:35`), `createTestComponentWithHierarchyConfig` (`hierarchy_integration_test.go:117`),
  or `natsfixture.New` directly. Beyond a plain server:
  - Restart: `guard_generation_integration_test.go:115` (test `:70`) and `:194` (test `:155`), with
    `CreateMemoryStream` ×4.
  - FaultKV: `commit_ambiguity_integration_test.go` (`FailAfter` ×2, `Calls`) and `settlement_integration_test.go`
    (`FailBefore` ×2, `Calls`).
  - prochost: `settlement_integration_test.go`.
  - `JetStream()` used directly: guard_generation 4, long_apply 3, query_verbs 2, settlement 5.

### PR #200

- Adds `internal/harness/natsfixture/restart_port_integration_test.go`, the fixture's own test.

### Totals on fd9463c (71 files)

- 69 start a container; 2 start nothing.
- Restart: 4 product tests in 2 files.
- Server stopped mid-test: 1 file.
- Max payload: 1 file.
- FaultKV: 2 files.
- prochost: 1 file.
- **No caller outside natsfixture's own tests, on main or #93: `CreateKeyValue` and `Consume`.** Consume drives the
  handler-join machinery in Stop and Restart (`fixture.go:528-616`, `stop.go:150-200`).

### Already in-process today

Four natsclient **unit** tests start nats-server inside the test process:

- `natsclient/client_connect_test.go:114,287`
- `client_lifecycle_test.go:110`
- `client_close_held_test.go:152,209`
- `client_close_final_test.go:185`

setup-02 rejected in-process for the fixture in O1(b) (`design.md:36-42`): it "cannot produce #6's Docker pass
evidence", it enlarges the govulncheck surface, a server panic kills the test binary, and it drifts from the image's
version.

## 4. Mac against CI, per recorded flake and harness bug (#205 item 4)

| Issue | Where seen | Cause | Stock answer |
|---|---|---|---|
| #49 flake, closed by #44 | CI runs 36913744883, 36917193756 | probe's own test counted ticks in a 200 ms window | none needed: test made deterministic |
| #62 flake, closed by #63 | local macOS under load; "No CI run has shown this failure" (#62) | runner contract test read a pid file early | exists only with L4 |
| #144 flake, closed by #156 | CI run 37927038481 | product test asked before graph-ingest subscribed | not machinery |
| #179 flake, closed by #118 | local Mac | fixture test misread the L10 replacement retry | exists only with L10's retry |
| #180 flake, open | Docker Desktop for Mac: Docker Desktop's API proxy drops a binding a Mac process holds on the wildcard address (PR #200 comment 6128094108, "Cause") | Docker Desktop, not our code | **CI unmeasured.** The diagnosis says "Linux CI: dockerd binds the host port itself there, so this class should not occur; not tested". Stock waits do not help while the holder stays (L10). Loopback publishing only moves the clash (6128508133). A Docker Desktop setting is unmeasured. Ryuk's own port is exposed the same way (6128508133, `tc:reaper.go:379-405`, not run). |
| #201 flake, open; PR #200 closes it | macOS host name | lock owner record (L2) | exists only with L2 |
| #202 bug, open | local Mac, load average 5.8–8.4 | runner test watchdog | exists only with L2 and L4 |
| #128 bug, open | bash 5 with a comma-decimal locale; macOS `/bin/bash` 3.2 unaffected (#128) | runner `now_ms` | exists only with L4 and L8 timings |
| #50, #124, #126 bugs, closed | local | lock liveness (L2 and L3) | exist only with L2 and L3 |
| #203 bug, open | — | PR #200's Restart retry writes no evidence | exists only with L8 and L13 |

## Categories (contract)

1. **Claimed gap.**
   - "No written guidance on test infrastructure": `docs/testing.md` covers how to use the runner and fixture
     (`:57-69`, `:554-569`) but sets no rule on what harness code may exist
     (`grep -n -i -E 'lock|runner|admission|ryuk|testcontainers|in-process|embedded'` → only usage lines `:66-69`,
     `:561-562`).
   - The plan has the policy fragments (`docs/setup-plan.md:258-271`). The gap is real.
2. **Every spelling of the facts modelled.**
   - Owner liveness: twice, `test-integration.sh:175-187` and `admission.go:92-133` (plus doctor's simpler one,
     `doctor.sh:107-122`).
   - Container cleanup: four places: Ryuk (`tc:reaper.go`), fixture Stop (`stop.go:120-141`), fixture rollback
     (`fixture.go:375-415`), runner leak check (`:509-555`).
   - Mapped-port recovery: Start (`fixture.go:148-160`) and Restart (PR #200).
   - Session label: `evidence.go:59-85` and `runner:519-532`.
   - Image pin: `.nats-image`, `runner:59-83`, `fixture.go:261-268`, `admission.go:23`.
3. **Adjacent claims.**
   - Specs: `integration-test-runner`, `nats-fixture`, `harness-boundaries` (`:68-101`, `:125-146`);
     `docs/testing.md`.
   - Ledger rows L3 (`admission-ledger.yaml:148`), L6 (`:196`), L8 (`:231`), `natsclient/mapped_port_retry_test.go`
     (`:136-146`, defer-exclude: "natsfixture owns the one mapped-port replacement").
   - Issues: #129, #195 (`natsfixture.Open` adoption, blocked), #201, #202, #203, #128, #180.
   - Open PRs in scope paths (`gh api --paginate …/pulls/<n>/files`):
     - **#200** (draft): `natsfixture/{admission,deps,fixture,restart}.go`, tests, `runner_test.go`, `doctor.sh`,
       `test-integration.sh`, deltas `lock-owner-same-machine/specs/{integration-test-runner,nats-fixture}` and
       `natsfixture-restart-port/specs/nats-fixture`.
     - **#93** (ready, 376 files): `Taskfile.yml`, `docs/testing.md`, `go.mod`, `scripts/cover-check.sh`,
       `internal/harness/contract/*`, `natsfixture/{open.go,open_test.go,fixture_integration_test.go}`,
       `openspec/specs/{harness-boundaries,nats-fixture}/spec.md`.
     - #188 and #194: none in scope paths.
     - #206: this claim, 0 files.
     - Both #200 and #93 merge before this change builds anything (#205 "Order").
4. **Who it is for.** The inventory proposes no new surface.
5. **Problem shape.**
   - "Clean up after a test process, including one that dies": the stock instance is Ryuk plus `t.Cleanup`
     (`tc:reaper.go:158-180`, `fixture.go:95-101`).
   - "Mutual exclusion across processes": the only instance is L2. setup-02 rejected `flock` because "macOS has no
     `flock` CLI" (`design.md:47-48`).
6. **Pin decision records.**
   - No package is carried from the pin.
   - The runner is adapted (L6). I fetched all 85 records at 8b99efe9 and searched them for
     `integration-tests|integration.lock|testcontainers|ryuk|host lock|mapped.port|run-integration`. The only hit is
     ADR-025 line 138, a passing mention of "integration tests (testcontainers)".
   - A search for `test infra|flake|docker daemon` hits ADR-037, 043 and 070 in other senses.
   - No record governs this machinery.

**Intent check.** Not triggered: no boundary of the port set, tier or capabilities moves.

## Collision table: container cleanup and daemon admission

| Dimension | Evidence |
|---|---|
| Semantic class | (a) removing a run's containers; (b) deciding who may use the daemon |
| Owners | (a) Ryuk `tc:reaper.go:158-180`; fixture Stop `stop.go:120-141`; rollback `fixture.go:375-415`; leak check `runner:509-555`. (b) lock `runner:137-277`; admission `admission.go:30-77`; SemStreams' runner (pin `:208-242`) |
| Catalogs | session label file `evidence.go:76`; lock owner file `<lock>/owner` |
| Status | `runner.env` `leak_check`, fixture JSON `remaining`; doctor `admission` line `doctor.sh:111-122` |
| Lifecycle | Ryuk: session end + 10 s (`config.go:81`); Stop: `t.Cleanup` with 60 s (`fixture.go:34,95-101`); lock: quarantine of a dead owner `runner:189-202` |
| Ownership | session = parent pid + create time (`bootstrap.go:86-93`); lock token `runner:106` |
| Readers | leak check reads the session file; admission reads the owner file; `cover-check.sh` reads `runner.env` |
| Writers | fixture writes the session file; runner writes the owner file and `runner.env`; SemStreams' runner writes the same owner file |
| Recovery | Ryuk; quarantine; "a lock directory without an owner file … stays until it is removed by hand" (`integration-test-runner/spec.md:24-25`) |

## Adopter seam inventory

Two seams are reached from outside this code: a developer writing an integration test, and SemStreams' runner.

1. **What must a developer know today?**
   - Run only through `task test:integration` (`docs/testing.md:59-69`).
   - Another run in either repository makes theirs fail fast unless they set
     `SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS` (`runner:49-54`).
   - Call `New`, then `Start`.
   - Restart may move the port (`restart.go:28-30`).
   - That is four facts. More than two is a design finding.
   - SemStreams' runner must keep reading SemEngine's owner record as an ordinary owner (ledger L6 contract,
     `admission-ledger.yaml:202-205`).
2. **If they do nothing:**
   - plain `go test -tags integration` returns `ErrNotAdmitted`, loud and naming the task;
   - a concurrent session prints the owner and exits non-zero;
   - a stale lock from another host needs manual recovery (`design.md:360`).
3. **Where they find out.** A typed runtime error, or the runner's exit message. The lock-wait knob is only in a doc
   (`docs/testing.md` does not mention it) and in the runner's failure message.
4. **What they should have to know.** Ideally `natsfixture.New(t)` then `Start(ctx)`, and nothing about other runs or
   repositories. The gap is items 2 to 4 of point 1.

## Open evidence questions

1. Do two runs at once fail or saturate (section 2)? On Mac and on Linux.
2. Has #180 ever happened on CI? Count the CI evidence artifacts' fixture records.
3. Has the leak check ever found a survivor that Ryuk later removed?
4. Does a nil from `ContainerRemove` already mean the container is absent? This decides L11's observe step.
5. Can Docker's own generated names contain a destroyer substring? This decides whether L15 needs `Name`.
6. Is there a Docker Desktop setting (networking mode, port allocation) that ends #180? Not probed.
7. `-p 2` against `-p` unset, on both platforms (L9).
