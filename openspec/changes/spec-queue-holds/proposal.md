# spec-queue-holds

Status: revision 2, answering the pre-owner design review's round 1. It rests on `inventory.md`, which passed the
independent inventory review (`INVENTORY PASS` on the file with sha256 `70882609…bff3`; this copy differs only by one
repeated word removed at `:183`). Base `8abb383`. Issue #76; claim PR #113. The recommendation keeps a failing
command, but not the one the issue proposed, and the owner is asked to choose (`design.md`, "Questions for the owner",
Q1).

## Why

A hold is a note in an OpenSpec `tasks.md` that says an open task waits for something: the text `Hold:` followed
by what it waits for. `task spec:queue` exists to show, for each change in flight, why it is still open. Today it reads
only the first line of each open task (`scripts/openspec-queue.sh:182`). A hold written on any later line of the task
is shown by no command, and nothing fails:

- PR #48, task 3.6: a nested `- Hold:` bullet. Codex's review found it (F7, PR #48 comment 5968489295), not a command.
- PR #73, task 3.1: the hold slipped to a continuation line in `b91c60d`; the queue printed `ok`.
- PR #93, in flight: 18 of its 21 holds sit below their task's first line, and the queue shows none of them. For the
  other 3, the queue cuts the line at 104 bytes before it reaches what the hold waits for (`inventory.md` §2.5, §7).

The three documents that state the rule (`.agents/protocol.md:24-26`, `AGENTS.md:107`, the reviewer contract
`:159-160`) say a hold is written "on the unticked task it stops". None of them says "on its first line". The
first-line reading exists because of what the queue reads.

## What Changes

- **The queue reads the whole open task.** For each open task (`- [ ]` or `- [~]`), `task spec:queue` reads every
  line of the task, not just the first, and prints each hold as a `BLOCKED` line. The line is numbered where `Hold:`
  is written and its text runs from `Hold:` onward, so it shows what the hold waits for even when that wraps onto
  the next line. Nothing else the queue prints changes.
- **`task spec:check` fails on a hold outside every task.** A new `--check` mode of the same script fails when a
  `Hold:` is written below a `tasks.md`'s first `##` heading and in no task. That is the case the protocol names: a
  hold in a heading or a paragraph. `spec:check` runs it after `openspec validate`. On every real `tasks.md` it finds
  nothing, so no change in flight has to move anything.
- **Tests of the queue and the check.** `internal/harness/contract/specqueue_test.go` runs the script from a
  throwaway directory with a stand-in OpenSpec CLI and planted `tasks.md` files. The fixtures include PR #73's
  regression (`b91c60d`, task 3.1) and PR #48's nested bullet. The file also carries the nine cases of the SemStreams
  fixture test that was never ported, so today's first-line labels cannot change unnoticed, and a test that holds
  `spec:check` to running the check.
- **The rule's wording.** `.agents/protocol.md` and the `AGENTS.md` rules row say a hold may sit on any line of the
  task it stops, and name the check and the tests that hold it.

Not in this change:

- A rule that a hold must be on its task's first line (`design.md`, O3). It is designed in full in case the owner
  chooses it.
- The queue's other words (`halt`, `red`, `blocked`, `deliberate`, `still open`), which stay first-line only (issue
  #76, "Not in scope").
- A hold spelled another way, or written above a `tasks.md`'s first `##` heading, which is neither shown nor failed.
- The archive.
- The `AGENTS.md:85` overstatement (`inventory.md` §4, item 4), which this change does not touch.

## Capabilities

### New Capabilities

- `spec-queue`: what `task spec:queue` reads from an in-flight change's `tasks.md` and what it prints, and what its
  `--check` mode fails on. No current capability covers it (`design.md`, D7).

## Impact

- `scripts/openspec-queue.sh` (the reading loop, the `--check` mode and the header comment); `Taskfile.yml` (one line
  and the `desc` of `spec:check`); a new test file in `internal/harness/contract`; `.agents/protocol.md:24-26`;
  `AGENTS.md:35` and `:107`; `.agents/skills/semengine-preflight/SKILL.md:31`; `docs/testing.md:338-341`;
  `scripts/doctor.sh:40-42`; `docs/repository-map.md` at the archive.
- No new language, tool, task, `verify` step or CI job. No exported name. A local `task verify` comes to need
  python3, which only `task spec:queue` needed; CI already checks for it with `task doctor` (`design.md`, O10).
- A code pull request: Codex's review of record is needed before merge.
- PR #93 needs no edit. Once it merges `main`, its `spec:check` passes and its queue lists all 21 holds with what
  each waits for (`design.md`, P8, P14). Neither change waits for the other.
- If the owner chooses the first-line check (Q1), #93 moves 18 holds; if the owner wants no failing command, the
  check and its tests are dropped. Neither answer needs another design round.
