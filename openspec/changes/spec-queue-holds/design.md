# Design: spec-queue-holds

Status: revision 2, answering the pre-owner design review's round 1 (CHANGES REQUESTED, nine findings). Base
`8abb383`. `inventory.md` passed the independent inventory review (`INVENTORY PASS`, sha256 `70882609…bff3`); the
copy in this change differs only by one repeated word removed at `:183`. The recommendation keeps a failing command,
but not the one the issue proposed, so it waits for the owner's answer to Q1.

## Terms

- **The queue**: `task spec:queue`, which runs `scripts/openspec-queue.sh`. It lists each OpenSpec change in flight
  and the lines of its `tasks.md` that explain why the change is still open. It is advisory: it exits 0 whether or not
  it shows anything, and neither `task verify` nor CI runs it (`AGENTS.md:54`).
- **Open task**: a line of `tasks.md` that begins, after any indentation, with `- [ ]` (not done) or `- [~]`
  (partly done). A *ticked task* begins with `- [x]`.
- **First line** and **continuation line**: a task's checkbox line is its first line. Text wrapped under it, indented
  deeper, nested bullets included, is on its continuation lines.
- **Block**: a task's first line plus its continuation lines. The block stops at the next checkbox line, or at the
  next non-blank line indented no deeper than the task (a heading, or a paragraph at the left margin).
- **Hold**: the text `Hold:`, written in a task to say what the task waits for, as in `Hold: task 3.12a.` or
  `Hold: until PR #48 merges`.
- **Fail** and **show**: a check *fails* when its command exits non-zero, so `task verify` and CI go red. The queue
  *shows* a hold when it prints a line for it.
- **The check**: `scripts/openspec-queue.sh --check`, a new mode of the same script that prints no queue and fails
  on a misplaced hold. `task spec:check` runs it (D9, D10).

## Premises

Each premise is measured. The inventory's sections are cited as `inventory.md:<line>`.

- **P1. The queue reads only first lines.** The script hands each task to its classifier one line at a time, using
  `grep -nE '^[[:space:]]*- \[( |~)\]'` (`scripts/openspec-queue.sh:182`; `inventory.md:43`). The OpenSpec CLI hands
  it no task text (`inventory.md:34`).
- **P2. Nothing fails on a hold below the first line.** At `b91c60d`, the queue printed `ok`, `--strict` exited 0
  and `openspec validate --all --strict` exited 0. At `a7dbf9d`, the nested bullet was not shown and validate exited 0
  (`inventory.md:218`, `:288`). No test of the queue exists (`inventory.md:76`).
- **P3. The rule never says "first line".** The protocol (`.agents/protocol.md:24-26`), the `AGENTS.md` row (`:107`)
  and the reviewer contract (`:159-160`) say "on the unticked task it stops". The first-line wording appears only in
  four archived `tasks.md` headers, which split between "first line" and "first words" (`inventory.md:542`).
- **P4. Holds below a first line are common.** Two in one day (`inventory.md:198-321`). PR #93 at `c758aaf5`,
  written under a header that says only `says "Hold:"`, has 21 holds in its 38 open tasks, and 18 of them sit below
  the first line (`inventory.md:137`, `:570`). Today the queue prints 4 lines for #93 (`inventory.md:581`).
- **P5. First-line holds lose what they wait for.** The queue prints at most 104 bytes of a line (`:179`). All three
  of #93's first-line holds start at columns 105-113, so the queue never prints what any of them waits for
  (`inventory.md:182-196`). Markdown lint caps a line at 120 characters (`.markdownlint.yaml:9-10`), and 43 of #93's 57
  first lines are already 110 characters or longer.
- **P6. The task shapes are few.** Across the 296 archived tasks and #93's 57:
  - every task is a `-` checkbox line at column 0, and every one starts with a task number (`N.N`, optionally
    followed by letters);
  - continuation lines are indented 6, 8 or 10 spaces, and nested plain bullets 6 or 8;
  - there are no tabs, no fenced code and no blank line inside a block (`inventory.md:82-128`, `:130-139`).

  #93's task numbers are measured: 52 `N.N`, 3 `N.Na`, 2 `N.Nb`.
- **P7. No task mentions `Hold:` except as a hold.** In the archive, `Hold:` appears on 8 first lines (all holds) and
  in 5 headers. In #93 at `c758aaf5` it appears 22 times: the 21 holds and the header on line 3, which is outside every
  task. Measured with `grep -n 'Hold:'`.
- **P8. Reading the whole task shows every hold.** The behaviour of D1-D4 was modelled in a scratch script (local
  only, `/tmp/semengine-76/tools/model_block_queue.py`) and run on the real files:

  | File | What the queue would print |
  | --- | --- |
  | `b91c60d` | `BLOCKED  L62    3.1 Hold: until PR #48 merges — ...` |
  | `50b4a0c` and `b0bb713` | the same line at L53 and L55 |
  | `a7dbf9d` | ``BLOCKED  L258   3.6 Hold: `message` waits for task 2.8. ...``, plus the existing 7.1 line |
  | #93 at `c758aaf5` | 22 lines in place of 4: 21 `BLOCKED` lines, each with what it waits for (for example `3.12b Hold: task 3.12a.`), and the existing `RED L207` |

  The model is a measurement of the proposed behaviour, not its implementation.
- **P9. The queue can be tested without npm.** A stand-in CLI at `<root>/node_modules/.bin/openspec` drives the
  script (`inventory.md:452`). The script needs `python3`, and CI has it: `task doctor` printed
  `ok    python3       3.12.3` in main's run 37624638249 on `00b4a81`. The pin's fixture test for the script has nine
  cases and was never ported (`inventory.md:459`). Run through the stand-in, all nine give the pin's expected result on
  the unchanged script.
- **P10. Helpers exist, and the names are free.** `copyScript` and `fakeBin` are at
  `internal/harness/contract/repo_test.go:155` and `:132`. The package-level names this design adds
  (`TestSpecQueueHolds`, `TestSpecQueueFirstLineCaveats`, `TestSpecQueueCheck`, `TestSpecCheckWiring`,
  `runSpecQueue`, `specQueueRun`) are not declared in the package at the base, nor in #93's four files of the package
  at `c758aaf5`. Measured by listing every top-level `func`, `var`, `const` and `type` in both.
- **P11. The candidate homes for a failing form, and what each costs** (`inventory.md:322-431`):
  - `spec:check` runs one command, and nothing pins it.
  - `docs:check` reads every `tasks.md`, active and archived. Its only extension point is a JavaScript rule, and the
    repository has no JavaScript (`inventory.md:337`, `:349`).
  - `lint` already runs a shell guard as an extra command, but reads only Go (`inventory.md:365`).
  - A new `verify` step forces no change to `mergegate_test.go` or the `merge-gate` spec, although `AGENTS.md:85`
    says otherwise (`inventory.md:387-397`).
  - A Go guard runs under `task test:unit`, which preflight does not select for an OpenSpec-only diff
    (`inventory.md:416`).
  - `--strict` fails on correct holds and passes the regression (`inventory.md:61`).
- **P12. Readers outside the repository read the queue's words, not its format.** The owner's global `pickup` and
  `handoff` skills find the task by the word `queue` and read `BLOCKED` lines as prose (`inventory.md:682`).
- **P13. The repository's own rule about rows.** `AGENTS.md:69` — "A change that can turn a "review only" row into a
  failing command should."
- **P14. A hold outside every task can fail at no cost to #93.** A check that reports a `Hold:` below a file's first
  `##` heading and in no task's block (ticked tasks included) was modelled (local only,
  `/tmp/semengine-76/tools/model_orphan_check.py`). On 15 real files (the 11 archived `tasks.md`, #93 at `c758aaf5`,
  `a7dbf9d`, `b91c60d` and this change's `tasks.md`) it reports nothing. On a planted file it reports the hold in a
  paragraph after the task list, and passes the header's sentence and a hold in a ticked task. The pre-owner review
  measured the same.
- **P15. The first-line check's reach.** A check that also reports a `Hold:` that is in an open task's block but not
  on a task's first line was modelled (local only, `/tmp/semengine-76/tools/model_placement_check.py`). On the same
  files plus `b0bb713` it reports 20 lines: #93's 18 (`inventory.md:569-580`), `a7dbf9d:258` and `b91c60d:62`.
  `b0bb713`, the archive and this change pass.
- **P16. The cut is in bytes.** `/bin/bash` 3.2.57, the only bash on the measuring machine, printed the first two
  bytes of `—a` for `printf "%.2s"` (`342 200`), splitting the character. Bash 5.2.37, in the official `bash:5.2`
  image, printed the same under `LC_ALL=C` and `LC_ALL=C.UTF-8`. The runner's own bash (ubuntu-24.04) was not
  measured.
- **P17. A dated stand-in makes the test depend on the date.** With `lastModified` in the stand-in's JSON, the
  script printed `x 0/1 [stale: 36d]` (`:134-147`); without it, no staleness note.

## Rulings and constraints in force

The owner's rulings first, quoted exactly; then the orchestrator's brief, which is not a ruling.

- **R1**, the owner on #76, comment 6035540964 (2026-10-07), the sentence this design answers: "The issue's own
  recommendation is the starting point for the design; whether the failing form lives in `spec:check` or a new
  `verify` step is the design's call."
- **R2**, the owner's style rule (2026-10-07), as given in the brief: "clean, idiomatic, pragmatic code;
  simple-yet-detailed beats complex. A design that adds a language or a tool where a short script would do has to
  justify it."
- **R3**, the owner's documentation rule, as given in the brief: "every document is written for a working developer.
  Define coined terms where they are first used; no jargon, no marketing."

The orchestrator's brief (Claude session, PR #113); these are instructions, not rulings:

- **B1**: "Your §4 has shown that `docs:check` and `lint` are also candidate homes, so frame them as well."
- **B2**: "`design.md` carries a per-ruling conformance table (each ruling above, then where the design meets it,
  `file:line` in the drafts). The cross-agent reviewer (Codex) expects one."
- **B3**: "The inventory's three open questions (does "the unticked task it stops" mean its first line; first line or
  first words, given the 104-character cut at `openspec-queue.sh:179`; which merges first, this change or #93) go into
  the design as owner questions. Write each in plain language with the recommendation first, then the reasons, then
  what the other answer costs, including what it costs the owner and the #93 session (18 holds to move). Include the
  option that makes the queue read the whole task block, so a continuation-line hold becomes visible rather than
  failing. Weigh it fairly against failing, along with doing nothing." The whole-task option, O2, came from B3.
- **B4**: "The rules table requires that the design says which of this change and #93 merges first. Do not ask or
  contact the #93 session; just state the cost of each order."

## Options

Each option is weighed by what it adds, who has to learn something new, and what it costs PR #93. O3 is the issue's
recommendation, the starting point under R1. Revision 2 adds O10, which the pre-owner review found missing.

| Option | What it adds | Who learns what | Cost to #93 | Verdict |
| --- | --- | --- | --- | --- |
| O1. Do nothing | nothing | every author keeps a rule no command checks | none; its 18 holds stay unseen | rejected |
| O2. The queue reads the whole open task and shows each hold from `Hold:` on | a change to the script's reading loop; two Go tests | nothing new: write `Hold:` anywhere in the task | none; after it merges `main`, its queue shows all 21 holds | the answer if the owner wants no failing command |
| O10. O2, and `spec:check` fails on a hold outside every task | O2, plus a `--check` mode in the same script, one line in `spec:check`, two Go tests | that a hold belongs in a task, which a failing `spec:check` tells them | none: the check finds nothing in #93 (P14) | **recommended** |
| O3. O2, and `spec:check` fails on a hold outside every task or below a task's first line | O10, plus a placement rule in the check, two Go tests | that a hold goes on the task's first line | move 18 holds before its next green CI, if this lands first | the answer if the owner wants the first-line check; designed in full below |
| O4. As O3, in a new `verify` step | as O3, plus a new task and the step list in four documents | as O3 | as O3 | rejected |
| O5. Fail in `docs:check` with a custom rule | a JavaScript rule module, the repository's first JavaScript file | as O3, plus JavaScript for maintainers | as O3 | rejected |
| O6. Fail as an extra command of `lint` | as O3, wired into the Go linter | as O3, and nobody looks for a Markdown check under `lint` | as O3 | rejected |
| O7. Fail in a Go guard under `task test:unit` | a second parser of "open task", in Go, beside the queue's | as O3 | as O3 | rejected |
| O8. O2, and a check that fails only on placement | O3 without the outside-every-task rule | as O3 | as O3 | rejected |
| O9. Run the existing `--strict` in `verify` | one line | none | `--strict` fails every held change, #93 included | rejected |

The reasons, one paragraph each:

- **O1, do nothing.** No code cost. But the rule stays the kind `AGENTS.md` calls the class that drifted (P13): two
  slips in one day, and 18 more in the change now in flight (P4). Every slip costs a review finding, and every
  pickup of #93 reads a held change as open work.
- **O2, the queue reads the whole open task.** This extends the one surface that already reads holds, instead of
  adding a second reader. Once the queue reads every line of an open task, where a hold sits in the task stops
  mattering, and the line printed for a hold starts at `Hold:`, which fixes P5. It adds no language and no tool (R2).
  Its costs: nothing fails, and a hold is seen only by whoever runs `task spec:queue`, which neither `task verify`
  nor CI runs (`AGENTS.md:54`). A hold written outside every task, such as in a heading or a paragraph (the case
  `.agents/protocol.md:26` names), is still neither shown nor failed.
- **O10, O2 and a check for holds outside every task.** The check is the cheap failing form the first revision
  missed. It fails `spec:check` on a hold written below the first `##` heading and in no task's block (P14). The
  sentence that explains holds, above the first `##`, is left alone, as is a hold in a ticked task (history). It
  reads with the same block reader as O2, so `tasks.md` keeps one reader. On the real files it finds nothing, so #93
  pays nothing, and it gives #76 its failing command and `AGENTS.md:69` its letter. Its costs over O2: a flag and its
  message, one line in `Taskfile.yml:113`, two Go tests (D9, D10), and three descriptions (`AGENTS.md:35`, preflight
  `SKILL.md:31`, the task's `desc`). It guards a case named by the protocol but measured 0 times in 15 files, and a
  hold inside a task is still shown only by the queue. A local `task verify` also comes to need python3: its first
  step, `spec:check`, runs the check, which reads the change list through the script's python3 parse (`:106`). No
  `verify` step needs python3 today (`scripts/test-integration.sh:91` falls back to it only for a clock). CI is
  unaffected, since `task doctor` already fails without python3 (`ci.yml:52`, `scripts/doctor.sh:41-42`); task 3.1
  updates doctor's message (D8).
- **O3, the first-line check.** The issue's recommendation, built on O10 so that it includes O2's whole-task reader
  and printing from `Hold:` on. The check also fails on a hold in an open task's block that is not on the task's first
  line. On the real files it reports 20 lines: #93's 18, `a7dbf9d:258` and `b91c60d:62` (P15). Over O10 it costs a
  placement rule every author must learn, and #93's 18 holds. Each is a re-wrap of a task whose first line is already
  close to the 120-character limit (P5). Its gain over O10 is a single place to look for a hold, on the first line.
  The section "If the owner chooses the first-line check" designs it in full.
- **O4, a new `verify` step.** The same check as O3, with more wiring: a new task, `scripts/verify.sh:12-13`, and the
  step list in `AGENTS.md:52-54`, preflight `SKILL.md:47` (and a new row), and `docs/testing.md:115`. It would also
  leave `AGENTS.md:85` untrue (P11). Nothing gained over running the check inside `spec:check`.
- **O5, a `docs:check` custom rule.** It already reads every `tasks.md`, but its only extension point is a
  JavaScript module. That would be the repository's first JavaScript file and a second parser of "open task", in a
  third language, where a few lines of the existing script do (R2).
- **O6, extra commands in `lint`.** The cheapest wiring (one line, nothing pinned). But `lint` checks Go, and its
  descriptions (`AGENTS.md:41`, preflight `SKILL.md:37`) say so. A Markdown task check there is where nobody would
  look for it.
- **O7, a Go guard.** It fits the structural-guard pattern (`testtext_test.go:20`). But it is a second interpreter of
  the same fact, "which lines belong to an open task", beside the queue's; the architect contract asks for one home
  per interpreted fact (`.agents/contracts/semengine-architect.md:266`). And it does not run for an OpenSpec-only
  diff under preflight (P11).
- **O8, placement only.** O3 without the outside-every-task rule: it pays #93's cost and still misses a hold in a
  paragraph.
- **O9, `--strict`.** It fails on every shown caveat, so it fails a correctly placed hold, and it passes the
  regression (P11).

## Recommendation

O10. The queue reads every line of an open task and prints each hold from `Hold:` on (O2), and `task spec:check`
fails on a hold written outside every task. Both are held by Go tests whose fixtures include PR #73's regression and
PR #48's nested bullet. There is no first-line rule, so this departs from the issue's own failing form and is put to
the owner as Q1.

## Questions for the owner

### Q1. Keep a failing command or not?

**Recommended answer: keep one, but make it fail on a hold written outside every task, not on a hold below a task's
first line (O10).** The queue then shows every hold written inside a task, wherever it sits.

What the recommended answer costs:

- A hold written inside a task is never failed on. It is shown only to whoever runs `task spec:queue`, and neither
  `task verify` nor CI runs it (`AGENTS.md:54`).
- The failing command guards a case named by the protocol (`.agents/protocol.md:26`) but measured 0 times in 15 real
  files (P14).
- It adds a flag, its message, one `Taskfile.yml` line and two tests (D9, D10).
- A local `task verify` comes to need python3, which only `task spec:queue` needs today. CI already checks for it
  (`ci.yml:52`), and `task doctor`'s message says so (D8, task 3.1).

Reasons:

- The protocol, `AGENTS.md` and the reviewer contract say "on the unticked task it stops", not "on its first line"
  (P3). #93's 18 holds below a first line break the rule only under the first-line reading this question asks about;
  under the documents' own words they are on their tasks.
- #76 gets a failing command, and `AGENTS.md:69` is met by its letter, while #93 pays nothing (P14).
- Nobody learns where in a task a hold goes, nothing has to move, and the queue starts printing what each hold waits
  for (P5, P8).

What the other answers cost:

- **No failing command (O2 alone).** It saves the flag, the `Taskfile.yml` line and two tests. But #76's request for a
  failing command goes unmet, a hold outside every task stays review only, and `AGENTS.md:69` is not met by its
  letter. #93 pays nothing. To take it, drop task 2.3, the check's parts of tasks 2.4-2.6 and 3.1, D9, D10 and the
  delta's third requirement. Task 3.1 then leaves out D8's clauses about the check:
  - the protocol's "and `task spec:check` fails on it";
  - in `AGENTS.md:107`'s enforcement text, everything but the `TestSpecQueueHolds` sentence; its review-only list ends
    "a hold outside every task" in place of "a hold above the file's first `##` heading";
  - the texts for `AGENTS.md:35`, preflight `SKILL.md:31` and `Taskfile.yml:111`, which stay as they are;
  - the `scripts/doctor.sh` change, since `task verify` then does not need python3.

  That is an edit, not a design round.
- **The first-line check (O3).** The owner also rules on Q2. Every author learns the placement rule. If this change
  lands first, the #93 session moves 18 holds onto their tasks' first lines before its next green CI after it merges
  `main`; under "first words" (Q2), all 21 move. The section "If the owner chooses the first-line check" designs it
  in full, so this answer needs no new design round.

### Q2. Only if the owner chooses the first-line check: first line, or first words?

**Recommended answer, if Q1 is answered with the first-line check: the first line, anywhere on it.** O3 prints each
hold from `Hold:` on, so where on the line the hold starts no longer affects what the queue shows.

Reasons: the 104-byte cut (`:179`) is what made position matter (P5), and O3 removes it for holds. "First line"
moves 18 of #93's holds; "first words" moves all 21.

What "first words" costs: every held task is reworded to read `N.N Hold: ...` first, the #93 session rewrites 21
holds instead of 18, and the `tasks.md` headers must state the stricter rule. For the owner, it is one more ruling
with no gain in what the queue shows.

### Q3. Which merges first, this change or #93?

**Recommended answer: this change may merge first, and neither waits for the other.** Under O10 the check finds
nothing in #93 (P14), so the order costs #93 nothing.

What each order costs, under O10 or O2 alone:

- **This change first.** #93 merges `origin/main` as usual. Its `spec:check` passes, and its queue starts showing all
  21 holds, which is 22 lines in place of 4 (P8). No edit for the #93 session.
- **#93 first.** #93 archives its `tasks.md` in its last commit (the queue does not read the archive). This change
  then merges `origin/main` and runs `task verify` again. No cost to either side beyond that.

Under O3, the order matters:

- **This change first.** #93 must move 18 holds (21 under "first words") before its next green CI after it merges
  `main`.
- **#93 first.** #93 pays nothing, because its tasks are all ticked by its archive commit. This change waits for #93's
  merge, which has no date (38 of #93's 57 tasks are open), and the owner's queue comment asks for the two to run
  alongside each other.

## Decisions

Each decision states what a caller can observe. The developer chooses the mechanism, within D5.

- **D1. What the queue reads.** For each open task of an in-flight change, the queue reads the task's whole block
  (Terms). Ticked tasks, and text in no open task's block, are not searched for holds by the queue. The change list
  still comes from `openspec list --json` (`:98`), and the file read is still `openspec/changes/<name>/tasks.md`
  (`:152`).
- **D2. What it prints for a hold.** One `BLOCKED` line per open task whose block contains a hold. The line is
  numbered where the first hold in the block is written. Its text is the task number, then the block's text from
  `Hold:` onward, joined across lines, cut at 104 bytes as today (P16).
  - The class name stays `BLOCKED`, so readers that look for that word (P12) need no change.
  - When the first line's own label is `BLOCKED`, it is not printed a second time.
  - When the first line's label is anything else (`HALT`, `RED`, `WONTDO`, `OPEN-Q`), that line is printed too.
  - Within a change, lines are printed in line-number order.
- **D3. What does not change in the queue.**
  - The labels on first lines, and their order (`:80-84`).
  - `[~]` read as `WONTDO` (`:169`).
  - The `ok` line (`:185`).
  - The exit statuses: 0, 1 under `--strict` when anything is printed, 2 when the read is unavailable (`:95-102`).

  Because a hold line is a printed line, `--strict` now exits 1 on the `b91c60d` regression; nothing runs
  `--strict` (P11).
- **D4. What counts as a hold.** The text `Hold:`, matched with its case. Other words on continuation lines are not
  read (`hold:` in lowercase, `on hold`, `blocked on`): the other words stay first-line only, as the issue scopes them
  out. A task that mentions `Hold:` in passing reads as held; that is loud and visible, and P7 measured no such task.
- **D5. No new language or tool (R2).** The change stays inside `scripts/openspec-queue.sh` and uses only what it
  already runs: bash, grep, sed and python3. The tests are Go, in the package that already tests this repository's
  scripts, and use its helpers (P10).
- **D6. The tests.** A new file, `internal/harness/contract/specqueue_test.go`, holds `TestSpecQueueHolds`,
  `TestSpecQueueFirstLineCaveats`, `TestSpecQueueCheck` and `TestSpecCheckWiring`.
  - The harness `runSpecQueue` copies the script into a throwaway root with `copyScript`. It writes a stand-in
    `node_modules/.bin/openspec` that answers `list --json` as the case says (one change, none, a non-zero exit, or
    text that is not JSON) and refuses any other call (exit 3 with a message, as `fakeGH` does at
    `mergecheck_test.go:20-47`). It plants `openspec/changes/<name>/tasks.md`, runs the script with bash and the given
    flags, and returns standard output, standard error and the exit status apart.
  - The stand-in's JSON has no `lastModified`, so the script prints no staleness note and the output does not depend
    on the date (P17).
  - Assertions name the label, the line number and the start of the text. Expected values are written by hand from
    the fixture, never computed by parsing it.
- **D7. A new capability, `spec-queue`.**
  - `merge-gate` covers what must hold before a merge: flake defences, the review check and the runner. The queue
    gates nothing, and the check is part of OpenSpec validation.
  - `harness-boundaries` covers rules over this repository's code and tests. The queue is OpenSpec tooling.
  - The other nine capabilities describe runtime and harness packages.
  - No current spec names `spec:queue` (`inventory.md:525-538`).

  The delta adds three requirements: holds anywhere in an open task (new behaviour), the first-line labels (today's
  behaviour, now under test), and the check. The capability's Purpose is written in the archive commit, as
  `mutation-check` did.
- **D8. Documents.**
  - `.agents/protocol.md:24-26`. From: "A hold is written on the unticked task it stops, as `Hold:` followed by what
    it waits for (an issue or PR number, or the owner's ruling). The queue reads unticked task lines only: a hold in a
    heading or a paragraph does not show." To: "A hold is written in the unticked task it stops, on any of its lines,
    as `Hold:` followed by what it waits for (an issue or PR number, or the owner's ruling). The queue reads every
    line of an unticked task and prints each hold from `Hold:` on. A hold in a heading or a paragraph outside every
    task does not show, and `task spec:check` fails on it."
  - `AGENTS.md:107`, the rule and its enforcement:
    - **Rule.** From: "Every OpenSpec task can be ticked in or before the archive commit; a hold is written as
      `Hold:` on the unticked task it stops". To: "... a hold is written as `Hold:` in the unticked task it stops, on
      any of its lines".
    - **Enforced by.** From: "review only; `task spec:queue`, run in the claim's worktree, displays a hold written
      this way and fails nothing". To: "`task spec:check` fails on a `Hold:` written outside every task
      (`scripts/openspec-queue.sh --check`; `TestSpecQueueCheck`, `TestSpecCheckWiring`). `TestSpecQueueHolds`
      (`task test:unit`) holds that `task spec:queue` shows every `Hold:` in an unticked task, on any line, with what
      it waits for. Review only: that every task can be ticked by the archive; a hold spelled another way; a hold above
      the file's first `##` heading".
    - **Canonical home.** Gains "`spec-queue` spec".
  - The three descriptions of `spec:check` also name the check:
    - `AGENTS.md:35`. From: `task spec:check   # strict OpenSpec validation`. To:
      `task spec:check   # strict OpenSpec validation; fails on a Hold: outside every task`.
    - Preflight `SKILL.md:31`. From: "Validates all OpenSpec changes and specs strictly". To: "Validates all OpenSpec
      changes and specs strictly, and fails on a `Hold:` written outside every task".
    - `Taskfile.yml:111`, the `desc`. From: "Validate all OpenSpec changes and specs strictly". To: "Validate all
      OpenSpec changes and specs strictly; fail on a Hold: outside every task".
  - `scripts/doctor.sh:40-42`. The comment adds that `task spec:check` runs the script with `--check`, so
    `task verify` needs python3 too. The message, from "not found (task spec:queue needs it)", to "not found
    (task spec:check, task verify and task spec:queue need it)".
  - `docs/testing.md:338-341`: the list of scripts tested from Go gains the queue and its tests.
  - The script's header comment says it reads whole open tasks for holds and what `--check` does.
  - At the archive, `docs/repository-map.md` gets the change's row and lists the `spec-queue` spec as present.
  - Unchanged, because they stay true: the reviewer contract (`:159-160`, "on the unticked task it stops"),
    `AGENTS.md:36`, preflight `SKILL.md:32`, `docs/repository-map.md:82`, and the archived `tasks.md` headers (history).
  - **Not touched: `AGENTS.md:85`.** Its overstatement concerns the `verify` step list, which this design does not
    change (`inventory.md:391`). It is left for a separate fix.
- **D9. The check.** `scripts/openspec-queue.sh --check` reads the same in-flight changes as the queue. It reports
  each line that contains `Hold:`, lies below the file's first `##` heading, and lies in no task's block, ticked
  tasks included.
  - For each such line it prints `<path>:<line>: Hold: outside every task; task spec:queue cannot show it. Put it in
    the task it stops.`, then exits 1.
  - With nothing to report it prints `holds: ok (<n> tasks.md read)` and exits 0, so a check that read nothing says so,
    as `scripts/cleanup-roots-check.sh:24` does.
  - When the change list cannot be read (`openspec list --json` fails, or prints text that is not JSON), it prints the
    cause on standard error, starting `queue unavailable:` as the queue does (`:95-123`), prints no `holds: ok`, and
    exits 2.
  - Plain awk would avoid python3: an eight-line awk prototype (local only, `/tmp/semengine-76/tools/orphan_check.awk`)
    matches P14's model on the same 16 files, reading `openspec/changes/*/tasks.md`, which reaches the directories
    `openspec list --json` names (measured: every one but `archive`, even an empty one), and awk already runs in
    `verify` (`scripts/cover-check.sh`). It is not taken, to keep one reader of which changes are in flight; it would
    keep python3 out of `task verify`, and the owner may prefer it under R2.
  - It prints no queue.
- **D10. `spec:check` runs it.** `Taskfile.yml`'s `spec:check` gains a second command,
  `scripts/openspec-queue.sh --check`, after `openspec validate`. `TestSpecCheckWiring` holds those two commands and
  their order, with the command lines as constants in the test, as `TestLedgerCheckWiring` does for `ledger:check`
  (`ledgercheck_test.go:14-32`). `verify`'s step list does not change.

## Tests

`TestSpecQueueHolds`, one planted `tasks.md` per case. Each case maps to a scenario of the delta's first requirement.

| Case | Planted | Required output |
| --- | --- | --- |
| H1 | task 3.1 of `b91c60d`, lines 55-65 verbatim, then a `- [ ] 3.2` line | `BLOCKED` at the line of `Hold:` (the block's eighth line), text from `3.1 Hold: until PR #48 merges`; nothing else for the change |
| H2 | task 3.6's first line from `a7dbf9d`, then its nested bullets of lines 253-259 | `BLOCKED` at the `- Hold:` bullet, text from ``3.6 Hold: `message` waits for task 2.8`` |
| H3 | a first line ending in `Hold:`, then a continuation line `task 3.12a.`, indented 6 spaces | one `BLOCKED` line at the first line whose text contains `Hold: task 3.12a.` |
| H4 | task 3.1 of `b0bb713` (hold on the first line) | exactly one `BLOCKED` line, at the first line |
| H5 | open task 2.1 with no hold, then open task 2.2 with a hold on its second line | one `BLOCKED` line at 2.2's second line, text from `2.2 Hold:` |
| H6 | ticked task `- [x] 1.2` with a hold on its second line | no `BLOCKED` line; the `ok` line |
| H7 | a hold in the paragraph under the title, and a hold in a column-0 paragraph after the list and a blank line | no `BLOCKED` line |
| H8 | `hold: until #12` (lowercase) and `blocked on #12` on an open task's second line | no line for the task |
| H9 | a `- [~]` task with a hold on its second line | a `WONTDO` line at the first line and a `BLOCKED` line at the second |
| H10 | an open task whose first line says `failing` and whose second line holds | a `RED` line and a `BLOCKED` line, in line order |
| H11 | H1's file, run with `--strict`, then without | exit 1, then exit 0 |

`TestSpecQueueFirstLineCaveats` carries the nine cases of the pin's unported fixture test, P1-P9 in the delta's second
requirement, with the pin's texts word for word (SemStreams `8b99efe9`,
`scripts/openspec-queue_fixture_test.sh:79-113`). All nine pass on the unchanged script (P9), so the test shows that
the new reading loop leaves today's labels alone.

`TestSpecQueueCheck`, run with `--check`, one planted file per case. Each maps to a scenario of the third
requirement:

| Case | Planted | Required |
| --- | --- | --- |
| C1 | a column-0 paragraph `Hold: task 1.1 waits for #12.` after the task list and a blank line, below `## 1. Work` | exit 1; output names the file's path and that line |
| C2 | a heading `## 3. Hold: waits for #12` below the first `##` | exit 1; output names that line |
| C3 | the sentence that explains holds, above the first `##`, as line 3 of every recent `tasks.md` | exit 0; `holds: ok (1 tasks.md read)` |
| C4 | a hold on the second line of a ticked task | exit 0 |
| C5 | task 3.1 of `b91c60d` (a hold in an open task's block) | exit 0 |
| C6 | no in-flight change (the stand-in lists none) | exit 0; `holds: ok (0 tasks.md read)` |
| C7 | the stand-in's `list --json` exits 1; then, in a second run, it prints `not json` | exit 2 each time; standard error starts `queue unavailable:`; standard output has no `holds: ok` |

`TestSpecCheckWiring` reads `Taskfile.yml` and requires `spec:check` to run exactly
`npx --no-install openspec validate --all --strict --no-interactive`, then `scripts/openspec-queue.sh --check`. Its
planted Taskfiles: the check line removed; the two lines swapped; `--check` dropped from the second line. Each must
fail naming the task and the command.

**Shown able to fail.** The tests are written first and run against the unchanged script and `Taskfile.yml`:

- In `TestSpecQueueHolds`, H1, H2, H3, H5, H9, H10 and H11 fail there, because the base script reads first lines only.
  H4, H6, H7 and H8 pass there, as they should: they hold what must not change or must not be reported.
- `TestSpecQueueCheck` fails in every case, because the base script refuses `--check` (`unknown argument`, exit 2).
  C7 fails too: its exit status is 2, but standard error says `unknown argument`, not `queue unavailable:` (measured
  with a stand-in on the base script).
- `TestSpecCheckWiring` fails, because `spec:check` runs one command.

That run is recorded. Then come the wrong changes, made by hand in the new script or `Taskfile.yml`
(`task mutate:check` takes only Go source files, `docs/testing.md:142`). Each names the case that must catch it:

- read first lines only (H1);
- end the block at the first nested bullet (H2);
- let a block run into the next task (H5);
- read ticked tasks' blocks (H6);
- match `Hold:` without regard to case (H8, through its lowercase `hold:`);
- take the text from the hold's own line only (H3);
- drop the hold line when the first line has another label (H10);
- drop `-i` from the `still open` match at `:84` (`TestSpecQueueFirstLineCaveats`, the `**STILL OPEN**` case);
- report a hold above the first `##` (C3);
- report a hold in a ticked task's block (C4);
- report a hold in an open task's block (C5);
- print the finding but exit 0 (C1);
- treat an unreadable change list as empty, printing `holds: ok (0 tasks.md read)` and exiting 0 (C7);
- remove the check's line from `spec:check` (`TestSpecCheckWiring`).

The baseline, wrong-change and restored runs are recorded on the pull request.

**Generated checks.** Examples are enough (`docs/testing.md`, "Decide whether generated checks are needed"). The
input shapes are few and were measured on every real `tasks.md` (P6), and each gets its own case. The two laws, "each
open task with a hold gives exactly one hold line, and nothing else gives one" and "the check fails exactly on a hold
below the first `##` in no task's block", are covered by H1-H7 and C1-C6. A generated grammar of Markdown would invent
shapes no file in the repository has.

**On real files.** The new script is also run by hand, and the runs are recorded on the pull request:

- the queue on #93's `tasks.md` at `c758aaf5`, on `b91c60d` and on `a7dbf9d`. Its output must match P8: 22 lines for
  #93, with the line numbers of `inventory.md:569-580`.
- `--check` on the 15 files of P14. It must report nothing.

## Invariants

Each holds for every `tasks.md`, and each is cited to the delta (`specs/spec-queue/spec.md`):

- **I1.** Each open task whose block contains a hold gives exactly one `BLOCKED` hold line, numbered at the first
  hold in the block. Source: requirement "A hold on any line of an open task" (`spec.md:5`), and the scenarios "Hold
  on the first line" and "The hold belongs to the next task".
- **I2.** No hold line comes from a ticked task's block or from text outside every open task's block. Source: the same
  requirement, and the scenarios "Ticked task" and "Hold outside any task".
- **I3.** For a task with no hold, the queue prints what it printed before this change. Source: requirement "Labels on
  a task's first line" (`spec.md:80`).
- **I4.** `--check` exits 1 exactly when some line below the first `##` contains a hold and lies in no task's block,
  and then names every such line. Source: requirement "A hold outside every task fails spec:check" (`spec.md:146`).

## Adopter seam, after the change

The adopter is someone writing a `tasks.md`, or reading the queue.

- **What they must know.** Two facts: write `Hold:` in the task it stops, on any line; and do not write `Hold:` in an
  open task unless it is a hold.
- **If they know neither.** A hold written anywhere in its task is shown with what it waits for. A hold written outside
  every task fails `task spec:check`, naming the file and line.
- **Where they find out.** In the queue's own output, or in a failing `task spec:check`: an observation, not a rule
  to predict.

What is left: a hold spelled another way, or written above the first `##` heading, is neither shown nor failed (see
"What this does not cover").

## What this does not cover

- Holds spelled another way (`blocked until`, lowercase `hold:`). They stay review only, as they would under every
  option.
- A hold written above a file's first `##` heading. That region holds the sentence that explains holds in every recent
  `tasks.md` (P7), so the check leaves it alone; such a hold is neither shown nor failed.
- The queue's other words, which stay first-line only (issue #76).
- The archive, which neither the queue nor the check reads.
- What the queue itself does when `openspec list` is unavailable (exit 2, `:95-102`). This is today's behaviour; the
  delta specifies it for `--check` only.
- How the runner's own bash (ubuntu-24.04) counts the 104-byte cut. Bash 3.2.57 and the official `bash:5.2` image
  (5.2.37) count bytes (P16); the runner's bash was not measured.
- `AGENTS.md:85` (D8).
- The change's id. The drafts used `hold-first-line-check`, which named the rejected O3 and tripped the queue's
  word list (`hold` is a word there; measured on this change's own task 5.1, since reworded). It was renamed to
  `spec-queue-holds` before the first content commit; the word list does not match `holds` (the patterns of
  `:80-84`).

## If the owner chooses the first-line check (O3)

This section is the design for a Q1 answer of "the first-line check", so that answer needs no new design round. It
adds to O10; everything above stays.

- **Q2, conditionally.** The first line, anywhere on it (Q2's recommended answer).
- **The check, extended (issue #76, step 2).** `--check` also reports a line below the first `##` that contains
  `Hold:`, lies in an open task's block, and is not that task's first line. For such a line it prints
  `<path>:<line>: Hold: below the first line of task <n>; put it on the task's first line.`, then exits 1. A hold on
  a task's first line, and anything in a ticked task's block, pass. The issue's message said the queue cannot see
  such a hold; under O3 it can, so that clause is dropped. The flag is the same `--check`.
- **`Taskfile.yml`.** No further line: `spec:check` already runs `scripts/openspec-queue.sh --check` (D10).
- **A fourth requirement for the delta**, "A hold below an open task's first line fails spec:check":
  `scripts/openspec-queue.sh --check` SHALL also report each line below the file's first `##` heading that contains a
  hold, lies in an open task's block, and is not that task's first line, printing `<path>:<line>:`, the task's number
  and a message that the hold belongs on the task's first line, and SHALL then exit 1. Its scenarios:
  - **The `b91c60d` regression.** WHEN task 3.1 reads as at `b91c60d`, THEN `--check` exits 1 naming the file's
    path, line 62 of that file, and task 3.1.
  - **The `b0bb713` fix.** WHEN task 3.1 reads as at `b0bb713`, with its hold on its first line, THEN `--check`
    exits 0.
  - **The `a7dbf9d` nested bullet.** WHEN task 3.6 reads as at `a7dbf9d`, THEN `--check` exits 1 naming line 258
    and task 3.6.
  - **A ticked task.** WHEN a ticked task carries a hold on its second line, THEN `--check` exits 0.
- **Tests.**
  - `TestSpecQueueHoldPlacement` runs `--check` on the three fixtures above. `b91c60d` must fail at line 62,
    `a7dbf9d` must fail at line 258, and `b0bb713` must pass.
  - `TestSpecQueueHoldPlacementSensitivity` plants the edges. A hold on an open task's second line fails, and so does
    a hold in a nested bullet; a first-line hold and a hold on a ticked task's second line pass. Each failure must
    name the path, the line and the task's number.
  - Wrong changes: check only the line after the first (the nested-bullet case); report a hold in a ticked task's
    block (the ticked case); report a first-line hold (`b0bb713`).
- **`AGENTS.md:107`.**
  - **Rule:** "Every OpenSpec task can be ticked in or before the archive commit; a hold is written as `Hold:` on the
    first line of the unticked task it stops".
  - **Enforced by:** "`task spec:check` fails on a `Hold:` written outside every task or below a task's first line
    (`scripts/openspec-queue.sh --check`; `TestSpecQueueCheck`, `TestSpecQueueHoldPlacement`,
    `TestSpecCheckWiring`). Review only: that every task can be ticked by the archive; a hold spelled another way; a
    hold above the file's first `##` heading".
  - `.agents/protocol.md:24-26` says "on the first line of the unticked task it stops", and that `task spec:check`
    fails otherwise.
- **The three descriptions of `spec:check`,** in place of D8's:
  - `AGENTS.md:35`:
    `task spec:check   # strict OpenSpec validation; fails on a Hold: outside a task or below its first line`.
  - Preflight `SKILL.md:31`: "Validates all OpenSpec changes and specs strictly, and fails on a `Hold:` written
    outside every task or below a task's first line".
  - `Taskfile.yml:111`, the `desc`: "Validate all OpenSpec changes and specs strictly; fail on a Hold: outside every
    task or below a task's first line".
- **If Q2 is answered "first words".** `--check` also reports an open task's first line that contains `Hold:` but
  where `Hold:` does not follow the task's number straight away (`- [ ] <number> Hold:`, or `- [~] <number> Hold:`).
  - For such a line it prints `<path>:<line>: Hold: not at the start of task <n>; write it straight after the
    task's number.`, then exits 1.
  - The fourth requirement then says "on the task's first line, straight after its number", and gains two
    scenarios. **A first-line hold after other words:** WHEN an open task's first line reads as line 177 of #93's
    `tasks.md` at `c758aaf5` (`- [ ] 3.12b (D) Fail closed at birth ...`, ending `Hold:`), THEN `--check` exits 1
    naming that line and task 3.12b. **A hold straight after the number:** WHEN task 3.1 reads as at `b0bb713`
    (`- [ ] 3.1 Hold: until PR #48 merges ...`), THEN `--check` exits 0.
  - `TestSpecQueueHoldPlacementSensitivity` adds that edge, and the wrong change "accept `Hold:` anywhere on the first
    line" must be caught by it.
  - In `AGENTS.md:107`, the protocol and the three descriptions, "below a task's first line" (or "below its first
    line") reads "anywhere but straight after a task's number"; the rule reads "as `Hold:` straight after the number
    of the unticked task it stops".
  - #93 moves all 21 holds. This change's `tasks.md` header already writes holds straight after the task number.
- **#93.** If this change lands first, the #93 session moves 18 holds onto their tasks' first lines (the lines of
  `inventory.md:569-580`) before its next green CI after it merges `main`. If #93 lands first, nothing.
- **The tasks it adds** to `tasks.md`, after task 2.3, with the hold on each first line:
  - `- [ ] 2.3a Hold: task 1.3. (D) Written first: TestSpecQueueHoldPlacement and its sensitivity test, failing on the
    unchanged script. The run is recorded on this pull request.`
  - `- [ ] 2.3b Hold: task 1.3. (D) --check reports a hold below an open task's first line, as this section says, and
    both tests pass. Committed with 2.4. Gate: task verify.`
  - `- [ ] 2.3c Hold: task 1.3. (D) The three wrong changes of this section, each caught. The runs are recorded on
    this pull request.`

  Task 3.1 then writes this section's texts for `AGENTS.md:107` and `:35`, the protocol, preflight `SKILL.md:31` and
  `Taskfile.yml:111`, instead of D8's.

## Overlap and order

- PR #93 (draft, 98 files at `c758aaf5`) shares no file with this change. It shares the Go package
  `internal/harness/contract`, and none of the names this change adds (P10).
- Its capability delta is to `harness-boundaries`; this change adds `spec-queue`, so the deltas do not overlap.
- Order: this change may merge first, and neither waits (Q3). Whichever merges second merges `origin/main` and runs
  `task verify` again.

## Conformance to the rulings

Each owner ruling and each item of the brief (`design.md:108-133`), where these drafts meet it, and its state. Paths
are files of this change; `spec.md` is `specs/spec-queue/spec.md`.

| Ruling or brief item | Where the drafts meet it | State |
| --- | --- | --- |
| R1 (owner): the issue's recommendation is the starting point; whether the failing form lives in `spec:check` or a new `verify` step is the design's call | The starting point is O3, the issue's recommendation (`design.md:176-181`), designed in full (`:517-590`). The issue's step 1, reading the whole task block, is D1 (`:287-290`). Its step 2, a failing check, lives in `spec:check` (D9 and D10, `:368-387`); a new `verify` step is weighed and rejected (O4, `:182-184`). Its step 4, the harness test with PR #73's regression as the fixture, is D6 and case H1 (`:312-322`, `:395`) | **Departs from the starting point in what fails, and put to the owner.** The recommended check, O10, fails on a hold outside every task, not on a hold below a first line (`design.md:200-205`). The owner chooses in Q1 (`:209-249`), as stated in `proposal.md:5-7`. Implementation waits for the answer (`tasks.md:18-22`, and the hold on tasks 2.1-2.6 at `tasks.md:26`, `:31`, `:34`, `:37`, `:41`, `:44`) |
| R2 (owner): simple code; a new language or tool must be justified | D5: no new language or tool; the change stays inside the script with what it already runs (`design.md:309-311`). O5 is rejected for adding JavaScript (`:185-187`). The tests reuse existing helpers (P10, `:72-76`). `proposal.md:66` | Met |
| R3 (owner): written for a working developer; coined terms defined at first use | "Terms" defines the queue, open task, first and continuation line, block, hold, fail and show, and the check, before any use (`design.md:8-24`). The proposal defines a hold at first use (`proposal.md:11-12`). The delta defines in-flight change, open task, block and hold inside its first requirement (`spec.md:7-12`). The `tasks.md` header says what a hold does (`tasks.md:3-7`) | Met |
| B1 (brief): frame `docs:check` and `lint` as homes too | O5, `docs:check` (`design.md:147`, `:185-187`); O6, `lint` (`:148`, `:188-190`) | Met |
| B2 (brief): a per-ruling conformance table | This table | Met |
| B3 (brief): the three open questions as owner questions, recommendation first, then reasons, then the other answer's cost to the owner and the #93 session; the whole-task option weighed fairly against failing and doing nothing | Q1 (`design.md:209-249`), Q2 (`:251-261`) and Q3 (`:263-281`). Each gives the recommended answer first, then its reasons and its costs, then the other answers' costs, including the owner's extra rulings and #93's 18 holds (21 under "first words"). The whole-task option O2 is weighed beside doing nothing (O1) and every failing form (O10, O3-O9) (`design.md:135-198`) | Met |
| B4 (brief): say which merges first, and the cost of each order, without contacting #93 | Q3 recommends that this change may merge first, and gives each order's cost under O10 or O2 and under O3 (`design.md:263-281`). "Overlap and order" restates it (`design.md:592-598`). Nothing in the drafts asks the #93 session for anything | Met |
