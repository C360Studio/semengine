# Design: runner-image-pin

Status: revision 2. `inventory.md` has `INVENTORY PASS` and this design `DESIGN REVIEW PASS` (PR #112, comment
6036743505); Codex's implementation review approved `7c6dae2` (comment 6037693225). After that review, this paragraph
and "Conformance to the rulings" were added, with no decision changed. The owner's rulings on #61 (comment
6036642422) leave no open owner question. Base `1e383fe`.

## Why a design file

The workflow change is three lines, but two things need writing down before code: the first ruling assumed a premise
that does not hold (P3: the contract test does not read `runs-on`), and the new check could assert several things,
which decides whether moving to Ubuntu 26 later is a deliberate spec change or a quiet edit.

## Premises

- **P1.** All three jobs run on `ubuntu-latest`: `.github/workflows/ci.yml:25`, `:79`, `:101`.
- **P2.** The pin changes nothing on the day it lands: main's run 37606537318 (`1e383fe`) reports
  `Image: ubuntu-24.04` for all three jobs.
- **P3.** `TestCIWorkflowPinned` does not hold `runs-on`. `ciJob` (`internal/harness/contract/mergegate_test.go:251`)
  has no such field, so the three-line workflow change passes it unchanged. Without a new check the pin is review only.
- **P4.** `ubuntu-24.04` names a release, not a build. In run 37606537318, `Verify` and `Merge check` ran on image
  version `20260927.320.1` and `Required`, five minutes later, on `20261004.327.1`.
- **P5.** There is one workflow file: `.github/workflows/` holds only `ci.yml`.
- **P6.** No open pull request changes `ci.yml`, `mergegate_test.go` or the `merge-gate` spec (`inventory.md` §3).
- **P7.** `gopkg.in/yaml.v3` v3.0.1 (the module's version, `go.mod:19`) decodes `runs-on` read as `any` to a
  string for `ubuntu-24.04` (quoted or not), `ubuntu-latest` and `${{ vars.RUNNER_IMAGE }}`; to `[]interface {}` for
  a list; to `map[string]interface {}` for a runner group; to `nil` when absent. Measured with a scratch program, not
  in the tree, so a type check plus one string comparison separates the label from every other spelling.

## Owner's rulings

From #61, comment 6036642422 (2026-10-07):

- **R1. The check holds the exact label.** Every job's `runs-on` is exactly `ubuntu-24.04`, in the `merge-gate`
  spec and in `TestCIWorkflowPinned`. Moving to Ubuntu 26 later changes the requirement, the test's constant and the
  three lines together. The same comment records P3 as a correction to the first ruling.
- **R2. The weekly build is accepted.** The pin fixes the Ubuntu release, not GitHub's build of the runner machine
  (P4), which supplies the Docker engine, git, `gh` and `jq`. In the owner's words: "use github w 24.04 and docker
  official image when we can and stop trying to fix upstream issues we do not control." The residual is a ruled,
  accepted cost: no job container and no follow-up issue for it. Containers SemEngine starts itself stay on official
  images pinned by digest, as the NATS fixture is (`internal/harness/natsfixture/admission.go:22`).

Alternatives R1 closes, one line each: no check (the pin stays review only); refusing only `-latest` labels (a move to
26 needs no spec change, and lists, groups, expressions and `ubuntu-24.04-arm` pass); a `ubuntu-NN.NN` pattern (the
same, with a pattern where one constant does); one spelling through a repository variable (the pin leaves the tree).

### Conformance to the rulings

Each ruling, where this branch carries it out and what shows it. Paths under `internal/harness/contract/` are given by
file name; `specs/merge-gate/spec.md` is this change's delta. No deviation from a ruling was found (Codex's review,
PR #112 comment 6037693225), so there is no DEVIATION row.

| Ruling | Carried out at | Shown by | State |
| --- | --- | --- | --- |
| R1: every job's `runs-on` is exactly `ubuntu-24.04`, in the spec and in `TestCIWorkflowPinned` | `.github/workflows/ci.yml:27`, `:81` and `:103` (`verify`, `merge-check`, `required`), with the comment at `:22`; `runnerLabel` at `mergegate_test.go:131`; the check in `ciWorkflowViolations` (`mergegate_test.go:305`) at `:329`–`:332`; the requirement "Pinned runner image" at `specs/merge-gate/spec.md:5` | `TestCIWorkflowPinned` (`mergegate_test.go:134`) and `TestCIWorkflowPinnedSensitivity` (`:142`) with its seven planted workflows (task 2.2); the check failing first on the unchanged workflow (task 2.1) and four wrong changes, each caught (task 2.4), on PR #112 comment 6037163135; CI run 37615895793 on `7c6dae2`, all three jobs on `ubuntu-24.04` (comment 6037693225) | Carried out |
| R2: GitHub's builds within 24.04 are an accepted cost; containers SemEngine starts stay on official images pinned by digest | No build pin and no `container:` key in `.github/workflows/ci.yml`; the NATS fixture's digest check `internal/harness/natsfixture/admission.go:22` and the pin `.nats-image:5`, both unchanged on this branch | Run 37614567578 (task 2.5): `Verify` and `Merge check` on build `20260927.320.1`, `Required` on `20261004.327.1`, recorded as the accepted variation on PR #112 comment 6037163135 | Carried out |

## Decisions

- **D1. An ADDED requirement, "Pinned runner image".** Modifying "Required needs both jobs" would restate that whole
  requirement for one sentence. A separate requirement held by the same test already exists: "A run when a pull
  request is marked ready" (`mergegate_test.go:115`).
- **D2. The check.** `ciJob` gains a `RunsOn` field (`yaml:"runs-on"`, read as `any`) and the test gains the
  constant `runnerLabel = "ubuntu-24.04"` beside `verifyTimeoutMinutes` (`:129`). In the existing loop over every job
  (`mergegate_test.go:298`, beside the `writeGrants` call at `:299`), a job passes only when its value is a string
  equal to `runnerLabel`. Anything else (missing, another string, a list, a mapping, an expression) is one violation
  that names the job, what it found or that it has none, the label required, and the requirement's title. No new
  function is needed. A one-item list `[ubuntu-24.04]`, which GitHub would accept, is refused too: it fails closed,
  and the message says to write one label. The comment at `:115`–`:122` goes from six facts to seven and names the
  new requirement.
- **D3. Planted workflows.** The clean fixture (`:140`–`:171`) gains `runs-on: ubuntu-24.04` on each of its three
  jobs. Each plant defeats one plausible weaker check, and one adds a fourth job, so a check of the three named jobs
  alone fails:

  | Plant | Job | Weaker check it defeats |
  | --- | --- | --- |
  | `ubuntu-latest` | a fourth job, `lint: {runs-on: ubuntu-latest, steps: [{run: echo}]}` | no check, or a check of the three named jobs only |
  | `runs-on` removed | `required` | a check that runs only when `runs-on` is present |
  | `ubuntu-26.04` | `verify` | a refusal of `-latest` labels only |
  | `ubuntu-24.04-arm` | `merge-check` | a prefix match, `strings.HasPrefix(s, "ubuntu-24.04")` |
  | `[self-hosted, ubuntu-24.04]` | `verify` | a "contains `ubuntu-24.04`" test of the printed value |
  | `{group: ci, labels: ubuntu-24.04}` | `merge-check` | a check that handles a list but not a mapping |
  | `${{ vars.RUNNER_IMAGE }}` | `required` | a check that skips expressions, as the step check sets them apart (`:507`) |

  Each case requires one violation naming the job and the value found (`requireViolation`,
  `internal/harness/contract/repo_test.go:104`).
- **D4. The workflow.** The three lines read `runs-on: ubuntu-24.04`. A two-line comment above `jobs:` says every job
  is pinned by the `merge-gate` spec and that a move is a change to that spec whose own CI run is the `task verify`
  run on the new image. That is where someone editing the file looks first.
- **D5. No `AGENTS.md` row.** `AGENTS.md:67`–`:69` gives a spec requirement a row only when it binds every change.
  This one binds one file; only an edit of `ci.yml` can break it. The other facts the same test holds (the 15-minute
  limit, no write permission, `merge-check`'s three reads) have no row either; all are indexed by the `merge-gate` spec.
- **D6. Documents.** No current document names the runner or its image (`inventory.md` §2). `.agents/protocol.md:144`,
  `.agents/skills/semengine-preflight/SKILL.md:49` and `docs/repository-map.md:40` and `:52` describe the jobs and
  stay true. The change edits: the `merge-gate` Purpose (one clause, in the archive commit, as `review-gate-check`
  did), one archive row in `docs/repository-map.md` (as `mutation-check` did at `:65`), and the `ci.yml` comment of
  D4. Left alone: `Taskfile.yml:4`–`:6` ("Tool pins live in one place each") and `.github/dependabot.yml:16`, because
  the runner label is not a tool pin; it is spelled on three lines of `ci.yml` and held by the test.

## Tests

Decision on generated checks (`docs/testing.md`, "Decide whether generated checks are needed"): examples are enough.
The input is one YAML value per job; its kinds are few and fixed (absent, the label, another string, an expression, a
list, a mapping), and the only interaction between jobs is "every job", which the fourth-job plant covers. There is
no law and no history.

Shown able to fail: written first, the new check run against the unchanged `ci.yml` fails naming all three jobs and
`ubuntu-latest`. Then, by hand (`task mutate:check` takes only non-test files, `docs/testing.md:142`), four wrong
changes to the check, each with the plant that must catch it: refuse only `-latest` labels (`ubuntu-26.04`); skip a
job with no `runs-on` (the removed `runs-on`); check only `verify`, `merge-check` and `required` (the fourth job);
accept any label that starts with `ubuntu-24.04` (`ubuntu-24.04-arm`). Baseline, wrong-change and restored runs are
recorded on PR #112.

## What this does not cover

- **Builds within 24.04.** Accepted by R2: two runs of one commit can still land on different builds of the same
  release.
- **The label's retirement.** When GitHub retires `ubuntu-24.04`, pinned jobs stop running, loudly; that forces the
  deliberate move. The date is not measured.
- **A second workflow file.** The requirement covers `ci.yml`, like the other workflow requirements (P5).
- **Ubuntu 26.** Whether `task verify` passes there is measured by the later move's own CI run.

## Overlap and order

PR #93 (draft, 23 files) shares no file and no capability with this change. It shares the Go package
`internal/harness/contract`; its new package-level names (`goImageLiteral`, `prometheusPath`, `promautoPath`,
`globalRegistrationRule`, `prometheusGlobals`, `globalRegistrationViolations`, `TestNoProcessGlobalRegistration` and
its sensitivity test) do not include `runnerLabel`, the only package-level name this change adds (`RunsOn` is a field
of `ciJob`). Neither waits for the other. This change is expected to merge first (deadline 2026-10-19, and it is far
smaller); whichever merges second merges `origin/main` and runs `task verify` again.
