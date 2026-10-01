# Design: flake-defense

This is the architect's design for issue #42, third revision. It was written after `inventory.md` in this folder
(third revision, base `4d96860`, sha256 `a632315a30abec4cb56a32c86a7986d34e5d5ceb8352d640db8db6d389eba7b4`) received
`INVENTORY PASS` on PR #44 (comment 5935397241). "Inventory 3.3" and similar point into that file, which is the
evidence base and is not repeated here.

How it got here:

- The first revision (at `0f30b12`, sha256 `14ccfa9efae6b3debcb336209c494e499dabbb98a7786ba7dd49e4d5c41f26c1`) went to
  independent design review, which returned DESIGN CHANGES REQUESTED on PR #44 (comment 5936056866): two HIGH, eight
  MEDIUM and two NIT findings.
- The second revision (at `6895c76`, sha256 `e8671ebc42aa3f18d7810724a69335bc4626fde692fb84beec21363171118552`)
  answered them. Its re-check returned DESIGN REVIEW PASS (comment 5936590347) and left four findings to apply, A to
  D.
- The owner then ruled twice on #42: on questions Q1 to Q5 (comment 5937751262), and to strike the slice 04A
  measurement (comment 5937807011).

This revision records the two rulings and applies findings A to D. It decides nothing new. "The owner's ruling" is the
last section. "Corrections after design review", before it, lists each finding and where it was answered. The revision
still needs a check confined to its own diff (task 2.4), and nothing in this file says that check has passed.

Line pins are at `0f30b12`, which contains `main` at `9286055`. Every pin was read again there for the second
revision. This revision was written at `bba268a`. Between the two commits only this change's own files differ (`git
diff --stat 0f30b12 bba268a`), so the pins hold. Measurements carry one of three marks:

- **Measured for this design:** taken on 2026-10-01 at `6c56846`, on a scratch copy outside the repository, on a
  12-CPU host shared with other sessions (load average 1.5 to 3.1).
- **Measured for the correction pass:** taken on 2026-10-01 at `0f30b12`. These are reads of GitHub with `gh`. Under
  this mark the second revision also carried counts and two test runs taken over a copy of the SemStreams pin
  snapshot. The owner struck them, and none remains in this file (see "Effects on slice 04A").
- **Measured for the ruling pass:** taken on 2026-10-01 at `bba268a`. These are reads of GitHub with `gh`.

Words used below:

- **Flake:** a test or CI step that passes and fails on the same code.
- **Re-roll:** getting a green after a red without fixing the cause, by re-running the same commit or by pushing
  another one.
- **Known flake:** from this design on, an open GitHub issue labelled `class:flake`. Only a failure that a pull
  request in this repository can end is filed under that label (ruled, Q1a).
- **Closing reference:** an issue GitHub will close when a pull request merges. It comes from a line such as
  `Closes #40` in the pull request's description, or from a link made by hand in the pull request's side panel.
- **Exemption:** what a pull request gets from the known-flake check by closing every open flake (D8).
- **Exception:** a way past an open flake without a fix, granted by the owner. The owner ruled that there is none
  (Q1b).
- **One-CPU run:** `go test -cpu 1`. The test binary gets one scheduler thread, so a goroutine that was just started
  does not run until the test's own goroutine blocks or yields.
- **Bubble:** `testing/synctest` runs a function in an isolated group of goroutines with a fake clock. The clock jumps
  forward when every goroutine in the group is blocked, so a timer costs no real time.
- **Up to date:** a pull request's head contains the current tip of `main`.

## Context

What the inventory established, in the order a flake travels:

- **Born.** Two test shapes have a failing command (fixed ports, unbounded cleanup contexts). Sleeps, skips,
  wall-clock assertions, unsynchronised observation, `t.Parallel` over shared state and test order have prose only
  (inventory 2.2). The tree has no `time.Sleep` call and one skip.
- **Merged.** A test without a build tag runs two or three times per `task verify`, always `-count=1`, never with
  `-cpu` set (inventory 3.1). The #40 defect passed that on PR #13. On the old code it failed 0 of 300 under `-race`
  at the default CPU count and 99 to 100 of 100 without `-race` at one CPU (inventory 5.3).
- **Survives.** A `Required` green stays valid however far `main` has moved; PR #14 and PR #39 each hold one
  (inventory 3.3). While #40 was open, PR #39's head was green and only a prose rule stood in the way of merging it
  (inventory 3.4). Both re-rolls so far were new pushes; no run has ever been re-run. One red named no test at all,
  because `scripts/cover-check.sh` discards test output (inventory 4).
- **SemStreams.** 23 flake issues, 18 closed by a fix to the filed failure, one at a time. The two largest shapes
  (11 of 23) had no failing command. The one adjacent command accepted 254 existing sleeps, and a test that later
  flaked was among them (inventory 6.2, 6.5). Six issues record a merge past the flake or a re-run (inventory 6.3).

## The owner's direction

Recorded on #42, comment 5935179232, before these options existed. It has three parts, kept apart here.

**1. Five recommendations, accepted.** The owner's words: "otherwise i accept your recommendaations on 1-5".

1. Stress at birth, reshaped: vary the scheduling and repeat the whole suite, in a task; make the 2 s matrix test
   instant first.
2. Scheduled suite stress: not now. No issue is filed by automation.
3. Rerun guard: replaced by (a) a pull request must be up to date with `main`, and (b) `Required` fails while a
   flake issue is open. The mechanism for (b) was left open.
4. Root-cause guards with no list of accepted exceptions; fix the port guard's pointer.
5. No quarantine, as a check and not a review rule.

Outside the five: fix `cover-check.sh`; network fetches in the required job are covered by no candidate.

**2. The owner's own words** on waivers and on the unmeasured GitHub behaviours:

> if the owner waivers help we can keep them but i likely waived too much and for too long. on the GH runs - about
> all we can do is start with best practices, lessons learned and solid policy and measure as we go while not
> allowing debt to be ignored

**3. The session's reading of those words.** It is the session's, not a ruling: any waiver kept is narrow and has an
end, and the default is no waiver; the unmeasured behaviours do not block the design and are stated as assumptions.

The owner's words allow waivers to stay "if they help". The second revision recommended keeping none. That went
further than the owner's words, so it was put to the owner as Q1b and not decided here. The owner ruled that there is
none (see "The owner's ruling").

## Goals and non-goals

Goals, each tied to the stage it closes:

- A test whose result depends on the scheduler fails before it merges, for the one shape measured here.
- A sleep, a skip, or a test hidden behind a build tag fails a command, with no list of accepted exceptions.
- A green older than `main` cannot merge.
- A known flake stops merges until it is fixed, by a command and not by memory.
- A failing gate always names the test that failed.

Non-goals: a scheduled run; any issue filed by automation; a guard on re-runs; retries; quarantine; an AST guard or
a baseline of accepted debt; changes to the Docker-backed invocation; fixing network fetches in CI.

## Options for the change as a whole

| Option | What it adds | What it leaves open | Cost |
|---|---|---|---|
| O0 Do nothing | nothing | every stage above; the #40 path repeats | none now; SemStreams' record is the later cost |
| O1 Extend existing guards only | sleep, skip and build-tag checks beside the existing ones; `cover-check.sh` names failures; the port guard's message; the matrix test made instant | #40's shape has no text to match (inventory 7.4), so birth is unchanged for the largest class; a stale green and a merge past a known flake stay possible | three small tests, no CI time |
| O2 O1 plus varied, repeated runs and the up-to-date rule | one-CPU runs with and without `-race`, five repetitions without it; the ruleset refuses a head behind `main` | the known-flake rule stays prose | about 90 s per run (measured); one more CI run per pull request for each merge that lands ahead of it |
| O3 O2 plus a known-flake check in CI (recommended) | a CI job that fails while a `class:flake` issue is open, unless the pull request closes every open one; the up-to-date rule read back from GitHub on every run | a red that nobody files; re-rolls before filing; a fix that is declared and not real; what changes after a run (see D8) | a second CI job that reads GitHub; a repository-wide stop while a flake is open |
| O4 O3 with a merge queue in place of the up-to-date rule | checks run again at merge time on the merged result, so no pull request updates by hand and nothing that changed after the last run slips past | nothing more than O3 | a `merge_group` trigger, a queue to operate, and more GitHub behaviour that nothing here has measured |
| O5 The five candidates as first filed in #42 | per-diff stress, a nightly run that files issues, a guard on re-runs with a waiver, a guard with accepted exceptions | see the direction above: a re-run guard would have seen nothing (no run was ever re-run); a baseline accepted the sleeps of a test that later flaked | a schedule the owner's SemStreams ruling reserves for security scanning; an automated writer; a waiver path |

**Recommendation: O3.** It is the accepted direction with the mechanism for (b) filled in. O1 alone does not reach
the shape that produced #40. O2 leaves the one rule SemStreams broke most often, the merge past a known flake, in
prose. O4 is the better mechanism for merge-time truth and is named here so it can be picked up when the up-to-date
rule's cost is felt; today it buys little for four open pull requests and adds unmeasured behaviour.

## Decisions

Each decision gives its premises with the measurement, the check that fails when the rule is broken, and how that
check is itself shown able to fail. The repository's guards pair each check with a sensitivity test, which plants a
violation and requires the check to name it; the new checks follow that pattern (`addresses_test.go:21` with `:26`).

### D1 The unit run uses one CPU under the race detector

`task test:unit` becomes `go test -race -count=1 -cpu 1 ./...`. The Docker-backed step already runs the same tests
under `-race` on every CPU (`scripts/test-integration.sh:56`, `:402`), so the two existing race runs now differ
instead of repeating each other.

- Premise: the old #40 code failed 0 of 300 under `-race` at the default and 4% to 8% under `-race` at one CPU
  (inventory 5.3). Measured for this design with the matrix inside a bubble (D4): 6 of 100 at `-race -cpu 1`,
  0 of 100 at `-race -cpu 4`.
- Premise: `-cpu 1` does not serialise parallel tests, because `-parallel` keeps its default. Measured for this
  design: `runner` takes 18.0 s at `-cpu 1` and 18.2 s at the default; the whole command takes 20.8 s (19 s in CI
  today).
- Check: D3.

### D2 A new step repeats the unit suite at one CPU without the race detector

`task test:repeat` runs `go test -count=5 -cpu 1 -shuffle=on ./...` and accepts packages after `--`, as
`test:integration` does. `scripts/verify.sh` runs it last, since it is the most expensive step. `-shuffle=on` runs
the tests of each package in a different random order each time and prints the seed on failure; it gives test-order
dependence, which has no check today (inventory 2.2), a failing command for one flag.

- Premise: the setting matters more than the count for #40's class (inventory 7.1). Measured for this design, old
  defect restored, matrix inside a bubble, 100 executions each: 100 failures at `-cpu 1`; 6 at `-race -cpu 1`; 1 at
  `-cpu 4`; 0 at `-race -cpu 4`. So the run without `-race` at one CPU is the one worth repeating, and a repeated
  `-race` run at one CPU would double the step's cost for a setting that finds this class about one time in sixteen.
- Premise, cost: measured for this design, the exact command on a scratch copy passes in 90.8 s; `runner` is 90.3 s of
  that (5 x 18.1 s) and the other four packages finish in under 14 s. `task verify` takes 61 s of steps in CI today
  (run 36883858915). No projection is made for ported code (see "Effects on slice 04A").
- The count is one number in one place. Five is a cost choice, not a derived one: for a flake that fails a one-CPU
  execution with probability p, five executions catch it with probability 1 - (1 - p)^5, which is 97% at p = 0.5,
  67% at p = 0.2 and 23% at p = 0.05. The number rises when `runner`'s tests get cheaper; it is never lowered to buy
  time without the owner's ruling (see "Not now").
- Check: D3. Evidence the step works on the one flake this repository has had: task 6.3.

After D1 and D2, a test without a build tag runs seven times per `task verify` under three settings; a test in
`lifecycletest` or `probe` runs an eighth time under a fourth (`cover:check`, no `-race`, all CPUs). The Docker-backed
tests are not repeated. They stay at one execution, which the owner ruled as a declared cost (Q2).

### D3 The invocations are pinned by a contract test

A new test in `internal/harness/contract` parses `Taskfile.yml` and `scripts/verify.sh` and fails unless
`test:unit` and `test:repeat` hold exactly the command lines of the spec and `verify.sh` lists both steps with
`test:repeat` last. This is the shape the integration argv already has (`runner_test.go:420` behind
`integration-test-runner/spec.md:76`).

- Premise: nothing pins the unit invocation today. `git grep -n -E 'count=1|"-race"' -- '*_test.go'` finds one
  line, `runner_test.go:420`, which is the integration argv.
- Shown able to fail: its sensitivity test plants a Taskfile with the count lowered, one without `-cpu 1`, one with
  `-race` added to the repeat step, a `verify.sh` without the step, and a `verify.sh` with the step not last, and
  requires each to be named.
- No test invokes the real `go` tool. SemStreams #1340 was a test that shelled out to the toolchain and blocked on
  its lock (inventory 6.1); the tests here read files or put a fake `go` or `gh` first on `PATH`, as the runner's
  contract tests do.

### D4 The matrix test runs in a bubble and has one failpoint table

`TestEachFailpointTripsExactlyItsCheck` waits out the suite's 2 s grace in real time on every execution (inventory
2.3, 5.4). Each failpoint's subtest moves inside `synctest.Test`.

- Premise, measured for this design on a scratch copy: the matrix then reports 0.00 s (2.00 s before); the package
  at `-race -count=20` takes 1.4 s (41.5 s before, inventory 5.3). This settles the open item in inventory 11.
- Premise: the bubble does not hide #40's class. With the old defect restored, the matrix inside a bubble failed 100
  of 100 at `-cpu 1`, and `TestAbortStopThenFinishJoinsWorker` failed on the first run.
- `finalize` joins the worker of `stopReturnsNilWithWorkerRunning` before it returns, and its "still holds"
  assertion loses the exemption at `refowner_test.go:198`. Today it cancels at `:204` and returns.
- The ten failpoints (`:16-27`), the matrix table (`:233-241`) and the list in the forced-interleaving test
  (`:269-272`) become one table. Both tests take their cases from it, and a declared failpoint with no expected
  check fails a test.
- Check: the lifecycle suite's own tests. Shown able to fail: the completeness check is a function over the table,
  tested with a planted table that has a hole; the join is shown by the assertion that no longer exempts the
  failpoint.

### D5 `cover:check` prints the output of its test run

`scripts/cover-check.sh:22` stops sending `go test` output to `/dev/null`. This is the only gate that discards test
output (inventory 4).

- Check: a new contract test copies the script into a throwaway root, puts a fake `go` on `PATH` that prints a
  `--- FAIL` line and exits 1, runs the no-argument mode, and requires the line in the output. No test covers that
  mode today (`cover_test.go:49`).
- Shown able to fail: the test is written first and fails on the current script, which prints nothing.

### D6 Sleeps, skips and build tags fail a contract test, with no accepted exceptions

Three checks in `internal/harness/contract`, in the shape of `TestNoFixedAddressesInTests`, each with a sensitivity
test and each failing when it scanned no test file:

- no `time.Sleep` in any `*_test.go` file or in any Go file under `internal/harness/`;
- no `Skip`, `Skipf` or `SkipNow` call in any `*_test.go` file;
- no build constraint in a `*_test.go` file other than `//go:build integration`.

There is no baseline file, no allowlist and no inline marker. Premises: the tree has 0 sleeps, 1 skip
(`runner_test.go:264`) and 1 build tag (`fixture_integration_test.go:1`), all measured for this design with
`git grep` and read again at `0f30b12`. The one skip is replaced: `deadPID` tries again with a fresh child a bounded
number of times and then fails with the reason, so a pid that was reused is reported instead of hidden.

These are Go tests and not new scripts because the existing owner of "refuse a text shape in test files, proved by
a planted violation" is the contract package; a new script would need a Taskfile step and a `verify.sh` entry for
nothing. They match text. A renamed `time` import, a wait built from `time.After` or a timer, a skip called
through a helper, and a `_linux_test.go` file name are outside them and stay review only. They do not catch #40's
shape, which has no text; D1 and D2 carry that.

Ported SemStreams tests arrive later, and SemStreams' own guard accepted 254 existing sleeps (inventory 6.5). With no
exception list, a ported file that holds one cannot land until it is repaired, which is what a `repair-before-port`
row in the admission ledger already says in prose (`docs/setup-plan.md:201-202`). The owner ruled that the three
checks stand for ported tests as written (Q5): each slice repairs the test files it ports, and there is no list of
accepted files. One repair is already allowed by the checks: a test that is skipped only because it belongs with the
integration tests can be moved into a file tagged `integration`. Its skip call goes either way. How many files the
checks reach is not measured in this design (see "Effects on slice 04A").

### D7 The port guard stops pointing at a race and loses its exemption marker

`scripts/lint-test-ports.sh:52-53` tells the author to bind port 0 and read the port, and names a `freePort` helper
in a SemStreams file. SemStreams deleted that helper as a race in its #1120 (inventory 2.1). The message is
replaced by one that says to bind `127.0.0.1:0` and hand the listener itself to the code under test. The inline
marker at `:41`, which exempts a line with no recorded reason and has no uses, is removed with its fixture case.

- The two scripts stop being byte-identical to the pin. Their ledger rows (`docs/admission-ledger.yaml:149-160`
  and `:162-171`) change from `carry` to `adapt` and say what differs; `Taskfile.yml:52` and
  `docs/provenance.md:39-40` follow. The ledger's header has a rule for port refactors that have a tracking issue
  (`:20-23`). These two rows are a repair of a carried script and have none; the developer writes them against the
  header as it stands.
- Bind-then-close-then-bind stays a false negative of the guard and is added to its header list.
- Check: a contract test plants a fixed port carrying the old marker and requires exit 1, the line named, and the
  new guidance. Shown able to fail: it is written first and fails on the current script, which exits 0 for a marked
  line.

### D8 A known flake stops merges through a CI job that reads GitHub issues

The rule exists today as prose: "no known unfixed flake in a required job" (`.agents/protocol.md:50-51`). The
record it points to is a GitHub issue ("file it"). The design keeps that record and adds the reader it lacks.

A new script, `scripts/merge-check.sh`, runs as a new CI job `merge-check` that `required` needs beside `verify`.

**Which run it is.** The script takes this from two variables that the Actions runner sets, never from whether a
number was given. It believes `GITHUB_EVENT_NAME` only when `GITHUB_ACTIONS` is `true`. Outside Actions it does not
read the event at all, so naming a `push` event on a local command line changes nothing.

| `GITHUB_ACTIONS` | `GITHUB_EVENT_NAME` | What the script does |
|---|---|---|
| `true` | `pull_request` | a pull-request run: needs the pull request's number, which the workflow passes; with no number it exits non-zero |
| `true` | `push` | a push run: says the known-flake part does not apply and checks only D9; pushes run for `main` alone (`ci.yml:4-5`) |
| `true` | anything else, or not set | exits non-zero naming the event; a new trigger such as `merge_group` adds its rule here first |
| not `true`: someone runs `task merge:check -- <n>` | not read | a pull-request run: needs a number; with none it exits non-zero |

The first line the script prints names the kind of run and, for a pull-request run, the number it checks.

The second revision took the kind of run from `GITHUB_EVENT_NAME` alone. A local `GITHUB_EVENT_NAME=push task
merge:check -- <n>` then took the push path and exited 0 with a flake open, and that local run is what the review-only
rule "immediately before merging" depends on (the second review's finding B). The table above closes that with a
command. What is left is review only: someone who sets both variables by hand still gets the push path, and no command
can tell a variable the runner set from one typed into a shell. The script's first line shows which path was taken.
Inside Actions the same trick takes an edit to the `merge-check` job, and that edit is reviewed as a change to the
merge gate (below).

**What it does for pull request `n`.** Only the number comes from the event. Everything else is read from GitHub
when the script runs, so a re-run sees the state of that moment.

1. It fails if the label `class:flake` does not exist. Measured for the correction pass: with no such label,
   `gh issue list --label class:flake --state open` prints `[]` and exits 0, so a deleted label would otherwise
   read as "no known flake".
2. It lists the open issues carrying the label. It asks for up to 100 and fails if it gets 100, because the list
   may then be cut short.
3. It passes if there are none.
4. Otherwise it reads the pull request's closing references.
5. It passes if **every** open flake issue is among them. Issues are compared by URL, so an issue with the same
   number in another repository does not count. It prints one line per issue and raises a warning on the run (a
   `::warning::` line), so the exemption shows on the pull request's checks without opening the log.
6. Otherwise it fails, naming each open flake issue the pull request does not close.

A read that fails, or an answer that is not the JSON list the script asked for, ends the script with an error
naming the read. It is never taken as "no known flake"; that is the rule `scripts/openspec-queue.sh:95-99` applies
to its own reads. The job is granted `issues: read` and `pull-requests: read`, and nothing else beyond
`contents: read`.

**What the exemption is.** A closing line is also how work is claimed here: a draft pull request with `Closes #n`,
opened before the work (`.agents/protocol.md:16-17`). So the pull request that claims a flake is exempt from its first
run, before it holds a fix; a draft cannot merge (documented, not measured; inventory 7.6.1 and A15). The check cannot
tell a fix from a declaration, and no command can. When the pull request merges, GitHub closes the issues it names and
the stop lifts for everyone. That is the Close step working as written (`.agents/protocol.md:54-55`), and it is also
the way a closing line with no real fix behind it would lift the stop. The warning in step 5 makes every use visible.
Two rules cover the rest, and both are review only:

- A pull request that closes a `class:flake` issue shows the reproduction: the command, how often it failed before
  the fix and how often after (the shape of task 6.3). The reviewer confirms it before the Land step.
- A flake that comes back after its fix reopens the same issue, which puts the stop back.

**Two flakes open at once.** A pull request that fixes one of them is red, and the job names the other. The two fixes
land in one pull request, and the head that merges has run with both in place. The first revision exempted a pull
request that closed "at least one". Under that rule a fix for one flake merged on a green that the other flake could
have produced by chance, which is the re-roll this rule exists to stop, and retrying such a run was stopped by
nothing. The owner ruled "every" (Q4), with its cost: two open flakes are fixed in one pull request, and a flake that
cannot be fixed yet holds back the fix of the others.

**When the check runs, and what it does not see.**

- **Editing the description starts no run.** `ci.yml:7` names no event types, so GitHub's defaults apply: a run
  starts when a pull request is opened, reopened or pushed to (documented, not measured; A11). A pull request whose
  description gains the closing line stays red until its next push, or until its failed run is re-run.
- **A result is a snapshot** of three things: the head, the issue list and the pull request's description. Two changes
  after a green leave that green standing. A flake filed afterwards: with D9, and given A1, at most one already-green,
  up-to-date pull request can merge per filing, because its merge puts every other pull request behind `main`. A
  closing line removed afterwards: that pull request merges without closing the flake, the issue stays open, and every
  pull request after it is stopped. Running `task merge:check -- <n>` immediately before merging, in a shell where
  `GITHUB_ACTIONS` is not set, covers both; that rule is review only. O4 would close both.
- **Pull requests on another base.** `ci.yml:6-7` runs CI whatever the base. GitHub fills closing references only
  when the base is the default branch (documented, not measured; A10). So while a flake is open every stacked pull
  request is red, and a flake fix cannot be delivered as a stacked pull request. The check is deliberately not
  limited to pull requests whose base is `main`. Changing a pull request's base is an edit, which starts no run, so
  a green earned on another base without the check would still stand after the pull request was pointed at
  `main`. All four open pull requests have `main` as their base (measured for the correction pass).
- **It acts only on a filed, labelled flake.** #40 was filed 24 minutes and three green heads after its first red
  (inventory 3.4). Before filing, a re-roll by push or by re-run is stopped by nothing. The rule "a red you cannot
  explain by your diff is filed before the next push" stays review only.
- **What is filed under the label is sorted by a person.** Ruled (Q1a): a `class:flake` is only a failure that a pull
  request in this repository can end, which means a test or check that passes and fails on the same tree. A failed
  network fetch is not one. Its log names the remote and no test, and with D5 every failing test is named. It is
  recorded as a comment on the pull request it hit, with the run's link, and the run is re-run when the remote
  answers. The second failure of the same fetch gets an ordinary issue against that fetch, without the label. No
  command reads the class of a failure, so the sorting is review only: a test flake misread as a fetch failure is
  stopped by nothing, and a failed fetch never stops a merge however often it comes.
- **One login.** Owner and agents share `cglusky` (inventory 4), so no command can tell who closed or unlabelled an
  issue. Three acts lift the stop with no fix behind them and no warning on any run: closing the issue; taking the
  label off an open issue; and renaming the label and creating a new, empty `class:flake`, after which step 1 passes
  and step 2 finds nothing. Closing a flake issue without a merged fix already takes the owner's word on the issue
  (`.agents/protocol.md:56-57`). The same rule now covers the label: a flake issue is closed, or its label removed or
  renamed, only by a merged fix or on the owner's word. That stays review only. GitHub's issue timeline records a
  close and a label taken off; whether anything records a renamed label was not measured.
- **A pull request runs its own copy** of the script, the workflow and their tests, so it can change them in its
  own diff. That holds for every gate in `task verify` too. A change to `scripts/merge-check.sh`, to the
  `merge-check` or `required` job, or to their tests is reviewed as a change to the merge gate; that rule is review
  only. The same property is how a fix lands if GitHub changes the shape of an answer the script reads.
- **The result depends on GitHub's state as well as the tree.** `merge:check` is therefore not part of
  `task verify`, which stays a function of the tree and runs locally while a flake is being fixed.

Why a reader of issues and not a record in the tree. The mechanisms considered:

| Mechanism | Where the record lives | What it would take | Why it is not chosen |
|---|---|---|---|
| M0 Prose only | the issue | nothing | PR #39 was green and mergeable while #40 was open |
| M1 An OpenSpec hold read by `spec:queue --strict` | `tasks.md` of an active change | an active change on `main`; a new marker class; the script's test, which was never ported | a hold stops its own change from landing; it is never on `main`, where another pull request could see it (`git ls-tree --name-only origin/main openspec/changes/`: `.gitkeep` and `archive`). `--strict` fails on any hold, including this change's own |
| M2 A `repair-before-port` row in the admission ledger | `docs/admission-ledger.yaml` | a row with no SemStreams source | the schema is provenance: every row needs a SemStreams `source_path` and `source_sha` (`ledger_test.go:102-110`), and nothing acts on a disposition (inventory 7.6.1). It stays the record for flakes inherited with ported code |
| M3 A new file of known flakes read by `task verify` | a tracked file | a new record, a step, and a rule for the pull request that adds an entry | a second home for a fact whose home is an issue; the entry reaches other pull requests only after its own pull request passes CI on the flaky tree; that pull request is red by its own entry unless the check compares with the base |
| M4 An issue label read by a CI job (chosen) | the issue | one label, one script, one job, two read permissions | see the limits above |
| M5 A second required check that nothing reports | the ruleset | a settings change per flake | it also blocks the fix; lifting it lets everything through; it leaves no reviewed record |
| M6 M4 inside a merge queue | the issue | O4 | not now (see O4) |

Inventory 7.3 calls an OpenSpec hold "the closest existing durable record" of "this may not proceed until X". A
`repair-before-port` ledger row is the same kind of record, as the re-check noted. Both were weighed (M1, M2).
Neither can carry a flake in this repository's own tests without becoming a different thing, and the issue already
is the record the rule names.

Premises. Measured for this design: no `class:flake` label exists (`gh label list`); nothing in `.github`,
`scripts` or `Taskfile.yml` reads an issue or a pull request (`git grep -n -E 'issues:|pull-requests:|gh
(issue|pr|api)'`: 0 hits); the issue list answers an unauthenticated read with HTTP 200, so the data is public.
Measured for the correction pass: `gh pr view --json closingIssuesReferences` returns `[42]` for PR #44, `[37]`
for PR #39 and `[]` for PR #47, whose description says "Addresses #9"; each entry carries `number`, `url` and
`repository`.

Check: a contract test runs the script from a throwaway root with a fake `gh` on `PATH`. Shown able to fail, by
its cases:

- one open flake and a pull request that closes none (exit 1, the issue named);
- the same with a closing pull request (exit 0, the exemption and the warning line printed);
- two open flakes and a pull request that closes one (exit 1, the other named);
- two open flakes and a pull request that closes both (exit 0);
- a closing reference to the same number in another repository (exit 1);
- no open flake (exit 0); a missing label (non-zero); a failing `gh` (non-zero, "unavailable"); a list of 100
  (non-zero);
- inside Actions: a `pull_request` event with no number, an unknown event, and no event (each non-zero);
- outside Actions: no number (non-zero);
- a `push` event inside Actions (the known-flake part not applied);
- `GITHUB_EVENT_NAME=push` outside Actions, with a number and an open flake the pull request does not close (exit 1,
  the issue named: the event was not believed).

A second pinned-file test reads `.github/workflows/ci.yml` and requires four things: `required` needs both jobs;
the `merge-check` job's permissions are exactly the three reads; no job is granted a write; and the `verify` job's
limit is 15 minutes (see "Not now"). Today nothing tests the workflow's permissions or its limit
(`git grep -n 'ci.yml' -- internal`: one hit, `docker_test.go:42`). The red path and the exemption are each
exercised once against real GitHub before the change lands, with a drill issue that is opened and closed for the
purpose (task 7.6).

### D9 The up-to-date rule is a ruleset setting that every run reads back

- **Made.** One API call sets `strict_required_status_checks_policy` to `true` on ruleset 24272345. The call is a
  `PUT` whose body is the ruleset as just read with that one value changed, because GitHub documents that the rules
  list is replaced whole (documented, not measured). The ruleset is read back and compared with the read before it;
  any other difference is put back and reported. The session makes the call with `gh api` at task 7.3, before this
  change merges, and posts the ruleset as read before and after on #42 (ruled, Q3).
- **Checked.** On every run, pull request and push, `scripts/merge-check.sh` asks GitHub which rules are in force
  on `main` and fails unless they require `Required` with the strict setting on. Turning the setting off in
  GitHub then turns every run red, and the script's expectation changes only by a reviewed change to the script
  and to the spec requirement that states it.

The first revision recorded the whole ruleset in a file and compared the live `enforcement`, `conditions` and
`rules` with it exactly. The live answer carries fields GitHub fills in itself (`do_not_enforce_on_create`,
`integration_id`, `required_reviewers`, `require_extra_approval_for_unattributed_changes`,
`allowed_merge_methods`, `dismissal_restriction`). One more such field would have turned every pull-request run
and every push to `main` red with no change in the tree. The script now reads only the fields that carry the
decision:

| Read | Field | Why it carries the decision |
|---|---|---|
| the rules in force on `main` (`rules/branches/main`) | `type` is `required_status_checks` | finds the rule. GitHub answers "which rules apply to `main`", so the script does not work that out from `conditions.ref_name` |
| the same answer | `parameters.required_status_checks[].context` includes `Required` | which check must pass |
| the same answer | `parameters.strict_required_status_checks_policy` is `true` | the up-to-date rule itself |
| the same answer | `ruleset_id` | says which ruleset to read next; it is not compared with a number in the tree |
| that ruleset (`rulesets/<id>`) | `enforcement` is `active` | a ruleset set to `evaluate` or `disabled` enforces nothing |

The last read may be redundant. GitHub documents that the first answer lists only the rules of active rulesets, which
the second review pointed out (finding D). The read stays. That behaviour is documented and not measured, and
measuring it would mean switching the live ruleset off. With the read in place the design does not rest on it (A16),
at the price of one read per run.

Everything else in the answers is ignored: the other three rules (`deletion`, `non_fast_forward`,
`pull_request`), every other parameter, `integration_id`, names, ids and dates. They are settings this change does
not decide. With nothing else compared, the record file of the first revision held no value the script did not
already demand, so it is dropped: the expected values are the two in the table, stated in the spec. The whole
ruleset as read before and after the edit is posted on #42 (task 7.3); that comment is what a restore would use.

The review proposed comparing `enforcement`, `conditions.ref_name`, the check names and the strict setting with a
record. This design covers the same four facts with one difference: it asks GitHub which rules apply to `main`
instead of comparing `conditions.ref_name` with a stored copy, so a ruleset that is replaced, split in two or
given a new id is still read correctly, and one fewer field's shape is assumed.

Premises. Measured for this design: the ruleset reads `strict_required_status_checks_policy: false` and
`bypass_actors: []`; PR #39's head was one commit behind `main` and GitHub reported it `CLEAN`. Measured for the
correction pass: `gh api repos/C360Studio/semengine/rules/branches/main` returns the four rules of ruleset
24272345, each with its `ruleset_id`, to a signed-in and to an unauthenticated read (HTTP 200); the unauthenticated
read of the ruleset returns `enforcement` and not `bypass_actors`; the compare API reports PR #44 and PR #47 level
with `main`, PR #39 two commits behind and PR #14 four.

Cost with several open pull requests: every merge puts every other open pull request behind `main`. Each must take
`main` in and pass a full run before it can merge, about three minutes with D2. A draft pays that once, when it is
ready to land. When two are ready together, the one that loses the race updates and runs again. The cost grows
with the number of pull requests landing in the same hour, not with the number open. `allow_update_branch` stays
off; an agent updates in its own worktree. O4 removes this cost and is the next step if it is felt.

Check: the `merge-check` contract test includes these cases: the strict setting off (exit 1, the field printed);
no rule on `main` that requires `Required` (exit 1); the ruleset not `active` (exit 1); and an answer with an
extra field the script does not name (exit 0). The setting's effect is observed when it is made (task 7.3).

### D10 The protocol's merge rule names the commands

`.agents/protocol.md`, `AGENTS.md` and the preflight skill restate the merge gate in three places (inventory 3.3:
`.agents/protocol.md:50-51`, `AGENTS.md:70`, `semengine-preflight/SKILL.md:82`). Their text changes to say what is
ruled:

- a known flake is an open `class:flake` issue, and the label is for a failure that a pull request in this repository
  can end: a test or check that passes and fails on the same tree (Q1a);
- a failed network fetch is not a `class:flake`. It is a comment on the pull request it hit, with the run's link; the
  run is re-run when the remote answers; the second failure of the same fetch gets an ordinary issue, without the
  label (Q1a);
- `Required` fails while a known flake is open unless the pull request closes every open one (Q4), and there is no
  other way past: no waiver (Q1b);
- a pull request that closes a `class:flake` issue shows the reproduction before and after the fix, and a flake that
  comes back reopens its issue;
- a `class:flake` issue is closed, or its label removed or renamed, only by a merged fix or on the owner's word;
- a red you cannot explain is filed before the next push;
- the head must be up to date;
- run `task merge:check -- <n>` immediately before merging, in a shell where `GITHUB_ACTIONS` is not set.

The waiver sentence leaves the Land step (Q1b). Today the step says of a known flake: "fix it, or file it and obtain
an explicit owner waiver recorded as a PR comment" (`.agents/protocol.md:51`). Everything after "fix it" goes. The
preflight skill's "Apply the protocol's fix-or-recorded-waiver rule" (`semengine-preflight/SKILL.md:83`) changes with
it. Two other uses of the word stay, because neither offers a way past a flake: the Close step lists "a waiver" among
the approvals that never widen into a close (`.agents/protocol.md:57`), and the pickup skill tells a session to check
"known failures or waivers" (`semengine-pickup/SKILL.md:21-22`).

The command list at `AGENTS.md:43`, which is four steps behind `scripts/verify.sh:10-11` (inventory 1), is brought up
to date, and it and the preflight table gain the new step.

## Effects on slice 04A (PR #47)

PR #47 is the sister session's claim on slice 04A, the first ported code. It holds no code yet. Its description sizes
the port set at 65 packages and 140,842 lines at the pin `8b99efe9`; the archived SETUP 03B design (D4) gives that
figure as non-test lines. This design changes none of PR #47's files, but its decisions reach it.

**The bill for slice 04A is not measured in this design.** The bill is three things: how many ported test files the
checks of D6 refuse, how long the repeat step takes over the ported packages, and whether any ported test fails it.
The second revision gave numbers for all three. They came from a copy of the SemStreams pin snapshot, taken from the
sister session's scratch directory, and from running that copy's tests in scratch. `docs/inventory-scope.md` allows
SemStreams "code facts only from the `8b99efe9` snapshot with path:line", cited with `gh api … ?ref=<sha>` (`:24`,
`:37-38`), and says nothing about running SemStreams tests. The measurement was therefore taken outside the reading
scope that document states. The owner struck it on 2026-10-01 (#42, comment 5937807011); the owner's words are "strike
them". Every number and fact that came from the copy is removed from this change. Sizing the repair is left to slice
04A's own inventory, inside that scope.

What reaches PR #47 follows from this repository's own tree and settings:

1. **The strict setting, from the moment it is made (task 7.3), which is before this change merges.** No pull request
   then merges unless its head contains the tip of `main`. PR #47 was level with `main` when the correction pass
   measured it. Every later merge, this change's included, puts it behind, and it pays one update and one full run
   each time it is ready to land.
2. **The text checks of D6, in the slice that ports each file.** They scan every `*_test.go` file in the tree. A
   ported test file that holds a sleep, a skip call or a build tag other than `integration` cannot land as it is. The
   setup plan says "unchanged extraction retains earned tests and adds only missing boundary evidence"
   (`docs/setup-plan.md:210-211`), and D6 changes such a test before it lands. The owner ruled that ported tests land
   repaired (Q5).
3. **The unit runs of D1 and D2, as ported packages arrive.** `./...` takes in every ported package, so each of its
   tests without a build tag runs seven times per `task verify`, five of them in the repeat step. A ported test that
   fails `test:repeat` is a failure in the pull request that ports it and is repaired there.
4. **One budget for every step.** The 15 minutes of `ci.yml:22` are one budget for the whole `verify` job. Inside it
   `task verify` runs the unit run under the race detector, the Docker-backed run, which starts two packages at a time
   under its own 10-minute timeout (`scripts/test-integration.sh:402`), and the repeat step. This design makes no
   projection of how those steps grow with ported code. What happens when the job outgrows its limit is under "Not
   now".
5. **The known-flake check, after this change merges.** PR #47 gets the `merge-check` job when it next takes `main`
   in, which item 1 forces. From then on any open `class:flake` issue anywhere in the repository makes it red. A flaky
   test that a slice brings in is, before it merges, that pull request's own failure. After it merges it is a known
   flake that stops every pull request, the next slice's included.

## Not now, and what would reopen it

- **A scheduled run of the suite, and what happens when the job outgrows its limit.** With D1 and D2 every head
  runs every untagged test seven times, which is also what finds a flake already on `main`. The direction says a
  schedule is revisited when the suite outgrows the pull-request budget. This design gives that moment a failing
  check and a decider:
  - The hard stop is the `verify` job's limit of 15 minutes (`ci.yml:22`). A run that is killed there fails
    `Required`.
  - Neither the repeat count (D3) nor that limit (D8's workflow test) can change without failing a contract test
    and changing the spec requirement that states it. A spec change is accepted by the owner, so the decision is
    the owner's, on the change that proposes it.
  - The early signal is review only: every run prints its step timings, and the first pull request whose `Verify`
    job runs longer than 10 minutes in CI says so in its description and takes it to the owner before it merges.
  - The options at that point, none chosen now: a CI job of its own for `test:repeat`, also needed by `Required`
    (the same repetitions in parallel, for more runner minutes); repeating only the packages a diff changes and
    the ones that import them (the mapping the direction left out when the module had five packages); a lower
    count; a scheduled run.

  Whether slice 04A brings that moment is not measured in this design (see "Effects on slice 04A").
- **Issues filed by automation.** Unchanged from the direction.
- **A guard on re-runs.** No run has been re-run (maximum `run_attempt` 1 across all 52 runs, measured for the
  correction pass). Refusing a second attempt would not close re-rolls, because a push does the same job; it would
  turn a retry after a failed network fetch into an empty commit. Left out, with that maximum as the number to watch.
  Two kinds of re-run are expected from here on, and each leaves a record: task 7.6 makes one deliberate re-run and
  records its run id, and a re-run after a failed fetch has a comment on its pull request that names the fetch (Q1a).
  The number to watch is then "no attempt above 1 on a run with neither record".
- **A run when a pull request's description is edited.** Adding `edited` to the workflow's trigger would close
  the two snapshot limits of D8 that concern the description and the base. Cost: a full run for every edit of a
  description, which cancels the run in progress. Measured for the correction pass: the descriptions of PR #21,
  PR #44, PR #43, PR #39 and PR #13 were edited 11, 6, 4, 4 and 2 times. Left out; reopens if a pull request ever
  merges past a flake by one of those two routes.
- **A merge queue.** O4.

## Where the evidence supports the direction and where it does not

| Direction | Supported by | Not supported, or thinner than it reads |
|---|---|---|
| 1 Vary the scheduling, repeat the whole suite | 100 of 100 at one CPU without `-race` against 0 of 300 under `-race` at the default (inventory 5.3; re-measured here inside a bubble). SemStreams #1059: 0 of 100 at the default, 100 of 100 at one CPU (inventory 6.1) | two flakes are the whole sample for one CPU. Eleven SemStreams flakes did not reproduce when re-run alone at default settings (inventory 6.3); of the three that then did reproduce, one needed concurrent load and one a forced delay, which this step does not apply. "With `-race` at one CPU" is the weak half: 6 of 100. "The whole suite" is read here as the 70 untagged tests (ruled, Q2). The premise "the module has five packages" ends with slice 04A |
| 2 No schedule | SemStreams found no flake by a scheduled run and its owner ruling reserves schedules for security scanning (inventory 6.3) | nothing here measures how a schedule would behave; the design does not rest on it |
| 3a Up to date with `main` | PR #14 and PR #39 hold greens older than `main` (inventory 3.3) | the setting's behaviour is documented, not measured, until task 7.3 |
| 3b `Required` fails while a flake issue is open | PR #39 during #40 (inventory 3.4); six SemStreams merges past a flake (inventory 6.3) | it acts only after filing; 14 of 23 SemStreams flakes were filed after the same rule existed in prose (inventory 6.4). One already-green pull request per filing can still merge. A fix that is declared and not real passes |
| 3 Rerun guard replaced | no run has ever been re-run (inventory 3.4) | that is a fact about the past. A re-run stays an open way to re-roll before a flake is filed |
| 4 Guards with no accepted exceptions | 254 accepted sleeps and a flake among them (inventory 6.5); 0 sleeps here | the sleep check cannot match #40's shape or the line that failed in #1063, which was an elapsed-time assertion. Whether sleeps were involved in SemStreams' other flakes was not measured (inventory 6.5). The port set's bill is not measured: the direction was accepted, and Q5 ruled, without it |
| 5 No quarantine as a check | a skip is invisible in today's logs (inventory 4); SemStreams closed no flake by a skip | whether `runner_test.go:264` ever fired is not established |

## Rules and the checks that fail

Every rule either names the check that fails when it is broken, or is marked review only.

| Rule | Failing check | How the check is shown able to fail |
|---|---|---|
| Unit run: `-race`, one CPU | `contract` pin test (D3) | planted Taskfile without `-cpu 1` |
| Repeat run: five times, one CPU, no `-race`, shuffled, last in `verify` | `contract` pin test (D3) | planted Taskfile with the count lowered or `-race` added; planted `verify.sh` without the step or with it not last |
| A scheduling-dependent test fails before merge | `task test:repeat` | one-time evidence on the old #40 code (task 6.3); in general this is a probability, not a guarantee |
| A failing gate names the test | `contract` test of `cover-check.sh` with a fake `go` (D5) | written first; fails on the current script |
| No `time.Sleep` in tests or harness code | `contract` test (D6) | planted test file and planted harness file, line named |
| No skip call in tests | `contract` test (D6) | planted file, line named |
| No build tag on a test other than `integration` | `contract` test (D6) | planted file named |
| A ported test file meets the same three checks before it lands; there is no list of accepted files (ruled, Q5) | the three `contract` tests of D6, which scan every `*_test.go` file | the same planted files |
| No fixed port, no exemption marker, a message that names nothing outside this repository | `scripts/lint-test-ports.sh` with its fixture test and a `contract` test (D7) | planted marked line; fixture case flipped to `match` |
| Every failpoint is in the matrix; `finalize` joins every worker | `lifecycletest` tests (D4) | planted table with a hole; assertion without the exemption |
| No merge while a known flake is open, unless the pull request closes every open one (ruled, Q4). There is no waiver (ruled, Q1b): the job reads no comment and no label on the pull request | CI job `merge-check`, needed by `Required` (D8) | fake-`gh` cases; one real red and one real exemption before landing (task 7.6) |
| The kind of run comes from the event, and the event is believed only inside Actions; a pull-request run with no number fails; an unknown event fails | `scripts/merge-check.sh` (D8) | fake-`gh` cases, one of them a `push` event named outside Actions |
| The label and every read must be there; a list that may be cut short fails | `scripts/merge-check.sh` (D8) | fake-`gh` cases: missing label, failing read, 100 issues |
| `Required` needs `verify` and `merge-check`; the job holds three read permissions; no job can write; `verify` is limited to 15 minutes | `contract` pin test of `ci.yml` (D8) | planted workflows: job dropped, a write permission, a fourth permission, another limit |
| A head behind `main` cannot merge | GitHub's ruleset (D9) | observed when the setting is made (task 7.3) |
| The rules in force on `main` require `Required` with the strict setting, from an active ruleset | CI job `merge-check` (D9) | fake-`gh` cases: setting off, no such rule, ruleset not active, extra field |
| A pull request that closes a `class:flake` issue shows the reproduction before and after the fix | review only | no command can tell a fix from a declaration; the job's warning makes each use visible |
| A flake that comes back after its fix reopens its issue | review only | no command can tell a recurrence from a new failure |
| A red you cannot explain is filed before the next push | review only | no command can tell an unexplained red from a real one |
| Only a failure that a pull request here can end is filed under `class:flake`. A failed fetch is a comment on its pull request, a re-run when the remote answers, and an ordinary issue the second time (ruled, Q1a) | review only | the class of a failure is read from its log by a person |
| `task merge:check -- <n>` is run immediately before merging, in a shell where `GITHUB_ACTIONS` is not set | review only | covers a flake filed, or a closing line removed, after the last run; the first line of the output names the kind of run, so a push run made by hand shows |
| A flake issue is closed, or its `class:flake` label removed or renamed, only by a merged fix or on the owner's word | review only | one login; each of the three lifts the stop with no warning on any run |
| A change to `scripts/merge-check.sh`, to the `merge-check` or `required` job, or to their tests is reviewed as a change to the merge gate | review only | a pull request runs its own copy |
| The ruleset has no bypass actors | review only | the field is absent from the reads the job makes; it is read back with the owner's login in task 7.3 |
| The first pull request whose `Verify` job passes 10 minutes takes it to the owner | review only | the failing check behind it is the job's 15-minute limit |
| Unsynchronised observation, wall-clock budgets, `t.Parallel` over shared state, sleeps and skips by another spelling | review only | no text to match; D1 and D2 exercise the first three |

## Invariants and their spec homes

`scripts/merge-check.sh` is the one new surface with a decision table. "Inside Actions" means `GITHUB_ACTIONS` is
`true`. A pull-request run is a `pull_request` event inside Actions, or any run outside Actions. For every combination
of inputs:

- On a pull-request run with a number it exits 0 exactly when all of these hold: the rules in force on `main` require
  `Required` with the strict setting from an active ruleset; the label exists; the list of open `class:flake` issues
  is complete; and every issue in that list is among the pull request's closing references. The last holds trivially
  when the list is empty. Spec: `merge-gate`, "Known-flake check" and "Up-to-date rule".
- On a `push` event inside Actions it exits 0 exactly when the rules in force on `main` require `Required` with the
  strict setting from an active ruleset. Spec: `merge-gate`, "Known-flake check", scenario "Push run"; "Up-to-date
  rule".
- Inside Actions on any other event, or on none, and anywhere when a number is needed and missing, it never exits 0.
  Spec: `merge-gate`, "Known-flake check", scenarios "Pull-request run without a number", "Local run without a
  number", "Event with no rule" and "No event inside Actions".
- Outside Actions the value of `GITHUB_EVENT_NAME` never changes its result. Spec: `merge-gate`, "Known-flake check",
  scenario "Push event named outside Actions".
- A failed read, an answer of the wrong shape, or a list that may be cut short never produces exit 0. Spec:
  `merge-gate`, "Known-flake check", scenarios "A read fails" and "List may be cut short".
- A field the script does not name never changes its result. Spec: `merge-gate`, "Up-to-date rule", scenario
  "Field the script does not read".

A property or fuzz test of the script takes these six statements as its source, not the script.

## What a test author has to know after this change

Inventory 9 lists thirteen facts a test author must hold today, ten of them found in a document or nowhere.

- Found out from a failing command that names the line: a sleep, a skip, a hidden build tag, a marked fixed port
  (new); unbounded cleanup and fixed addresses (as today).
- Found out from a failing command that names the test: a result that depends on the scheduler, for the shapes the
  one-CPU runs reach; a unit test that fails in the coverage run.
- Found out from GitHub: the pull request is behind `main`.
- Found out from a failing job that names the issue: a known flake is open, and which ones this pull request does
  not close.
- Still a document: file a red you cannot explain; tell a failed fetch from a flake; show the reproduction when
  closing a flake; wall-clock budgets; `t.Parallel` over shared state; `lifecycletest.Run`'s 2 s grace for an
  adopter's real owner.

Nothing new asks the author to predict a value. The known-flake check reads the state when the job runs, the
up-to-date rule is GitHub's own answer at merge time, and the script asks GitHub which rules apply to `main`
instead of working it out.

## Assumptions that are documented and not yet measured

The owner's words: "start with best practices, lessons learned and solid policy and measure as we go while not
allowing debt to be ignored". Each row stays open until the place named in the last column has produced a result;
a row is closed by recording the result on #42, never by dropping it. Every place named is before this change
merges, except where the row says it stays open.

The first group is every place where the design relies on the shape or behaviour of a GitHub answer.

| Behaviour | Does the design rest on it | What would show it wrong | Where it gets measured |
|---|---|---|---|
| A1 With the strict setting on, GitHub refuses to merge a head behind `main` | yes (D9) | a pull request behind `main` reports a merge state other than `BEHIND` | task 7.3, on PR #14 and PR #39 at the moment the setting is made |
| A2 In CI, `gh` and `jq` are present, and a job with `issues: read` and `pull-requests: read` can search labels, list labelled issues and read closing references | yes (D8) | the job fails with "command not found" or "Resource not accessible by integration" | the first CI run with the job (task 7.5); task 7.6 for the failing path |
| A3 The job's token can read the rules in force on `main` and the ruleset's `enforcement` | yes (D9) | a read fails, or a field of D9's table is absent | task 7.5 prints the five fields the job sees. Both reads answer an unauthenticated request today |
| A4 Closing references reflect the pull request's description at the moment they are read | yes (the fix must be able to land) | after `Closes #<drill>` is added, the re-run still fails | task 7.6 |
| A5 A re-run keeps the run id, raises `run_attempt`, replaces the check result and reads live state again (one of the owner's three) | only for "or until its failed run is re-run" in D8; a re-run is otherwise left unguarded | the re-run in task 7.6 behaves otherwise | task 7.6, one deliberate re-run |
| A10 Closing references are filled only when the base is the default branch; a link made by hand in the side panel fills them too | yes, for the stacked-pull-request limit. The hand-made link is one more way to make the same declaration, and gets the same warning | a pull request on another base with `Closes #n` shows a closing reference | not planned: no stacked pull request exists. Stays open |
| A11 Editing a pull request's description or base starts no run | yes (D8, two limits) | a run starts when the description is edited | task 7.6, for the description: the run list before and after the edit. The base is not measured and stays open |
| A12 A `::warning::` line shows on the run and on the pull request's checks | only for how visible the exemption is; the log line stands without it | no warning appears on the drill's exempt run | task 7.6 |
| A13 `gh` exits non-zero when a read fails | yes ("a failed read never passes") | a run during a GitHub incident passes with empty lists | modelled by the fake `gh`; the script also refuses an answer that is not the list it asked for. Not exercised against a real outage. Stays open |
| A14 The ruleset `PUT` replaces the rules list whole and accepts the body as it was read | only for how the edit of D9 is made | the read-back differs from the read before it in more than the one value | task 7.3; any other difference is put back and reported |
| A15 GitHub refuses to merge a pull request while it is a draft | yes: it is what bounds the exemption a claim gets on its first run (D8) | a draft is merged | not planned: the only test is an attempted merge. Measured for the ruling pass: PR #44, a draft with a green `Required`, reports `isDraft: true` and `mergeStateStatus: CLEAN`, so the merge state alone does not show the refusal. Stays open |
| A17 Inside Actions, `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` names the event | yes (D8, "Which run it is") | the log of task 7.5 names another kind of run; or a push run on `main` asks for a number, which is a red run and not a silent pass | task 7.5, for the pull-request run. A push run happens only on `main`, so the fake `gh` cases model it and it is first seen for real on this change's merge commit. That half stays open |

The second group is the rest.

| Behaviour | Does the design rest on it | What would show it wrong | Where it gets measured |
|---|---|---|---|
| A6 Whether re-runs can be forbidden (one of the owner's three) | no | not applicable | not planned; reopens with "A guard on re-runs" above |
| A7 Scheduled runs, their ref, and a pending run in a busy concurrency group on `main` (one of the owner's three) | no; there is no schedule | not applicable | reopens with "A scheduled run of the suite" above |
| A8 Every hosted run gets 4 CPUs | only for the time estimates; `-cpu 1` fixes the setting itself | the repeat step's time in `verify`'s step timings is far from 5 x `runner` | every CI log already prints step timings |
| A9 Dependabot brings its own pull requests up to date, and stops doing so once anyone else pushes to its branch | no | PR #14 stays behind with no update | observed on PR #14 after task 7.3. If it stays behind, the update is asked for with a `@dependabot rebase` comment and not by a push, so that Dependabot keeps maintaining the branch |
| A16 The rules in force on `main`, as GitHub lists them, come only from active rulesets | no: the script reads `enforcement` itself (D9) | not applicable | not planned: measuring it means switching the live ruleset off |

## Declared costs

- About 90 s more per `task verify`, locally and in CI, of which `runner`'s tests are all but a few seconds. The
  `Verify` job goes from 86 s (run 36883858915) to about three minutes.
- For slice 04A: ported test files are repaired before they land (ruled, Q5), and every ported test without a build
  tag runs seven times per `task verify`. Neither cost is measured in this design (see "Effects on slice 04A").
- One more CI run per pull request for each merge that lands ahead of it (D9). This starts when the setting is made at
  task 7.3, before this change merges (ruled, Q3).
- While a flake is open, every pull request is red except one that closes every open flake issue. This is the intended
  stop. With two flakes open, neither fix can land alone, and a flake that cannot be fixed yet holds back the fix of
  the others (ruled, Q4).
- There is no waiver (ruled, Q1b). A flake nobody can diagnose stops every merge until it is fixed, its failing
  assertion is removed on the owner's ruling, or the owner closes the issue unfixed.
- A failed fetch never stops a merge, however often it comes, and telling one from a test flake is review only (ruled,
  Q1a).
- Five GitHub reads per pull-request run and two per push, inside a job `Required` needs. A GitHub failure fails
  the job and says so.
- The Docker-backed tests still run once per verification (ruled, Q2).
- `-failfast` still hides a second failure behind the first in the Docker-backed step
  (`integration-test-runner/spec.md:76`); unchanged.
- The repeat step may surface a failure in an existing test, most likely a wall-clock watchdog in `runner`
  (inventory 2.3). One run of the exact command passed. A failure it surfaces is fixed in this change.

## Left out, and why

- **Network fetches in the required job** (inventory 2.5): out of scope. None of the seven CI failures to date was a
  failed fetch (inventory 3.4), and removing the fetches is infrastructure work (caches, mirrors, pre-pulled images)
  with no evidence yet of which one matters. How a red from a failed fetch is recorded is ruled (Q1a): it is not a
  known flake, and D8 and D10 say what is done with it. `merge-check` adds GitHub reads to the same class and reports
  a failed read as such.
- **Repeating the Docker-backed tests:** ruled out (Q2). They stay at the one execution that
  `integration-test-runner/spec.md:76` fixes.
- **Runtime skip counting** (`go test -json`): the static check holds the count at zero, and counting at run time
  would change every invocation's output, including the argv a spec fixes.
- **A count of tests** to notice a deleted one: a deletion is a visible line in a reviewed diff.
- **The other six sites in inventory 2.2:** four are reads of asynchronous state at one instant, which can miss a
  defect; the inventory records none that can fail on good code. Two are goroutines in non-test code left unjoined
  by design (`natsfixture/deps.go:86`, `lifecycletest.go:293`). They are a matter of how much the tests detect,
  not of flakes, and belong with the testing page in PR #39.
- **Wall-clock budgets** (inventory 2.3): no text separates a budget that is a contract from a predicted one.
- **The unported fixture test for `scripts/openspec-queue.sh` and the two files with no ledger row** (inventory
  7.6.1): real, and not part of how a flake is born, merged or survives.
- **A record of the whole ruleset in the tree:** dropped in this revision (D9).

## New surfaces and who uses them

No exported Go symbol is added and no non-test Go code changes, so the context and lifecycle rules have nothing new
to cover; `TestNoRetainedContext` keeps covering the harness. Each new surface has a user on the day it lands:

- `task test:repeat`: `scripts/verify.sh`, and so CI and every local `task verify`.
- `scripts/merge-check.sh` and `task merge:check`: the `merge-check` CI job, and the agent about to merge.
- The label `class:flake`: `scripts/merge-check.sh`.
- The new contract tests: `task test:unit` and the two steps that run the same packages again.

## Adjacent claims

- PR #39 (issue #37) edits `AGENTS.md`, the role contracts and adds `docs/testing.md`. This change edits the merge
  rule and command list in `AGENTS.md`, the Land and Close steps in `.agents/protocol.md`, and the preflight
  skill. Whichever lands second takes the other's text in; with D9 that update is forced. If PR #39's rules table
  is on `main` by then, the review-only rows above are added to it.
- PR #39 and PR #14 are behind `main` (two and four commits); PR #44 and PR #47 are level. Task 7.3 makes that
  visible: PR #14 and PR #39 are the two it reads.
- PR #47 claims slice 04A. Every effect on it is in "Effects on slice 04A".
- The owner's global instructions describe the merge gate with a waiver for all repositories. This design changes only
  this repository's files. After the ruling on Q1b the two differ: a waiver given under those instructions passes
  nothing here, because no command reads it.

## Where this design goes beyond or departs from the direction

Items 1, 2, 4, 10 and 15 were put to the owner and are ruled (see "The owner's ruling"). The rest stand as the
second review passed them.

1. There is no way past an open flake other than a fix, although the owner's words allowed waivers to stay. Put as
   Q1b; ruled: none.
2. What is filed as a known flake at all. The direction said only that no candidate covers network fetches. Put as
   Q1a; ruled: only a failure that a pull request in this repository can end.
3. "With `-race` at one CPU" is done by changing the existing unit run, not by a repeated run; only the run without
   `-race` is repeated (D1, D2).
4. The Docker-backed tests are not repeated. Put as Q2; ruled: they stay at one execution.
5. `-shuffle=on` is added (D2).
6. A contract test pins the two command lines and the step order (D3). The direction said the repetition lives in a
   task; it did not ask for a pin.
7. The single failpoint table and the join in `finalize` (D4) come from the review of PR #43, routed to #42
   (inventory 5.4). They are not among the five recommendations; the direction named only making the matrix test
   instant.
8. A build-tag check is added beside the skip check, and the existing skip becomes a failure (D6).
9. The port guard's exemption marker is removed; the direction named only its pointer (D7).
10. A pull request that closes every open flake issue is exempt from the known-flake check; without that the fix could
    not land (D8). Put as Q4; ruled: every open one.
11. The known-flake check does not reach a red that nobody files, a fix that is declared and not real, or what
    changes after a run (D8). The direction's wording does not carry those limits.
12. The up-to-date rule is read back from GitHub on every run (D9). The direction asked for the rule, not for a
    check that it stays on.
13. The workflow test pins the job's permissions and the `verify` job's 15-minute limit as well as what `Required`
    needs (D8, "Not now"). The limit is pinned so that raising it is a decision and not an edit.
14. The command list in `AGENTS.md` is brought up to date (D10). The inventory found it four steps behind; the
    direction did not mention it.
15. The repair of ported tests was put to the owner again (Q5), although the direction already accepted guards with no
    exceptions. Ruled: they land repaired. The size of that repair is not measured in this design.

## Corrections after design review

The review is comment 5936056866 on PR #44. Each finding, what was done, and where. "Disputed" means the design
keeps a position the review questioned and gives its evidence.

| Finding | Answer | Where |
|---|---|---|
| HIGH 1: the exemption lets a pull request merge past a flake it does not fix; a closing line is an unremarked way through | Fixed. Exempt only when every open flake is among the closing references, compared by URL. Each use raises a warning on the run. The reproduction rule and the reopen rule are added as review only. Any-versus-all goes to the owner | D8 "What the exemption is", "Two flakes open at once"; rules table; spec "Known-flake check"; Q4 |
| HIGH 2: exact comparison of the ruleset makes `Required` depend on the shape of GitHub's answer | Fixed, with a different read from the one proposed. Five fields are read and nothing else; the record file is dropped. Every other reliance on a GitHub answer is listed as an assumption with what would show it wrong | D9 and its table; assumptions A1 to A5 and A10 to A14; spec "Up-to-date rule" |
| MEDIUM 3: A4 is measured only at the first real fix; a description edit starts no run | Fixed. The drill adds the closing line to PR #44 and records the exemption passing; D8 says the pull request stays red until pushed or re-run, and the drill measures both halves | D8 "When the check runs"; A4, A5, A11; task 7.6 |
| MEDIUM 4: the known-flake check is skipped whenever no number is given | Fixed. The kind of run comes from `GITHUB_EVENT_NAME`; a pull-request run or a local run without a number fails; an unknown event fails. The second review found this fix incomplete (finding B, below) | D8 "Which run it is"; spec scenarios; task 7.2 |
| MEDIUM 5: stacked pull requests get no exemption and are red while a flake is open | Said so, as the review's first alternative. Its second alternative, applying the check only when the base is `main`, is disputed: changing the base starts no run, so a green earned without the check would stand on `main` | D8 "Pull requests on another base"; A10; spec scenario "Pull request on another base" |
| MEDIUM 6: Q1 is not framed fairly | Fixed. The two existing exception shapes are weighed before any new one; the cost of none is given in full; what is filed as a flake is a question of its own | Q1a, Q1b |
| MEDIUM 7: the effect on PR #47 is understated | Fixed in the second revision with numbers measured at the pin and a projection. The owner struck those afterwards (below). What stands: the effects that follow from this repository's own tree and settings, and a failing check and a decider for the moment the job outgrows its limit | "Effects on slice 04A"; "Not now"; Q3; Q5 |
| MEDIUM 8: stale line pins | Fixed. Every pin was read again at `0f30b12`. Changed: `setup-plan.md:201-202`, the ledger rows `:149-160` and `:162-171`, `provenance.md:39-40`, and the counts of commits behind `main` | header; D6; D7; D9 |
| MEDIUM 9: the list of departures is incomplete | Fixed. D4's table and join, D10's command list and five more are added | "Where this design goes beyond or departs from the direction", items 2, 6, 7, 12 to 15 |
| MEDIUM 10: review-only rules not labelled so | Fixed, one each way. "A pull request runs its own copy" is labelled review only. The job's permissions are pinned by the workflow test | D8 limits and "Check"; rules table; spec "Required needs both jobs" |
| NIT: spec scenarios omit cases D3 claims | Fixed. Three scenarios added | spec `merge-gate` "Race detector added to the repeat step", "Repeat step not last"; spec `harness-boundaries` "Sleep added to harness code" |
| NIT: A9 and pushing to the Dependabot branch | Fixed | A9 |

Found during the correction pass, not in the first review:

- A closing line removed after a green leaves the green standing (D8, "A result is a snapshot"). Bounded: the flake
  stays open and stops everything after that one merge. Review only.
- A closing reference to an issue of the same number in another repository would have matched. Issues are now
  compared by URL (D8 step 5).
- The closing line is the protocol's claim, so a claim on a flake is exempt from its first run (D8).
- With no `class:flake` label, the issue list reads as empty with exit 0 (D8 step 1). This is the measurement
  behind the label check.
- A fifth item, about the port set at the pin, came from the measurement the owner struck and is removed with it
  (below).

### After the second review and the owner's rulings

The second review is comment 5936590347 on PR #44: DESIGN REVIEW PASS, with findings A to D left to apply. The owner's
two rulings are comments 5937751262 and 5937807011 on #42. This pass records the rulings and applies the findings. It
adds no decision.

| Item | What was done | Where |
|---|---|---|
| The owner's ruling on Q1 to Q5 | Recorded. "Questions for the owner" is now "The owner's ruling", with each option ruled and each option not taken. The decisions, the spec changes and the tasks say what is ruled, no longer what is recommended | "The owner's ruling"; D2, D6, D8, D9, D10; rules table; declared costs; spec `merge-gate` and `harness-boundaries`; tasks 2.3, 7.1, 7.3, 8.1 |
| The owner's ruling to strike the slice 04A measurement | Struck. Removed: the table of counts and timings taken over the copied snapshot; the test that failed in it; the projection for the hosted runner and its slowdown factor; the note on how the copy was identified as the pin; and every repeat of those in D2, D6, "Not now", the evidence table, the declared costs, the questions and `proposal.md`. Kept: the port set's size as PR #47's description and the archived SETUP 03B design give it, and the effects that follow from this repository's own tree and settings | "Effects on slice 04A"; the measurement marks in the header; "The owner's ruling"; `proposal.md`, "Impact" |
| A, MEDIUM: taking the label off an issue, or renaming the label, lifts the stop with no close and no warning | Fixed as the review proposed. The review-only rule now reads "closed, or its label removed or renamed, only by a merged fix or on the owner's word" | D8 "One login"; D10; rules table |
| B, MEDIUM: "no environment variable SHALL exempt a pull request" contradicts taking the kind of run from `GITHUB_EVENT_NAME`; a local run with the variable set to `push` passes with a flake open | Fixed with the review's first alternative. The event is believed only when `GITHUB_ACTIONS` is `true`, and outside Actions it is not read. The spec sentence names the two variables, and a scenario plants the case. Left as review only: a local run with both variables set by hand | D8 "Which run it is"; spec `merge-gate`, "Known-flake check", scenarios "Push event named outside Actions" and "No event inside Actions"; invariants; A17; rules table |
| C, MEDIUM: three points about the slice 04A projection and the measurement behind it | Overtaken by the strike. The projection and the measurement are removed, not relabelled | "Effects on slice 04A" |
| D, NIT: the separate `enforcement` read looks redundant | Kept, with the reason: the behaviour that would make it redundant is documented and not measured | D9; A16 |
| D, NIT: "a draft cannot merge" is not in the assumptions table | Added, with one read made for this pass | A15; D8 "What the exemption is" |
| Review note: Q5 (a) left out a repair the checks already allow | Added: a test skipped only because it belongs with the integration tests can be moved into a file tagged `integration` | D6; Q5 in "The owner's ruling"; spec `harness-boundaries`, scenario "Skipped test moved behind the integration tag" |
| Review note: task 7.5 should run after task 7.3 | The order is stated in the tasks | `tasks.md`, the notes on order and tasks 7.3 to 7.5 |

Found during the ruling pass, not in either review:

- Task 7.6 said the drill issue "quotes the ruling of task 2.3 as the word for opening and closing it". Neither ruling
  names the drill, and under the Close step the approval of a design never widens into a close
  (`.agents/protocol.md:56-58`). Task 7.6 now carries a hold for the owner's word on the drill. Giving that word is
  the owner's; this design does not assume it.
- Task 6.1 writes a test that fails until task 6.2 is done. The tasks now say the two are pushed together, and that a
  test "written first" is seen to fail locally and not in CI.
- The drill of task 7.6 needs two pushes after the drill issue is opened. The task now names them.
- Findings B and D bring two assumptions the table did not have: what the runner sets (A17) and what GitHub's list of
  rules contains (A16).
- The second revision's recommendation on Q5 ended "with these numbers handed to PR #47". The numbers are struck, so
  nothing is handed over and no task posts on PR #47.

## The owner's ruling

The second revision ended with five questions for the owner. Q1 has two parts, so six decisions were put, each with a
recommendation. The owner ruled on #42 on 2026-10-01. The ruling is recorded there as comment 5937751262, posted by
the session on the owner's word. The owner's words:

> continue with your recommendations

The comment writes out what was recommended for each decision. That is what is ruled, and nothing wider.

| Decision | Ruled | Where it lands |
|---|---|---|
| Q1a What is filed under `class:flake` | (b) Only a failure that a pull request in this repository can end | D8 "What is filed under the label"; D10; spec `merge-gate`, "Known-flake check"; tasks 7.1 and 8.1 |
| Q1b A way past an open flake without a fix | (a) None. The waiver sentence leaves the protocol's Land step | D10; spec `merge-gate`, "Known-flake check"; task 8.1 |
| Q2 Repeating the Docker-backed tests | (a) No. They stay at one execution, as a declared cost | D2; spec `merge-gate`, "Varied and repeated unit runs"; declared costs |
| Q3 The ruleset edit | (a) The session makes it with `gh api` at task 7.3 and posts the ruleset before and after on #42 | D9; task 7.3 |
| Q4 The exemption | (a) Every open flake must be among the pull request's closing references | D8; spec `merge-gate`, "Known-flake check"; task 7.2 |
| Q5 Ported tests | (a) They land repaired. There is no list of accepted files | D6; spec `harness-boundaries`; "Effects on slice 04A" |

Each decision follows with the cost that was stated to the owner and the options that were not taken. The costs of
those options are kept short, so that a later reader can see what was refused.

### Q1a What is filed under `class:flake`: ruled (b)

Only a failure that a pull request in this repository can end is a `class:flake`: a test or check that passes and
fails on the same tree. A failed network fetch is recorded as a comment on the pull request it hit, and the run is
re-run when the remote answers. The second failure of the same fetch gets an ordinary issue, without the label.

Cost stated to the owner: telling a fetch failure from a test flake is review only, and fetch failures never stop a
merge. One more cost, from the second revision: a re-run becomes legitimate in that one named case, and no command
tells it from a re-roll.

Not taken:

- (a) Every red on unchanged code is filed under the label, whatever its cause. Cost: a failed fetch stops every
  merge. For a host the job cannot avoid (the npm registry on a cold cache, the vulnerability database, GitHub's own
  API) no pull request can end it, and the stop waits each time for the owner to close the issue unfixed.
- (c) As (b), but the second failure of the same fetch gets the label. Cost: for a host the job cannot avoid it is the
  dead end of (a), one failure later.

### Q1b A way past an open flake without a fix: ruled (a), none

The only way past an open flake is a pull request that closes the open flakes (Q4). The waiver sentence leaves the
protocol's Land step (D10).

Why it had to be ruled: a waived merge can only land on a green (inventory 7.6.4), and with D8 the `merge-check` job
is part of that green. The protocol's sentence "obtain an explicit owner waiver recorded as a PR comment"
(`.agents/protocol.md:51`) would have let nothing through once D8 landed, because no command reads it. Either the
check read an exception, or the sentence went. No waiver has been issued in this repository, and the record of the
ruling on #40 (comment 5933434399) reads "no waiver".

Cost stated to the owner: a flake nobody can diagnose stops every merge until it is fixed, its failing assertion is
removed on the owner's ruling (as in SemStreams #1284), or the owner closes the issue unfixed. More costs, from the
second revision:

- Closing unfixed becomes the real exception, and it is the widest one there is: it covers every pull request, has no
  end and names no test that would prove the flake gone. SemStreams #750 was closed without a fix and came back as
  #1284.
- Under Q4, a flake that cannot be fixed yet also holds back the fix of every other flake.
- Adding an exception later would be done while merges are stopped. The pull request that adds it can pass only
  because a pull request runs its own copy of the check (D8).

Not taken:

- (b) The exception shape the setup plan already has: "an owner, bounded scope, proving test, and due milestone"
  (`docs/setup-plan.md:208-209`). The owner's comment on the flake issue would state the four, the issue would get a
  second label and the due milestone, and the check would leave that issue out and print the exception on every run.
  Cost: it exempts every pull request until the milestone, and nothing fails when a milestone passes. No command can
  tell the owner's label from an agent's. The plan offers the shape for "noncritical duplication, naming, or
  optimization" only; of the admission gates beside it, completed joins among them, it says "An issue, elapsed audit
  budget, or passing coverage number cannot waive them" (`:206-208`). A flake that is not yet diagnosed cannot be
  shown to be noncritical: #40 was a join that was never made.
- (c) An override that must carry a reason, in the shape of `scripts/test-integration.sh:58-70`. The owner's comment
  on the flake issue would name one pull request and a reason, and the check would exempt that pull request from that
  flake. Cost: the check reads and parses comment text, and with one login the valve is in practice in the author's
  hands. SemStreams used the same valve for at least seven merges (inventory 7.6.4).

### Q2 Repeating the Docker-backed tests: ruled (a), no

The design repeats the 70 untagged tests and leaves the 11 Docker-backed tests at one execution, as the spec fixes
their invocation. That is a declared cost. SemStreams' broker-timing flakes were not found by repetition: #1069 never
reproduced in 60 runs, and #1375 passed 50 of 50 (inventory 6.1).

Not taken:

- (b) Repeat them. Cost: about 33 s per extra execution today, a container start per test each time, and a change to a
  spec requirement, its test and a ledger row.

### Q3 The ruleset edit: ruled (a), the session makes it at task 7.3

The session makes the edit with `gh api` at task 7.3, before this change merges, and posts the ruleset as read before
and after on #42. The call replaces the rules list whole, so the session sends back what it read with one value
changed and compares the read-back (D9). The edit has to be in force before this change's own `merge-check` job can
pass.

Stated to the owner: from that moment no pull request merges unless its head contains the tip of `main`. When the
correction pass measured it, PR #39 was two commits behind and PR #14 four; PR #44 and PR #47 were level.

Not taken:

- (b) The owner makes the edit in GitHub's settings, at a time the owner picks, and the session posts the read-back.
  It would have avoided building the request by hand. The second revision gave it no cost of its own.

### Q4 The exemption: ruled (a), every open flake

A pull request is exempt from the known-flake check only when every open `class:flake` issue is among its closing
references. The head that merges has then run with every known fix in place.

Cost stated to the owner: two open flakes are fixed in one pull request, and a flake that cannot be fixed yet holds
back the others. Under the claim rules, one pull request for two flakes means one branch takes the other's commits and
one of the two claims is closed. Nothing else could merge in that time under either option, so what (a) adds is the
wait on the fix that is ready.

Not taken:

- (b) Any one open flake among the closing references. Fixes then land one at a time, as SemStreams' did (inventory
  6.3). Cost: the fix for one flake merges on a green that the other flake could have produced by chance, and retrying
  that run is a re-roll nothing stops. One closing line passes the job whichever flake is open.

Under both options a closing line with no real fix behind it passes. What stands against that is the warning on the
run and the two review-only rules of D8.

### Q5 Ported tests: ruled (a), they land repaired

Each slice repairs the test files it ports, as D6 and D2 require. There is no list of accepted files. The repairs:

- a sleep becomes a wait on a signal;
- a skip is removed, or its test is deleted on a ruling;
- a test whose skip only says that it belongs with the integration tests can be moved into a file tagged
  `integration`, which the build-tag check allows;
- a test file with any other build tag does not land as it is;
- a ported test that fails `test:repeat` is repaired in the slice, or its package gets a `repair-before-port` row.

The setup plan's "unchanged extraction retains earned tests" (`docs/setup-plan.md:210-211`) is read as "retained, and
repaired where a gate requires it". The work falls in slice 04A (PR #47). This change edits none of PR #47's files.

The cost of this ruling for slice 04A is not measured in this design. The question was put with numbers from the
measurement that the owner then struck (next heading). The ruling on Q5 stands without them (comment 5937807011).

Not taken:

- (b) A list of accepted files for ported code, which shrinks as they are repaired. Cost: this is the baseline the
  direction refused. SemStreams' list held 254 entries, and a test that later flaked was among them (inventory 6.5).
- (c) The three checks cover `internal/harness` and new files only. Cost: the largest body of test code is outside the
  rule.
- (d) A test that fails the checks is not ported, and its coverage is earned again later. Cost: it drops evidence the
  plan says to keep.

### The slice 04A measurement: struck

The second revision sized Q5 with counts and timings taken over a copy of the SemStreams pin snapshot. The copy came
from the sister session's scratch directory, and its tests were run in scratch. That is outside the reading scope
`docs/inventory-scope.md` states (`:24`, `:37-38`). The ruling comment on Q1 to Q5 recorded that the owner had not
answered that point. The owner then answered it on #42 (comment 5937807011, posted by the session on the owner's
word). The owner's words:

> strike them

What that changes here: every number and fact that came from the copy is removed from this file and from
`proposal.md`, the only two files of this change that held them. The ruling on Q5 stands. Its cost for slice 04A is
not measured in this change, and sizing it is left to slice 04A's own inventory (PR #47), inside the reading scope
that document states. "Corrections after design review" lists what was removed.

### Not put to the owner

- What happens when the `verify` job outgrows its 15 minutes. "Not now" says what fails, who decides and which options
  are open when it comes.
- The owner's word for the drill issue of task 7.6. The drill files an issue under `class:flake` that is no flake and
  closes it with no merged fix. Under the Close step, a close with no merged pull request behind it takes the owner's
  word on the issue itself, and the approval of a design never widens into a close (`.agents/protocol.md:56-58`).
  Neither ruling names the drill, so this design does not read that word into them, and task 7.6 carries a hold until
  it is recorded. Until this change merges only PR #44 runs the `merge-check` job, so the drill stops no other pull
  request.
