# Inventory: simplify-04a-05-prochost-helper-exit

base: `d5ea8f672eed5215cef6ebb40f79d129950f42e5` (main `e60229334b8d8da3c0350e17802e2ea9631f91c8` plus the empty
claim commit of PR #264)

Question: why a `prochost` helper outlives a test process that ends without running its cleanup, what already decides
when a helper ends, and what the operating system and Go do when the test process ends in each way it can. This
change carries no package from the SemStreams pin.

A **helper** is a function in a test binary that `prochost.Start` runs in a child process by re-running the test
binary. The **test process** is the test binary that called `Start`. A test process **ends without cleanup** when it
exits by any path that skips `t.Cleanup`: a signal it does not handle, SIGKILL, a panic, `testing`'s `-timeout`
alarm, or `os.Exit`.

## 1. The claimed gap

- `internal/harness/prochost/prochost.go:91` — `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`: the helper
  runs in a process group of its own, so a terminal's Ctrl-C (sent to the foreground group) does not reach it.
- `internal/harness/prochost/prochost.go:98-99` — `t.Cleanup(func() {` / `if err := p.Kill(); err != nil {`: the only
  thing in the tree that ends a helper the test did not end itself.
- `internal/harness/prochost/prochost.go:88-90` — `cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")`
  …`cmd.Stdout, cmd.Stderr = stdout, stderr`: `cmd.Stdin` is not set, so the helper's stdin is the null device and
  no pipe links it to the test process today. The issue's "use what the helper already has (for example, exit when
  the pipe to its parent closes)" names a pipe that does not exist yet; adding it is the change.
- `processor/graph-ingest/settlement_integration_test.go:88-89` — `signal.Notify(terminated, syscall.SIGTERM)` /
  `<-terminated`: the one caller's helpers park until SIGTERM, so nothing ends them once the cleanup is skipped.
- Real trigger: PR #206 comment 6136509092, step 2: pid 62014,
  `graph-ingest.test -test.run=^TestHelperProcess$`, PPID 1, PGID 62014, state `SN`, holding a TCP connection to a
  leftover NATS container after a Ctrl-C to the run's process group; stopped by hand with `kill -TERM 62014`.
- Searches that found no existing mechanism: `git grep -n -E "Pdeathsig|StdinPipe|os\.Stdin" -- '*.go'` → no hit.

## 2. Every place that decides when a helper ends

| Site | What it does |
| --- | --- |
| `prochost.go:98-107` | cleanup: SIGKILL to the group, then a wait bounded by `cleanupBudget` (`:35`, 30 s) |
| `prochost.go:176-184` | `Kill`: SIGKILL to the group via `signalGroup` (`:144`, `syscall.Kill(-pgid, sig)`) |
| `prochost.go:153-166` | `Signal`, `Pause` (`:169`), `Resume` (`:172`): group signals the test sends |
| `prochost.go:41-47` | `Helper`: `fn()` then `os.Exit(0)`; a helper whose `fn` returns exits 0 |
| `settlement_integration_test.go:88-89` | the caller's helper ends on SIGTERM; `:284` `first.Kill()` |
| `prochost_test.go:43-50` | `parkUntilSIGTERM`, `block`: this package's test helpers end on SIGTERM |

Nothing in the child observes the test process.

## 3. Adjacent claims

- Spec `openspec/specs/process-host/spec.md:48-56`, "No orphan survives the test": covers a test that returns
  (cleanup runs); silent on a test process that ends without cleanup. `:13-17` requires the helper's own process
  group.
- Spec `openspec/specs/nats-fixture/spec.md:175` — `a prochost child, or a run whose -run selects no test that
  resets, runs the package's TestMain`: a helper starts no container; unaffected.
- Spec `integration-test-runner`: no process-group or helper requirement remains (`grep -n -i "process group\|helper"`
  → one unrelated line, `:49`). Test-infra audit removed the runner's group handling (archived
  `2026-10-15-test-infra-audit/design.md:89`, L4: "Ctrl-C reaches the terminal's foreground group; leftovers go to
  the sweep") and listed prochost as L18 (`inventory.md:325-329`) with no disposition on interrupted runs.
- Spec `harness-boundaries`: no `time.Sleep` under `internal/harness/`, no bare `select {}`; no prochost requirement.
- `docs/test-infrastructure.md:127-128` — `A test process that is killed (Ctrl-C, a crash, or its -timeout) runs no
  cleanup, so its containers stay.` The 30-minute sweep removes containers, not processes.
- `docs/test-infrastructure.md:146-148`: harness code exists only for a failure on today's tree that `go test`,
  `t.Cleanup`, testcontainers or Task do not handle, and its doc comment names that failure and its issue. `go test`
  and `t.Cleanup` do not handle this one (probe P1, rows A).
- Why the group exists: archived `2026-10-05-setup-04a-01-floor/design.md:149-154` — "in its own process group … Reused
  from `runner_test.go`: … `Setpgid` and group signalling". The reuse came from the removed runner's tests, which
  needed group signals to reach a shell script's children (archived `2026-10-01-setup-04a-foundation/inventory.md:
  365`). No stated reason is specific to prochost. Group signalling keeps `Kill` and `Pause` reaching any process a
  helper starts; no helper starts one today (`prochost_test.go:25-34`, `settlement_integration_test.go:64-66`).
- `docs/testing.md:114-119` and `prochost.go:10-12` explain the SIGTERM park with Go's deadlock detector. Probe P3
  changes that (below).
- Open pull requests (`gh pr list --state open --json number,title,changedFiles,files`, 2026-10-15): #264 (this claim,
  no files), #263 (graph-ingest query prefix; touches `processor/graph-ingest/component_test.go`, not the settlement
  test), #261 (vocabulary), Dependabot #242-#248 (`go.mod`, `go.sum`, `package*.json`, `ci.yml`). None touches
  `internal/harness/prochost`, `settlement_integration_test.go`, `docs/testing.md` or the `process-host` spec.
- Open issues (`gh issue list --state open --search <q>` for `prochost`, `"process group"`, `TestHelperProcess`,
  `settlement_integration_test`, `process-host`): #208 (this); #223 (epic, lists #208 in its entry condition). #131,
  #136 and #9 name the words only, not the package.
- No admission-ledger row (`grep -n prochost docs/admission-ledger.yaml` → none: harness code, not ported). No
  coverage floor (`scripts/cover-check.sh:24-26` lists `lifecycletest`, `probe`, `natsfixture`).

## 4. Who it is for

No exported symbol is added. The one caller in this repository is `settlement_integration_test.go:142`
(`prochost.Start`). It gains the behaviour without a change.

## 5. The problem shape

The shape: a child process that ends when its parent ends. Other child processes the tree starts:

- `internal/harness/mutcheck/runs.go:38-41` and `internal/harness/pindiff/pin.go:88-92` — own group, killed by
  `exec.CommandContext`'s `Cancel` when the parent's context ends. A parent killed by SIGKILL does not cancel the
  context. Both children are bounded work (a `go test` run, a `git fetch`) that ends on its own, and neither waits
  for a signal.
- `message/generic_json_shape_test.go:167`, `natsclient/test_helpers_integration_test.go:197` and
  `processor/graph-ingest/test_owner_child_test.go:67` — `exec.CommandContext` re-execs with `CombinedOutput`. Each
  child runs one test and exits, so none waits for a signal.

None of these is an instance of "a helper that would otherwise wait forever learns that its parent is gone". This
change establishes the shape inside `prochost` only. Adoption sweep: none of the five starters above waits for a
signal, so none should adopt it. A child that waits for a signal should be started through `prochost`.

## 6. Pin decision records

None. This change carries no package from the pin.

## Collision table, adopter seam inventory, intent check

- **Collision table: not triggered.** This is test-harness process handling, not a durable, communication or runtime
  coordination primitive.
- **Adopter seam inventory: not applicable.** `internal/harness/prochost` cannot be imported from outside this module
  (Go's `internal` rule), and production code may not import it (contract test T-B1, `prochost.go:7-8`).
  - For in-repo test authors there is one new fact: a helper's stdin belongs to `prochost`.
  - If they do nothing, the default path is safe, because no helper reads stdin today (`git grep -n "os\.Stdin" --
    '*.go'` → none).
  - They find out from the doc comment.
- **Intent check: not triggered.** No boundary moves.

## Probes (go1.26.9 darwin/arm64, Darwin 25.5.0)

Source, commands and output: PR #264 comment 6198200054
(<https://github.com/C360Studio/semengine/pull/264#issuecomment-6198200054>). The probe is a stand-alone module at
`go 1.26.9`, outside the repository.

- A parent test binary starts a helper by re-executing itself.
- A shell runs the parent in a process group of its own, standing in for a terminal's foreground group.
- The parent is ended in one of six ways.
- The driver then checks with `kill -0`, for up to 5 s, whether the helper's pid is still alive.

**P1. Is the helper gone within 5 s after its parent ends?**

| Parent ends by | A: own group, no pipe (today) | B: shared group, no pipe | C: own group, stdin EOF exit |
| --- | --- | --- | --- |
| `os.Exit(1)`, no cleanup | survives (PPID 1) | survives | gone |
| SIGINT to the parent's group (Ctrl-C), exit 130 | survives | gone | gone |
| SIGTERM to the parent alone, exit 143 | survives | survives | gone |
| SIGKILL to the parent alone, exit 137 | survives | survives | gone |
| panic in the test, exit 2 | survives | survives | gone |
| `-test.timeout=2s` alarm panic, exit 2 | survives | survives | gone |

In C, the helper printed `helper: stdin EOF, exiting` each time. The parent's exit code 130 on SIGINT shows that a Go
test binary does not catch SIGINT, so its cleanups do not run, matching the issue.

**P2. Paused helpers and two helpers.**

- A helper stopped with SIGSTOP whose parent is SIGKILLed was gone within 5 s in A, C and in a shared group with the
  pipe. In C it printed no end-of-file line, so it was not the pipe that ended it. The stopped process cannot read.
- Assumed cause: the rule for an orphaned process group that holds a stopped process (SIGHUP, then SIGCONT). This
  probe did not observe the signal. The result is measured on macOS only and is not promised.
- With two helpers started from one parent, `lsof` showed each helper holding only its own pipe's read end. Both
  ended on the parent's SIGKILL and on its SIGTERM.

**P3. The deadlock detector.**

- A program whose main goroutine blocks on a nil channel exits 2 with `all goroutines are asleep` when nothing else
  runs.
- With a goroutine reading stdin until end-of-file, it stayed up until its stdin (a shell pipe) closed, then exited
  through the watcher (status 3).
- The watcher therefore removes the deadlock exit that `prochost.go:10-12` and `docs/testing.md:117-119` describe.
  This was measured with a shell pipe, not an `os/exec` pipe.
