# spec-queue Specification

## Purpose
The spec queue is `ateam state spec-queue`, run as `task spec:queue`: for each OpenSpec change in flight it prints
how many tasks are ticked, one line for each open task whose first line carries a label, and one line for each hold
(the text `Hold:`) from `Hold:` on, wherever in the task it is written, so a reader sees why a change is still open.
It is advisory and exits 0; exit 2 means a change's file could not be read. `task spec:check` runs the pinned
`openspec validate` and then `ateam state spec-check`, which fails on a hold written below a file's first `##`
heading and outside every task, where the queue cannot show it, and warns on a proposal over one page or a design
over its limit. Both commands are ateam's at the pinned release, which tests them; this specification states how
they are wired here and what a reader can expect from them.

## Requirements

### Requirement: The spec queue is ateam state spec-queue at the pin

`task spec:queue` SHALL run `ateam state spec-queue` (`ateam` on `PATH`) and nothing else. At the pinned release
the queue reads each `openspec/changes/<name>/tasks.md` but `archive`'s, with no `openspec` CLI, and prints per
change its ticked/total count, a line per open task whose first line carries a label (`WONTDO`, `HALT`, `RED`,
`BLOCKED`, `OPEN-Q`), a line per hold from `Hold:` on, `ok` for neither, or `done, not archived` when all are
ticked; it exits 0, or 2 when a file cannot be read.

#### Scenario: Hold on a continuation line

- **WHEN** an in-flight change's `tasks.md` holds task 1.1 with `Hold: OPEN-Q proposal awaits owner approval.` on
  the block's third line, as this change's does at its first commit
- **THEN** the queue prints an `OPEN-Q` line numbered at that third line, whose text begins
  `Hold: OPEN-Q proposal awaits owner approval`

#### Scenario: Every task ticked

- **WHEN** an in-flight change's open tasks are all ticked and none carries a label
- **THEN** the queue prints `done, not archived` for the change and names the archive directory to move it to

#### Scenario: A tasks.md that is not UTF-8

- **WHEN** the only in-flight change's `tasks.md` contains the two bytes `0xff 0xfe`
- **THEN** the queue prints `queue unavailable: cannot read openspec/changes/<name>/tasks.md` on standard error,
  prints no `ok` line, and exits 2

### Requirement: A stray hold fails ateam state spec-check

At the pinned release `ateam state spec-check` SHALL, after strict validation, print `<path>:<line>:` and a message
that `task spec:queue` cannot show a hold outside a task for each `Hold:` below a `tasks.md`'s first `##` heading
that lies in no task's block, and exit 1; exit 2 when a change's file cannot be read; else print a warning for a
`proposal.md` over 60 lines or a `design.md` over 120, then `holds: ok (<n> tasks.md read)`, and exit 0.

#### Scenario: Hold in a paragraph after the task list

- **WHEN** below `## 1. Work` and its tasks, after a blank line, a paragraph at column 0 reads
  `Hold: task 1.1 waits for #12.`
- **THEN** `ateam state spec-check` prints that file's path and line number, then
  `Hold: outside every task; task spec:queue cannot show it. Put it in the task it stops.`, and exits 1

#### Scenario: The sentence that explains holds

- **WHEN** the only hold outside every task is in the paragraph above the file's first `##` heading, and every
  other hold is in a task's block
- **THEN** `ateam state spec-check` prints `holds: ok (<n> tasks.md read)` and exits 0

#### Scenario: Proposal over one page

- **WHEN** an in-flight change's `proposal.md` has 61 lines and its holds are all in task blocks
- **THEN** `ateam state spec-check` prints a `warning:` line naming that file and `61 lines`, then `holds: ok`, and
  exits 0

### Requirement: spec:check runs validate then ateam state spec-check

`task spec:check` SHALL run `npx --no-install openspec validate --all --strict --no-interactive`, then
`ateam state spec-check`, and nothing else, each a plain string under `cmds` with no key that can skip a command or
discard its exit status; `task spec:queue` SHALL run `ateam state spec-queue` alone, the same way. A contract test
SHALL fail naming the task and the command when either entry differs.

#### Scenario: The validate line is the fail-closed one

- **WHEN** no `openspec` is reachable by `npx` (no `node_modules/.bin` in the tree or a parent, none in npm's
  global prefix or cache) and none on `PATH`, as on a CI runner before `npm ci`, and `task spec:check` runs
- **THEN** its first command exits non-zero before `ateam state spec-check` runs, whose own validation would have
  printed `strict validation skipped` and gone on

#### Scenario: The check line removed

- **WHEN** a `Taskfile.yml` under test lists only the validate line under `spec:check`, or lists the two lines in
  the other order, or sets `ignore_error` on the task
- **THEN** the contract test fails naming `spec:check` and the command or key at fault

#### Scenario: The queue entry changed

- **WHEN** a `Taskfile.yml` under test runs `scripts/openspec-queue.sh` under `spec:queue`
- **THEN** the contract test fails naming `spec:queue` and `ateam state spec-queue` as the command it wants
