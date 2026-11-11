# Proposal: fix-599-ci-split

Track: design (one capability, `merge-gate`; four requirements added, two removed, four modified).

## Why

CI's one `Verify` job runs all of `task verify` and is cut off at 15 minutes (`.github/workflows/ci.yml:28`). The
alpha.4 ports added integration packages, and the last green runs took 9.3 to 13.2 minutes. Main run 43103302538 on
`2b803166` printed `verify: ok` and was then cancelled at the limit in a post-run step, so `Required` failed with
nothing broken (#599). `test:integration` alone takes 420 to 468 seconds of that time.

The owner ruled on 2026-11-11 (#599 comment 6807839061): "split CI into two jobs that run in parallel. One runs the
checks, unit tests and repeat run; the other runs the integration tests and the cover check. `Required` needs both.
Each job gets a time limit of about twice its normal run time. Local `task verify` stays the full gate." #599 is not
a flake.

Approved-by:

## What Changes

- `scripts/verify.sh` takes an optional group name. With none it runs every step, as today. `checks` runs every step
  but `test:integration` and `cover:check`; `integration` runs `ateam:doctor`, `test:integration` and `cover:check`.
  The step list stays the one list; the order, the host lines, the timings and the tracked-files check are unchanged
  in each run. An unknown group fails naming the two groups. `task verify` passes `--` arguments through.
- `.github/workflows/ci.yml`: the `verify` job becomes two jobs with the same setup steps, `checks` (runs
  `task verify -- checks`, limit 15 minutes) and `integration` (runs `task verify -- integration`, limit 20 minutes),
  with no dependency between them. `required` needs both.
- `TestCIWorkflowPinned` holds the two jobs, their limits, their run lines and `required`'s needs; a new
  `TestVerifyGroups` runs `scripts/verify.sh` with a stand-in `task` and checks each group's steps.
- The documents that describe the CI jobs say there are two.

## Capabilities

- Modified `merge-gate`: added "Verify runs in groups", "CI runs verify as two parallel jobs", "Each CI job has a
  time limit" and "Required needs both CI jobs"; removed "The verify job has a time limit" and "Required needs verify",
  which the last two replace; modified "Varied and repeated unit runs" (the "Why five minutes" scenario), "Pinned
  runner image", "How the changed files are found" and "A contract test guards both workflows".

## Impact

- `scripts/verify.sh`, `Taskfile.yml`, `.github/workflows/ci.yml`, `internal/harness/toolcheck/mergegate_test.go`,
  `scripts/tools-changed.sh` (a comment), `AGENTS.md`, `.agents/protocol.md`, `.agents/rules-index.md`,
  `.agents/skills/preflight/SKILL.md`, `docs/repository-map.md`, `openspec/specs/merge-gate/spec.md`.
- The `main` ruleset requires `Required` and `Merge check` only, so the job renames need no ruleset edit.
- Closes #599.
