# spec-queue

## ADDED Requirements

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

- **WHEN** the repository's `node_modules` is absent and `task spec:check` runs
- **THEN** its first command exits non-zero before `ateam state spec-check` runs, whose own validation would have
  printed `strict validation skipped` and gone on

#### Scenario: The check line removed

- **WHEN** a `Taskfile.yml` under test lists only the validate line under `spec:check`, or lists the two lines in
  the other order, or sets `ignore_error` on the task
- **THEN** the contract test fails naming `spec:check` and the command or key at fault

#### Scenario: The queue entry changed

- **WHEN** a `Taskfile.yml` under test runs `scripts/openspec-queue.sh` under `spec:queue`
- **THEN** the contract test fails naming `spec:queue` and `ateam state spec-queue` as the command it wants

## REMOVED Requirements

### Requirement: A hold on any line of an open task

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A task's block and a hold are defined by the text

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A hold prints one BLOCKED line

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A first line already labelled BLOCKED is not printed twice

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A hold in a ticked task or outside every block is not reported

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A hold line counts as a reported line under --strict

**Reason**: the option belonged to the retired script and nothing in the repository called it (`Taskfile.yml` ran
the script bare and with `--check` only).

**Migration**: none; the option goes with the script.

### Requirement: Labels on a task's first line

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: Label words are matched without regard to case

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A labelled first line is printed once; other tasks print nothing

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A change with no reported line prints ok

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: Without --strict the queue exits 0

**Reason**: the option belonged to the retired script and nothing in the repository called it (`Taskfile.yml` ran
the script bare and with `--check` only).

**Migration**: none; the option goes with the script.

### Requirement: The label scenarios are the pin's fixture cases

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A hold outside every task fails spec:check

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: The check prints each stray hold and exits by what it found

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: The check reports an unreadable change list

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: spec:check runs the check after validate

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: A tasks.md that cannot be read stops the queue and the check

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: An unreadable tasks.md exits 2 whatever was already printed

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: An unread file is never reported as free of caveats

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: An unreadable file takes precedence over earlier reports

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: The unavailable line goes to standard error

**Reason**: the queue and the hold check are `ateam state spec-queue` and `ateam state spec-check` at the pinned
release. What this requirement stated is restated in "The spec queue is ateam state spec-queue at the pin", "A stray
hold fails ateam state spec-check" or "spec:check runs validate then ateam state spec-check", or it was a mechanic
of the retired script, whose oracle is now the pin's own tests (`internal/state/openspec_test.go`).

**Migration**: none; `task spec:queue` and `task spec:check` keep their names and exit statuses.

### Requirement: --stale-days takes a whole number of days

**Reason**: the option belonged to the retired script and nothing in the repository called it (`Taskfile.yml` ran
the script bare and with `--check` only).

**Migration**: none; the option goes with the script.

### Requirement: An unknown argument is refused

**Reason**: the option belonged to the retired script and nothing in the repository called it (`Taskfile.yml` ran
the script bare and with `--check` only).

**Migration**: none; the option goes with the script.
