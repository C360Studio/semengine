# spec-queue Specification

## Purpose
The spec queue is `scripts/openspec-queue.sh`, run as `task spec:queue`: for each OpenSpec change in flight it
prints how many tasks are ticked and one line for each open task that carries a label, so a reader sees why a change
is still open. It reads every line of each open task, and it prints a hold (the text `Hold:`) from `Hold:` on,
wherever in the task the hold is written. The queue is advisory: it exits 0 whatever it shows unless run with
`--strict`, exit 2 means it could not read the changes or refused its arguments, and neither `task verify` nor CI runs it. Run with `--check`,
as `task spec:check` runs it, the same script fails on a hold written below the file's first `##` heading and outside
every task, where the queue cannot show it.

## Requirements

### Requirement: A hold on any line of an open task

`task spec:queue` SHALL report every hold in an open task of an in-flight change. Here an *in-flight change* is one
that `openspec list` lists; its `tasks.md` is `openspec/changes/<name>/tasks.md`. An *open task* is a line of that file
that begins, after any indentation, with `- [ ]` or `- [~]`.

#### Scenario: Hold on a continuation line

- **WHEN** an in-flight change's `tasks.md` holds task 3.1 as it reads at `b91c60d` (its first line
  `- [ ] 3.1 (D) ...`, and `Hold: until PR #48 merges` on the block's eighth line)
- **THEN** the queue prints a `BLOCKED` line numbered at that eighth line, whose text begins
  `3.1 Hold: until PR #48 merges`

### Requirement: A task's block and a hold are defined by the text

A *task's block* SHALL be the task's line and every line after it, up to the next line that begins, after any
indentation, with `- [` followed by one character and `]`, or up to the next line that is not blank and is indented no
deeper than the task's line, whichever comes first. A *hold* is the text `Hold:`, matched with its case.

#### Scenario: The hold belongs to the next task

- **WHEN** open task 2.1 carries no hold and the open task 2.2 that follows it carries one on its second line
- **THEN** the queue prints one `BLOCKED` line, numbered at task 2.2's second line, whose text begins `2.2 Hold:`,
  and prints nothing for task 2.1

#### Scenario: Other words on a continuation line

- **WHEN** an open task's only marker-like text is the lowercase `hold: until #12` or `blocked on #12`, on its second
  line
- **THEN** the queue prints no line for the task

### Requirement: A hold prints one BLOCKED line

For each open task whose block contains a hold, the queue SHALL print one `BLOCKED` line, numbered at the first line
of the block that contains a hold. Its text is the first word after the task's checkbox (the task number), then the
block's text from that hold onward: the lines joined with single spaces, `**` removed, and cut at 104 bytes, the width
the queue prints today.

#### Scenario: Hold in a nested bullet

- **WHEN** an open task 3.6's block contains, indented under it, the nested bullet
  ``- Hold: `message` waits for task 2.8.``, as at `a7dbf9d`
- **THEN** the queue prints a `BLOCKED` line numbered at the bullet's line, whose text begins
  ``3.6 Hold: `message` waits for task 2.8``

#### Scenario: What the hold waits for is on the next line

- **WHEN** an open task 3.12b's first line ends with `Hold:` and its next line reads `task 3.12a.`
- **THEN** the queue prints one `BLOCKED` line numbered at the first line, whose text contains `Hold: task 3.12a.`

### Requirement: A first line already labelled BLOCKED is not printed twice

When the task's first line would also be labelled `BLOCKED` (requirement "Labels on a task's first line"), the queue
SHALL NOT print it as a second `BLOCKED` line; a first line with any other label is still printed. Within a change,
lines are printed in line order.

#### Scenario: Hold on the first line

- **WHEN** an open task's first line contains a hold, as task 3.1 does at `b0bb713`
- **THEN** the queue prints exactly one `BLOCKED` line for that task, numbered at its first line, whose text begins
  `3.1 Hold:`

#### Scenario: A held task with a red first line

- **WHEN** an open task's first line contains the word `failing` and the task carries a hold on its second line
- **THEN** the queue prints a `RED` line numbered at the first line and a `BLOCKED` line numbered at the second, in
  that order

### Requirement: A hold in a ticked task or outside every block is not reported

The queue SHALL NOT report a hold in the block of a ticked task (`- [x]`), nor a hold in text that lies in no open
task's block.

#### Scenario: Ticked task

- **WHEN** the only hold in the file is on the second line of a ticked task `- [x] 1.2`
- **THEN** the queue prints no `BLOCKED` line and prints the `ok` line for the change

#### Scenario: Hold outside any task

- **WHEN** the only hold in the file is in the paragraph under the title, as in the sentence that explains holds at
  the top of a `tasks.md`, or in a paragraph that follows the task list after a blank line and starts at column 0
- **THEN** the queue prints no `BLOCKED` line for it

### Requirement: A hold line counts as a reported line under --strict

A hold line counts as a reported line: with `--strict`, the queue SHALL exit 1 when it prints one.

#### Scenario: Strict run

- **WHEN** the queue is run with `--strict` on a change whose only reported line is a hold on a continuation line
- **THEN** it exits 1, and run without `--strict` on the same change it exits 0

### Requirement: Labels on a task's first line

For each open task, `task spec:queue` SHALL label the task's first line. The labels are tried in this order, and the
first that matches wins:

- `WONTDO`, for a `- [~]` task.
- `HALT`, for the word `halt`, `halts`, `halted` or `halting`.
- `RED`, for the word `red`, `failed` or `failing`.
- `BLOCKED`, for the word `hold`, `blocked` or `blocking`.
- `WONTDO`, for a word that starts with `deliberate`, or for `not done`, `wont do` or `wontdo`.
- `OPEN-Q`, for the text `still open`.

#### Scenario: Lowercase halt mid-sentence in an open task

- **WHEN** an open task reads `- [ ] 4.3 If the pre-v1 wipe window closed before 3.1, halt: record the missed window.`
- **THEN** the queue prints a `HALT` line for it

#### Scenario: Partial marker regardless of wording

- **WHEN** a task reads `- [~] 4.2 Verifying Workflow.Name equals the Schema own Workflow().`
- **THEN** the queue prints a `WONTDO` line for it

#### Scenario: Explicit RED gate

- **WHEN** an open task reads `- [ ] 4.2 RED — semantic e2e is failing, see gh#830.`
- **THEN** the queue prints a `RED` line for it

#### Scenario: HOLD wording

- **WHEN** an open task reads `- [ ] 8.3 On HOLD pending the owner ruling.`
- **THEN** the queue prints a `BLOCKED` line for it

#### Scenario: STILL OPEN wording

- **WHEN** an open task reads `- [ ] 8.3 **STILL OPEN** — decide whether it rides the sister replay.`
- **THEN** the queue prints an `OPEN-Q` line for it

#### Scenario: Deliberate not-done wording

- **WHEN** an open task reads
  `- [ ] 4.2 Not enforced, deliberately, because it converts a posture into a boot failure.`
- **THEN** the queue prints a `WONTDO` line for it

#### Scenario: Completed task mentioning halt is history, not a live condition

- **WHEN** a ticked task reads `- [x] 4.3 The wipe window halt condition was evaluated and did not fire.`
- **THEN** the queue prints no `HALT` line

#### Scenario: Ordinary open task with no caveat

- **WHEN** an open task reads `- [ ] 2.1 Run one comparative benchmark and record it as ADR evidence.`
- **THEN** the queue prints no `HALT` line

#### Scenario: Clean change emits an explicit no-marker line

- **WHEN** a change's only tasks are `- [ ] 2.1 Run one comparative benchmark and record it as ADR evidence.` and
  `- [x] 2.2 Enumerate the consumers.`
- **THEN** the queue prints `no halt/hold/deliberate marker` for the change

### Requirement: Label words are matched without regard to case

Words SHALL be matched without regard to case, at word boundaries, except `still open`, which is matched anywhere.

#### Scenario: Label match ignores case

- **WHEN** an open task's first line reads `- [ ] 8.3 On HOLD pending the owner ruling.`
- **THEN** the queue matches `hold` in `HOLD` and prints a `BLOCKED` line for it

### Requirement: A labelled first line is printed once; other tasks print nothing

A task whose first line matches a label SHALL print one line numbered at the first line, with the line's text after its
checkbox, `**` removed, cut at the queue's width. A first line that matches none of them is not printed. A ticked task
is not labelled.

#### Scenario: Unlabelled and ticked tasks print nothing

- **WHEN** a first line matches no label, or the task is ticked
- **THEN** the queue prints no line for it

### Requirement: A change with no reported line prints ok

When a change has no reported line, the queue SHALL print `ok` and `no halt/hold/deliberate marker in the open
tasks`.

#### Scenario: Nothing to report

- **WHEN** a change's open tasks carry no label and no hold
- **THEN** the queue prints `ok` and `no halt/hold/deliberate marker in the open tasks` for it

### Requirement: Without --strict the queue exits 0

Without `--strict`, the queue SHALL exit 0 whether or not it reports a line.

#### Scenario: Reported line without --strict

- **WHEN** the queue is run without `--strict` on a change with a reported line
- **THEN** it exits 0

### Requirement: The label scenarios are the pin's fixture cases

The nine scenarios of "Labels on a task's first line", from "Lowercase halt mid-sentence in an open task" through
"Clean change emits an explicit no-marker line", SHALL be the nine cases of SemStreams' fixture test for this script,
at `8b99efe9` (`scripts/openspec-queue_fixture_test.sh:79-113`), with its texts word for word.

#### Scenario: Fixture cases match the pin

- **WHEN** the nine scenarios are compared with the fixture test at `8b99efe9`
- **THEN** each uses the fixture's text word for word

### Requirement: A hold outside every task fails spec:check

`scripts/openspec-queue.sh --check` SHALL read the `tasks.md` of every in-flight change and report each line that
contains a hold, lies below the file's first line that begins with `##` and a space, and lies in no task's block.
For this check a task is any line that begins, after any indentation, with `- [` followed by one character and `]`,
ticked tasks included, and its block is as defined above.

#### Scenario: Hold in a paragraph after the task list

- **WHEN** below `## 1. Work` and its tasks, after a blank line, a paragraph at column 0 reads
  `Hold: task 1.1 waits for #12.`
- **THEN** `scripts/openspec-queue.sh --check` prints that file's path and that line's number and exits 1

#### Scenario: Hold in a heading

- **WHEN** a heading below the file's first `##` heading reads `## 3. Hold: waits for #12`
- **THEN** `scripts/openspec-queue.sh --check` prints that file's path and that line's number and exits 1

#### Scenario: The sentence that explains holds

- **WHEN** the only hold outside every task is in the paragraph above the file's first `##` heading
- **THEN** `scripts/openspec-queue.sh --check` prints `holds: ok (1 tasks.md read)` and exits 0

#### Scenario: Holds in tasks

- **WHEN** the only holds are in task blocks, as in task 3.1 at `b91c60d` or on the second line of a ticked task
- **THEN** `scripts/openspec-queue.sh --check` prints `holds: ok (1 tasks.md read)` and exits 0

### Requirement: The check prints each stray hold and exits by what it found

For each such line it SHALL print the file's path and the line's number, `<path>:<line>:`, then a message saying that
`task spec:queue` cannot show a hold outside a task and that the hold belongs in the task it stops. It SHALL then exit
1. When there is no such line it SHALL print `holds: ok (<n> tasks.md read)` and exit 0. With `--check` the script does
not print the queue.

#### Scenario: The message names the fix

- **WHEN** `--check` finds a hold outside every task
- **THEN** it prints `<path>:<line>:` and a message that `task spec:queue` cannot show a hold outside a task and that
  the hold belongs in the task it stops, and exits 1

### Requirement: The check reports an unreadable change list

When the list of in-flight changes cannot be read, `scripts/openspec-queue.sh --check` SHALL print the cause on
standard error, starting `queue unavailable:` as the queue does, print no `holds: ok`, and exit 2.

#### Scenario: The change list cannot be read

- **WHEN** `openspec list --json` exits non-zero, or prints text that is not JSON
- **THEN** `scripts/openspec-queue.sh --check` prints a line starting `queue unavailable:` on standard error, prints
  no `holds: ok`, and exits 2

### Requirement: spec:check runs the check after validate

`task spec:check` SHALL run `scripts/openspec-queue.sh --check` after `openspec validate`, and a contract test SHALL
fail when the `spec:check` task does not run exactly those two commands in that order.

#### Scenario: spec:check runs the check

- **WHEN** the `spec:check` task in `Taskfile.yml` does not run `scripts/openspec-queue.sh --check` after
  `npx --no-install openspec validate --all --strict --no-interactive`
- **THEN** the contract test fails naming the task and the command it lacks

### Requirement: A tasks.md that cannot be read stops the queue and the check

When an in-flight change has a `tasks.md` file that `scripts/openspec-queue.sh` cannot open, or whose content is not
valid UTF-8, the script SHALL print on standard error the line `queue unavailable: cannot read <path>`, where `<path>`
is `openspec/changes/<name>/tasks.md`, and SHALL exit 2. This holds when the script prints the queue
(`task spec:queue`), with or without `--strict`, and when it runs with `--check` (`task spec:check`).

#### Scenario: The queue meets a tasks.md that is not UTF-8

- **WHEN** the only in-flight change's `tasks.md` contains the two bytes `0xff 0xfe`, which are not valid UTF-8, and
  `scripts/openspec-queue.sh` is run without `--strict`
- **THEN** it prints `queue unavailable: cannot read openspec/changes/<name>/tasks.md` as a line on standard error,
  prints no `ok` line, and exits 2

#### Scenario: The check meets a tasks.md that is not UTF-8

- **WHEN** the only in-flight change's `tasks.md` contains the two bytes `0xff 0xfe`, and
  `scripts/openspec-queue.sh --check` is run
- **THEN** it prints `queue unavailable: cannot read openspec/changes/<name>/tasks.md` as a line on standard error,
  prints no `holds: ok`, and exits 2

### Requirement: An unread file is never reported as free of caveats

The script SHALL NOT print the `ok` line of requirement "Labels on a task's first line" for that change, and with
`--check` it SHALL NOT print `holds: ok`: a file the script did not read is never reported as free of caveats or of
misplaced holds.

#### Scenario: No ok line for an unread file

- **WHEN** a change's `tasks.md` cannot be read
- **THEN** the script prints no `ok` line for it, and with `--check` prints no `holds: ok`

### Requirement: An unreadable file takes precedence over earlier reports

The requirement "A tasks.md that cannot be read stops the queue and the check" SHALL take precedence over the `ok` and
`holds: ok` lines and over the exit 0 and exit 1 that the requirements above state, also when a change listed before
that file has already reported a line.

#### Scenario: An earlier change reported a line

- **WHEN** a change listed first reported a line and a later change's `tasks.md` cannot be read
- **THEN** the script exits 2, not 0 or 1

### Requirement: The unavailable line goes to standard error

The `queue unavailable:` line SHALL be printed on standard error; other text, such as the reader's own error, MAY come
before that line on standard error.

#### Scenario: Reader error before the line

- **WHEN** the reader prints its own error before the script reaches the line
- **THEN** `queue unavailable: cannot read <path>` is still printed as a line on standard error

### Requirement: Output printed before the unreadable file stays on standard output

The script SHALL exit as above even when what it printed before it reached that file is already on standard output: the
queue's lines for the changes listed before it, and that change's own count line, which the queue prints before it reads
the change's `tasks.md`; or the `<path>:<line>:` lines that `--check` printed for the changes listed before it.

#### Scenario: A count line precedes the failure

- **WHEN** the queue prints a change's count line and then cannot read that change's `tasks.md`
- **THEN** the count line is already on standard output and the script exits 2

### Requirement: --stale-days takes a whole number of days

`scripts/openspec-queue.sh` SHALL accept `--stale-days` only when the next argument is ASCII digits alone that
bash's integer test reads, as a decimal number of days. Otherwise it SHALL print `--stale-days needs a whole number
of days` on standard error, print nothing on standard output, and exit 2, before it reads the change list or what
follows. It SHALL never read another option as the operand, and its argument processing SHALL end on every argument list.

#### Scenario: --stale-days is the last argument

- **WHEN** `scripts/openspec-queue.sh --stale-days` is run
- **THEN** it ends, prints `--stale-days needs a whole number of days` as a line on standard error, prints nothing
  on standard output, and exits 2

#### Scenario: The operand is not a whole number

- **WHEN** the script is run with `--stale-days abc`, `-1`, `7d`, `''`, or `99999999999999999999`
- **THEN** it prints `--stale-days needs a whole number of days` on standard error, prints nothing on standard
  output, and exits 2

#### Scenario: An option in the operand's place

- **WHEN** the script is run with `--stale-days --strict`, or with `--strict --stale-days abc`
- **THEN** it prints `--stale-days needs a whole number of days` on standard error, prints nothing on standard
  output, and exits 2

#### Scenario: Zero and leading zeros are whole numbers

- **WHEN** the script is run with `--stale-days 0` or `--stale-days 08` on a change whose open tasks carry no caveat
- **THEN** it prints the queue with that change's `ok` line, prints nothing on standard error, and exits 0

### Requirement: An unknown argument is refused

For an argument it does not know, `scripts/openspec-queue.sh` SHALL print `unknown argument: <arg>` on standard error
and exit 2.

#### Scenario: An unknown argument

- **WHEN** the script is run with `--bogus`
- **THEN** it prints `unknown argument: --bogus` on standard error and exits 2
