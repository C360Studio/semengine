# spec-queue-unreadable-tasks

Status: revision 2, draft. It rests on `inventory.md` (`INVENTORY PASS`, review round 1) and `design.md` (round 1
asked for changes; revision 2 answers them). Issue #115, claim PR #122.

## Why

PR #113 made `scripts/openspec-queue.sh` read each in-flight change's `tasks.md` with a UTF-8 reader and fail closed:
when the file cannot be opened or is not valid UTF-8, `task spec:queue` and `task spec:check` print
`queue unavailable: cannot read <path>` on standard error and exit 2. Before #113, an unreadable file printed `ok`,
so a held change looked unheld. Nothing holds the new behaviour: no test plants an unreadable `tasks.md`, and the
`spec-queue` spec states exit 2 only for an unreadable change list. If the refusal were later turned into a skip, the
queue could again print `ok` for a change it never read, and every gate would stay green. Codex's review of #113
named this as a limit (comment 6041276484).

## What Changes

- **The spec.** `spec-queue` gains one requirement, "A tasks.md that cannot be read stops the queue and the check",
  with one scenario for the queue and one for `--check`. It says the script prints no `ok` or `holds: ok` for a file
  it did not read, and that its exit 2 takes precedence over the lines, and the exit 0 and exit 1, that the other
  requirements state.
- **The tests.** `internal/harness/contract/specqueue_test.go` gains H12 in `TestSpecQueueHolds` and C8 in
  `TestSpecQueueCheck`. Each plants a `tasks.md` that is not valid UTF-8 and expects exit 2, the standard-error line,
  and no `ok` or `holds: ok` (`design.md`, D2).
- **Shown able to fail.** By hand, with three wrong changes to the script, each recorded on the pull request
  (`design.md`, D3).

`scripts/openspec-queue.sh` gets a one-line help-text edit (`:47`, printed by `--help`, now names an unreadable
`tasks.md` beside the change list); no behaviour change (`design.md`, D4).

Not in this change: `--strict`, a mode-000 file, several changes at once, and a change with no `tasks.md`, each
measured but not tested (`design.md`, "Not in this change"); `AGENTS.md:85` and the trailing space in a printed hold,
which #115 excludes.

## Capabilities

### Modified Capabilities

- `spec-queue`: one requirement added. No existing requirement changes.

## Impact

- `internal/harness/contract/specqueue_test.go` (two cases, two comments); `scripts/openspec-queue.sh` (one help-text
  line); `openspec/specs/spec-queue/spec.md` (by the archive); `docs/repository-map.md` (the archive row and the
  `spec-queue` row).
- A code pull request: Codex's review of record is needed before merge.
- No new exported name, flag, message, exit status, job or tool.
- PR #93 and draft #121 change other files of the same Go package; #117 bumps the OpenSpec CLI this change is
  validated with. None shares a file; neither side waits (`design.md`, "Overlaps and order").
