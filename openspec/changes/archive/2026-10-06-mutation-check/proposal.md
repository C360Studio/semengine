# mutation-check

Status: accepted by the owner (#79, comment 5982900883: "Accept, agree with all five"). Revision 6 corrects the delta
after implementation review rounds 1 to 3 and the owner's "fix now" (#79, comment 5994738791), with no decision changed.
It rests on `inventory.md`, which has `INVENTORY PASS` (PR #82, comment 5981915103), and on `design.md`.

## Why

`docs/testing.md`, "Show that the test can fail", asks for a mutation check: one deliberate wrong change, and three
runs of the test (before, with and after it), recorded on the pull request. The outcome is classified by hand, and
the record shows the hand classification going wrong: a whole-run timeout, a process killed by the operating system,
a stack overflow and a deadlock panic were each counted as a detection (inventory 2b). The manual procedure also
writes the wrong change into the shared tree and relies on a restore that a killed run never makes (inventory 2c,
6). Issue #79 asks for a command that runs the experiment and classifies it by rule.

A trial on 12 recorded wrong changes and 9 planted ones (`design.md`, "How overlay went") showed that Go's
`-overlay` build option can apply a wrong change to Go source without writing any file. The three runs of each wrong
change agreed in every case. Go does not apply an overlay to a file it builds with coverage, so the design measures
reach on the unchanged code and says why that answers the question.

## What Changes

- **`task mutate:check`** runs a new program, `internal/harness/mutcheck`, for one wrong change to one non-test Go
  source file. The implementer gives the package, the test, the target file, a changed copy of it kept outside the
  repository, and the assertion expected to fail, by the location Go prints or by its message.
- **The program writes nothing inside the repository.** The wrong change reaches the test, and any `go build` the test
  starts, through `GOFLAGS=-overlay=<file>`. Nothing needs restoring after an interrupt or a kill. The tree's
  fingerprint is compared before and after. A `GOFLAGS` that sets an overlay or coverage is refused.
- **It classifies by rule:** three baseline runs, three mutant runs and one after-run, giving detection, survivor,
  invalid or inconclusive. Survivor and invalid are told apart by one more run of the unchanged code with coverage:
  whether it executed the lines the wrong change removes or replaces, or the place where it inserts lines. A timeout,
  a kill or a failing exit status alone is never a detection; a race-detector report counts only when it is named.
- **Every run has the same flags and environment** apart from the overlay and the reach run's coverage, with the
  caller's `GOFLAGS` read from `go env` (so a value set with `go env -w` counts) and passed to every run. Every run sets
  `RAPID_SEED` and `RAPID_NOFAILFILE=true`, and the command refuses to start when an untracked Rapid failure file is
  present.
- **It prints a report a pull request can quote,** with the verdict on its last line, and exits zero only for
  detection.
- **Documents** (after #48 and then #73 merge, as the owner sequenced): `docs/testing.md` gains the command, the
  outcome invalid and how reach is judged, the rule on bounded assertions, the race detector as ruled, and equivalence
  as an assessment; the contracts name the command and SHA-256; `AGENTS.md` gains the command, and its mutation row
  changes as the owner rules.

Not in this change: wrong changes to scripts, YAML, test data or test files, which overlay cannot reach or the page
forbids (they keep the manual procedure); integration tests; generating wrong changes; any CI or `task verify` step;
any change to `merge-check`.

## Capabilities

### New Capabilities

- `mutation-check`: the command's inputs and refusals, the guarantee that the program writes nothing, the runs and
  their environment, the changed region and reach, the outcome classification, and the report and exit status.

## Impact

- New: `internal/harness/mutcheck` (the program, its tests, and recorded `go test -json` output, line diffs and
  coverage profiles under `testdata/`); one `Taskfile.yml` entry.
- Changed later in the change (each waits as `tasks.md` says): `docs/testing.md`, `AGENTS.md`,
  `docs/repository-map.md`, the developer and reviewer contracts, and the preflight skill's command table.
- Test time: measured at 31.607 s summed over the package's `test:unit` and `test:repeat` runs (CI run 37228603260),
  under the 60 s budget (task 2.9).
- Owner questions in `design.md`: the `AGENTS.md` row (Q1), timeouts by design (Q2), scripts and other files read at
  run time (Q3), a Go program instead of the script #79 names (Q4), and race-detector reports (Q5).
- Overlaps: PR #48 and PR #73 (documents; ruled), #80 (documents; different sections), #60 (none: this change adds
  no writer).
