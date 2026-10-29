# Proposal: adopt-ateam-spec-queue

## Why

Issue #450, PR C of epic #446. The spec queue is still SemEngine's own `scripts/openspec-queue.sh` (280 lines of
bash and python3) with a 500-line test file in `internal/harness/toolcheck`. ateam's `state spec-queue` and
`state spec-check` were ported from that script and read `tasks.md` the same way (ateam v0.3.4,
`internal/state/openspec.go`: the task block, the five labels, the hold text from `Hold:` on, the stray-hold check
with the same message), and they add what the #447 rulings rely on: the `Approved-by:` read and the proposal and
design length warnings. They read the queue from the directory, with no `openspec list --json` and no python3.

Owner ruling on this epic: match ateam unless there is a very good reason not to, and record the divergence.

Approved-by:

## What Changes

- `task spec:queue` runs `ateam state spec-queue`. `task spec:check` runs
  `npx --no-install openspec validate --all --strict --no-interactive`, then `ateam state spec-check`.
- `scripts/openspec-queue.sh` goes, with its `--strict` and `--stale-days` options (nothing in the repository
  called them), and so does `internal/harness/toolcheck/specqueue_test.go`. One contract test keeps the two Task
  entries wired as above and is shown able to fail.
- `scripts/doctor.sh`: the python3 row stays, because `scripts/port-status.sh` (`task status`) still needs
  python3; its comment names that script instead of the queue. #450 expected the row to go; it does not.
- `openspec/specs/spec-queue/spec.md` is rewritten: three requirements name the ateam commands at the pin and
  the Task wiring, in place of the 23 that spelled out the script.
- `.agents/rules-index.md`, `docs/testing.md` and `internal/harness/toolcheck/doc.go` name the new test in place of
  the five script tests. `AGENTS.md` names only the Task entries and is unchanged.

Divergence kept: `task spec:check` keeps its own `openspec validate` line before `ateam state spec-check`. At the
pin the ateam check runs the same validation itself when it finds `node_modules/.bin/openspec` or `openspec` on
`PATH`, but prints `strict validation skipped` and exits 0 when it finds neither (`openspec.go:467-485`); the Task
line fails closed, since `npx --no-install` exits non-zero when the pinned CLI is missing. The validation runs twice,
under a second each (0.8 s measured on 2026-10-29).

## Capabilities

### Modified Capabilities

- `spec-queue`: the script's requirements are replaced by the ateam commands at the pin and the Task wiring.
