# Design: flake-defense

This is the architect's design for issue #42, written after `inventory.md` in this folder (third revision, base
`4d96860`, sha256 `a632315a30abec4cb56a32c86a7986d34e5d5ceb8352d640db8db6d389eba7b4`) received `INVENTORY PASS` on
PR #44 (comment 5935397241). "Inventory 3.3" and similar point into that file, which is the evidence base and is not
repeated here. Measurements marked "measured for this design" were taken on 2026-10-01 at `6c56846`, on a scratch
copy outside the repository, on a 12-CPU host shared with other sessions (load average 1.5 to 3.1).

This is a draft for independent design review and then the owner's ruling. Nothing in it is approved.

Line pins are at `6c56846`. While this was written `main` moved to `9286055` (PR #21, merged 2026-10-01T16:22Z).
That merge changed no script, workflow, Taskfile, Go file or spec. It moved lines in four files this design cites:
the two ledger rows of D7 are at `docs/admission-ledger.yaml:149-171` on `main`, the sentence in
`docs/provenance.md` at `:39-40`, the "repair before port" rule in `docs/setup-plan.md` at `:201-202`, and one row
of the preflight table changed. No premise below changes; the pins are re-read when the branch takes `main` in.

Words used below:

- **Flake:** a test or CI step that passes and fails on the same code.
- **Re-roll:** getting a green after a red without fixing the cause, by re-running the same commit or by pushing
  another one.
- **Known flake:** from this design on, an open GitHub issue labelled `class:flake`.
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

This design needs one thing that only the reading supplies. The owner's words allow waivers to stay "if they help".
The recommendation below keeps none. That goes further than the owner's words, so it is question Q1 and not a
decision.

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
| O3 O2 plus a known-flake check in CI (recommended) | a CI job that fails while a `class:flake` issue is open, unless the pull request closes one; the ruleset compared with a record in the tree | a red that nobody files; re-rolls before filing; one already-green pull request per filing (see D8) | a second CI job that reads GitHub; a repository-wide stop while a flake is open |
| O4 O3 with a merge queue in place of the up-to-date rule | checks run again at merge time on the merged result, so no pull request updates by hand and no already-green head slips past a new flake issue | nothing more than O3 | a `merge_group` trigger, a queue to operate, and more GitHub behaviour that nothing here has measured |
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
- Premise, cost: measured for this design, the exact command on a scratch copy passes in 90.8 s; `runner` is 90.3 s
  of that (5 x 18.1 s) and the other four packages finish in under 14 s. `task verify` takes 61 s of steps in CI
  today (run 36883858915).
- The count is one number in one place. Five is a cost choice, not a derived one: for a flake that fails a one-CPU
  execution with probability p, five executions catch it with probability 1 - (1 - p)^5, which is 97% at p = 0.5,
  67% at p = 0.2 and 23% at p = 0.05. The number rises when `runner`'s tests get cheaper; it is never lowered to buy
  time without the owner's ruling (see "Not now").
- Check: D3. Evidence the step works on the one flake this repository has had: task 6.3.

After D1 and D2, a test without a build tag runs seven times per `task verify` under three settings; a test in
`lifecycletest` or `probe` runs an eighth time under a fourth (`cover:check`, no `-race`, all CPUs).

### D3 The invocations are pinned by a contract test

A new test in `internal/harness/contract` parses `Taskfile.yml` and `scripts/verify.sh` and fails unless
`test:unit` and `test:repeat` hold exactly the command lines of the spec and `verify.sh` lists both steps. This is
the shape the integration argv already has (`runner_test.go:420` behind `integration-test-runner/spec.md:76`).

- Premise: nothing pins the unit invocation today. `git grep -n -E 'count=1|"-race"' -- '*_test.go'` finds one
  line, `runner_test.go:420`, which is the integration argv.
- Shown able to fail: its sensitivity test plants a Taskfile with the count lowered, one without `-cpu 1`, one with
  `-race` added to the repeat step, and a `verify.sh` without the step, and requires each to be named.
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
`git grep`. The one skip is replaced: `deadPID` tries again with a fresh child a bounded number of times and then
fails with the reason, so a pid that was reused is reported instead of hidden.

These are Go tests and not new scripts because the existing owner of "refuse a text shape in test files, proved by
a planted violation" is the contract package; a new script would need a Taskfile step and a `verify.sh` entry for
nothing. They match text. A renamed `time` import, a wait built from `time.After` or a timer, a skip called
through a helper, and a `_linux_test.go` file name are outside them and stay review only. They do not catch #40's
shape, which has no text; D1 and D2 carry that.

Ported SemStreams tests arrive later with their sleeps. With no exception list, such a file cannot land until it
is repaired, which is what a `repair-before-port` row in the admission ledger already says in prose
(`docs/setup-plan.md:180-181`).

### D7 The port guard stops pointing at a race and loses its exemption marker

`scripts/lint-test-ports.sh:52-53` tells the author to bind port 0 and read the port, and names a `freePort` helper
in a SemStreams file. SemStreams deleted that helper as a race in its #1120 (inventory 2.1). The message is
replaced by one that says to bind `127.0.0.1:0` and hand the listener itself to the code under test. The inline
marker at `:41`, which exempts a line with no recorded reason and has no uses, is removed with its fixture case.

- The two scripts stop being byte-identical to the pin. Their ledger rows (`docs/admission-ledger.yaml:144-166`)
  change from `carry` to `adapt` and say what differs; `Taskfile.yml:52` and `docs/provenance.md:37-38` follow.
  PR #21 added a rule to the ledger's header for port refactors that have a tracking issue. These two rows are a
  repair of a carried script and have none; the developer writes them against the header as it stands on `main`.
- Bind-then-close-then-bind stays a false negative of the guard and is added to its header list.
- Check: a contract test plants a fixed port carrying the old marker and requires exit 1, the line named, and the
  new guidance. Shown able to fail: it is written first and fails on the current script, which exits 0 for a marked
  line.

### D8 A known flake stops merges through a CI job that reads GitHub issues

The rule exists today as prose: "no known unfixed flake in a required job" (`.agents/protocol.md:50-51`). The
record it points to is a GitHub issue ("file it"). The design keeps that record and adds the reader it lacks.

A new script, `scripts/merge-check.sh`, runs as a new CI job `merge-check` that `required` needs beside `verify`.
On a pull-request run it:

1. fails if the label `class:flake` does not exist (a filter that can match nothing is a broken filter);
2. lists the open issues carrying it;
3. passes if there are none;
4. otherwise reads the pull request's closing references and passes, saying so, if it closes at least one of them;
5. otherwise fails, naming each open flake issue.

A read that fails ends the script with an error naming the read. It is never taken as "no known flake"; that is the
rule `scripts/openspec-queue.sh:95-99` applies to its own reads. On a push to `main` the known-flake part does not
run. The job is granted `issues: read` and `pull-requests: read`, and nothing else beyond `contents: read`.

Why a reader of issues and not a record in the tree. The mechanisms considered:

| Mechanism | Where the record lives | What it would take | Why it is not chosen |
|---|---|---|---|
| M0 Prose only | the issue | nothing | PR #39 was green and mergeable while #40 was open |
| M1 An OpenSpec hold read by `spec:queue --strict` | `tasks.md` of an active change | an active change on `main`; a new marker class; the script's test, which was never ported | a hold stops its own change from landing; it is never on `main`, where another pull request could see it (`git ls-tree --name-only origin/main openspec/changes/`: `.gitkeep` and `archive`). `--strict` fails on any hold, including this change's own four |
| M2 A `repair-before-port` row in the admission ledger | `docs/admission-ledger.yaml` | a row with no SemStreams source | the schema is provenance: every row needs a SemStreams `source_path` and `source_sha` (`ledger_test.go:102-110`), and nothing acts on a disposition (inventory 7.6.1). It stays the record for flakes inherited with ported code |
| M3 A new file of known flakes read by `task verify` | a tracked file | a new record, a step, and a rule for the pull request that adds an entry | a second home for a fact whose home is an issue; the entry reaches other pull requests only after its own pull request passes CI on the flaky tree; that pull request is red by its own entry unless the check compares with the base |
| M4 An issue label read by a CI job (chosen) | the issue | one label, one script, one job, two read permissions | see the limits below |
| M5 A second required check that nothing reports | the ruleset | a settings change per flake | it also blocks the fix; lifting it lets everything through; it leaves no reviewed record |
| M6 M4 inside a merge queue | the issue | O4 | not now (see O4) |

Inventory 7.3 calls an OpenSpec hold "the closest existing durable record" of "this may not proceed until X". A
`repair-before-port` ledger row is the same kind of record, as the re-check noted. Both were weighed (M1, M2).
Neither can carry a flake in this repository's own tests without becoming a different thing, and the issue already
is the record the rule names.

Premises, measured for this design: no `class:flake` label exists (`gh label list`); nothing in `.github`,
`scripts` or `Taskfile.yml` reads an issue or a pull request (`git grep -n -E 'issues:|pull-requests:|gh
(issue|pr|api)'`: 0 hits); the issue list and the ruleset answer an unauthenticated read with HTTP 200, so the data
is public; `gh pr view 44 --json closingIssuesReferences` returns `[42]` and PR #43 returns `[40]`.

Limits of the mechanism, stated plainly:

- **It acts only on a filed, labelled flake.** #40 was filed 24 minutes and three green heads after its first red
  (inventory 3.4). Before filing, a re-roll by push or by re-run is stopped by nothing. The rule "a red you cannot
  explain by your diff is filed before the next push" stays review only.
- **A result belongs to a head.** A pull request that was green and up to date when the flake was filed keeps its
  green. With D9, and given assumption A1, at most one such pull request can merge per filing: its merge moves
  `main`, every other pull request is then behind, and their next run meets the check. Running
  `task merge:check -- <n>` immediately before merging covers that one; that rule is review only.
- **One login.** Owner and agents share `cglusky` (inventory 4), so no command can tell who closed or unlabelled an
  issue. Closing a flake issue without a merged fix already takes the owner's word on the issue
  (`.agents/protocol.md:56-57`); that stays review only. GitHub's issue timeline records the event.
- **The result depends on GitHub's state as well as the tree.** `merge:check` is therefore not part of
  `task verify`, which stays a function of the tree and runs locally while a flake is being fixed.
- **Two flakes open at once.** Each fix's run can be hit by the other flake. Retrying that run is a re-roll past a
  known flake, which no command stops. The fixes can share one pull request.

Check: a contract test runs the script from a throwaway root with a fake `gh` on `PATH`. Shown able to fail: its
cases are an open flake and an unrelated pull request (exit 1, the issue named), the same with a closing pull
request (exit 0, the exemption printed), no open flake (exit 0), a missing label (non-zero), a failing `gh` (non-zero,
"unavailable"), and a push run (the known-flake part not applied). A second pinned-file test requires `required` to
need both jobs in `.github/workflows/ci.yml`. The red path is exercised once against real GitHub before the change
lands (task 7.6).

### D9 The up-to-date rule is a ruleset setting with a record in the tree

- **Made.** One API call sets `strict_required_status_checks_policy` to `true` on ruleset 24272345. The call is a
  `PUT` whose body carries all four existing rules, because GitHub documents that the rules list is replaced whole
  (documented, not measured; the ruleset is read back after the call). Who makes it is Q3.
- **Recorded.** `.github/rulesets/main-required-checks.json` holds the ruleset's `id`, `name`, `enforcement`,
  `conditions` and `rules`. The why is the owner's ruling on #42.
- **Kept from drifting.** `scripts/merge-check.sh` reads the live ruleset on every run, pull request and push, and
  fails when `enforcement`, `conditions` or `rules` differ from the record, printing both. It also fails if the
  record itself does not require `Required` with the strict setting on. Turning the setting off then takes a
  reviewed change to the record as well as a settings change, and either one alone turns every run red. This is
  the pin shape `task doctor` applies to tool versions: one file holds the value and a command compares the live
  value with it.

Premises, measured for this design: the ruleset reads `strict_required_status_checks_policy: false` and
`bypass_actors: []`; an unauthenticated read returns `enforcement`, `conditions` and `rules` but not
`bypass_actors`; PR #39's head was one commit behind `main` and GitHub reported it `CLEAN`. Since PR #21 merged,
the compare API reports PR #44 one commit behind, PR #39 two and PR #14 four.

Cost with several open pull requests: every merge puts every other open pull request behind `main`. Each must take
`main` in and pass a full run before it can merge, about three minutes with D2. A draft pays that once, when it is
ready to land. When two are ready together, the one that loses the race updates and runs again. The cost grows
with the number of pull requests landing in the same hour, not with the number open. `allow_update_branch` stays
off; an agent updates in its own worktree. O4 removes this cost and is the next step if it is felt.

Check: the `merge-check` contract test includes a live ruleset with the setting off (exit 1, the field printed) and
a record with it off (exit 1). The setting's effect is observed when it is made (task 7.3).

### D10 The protocol's merge rule names the commands

`.agents/protocol.md`, `AGENTS.md` and the preflight skill restate the merge gate in three places (inventory 3.3).
Their text changes to say: a known flake is an open `class:flake` issue; `Required` fails while one is open unless
the pull request closes one; a red you cannot explain is filed before the next push; the head must be up to date;
run `task merge:check -- <n>` immediately before merging. The waiver clause follows Q1. The command list at
`AGENTS.md:43`, which is four steps behind `scripts/verify.sh:10-11` (inventory 1), is brought up to date, and it
and the preflight table gain the new step.

## Not now, and what would reopen it

- **A scheduled run of the suite.** With D1 and D2 every head runs every untagged test seven times, which is also
  what finds a flake already on `main`. The question reopens the first time the repeat
  count would have to be lowered to keep the job inside its 15-minute limit; lowering it is the owner's ruling, not
  a tuning change.
- **Issues filed by automation.** Unchanged from the direction.
- **A guard on re-runs.** No run has been re-run (maximum `run_attempt` 1 across every run, measured for this
  design). Refusing a second attempt would not close re-rolls, because a push does the same job; it would turn a
  retry after a failed network fetch into an empty commit. Left out, with that maximum as the number to watch.
- **A merge queue.** O4.

## Where the evidence supports the direction and where it does not

| Direction | Supported by | Not supported, or thinner than it reads |
|---|---|---|
| 1 Vary the scheduling, repeat the whole suite | 100 of 100 at one CPU without `-race` against 0 of 300 under `-race` at the default (inventory 5.3; re-measured here inside a bubble). SemStreams #1059: 0 of 100 at the default, 100 of 100 at one CPU (inventory 6.1) | two flakes are the whole sample for one CPU. Eleven SemStreams flakes did not reproduce when re-run alone at default settings (inventory 6.3); of the three that then did reproduce, one needed concurrent load and one a forced delay, which this step does not apply. "With `-race` at one CPU" is the weak half: 6 of 100. "The whole suite" is read here as the 70 untagged tests (Q2) |
| 2 No schedule | SemStreams found no flake by a scheduled run and its owner ruling reserves schedules for security scanning (inventory 6.3) | nothing here measures how a schedule would behave; the design does not rest on it |
| 3a Up to date with `main` | PR #14 and PR #39 hold greens older than `main` (inventory 3.3) | the setting's behaviour is documented, not measured, until task 7.3 |
| 3b `Required` fails while a flake issue is open | PR #39 during #40 (inventory 3.4); six SemStreams merges past a flake (inventory 6.3) | it acts only after filing; 14 of 23 SemStreams flakes were filed after the same rule existed in prose (inventory 6.4). One already-green pull request per filing can still merge |
| 3 Rerun guard replaced | no run has ever been re-run (inventory 3.4) | that is a fact about the past. A re-run stays an open way to re-roll before a flake is filed |
| 4 Guards with no accepted exceptions | 254 accepted sleeps and a flake among them (inventory 6.5); 0 sleeps here | the sleep check cannot match #40's shape or the line that failed in #1063, which was an elapsed-time assertion. Whether sleeps were involved in SemStreams' other flakes was not measured (inventory 6.5) |
| 5 No quarantine as a check | a skip is invisible in today's logs (inventory 4); SemStreams closed no flake by a skip | whether `runner_test.go:264` ever fired is not established |

## Rules and the checks that fail

| Rule | Failing check | How the check is shown able to fail |
|---|---|---|
| Unit run: `-race`, one CPU | `contract` pin test (D3) | planted Taskfile without `-cpu 1` |
| Repeat run: five times, one CPU, no `-race`, shuffled, last in `verify` | `contract` pin test (D3) | planted Taskfile with the count lowered or `-race` added; planted `verify.sh` without the step |
| A scheduling-dependent test fails before merge | `task test:repeat` | one-time evidence on the old #40 code (task 6.3); in general this is a probability, not a guarantee |
| A failing gate names the test | `contract` test of `cover-check.sh` with a fake `go` (D5) | written first; fails on the current script |
| No `time.Sleep` in tests or harness code | `contract` test (D6) | planted file, line named |
| No skip call in tests | `contract` test (D6) | planted file, line named |
| No build tag on a test other than `integration` | `contract` test (D6) | planted file named |
| No fixed port, no exemption marker, a message that names nothing outside this repository | `scripts/lint-test-ports.sh` with its fixture test and a `contract` test (D7) | planted marked line; fixture case flipped to `match` |
| Every failpoint is in the matrix; `finalize` joins every worker | `lifecycletest` tests (D4) | planted table with a hole; assertion without the exemption |
| No merge while a known flake is open, unless the pull request closes one | CI job `merge-check`, needed by `Required` (D8) | fake-`gh` cases; one real red before landing (task 7.6) |
| The label and every read must be there | `scripts/merge-check.sh` (D8) | fake-`gh` cases: missing label, failing read |
| `Required` needs `verify` and `merge-check` | `contract` pin test of `ci.yml` (D8) | planted workflow without the job |
| A head behind `main` cannot merge | GitHub's ruleset (D9) | observed when the setting is made (task 7.3) |
| The ruleset matches its record | CI job `merge-check` (D9) | fake-`gh` cases: live setting off, record setting off |
| A red you cannot explain is filed before the next push | review only | no command can tell an unexplained red from a real one |
| `task merge:check -- <n>` is run immediately before merging | review only | covers the one already-green pull request |
| A flake issue is closed only by a merged fix or on the owner's word | review only | one login; the issue timeline is the record |
| The ruleset has no bypass actors | review only, unless task 7.5 shows the job can read them | the field is absent from an unauthenticated read |
| Unsynchronised observation, wall-clock budgets, `t.Parallel` over shared state, sleeps and skips by another spelling | review only | no text to match; D1 and D2 exercise the first three |

## Invariants and their spec homes

`scripts/merge-check.sh` is the one new surface with a decision table. For every combination of inputs:

- On a pull-request run it exits 0 exactly when the ruleset matches its record, the label exists, and either no
  `class:flake` issue is open or the pull request closes at least one. Spec: `merge-gate`, "Known-flake check" and
  "Ruleset record".
- On a push run it exits 0 exactly when the ruleset matches its record. Spec: `merge-gate`, "Known-flake check",
  scenario "Push run"; "Ruleset record".
- A failed read never produces exit 0. Spec: `merge-gate`, "Known-flake check", scenario "A read fails".

A property or fuzz test of the script takes these three statements as its source, not the script.

## What a test author has to know after this change

Inventory 9 lists thirteen facts a test author must hold today, ten of them found in a document or nowhere.

- Found out from a failing command that names the line: a sleep, a skip, a hidden build tag, a marked fixed port
  (new); unbounded cleanup and fixed addresses (as today).
- Found out from a failing command that names the test: a result that depends on the scheduler, for the shapes the
  one-CPU runs reach; a unit test that fails in the coverage run.
- Found out from GitHub: the pull request is behind `main`.
- Found out from a failing job that names the issue: a known flake is open.
- Still a document: file a red you cannot explain; wall-clock budgets; `t.Parallel` over shared state;
  `lifecycletest.Run`'s 2 s grace for an adopter's real owner.

Nothing new asks the author to predict a value. The known-flake check reads the state when the job runs, and the
up-to-date rule is GitHub's own answer at merge time.

## Assumptions that are documented and not yet measured

The owner's words: "start with best practices, lessons learned and solid policy and measure as we go while not
allowing debt to be ignored". Each row stays open until the place named in the last column has produced a result;
a row is closed by recording the result on #42, never by dropping it.

| Behaviour | Does the design rest on it | What would show it wrong | Where it gets measured |
|---|---|---|---|
| A1 With the strict setting on, GitHub refuses to merge a head behind `main` | yes (D9) | a pull request behind `main` reports a merge state other than `BEHIND` | task 7.3, on PR #14 and PR #39 at the moment the setting is made |
| A2 A job with `issues: read` and `pull-requests: read` can list labelled issues and read closing references through `gh` | yes (D8) | the job fails with "Resource not accessible by integration" | the first CI run of the implementing pull request (pass path); task 7.6 (fail path) |
| A3 The job's token can read the ruleset, and what it sees of `bypass_actors` | yes for `rules`; no for `bypass_actors` | the read fails, or the keys differ from the unauthenticated read | task 7.5 prints the keys the job sees |
| A4 Closing references reflect the pull request's body at run time | yes (the fix must be able to land) | a pull request with `Closes #n` for an open flake is refused | fake `gh` in tests; live at the first real flake's fix. Open until then |
| A5 Re-run semantics: a re-run keeps the run id, raises `run_attempt`, replaces the check result (one of the owner's three) | no. A re-run is left unguarded, and would re-evaluate `merge-check` against the issue state of that moment | a run with `run_attempt` above 1 behaving otherwise | the first re-run anyone makes; `gh api repos/C360Studio/semengine/actions/runs --jq '[.workflow_runs[].run_attempt] \| max'` reads 1 today |
| A6 Whether re-runs can be forbidden (one of the owner's three) | no | not applicable | not planned; reopens with "A guard on re-runs" above |
| A7 Scheduled runs, their ref, and a pending run in a busy concurrency group on `main` (one of the owner's three) | no; there is no schedule | not applicable | reopens with "A scheduled run of the suite" above |
| A8 Every hosted run gets 4 CPUs | only for the time estimate; `-cpu 1` fixes the setting itself | the repeat step's time in `verify`'s step timings is far from 5 x `runner` | every CI log already prints step timings |
| A9 Dependabot brings its own pull requests up to date | no | PR #14 stays behind with no update | observed on PR #14 after task 7.3 |

## Declared costs

- About 90 s more per `task verify`, locally and in CI, of which `runner`'s tests are all but a few seconds. CI
  goes from about 100 s to about 190 s a run.
- One more CI run per pull request for each merge that lands ahead of it (D9).
- While a flake is open, every pull request is red except one that closes a flake issue. This is the intended stop.
- Three GitHub API reads per run inside a job `Required` needs. A GitHub API failure fails the job and says so.
- The Docker-backed tests still run once per verification (Q2).
- `-failfast` still hides a second failure behind the first in the Docker-backed step
  (`integration-test-runner/spec.md:76`); unchanged.
- The repeat step may surface a failure in an existing test, most likely a wall-clock watchdog in `runner`
  (inventory 2.3). One run of the exact command passed. A failure it surfaces is fixed in this change.

## Left out, and why

- **Network fetches in the required job** (inventory 2.5): out of scope. None of the seven CI failures to date was
  a failed fetch (inventory 3.4), and removing the fetches is infrastructure work (caches, mirrors, pre-pulled
  images) with no evidence yet of which one matters. A red from a failed fetch is still a red nobody can explain
  by their diff, so it is filed as `class:flake` like any other; SemStreams #1358 was handled that way.
  `merge-check` adds API reads to the same class and reports a failed read as such.
- **Repeating the Docker-backed tests:** Q2.
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

## New surfaces and who uses them

No exported Go symbol is added and no non-test Go code changes, so the context and lifecycle rules have nothing new
to cover; `TestNoRetainedContext` keeps covering the harness. Each new surface has a user on the day it lands:

- `task test:repeat`: `scripts/verify.sh`, and so CI and every local `task verify`.
- `scripts/merge-check.sh` and `task merge:check`: the `merge-check` CI job, and the agent about to merge.
- The label `class:flake` and `.github/rulesets/main-required-checks.json`: `scripts/merge-check.sh`.
- The new contract tests: `task test:unit` and the two steps that run the same packages again.

## Adjacent claims

- PR #39 (issue #37) edits `AGENTS.md`, the role contracts and adds `docs/testing.md`. This change edits the merge
  rule and command list in `AGENTS.md`, the Land step in `.agents/protocol.md`, and the preflight skill. Whichever
  lands second takes the other's text in; with D9 that update is forced. If PR #39's rules table is on `main` by
  then, the review-only rows above are added to it.
- Every open pull request, this one included, is behind `main` since PR #21 merged. Task 7.3 makes that visible:
  PR #14 and PR #39 are the two it reads.
- PR #47 claims slice 04A, the first ported code. It is the first change that will meet D6's checks with ported
  tests: a ported test file with a sleep, a skip or another build tag cannot land until it is repaired.
- The owner's global instructions describe the merge gate with a waiver for all repositories. This design changes
  only this repository's files.

## Questions for the owner

**Q1. Does the design keep a waiver?** A waived merge can only land on a green (inventory 7.6.4), and with D8 the
`merge-check` job is part of that green, so a waiver that no command reads would no longer let anything through.

- (a) No waiver. The one way past an open flake is a pull request that closes a flake issue. The waiver clause
  leaves the protocol's Land step. Cost: a flake nobody can diagnose stops every merge until it is fixed, its test
  is removed by the owner's ruling (SemStreams #1284), or the owner closes the issue unfixed, which is the state
  SemStreams #750 left behind. With two flakes open, see D8's last limit.
- (b) A narrow waiver the check reads: one comment on the flake issue naming one pull request, which exempts that
  pull request from that flake and is printed in the job's output. Cost: more script and more cases; no command can
  tell the owner's comment from an agent's; SemStreams used the same valve for at least seven merges (inventory
  7.6.4).
- Recommendation: (a). No waiver has been issued here and the one ruling on a flake refused one. (b) can be added
  by a spec change the first time a real case needs it, with that case as its evidence.

**Q2. Does "repeat the whole suite" include the 11 Docker-backed tests?** The design repeats the 70 untagged tests
and leaves the Docker-backed invocation as the spec fixes it.

- (a) Leave it at one execution. SemStreams' broker-timing flakes were not found by repetition: #1069 never
  reproduced in 60 runs, #1375 passed 50 of 50 (inventory 6.1).
- (b) Repeat it. Cost: about 33 s per extra execution, a container start per test each time, and a change to a
  spec requirement, its test and a ledger row.
- Recommendation: (a), declared as a cost.

**Q3. Who makes the ruleset edit?** The direction accepted the up-to-date rule. The edit is a repository setting,
made outside any pull request.

- (a) The session makes it with `gh api` after the ruling, and posts the ruleset before and after on #42.
- (b) The owner makes it in GitHub's settings.
- Recommendation: (a), on the owner's word in the ruling. Either way the record file and the comparison in D9 land
  with the change.

## Where this design goes beyond or departs from the direction

1. It recommends no waiver at all (Q1); the owner's words allow one.
2. "With `-race` at one CPU" is done by changing the existing unit run, not by a repeated run; only the run without
   `-race` is repeated (D1, D2).
3. The Docker-backed tests are not repeated (Q2).
4. `-shuffle=on` is added (D2).
5. A build-tag check is added beside the skip check, and the existing skip becomes a failure (D6).
6. The port guard's exemption marker is removed; the direction named only its pointer (D7).
7. The ruleset gets a record file and a comparison on every run (D9).
8. A pull request that closes a flake issue is exempt from the known-flake check; without that the fix could not
   land (D8).
9. The known-flake check does not reach a red that nobody files, and one already-green pull request can still
   merge after each filing (D8). The direction's wording does not carry those limits.
