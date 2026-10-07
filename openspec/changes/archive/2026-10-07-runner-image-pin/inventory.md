# Inventory: runner-image-pin

- base: `1e383fe937b441d6627bf3b130b86cf72c9ce7b6` (`origin/main`). The claim branch `claude/runner-pin` adds only
  the empty claim commit `a6e522f`, so every pin below holds on both.
- revision 2 (revision 1 was `5e3e0afc`), measured 2026-10-07; issue #61, claim PR #112.
  Pinned texts are trimmed of leading whitespace.
- repositories read: this one only; the question needs no sister repository.

Question: where is the CI runner image chosen, what holds it, what else claims the same files, and what does a pin
change and not change?

## 1. The claimed gap

Claim (#61): "the runner image is the one thing CI does not pin". Measured true.

| Pin | Text |
| --- | --- |
| `.github/workflows/ci.yml:25` | `runs-on: ubuntu-latest` (job `verify`) |
| `.github/workflows/ci.yml:79` | `runs-on: ubuntu-latest` (job `merge-check`) |
| `.github/workflows/ci.yml:101` | `runs-on: ubuntu-latest` (job `required`) |
| `.github/workflows/ci.yml:28` | `- uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1` (actions pinned by SHA; also `:30`, `:34`, `:46`, `:61`, `:86`) |

`ls .github .github/workflows` lists `dependabot.yml` and `workflows/ci.yml` only: one workflow, no reusable workflow,
no composite action. `git diff --stat 89c878e 1e383fe -- .github/workflows/ci.yml` is empty, so the ruling's line
numbers (25, 79, 101 on `89c878e`) hold at the base.

Claim (owner's ruling): "`TestCIWorkflowPinned` holds the workflow text". Measured: it holds six facts and not
`runs-on`. `ciJob` has no `runs-on` field, so the three-line workflow change alone passes it unchanged.

| Pin | Text |
| --- | --- |
| `internal/harness/contract/mergegate_test.go:116` | `// .github/workflows/ci.yml held to six facts. The job merge-check runs under exactly three read` |
| `internal/harness/contract/mergegate_test.go:251` | `type ciJob struct {` (fields `:252` to `:259`: needs, if, continue-on-error, permissions, timeout-minutes, env, defaults, steps) |
| `internal/harness/contract/mergegate_test.go:140` | ``const good = `on:`` (the sensitivity fixture, `:140` to `:171`; no job has `runs-on`) |

What `ubuntu-latest` resolves to, from main's run 37606537318 at `1e383fe` (each job's "Set up job" log): `Verify`
and `Merge check` ran on `Image: ubuntu-24.04`, `Version: 20260927.320.1`; `Required`, five minutes later in the same
run, on `Image: ubuntu-24.04`, `Version: 20261004.327.1`. The label `ubuntu-24.04` itself moves between image builds,
within one run.

## 2. Every current spelling of the fact (which image CI runs on)

Search: `git grep -n -i -E "ubuntu|runs-on|runner image|runner-image|ImageOS|ImageVersion|RUNNER_OS|runner\.os" --
. ':!openspec/changes/archive'` returns the three `ci.yml` lines of section 1 and nothing else.
`gopls workspace_symbol -matcher=fuzzy runsOn` and `... RunnerImage` return no declaration of either name. No
script, Taskfile entry, test, spec or current document names the image. History only:

| Pin | Text |
| --- | --- |
| `openspec/changes/archive/2026-09-30-setup-02-isolated-harness/tasks.md:57` | `` `ubuntu-latest`, the Linux default range, adjacent to SemStreams' 34xxx e2e bands) `` |
| `openspec/changes/archive/2026-10-01-flake-defense/inventory.md:305` | `` \| Runner \| `.github/workflows/ci.yml:21` \| `runs-on: ubuntu-latest`; image `ubuntu-24.04` version `20260927.320.1` in runs 36856716070 and 36883858915 \| `` |
| `openspec/changes/archive/2026-10-03-review-gate-check/inventory.md:357` | `` **Issues.** #66 is this work. #64 (closed by PR #65) holds the rulings. #61 (open): `ubuntu-latest` moves to Ubuntu `` (it adds that `gh` and `jq` under `merge-check` change with the image) |

What the image supplies and nothing pins: the Docker daemon under `test:integration` (`.github/workflows/ci.yml:52`:
`# verify includes test:integration, which needs the runner's Docker daemon and`), git under the pin fetch of
`task ledger:check`, and `gh` and `jq` under `scripts/merge-check.sh`. Go, Node and Task come from pinned actions
reading `go.mod`, `.nvmrc` and `.task-version`.

## 3. Adjacent claims on the territory

| Pin | Text |
| --- | --- |
| `openspec/specs/merge-gate/spec.md:4` | ``The merge gate is what must hold before a pull request can merge to `main`. This capability covers the parts of it`` (Purpose: flaky-test defences and the review check; no runner) |
| `openspec/specs/merge-gate/spec.md:202` | `### Requirement: Required needs both jobs` |
| `openspec/specs/merge-gate/spec.md:206` | ``the workflow SHALL be granted a write permission. The job `verify` SHALL have a limit of 15 minutes. The job`` |
| `openspec/specs/merge-gate/spec.md:416` | `### Requirement: A run when a pull request is marked ready` |
| `AGENTS.md:68` | `gets a row only when it binds every change in the repository, as the test-text and merge rules do; the rest are` |
| `.agents/protocol.md:125` | `request in this repository can end: a test or check that passes and fails on the same tree. A network fetch that` |
| `.github/dependabot.yml:3` | `- package-ecosystem: github-actions` (edits `uses:` lines of `ci.yml` weekly) |
| `docs/repository-map.md:58` | `` \| `openspec/specs/merge-gate/` \| Current truth for the merge gate's flake defenses, synced by the `flake-defense` archive \| `` |

ADRs: `docs/adr/102-entity-id-segment-semantics.md` and `docs/adr/104-unique-platform-authority.md` only
(`git ls-files | grep -i adr`); neither is about CI. Active OpenSpec changes on `main`: none (`openspec/changes/`
holds only `archive/`). `AGENTS.md` has no row for the other facts `TestCIWorkflowPinned` holds (the 15-minute limit,
no write permission, `merge-check`'s three reads): `grep -n "15 minutes\|write permission" AGENTS.md` is empty; the
test is named only as a holder of other rules (`AGENTS.md:93`, `:106`). Admission ledger: not touched.

Open pull requests, from `gh pr list --state open --json number,title,changedFiles,files` (2026-10-07):

| PR | Draft | Files | Overlap with `ci.yml`, `mergegate_test.go`, the `merge-gate` spec |
| --- | --- | --- | --- |
| #112 (this claim) | yes | 0 | is this change |
| #93 setup-04a-02-ingest-kernel | yes | 23 | none of the three files and no `merge-gate` delta. Same Go package `internal/harness/contract` (`boundaries_test.go`, `imagepin_test.go`, `promglobal_test.go`); its new package-level names (`prometheusPath`, `promautoPath`, `globalRegistrationRule`, `prometheusGlobals`, `goImageLiteral`, `TestNoProcessGlobalRegistration*`, `globalRegistrationViolations`) do not include `runnerLabel`, the one package-level name this change adds |

No pull request has more than 100 files. No Dependabot pull request is open.

Issues: #61 (this work; ruling comment 6035540608). #76 (spec queue) follows #61 by the owner's order and touches
none of these files. #86 (open, unclaimed) says `docs/repository-map.md` lacks three archive rows; this change's
archive row sits beside them. No open `class:flake` issue (`gh issue list --state open`).

## 4. The consumer at birth

No exported symbol, port, subject, bucket or config field. The new names are private to the test file: one field on
`ciJob` (`RunsOn`) and one constant, `runnerLabel`, beside `verifyTimeoutMinutes`
(`mergegate_test.go:129`: `const verifyTimeoutMinutes = 15`). Their consumer is `ciWorkflowViolations`, called by
`TestCIWorkflowPinned` and `TestCIWorkflowPinnedSensitivity` (`gopls references mergegate_test.go:279:6`: `:136`,
`:172`, `:246`). `ciJob` is read at `:284`, `:467` and `:558` (`gopls references mergegate_test.go:251:6`); nothing
builds a `ciJob` literal, so a new field changes no reader.

## 5. The problem shape

Shape: hold one externally moving version to an exact value, with the expected value a constant in the test and
never read from the file under test. Closest instances:

| Pin | Text |
| --- | --- |
| `internal/harness/contract/mergegate_test.go:335` | `} else if limit, ok := v.TimeoutMinutes.(int); !ok \|\| limit != verifyTimeoutMinutes {` |
| `internal/harness/contract/mergegate_test.go:121` | `// workflow writes it, with each set of results planted. As above, the expected values are constants` |
| `internal/harness/contract/mergegate_test.go:298` | `for _, name := range names {` (every job, sorted at `:297`; `writeGrants` applied to each at `:299`) |
| `internal/harness/contract/imagepin_test.go:11` | `// T-B3: the NATS image is spelled once, in .nats-image, by digest (owner ruling Q6). A second` |
| `scripts/doctor.sh:24` | `# task: .task-version is the pin (CI installs it from the same file).` |

The first three are the shape this change adopts: a field read in the existing per-job loop and compared with a
constant. No reusable primitive is introduced, so no adoption sweep is owed.

## Not triggered, with the reason

- Same-class collision table: no durable, communication or runtime-coordination primitive.
- Intent check: no boundary, port set, tier or capability moves.
- Pin probe and surface audit: nothing is ported from SemStreams.
- Adopter seam inventory: `ci.yml` is reached by no consumer outside this repository. For the in-repo adopter, a
  developer who later adds a job or moves to Ubuntu 26: (1) must know that every job runs on `ubuntu-24.04` and that
  a move is a `merge-gate` spec change, two items; (2) doing nothing, writing `runs-on: ubuntu-latest` as GitHub's
  examples do, fails nothing today; (3) finds out nowhere today, and from a failing `task test:unit` with a check;
  (4) should need to know nothing the failing check's message does not say. The outside party is GitHub, which
  moves what `ubuntu-24.04` resolves to (section 1) and will one day retire the label.

## Not measured

- Whether `task verify` behaves differently on Ubuntu 26 (out of scope by the ruling; the later move measures it).
- When GitHub retires `ubuntu-24.04`, and whether GitHub offers any way to pin a hosted image build.
- Whether GitHub matches runner labels without regard to case (matters only for a false refusal of `Ubuntu-24.04`).
