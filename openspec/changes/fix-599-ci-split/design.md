# Design: fix-599-ci-split

## Context

Tree cites are at `2b803166`. Timings are from the `step timings:` block of CI runs on `ubuntu-24.04` (4 cores).

- `.github/workflows/ci.yml:25-65`: one job `verify` (`Verify`), `timeout-minutes: 15` (`:28`), setup steps
  `:31-62` (checkout with `fetch-depth: 0`, setup-go, setup-node, `npm ci`, Task, ateam, `task doctor`), then
  `task verify` (`:65`). `required` has `needs: [verify]` (`:71`) and fails on any result but `success` (`:76-84`).
- `scripts/verify.sh:14-15` is the one step list; `:20-21` and `:53-57` the tracked-files check; `:27-37` and `:51`
  the host line, printed at start and end. `toolcheck`'s `unitInvocationViolations` reads that list
  (`internal/harness/toolcheck/mergegate_test.go:185-236`): `ateam:doctor` first, `test:tools:changed` after
  `test:unit`, `test:repeat` last.
- `test:unit` writes no coverage profile (`Taskfile.yml:110`). `cover:check` with no arguments makes its own unit
  profile (`scripts/cover-check.sh:86-89`) and reads `coverage/integration.coverprofile` only if
  `coverage/integration.tree` matches the tree now (`:95-99`), which `task test:integration` writes after a pass
  (`Taskfile.yml:131-133`).
- `ciWorkflowViolations` (`mergegate_test.go:617-669`) pins `requiredNeeds = ["verify"]` (`:251`),
  `verifyTimeoutMinutes = 15` (`:262`) and `fetch-depth: 0` on `verify`'s checkout.
- The `main` ruleset requires the checks `Required` and `Merge check` only (`gh api repos/C360Studio/semengine/rulesets`).
- Harness-boundaries "ateam doctor gates verify" (`openspec/specs/harness-boundaries/spec.md:1069`): "`scripts/verify.sh`
  SHALL run `task ateam:doctor` first".

Measured step times (seconds):

| Run | test:unit | test:repeat | build+vet | test:integration | cover:check | Verify job |
| --- | --- | --- | --- | --- | --- | --- |
| 43090022023 | 71 | 38 | 1 | 420 | 8 | 10.0 min |
| 43101438546 | 121 | 50 | 3 | 450 | 12 | 11.9 min |
| 43103669895 | 162 | 59 | 41 | 464 | 21 | 14.0 min |
| 43103302538 (cancelled) | 189 | 67 | 48 | 468 | 25 | 15 min |

The cancelled run's other steps add 55 s (`docs:check` 16, `test:tools:changed` 15, `ledger:check` 8, `lint` 6,
`vuln` 6, `tidy:check` 3, `spec:check` 1). Setup takes about a minute.

## Goals / Non-Goals

Goals: two CI jobs in parallel, each with a limit about twice its normal time; `Required` needs both; local
`task verify` unchanged. Non-goals: making any step faster; changing which steps exist or their order; a new CI
mechanism (artifacts, reusable workflows, composite actions).

## Decisions

**D1. A job picks its steps by a group name passed to `scripts/verify.sh`.** `scripts/verify.sh [checks|integration]`:
no argument runs every step; `checks` skips `test:integration` and `cover:check`; `integration` runs `ateam:doctor`,
`test:integration` and `cover:check`. The script walks the one `steps=(...)` list in order and skips what the group
leaves out, so order and the pinned list stay single. `task verify` runs `scripts/verify.sh {{.CLI_ARGS}}`, and CI
runs `task verify -- checks` and `task verify -- integration`. An unknown group exits non-zero naming both.
Rejected: listing `task <step>` lines in `ci.yml` (a second step list, and each job loses the timings, host lines and
tracked-files check); two Taskfile tasks each listing its steps (a second list).
`integration` keeps `ateam:doctor` (0 s) so every run of `scripts/verify.sh` still starts with it and harness-boundaries
"ateam doctor gates verify" stays true unedited.

**D2. Two named jobs, setup steps written in each.** `checks` (`Checks`) and `integration` (`Integration`) carry the
same setup steps as `verify` today, then their one `task verify -- <group>` line; neither needs the other.
Rejected: one matrix job over the two groups. It writes setup once, but `required` would see one job, `verify`, whose
combined result over the two legs this change has not measured, and its limit would be an expression the contract
test cannot hold as a number. Rejected: a composite action or reusable workflow for the shared setup (new mechanism).
Cost: about 30 lines of setup appear twice in `ci.yml`; `TestCIWorkflowPinned` checks both jobs' runner and grants.

**D3. No profile passes between jobs.** The `integration` job runs `test:integration`, then `cover:check` in the same
checkout. `cover:check` makes its own unit profile (`scripts/cover-check.sh:86-89`), and the fingerprint it compares
(`:95-99`) was written by the `test:integration` before it on the same tree. So the job needs neither `test:unit` nor
an artifact. `test:unit` and `test:repeat` stay in `checks` only.

**D4. Host lines and the tracked-files check run in both jobs.** They are in `scripts/verify.sh` around the step loop
(`:20-21`, `:37`, `:51`, `:53-57`), so each job prints its host line at start and end and fails if its own steps
changed a tracked file. Nothing moves.

**D5. Limits: `checks` 15 minutes, `integration` 20 minutes.** `checks` runs 3 to 6 minutes of steps (table) plus a
minute of setup, so 4 to 7 minutes; 15 is about twice the busiest. `integration` runs 7.1 to 8.2 minutes of steps plus
setup, so 8 to 10 minutes; 20 is about twice the busiest. `required` keeps 5 minutes. The "Why five minutes" scenario
(unit hang ends inside the job) moves to `checks`' 15 minutes, still three times the five-minute bound.

**D6. Job ids `checks` and `integration`.** The ruleset names neither (Context), so renaming `verify` needs no ruleset
change. Keeping `verify` for the first job would edit fewer lines but would name a job that no longer runs all of
`task verify`.

## Adopter seam

No surface is reached from outside this repository: consumers do not run SemEngine's CI. For a developer here,
nothing changes by default: `task verify` with no argument is the full gate. To rerun one CI job locally,
`task verify -- checks` or `task verify -- integration`; a misspelt group fails at once naming the two.

## Proving tests

- `TestVerifyGroups` (new, `mergegate_test.go`): copies `scripts/verify.sh` into a temporary git repository with a
  stand-in `task` on `PATH` that logs its argument and exits 0; runs it with no argument, `checks`, `integration` and
  `bogus`; compares each log with step lists written as constants in the test; `bogus` exits non-zero naming both
  groups. Shown able to fail by planting a `verify.sh` whose `integration` group drops `cover:check`.
- `TestCIWorkflowPinned` and its sensitivity cases: `required` needs `checks` and `integration`; limits 15 and 20;
  `checks` checks out with `fetch-depth: 0`; each job's last step is exactly its `task verify -- <group>` line.
- `TestUnitInvocationsPinned` unchanged: the one list keeps its order rules.
- No generated checks: three fixed groups and one unknown name are the whole input space.

## Risks / Trade-offs

- The `integration` job no longer reuses the race-built packages `test:unit` compiled in the same job. The measured
  `test:integration` time varied little between warm and cold builds (420 s with `build` 1 s, 464 s with `build`
  18 s), and 20 minutes leaves the margin; this PR's own CI run records both jobs' times.
- Two jobs use two runners per push.

## Migration Plan

None. The ruleset is unchanged; the merge of this PR is the cut-over.

## Open Questions

- Owner: job ids `checks` / `integration` (D6) and limits 15 / 20 minutes (D5) are this design's reading of "about
  twice"; say if either should differ.
