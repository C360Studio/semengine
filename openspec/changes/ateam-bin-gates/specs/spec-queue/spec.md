# spec-queue

## MODIFIED Requirements

### Requirement: The spec queue is ateam state spec-queue at the pin

`task spec:queue` SHALL run `.bin/ateam state spec-queue` (the pinned release, built by `task ateam:bin`) and nothing
else. At the pinned release the queue reads each `openspec/changes/<name>/tasks.md` but `archive`'s, with no `openspec`
CLI, and prints per change its ticked/total count, a line per open task whose first line carries a label (`WONTDO`,
`HALT`, `RED`, `BLOCKED`, `OPEN-Q`), a line per hold from `Hold:` on, `ok` for neither, or `done, not archived` when all
are ticked; it exits 0, or 2 when a file cannot be read.

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

### Requirement: spec:check runs validate then ateam state spec-check

`task spec:check` SHALL run `npx --no-install openspec validate --all --strict --no-interactive`, then `.bin/ateam state
spec-check`, and nothing else, each a plain string under `cmds`; besides `desc` and `summary` the only other key either
task may set is `deps: [ateam:bin]`, which builds `.bin/ateam`, and no key can skip a command or discard its exit
status; `task spec:queue` SHALL run `.bin/ateam state spec-queue` alone, the same way. A contract test SHALL fail naming
the task and the command when either entry differs.

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
- **THEN** the contract test fails naming `spec:queue` and `.bin/ateam state spec-queue` as the command it wants

#### Scenario: A second dependency

- **WHEN** a `Taskfile.yml` under test sets `deps: [ateam:bin, cleanup]` under `spec:queue`
- **THEN** the contract test fails naming `spec:queue` and `deps`
