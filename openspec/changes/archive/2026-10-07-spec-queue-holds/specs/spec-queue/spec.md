# spec-queue

## ADDED Requirements

### Requirement: A hold on any line of an open task

`task spec:queue` SHALL report every hold in an open task of an in-flight change. Here an *in-flight change* is one
that `openspec list` lists; its `tasks.md` is `openspec/changes/<name>/tasks.md`. An *open task* is a line of that file
that begins, after any indentation, with `- [ ]` or `- [~]`. A *task's block* is the task's line and every line after
it, up to the next line that begins, after any indentation, with `- [` followed by one character and `]`, or up to
the next line that is not blank and is indented no deeper than the task's line, whichever comes first. A *hold* is
the text `Hold:`, matched with its case. For each open task whose block contains a hold, the queue SHALL print one
`BLOCKED` line, numbered at the first line of the block that contains a hold. Its text is the first word after the
task's checkbox (the task number), then the block's text from that hold onward: the lines joined with single spaces,
`**` removed, and cut at 104 bytes, the width the queue prints today. When the task's first line would also be
labelled `BLOCKED` (requirement "Labels on a task's first line"), the queue SHALL NOT print it as a second `BLOCKED`
line; a first line with any other label is still printed. Within a change, lines are printed in line order. The queue
SHALL NOT report a hold in the block of a ticked task (`- [x]`), nor a hold in text that lies in no open task's
block. A hold line counts as a reported line: with `--strict`, the queue SHALL exit 1 when it prints one.

#### Scenario: Hold on a continuation line

- **WHEN** an in-flight change's `tasks.md` holds task 3.1 as it reads at `b91c60d` (its first line
  `- [ ] 3.1 (D) ...`, and `Hold: until PR #48 merges` on the block's eighth line)
- **THEN** the queue prints a `BLOCKED` line numbered at that eighth line, whose text begins
  `3.1 Hold: until PR #48 merges`

#### Scenario: Hold in a nested bullet

- **WHEN** an open task 3.6's block contains, indented under it, the nested bullet
  ``- Hold: `message` waits for task 2.8.``, as at `a7dbf9d`
- **THEN** the queue prints a `BLOCKED` line numbered at the bullet's line, whose text begins
  ``3.6 Hold: `message` waits for task 2.8``

#### Scenario: What the hold waits for is on the next line

- **WHEN** an open task 3.12b's first line ends with `Hold:` and its next line reads `task 3.12a.`
- **THEN** the queue prints one `BLOCKED` line numbered at the first line, whose text contains `Hold: task 3.12a.`

#### Scenario: Hold on the first line

- **WHEN** an open task's first line contains a hold, as task 3.1 does at `b0bb713`
- **THEN** the queue prints exactly one `BLOCKED` line for that task, numbered at its first line, whose text begins
  `3.1 Hold:`

#### Scenario: The hold belongs to the next task

- **WHEN** open task 2.1 carries no hold and the open task 2.2 that follows it carries one on its second line
- **THEN** the queue prints one `BLOCKED` line, numbered at task 2.2's second line, whose text begins `2.2 Hold:`,
  and prints nothing for task 2.1

#### Scenario: Ticked task

- **WHEN** the only hold in the file is on the second line of a ticked task `- [x] 1.2`
- **THEN** the queue prints no `BLOCKED` line and prints the `ok` line for the change

#### Scenario: Hold outside any task

- **WHEN** the only hold in the file is in the paragraph under the title, as in the sentence that explains holds at
  the top of a `tasks.md`, or in a paragraph that follows the task list after a blank line and starts at column 0
- **THEN** the queue prints no `BLOCKED` line for it

#### Scenario: Other words on a continuation line

- **WHEN** an open task's only marker-like text is the lowercase `hold: until #12` or `blocked on #12`, on its second
  line
- **THEN** the queue prints no line for the task

#### Scenario: A held task with a red first line

- **WHEN** an open task's first line contains the word `failing` and the task carries a hold on its second line
- **THEN** the queue prints a `RED` line numbered at the first line and a `BLOCKED` line numbered at the second, in
  that order

#### Scenario: Strict run

- **WHEN** the queue is run with `--strict` on a change whose only reported line is a hold on a continuation line
- **THEN** it exits 1, and run without `--strict` on the same change it exits 0

### Requirement: Labels on a task's first line

For each open task, `task spec:queue` SHALL label the task's first line and print one line numbered at the first
line, with the line's text after its checkbox, `**` removed, cut at the queue's width. The labels are tried in this
order, and the first that matches wins:

- `WONTDO`, for a `- [~]` task.
- `HALT`, for the word `halt`, `halts`, `halted` or `halting`.
- `RED`, for the word `red`, `failed` or `failing`.
- `BLOCKED`, for the word `hold`, `blocked` or `blocking`.
- `WONTDO`, for a word that starts with `deliberate`, or for `not done`, `wont do` or `wontdo`.
- `OPEN-Q`, for the text `still open`.

Words are matched without regard to case, at word boundaries, except `still open`, which is matched anywhere. A first
line that matches none of them is not printed. A ticked task is not labelled. When a change has no reported line, the
queue SHALL print `ok` and `no halt/hold/deliberate marker in the open tasks`. Without `--strict`, the queue SHALL exit
0 whether or not it reports a line. The scenarios below are the nine cases of SemStreams' fixture test for this
script, at `8b99efe9` (`scripts/openspec-queue_fixture_test.sh:79-113`), with its texts word for word.

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

### Requirement: A hold outside every task fails spec:check

`scripts/openspec-queue.sh --check` SHALL read the `tasks.md` of every in-flight change and report each line that
contains a hold, lies below the file's first line that begins with `##` and a space, and lies in no task's block.
For this check a task is any line that begins, after any indentation, with `- [` followed by one character and `]`,
ticked tasks included, and its block is as defined above. For each such line it SHALL print the file's path and
the line's number, `<path>:<line>:`, then a message saying that `task spec:queue` cannot show a hold outside a task
and that the hold belongs in the task it stops. It SHALL then exit 1. When there is no such line it SHALL print
`holds: ok (<n> tasks.md read)` and exit 0. When the list of in-flight changes cannot be read, it SHALL print the
cause on standard error, starting `queue unavailable:` as the queue does, print no `holds: ok`, and exit 2. With
`--check` the script does not print the queue. `task spec:check` SHALL run
`scripts/openspec-queue.sh --check` after `openspec validate`, and a contract test SHALL fail when the `spec:check`
task does not run exactly those two commands in that order.

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

#### Scenario: The change list cannot be read

- **WHEN** `openspec list --json` exits non-zero, or prints text that is not JSON
- **THEN** `scripts/openspec-queue.sh --check` prints a line starting `queue unavailable:` on standard error, prints
  no `holds: ok`, and exits 2

#### Scenario: spec:check runs the check

- **WHEN** the `spec:check` task in `Taskfile.yml` does not run `scripts/openspec-queue.sh --check` after
  `npx --no-install openspec validate --all --strict --no-interactive`
- **THEN** the contract test fails naming the task and the command it lacks
