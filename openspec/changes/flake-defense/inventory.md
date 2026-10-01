# Inventory: flake-defense

- base: `97e53e630c56bb8f68cac188c045f6899f9cfd20` (tree `26bde15e9135a5efff678effe5e7eea4838ddc8d`, identical to
  `origin/main` `6a006df7ff8b108aed59cf886b862d55cb1e9616`; the one extra commit is the empty claim)
- semstreams (code facts only): `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read through `gh api ... ?ref=`
- unmerged branch read: `origin/claude/harness-lessons` at `74db5cc9a64d82237a7405a48a9cebc9611e2a2b` (PR #39)
- measured: 2026-10-01; issue #42, claim PR #44

This is the architect's inventory-only deliverable (contract step 2). It contains no target state, options or
recommendation, and it has not yet had independent inventory review.

## 0. The question and the reading scope

**Question.** What lets a flaky test be born, be merged, and survive in this repository today; and what does the
SemStreams record show about how its flakes were born and what closed them?

**Scope.** `docs/inventory-scope.md` limits SemStreams to code facts at the pin. SemStreams' GitHub issue record for
`class:flake` (23 issues, their comments, and the pull requests that closed them) was read for this question on the
owner's direct word of 2026-10-01, quoted on #42 and #40: "flaky tests have to not only be fixed but we have to stop
the bleeding properly. flaky tests were part of what spelled doom for semstreams". Seven SemStreams issues without
that label (#199, #209, #220, #750, #1283, #1414, #1417) were read by title, state and, for #220, body, because the
labelled issues and this repository's carried scripts cite them. No other sister repository was read. No git command
ran in a sister checkout.

Pins are `path:line` with the line's text, at the base unless a row says otherwise. GitHub facts carry the `gh`
query that produced them. Statements about GitHub or Go behavior that were not exercised here are marked
**documented, not measured**.

## 1. Premises in #40, #42 and the briefing that measurement changed

| Premise | Source | Measured |
|---|---|---|
| "Nothing runs a new test more than once before merge" | #42, Why now | Each untagged test runs twice under `-race` per `task verify` (unit step, then again inside the integration step, which defaults to `./...`), and `lifecycletest` and `probe` run a third time without `-race` in `cover:check`. Every run is `-count=1`. Section 3. |
| The failure is in `task verify` then `task test:unit` | #40, What fails | Run 36869511251 failed it inside `test:integration` (`verify: FAILED: test:integration cover:check`); runs 36871402708 and 36872498596 failed it inside `test:unit`. |
| Two occurrences | #40, Occurrences | Three: 36869511251 (PR #21, `be2203c`), 36871402708 (PR #39, `c799b31`), 36872498596 (PR #21, `41137eb`). |
| The root cause of #40 is not available | briefing | Issue #41 (opened 2026-10-01T14:11Z, 14 minutes after #40, for the third occurrence) states a cause and a reproduction. PR #43 claims #40 only; #41 is unclaimed. Section 5. |
| `task verify` runs nine gates ending in `test:unit` | `AGENTS.md:43` | `scripts/verify.sh:10-11` runs thirteen, including `cleanup-roots:check`, `ledger:check`, `test:integration` and `cover:check`. |
| SemStreams closed 21 of 23 `class:flake` issues | #42, Why now | Count confirmed. Two of the 21 closed with no fix recorded for the class (#736, #1421). Section 6. |

Pins for the fifth row:

- `AGENTS.md:43` — `task verify       # spec:check docs:check fmt:check tidy:check build vet lint vuln test:unit,
  cheapest first;`
- `scripts/verify.sh:10` — `steps=(spec:check docs:check fmt:check tidy:check cleanup-roots:check build vet lint
  vuln ledger:check test:unit`
- `scripts/verify.sh:11` — `test:integration cover:check)`

## 2. Birth: what stops a timing-dependent test from being written or merged

The complete set of failing commands is the thirteen steps of `scripts/verify.sh:10-11` plus `revive.toml`'s rule
list (`revive.toml:10-36`, none about time, sleeps, parallelism or skips). Of the eight shapes the brief names, two
have a failing command and six do not. Skips are added below as a seventh row without one.

### 2.1 Shapes with a failing command

**Fixed ports and fixed broker addresses.**

| Pin | Line text | Role |
|---|---|---|
| `Taskfile.yml:53` | `- scripts/lint-test-ports.sh` | guard, in `task lint` |
| `Taskfile.yml:54` | `- scripts/lint-test-ports_fixture_test.sh` | its sensitivity test (shell fixture matrix) |
| `internal/harness/contract/addresses_test.go:48` | `func TestLintTestPortsPasses(t *testing.T) {` | runs the script from `task test:unit` |
| `internal/harness/contract/addresses_test.go:21` | `func TestNoFixedAddressesInTests(t *testing.T) {` | guard for four broker literals (`:14-19`) |
| `internal/harness/contract/addresses_test.go:26` | `func TestNoFixedAddressesInTestsSensitivity(t *testing.T) {` | plants each literal, requires the file and line to be named |
| `openspec/specs/harness-boundaries/spec.md:73` | `### Requirement: No fixed addresses in tests` | requirement |

What the port guard does not match, from its own header and patterns:

- `scripts/lint-test-ports.sh:13-20` lists known false negatives: multi-line calls, string concatenation, variable
  indirection, and every API other than `net.Listen` (`:19` — `#   - Non-net.Listen APIs: http.Server{Addr:
  fmt.Sprintf(":%d", N)} etc.`).
- `scripts/lint-test-ports.sh:41` — `SUPPRESS_MARKER='// gh#220:allow-fixed-port'`: an inline exemption with no
  recorded reason. Uses in this tree: 0 (`grep -rn 'gh#220:allow-fixed-port' --include='*.go' .`).
- Probe-then-bind (bind `:0`, read the port, close, bind again later) passes both patterns. It is SemStreams #1120.
  The guard's own fix message recommends that helper: `scripts/lint-test-ports.sh:53` — `echo 'See
  service/service_manager_health_listener_test.go freePort() helper for the canonical pattern.'`. That path does
  not exist in this repository, and #1120 names `freePort` at that path as the race.

**Unbounded cleanup contexts.**

| Pin | Line text | Role |
|---|---|---|
| `Taskfile.yml:59` | `- scripts/cleanup-roots-check.sh` | guard, own `verify` step |
| `internal/harness/contract/cleanuproots_test.go:18` | `func TestCleanupRootsCheckSensitivity(t *testing.T) {` | plants three forms, requires `x/x_test.go:4` named; proves the bounded form passes |
| `openspec/specs/harness-boundaries/spec.md:83` | `### Requirement: Bounded cleanup roots` | requirement |

- `scripts/cleanup-roots-check.sh:13` — `# One line holding both the call and the unbounded root. --untracked
  includes new`. The pattern at `:15` matches `Stop`, `Close` or `Terminate` with `context.Background()` or
  `context.TODO()` on the same line. A context assigned on one line and passed on another, and any other method
  name (`Shutdown`, `Drain`, `Wait`, `Kill`), are not matched. PR #39's rules table says the same at
  `AGENTS.md:62` on that branch: "other unbounded roots are review only".

### 2.2 Shapes with prose only

| Shape | Failing command | Prose homes | In the tree at base |
|---|---|---|---|
| `time.Sleep` in tests | none | `.agents/contracts/semengine-developer.md:187`; `.agents/contracts/semengine-reviewer.md:173`; `docs/setup-plan.md:243`; `docs/testing.md:158` on PR #39 | 0 hits: `git grep -n -E 'time\.Sleep' -- '*.go'` |
| Wall-clock assertions | none | `semengine-developer.md:188`; `semengine-reviewer.md:173` | one elapsed assertion, `internal/harness/natsfixture/fixture_test.go:181`; see budgets below |
| Unsynchronised goroutine observation | none; `-race` does not flag it (all three #40 failures ran under `-race`) | `openspec/specs/lifecycle-suite/spec.md:16`; `docs/testing.md:158-159` on PR #39 | the #40 shape, `internal/harness/lifecycletest/refowner_test.go:147-150` read at `:187` |
| `t.Parallel` with shared state | none | `semengine-developer.md:187`; `semengine-reviewer.md:172`; `docs/testing.md:169` on PR #39 | 18 `t.Parallel()` calls, all in `internal/harness/runner/runner_test.go` (first at `:274`) |
| Test-order dependence | none; no `-shuffle` anywhere (`git grep -n -i shuffle` finds only an unrelated word at `semengine-reviewer.md:31`) | `docs/testing.md:168` on PR #39 | not measurable without a shuffled run |
| Reliance on container or broker timing | none for the test's own assertions; the fixture and runner bound their own phases (below) | `docs/setup-plan.md:243` | 11 integration tests in one tagged file |
| Skips | none | `.agents/skills/semengine-preflight/SKILL.md:78` | one, `internal/harness/runner/runner_test.go:264` |

Prose pins:

| Pin | Line text |
|---|---|
| `.agents/contracts/semengine-developer.md:187` | ``- Use ephemeral ports, explicit synchronization instead of sleeps, and no `t.Parallel()` around process-global state`` |
| `.agents/contracts/semengine-developer.md:188` | ``such as `slog.SetDefault`. Explain wall-clock assertions and give them realistic tolerance.`` |
| `.agents/contracts/semengine-reviewer.md:172` | ``- Network listeners use ephemeral ports. Tests mutating global state such as `slog.SetDefault` are not parallel.`` |
| `.agents/contracts/semengine-reviewer.md:173` | `- Wall-clock assertions have a rationale and realistic tolerance; concurrent tests use explicit synchronization.` |
| `docs/setup-plan.md:243` | `Wait for authoritative readiness and expected state with deadlines. Avoid fixed sleeps and shared default ports.` |
| `openspec/specs/lifecycle-suite/spec.md:16` | `as subtests. The suite SHALL NOT use aggregate goroutine or memory counts as proof of joins.` |
| PR #39 `AGENTS.md:53` | `The linked file is the rule; this table is only its index. "Review only" means no command fails when the rule is` |
| PR #39 `AGENTS.md:77` | the row "Tests use an independent oracle and are shown able to fail", enforced by "the structural guards carry paired sensitivity tests (`internal/harness/contract`); elsewhere review only" |
| PR #39 `docs/testing.md:158` | ``- Wait on a signal, never on `time.Sleep`. Prefer a channel, callback or `sync.WaitGroup`; next, an injected clock;`` |

The sleep rule therefore has four prose homes and no command. PR #39's rules table has no row for sleeps,
wall-clock assertions, `t.Parallel`, test order or skips.

### 2.3 Time budgets already in the harness

These are numbers a test or a suite waits against. SemStreams #1284, #1290 and #1397 are each a number of this kind
that host load exceeded.

| Pin | Line text | What it bounds |
|---|---|---|
| `internal/harness/lifecycletest/lifecycletest.go:26` | `startBound = 2 * time.Minute` | a Start that never returns |
| `internal/harness/lifecycletest/lifecycletest.go:27` | `stopBound  = 30 * time.Second` | the suite's controlled Stop |
| `internal/harness/lifecycletest/lifecycletest.go:28` | `grace      = 2 * time.Second` | wait past a call's own deadline before reporting the owner ignored it |
| `internal/harness/lifecycletest/lifecycletest.go:301` | `timer := time.NewTimer(max(bound, 0) + grace)` | the wall-clock classification every adopter of `Run` executes |
| `internal/harness/probe/await.go:13` | `const pollInterval = 20 * time.Millisecond` | the sanctioned polling probe |
| `internal/harness/natsfixture/deps.go:20` | `dialTimeout = 5 * time.Second` | fixture connect |
| `internal/harness/natsfixture/fixture.go:33` | `cleanupBudget = 60 * time.Second` | fixture cleanup |
| `internal/harness/natsfixture/rollback.go:12` | `const rollbackBudget = 15 * time.Second` | rollback of a failed start attempt |
| `internal/harness/natsfixture/fixture_test.go:181` | `if elapsed := time.Since(began); elapsed > dialTimeout/2 {` | elapsed-time assertion: a 100 ms context must return within 2.5 s |
| `internal/harness/natsfixture/fixture_integration_test.go:468` | `short, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)` | a deadline the test expects to expire |
| `scripts/test-integration.sh:43` | `readonly leak_wait_seconds=15    # Ryuk reaps a dead session's containers after 10s` | leak check |

The parent of the #40 subtest takes 2.01 s in every CI log read; `grace` is 2 s.

### 2.4 Container and broker timing: what the harness already absorbs

- `openspec/specs/nats-fixture/spec.md:9` — `` `natsfixture.Start` SHALL refuse with ErrNotAdmitted, naming `task
  test:integration`, unless``: a test cannot reach Docker outside the runner and its host lock.
- `openspec/specs/nats-fixture/spec.md:26` — `directory. At most one replacement attempt is permitted, only for
  the mapped-port phase with a live parent and`. Implemented at `internal/harness/natsfixture/fixture.go:126` —
  `for attempt := 1; attempt <= maxAttempts; attempt++ {`. This is a retry that exists today; each attempt is
  recorded in the evidence directory. `fixture.go:27-28` attributes it to "the one transient SemStreams observed
  under Docker API" load.
- `scripts/test-integration.sh:402` — `argv=(test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m`;
  `:407-408` records that `-p 2` is "inherited from SemStreams gh#736 ... unmeasured here".
- Nothing refuses a test that asserts on broker state immediately after an operation that the server applies
  asynchronously (the SemStreams #1069 and #1375 shape). `probe.Await` exists for it; using it is voluntary.

## 3. Merge: how often a test runs before merge, and what a rerun leaves behind

### 3.1 Executions per verification

| Step | Pin | Line text | Flags that matter |
|---|---|---|---|
| `test:unit` | `Taskfile.yml:74` | `- scripts/gopkgs.sh go test -race -count=1 ./...` | `-race`, once, default `-p`, Go's default 10 m timeout, source order, no `-failfast` |
| `test:integration` | `scripts/test-integration.sh:402` | `argv=(test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m` | `-race`, once, two packages at a time, stops a package at its first failure |
| (its package set) | `scripts/test-integration.sh:56` | `((${#packages[@]} > 0)) \|\| packages=(./...)` | every package, so untagged tests run a second time here beside the container tests |
| `cover:check` | `scripts/cover-check.sh:22` | `go test -count=1 -coverprofile="$unit" "./internal/harness/lifecycletest/" "./internal/harness/probe/" > /dev/null` | third run of two packages, no `-race`, output discarded |

Confirmed in the log of main's run 36856716070: `lifecycletest` reports twice (3.038 s; 3.048 s with coverage) and
`cover: lifecycletest 89.5% (unit profile)` a third time. Step timings in that run: `test:unit` 21 s,
`test:integration` 33 s, `cover:check` 2 s.

The canonical integration invocation is also a spec requirement and a test. The requirement is at
`openspec/specs/integration-test-runner/spec.md:76`, which begins "The runner SHALL run" followed by the argv
above; it is asserted at `internal/harness/runner/runner_test.go:420` and recorded at
`docs/admission-ledger.yaml:109`.

No invocation sets `-count` above 1, `-cpu`, `GOMAXPROCS`, or `-shuffle`
(`git grep -n -E 'synctest|GOMAXPROCS|-cpu[ =]|goleak|GOFLAGS'`: 0 hits).

### 3.2 Where and on what

| Fact | Pin or query | Value |
|---|---|---|
| Triggers | `.github/workflows/ci.yml:4` and `:7` | `push:` (main only, `:5`) and `pull_request:` |
| Superseded PR runs | `.github/workflows/ci.yml:13` | `cancel-in-progress: ${{ github.event_name == 'pull_request' }}` |
| Runner | `.github/workflows/ci.yml:21` | `runs-on: ubuntu-latest`; image `ubuntu-24.04` version `20260927.320.1` in run 36856716070 |
| Job bound | `.github/workflows/ci.yml:22` | `timeout-minutes: 15` |
| The gate | `.github/workflows/ci.yml:50` | `- run: task verify` |
| Local | `AGENTS.md:47` | ``Run `task verify` before every implementation push; CI runs the same commands in two jobs, `verify` and `required`.`` (prose) |
| Local host | `sysctl -n hw.ncpu`, `go version` | 12 logical CPUs, `go1.26.6 darwin/arm64`, shared by several agent sessions |
| Local reds | `.agents/protocol.md:42` | `worktrees fix the git collision, not the CPU one. CI is the arbiter; a local red under contention is not a finding.` |

The CI runner's CPU count and memory are not recorded in any run evidence (`grep -n -E 'nproc|cpu|CPU|GOMAXPROCS|sysctl'
scripts/doctor.sh scripts/test-integration.sh`: 0 hits). The repository is public (`gh repo view --json visibility`).

So before merge a new or changed untagged test runs twice under `-race` at `-count=1` per CI run, on one hosted
runner, for each head pushed; a tagged integration test runs once. After merge it runs the same way once on `main`.

### 3.3 The required job and the ruleset

`gh api repos/C360Studio/semengine/rulesets/24272345` (`main-required-checks`, enforcement `active`, target
`~DEFAULT_BRANCH`):

| Rule | Value |
|---|---|
| `required_status_checks` | one context, `Required` (integration 15368); `strict_required_status_checks_policy: false` |
| `pull_request` | `required_approving_review_count: 0`; `allowed_merge_methods: ["squash"]` |
| `bypass_actors` | `[]`; `current_user_can_bypass: "never"` |
| other | `deletion`, `non_fast_forward` |

- `.github/workflows/ci.yml:67-70` — job `required`, `needs: [verify]`, `if: always()`; `:76` — `RESULTS: ${{
  join(needs.*.result, ' ') }}`. It reads only the result of `verify` in the same attempt.
- `.agents/protocol.md:50` — `undraft, then CI green with **no known unfixed flake in a required job** (a fresh
  green over a known flake is`; `:51` — `rerun-to-green: fix it, or file it and obtain an explicit owner waiver
  recorded as a PR comment), then squash`. Restated at `AGENTS.md:70` and
  `.agents/skills/semengine-preflight/SKILL.md:82`. No command reads this rule.
- PR #39's rules table lists the merge row as enforced by "CI job `required`" and names only "claim before work
  and close by merged PR" as review only (`AGENTS.md:69` on that branch). The flake clause is not in that row.
- Repository settings (`gh api repos/C360Studio/semengine`): `allow_auto_merge: false`, `allow_update_branch:
  false`. There is no merge-queue rule.

### 3.4 Reruns and re-rolls

- `gh api 'repos/C360Studio/semengine/actions/runs?per_page=100'`: 40 runs in the repository's life (37 `CI`, 3
  `Dependabot Updates`). `CI`: 29 success, 5 failure, 2 cancelled, 1 in progress. Maximum `run_attempt`: 1. No run
  has ever been re-run here.
- The five failures: `docs:check` on `f6ceab4` (PR #12); `TestIgnoredInterruptIsDetected` on `ec8e685` (PR #13,
  failed in both the unit and integration steps of one run; `b8ec759` "fix(runner): portable SIGINT probe ..."
  followed two commits later); and the three #40 occurrences.
- `github.run_attempt` is available to a workflow and is already used:
  `.github/workflows/ci.yml:59` — `name: integration-evidence-${{ github.run_id }}-${{ github.run_attempt }}`.
  The run object carries `run_attempt` and `previous_attempt_url` (null at attempt 1, observed on run 36871402708).
- **Documented, not measured:** a re-run keeps the run id, increments `run_attempt`, and replaces the check result
  on the commit; earlier attempts stay readable at `/actions/runs/{id}/attempts/{n}`. A new push creates a new run
  at attempt 1.
- A re-roll by push has already happened twice, with no rerun involved:

| PR | Red head (run) | Later heads | Result |
|---|---|---|---|
| #39 | `c799b31` (36871402708, 13:48Z) | `ef8b722`, `74db5cc` | both success, attempt 1 |
| #21 | `be2203c` (36869511251, 13:33Z) | `8a4c2b8`, `61fcfbe`, `d8dd15a` | all success, attempt 1; then `41137eb` failed again (13:56Z) |

- PR #39's current head `74db5cc` has `Required` green while #40 is open. The ruleset would admit that merge once
  the PR is undrafted; only the prose rule stands in the way. #40 was filed at 13:57Z, 24 minutes and three green
  heads after the first occurrence.
- `.github/workflows/ci.yml:15-16` — `permissions:` / `contents: read`. No job in CI can create or comment on an
  issue or a pull request. The repository default is `default_workflow_permissions: write`
  (`gh api .../actions/permissions/workflow`); the workflow narrows it.
- Evidence of a failed attempt is an artifact kept for 14 days: `.github/workflows/ci.yml:63` — `retention-days: 14`.

## 4. Survival: what finds a latent flake, files it, or notices a skip

- **No run without a push.** `ci.yml:3-7` has no `schedule` and no `workflow_dispatch`
  (`git grep -n -i -E 'schedule:|cron|workflow_dispatch|merge_group'` finds only `.github/dependabot.yml:5`, `:9`,
  `:14`). Dependabot's weekly pull requests are the only time-driven source of CI runs, and only when an update
  exists. `main` has run CI three times in total.
- **Nothing files an issue.** #40 and #41 were both opened by hand for the same flake, 14 minutes apart, by two
  sessions. Every issue, pull request and ruling comment in this repository is authored by the login `cglusky`
  (`gh issue list --state all --json author`: 35 of 35; pull requests: 9 of 10, the other is Dependabot); the
  owner ruling on #40 is recorded as "posted by the Claude session on the owner's word". There is no
  `class:flake` label here (`gh label list`); #40 and #41 carry `bug` and a `flake(...)` title prefix.
- **Routing rule for filing.** `.agents/protocol.md:43` — `- **File:** before opening an issue, route the finding.
  A residual of a decision this change just made deliberately`; `:46-47` restrict issues to architectural
  findings and say to ask the owner when placement is a scheduling call.
- **Pickup looks at three main runs.** `.agents/protocol.md:28-29` — the Start ritual includes `gh run list
  --branch main --limit 3`.
- **Skips and quarantine.** One skip exists: `internal/harness/runner/runner_test.go:264` — `t.Skipf("pid %d was
  reused before the test could use it", pid)`. One build tag exists:
  `internal/harness/natsfixture/fixture_integration_test.go:1` — `//go:build integration`. No command counts
  tests, counts skips, or lists build tags. A test file under a tag
  no gate passes is compiled by nothing and noticed by nothing. `task cover:check` would notice a skip only if it
  took `natsfixture`, `lifecycletest` or `probe` below 80% (`scripts/cover-check.sh:14`). **Documented, not
  measured:** `go test` without `-v` prints nothing for a skipped test, and both invocations run without `-v`, so a
  CI log cannot show whether `runner_test.go:264` fired.
- **Masking.** `-failfast` (`scripts/test-integration.sh:402`) stops a package at its first failure, so a second
  flaky test behind the first is not reported in that run. `scripts/verify.sh:24` keeps going across steps.
- **Baseline for a count.** 80 top-level `Test`/`Fuzz` functions in 16 files, 11 of them in the tagged file
  (`git grep -c -E '^func (Test|Fuzz)' -- '*_test.go'`).

## 5. The live flake as an input (#40, #41, PR #43)

- Site: `internal/harness/lifecycletest/refowner_test.go:245` — `finalize(t, o)` inside
  `TestEachFailpointTripsExactlyItsCheck` (`:216`). `finalize` (`:184`) calls `finish` then reads `Observe`
  (`:186-187`); `Observe` reads `workerDone` without blocking (`:147-150`).
- #41 states the cause: under `abortStopDropsCause`, `Stop` returns nil without joining the worker, the second
  `Stop` from `finalize` returns early, and `Observe` reads before the worker goroutine has run its deferred
  close. That is consistent with the lines above. This inventory did not trace the failpoint branch and does not
  confirm it; PR #43 owns the root cause and has no content commit yet.
- The adopter path reads the same way: `internal/harness/lifecycletest/lifecycletest.go:171` — `return
  requireNothingRetained(o, "after a controlled Stop returned nil")` calls `o.Observe()` (`:314`) immediately
  after a Stop that returned nil, and `Run` calls `finish` after every check (`:92`). An adopter's `Observe` must
  already reflect the join at the moment its Stop returns nil.
- Reproduction measured for this inventory at the base, `go test -race -count=300 -run
  'TestEachFailpointTripsExactlyItsCheck/abortStopDropsCause' ./internal/harness/lifecycletest/`, host load
  average 2.6 to 3.5 with other agent sessions active:

| Setting | Failures |
|---|---|
| default (12 CPUs) | 0 of 300 |
| `-cpu 4` | 0 of 300 |
| `-cpu 2` | 1 of 300 |
| `-cpu 1` | 24 of 300 |
| `GOMAXPROCS=1 -cpu 1` | 15 of 300 |
| whole package, `-race -count=20`, default | 0 failures, 41.5 s |

- #41 reports 32 of 500 at `GOMAXPROCS=1 -cpu 1` and 300 of 300 passing at default parallelism. In CI it failed in
  3 of the 19 runs completed since the harness reached `main` (each run executes it three times).
- It was introduced by PR #13 and passed that PR's CI. PR #13's body records no repeated-run evidence
  (`gh pr view 13 --json body | grep -i -E 'count=|stress|flak'`: 0 hits).

## 6. The SemStreams record

`gh issue list -R C360Studio/semstreams --label class:flake --state all --limit 100`: 23 issues, 21 closed, 2 open,
all authored by `cglusky`, all filed by hand, created 2026-07-05 to 2026-09-29 (20 of them from 2026-08-23 on). All
23 bodies and comments were read; closing pull requests were read by title, file list and body summary.

The label is a floor. #199, #209, #220 and #750 are flake issues without it, and #220's table of 2026-06-03 lists
eight flakes of which five are marked "unfiled".

### 6.1 Per issue

"Isolated repeat" is what the issue or its closing pull request records about re-running the test alone.

| Issue | Root-cause shape | Detected by | Closed by | Isolated repeat |
|---|---|---|---|---|
| #469 | not recorded ("Likely cause (to confirm)": websocket warm-up or teardown race) | PR run on an unrelated PR; again on a docs-only PR | open since 2026-07-05 | passed 2 of 2 |
| #736 | substrate contention: Docker daemon oversubscribed under package parallelism; container start and port lookup exceed budgets | a session's local gate runs; later two `main` runs | closed 2026-09-29 by the merge of PR #1404, an unrelated config change whose body contains "(c) fix #736 first" and says "#736 stays with the test-suite cleanup"; mitigations earlier (PR #891, and the `-p 2` cap in PR #1344); its last three comments record new occurrences as "not fixed by anything in flight" | passes alone |
| #1010 | production defect: List fails when a key is deleted between `Keys()` and `Get()` | a person, in a consumer's E2E suite (about 1 full run in 4) | fix to production code (PR #1085) | scripted repro, 12% |
| #1054 | not recorded in the issue ("Deliberately not diagnosed here"); a comment reframes it as a possible symptom of #736; the closing PR groups it under "timing and sleep-based concurrency assertions" | `main` runs (4 of 8 consecutive) and PR runs | test and lifecycle-suite changes (PR #1068) | not attempted |
| #1059 | unsynchronised observation: readiness asserted right after `Start` returns while the first health check is asynchronous | "the required integration suite" (PR or main not stated) | fix to the test: wait on the health callback (PR #1068) | 0 of 100 at default scheduler; 100 of 100 at `GOMAXPROCS=1` |
| #1061 | unsynchronised observation: test races the runner's TERM-to-KILL escalation between non-atomic reads | local full `go test -race ./...` | fix to the test: causal pause (PR #1068) | 100 of 100 passed |
| #1062 | unbounded cleanup: `Stop(context.Background())` in `t.Cleanup`, plus NATS terminated before the processor's Stop | local integration gate; a person sent SIGQUIT after 12 minutes | fix to two test sites (PR #1068); a cleanup-root guard landed 36 days later (PR #1414, filed against #1064) | passed alone in 2.1 s |
| #1063 | wall-clock assertion used as a concurrency oracle (`elapsed < 800ms`) | local integration gate | fix to the test: causal barrier (PR #1068) | 10 of 10 passed |
| #1069 | broker timing: cluster metadata not yet propagated after stream create | PR run, then a `main` run after that PR "merged past the gate" | fix to the test fixture: wait until visible on a typed not-found (PR #1072) | never reproduced: 60 runs at `-race -count=10 -cpu 1,2,4` plus a loaded probe |
| #1120 | port: probe-then-bind helper (`freePort`), 10 then 18 then 27 call sites | a person in PR review; later a local full run | fix to production code and tests: the listener is handed over, not a port (PR #1386); open 29 days | not stated |
| #1157 | wall-clock assertion (startup under 15 s) | local run under disk pressure; "the retry that went green afterward is substantially a re-roll" | fix to the test: readiness instead of elapsed time (PR #1385); open 27 days | 0.41 s alone |
| #1175 | a guard that overstated its coverage: port preflight probed 2 of 46 ports and printed OK | PR run, required job, on an OpenSpec-only commit | the guard was fixed (in PR #1148); closed by hand on the owner's word | 1 in 15 runs |
| #1279 | port: 29 of 44 published host ports inside the kernel's ephemeral range; the preflight ran 77 s before the bind | PR run, required job, on an OpenSpec-only commit | infrastructure fix: reserve the ports, with a fixture test (PR #1280) | not applicable |
| #1284 | predicted wall-clock budget (3 s per repetition) below a 5 s deadline the framework already enforces; #750 had been closed by adding a comment at the assertion | PR run, required job; 5 firings, one on `main` | the assertion was deleted on the owner's ruling, "deleted, not widened" (PR #1285) | not applicable |
| #1290 | predicted budget (3 s wait for a process exit); "landed inside the #1284 fix" | local measurement under full-suite load; CI had not yet hit it | fix to the test: wait bounded by the test deadline (PR #1291) | 0 of 5 alone; 2 of 10 under concurrent load |
| #1317 | not recorded ("Not yet diagnosed"): host-port bind fails after a verified reservation and a green preflight | PR run, required job, documentation-only tree; recurred 2026-09-19 | open; diagnostics landed (PR #1322) under an owner waiver with a same-head retry | not applicable |
| #1336 | unsynchronised observation: scenario queried an index before it was ready | PR run on a comment-only commit; 1 in 19 | fix to the harness: bounded wait on a typed transient error (PR #1337); a comment records three more stages of the same shape left uncovered | not applicable |
| #1340 | a test shells out to `go list` and blocks on the toolchain lock when two full-module runs overlap on one host | local gate, overlapping agent sessions | the check was moved out of `go test` into a lint guard with a 14-case fixture test (PR #1385) | healthy alone, even under load |
| #1358 | external dependency: an unpinned installer fetch got a 504; not a test | `main` run | CI fix: pinned install through the module proxy (PR #1359) | not applicable |
| #1375 | unsynchronised observation of broker state: the test released when a drain was issued, not when the server had dropped the subscription | PR run, once; passed on a re-run under an owner waiver | fix to the test: wait on consumer state (PR #1389) | 50 of 50 passed before the fix; reproduced only with a delay forced into a scratch copy of the SDK |
| #1394 | unsynchronised observation: cleanup assertion rejected an outcome the SDK can legitimately produce; "inferred ... not traced" | PR run on an empty claim commit | fix to the test: tolerate the error (PR #1395) | not applicable |
| #1397 | predicted budget (four 3 s pipe waits); same test as #1061, same class as #1290 | local gate | fix to the test: causal signals under one 35 s budget (PR #1408); an owner waiver for PR #1388 | 1 of 5, then 5 of 5 passed |
| #1421 | not recorded ("Historical causation remains unproven"): a KV listing exceeded the framework's 5 s deadline | PR run, then a `main` run | closed on the owner's direction with the cause unproven, after partial repairs (PRs #1432, #1435) and waivers for four merges, the last recorded as "my last waiver on 1421" | not reproduced |

### 6.2 Counts by root-cause shape

| Shape | Issues | Count |
|---|---|---|
| Unsynchronised observation of asynchronous state | in-process: #1059, #1061; broker or service state: #1069, #1336, #1375, #1394 | 6 |
| Predicted wall-clock budget or assertion | #1063, #1157, #1284, #1290, #1397 | 5 |
| Root cause not recorded | #469, #1054, #1317, #1421 | 4 |
| Port collision (probe-then-bind, host ports, preflight) | #1120, #1175, #1279 | 3 |
| External or toolchain dependency inside a gate | #1340, #1358 | 2 |
| Substrate contention (Docker) | #736 | 1 |
| Unbounded cleanup | #1062 | 1 |
| Production defect that presented as a flake | #1010 | 1 |

### 6.3 Counts by closing mechanism

| Mechanism | Issues | Count |
|---|---|---|
| Fix to the test or harness, no production change | #1054, #1059, #1061, #1062, #1063, #1069, #1157, #1290, #1336, #1375, #1394, #1397 | 12 |
| Fix to infrastructure, CI or an existing guard | #1175, #1279, #1358 | 3 |
| Fix to production code | #1010, #1120 | 2 |
| Closed with no fix recorded for the class | #736, #1421 | 2 |
| Open | #469, #1317 | 2 |
| Assertion deleted on an owner ruling | #1284 | 1 |
| Check moved out of the test suite into a lint guard | #1340 | 1 |
| Skip or quarantine | none | 0 |

Related facts from the same record:

- **Retries.** Two fixes are waits that retry only on one typed error (#1069, #1336). The others replace a timing
  inference with a signal.
- **Reruns and waivers.** Six issues record a merge past the flake or a rerun: #1069 ("merged past the gate ... put
  main red"), #1157, #1317, #1375, #1397, #1421.
- **Tier move.** PR #1385, while closing #1157 and #1340, also moved six real-Ollama tests from the required
  integration tier to a `live_llm` tier.
- **Detection.** Hosted PR run on a diff that could not have caused it: 10 (#469, #1069, #1175, #1279, #1284,
  #1317, #1336, #1375, #1394, #1421). A session's local gate run: 8 (#736, #1061, #1062, #1063, #1157, #1290,
  #1340, #1397). `main` run first: 2 (#1054, #1358). A person outside CI: 2 (#1010, #1120). Not stated: 1
  (#1059). None was found by a scheduled or repeated run; at the pin `ci.yml` triggers on `push` and
  `pull_request` only, and the other five workflows on `push`, `pull_request` or `workflow_dispatch`.
- **Isolated repetition.** Where the record says, re-running the test alone at default settings did not reproduce
  the failure in eleven issues (#469, #736, #1059, #1061, #1062, #1063, #1069, #1157, #1290, #1340, #1375).
  It reproduced under a single CPU (#1059), concurrent load (#1290), or a forced delay (#1375).
- **Guards created by the closing work.** Three: the pin-check lint guard (#1340), the port reservation with its
  fixture test (#1279), and the corrected preflight (#1175). File lists of PRs #1072, #1291, #1337, #1389, #1395
  and #1408 show test or test-and-docs changes only. PR #1068 also touches `test/testinfra/policy_baseline.json`,
  whose role was not read.

### 6.4 Shapes that recurred after a guard or rule existed

| Guard or rule (date) | Kind | Later issues of the same shape | Why the guard did not apply, as recorded |
|---|---|---|---|
| Fixed-port lint, #220 Subclass 2 (2026-06-03) | failing command | #1120, #1175, #1279, #1317 | #1120 binds `:0` and re-binds, which the regex passes and the guard's message recommends; the others bind in Compose files, outside `*_test.go` |
| #220's wall-clock rule: rationale comment and at least 3x tolerance (2026-06-03) | prose, reviewer signal | #750, #1063, #1157, #1284, #1290, #1397 | #1063 is a test #220's own table names as unfiled, filed 81 days later; PR #1285 records that the 3x rule "was satisfied all along ... and was never the problem" |
| #750 closed by a comment at the assertion (2026-07-30) | documentation | #1284 | "a known-unfixed flake in a required job has no open issue behind it" |
| `-p 2` cap for Docker contention (PR #518, again PR #1344 on 2026-09-19) | runner flag | #736 occurrences on 2026-09-25, 09-27 and 09-29 | the suggested `-p 1` was measured as 94% slower on the full suite (2026-07-31) and recorded as falsified again on 2026-09-19 |
| Port preflight derived from Compose (2026-08-30) | preflight | #1279 | "the preflight is what passed here": a snapshot 77 s before the bind |
| Port reservation (PR #1280, 2026-09-10) | infrastructure | #1317 | reservation verified 63 s before the bind failed; cause open |
| #1284's ruling against predicted budgets (2026-09-11) | owner ruling, prose | #1290 the next day, #1397 two weeks later | #1290 "landed inside the #1284 fix"; #1397's test "kept its four 3s budgets when #1291 landed" |
| #1062's two-site fix (2026-08-24) | point fix | #1283 hang; #1417, open, counts 334 reviewed unbounded cleanup entries | the guard (PR #1414) landed 2026-09-29 |
| Merge-gate rule "no rerun-to-green" (updated 2026-08-24, per #1069) | prose | 14 of the 23 issues were filed after it; six record a waiver or rerun | the rule acts once a flake is known: fix it, or file it and obtain a waiver |

No sleep or wall-clock guard exists in SemStreams' `scripts/` at the pin (guard-named files there:
`check-cleanup-roots.sh`, `e2e-check-ports.sh`, `lint-nats-kv-sdk-pin.sh`, `lint-test-ports.sh`, each with a
fixture test, and `run-integration-tests.sh`). This repository's archived inventory records `time.Sleep` in 119
SemStreams test files (`openspec/changes/archive/2026-09-30-setup-02-isolated-harness/inventory.md:208`).

## 7. The five candidates in #42: overlaps and missing facts

Nothing here evaluates or shapes a candidate.

### 7.1 Stress at birth

Overlaps:

- The three invocations in section 3.1, all `-count=1`. The integration one is fixed by a spec requirement
  (`integration-test-runner/spec.md:76`), a test (`runner_test.go:420`) and a ledger row
  (`docs/admission-ledger.yaml:109`).
- Nothing maps a diff to packages. `scripts/verify.sh:16` uses `git diff HEAD` only to fingerprint the tree.
- The module has five packages (`go list ./...`), all under `internal/harness`.

Facts not established:

- The CI runner's CPU count. The live flake's rate depends on it (section 5): 0 of 300 at 4 CPUs and at 12, 24 of
  300 at 1, and 3 of 19 CI runs.
- Whether any repetition count at CI's default parallelism would have failed #40 on PR #13.
- The cost: the whole `lifecycletest` package at `-race -count=20` took 41.5 s on the local host; the job bound is
  15 minutes; no per-test timing is kept for unit runs.
- How the eleven SemStreams issues that isolated repetition did not reproduce (section 6.3) would have behaved
  under whatever load a stress run applies.

### 7.2 Scheduled suite stress with an auto-filed issue

Overlaps: no schedule trigger and three `main` runs ever (section 4); `contents: read` (`ci.yml:15-16`); the File
routing rule (`protocol.md:43-47`); no flake label; two hand-filed issues for one flake; evidence artifacts kept 14
days. The host lock (`scripts/admission-lock.sh:7` — `readonly
admission_lock_default="/tmp/semstreams-integration.lock"`) is per host and plays no part between hosted runners.

Facts not established: how often a scheduled run would have to repeat the suite to see a flake at #40's CI rate;
what identifies "the same flake" across runs (#40 and #41 differ in title and body for one failure line);
**documented, not measured:** GitHub disables scheduled workflows in a public repository after 60 days without
activity.

### 7.3 Rerun guard with a linked issue and a recorded waiver

Overlaps:

- `github.run_attempt` is in use (`ci.yml:59`); the maximum observed is 1.
- Both re-rolls observed here were new pushes at attempt 1 (section 3.4). A check on `run_attempt` does not see
  them.
- `required` reads only `needs.*.result` (`ci.yml:76`).
- The waiver's stated home is a PR comment (`protocol.md:51`); a ruling's stated home is an issue comment
  (`protocol.md:10` — `status:blocked`. `status:needs-decision` is the owner's docket; a ruling is posted as an
  issue comment and the`). The one ruling so far on a flake, on #40, is an issue comment.
- Owner and agents post under one login (section 4).
- The closest existing instance of "an exception passes only with a stated, recorded reason" is the image
  override: `scripts/test-integration.sh:67` — `if [ -z "${SEMENGINE_NATS_IMAGE_OVERRIDE_REASON:-}" ]; then`,
  refused at `:68-69`, warned at `:81`, recorded in `runner.env`
  (`openspec/specs/integration-test-runner/spec.md:80-82`).

Facts not established: whether "re-run failed jobs" and "re-run all jobs" present differently to `required`; what
permission a job needs to read a prior attempt's conclusion or an issue; how a command would tell an owner's waiver
from an agent's comment; whether GitHub can forbid re-runs at all (no such setting appears in the ruleset or the
repository settings read).

### 7.4 Root-cause guards

Overlaps: the two guarded shapes and their three sensitivity patterns (section 2.1); `go tool revive` and `go vet`
as existing analysis steps (`Taskfile.yml:46`, `:51`). Present counts a new guard would meet at base: `time.Sleep`
0; `time.Now`/`time.Since` in tests 4 lines; `time.NewTimer` watchdogs 4; `t.Parallel()` 18; skips 1.

Facts not established: a textual signature for unsynchronised observation, the largest SemStreams class (6 of 23)
and the #40 shape; none of those issues records one. Whether a budget that is a contract (SemStreams #1290 kept
`elapsed > 4*time.Second` deliberately) can be told from a predicted one by its text. Go 1.26.6 (`go.mod:3`)
includes `testing/synctest` (**documented, not measured**); it is unused here.

### 7.5 No quarantine

Overlaps: one skip, one build tag, nothing that counts either (section 4);
`.agents/contracts/semengine-reviewer.md:215` — `- **A silent skip, drop, or degrade is a finding at any
severity.** Any path that continues past a failure, takes a` (written about product paths);
`semengine-preflight/SKILL.md:78` asks evidence to separate "failure, skip, no selected tests". In SemStreams no
`class:flake` issue was closed by a skip; one closing PR moved six tests to a non-required tier.

Facts not established: whether `runner_test.go:264` has ever fired in CI (logs are not verbose).

### 7.6 Same-class collision table

Candidates 2 and 3 would add durable records (an auto-filed issue; a waiver) and a coordination rule (a gate on
reruns). Semantic class: *the record that a required-job failure is a known flake, and the authority to merge past
it.*

| Dimension | Existing owners and evidence |
|---|---|
| Semantic class | "No known unfixed flake in a required job"; a green over one is rerun-to-green; fix, or file and obtain a waiver (`.agents/protocol.md:50-51`) |
| Owners | The protocol's Land step; `AGENTS.md:70`; `semengine-preflight/SKILL.md:82-83`; the owner, for the waiver. No code owner. |
| Catalogs | GitHub issues (#40, #41: label `bug`, title prefix `flake(`; no flake label); the Actions run and attempt history; artifact `integration-evidence-<run_id>-<run_attempt>` (`ci.yml:59`) |
| Status | The `Required` check (ruleset 24272345); labels `status:needs-decision` and `status:blocked` (`protocol.md:9-11`); pickup reads "failures or waivers" (`.agents/skills/semengine-pickup/SKILL.md:22`) |
| Lifecycle | A waiver is per merge, a PR comment; an issue closes by a merged PR that declared `Closes #n` (`protocol.md:54-58`); artifacts expire at 14 days (`ci.yml:63`) |
| Ownership | Rulings are the owner's, on the issue (`AGENTS.md`, Roles); one GitHub login for owner and agents; `bypass_actors: []` |
| Readers | Agents at pickup (`protocol.md:28-29`); reviewers; the owner. No workflow reads issues or comments. |
| Writers | Agents and the owner through `gh`, by hand. No workflow can write (`ci.yml:15-16`). |
| Recovery | None. Duplicate filing observed (#40, #41). A flake whose issue is closed without a fix has no record (SemStreams #750, then #1284). |

## 8. Surface inventory categories

1. **The claimed gap.** #42's three absences, measured: nothing repeats a test beyond the two or three
   `-count=1` executions (section 3.1); nothing runs without a push (section 4); nothing stops a rerun, and
   nothing stops a re-roll by push either (section 3.4). The merge-gate flake clause is prose (section 3.3).
2. **Every current spelling.** The flake clause: `protocol.md:50-51`, `AGENTS.md:70`,
   `semengine-preflight/SKILL.md:82-83`. The sleep and synchronisation rule: four prose homes (section 2.2). The
   test invocation: `Taskfile.yml:74`, `scripts/test-integration.sh:402`, `scripts/cover-check.sh:22`, the spec at
   `integration-test-runner/spec.md:76`, the assertion at `runner_test.go:420`, the ledger at
   `docs/admission-ledger.yaml:109`, and `docs/testing.md:66` on PR #39. What `verify` runs: `scripts/verify.sh:10-11`
   and a stale list at `AGENTS.md:43`; `semengine-developer.md:189-192` also lists the older gate set and says no
   integration gate exists yet.
3. **Adjacent claims.** PR #43 (fix for #40, claim only). Issue #41 (same flake, unclaimed, cause stated). PR #39
   and issue #37 (rules table and `docs/testing.md`, unmerged; they would add the "review only" index and the
   testing page this work touches). PR #21 (red at head on this flake). Issue #38 (a decision on another
   lifecycle-suite check). Current specs `harness-boundaries`, `integration-test-runner`, `lifecycle-suite`,
   `nats-fixture`. No active OpenSpec change (`ls openspec/changes`: `archive` only). No ADR directory.
4. **The consumer at birth.** This phase proposes no surface. Present consumers of any defense: the authors of the
   16 test files, the `verify` CI job, and the pickup ritual. `internal/harness` is under `internal/`, so another
   module cannot import it; `docs/repository-map.md:9` says the only Go code is the test harness under
   `internal/harness/`.
5. **The problem shape.** Refuse a shape with a failing command, proved by a planted violation:
   `addresses_test.go:21` with `:26`; `scripts/cleanup-roots-check.sh` with `cleanuproots_test.go:18`;
   `scripts/lint-test-ports.sh` with its shell fixture test. Admit an exception only with a recorded reason:
   `scripts/test-integration.sh:67-69`. Refuse evidence that does not belong to the tree it claims:
   `scripts/cover-check.sh:29` — `if ! grep -qx 'go_test_status=0' "$run/runner.env" 2>/dev/null; then` and `:34`.
   Admit or refuse at a seam: `natsfixture` admission (`nats-fixture/spec.md:9-12`). Exempt by inline marker:
   `scripts/lint-test-ports.sh:41`.

Context retention: this work touches test and CI machinery only; there is no production Go code in the tree
(`docs/repository-map.md:9`), and `TestNoRetainedContext` (`internal/harness/contract/context_test.go:22`) covers
the harness's non-test structs.

## 9. Adopter seam: the test author

The adopter is a developer or agent writing a test here who has not read the contracts, and later a consumer
repository's author who copies `internal/harness` patterns (they cannot import them, section 8 item 4).

1. **What must they know today?** Eleven facts, each a debt:
   1. Wait on a signal, never a sleep (prose).
   2. A wall-clock assertion needs a rationale and tolerance (prose).
   3. No `t.Parallel()` around process-global state (prose).
   4. Cleanup needs a bounded context; only the one-line form is refused.
   5. No fixed ports; only `net.Listen` literals and four broker literals are refused, and probe-then-bind is not.
   6. Docker tests carry the `integration` tag and run through the runner.
   7. An untagged test runs twice per verification, the second time two packages at a time beside container tests.
   8. A local red under contention "is not a finding" (`protocol.md:42`).
   9. A red from a test they did not touch must be filed and fixed, not re-pushed past (`protocol.md:50-51`).
   10. `lifecycletest.Run` classifies their owner against `grace = 2s` and reads `Observe` the moment Stop
       returns nil.
   11. `-race` passing says nothing about an observation that races the scheduler.
2. **What happens if they do nothing?** The test passes locally and in the two CI executions, merges, and later
   fails on another author's unrelated pull request. That is what #40 did: introduced by PR #13, green there,
   first red 12.5 hours after merge on a docs-only diff. The next push usually comes back green.
3. **Where do they find out?** Facts 4 and 5 (in their matched forms) and 6: a failing `task verify` step or a
   typed `ErrNotAdmitted`. Facts 1, 2, 3, 8, 9: a contract or a page (`docs/testing.md` is not merged). Facts 7,
   10, 11: nowhere. For the author of the flake, the eventual signal reaches someone else.
4. **What should they have to know?** Nothing. The gap is all eleven items less the three with a command behind
   them, and for the three, the unmatched forms listed in section 2.1.

Observation versus prediction: the budgets in section 2.3 are values a test author predicts. Three SemStreams
issues (#1284, #1290, #1397) each record a predicted number sitting below a bound the framework already enforced.

## 10. Intent check

| Capability | Relation to the stated purpose | Relation to the owner direction |
|---|---|---|
| Flake defense for the test harness and CI | Not a product boundary. `AGENTS.md` ("What this is for") makes SemEngine a framework of primitives and contracts; the harness is the only Go code today and every later proof rests on it. No consumer domain semantics are involved. | The direction asks that flakes be fixed and that "the bleeding" stop. Measured against it: SemStreams closed 19 of 23 with a fix, one at a time, while its two largest recorded shapes (11 of 23) had prose rules only and kept recurring; this repository has a failing command for two shapes, prose for the rest, no repeated or scheduled run, and a merge gate whose flake clause no command reads. |

## 11. Not established

- The root cause of #40. #41 states one; PR #43 has not confirmed it.
- The CI runner's CPU count and memory.
- Whether `TestIgnoredInterruptIsDetected`'s red on PR #13 at `ec8e685` was nondeterministic.
- The root causes of SemStreams #469, #1054, #1317 and #1421: the issues do not record them.
- Whether #736's close by PR #1404 was intended. The mechanism (a closing keyword in a docket option) is inferred
  from the PR body; no comment on #736 records a fix or a decision to close.
- The role of `test/testinfra/policy_baseline.json` in SemStreams PR #1068, and so whether that PR added a guard.
- Where SemStreams #1059 was first detected.
- GitHub behaviors marked "documented, not measured": rerun semantics and scheduled-workflow disabling; and
  whether reruns can be forbidden at all.
- Go behaviors marked "documented, not measured": skip visibility without `-v`, and `testing/synctest` in 1.26.
- Any flake in SemStreams that was never filed. The label is a floor (section 6).
