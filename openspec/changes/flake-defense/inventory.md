# Inventory: flake-defense

- base: `4d968602f1608a987668d94a58bd71fdb72253e2` (tree `2eaed9bee7873e838dd9763264cfbc3260e7e5d9`; it differs from
  `origin/main` `d303c51addc9d11030cb08431ce8b9acfa9ac406` only by the four files under
  `openspec/changes/flake-defense/`)
- first written at base `97e53e630c56bb8f68cac188c045f6899f9cfd20` (commit `4d4d893`, sha256
  `2da7edb5d0907a85fc5397c26271acaa365b2fd143c7c4521df56da910f19502`); this revision corrects it after the
  independent inventory review recorded on PR #44 (comment 5934062218, INVENTORY CHANGES REQUESTED)
- semstreams (code facts only): `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read through `gh api ... ?ref=`
- unmerged branch read: `origin/claude/harness-lessons` at `74db5cc9a64d82237a7405a48a9cebc9611e2a2b` (PR #39)
- measured: 2026-10-01; issue #42, claim PR #44

This is the architect's inventory-only deliverable (contract step 2). It contains no target state, options or
recommendation. The corrected file has not yet had its re-check by the independent reviewer. Section 12 maps each
review finding to the place that answers it.

Between the two bases one Go file changed: PR #43 merged as `d303c51` and closed #40 by changing
`internal/harness/lifecycletest/refowner_test.go` (+52/-2). Every pin into that file is re-pinned at `4d96860`.
"Old code" below means the tree before that fix (`6a006df` and `97e53e6` carry the same Go files); measurements
taken on it are historical and are labelled so.

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

Added in this revision, all prompted by the review's findings:

- SemStreams code at the pin: `test/testinfra/policy_guard_test.go`, `test/testinfra/policy_baseline.json`,
  `test/testinfra/cleanup_baseline.json` (counts only), `service/service_manager_health_listener_test.go`,
  `scripts/lint-test-ports.sh`, `taskfiles/lint.yml`, the six files under `.github/workflows/`, and the commit
  history of the two policy files reachable from the pin.
- SemStreams' record of owner waivers. The review cites two waiver comments that are not on a `class:flake` issue
  (PR #1373, PR #1322). To see how each waiver was carried out, `gh search prs --repo C360Studio/semstreams "owner
  waiver"` was run against comments (21 hits) and bodies (11 hits), and only the paragraphs naming a waiver were
  read. This goes one step past the pull requests that closed a labelled issue; it is reported here so the owner can
  confirm or strike it.

Pins are `path:line` with the line's text, at the base unless a row says otherwise. GitHub facts carry the `gh`
query that produced them. Statements about GitHub or Go behavior that were not exercised here are marked
**documented, not measured**. A measurement taken from another record is cited with its source and marked **not
re-measured**.

## 1. Premises that measurement changed

Premises in #40, #42, the briefing, and the first version of this inventory.

| Premise | Source | Measured |
|---|---|---|
| "Nothing runs a new test more than once before merge" | #42, Why now | Each untagged test runs twice under `-race` per `task verify` (unit step, then again inside the integration step, which defaults to `./...`). `lifecycletest` and `probe` run a third time without `-race` in `cover:check`, and the two `TestAdmissionLedger*` tests run a third time without `-race` in `ledger:check`. Every run is `-count=1`. Section 3. |
| The failure is in `task verify` then `task test:unit` | #40, What fails | Run 36869511251 failed it inside `test:integration` (`verify: FAILED: test:integration cover:check`); runs 36871402708, 36872498596 and 36878923325 failed it inside `test:unit`; run 36875948219 failed in `cover:check` with no test named. |
| Two occurrences | #40, Occurrences | Four by name (36869511251, 36871402708, 36872498596, 36878923325) and one unidentified `cover:check` failure (36875948219), in 22 completed runs on the old code. Section 5. |
| The root cause of #40 is not available | briefing | PR #43 states it, an independent review re-derived it, and the fix merged as `d303c51` on 2026-10-01T15:23Z; #40 is closed. #41 (the duplicate, opened 14 minutes after #40) and PR #45 (a second fix claim) are closed. Section 5. |
| `task verify` runs nine gates ending in `test:unit` | `AGENTS.md:43` | `scripts/verify.sh:10-11` runs thirteen, including `cleanup-roots:check`, `ledger:check`, `test:integration` and `cover:check`. |
| SemStreams closed 21 of 23 `class:flake` issues | #42, Why now | Count confirmed. Three of the 21 closed with no fix recorded for the failure itself (#736, #1175, #1421). Section 6. |
| "Repetition at default parallelism does not find #40" | #42, comment of 14:46Z, from the first version of this inventory | True only under `-race`. Without `-race` the old code fails 1 of 300 at the default and 300 of 300 at `-cpu 1` (reviewer), 99 of 100 at `-cpu 1` (PR #43), and 100 of 100 at `-cpu 1` re-measured here. `cover:check` runs without `-race` on every verify. Section 5. |
| "No sleep or wall-clock guard exists in SemStreams" | first version, section 6.4 | True of `scripts/` only. At the pin `test/testinfra/policy_guard_test.go` refuses new `time.Sleep` calls in integration-tagged files against a baseline of 254 grandfathered entries. Section 6.5. |
| "The CI runner's CPU count and memory are not recorded in any run evidence" | first version, section 3.2 | They are: the integration runner writes `docker info` into the evidence directory, and the uploaded artifact holds it. Two runs read: `CPUs: 4`, `Total Memory: 15.61GiB`. Section 3.2. |
| One CI run "in progress" | first version, section 3.4 | That run (36875948219, on the first version's own base commit `97e53e6`) had already failed. Section 3.4. |

Pins for the fifth row:

- `AGENTS.md:43` — `task verify       # spec:check docs:check fmt:check tidy:check build vet lint vuln test:unit,
  cheapest first;`
- `scripts/verify.sh:10` — `steps=(spec:check docs:check fmt:check tidy:check cleanup-roots:check build vet lint
  vuln ledger:check test:unit`
- `scripts/verify.sh:11` — `test:integration cover:check)`

## 2. Birth: what stops a timing-dependent test from being written or merged

The complete set of failing commands is the thirteen steps of `scripts/verify.sh:10-11` plus `revive.toml`'s rule
list (`revive.toml:10-36`; none is about sleeps, elapsed time, parallelism or skips, and `time-naming` at `:25`
checks variable names only). Of the eight shapes the brief names, two have a failing command and six do not. Skips
and external dependencies are added below as two further rows without one.

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
  The guard's own fix message recommends it in two lines:
  - `scripts/lint-test-ports.sh:52` — `echo 'Fix: use net.Listen("tcp", ":0") + read the resolved port from
    listener.Addr().(*net.TCPAddr).Port.'`: bind `:0` and read the port, the first half of the same pattern.
  - `scripts/lint-test-ports.sh:53` — `echo 'See service/service_manager_health_listener_test.go freePort() helper
    for the canonical pattern.'`. That path does not exist in this repository, and #1120 names `freePort` at that
    path as the race.
- The script is byte-identical to the SemStreams pin (`cmp` against the pin's `scripts/lint-test-ports.sh`: equal).
  At the pin the file the message points to has no `freePort` at all (`grep -c freePort`: 0 in
  `service/service_manager_health_listener_test.go` at `8b99efe9`). SemStreams PR #1386 (merged 2026-09-25, closes
  #1120) removed the helper and its nine uses from that file. So the carried message names a helper that its source
  repository deleted as a race five days before the pin commit (2026-09-30).

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
- This guard was chosen over SemStreams' AST guard by an earlier ruling, and the choice is recorded in three places:
  `openspec/changes/archive/2026-09-30-setup-02-isolated-harness/design.md:55` — ``- **O4 Unbounded-cleanup guard —
  ruled (b) grep guard.** A `git grep` guard in `verify` for the exact #1417 shape``; `:59` rejects "SemStreams'
  4,000-line AST guard (revisit at 04A when ported tests arrive)"; `docs/admission-ledger.yaml:179-190` is the
  ledger row for `test/testinfra/cleanup_guard_test.go`, disposition `defer-exclude`, with `:187` —
  `known_risks: 4,000 lines with a legacy baseline upstream`; and `scripts/cleanup-roots-check.sh:7` — `# Zero
  baseline: every hit fails. The harness's own cleanup roots are bounded and`.

### 2.2 Shapes with prose only

| Shape | Failing command | Prose homes | In the tree at base |
|---|---|---|---|
| `time.Sleep` in tests | none | `.agents/contracts/semengine-developer.md:187`; `.agents/contracts/semengine-reviewer.md:173`; `docs/setup-plan.md:243`; `docs/testing.md:158` on PR #39 | 0 hits: `git grep -n -E 'time\.Sleep' -- '*.go'` |
| Wall-clock assertions | none | `semengine-developer.md:188`; `semengine-reviewer.md:173` | one elapsed assertion, `internal/harness/natsfixture/fixture_test.go:181`; four test watchdogs; see budgets below |
| Unsynchronised goroutine observation | none; `-race` does not flag it (the four named #40 failures all ran under `-race`), and the run without `-race` is the more sensitive one (section 5) | `openspec/specs/lifecycle-suite/spec.md:16`; `docs/testing.md:158-159` on PR #39 | the #40 site is fixed; seven sites of the same family remain, listed below the prose pins |
| `t.Parallel` with shared state | none | `semengine-developer.md:187`; `semengine-reviewer.md:172`; `docs/testing.md:169` on PR #39 | 18 `t.Parallel()` calls, all in `internal/harness/runner/runner_test.go` (first at `:274`) |
| Test-order dependence | none; no `-shuffle` anywhere (`git grep -n -i shuffle` finds only an unrelated word at `semengine-reviewer.md:31`) | `docs/testing.md:168` on PR #39 | not measurable without a shuffled run |
| Reliance on container or broker timing | none for the test's own assertions; the fixture and runner bound their own phases (below) | `docs/setup-plan.md:243` | 11 integration tests in one tagged file |
| Skips | none | `.agents/skills/semengine-preflight/SKILL.md:78` | one, `internal/harness/runner/runner_test.go:264` |
| A network dependency inside the required job | none; each fetch fails the job when its far end fails | none found: `git grep -n -i -E 'registry\|proxy\|offline' -- .agents AGENTS.md docs/setup-plan.md` returns two lines (`AGENTS.md:10`, `docs/setup-plan.md:55`), neither about CI fetches | five fetches per hosted run, listed in section 2.5 |

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

Sites at `4d96860` where a goroutine is left unjoined, or where a test reads asynchronous state at one instant.
The first four were reported by PR #43 and its review and routed to #42. The last three were found by this
correction, by reading every hit of `git grep -n -E '^[[:space:]]*go (func|[a-zA-Z_.]+\()' -- '*.go'` (21 `go`
statements) and `git grep -n -E '^[[:space:]]*default:' -- '*_test.go'` (8 `default:` branches).

| Pin | Line text | What it is |
|---|---|---|
| `internal/harness/lifecycletest/refowner_test.go:204` | `o.cancel() // this double never stops its own worker; end it through Start authority` | `finalize` cancels the `stopReturnsNilWithWorkerRunning` worker and returns without waiting on `workerDone`; `:198` exempts that failpoint from the "still holds" check. PR #43's review calls it the one path left where a test-double worker outlives its subtest, and says it cannot cause a false failure. |
| `internal/harness/natsfixture/deps.go:86` | `go func() {` | the dial goroutine; on context end `connect` returns at `:99` and a second goroutine (`:94`) closes whatever connection arrives later. Neither is joined; the doc comment at `:76-79` declares it. Non-test code. |
| `internal/harness/probe/probe_test.go:51` | `if closed(cb.Joined()) \|\| closed(returned) {` | asserts at one instant that `Block` has not returned; it cannot fail falsely, and it catches the defect only when the scheduler has already run the goroutine |
| `internal/harness/probe/probe_test.go:120` | `if closed(gotDone) {` | the same shape: "Done returned while held", read once (PR #43 cites the `t.Fatal` at `:121`) |
| `internal/harness/natsfixture/fixture_integration_test.go:411` | `select {` | `:412-414` fail if Stop has already returned before the callback is released; one non-blocking read |
| `internal/harness/natsfixture/fixture_integration_test.go:615` | `select {` | the same shape for a handler still running (`:616-618`) |
| `internal/harness/lifecycletest/lifecycletest.go:293` | `go func() {` | `call` runs the owner's operation in a goroutine and abandons it when the bound fires (`:306-307`); the `stopIgnoresCallerDeadline` double then stays blocked until test cleanup closes `hang` (`refowner_test.go:188`). Non-test code, by design. |

The other `default:` branches are the `Observe` implementation (`refowner_test.go:160`), the two `closed` helpers
(`probe_test.go:19`, `fixture_integration_test.go:92`), two polls inside `probe.Await` (`runner_test.go:363`,
`:577`), and PR #43's forced-interleaving test (`refowner_test.go:289`), where a `synctest` bubble decides the
outcome.

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
| `internal/harness/natsfixture/admission_test.go:178` | `watchdog := time.NewTimer(10 * time.Second)` | test watchdog: a Stop given a 100 ms context must return within 10 s |
| `internal/harness/natsfixture/fixture_test.go:227` | `watchdog := time.NewTimer(5 * time.Second)` | test watchdog: a creation queued behind Stop must be refused within 5 s |
| `internal/harness/runner/runner_test.go:703` | `watchdog := time.NewTimer(10 * time.Second)` | test watchdog: the runner must exit within 10 s of a 1 s TERM grace |
| `internal/harness/runner/runner_test.go:785` | `watchdog := time.NewTimer(20 * time.Second)` | test watchdog: the runner must not wait on an escaped log writer for 20 s |
| `scripts/test-integration.sh:26` | `readonly max_wait_seconds=3600` | wait for the host admission lock |
| `scripts/test-integration.sh:27` | `readonly pull_budget_seconds=300 # a ceiling on registry latency, not a delay` | image pull |
| `scripts/test-integration.sh:43` | `readonly leak_wait_seconds=15    # Ryuk reaps a dead session's containers after 10s` | leak check |
| `scripts/test-integration.sh:402` | `argv=(test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m` | `-timeout 10m` per package in the integration step; the unit step uses Go's default |
| `.github/workflows/ci.yml:22` | `timeout-minutes: 15` | the `verify` job |

The four watchdogs each carry the comment "A watchdog for the failure, not a synchronisation". They are the same
kind of number as SemStreams #1290 and #1397, which were process and pipe waits in SemStreams'
`test/testinfra/integration_runner_contract_test.go`; `internal/harness/runner/doc.go:6` — `// The pattern, not the
code, comes from SemStreams test/testinfra/integration_runner_contract_test.go`. The runner
package also holds all 18 `t.Parallel()` calls and takes 16.7 s to 18.8 s per CI execution (runs 36875948219 and
36883858915).

The matrix test pays the suite's `grace` in real time. `stopIgnoresCallerDeadline` blocks on `<-o.hang`
(`refowner_test.go:115`) until the suite's timer at `lifecycletest.go:301` fires. Measured at `4d96860`, one run
without `-race` on this host: `--- PASS: TestEachFailpointTripsExactlyItsCheck (2.00s)`; every CI failure log shows
2.01 s for it. Section 5 gives what that costs a repeated run.

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

### 2.5 Network fetches inside the required job

SemStreams #1358 was a required job that failed because an installer fetch returned a 504. The same class exists
here; none of these has failed in the 42 CI runs.

| Pin | Line text | Fetch |
|---|---|---|
| `.github/workflows/ci.yml:24` | `- uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1` | five actions pinned by commit (`:24`, `:26`, `:30`, `:42`, `:57`), downloaded per run |
| `.github/workflows/ci.yml:35` | `- run: npm ci` | the npm registry, for OpenSpec and markdownlint |
| `Taskfile.yml:69` | `- scripts/gopkgs.sh go tool govulncheck ./...` | the vulnerability database, whose content changes with no push (**documented, not measured**) |
| `scripts/test-integration.sh:364` | `docker pull "$image" > "$evidence_dir/pull.log" 2>&1 &` | the NATS image by digest; run 36875948219's `runner.env` records `nats_digest_cached=no`, `pull_ms=2686` |
| `scripts/doctor.sh:104` | `else warn ryuk-image "not cached; testcontainers pulls it on first use: $ryuk"; fi` | the testcontainers reaper image, pulled by the library during the tests |

Go modules are a sixth fetch on a cold cache (`actions/setup-go` at `ci.yml:26`; its caching was not measured).

## 3. Merge: how often a test runs before merge, and what a rerun leaves behind

### 3.1 Executions per verification

| Step | Pin | Line text | Flags that matter |
|---|---|---|---|
| `ledger:check` | `Taskfile.yml:64` | `- go test -count=1 -run '^TestAdmissionLedger' ./internal/harness/contract/` | no `-race`, once, two tests of one package |
| `test:unit` | `Taskfile.yml:74` | `- scripts/gopkgs.sh go test -race -count=1 ./...` | `-race`, once, default `-p`, Go's default 10 m timeout, source order, no `-failfast` |
| `test:integration` | `scripts/test-integration.sh:402` | `argv=(test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m` | `-race`, once, two packages at a time, stops a package at its first failure |
| (its package set) | `scripts/test-integration.sh:56` | `((${#packages[@]} > 0)) \|\| packages=(./...)` | every package, so untagged tests run a second time here beside the container tests |
| `cover:check` | `scripts/cover-check.sh:22` | `go test -count=1 -coverprofile="$unit" "./internal/harness/lifecycletest/" "./internal/harness/probe/" > /dev/null` | third run of two packages, no `-race`, output discarded |

So one `task verify` holds four `go test` invocations, and a test's count depends on its package:

| Tests | `ledger:check` | `test:unit` | `test:integration` | `cover:check` | Executions |
|---|---|---|---|---|---|
| `contract`: `TestAdmissionLedger`, `TestAdmissionLedgerSchemaSensitivity` (`ledger_test.go:17`, `:63`) | no race | race | race | | 3 |
| `contract`: the other 17 tests | | race | race | | 2 |
| `lifecycletest` (8 tests), `probe` (9 tests) | | race | race | no race | 3 |
| `natsfixture` untagged (19 tests), `runner` (15 tests) | | race | race | | 2 |
| `natsfixture` tagged `integration` (11 tests) | | not compiled | race | | 1 |

Confirmed in the log of main's run 36856716070: `lifecycletest` reports twice (3.038 s; 3.048 s with coverage) and
`cover: lifecycletest 89.5% (unit profile)` a third time. Step timings in that run: `test:unit` 21 s,
`test:integration` 33 s, `cover:check` 2 s. Main's run 36883858915 on `d303c51`, after the fix, shows the same
shape: `ledger:check` reports `contract` in 0.019 s, `lifecycletest` 3.054 s and 3.068 s, and step timings of 1 s,
19 s, 33 s and 2 s.

The canonical integration invocation is also a spec requirement and a test. The requirement is at
`openspec/specs/integration-test-runner/spec.md:76`, which begins "The runner SHALL run" followed by the argv
above; it is asserted at `internal/harness/runner/runner_test.go:420` and recorded at
`docs/admission-ledger.yaml:109`.

No invocation sets `-count` above 1, `-cpu`, `GOMAXPROCS`, or `-shuffle`. `git grep -n -E
'GOMAXPROCS|-cpu[ =]|goleak|GOFLAGS'` has 0 hits. `synctest` now has four, all in
`internal/harness/lifecycletest/refowner_test.go` (`:8`, `:264`, `:274`, `:283`), added by PR #43.

### 3.2 Where and on what

| Fact | Pin or query | Value |
|---|---|---|
| Triggers | `.github/workflows/ci.yml:4` and `:7` | `push:` (main only, `:5`) and `pull_request:` |
| Superseded PR runs | `.github/workflows/ci.yml:13` | `cancel-in-progress: ${{ github.event_name == 'pull_request' }}` |
| Runner | `.github/workflows/ci.yml:21` | `runs-on: ubuntu-latest`; image `ubuntu-24.04` version `20260927.320.1` in runs 36856716070 and 36883858915 |
| Runner size | `scripts/test-integration.sh:338` — `if ! docker info > "$evidence_dir/docker-info.txt" 2>&1; then` | `CPUs: 4`, `Total Memory: 15.61GiB`, `Architecture: x86_64` in the artifacts of runs 36875948219 and 36883858915 |
| Job bound | `.github/workflows/ci.yml:22` | `timeout-minutes: 15` |
| The gate | `.github/workflows/ci.yml:50` | `- run: task verify` |
| Local | `AGENTS.md:47` | ``Run `task verify` before every implementation push; CI runs the same commands in two jobs, `verify` and `required`.`` (prose) |
| Local host | `sysctl -n hw.ncpu`, `go version` | 12 logical CPUs, `go1.26.6 darwin/arm64`, shared by several agent sessions |
| Local reds | `.agents/protocol.md:42` | `worktrees fix the git collision, not the CPU one. CI is the arbiter; a local red under contention is not a finding.` |

The runner's size is recorded only as a side effect: no script names a CPU count (`grep -n -E
'nproc|cpu|CPU|GOMAXPROCS|sysctl' scripts/doctor.sh scripts/test-integration.sh`: 0 hits), no log line prints it,
and the file that holds it lives in an artifact kept for 14 days. Two artifacts were read; whether every hosted run
gets the same size is **documented, not measured**. The repository is public (`gh repo view --json visibility`).

So before merge a new or changed untagged test runs two or three times at `-count=1` per CI run, on one 4-CPU
hosted runner, for each head pushed; a tagged integration test runs once. After merge it runs the same way once on
`main`.

### 3.3 The required job and the ruleset

`gh api repos/C360Studio/semengine/rulesets/24272345` (`main-required-checks`, enforcement `active`, target
`~DEFAULT_BRANCH`), re-read for this revision and unchanged; it is the only ruleset, and `main` has no classic
branch protection (`gh api .../branches/main/protection`: 404 "Branch not protected"):

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
- With `strict_required_status_checks_policy: false`, a `Required` result stays valid on its head however far
  `main` has moved. Two open pull requests show it. PR #14 (Dependabot, not a draft) is green from run 36779771840
  of 2026-09-30T21:29Z, 3.6 hours before the harness merged; its log reports `go: 0 package(s) found`, so that
  green ran none of today's 81 tests. PR #39's head `74db5cc` is green from run 36872937180, which ran on the old
  code.

### 3.4 Reruns and re-rolls

- `gh api 'repos/C360Studio/semengine/actions/runs?per_page=100'`: 45 runs in the repository's life (42 `CI`, 3
  `Dependabot Updates`). `CI`: 33 success, 7 failure, 2 cancelled. Maximum `run_attempt`: 1. No run has ever been
  re-run here.
- The seven failures:
  - `docs:check` on `f6ceab4` (PR #12).
  - `TestIgnoredInterruptIsDetected` on `ec8e685` (PR #13, failed in both the unit and integration steps of one
    run; `b8ec759` "fix(runner): portable SIGINT probe ..." followed two commits later).
  - #40 by name, four times: 36869511251 (PR #21, `be2203c`, integration step), 36871402708 (PR #39, `c799b31`,
    unit step), 36872498596 (PR #21, `41137eb`, unit step), 36878923325 (PR #44, `4d4d893`, unit step). The last is
    the run on the commit that added the first version of this inventory.
  - One with no test named: 36875948219 (PR #44, `97e53e6`), `verify: FAILED: cover:check`. Section 4, Masking.
- The first version of this inventory reported 36875948219 as "in progress" and counted five failures. That run had
  finished red at 14:25Z, 20 minutes before the inventory's commit. Its tree is identical to `6a006df`, whose own
  run (36856716070) passed.
- The two cancelled runs (36851343738 on `a120165`, PR #21; 36871295554 on `16ee22b`, PR #39) are superseded
  pull-request runs, cancelled by the concurrency group at `ci.yml:11-13`. A head whose run was cancelled has no
  result at all.
- `github.run_attempt` is available to a workflow and is already used:
  `.github/workflows/ci.yml:59` — `name: integration-evidence-${{ github.run_id }}-${{ github.run_attempt }}`.
  The run object carries `run_attempt` and `previous_attempt_url` (null at attempt 1, observed on run 36871402708).
- **Documented, not measured:** a re-run keeps the run id, increments `run_attempt`, and replaces the check result
  on the commit; earlier attempts stay readable at `/actions/runs/{id}/attempts/{n}`. A new push creates a new run
  at attempt 1. This inventory did not exercise a re-run: doing so would change the repository's Actions record,
  which a read-only role does not do. SemStreams' record shows the attempt URLs in use (section 7.6.4).
- A re-roll by push has already happened twice, with no rerun involved:

| PR | Red head (run) | Later heads | Result |
|---|---|---|---|
| #39 | `c799b31` (36871402708, 13:48Z) | `ef8b722`, `74db5cc` | both success, attempt 1 |
| #21 | `be2203c` (36869511251, 13:33Z) | `8a4c2b8`, `61fcfbe`, `d8dd15a` | all success, attempt 1; then `41137eb` failed again (13:56Z) |
| #44 | `97e53e6` (36875948219, 14:23Z); `4d4d893` (36878923325, 14:45Z) | `4d96860` | success, attempt 1, on a tree that contains the fix: not a re-roll |

- While #40 was open (13:57Z to 15:23Z), PR #39's head `74db5cc` had `Required` green. The ruleset would have
  admitted that merge once the PR was undrafted; only the prose rule stood in the way. #40 was filed at 13:57Z, 24
  minutes and three green heads after the first occurrence. PR #39's head has not moved since: its green is from
  before the fix. PR #43's body says in prose that PR #21, PR #39 and PR #44 "each then needs `main` merged in and
  CI run over the fixed tree"; no command asks for it (section 3.3, last bullet).
- `.github/workflows/ci.yml:15-16` — `permissions:` / `contents: read`. No job in CI can create or comment on an
  issue or a pull request. The repository default is `default_workflow_permissions: write`
  (`gh api .../actions/permissions/workflow`); the workflow narrows it.
- Evidence of a failed attempt is an artifact kept for 14 days: `.github/workflows/ci.yml:63` — `retention-days: 14`.
  The artifact holds `.evidence/` only (`ci.yml:60`), which is the integration step's record. A unit-step or
  `cover:check` failure leaves nothing in it but the log.

## 4. Survival: what finds a latent flake, files it, or notices a skip

- **No run of the suite without a push.** `ci.yml:3-7` has no `schedule` and no `workflow_dispatch`
  (`git grep -n -i -E 'schedule:|cron|workflow_dispatch|merge_group'` finds only `.github/dependabot.yml:5`, `:9`,
  `:14`). `main` has run CI four times in total (`819c461`, `34c9dc6`, `6a006df`, `d303c51`).
- **One thing does run on a clock: Dependabot.** `.github/dependabot.yml:5-6`, `:9-10` and `:14-15` set
  `schedule:` / `interval: weekly` for three ecosystems. It has run once: three `Dependabot Updates` runs (event
  `dynamic`, 2026-09-30T21:28:39Z, six seconds after the file reached `main`) and one pull request, #14, opened
  51 seconds later with the labels `dependencies` and `javascript`, which Dependabot applied itself. A Dependabot
  pull request triggers the `CI` workflow like any other. This is the only automated writer in the repository, and
  the only schedule. The first version said "no workflow can write" without naming it.
- **Nothing files an issue.** #40 and #41 were both opened by hand for the same flake, 14 minutes apart, by two
  sessions; PR #43 and PR #45 were both opened to fix it, 8 minutes apart. Every issue and every pull request but
  one is authored by the login `cglusky` (`gh issue list --state all --json author`: 35 of 35; pull requests: 9 of
  10, the other is Dependabot's); the owner ruling on #40 is recorded as "posted by the Claude session on the
  owner's word". There is no `class:flake` label here (`gh label list`); #40 and #41 carry `bug` and a `flake(...)`
  title prefix.
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
  took `natsfixture`, `lifecycletest` or `probe` below 80% (`scripts/cover-check.sh:14`). Measured here in a
  scratch module (`go1.26.4`, one skipping test and one passing test): plain `go test -count=1 ./...` prints one
  `ok` line and nothing about the skip; `go test -json` emits one `"Action":"skip"` record. Every invocation in
  section 3.1 runs without `-v` and without `-json`, so a CI log cannot show whether `runner_test.go:264` fired.
- **Masking.** Two mechanisms hide a failure's name:
  - `-failfast` (`scripts/test-integration.sh:402`) stops a package at its first failure, so a second flaky test
    behind the first is not reported in that run. `scripts/verify.sh:24` keeps going across steps.
  - `scripts/cover-check.sh:22` sends `go test`'s standard output to `/dev/null`, and `:10` is `set -euo pipefail`.
    A unit test that fails in that run ends the script there with status 1 and prints nothing: Go writes test
    failures to standard output. Every other exit of the script prints a line (`:24`, `:30`, `:35` to standard
    error; `:64`, `:67`, `:77` to standard output). Run 36875948219 is this case. Its log shows only `task: Failed
    to run task "cover:check": exit status 1` after the step header, 2.2 s after it started. Its artifact shows
    `.evidence/last-run` present and `go_test_status=0` in `runner.env`, which rules out the exits at `:23-26` and
    `:29-32`. So a test in `lifecycletest` or `probe` failed in the no-race run, and nothing recorded which.
  - Nothing tests that path. `internal/harness/contract/cover_test.go:49` — `out, err := exec.Command("bash",
    script, u, i).CombinedOutput()` always passes two profile arguments, and the no-argument mode (`:19-37` of the
    script) is the one `task cover:check` runs.
- **Baseline for a count.** 81 top-level `Test`/`Fuzz` functions in 16 files, 11 of them in the tagged file
  (`git grep -c -E '^func (Test|Fuzz)' -- '*_test.go'`); 80 before PR #43.

## 5. The #40 flake as an input (closed by `d303c51`)

Status at `4d96860`: #40 is closed by the squash merge of PR #43 (`d303c51`, 2026-10-01T15:23Z). #41 was closed as a
duplicate at 14:34Z (state reason `not_planned`), and PR #45, a second fix claim, was closed unmerged a few seconds
earlier. The flake lived on `main` for 14.3 hours, from PR #13's merge at 01:05Z.

### 5.1 The site, then and now

At `97e53e6` (old code, historical):

- `internal/harness/lifecycletest/refowner_test.go:245` — `finalize(t, o)` inside
  `TestEachFailpointTripsExactlyItsCheck` (`:216`). `finalize` (`:184`) called `finish` then read `Observe`
  (`:186-187`); `Observe` read `workerDone` without blocking (`:147-150`).
- `:108-111` — the `abortStopDropsCause` branch called `o.finishLocked()`, unlocked, and returned nil.

At `4d96860`:

- `internal/harness/lifecycletest/refowner_test.go:117` — `case abortStopDropsCause:`; `:121-122` unlock and return
  nil. `finishLocked()` is gone from the branch, and the comment at `:118-120` says why.
- `:256` — `finalize(t, o)` inside `TestEachFailpointTripsExactlyItsCheck` (`:227`). `finalize` (`:195`) calls
  `finish` then reads `Observe` (`:197-198`); `Observe` (`:153`) still reads `workerDone` without blocking
  (`:157-163`).
- `:268` — `func TestAbortStopThenFinishJoinsWorker(t *testing.T) {`, new: it holds the worker at its exit with a
  `probe.Callback` (`:276`) inside `synctest.Test` (`:274`) and uses `synctest.Wait()` (`:283`) to decide whether
  the second Stop returned before the worker exited.

### 5.2 Root cause, as PR #43 records it

The defect was in the test double, not in the suite's checks. Under `abortStopDropsCause`, `Stop` with an ended
context signalled the worker, marked the owner stopped, and returned nil without waiting for the worker. `finalize`'s
later `Stop` then returned at once because the owner was already marked not running. Neither `Stop` joined the
worker, and `finalize` read the owner's state before the worker goroutine had closed `workerDone`. The failpoint was
meant to break one rule and broke two. PR #43's independent review re-derived this from `origin/main` and proved the
new test's sensitivity by mutation: with the old line restored it failed 60 of 60 for `abortStopDropsCause` and
passed 60 of 60 for the other eight failpoints. This inventory read the two versions of the branch above and did
not repeat the mutation.

The adopter path reads the same way: `internal/harness/lifecycletest/lifecycletest.go:171` — `return
requireNothingRetained(o, "after a controlled Stop returned nil")` calls `o.Observe()` (`:314`) immediately
after a Stop that returned nil, and `Run` calls `finish` after every check (`:92`). An adopter's `Observe` must
already reflect the join at the moment its Stop returns nil.

### 5.3 Reproduction rates

All rows run `go test -count=N -run 'TestEachFailpointTripsExactlyItsCheck/abortStopDropsCause'
./internal/harness/lifecycletest/` and count the subtest's `--- FAIL` lines. Every rate is one uncontrolled sample
on a host shared with other agent sessions; they agree on direction and not on size. "First version" means measured
for the first version of this inventory at `97e53e6`, load average 2.6 to 3.5, and not re-measured.

Old code, with `-race`:

| Setting | Failures | Source |
|---|---|---|
| default (12 CPUs) | 0 of 300 | first version; reviewer of this inventory re-ran: 0 of 300; #41: 300 of 300 passed |
| `-cpu 4` | 0 of 300 | first version |
| `-cpu 4` | 11 of 1000 | PR #43 body (developer agent); not re-measured |
| `-cpu 2` | 1 of 300 | first version |
| `-cpu 2` | 1 of 1000 | PR #43 body; not re-measured |
| `-cpu 1` | 24 of 300 | first version |
| `-cpu 1` | 41 of 1000 | PR #43 body; not re-measured |
| `-cpu 1` | 20 of 300 | reviewer of this inventory (PR #44 comment 5934062218); not re-measured |
| `GOMAXPROCS=1 -cpu 1` | 15 of 300 | first version |
| `GOMAXPROCS=1 -cpu 1` | 32 of 500 | #41 and PR #43 comment 5933632906; not re-measured |
| whole package, `-count=20`, default | 0 failures, 41.5 s | first version |

Old code, without `-race`. The first version did not measure this setting at all:

| Setting | Failures | Source |
|---|---|---|
| `-cpu 1` | 300 of 300 | reviewer of this inventory; not re-measured at that count |
| `-cpu 1` | 99 of 100 | PR #43 comment 5934063413 (`main` `6a006df`, clean `git archive` copy) |
| `-cpu 1` | 100 of 100 | re-measured here, bounded: `git archive 97e53e6` copy outside the repository, `-count=100`, load average 3.4 to 4.0 |
| `-cpu 4` | 1 of 100 | measured here, same copy; the hosted runner has 4 CPUs |
| default | 1 of 300 | reviewer of this inventory; not re-measured at that count |
| `-cpu 12` (this host's default) | 1 of 100 | re-measured here, same copy |

Fixed code:

| Setting | Failures | Source |
|---|---|---|
| `-race -count=1000 -cpu 1,2,4`, the subtest and the new test | 0 | PR #43 body; not re-measured |
| `-race`, the new test at `-cpu 1,2,4`; the whole matrix 1000 times at each of `-cpu 1`, `2`, `4` | 0 of 3000; 0 of 27,000 subtests | PR #43 review; not re-measured |
| no race, `-cpu 1`; no race, default; no race, `-cpu 1`, the new test | 0 of 300 each | PR #43 comment 5934063413 (`6f82aa9`); not re-measured at that count |
| no race, `-cpu 1` and `-cpu 4`, the subtest and the new test together | 0 `--- FAIL` lines in 100 each | re-measured here at `4d96860`, bounded |

In CI: 22 completed runs executed the old code after the harness reached `main` (`CI` runs created from
2026-10-01T01:05Z with a success or failure conclusion, less the two fix heads `6f82aa9` and `1755f95` and the
post-fix heads `d303c51` and `4d96860`). Each ran the subtest three times: twice under `-race` and once without.
Four of the 44 `-race` executions failed by name. At most one of the 22 no-race executions failed: the unidentified
`cover:check` red, which fits this flake and cannot be proved to be it (section 4, Masking). The first version's "3
of 19" is superseded.

It was introduced by PR #13 and passed that PR's CI. PR #13's body records no repeated-run evidence
(`gh pr view 13 --json body | grep -i -E 'count=|stress|flak'`: 0 hits).

### 5.4 Inputs routed to #42 from PR #43's review

Each is recorded on #42 (comment 5934306943) and checked here against the code at `4d96860`.

- **An unjoined worker remains in `finalize`.** `refowner_test.go:127-131`: under
  `stopReturnsNilWithWorkerRunning`, `Stop` marks the owner stopped and returns nil without signalling the worker.
  `:201-207`: `finalize` then calls `o.cancel()` (`:204`) and returns; nothing waits on `workerDone`. `:198`
  exempts this failpoint from the "still holds" assertion, so no assertion reads the worker afterwards. Read here;
  the review's statement that it cannot cause a false failure follows from `:198` and was not tested.
- **The new test's failpoint list is written by hand.** `refowner_test.go:269-272` lists nine values. The declared
  set is the ten constants at `:16-27`; the matrix table at `:233-241` lists nine (all but `clean`); the new test
  omits `stopReturnsNilWithWorkerRunning` on purpose (`:267`). Nothing derives one list from another or compares
  them: `git grep -n failpoint -- '*.go'` shows no enumeration, and a failpoint added to `:16-27` enters neither
  test unless someone adds it.
- **A repeated run of the matrix costs real time.** Each execution of `TestEachFailpointTripsExactlyItsCheck` waits
  out the suite's `grace` once (section 2.3): 2.00 s measured here. One thousand executions are therefore about 33
  minutes per CPU setting before race-detector overhead, and about 100 minutes for the three settings of
  `-cpu 1,2,4`. #42's comment says "about 100 minutes per CPU setting"; that figure was not re-measured and does
  not follow from 2 s alone. Selecting only the `abortStopDropsCause` subtest avoids the wait: 100 executions took
  0.2 s here. PR #43's review notes that the subtest that waits, run inside a `synctest` bubble, would not consume
  real time; that was not measured.
- **Without `-race`, one CPU decides it for this class.** The rows above: 99 to 100 percent at `-cpu 1` without
  `-race`, against 0 of 300 under `-race` at the default. `cover:check` is the only invocation in `task verify`
  that runs these two packages without `-race` (section 3.1), on a 4-CPU runner.

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
| #1175 | not recorded for the failure itself: the closing comment says what held port 36060 "was never determined". The issue is about the guard, a port preflight that printed OK. Three defects were found in it: it probed 2 of the 46 ports the Compose files bind; it ran beside the teardown as a parallel Task `deps:` entry and answered 0.8 s before the teardown finished; and its `lsof` probe, run as non-root on Linux, cannot see the root-owned `docker-proxy` sockets it existed to catch | PR run, required job, on an OpenSpec-only commit | the three guard defects were fixed (in PR #1148); closed by hand on the owner's word. The closing comment lists under "Not claimed": "That this fixes the failure that surfaced it" | 1 in 15 runs |
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

The #1175 row is corrected in this revision. The first version classed it as a port collision closed by a fix to
its guard; the issue's own closing comment does not claim the failure was fixed or its cause found.

### 6.2 Counts by root-cause shape

| Shape | Issues | Count |
|---|---|---|
| Unsynchronised observation of asynchronous state | in-process: #1059, #1061; broker or service state: #1069, #1336, #1375, #1394 | 6 |
| Predicted wall-clock budget or assertion | #1063, #1157, #1284, #1290, #1397 | 5 |
| Root cause not recorded | #469, #1054, #1175, #1317, #1421 | 5 |
| Port collision (probe-then-bind, host ports) | #1120, #1279 | 2 |
| External or toolchain dependency inside a gate | #1340, #1358 | 2 |
| Substrate contention (Docker) | #736 | 1 |
| Unbounded cleanup | #1062 | 1 |
| Production defect that presented as a flake | #1010 | 1 |

Two of the five without a recorded cause, #1175 and #1317, show the same symptom: a host-port bind that failed
after a preflight passed.

### 6.3 Counts by closing mechanism

| Mechanism | Issues | Count |
|---|---|---|
| Fix to the test or harness, no production change | #1054, #1059, #1061, #1062, #1063, #1069, #1157, #1290, #1336, #1375, #1394, #1397 | 12 |
| Closed with no fix recorded for the failure itself | #736, #1175, #1421 | 3 |
| Fix to infrastructure or CI | #1279, #1358 | 2 |
| Fix to production code | #1010, #1120 | 2 |
| Open | #469, #1317 | 2 |
| Assertion deleted on an owner ruling | #1284 | 1 |
| Check moved out of the test suite into a lint guard | #1340 | 1 |
| Skip or quarantine | none | 0 |

So 18 of the 23 closed with a fix to the failure that was filed, 3 closed without one, and 2 are open. #1175
differs from the other two closed without one: its guard was repaired in three ways while the failure that exposed
the guard stayed unexplained.

Related facts from the same record:

- **Retries.** Two fixes are waits that retry only on one typed error (#1069, #1336). The others replace a timing
  inference with a signal.
- **Reruns and waivers.** Six issues record a merge past the flake or a rerun: #1069 ("merged past the gate ... put
  main red"), #1157, #1317, #1375, #1397, #1421. Section 7.6.4 records how each waived merge actually got past the
  required check.
- **Tier move.** PR #1385, while closing #1157 and #1340, also moved six real-Ollama tests from the required
  integration tier to a `live_llm` tier.
- **Detection.** Hosted PR run on a diff that could not have caused it: 10 (#469, #1069, #1175, #1279, #1284,
  #1317, #1336, #1375, #1394, #1421). A session's local gate run: 8 (#736, #1061, #1062, #1063, #1157, #1290,
  #1340, #1397). `main` run first: 2 (#1054, #1358). A person outside CI: 2 (#1010, #1120). Not stated: 1
  (#1059). None was found by a scheduled or repeated run. At the pin no workflow has a `schedule` trigger:
  `ci.yml` runs on `push` and `pull_request`; `e2e-ladder.yml` on `pull_request` and `workflow_dispatch`;
  `container.yml` on `workflow_run` and `push`; `release.yml` on `push`; `semspec-validation.yml` and
  `sister-validation.yml` on `workflow_dispatch`.
- **A recorded position on scheduled runs.** `.github/workflows/e2e-ladder.yml:26-28` at the pin — `Owner ruling
  (2026-08-27, #1117): wiring it here is PER-PR, not` / `nightly — schedule-triggered runs are reserved for
  out-of-band security` / `scanning (CVE and similar), never functional e2e.` SemStreams PR #1322's body adds, about
  #1317: "No continuing observation automation is scheduled."
- **Isolated repetition.** Where the record says, re-running the test alone at default settings did not reproduce
  the failure in eleven issues (#469, #736, #1059, #1061, #1062, #1063, #1069, #1157, #1290, #1340, #1375).
  It reproduced under a single CPU (#1059), concurrent load (#1290), or a forced delay (#1375).
- **Guards created by the closing work.** Three: the pin-check lint guard (#1340), the port reservation with its
  fixture test (#1279), and the corrected preflight (#1175). File lists of PRs #1072, #1291, #1337, #1389, #1395
  and #1408 show test or test-and-docs changes only. PR #1068 also touches `test/testinfra/policy_baseline.json`:
  it deletes two entries from an existing guard's baseline and adds no guard (section 6.5).

### 6.4 Shapes that recurred after a guard or rule existed

| Guard or rule (date) | Kind | Later issues of the same shape | Why the guard did not apply, as recorded |
|---|---|---|---|
| Fixed-port lint, #220 Subclass 2 (2026-06-03) | failing command | #1120, #1175, #1279, #1317 | #1120 binds `:0` and re-binds, which the regex passes and the guard's message recommends; the others bind in Compose files, outside `*_test.go` |
| #220's wall-clock rule: rationale comment and at least 3x tolerance (2026-06-03) | prose, reviewer signal | #750, #1063, #1157, #1284, #1290, #1397 | #1063 is a test #220's own table names as unfiled, filed 81 days later; PR #1285 records that the 3x rule "was satisfied all along ... and was never the problem" |
| `integration-time-sleep` ratchet in `test/testinfra/policy_guard_test.go` (2026-08-03) | failing command with a baseline of accepted debt | #1063 (filed 2026-08-23) | the test's two sleeps were baseline entries of kind `migration-debt`, so the guard passed them; the line that failed was an elapsed-time assertion, which is not one of the guard's categories; sleeps in files without the `integration` tag are outside it (section 6.5) |
| #750 closed by a comment at the assertion (2026-07-30) | documentation | #1284 | "a known-unfixed flake in a required job has no open issue behind it" |
| `-p 2` cap for Docker contention (PR #518, again PR #1344 on 2026-09-19) | runner flag | #736 occurrences on 2026-09-25, 09-27 and 09-29 | the suggested `-p 1` was measured as 94% slower on the full suite (2026-07-31) and recorded as falsified again on 2026-09-19 |
| Port preflight with a hand-written port list (before 2026-08-29) | preflight | #1175 | it covered 2 of 46 ports, raced the teardown, and could not see root-owned listeners; it printed OK |
| Port preflight derived from Compose (2026-08-30) | preflight | #1279 | "the preflight is what passed here": a snapshot 77 s before the bind |
| Port reservation (PR #1280, 2026-09-10) | infrastructure | #1317 | reservation verified 63 s before the bind failed; cause open |
| #1284's ruling against predicted budgets (2026-09-11) | owner ruling, prose | #1290 the next day, #1397 two weeks later | #1290 "landed inside the #1284 fix"; #1397's test "kept its four 3s budgets when #1291 landed" |
| #1062's two-site fix (2026-08-24) | point fix | #1283 hang; #1417, open, counts 334 reviewed unbounded cleanup entries | the guard (PR #1414) landed 2026-09-29 |
| Merge-gate rule "no rerun-to-green" (updated 2026-08-24, per #1069) | prose | 14 of the 23 issues were filed after it; six record a waiver or rerun | the rule acts once a flake is known: fix it, or file it and obtain a waiver |

### 6.5 Guards at the pin outside `scripts/`

The first version looked for guards only under `scripts/` and reported that SemStreams had no sleep guard. Under
`scripts/` that is true (guard-named files there: `check-cleanup-roots.sh`, `e2e-check-ports.sh`,
`lint-nats-kv-sdk-pin.sh`, `lint-test-ports.sh`, each with a fixture test, and `run-integration-tests.sh`). The
pin's tree was then listed in full (`gh api .../git/trees/8b99efe9...?recursive=1`, 4680 files, not truncated) and
filtered for `guard|policy|baseline|lint|ratchet`. Two Go guards live under `test/testinfra/`.

**The policy guard.** `test/testinfra/policy_guard_test.go` (625 lines, `package testinfra_test`, no build tag, so
any `go test ./...` runs it):

| Pin (at `8b99efe9`) | Line text | Role |
|---|---|---|
| `test/testinfra/policy_guard_test.go:56` | `func TestInfrastructurePolicyGuard(t *testing.T) {` | parses the repository's Go files and compares the findings with a baseline |
| `:62` | `if stats.GoFiles < 100 \|\| stats.TestFiles < 100 \|\| stats.IntegrationFiles == 0 \|\| stats.Calls == 0 {` | fails when it scanned too little: "a zero/near-zero scan is a broken guard, not a clean repository" (`:63`) |
| `:261-266` | `allCategories := []string{` and four entries | `untagged-container`, `direct-container-api`, `fabricated-testing-t`, `integration-time-sleep` |
| `:412` | `if integration && isImportedSelector(call.Fun, imports, "time", "Sleep") {` | the sleep category matches a `time.Sleep` call only in a file whose build constraint requires `integration` |
| `:239` | `name: "unit sleep is outside this integration ratchet",` | a fixture stating that limit |
| `:155` | `func TestInfrastructurePolicyFixtureMatrix(t *testing.T) {` | the sensitivity test; `:292-296` require a positive and a negative fixture per category |
| `:149-150` | `if category == "untagged-container" \|\| category == "direct-container-api" {` | two categories may never be baselined |
| `:114` | `func TestInfrastructurePolicyBaselineHasNoWritePath(t *testing.T) {` | the test file may not contain code that rewrites the baseline |
| `:595` | `t.Errorf("stale policy baseline entries (%d); remove resolved debt so the ratchet cannot hide a regression:\n  %s",` | an entry whose finding is gone fails the test |

Its baseline, `test/testinfra/policy_baseline.json`, holds 254 entries at the pin. All 254 are category
`integration-time-sleep`, kind `migration-debt`, reason "existing synchronization debt recorded when the Gate 0
ratchet landed" (counted by parsing the file). The commit that added the guard, `804cdbd2` of 2026-08-03, held 305;
fifteen commits reachable from the pin touched the file. So over eight weeks the count of accepted sleeps fell by
51, and at the pin 254 `time.Sleep` call sites in integration-tagged tests pass the guard.

SemStreams PR #1068, which closed #1063, removed two of those entries (`gh api .../pulls/1068/files`, the patch for
`policy_baseline.json`): the keys for `processor/agentic-tools/tools_integration_test.go`,
`TestIntegration_ToolConcurrentExecution`, `time.Sleep(100 * time.Millisecond)` and `time.Sleep(200 *
time.Millisecond)`. That is the test #1063 names. Its sleeps had been accepted debt since the guard landed, twenty
days before the flake was filed; the assertion that failed was `elapsed < 800ms`. Whether any other of the 23
issues concerns a test with baselined sleeps was not measured.

**The cleanup guard.** `test/testinfra/cleanup_guard_test.go` with `cleanup_analyzer_test.go`, behind
`scripts/check-cleanup-roots.sh` (`taskfiles/lint.yml:81` — `desc: Refuse new or stale lifecycle cleanup debt before
full verification`). Its `cleanup_baseline.json` holds 234 entries and 97 resolutions at the pin. This repository's
ledger excludes it and carries a grep guard with no baseline instead (section 2.1).

**In this repository.** Neither Go guard is carried, and the policy guard has no ledger row: `git grep -n -i -E
'policy_guard|policy_baseline|integration-time-sleep'` finds nothing outside this change, and
`docs/admission-ledger.yaml` has twelve rows (`:20` to `:190`), none for that file. This repository's archived
inventory records `time.Sleep` in 119 SemStreams test files
(`openspec/changes/archive/2026-09-30-setup-02-isolated-harness/inventory.md:208`) and lists "AST guard" among the
things left out (`:247`).

## 7. The five candidates in #42: overlaps and missing facts

Nothing here evaluates or shapes a candidate.

### 7.1 Stress at birth

Overlaps:

- The four invocations in section 3.1, all `-count=1`. The integration one is fixed by a spec requirement
  (`integration-test-runner/spec.md:76`), a test (`runner_test.go:420`) and a ledger row
  (`docs/admission-ledger.yaml:109`).
- `.agents/skills/semengine-preflight/SKILL.md:65` — `Do not hand-run a narrower command in place of a gate CI
  runs: the task owns flags and pins.` A repeated run outside a task is a hand-run command under this rule.
- Nothing maps a diff to packages. `scripts/verify.sh:16` uses `git diff HEAD` only to fingerprint the tree.
- The module has five packages (`go list ./...`), all under `internal/harness`.
- Heavy local gates are serialised by prose (`.agents/protocol.md:41-42`) and, for the integration step, by the
  host lock, which waits up to an hour (`scripts/test-integration.sh:26`).

Facts established since the first version:

- The hosted runner has 4 CPUs (section 3.2).
- The setting matters more than the count for #40's class. Old code, 100 to 300 executions each: 0 failures under
  `-race` at 12 CPUs; 4% to 8% under `-race` at `-cpu 1`; 99 to 100% without `-race` at `-cpu 1`; about 1% without
  `-race` at 4 or 12 CPUs (section 5.3). No invocation sets `-cpu`.
- In CI the old code failed 4 of 44 `-race` executions by name. That is higher than either local `-race -cpu 4`
  sample (0 of 300; 11 of 1000). The samples are uncontrolled and the difference is unexplained.
- The cost of repeating: 2.00 s of real time per execution of the matrix test, about 33 minutes per thousand per CPU
  setting; the single subtest alone runs 100 times in 0.2 s; the whole `lifecycletest` package at `-race
  -count=20` took 41.5 s (first version). Current step times in CI: `test:unit` 19 s to 21 s, `test:integration`
  33 s to 36 s, against a 15-minute job bound. No per-test timing is kept for unit runs.

Facts not established:

- Whether any repetition count, at any setting, would have failed #40 on PR #13 before it merged. #42's first
  comment, from PR #43's developer, says a stress run at birth "would very likely have caught it"; that rests on the
  local rates.
- How the eleven SemStreams issues that isolated repetition did not reproduce (section 6.3) would have behaved
  under whatever load a stress run applies.

### 7.2 Scheduled suite stress with an auto-filed issue

Overlaps:

- No schedule trigger on the suite and four `main` runs ever (section 4).
- One schedule and one automated writer already exist: Dependabot, weekly, with its own labels and pull requests
  (section 4; collision tables 7.6.2 and 7.6.3).
- The concurrency group at `.github/workflows/ci.yml:11-13` is keyed on workflow and ref, and cancels only
  pull-request runs. A run on `main` that no push started would carry `main`'s ref and share that group with push
  runs (the ref of a scheduled run, and what happens to a pending run in a busy group, are **documented, not
  measured**).
- `contents: read` (`ci.yml:15-16`); the File routing rule (`protocol.md:43-47`); the duplicate-close rule
  (`protocol.md:56-58`); a `class:` label namespace with one member and no flake label; a default `duplicate`
  label with no uses; two hand-filed issues and two fix claims for one flake; evidence artifacts kept 14 days.
- The host lock (`scripts/admission-lock.sh:7` — `readonly
  admission_lock_default="/tmp/semstreams-integration.lock"`) is per host and plays no part between hosted runners.
- SemStreams' owner ruling at the pin reserves schedule-triggered runs for security scanning (section 6.3).

Facts not established:

- How often a scheduled run would have to repeat the suite to see a flake at #40's CI rate.
- What identifies "the same flake" across runs. #40 and #41 differ in title and body for one failure line. The
  line itself, `refowner_test.go:245: finalize refowner (abortStopDropsCause): still holds [worker]`, is identical
  in all four named failures. The unit step keeps no file of its failures; the integration step keeps
  `per-package.txt` (`scripts/test-integration.sh:475`); `cover:check` discards its output.
- **Documented, not measured:** GitHub disables scheduled workflows in a public repository after 60 days without
  activity.

### 7.3 Rerun guard with a linked issue and a recorded waiver

Overlaps:

- `github.run_attempt` is in use (`ci.yml:59`); the maximum observed is 1.
- Both re-rolls observed here were new pushes at attempt 1 (section 3.4). A check on `run_attempt` does not see
  them.
- `required` reads only `needs.*.result` (`ci.yml:76`).
- The waiver's stated home is a PR comment (`protocol.md:51`); a ruling's stated home is an issue comment
  (`protocol.md:10` — `status:blocked`. `status:needs-decision` is the owner's docket; a ruling is posted as an
  issue comment and the`). The one ruling so far on a flake, on #40, is an issue comment, and it is a refusal: "no
  waiver". No waiver has been issued in this repository.
- Owner and agents post under one login (section 4).
- The ruleset has no bypass (`bypass_actors: []`, `current_user_can_bypass: "never"`). With `Required` red on a
  head, a merge can happen only after that head turns green by a re-run, or after a new head turns green. A waiver
  comment does not change the check. SemStreams carried out its waivers by exactly those two routes, the re-run
  among them (section 7.6.4). So the re-run this candidate would gate is one of the two routes a waived merge has.
- The closest existing instance of "an exception passes only with a stated, recorded reason" is the image
  override: `scripts/test-integration.sh:67` — `if [ -z "${SEMENGINE_NATS_IMAGE_OVERRIDE_REASON:-}" ]; then`,
  refused at `:68-69`, warned at `:81`, recorded in `runner.env`
  (`openspec/specs/integration-test-runner/spec.md:80-82`).
- The closest existing durable record of "this may not proceed until X" is an OpenSpec hold (collision table
  7.6.1).

Facts not established: whether "re-run failed jobs" and "re-run all jobs" present differently to `required`; what
permission a job needs to read a prior attempt's conclusion or an issue; how a command would tell an owner's waiver
from an agent's comment; whether GitHub can forbid re-runs at all (no such setting appears in the ruleset or the
repository settings read). None of these was exercised: each needs a re-run or a workflow change in this
repository.

### 7.4 Root-cause guards

Overlaps:

- The two guarded shapes and their three sensitivity patterns (section 2.1); `go tool revive` and `go vet` as
  existing analysis steps (`Taskfile.yml:46`, `:51`).
- An earlier ruling chose a grep guard with no baseline over SemStreams' AST guard and marked the AST guard
  "revisit at 04A" (section 2.1).
- SemStreams' prior art for exactly this candidate: an AST guard for `time.Sleep` in integration-tagged tests, with
  a fixture matrix and a baseline of accepted debt that stood at 254 entries at the pin and had accepted the sleeps
  of a test that later flaked (section 6.5).
- Present counts a new guard would meet at base: `time.Sleep` 0; `time.Now`/`time.Since` in tests 4 lines;
  `time.NewTimer` watchdogs 4; `t.Parallel()` 18; skips 1; instant reads of asynchronous state 4 and unjoined
  goroutines 3 (section 2.2).

Facts not established: a textual signature for unsynchronised observation, the largest SemStreams class (6 of 23)
and the #40 shape; none of those issues records one, and PR #43's same-shape search was done by reading. Whether a
budget that is a contract (SemStreams #1290 kept `elapsed > 4*time.Second` deliberately) can be told from a
predicted one by its text. `testing/synctest` is now used in this tree (`refowner_test.go:274`, `:283`) and passes
in CI on Go 1.26.6 (`go.mod:3`) with no experiment flag (run 36883858915); it is used by one test.

### 7.5 No quarantine

Overlaps: one skip, one build tag, nothing that counts either (section 4);
`.agents/contracts/semengine-reviewer.md:215` — `- **A silent skip, drop, or degrade is a finding at any
severity.** Any path that continues past a failure, takes a` (written about product paths);
`semengine-preflight/SKILL.md:78` asks evidence to separate "failure, skip, no selected tests". In SemStreams no
`class:flake` issue was closed by a skip; one closing PR moved six tests to a non-required tier. PR #43's body
states its fix used "No sleep, retry, skip or timeout change".

Facts not established: whether `runner_test.go:264` has ever fired in CI. A skip is invisible in the logs as they
are produced today (measured, section 4).

### 7.6 Same-class collision tables

Issue #42 names three primitives: a gate on re-runs with a recorded waiver (candidate 3), an issue filed by
automation (candidate 2), and a run of the suite on a schedule (candidate 2; candidate 1 adds repeated runs). They
sit in three different semantic classes with different existing owners, so each has its own table. The first
version had one table, for the first class only, and it missed owners in all three.

#### 7.6.1 The known-flake record and the authority to merge past it

| Dimension | Existing owners and evidence |
|---|---|
| Semantic class | "No known unfixed flake in a required job"; a green over one is rerun-to-green; fix, or file and obtain a waiver (`.agents/protocol.md:50-51`). More generally: the durable record that something may not proceed until a named condition holds, and who may lift it. |
| Owners | (1) The protocol's Land step, restated at `AGENTS.md:70` and `semengine-preflight/SKILL.md:82-83`; no code owner. (2) The `required` job (`ci.yml:67-82`) and ruleset 24272345, which own the only enforced merge condition. (3) OpenSpec holds: unchecked task lines read by `scripts/openspec-queue.sh:82` — `printf '%s' "$t" \| grep -qiE '\bhold\b\|\bblocked\b\|\bblocking\b'    && { echo "BLOCKED"; return; }`; `.agents/protocol.md:20` names them as the home of holds. (4) Labels `status:blocked` and `status:needs-decision` (`protocol.md:9-11`). (5) Milestones as release gates (`protocol.md:14-15`). (6) The admission-gate rule: `docs/setup-plan.md:185-187` says certain gates cannot be waived by "An issue, elapsed audit budget, or passing coverage number", and that a noncritical exception "may have an owner, bounded scope, proving test, and due milestone". (7) Exceptions that a command admits: the image override with a required reason (`scripts/test-integration.sh:67-69`), and the fixed-port marker with none (`scripts/lint-test-ports.sh:41`). (8) This change's own `.openspec.yaml:5` — `skip_specs: true`, with its reason at `:3-4` and its removal condition in `tasks.md` 3.1. (9) The owner, for rulings and waivers. |
| Catalogs | GitHub issues and their comments; PR comments; the Actions run and attempt history; artifact `integration-evidence-<run_id>-<run_attempt>` (`ci.yml:59`); `openspec/changes/<id>/tasks.md`; `runner.env` (`image_override`); the label list (15 labels); four milestones. |
| Status | The `Required` check on a head. `task spec:queue`: at `4d96860` it prints four `BLOCKED` lines for this change and exits 0; `scripts/openspec-queue.sh --strict` exits 1 on the same input (both run here). `status:blocked` is on nine open issues; `status:needs-decision` on none. Pickup reads "failures or waivers" (`.agents/skills/semengine-pickup/SKILL.md:22`). |
| Lifecycle | A waiver is per merge and lives in a PR comment (`protocol.md:51`). A hold lives until its task line is checked; the change is archived in the landing PR (`protocol.md:20-22`). An issue closes by a merged PR that declared `Closes #n`, or by hand on the owner's word (`protocol.md:54-58`). A `Required` result belongs to one head and does not expire when `main` moves (section 3.3). Artifacts expire at 14 days (`ci.yml:63`). |
| Ownership | Rulings are the owner's, on the issue (`AGENTS.md`, Roles). "an approval of adjacent work (a PR, a review round, a design, a waiver) never widens into a close" (`protocol.md:57-58`). One GitHub login for owner and agents. `bypass_actors: []`. |
| Readers | Agents at pickup (`protocol.md:28-29`, which includes `task spec:queue`); reviewers; the owner. `scripts/openspec-queue.sh` reads `tasks.md`. `scripts/cover-check.sh:29` and `:34` read `runner.env`. `required` reads `needs.*.result`. No workflow reads issues, comments, labels or milestones. Nothing calls `--strict`: `Taskfile.yml:94` runs the script with no argument, `task verify` leaves `spec:queue` out (`AGENTS.md:44`), and its header says the flag is "for CI or a pre-archive hook" (`scripts/openspec-queue.sh:33-34`). |
| Writers | Agents and the owner through `gh`, by hand, for issues, comments, labels and milestones. Commits, for `tasks.md`. The workflow writes only check results and artifacts (`ci.yml:15-16`). |
| Recovery | None for the flake record. A flake whose issue is closed without a fix has no record (SemStreams #750, then #1284). The queue script has no test here: `scripts/openspec-queue.sh:73` says SemStreams' fixture test for it is "not ported yet". A green produced before a fix stays green (PR #39). |

#### 7.6.2 A record written by automation, and how a duplicate is recognised

| Dimension | Existing owners and evidence |
|---|---|
| Semantic class | A durable GitHub record that a program, not a person, creates in response to an event, with its labels, its evidence, and the rule that keeps one event from producing two records. |
| Owners | (1) Dependabot: `.github/dependabot.yml:3-15`, three ecosystems; it wrote PR #14 and applied the labels `dependencies` and `javascript` (issue timeline: `labeled` by `dependabot[bot]`). (2) The CI workflow, which writes check runs and one artifact per attempt (`ci.yml:55-63`) and nothing else. (3) The integration runner, which writes the evidence directory the artifact carries (`scripts/test-integration.sh:338`, `:475`, `:528`). (4) Hand filing under the File routing rule (`protocol.md:43-47`). (5) The duplicate rule: "A close with no merged PR behind it (duplicate, stale, fixed elsewhere) takes the owner's word on the issue itself" (`protocol.md:56-57`). (6) GitHub's vulnerability alerts, enabled (`gh api .../vulnerability-alerts`: 204); automated security fixes are disabled. |
| Catalogs | The label list: a `class:` namespace with one member, `class:port-refactor` (12 issues, #25 to #36); `bug` (5 issues, among them #40 and #41); the default `duplicate` label (0 issues); no `class:flake`. The `flake(<package>):` title prefix (2 of 2 flake issues). The artifact list (32 artifacts). The pull-request list, where a non-draft PR is not a claim (`protocol.md:28`). |
| Status | Issue state and reason: #40 `completed`; #41 `not_planned`. #41's timeline has no `marked_as_duplicate` event and no `duplicate` label; its one comment says "Duplicate of #40". Milestone: #41 is in "Setup: foundation through contract"; #40 and #42 are in none. |
| Lifecycle | Dependabot runs weekly; PR #14 has been open since 2026-09-30 with a green that predates every test in the tree (section 3.3). An issue closes by a merged PR or by hand. Artifacts expire at 14 days, so an issue that cites one outlives its evidence. |
| Ownership | Two writers of record: the login `cglusky` (owner and all agents) and `app/dependabot`. Rulings are the owner's. |
| Readers | Pickup: `gh issue list --state open` and `gh pr list` with "drafts are claims; skip them" (`protocol.md:28`). Reviewers and the owner. No program reads an issue. |
| Writers | People and agents by hand; Dependabot. No job in the `CI` workflow can write an issue or a comment (`contents: read`). |
| Recovery | Duplicates have happened and were resolved by hand: #40 and #41, 14 minutes apart; PR #43 and PR #45, 8 minutes apart; both closed with a comment naming the earlier one. #41's close does not quote an owner word, which `protocol.md:56-57` asks for. Nothing detects a duplicate before it is filed except the Start ritual's listing. |

#### 7.6.3 Runs no push started, and coordination of runs on a ref or host

| Dimension | Existing owners and evidence |
|---|---|
| Semantic class | When the required suite runs, how many times, and what keeps two runs from colliding or one from being lost. |
| Owners | (1) The workflow triggers (`ci.yml:3-7`): `push` to `main`, and `pull_request`. (2) The concurrency group (`ci.yml:11-13` — `group: ci-${{ github.workflow }}-${{ github.ref }}` at `:12`), with the comment at `:9-10`: "Superseded PR runs are cancelled; pushes to main are not, so two quick merges never leave a cancelled Required on a commit that was never broken." (3) Dependabot's weekly schedule (`dependabot.yml:5-6`, `:9-10`, `:14-15`) and the `Dependabot Updates` workflow GitHub runs for it. (4) The invocations that fix each test's count and flags (section 3.1) and the step order (`scripts/verify.sh:10-11`). (5) The host admission lock (`scripts/admission-lock.sh:7`; wait bound `scripts/test-integration.sh:26`), shared byte-compatibly with SemStreams on one host. (6) Prose: "Heavy local gates run one agent at a time on a shared host" (`protocol.md:41`). (7) Bounds: `-p 2` and `-timeout 10m` (`scripts/test-integration.sh:402`); job timeouts (`ci.yml:22`, `:72`). |
| Catalogs | The Actions run list (45 runs) and workflow list (`CI`; `Dependabot Updates`, path `dynamic/dependabot/dependabot-updates`). `.evidence/<run>` per integration run, with `.evidence/last-run` pointing at the latest (`scripts/test-integration.sh:528`). Step timings in each log (`scripts/verify.sh:29-30`). |
| Status | A run's conclusion; `Required` on a head; `task doctor`'s admission line (it reports the lock `free` in every CI log read). |
| Lifecycle | A superseded pull-request run is cancelled: two observed (section 3.4). A push run on `main` is never cancelled in progress. Nothing starts a run on a clock except Dependabot, which has run once. Nothing can start one by hand: there is no `workflow_dispatch`, so the only manual path is a re-run, never used. Replacement of a pending run in a busy group, and disabling of schedules after 60 days of inactivity, are **documented, not measured**. |
| Ownership | The group key is workflow plus ref: every push run on `main` shares `ci-CI-refs/heads/main`, and each pull request has its own. The host lock is exclusive per host; on a hosted runner it is always free. |
| Readers | The `required` job and the ruleset (the result on a head); pickup (`gh run list --branch main --limit 3`, `protocol.md:29`); `cover:check` (the last integration run in the worktree). |
| Writers | GitHub, on a push, a pull-request event, or Dependabot's schedule. No one else. |
| Recovery | A stale host lock is quarantined by the next runner (`scripts/test-integration.sh:185`). A cancelled run is replaced by the next head's run, and the cancelled head keeps no result. Nothing re-runs a red `main`; pickup's three-run listing is the only place a red `main` is looked for. |

#### 7.6.4 How a waived merge gets past a required check

The first version recorded that the ruleset has no bypass and that SemStreams used waivers, and did not connect
them. In this repository no waiver has been used, so the only record of the mechanics is SemStreams'. Read from the
paragraphs that name a waiver in the pull requests the search in section 0 returned; the full threads were not
read.

| SemStreams PR | What the record says | Route |
|---|---|---|
| #1322 (merged 2026-09-18) | Waiver comment 5729152733. Body: "GitHub rejected an admin merge because the required aggregate check is red and the ruleset does not permit that bypass. One rerun of the cancelled Test job and its dependent status check was then started on the identical head"; attempt 2 passed. | same-head re-run |
| #1373 (merged 2026-09-25) | Waiver comment 5828024246, then comment 5828257046: "the repository ruleset allows no admin bypass of `CI Status Check`, so the owner chose to re-run the failed Test job once, knowing #1375 is a filed, waived flake." | same-head re-run |
| #1109 (merged 2026-08-27) | Comment 5441559001: the red was on intermediate head `19b0f386`; the job "has been green on `2f56384f`, `386cd8c2`, `f59a492f`, `b18fd518`, and every head since". | green on a later head |
| #1404 (merged 2026-09-29) | Body: `Test` failed on `49a540d6`, "Not re-run"; #1421 waived "this merge only"; the stop point is at a later head, `dbcfc295`. | green on a later head |
| #1379, #1429, #1435 | A known flake (#1375, #1421) waived for the merge while the merging head's own checks were green ("first attempt"; "Merge still requires current-head CI green"; "passed without a rerun"). | no red on the merging head |
| #1388 (merged 2026-09-27) | #1397 waived; the red was a local gate, and "Hosted CI is green on this head". | no red on the merging head |

In those records no waived merge landed while its required check was red. Each landed on a green: twice from a
re-run of the same head, otherwise from a later or untouched head. This repository's ruleset has the same property
(`bypass_actors: []`): a merge past a red `Required` needs a green on the merging head by one of those two routes.

## 8. Surface inventory categories

1. **The claimed gap.** #42's three absences, measured: nothing repeats a test beyond the one to three `-count=1`
   executions (section 3.1); nothing runs the suite without a push (section 4); nothing stops a rerun, and nothing
   stops a re-roll by push either (section 3.4). The merge-gate flake clause is prose (section 3.3). Two things #42
   treats as absent do exist in another form: a schedule and an automated writer (Dependabot), and a durable
   record of blocking conditions with a strict mode nobody calls (OpenSpec holds, section 7.6.1).
2. **Every current spelling.**
   - The flake clause: `protocol.md:50-51`, `AGENTS.md:70`, `semengine-preflight/SKILL.md:82-83`; pickup reads
     "failures or waivers" (`semengine-pickup/SKILL.md:22`).
   - The sleep and synchronisation rule: four prose homes (section 2.2).
   - The test invocation: `Taskfile.yml:64`, `Taskfile.yml:74`, `scripts/test-integration.sh:402`,
     `scripts/cover-check.sh:22`, the spec at `integration-test-runner/spec.md:76`, the assertion at
     `runner_test.go:420`, the ledger at `docs/admission-ledger.yaml:109`, and `docs/testing.md:66` on PR #39. The
     rule about who may run a narrower one: `semengine-preflight/SKILL.md:65`.
   - What `verify` runs: `scripts/verify.sh:10-11`; a stale list at `AGENTS.md:43`;
     `semengine-developer.md:189-192`, which lists the older gate set and says no integration gate exists yet; and
     `semengine-preflight/SKILL.md:44`.
   - The record of a blocking condition: OpenSpec holds, two `status:` labels, milestones, the waiver comment, and
     the two command-admitted exceptions (section 7.6.1).
   - The set of failpoints: three hand-kept lists in one file (section 5.4).
3. **Adjacent claims.**
   - PR #43, merged as `d303c51`: the fix for #40, with three inputs routed to #42 (section 5.4). #40 is closed.
   - Issue #41 and PR #45: the duplicate issue and the second fix claim for the same flake, both closed.
   - PR #39 and issue #37: rules table and `docs/testing.md`, unmerged; they would add the "review only" index and
     the testing page this work touches. PR #39's head is green from before the fix.
   - PR #21: red at head `41137eb` on the old code.
   - PR #14: Dependabot's open pull request, green from before the harness existed.
   - Issue #38: a decision on another lifecycle-suite check.
   - The earlier ruling for a grep guard over the AST guard, "revisit at 04A", and the ledger row that records it
     (section 2.1).
   - Current specs `harness-boundaries`, `integration-test-runner`, `lifecycle-suite`, `nats-fixture`. The
     integration argv is a spec requirement (`integration-test-runner/spec.md:76`).
   - This change is the only active OpenSpec change (`ls openspec/changes`: `archive`, `flake-defense`). No ADR
     directory (`ls docs`).
4. **The consumer at birth.** This phase proposes no surface. Present consumers of any defense: the authors of the
   16 test files that hold tests, the `verify` CI job, and the pickup ritual. `internal/harness` is under
   `internal/`, so another module cannot import it; `docs/repository-map.md:9` says the only Go code is the test
   harness under `internal/harness/`.
5. **The problem shape.**
   - Refuse a shape with a failing command, proved by a planted violation: `addresses_test.go:21` with `:26`;
     `scripts/cleanup-roots-check.sh` with `cleanuproots_test.go:18`; `scripts/lint-test-ports.sh` with its shell
     fixture test.
   - Refuse a shape against a baseline of accepted debt: no instance here (`scripts/cleanup-roots-check.sh:7`
     states a zero baseline); SemStreams' policy and cleanup guards at the pin (section 6.5).
   - Admit an exception only with a recorded reason: `scripts/test-integration.sh:67-69`.
   - Exempt by inline marker: `scripts/lint-test-ports.sh:41`.
   - Refuse evidence that does not belong to the tree it claims: `scripts/cover-check.sh:29` — `if ! grep -qx
     'go_test_status=0' "$run/runner.env" 2>/dev/null; then` and `:34`.
   - Say so when a scan was empty: `scripts/gopkgs.sh:9` — `echo "go: $n package(s) found in module $(go list
     -m)"`. The opposite case is `scripts/cover-check.sh:22`, which discards the output that would name a failure.
   - Admit or refuse at a seam: `natsfixture` admission (`nats-fixture/spec.md:9-12`).
   - Record a blocking condition and report it on demand: `scripts/openspec-queue.sh` (section 7.6.1).
   - Force an interleaving instead of waiting for the scheduler to produce it:
     `refowner_test.go:268` (`TestAbortStopThenFinishJoinsWorker`), built on `probe.Callback` and `synctest`; the
     plan names "explicit callback-entered/release/join probes" as part of the helper kit
     (`docs/setup-plan.md:264`).

Context retention: this work touches test and CI machinery only; there is no production Go code in the tree
(`docs/repository-map.md:9`), and `TestNoRetainedContext` (`internal/harness/contract/context_test.go:22`) covers
the harness's non-test structs. The two goroutines in non-test harness code that are not joined are listed in
section 2.2.

## 9. Adopter seam: the test author

The adopter is a developer or agent writing a test here who has not read the contracts, and later a consumer
repository's author who copies `internal/harness` patterns (they cannot import them, section 8 item 4).

1. **What must they know today?** Thirteen facts, each a debt:
   1. Wait on a signal, never a sleep (prose).
   2. A wall-clock assertion needs a rationale and tolerance (prose).
   3. No `t.Parallel()` around process-global state (prose).
   4. Cleanup needs a bounded context; only the one-line form is refused.
   5. No fixed ports; only `net.Listen` literals and four broker literals are refused, and probe-then-bind is not.
      The guard's own message recommends probe-then-bind.
   6. Docker tests carry the `integration` tag and run through the runner.
   7. An untagged test runs twice per verification under `-race`, the second time two packages at a time beside
      container tests; tests in `lifecycletest` and `probe` run a third time without `-race`.
   8. A local red under contention "is not a finding" (`protocol.md:42`).
   9. A red from a test they did not touch must be filed and fixed, not re-pushed past (`protocol.md:50-51`).
   10. `lifecycletest.Run` classifies their owner against `grace = 2s` and reads `Observe` the moment Stop
       returns nil.
   11. `-race` passing says nothing about an observation that races the scheduler, and the race detector changes
       the timing enough to hide it: #40 failed almost every time on one CPU without `-race` and never at the
       default with it.
   12. A red `cover:check` that prints nothing means a unit test failed in the run without `-race`.
   13. A green on their pull request may be older than `main`; nothing asks for a run over the current tree.
2. **What happens if they do nothing?** The test passes locally and in the two or three CI executions, merges, and
   later fails on another author's unrelated pull request. That is what #40 did: introduced by PR #13, green there,
   first red 12.5 hours after merge on a docs-only diff. The next push usually comes back green. Four more runs then
   went red on three pull requests before the fix reached `main`: three named this test and one named nothing.
3. **Where do they find out?** Facts 4 and 5 (in their matched forms) and 6: a failing `task verify` step or a
   typed `ErrNotAdmitted`. Facts 1, 2, 3, 8, 9: a contract or a page (`docs/testing.md` is not merged). Facts 7,
   10, 11, 12, 13: nowhere. For the author of the flake, the eventual signal reaches someone else.
4. **What should they have to know?** Nothing. The gap is all thirteen items less the three with a command behind
   them, and for the three, the unmatched forms listed in section 2.1.

Observation versus prediction: the budgets in section 2.3 are values a test author predicts. Three SemStreams
issues (#1284, #1290, #1397) each record a predicted number sitting below a bound the framework already enforced.

## 10. Intent check

| Capability | Relation to the stated purpose | Relation to the owner direction |
|---|---|---|
| Flake defense for the test harness and CI | Not a product boundary. `AGENTS.md` ("What this is for") makes SemEngine a framework of primitives and contracts; the harness is the only Go code today and every later proof rests on it. No consumer domain semantics are involved. | The direction asks that flakes be fixed and that "the bleeding" stop. Measured against it: SemStreams closed 18 of 23 with a fix to the filed failure, one at a time. Its two largest recorded shapes (11 of 23) had no failing command that matched them and kept recurring; the one adjacent command, the sleep ratchet, had accepted 254 existing sleeps at the pin. This repository has a failing command for two shapes, prose for the rest, no repeated or scheduled run of the suite, and a merge gate whose flake clause no command reads. Its first flake lived 14.3 hours on `main`, was filed twice and claimed twice, and turned four runs red by name; a fifth red in that time named no test. |

## 11. Not established

- Which test failed in run 36875948219. The evidence fits a unit-test failure in `cover:check`'s no-race run; the
  output that would name it was discarded.
- Whether every hosted run gets the 4-CPU, 15.61 GiB runner seen in the two artifacts read.
- Why the old code failed more often in CI under `-race` (4 of 44) than in either local `-race -cpu 4` sample.
- Whether `TestIgnoredInterruptIsDetected`'s red on PR #13 at `ec8e685` was nondeterministic.
- The root causes of SemStreams #469, #1054, #1175 (what held port 36060), #1317 and #1421: the issues do not
  record them.
- Whether #736's close by PR #1404 was intended. The mechanism (a closing keyword in a docket option) is inferred
  from the PR body; no comment on #736 records a fix or a decision to close.
- Whether any SemStreams flake other than #1063 concerned a test whose sleeps were in the policy guard's baseline.
- Where SemStreams #1059 was first detected.
- How each SemStreams waiver was carried out beyond the eight pull requests in section 7.6.4: only the paragraphs
  naming a waiver were read, in the pull requests one search returned.
- GitHub behaviors marked "documented, not measured". None was exercised, because each needs a re-run or a
  workflow change in this repository:
  - re-run semantics (`run_attempt`, replacement of the check result), and whether "re-run failed jobs" and
    "re-run all jobs" differ to `required`;
  - whether re-runs can be forbidden at all;
  - disabling of scheduled workflows after 60 days, the ref a scheduled run carries, and replacement of a pending
    run in a concurrency group on `main`;
  - what a job with `contents: read` can read of issues and earlier attempts;
  - Dependabot's cadence beyond its first run, and how it avoids opening a second pull request for one update.
- The "about 100 minutes per CPU setting" figure on #42, and whether running the 2 s subtest inside a `synctest`
  bubble removes the wait.
- Whether `govulncheck`'s database fetch and `actions/setup-go`'s module cache behave as documented.
- What reads `skip_specs` in this change's `.openspec.yaml`: the key is cited as a recorded exemption; the
  OpenSpec CLI's handling of it was not measured.
- Any flake in SemStreams that was never filed. The label is a floor (section 6).

Settled since the first version: the root cause of #40 (PR #43, reviewed); the role of
`test/testinfra/policy_baseline.json` (section 6.5); skip visibility without `-v` (measured, section 4);
`testing/synctest` on Go 1.26.6 (used in the tree and green in CI).

## 12. Corrections in this revision

The independent review of the first version is PR #44 comment 5934062218. Each finding and where it is answered:

| Finding | Answer |
|---|---|
| BLOCKING 1: an automated writer on a schedule (Dependabot) is missing | sections 4, 7.2, 7.6.2 (Owners, Writers), 7.6.3 (Owners, Lifecycle) |
| BLOCKING 2: catalog and dedupe (`class:` namespace, `duplicate` label, the duplicate-close rule) | section 7.6.2 (Catalogs, Status, Recovery) |
| BLOCKING 3: durable blocking-condition record (OpenSpec holds, `--strict`, the exception-record shape, milestones) | section 7.6.1 (Owners, Status, Readers); `--strict` run here: exit 1 |
| BLOCKING 4: the concurrency group | sections 3.4, 7.2, 7.6.3 (Owners, Lifecycle, Ownership) |
| BLOCKING 5: how a waiver is carried out | sections 7.3 and 7.6.4 |
| HIGH 1: the base commit's own run failed, silently | sections 1, 3.4, 4 (Masking), 5.3; the artifact was read to rule out two exits |
| HIGH 2: the run without `-race` was never measured | sections 1, 5.3 (re-measured, bounded), 5.4, 7.1 |
| HIGH 3: SemStreams' sleep guard is left out | sections 6.4 (new row), 6.5 (new), 7.4 |
| HIGH 4: the #1175 row is wrong | sections 6.1, 6.2, 6.3, 6.4 (new row), 10, 11 |
| MEDIUM: `ledger:check` is a fourth execution | section 3.1 |
| MEDIUM: other unsynchronised-observation sites | section 2.2 (seven sites, three of them new) |
| MEDIUM: rates differ between samples | section 5.3 (every sample with its source) |
| MEDIUM: `freePort` at the pin, and `:52` | section 2.1 |
| MEDIUM: `semengine-preflight/SKILL.md:65` | sections 7.1 and 8 item 2 |
| MEDIUM: PR #45 and the close of #41 | sections 5, 7.6.2 (Recovery), 8 item 3 |
| NIT: "PR #43 has no content commit yet" is stale | section 5 rewritten against `d303c51` |
| NIT: PR #44's title named three candidates | retitled by the reviewing session; nothing to change in this file |
| To measure 1 to 3: GitHub re-run, forbid, schedule and concurrency behavior | still not measured; section 11 says why |
| To measure 4: the runner's CPU count | measured from two artifacts: 4 (section 3.2) |
| To measure 5: skip visibility | measured (section 4) |

Surfaces this revision adds that the review did not name:

- network fetches inside the required job (section 2.5);
- four test watchdogs and three script budgets among the time budgets (section 2.3);
- two instant reads in the integration tests, and the suite's own abandoned goroutine (section 2.2);
- a green that predates `main`, on PR #14 and PR #39 (section 3.3);
- SemStreams' cleanup guard and its baseline, and this repository's earlier ruling against carrying it (sections
  2.1 and 6.5);
- SemStreams' recorded owner ruling on scheduled runs (section 6.3);
- the absence of any caller for `scripts/openspec-queue.sh --strict`, and of any test for that script (section
  7.6.1);
- the missing test for `cover-check.sh`'s no-argument mode was named by the review; the artifact check that closes
  two of its exits is new (section 4).

The three inputs from PR #43's review are in section 5.4, each pinned at `4d96860`.
