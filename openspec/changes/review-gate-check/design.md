# Design: review-gate-check

Status: **draft, not reviewed, not approved.** It depends on `inventory.md` in this directory, which has not had its
independent review; nothing here stands until that file has `INVENTORY PASS`, this design has passed pre-owner
review, and the owner has accepted it and answered the questions in "Questions for the owner". References such as
"inventory 2.3" are to sections of that file.

## Context

`.agents/protocol.md`, "Cross-agent review", says that on a code pull request the reviews that gate the merge are
done by the other agent: Codex reviews what Claude implemented, and the reverse. Nothing fails when that is skipped
(inventory 1). The owner ruled on 2026-10-02 (issue #64): "agree - we should enforce it for code and can waive it for
docs only", then "merge 65 and yes it should work both ways", then "start 66 once 65 is merged". His stated reason
is the SemStreams record: rules policed by review prose drifted, rules with a command behind them held.

Terms. A *code pull request* changes at least one file that is not Markdown and not under `openspec/`, or changes a
role adapter under `.claude/agents/`. Every other pull request is *documents only*. The *head* is the newest commit
of the pull request's branch. A *review record* is the pull request comment in which the reviewing session posts its
report. A record *carries over* when it names an earlier commit and still counts for the head. The *fake `gh`* is the
stand-in for the GitHub command line that the existing test puts first on `PATH`; it answers from files.

Four facts from the inventory shape everything below.

1. **One login.** The owner and both agents post and commit as `cglusky` (inventory 2.2, 2.3). No field of a pull
   request, a commit or a comment says which agent wrote it. A script can check what was written, never who wrote it.
2. **The heading `Review record` is already used for reviews that are not the gate.** PR #48 has three comments with
   that heading written by the implementing session's own reviewer (inventory 2.3).
3. **The lines are free text.** No description has a `reviewed-by:` line yet. Four pull requests record a review by
   the implementing agent's own reviewer inside the `implemented-by:` line (#44, #55, #56, #59). The word `claude`
   appears in one `implemented-by:` line out of twenty (inventory 2.2).
4. **A check runs only when a run starts.** A review is always written after the push it reviews, and neither a new
   comment nor an edited description starts a run (inventory 2.5).

## Goals and non-goals

Goals: a code pull request with no recorded review by the other agent cannot reach a green `Required`; a
documents-only pull request is untouched; every read that fails or is cut short fails the check; the output says
what to add.

Non-goals: proving who wrote a comment or a commit; reading a verdict; a waiver; a second script, job or required
check; any change to the known-flake check or the up-to-date rule; a general rule engine for pull request text.

## Options for the change as a whole

| Option | What it is | Cost | Why it is or is not chosen |
| --- | --- | --- | --- |
| O0 | Do nothing; the rule stays prose | none | The owner ruled it out on #64 |
| O1 | Extend `scripts/merge-check.sh`: a code pull request needs the two description lines, and they must name the two agents (D1, D2 only) | two more reads; a fixed pair of words in two lines | The floor the owner agreed to. It cannot tell a review of the head from a review of an earlier commit: once the line is written it stays true for every later push |
| O2 | O1, and a review record by the named reviewer that names the head or carries over to it (D3, D4) | two more reads; two fixed lines in the record; the costs under "Declared costs" | **Recommended.** It is the part of the rule most likely to be skipped by omission: PR #48's description today lists five fix commits as "Not yet reviewed", in prose |
| O3 | O2 with an exact comparison of the pull request's diff at two commits, using `git` in a full clone | a full clone in the job, `git` calls in the script, and a fake `git` or real repositories in the tests | Not chosen. The script today reads GitHub through `gh` only, and its test is built on that |
| O4 | GitHub's own pull request reviews | none in the script | Not possible: one login, and GitHub does not let an author approve their own pull request (documented). No pull request here has a GitHub review (inventory 1, 2.3) |
| O5 | A new script, job or required check | a ruleset change, a new job the workflow test must pin | Not chosen: the existing script is the surface the rule's row names (`AGENTS.md:90`), and it already fails closed |

All of O1 to O3 extend the existing script, job, task and spec. O2 is designed below. If the owner rules O1, D3 and
D4, their scenarios, and the comments and comparison reads drop out; nothing else changes.

## Decisions

### D1 Documents only is decided from the complete list of changed files

The script reads the pull request (`repos/{owner}/{repo}/pulls/<n>`: the draft flag, the head commit, the
description, the author's type, and `changed_files`) and every page of `repos/{owner}/{repo}/pulls/<n>/files`.

Each entry gives one name. A renamed file gives two, its new name and its previous name (`previous_filename`). A
deleted file gives its name like any other. A name is a *document name* when it ends in `.md` or starts with
`openspec/`, and does not start with `.claude/agents/`. Every other name is a *code name*. This is the protocol's
definition (`.agents/protocol.md:67-72`), word for word.

| The entries read | Result |
| --- | --- |
| at least one code name | code pull request. This is true however many entries are missing |
| no code name, and exactly as many entries as `changed_files` | documents only; the review check passes and reads nothing more |
| no code name, and a different number of entries | treated as a code pull request; the output gives both numbers and says the list may be incomplete |

Why the last row is not "unavailable". GitHub lists at most 3000 files for a pull request (documented, inventory 9
N2). A documents-only pull request larger than that could never show a complete list, and an "unavailable" exit
would hold it for ever. Treated as code, it has a way to pass: a review. A list that came up short for a passing
reason (a push between the two reads, inventory 2.4) fails once and passes on the re-run or on the run the push
started.

A pull request that changes no file is documents only. Every claim starts that way (PR #67 today).

Renames. Renaming `scripts/x.sh` to `docs/x.md` removes a script, so the previous name counts. GitHub also documents
a `copied` status with a previous name; the script does not look at the status, so a copy counts both names too. That
is stricter than needed and errs towards review.

Three things the definition lets through as documents, kept as written and put to the owner (Q5): any file type
under `openspec/`, where there is no script or Go file today; skill files, which are Markdown; and the contracts,
the protocol and `AGENTS.md`.

### D2 The two lines name the agents with two fixed words

For a code pull request the description must have exactly one line that starts `implemented-by:` and exactly one
that starts `reviewed-by:`. The line starts in the first column; a list item or a mention inside a sentence does not
count (PR #65's description mentions both inside sentences). A carriage return at the end of a line is ignored, since
a description edited in the browser may carry them (inventory 9 N8).

An *agent name* is the word `claude` or the word `codex`, in any letter case, with no letter or digit directly before
or after it. `Codex (orchestration)` and `claude-fable-5-1` name an agent; `opus`, `gpt-6-sol` and `codex2` do not.

| `implemented-by:` names | Author is a bot | `reviewed-by:` must name |
| --- | --- | --- |
| `claude` only | either | `codex` only |
| `codex` only | either | `claude` only |
| both | either | exactly one of them. The protocol says the owner names which; the script cannot read that |
| neither | yes | exactly one of them (a dependency bot's pull request, reviewed by either agent) |
| neither | no | fails: the line must say which agent wrote the commits |

Options considered:

- **Both lines present, nothing more.** `reviewed-by: opus (semengine-reviewer)` on a Claude pull request passes.
  That is the habit on #44, #55, #56 and #59, and it is the case the rule exists to stop. An honest session following
  the old habit would pass, so the check would not be about the other agent at all.
- **Present and different.** `opus` differs from `opus (reviewer)`. It checks nothing.
- **Two fixed words (chosen).** The smallest vocabulary that lets the script tell one agent from the other. The rest
  of each line stays free text, so the model or persona the protocol asks for still fits:
  `implemented-by: claude (opus semengine-developer; fable orchestrating)`.

Other signs of who implemented a pull request exist and are not used: the branch prefix names the claimer, not the
author of each commit; Codex commits carry no trailer; the commit author is always `cglusky` (inventory 2.2). The
protocol names the `implemented-by:` line as the record, and the script reads that one home.

When a line fails, the record is not looked at; the output says which line to add or correct.

### D3 A review record is a comment with a heading and two fixed lines

For the script, a review record is a comment on the pull request's conversation:

- whose first line, after any `#` characters and spaces, starts with `Review record`;
- which has a line that starts `reviewed-by:` and names exactly one agent;
- which has a line that starts `commit:` followed by the 40 characters of a commit id in lower case, with or without
  backticks.

The first line of each kind is the one read. Only records that name the agent in the description's `reviewed-by:`
line are considered. The check passes when one of them names the head. Otherwise only the newest of them is
considered, and the check passes when it carries over (D4). At most one comparison is read.

Why two lines and not the heading alone: the heading is already on same-agent reviews (fact 2), and existing
records name several commits each, as short ids in prose (inventory 2.3). A script that picked one would be
guessing. The `Review request` already gives the head by its 40 characters, so the reviewer copies it.

Why the `reviewed-by:` line is in the record as well as the description: the description is the implementer's text
and the record is the reviewer's. An implementing session's own early review, posted under the same heading, says
`reviewed-by: claude` on a Claude pull request and so never counts.

Not read: review comments on lines of the diff, GitHub's own reviews (none exist), and the record's verdict or kind
(see "What the check does not see").

Option considered and not chosen: put the reviewed commit in the description (`reviewed-by: codex at <id>`) and read
no comment. It needs no format in the record. But the description is the implementer's own text, and moving an id
forward after pushing fixes is the easy slip the check is for.

### D4 A record carries over when no file the pull request changes differs

When the newest record names a commit `R` other than the head `H`, the script reads GitHub's comparison
`compare/R...H`. The record carries over when all four hold:

1. the comparison's `status` is `ahead`, which means `H` contains `R`;
2. the comparison lists fewer than 300 files, which is where GitHub stops without saying so (measured, inventory
   2.4);
3. the list of the pull request's files is complete (D1's count);
4. no name in the comparison, new or previous, is a name in the pull request's list.

Otherwise the check fails and says which: the files that differ, that the head does not contain the recorded commit
(a rebase or a force-push, which the protocol forbids), or that a list may be incomplete so the record must name the
head.

This is what a script can tell exactly from one read: whether any file the pull request changes has different
content at `R` and at `H`. It covers the two ordinary cases the protocol describes (`.agents/protocol.md:87-90`). A
merge of `main` that changed other files carries over. A later commit that changes one of the pull request's files,
or a merge that brought in `main`'s change to one of them, does not, and the output names the file. Measured on
PR #48: the comparison of the commit its `Review request` names with the head then current lists nine files, all of
them files the pull request changes, which is the "Not yet reviewed" list in its description.

Two unusual commits are not seen; both are in "What the check does not see".

In the Land order the record that passes at merge time is normally the last one. Ticking a task and the archive
commit both change files the pull request changes, so an earlier record does not carry over them, and the archive
check's record, which names the archive commit, is the one that counts. That turns one more sentence of the protocol
into a command: "no later content commit bypasses the archive/spec-sync check" (`.agents/protocol.md:58-59`).

Option considered and not chosen: three more comparisons (the pull request's diff at `R`, and what `main` changed
between the two merge bases) would close both. Each has its own 300-file limit, so a pull request the size of #48
could then never carry a record over.

### D5 A draft is told, not failed; marking it ready starts a run

On a draft the script makes the same reads and prints the same findings, says they do not fail a draft, and does not
fail because of them. A failed read still fails. GitHub refuses to merge a draft (documented; assumption A4).

The workflow lists the activity types of its `pull_request` trigger: `opened`, `synchronize`, `reopened` and
`ready_for_review`. The first three are GitHub's defaults (`ci.yml:7` lists none today). The fourth starts a run when
a draft is marked ready, and that run applies the check to the head that can now merge. `TestCIWorkflowPinned` fails
when any of the four is missing, so the trigger cannot be dropped without a red test.

This matches the Land step as written: review, fixes, re-review, archive, the check of the archive, *then* undraft,
then CI green. The record and the `reviewed-by:` line exist before the pull request is marked ready, and the run
that marking starts sees them.

Option considered: fail drafts too. The script and workflow would be simpler. But the review always comes after the
push, so every push to a code pull request would show a red `Required` until a record named it, for the whole life
of a draft such as #48, and going green would always need a failed job re-run by hand. A check that is red by
default teaches everyone to look past it.

`edited` is not listed. It would start the whole workflow, `Verify` included, on every edit of a description, and
cancel a run in progress (`ci.yml:13`).

### D6 What fails, and what is printed

A finding is something the pull request lacks. It fails a pull request that is not a draft (exit 1, with the other
checks' results) and is printed for a draft. A read that fails, or returns something other than what was asked for,
ends the script at once naming the read (exit 2), as the known-flake check does through `read_gh`
(`scripts/merge-check.sh:56-67`). The spec says only "non-zero"; the two numbers are the script's existing habit.

| Condition | Result | What the output says |
| --- | --- | --- |
| documents only | pass | documents only, with the number of files |
| code pull request | goes on | code pull request, with at least one code name |
| a line missing, written twice, or naming the wrong agents | finding | which line, and the line to add, for example `reviewed-by: codex (<model or persona>)` |
| no review record by the named reviewer | finding | the heading and the two lines a record must carry, and how many comments had the heading without them |
| the newest record does not carry over | finding | the recorded commit, the head, and the files that differ, or that the head does not contain it |
| a list may be cut short | finding | both numbers; that the record must name the head |
| any finding on a pull request that is not a draft | exit 1 | also: a comment or an edited description starts no run, so re-run this job once they are in place |
| a failed or misshapen read; a failed `jq` step | exit 2 | "unavailable", the read, and what `gh` printed |

Two steps would read as a pass if `jq` failed and left an empty result: sorting the files into code names (empty
means documents only) and matching the comparison against the pull request's names (empty means it carries over).
Both check `jq`'s exit status, as `merge-check.sh:136-140` does for the closing references.

The review check runs before the known-flake check, after the up-to-date rule. The known-flake check ends the script
at several places; run first, a review finding is printed in the same run as a flake finding, so one run tells the
developer everything to fix.

The script must not depend on how `gh` prints several pages. `gh` 2.97 joins them into one list; other versions
print one list per page (inventory 9 N5). Both are handled, and the test serves both.

### D7 Where it runs, and under which permissions

- **CI, pull-request run:** the `merge-check` job, unchanged except for its step's name.
- **Locally:** `task merge:check -- <n>`, which is a pull-request run. Nothing changes in how it is called.
- **Push to `main`:** the check does not apply and the script says so. It reads no pull request.

No new permission. The pull request and its files need `pull-requests: read`; its conversation comments need
`issues: read` or `pull-requests: read`; the comparison needs `contents: read`. The job has exactly these three
(`ci.yml:74-77`), and the spec's two permission scenarios stay as they are. This is documented per endpoint and not
yet measured under the job's token (assumption A2). Because a draft makes the same reads, this pull request's own
runs measure it before the archive; a refused read fails the job with "unavailable" and the read's name.

Option considered for the push run: look up the pull request a pushed commit came from and check it after the fact.
It would turn `main` red after an unreviewed merge and stop nothing. Not designed; put to the owner (Q7).

### D8 Pull requests open when this lands, and this pull request

Two pull requests are open, both drafts (inventory 3).

**PR #48.** Its first run after this merges uses the new script: a pull-request run tests the pull request merged
with `main`, and #48 has to merge `main` in any case to settle the four files named under "Order". As a draft it
does not fail. Its output will list what it lacks: its `implemented-by:` line names no agent,
it has no `reviewed-by:` line, and its three `Review record` comments are the implementing session's own. Before it
is marked ready it needs the two lines and a record by Codex that names its head or carries over. Its open `Review
request` to Codex predates the two record lines; a note on #48 tells both sessions (task 5.5).

**This pull request (#67)** is a code pull request by Claude sessions, checked by its own copy of the script. Its
review of record is Codex's, and the `Review request` gives the two lines the record must carry. It is the first use
of the format.

**Order.** #67 merges first. #48 changes four files this change also edits (`AGENTS.md`, `docs/testing.md`, and the
developer and reviewer contracts) and is about half way through its tasks (22 of 45 at its last stop point). The
next time PR #48 merges `main` it resolves those four, and by the rule each of them then needs re-review there.

**Dependabot** opens pull requests that are not drafts, so each is red on `Merge check` until a session has reviewed
it, posted the record, added both lines, and re-run the job.

### D9 Tests

Driven as today: a copy of the script run from a throwaway directory with the fake `gh` first on `PATH`
(`internal/harness/contract/mergecheck_test.go`). The fake gains four answers (the pull request, its files, its
comments, the comparison) and keeps its rule: a call whose arguments are not the read the script must make exits 3.
For the comparison it answers only for the exact pair of commits the case expects, so a case cannot pass by
comparing the wrong pair.

- **`TestMergeCheckReview`** has one case per scenario of "Cross-agent review check". Each case writes the pull
  request, the file list, the comments and the comparison by hand and states the expected exit and the expected
  words. The expectation comes from the scenario, never from running the script.
- **The existing cases** of `TestMergeCheckKnownFlake` and `TestMergeCheckUpToDateRule` keep their expectations. The
  shared healthy state becomes a documents-only pull request, so the review check passes in them, and one assertion
  shows the new reads were made.
- **Two shapes of several pages:** the fake serves the 101-file list once as one list and once as two lists back to
  back; both give the same result.
- **`TestCIWorkflowPinned`** gains the trigger rule, and `TestCIWorkflowPinnedSensitivity` gains one planted workflow
  per scenario of "A run when a pull request is marked ready". Its clean fixture starts on `push` only today
  (`mergegate_test.go:138`) and gains the trigger. The workflow test does not decode the key `on` today
  (`mergegate_test.go:264-268`); how the YAML library decodes that key is measured first, because some decoders read
  an unquoted `on` as a boolean.

**Shown able to fail.**

- Written first: each new case is run against the script as it is today and seen to fail for its stated reason. The
  cases that expect a failure meet exit 0; the cases that expect a pass lack the words they require.
- The experiment of `docs/testing.md`, "Show that the test can fail", applies: this check allows or forbids a merge.
  One wrong change at a time, each with its baseline, wrong-change and restored runs recorded on the pull request:
  drop the `.claude/agents/` exception; ignore the previous name of a renamed file; stop after the first page; take a
  short list as documents only; accept a reviewer the implementer line names; count a record whatever its
  `reviewed-by:` line says; carry every record over; carry none over; fail a draft; pass a pull request that is not
  a draft as if it were one; let a failed `jq` step read as documents only. A wrong change the tests let through is
  reported as a survivor.
- Against real GitHub, read-only, with the expectation taken by hand from the pull request's page: #65 (documents
  only), #63 (code, no `reviewed-by:` line), #14 (code, by a bot), #48 (code, draft), #67. This is the check on the
  fake's fidelity that the fake cannot give.
- On this pull request: marked ready once before any record exists, the run that starts fails `Merge check` with the
  findings; then returned to draft.

**Generated checks: examples are enough.** `docs/testing.md` asks for the decision when an input has many
interacting cases. Three inputs here have them.

- *Names.* Three conditions on a path (ends in `.md`; starts with `openspec/`; starts with `.claude/agents/`). The
  last two cannot both hold, which leaves six combinations; the scenario "Names near the rule" lists each with the
  near misses (`README.MD`, `docs/a.md.txt`, `openspecs/`, `docs/openspec/`, a nested `.claude/agents/`). The rule
  over a list is "one code name decides", checked with the code name as the only entry, among documents, on the
  second page, as a previous name and as a deleted file.
- *Lines.* A fixed prefix at the start of a line and two fixed words. The cases are listed: absent, present, twice,
  not at the start, carriage returns, and a line naming neither, one, the other, both, or a longer word.
- *Records.* The one order-dependent rule is "the newest record is the one carried over", checked with two records.

There are no sizes or arithmetic except three limits, each an explicit example at its edge: 100 and 101 files, 300
files in a comparison, and a count that does not match. Each case costs a process, and a generator would need a
second implementation of the same path rules as its oracle. So no generated check is written.

### D10 Documents changed with the code

- `.agents/protocol.md`, "Cross-agent review" and "Verification": the two agent words, the record's two lines, what
  the script checks and what stays review only, and that a comment or an edit starts no run.
- `.agents/contracts/semengine-reviewer.md`: a review record on a code pull request carries the heading and the two
  lines. This is where the reviewing agent learns the format.
- `.agents/contracts/semengine-developer.md`, step 6: the two description lines and the request's commit id.
- `AGENTS.md`: row 90's "Enforced by" names the command and the tests and lists what stays review only; the command
  comment (line 45) and the Merge gate (121 to 128) follow.
- `.agents/skills/semengine-preflight/SKILL.md`, `docs/repository-map.md`, `docs/testing.md`, `Taskfile.yml`'s
  description, the workflow's comment and the script's header: one line each.
- The Purpose of `openspec/specs/merge-gate/spec.md` says the capability covers "the parts of it that defend against
  flaky tests". A delta cannot change a Purpose, so it is edited in the archive commit.

## What the check does not see

These parts of the rule stay review only, and `AGENTS.md` row 90 will say so.

- **Who wrote anything.** One login. The script cannot tell that the record was posted by the other agent, that the
  `implemented-by:` line is true, or that a comment was edited later. It stops omission, not a false statement.
- **The verdict and the kind of review.** A record that says `CHANGES REQUESTED` and names the head passes, if the
  implementer writes `reviewed-by:` and marks the pull request ready without changing anything. One record, the
  archive check's, satisfies the script even if no implementation review was recorded. Reading a verdict would need
  a third fixed line, and would hold a pull request on which the owner overruled the reviewer.
- **That the owner named the reviewer** when both agents wrote commits.
- **The rest of the record's content:** that it is the reviewer's report as written, what was run, and that evidence
  came from the tree and from CI.
- **That the reviewer did not write on the branch**, and that a disagreement went to the owner.
- **A change the pull request dropped.** If a commit after `R` returns a file to exactly what `main` has, that file
  leaves the pull request's list and the record carries over, though the diff is smaller than the one reviewed.
- **A merge that kept the pull request's version of a file `main` also changed.** The file is the same at `R` and
  `H`, so it is not in the comparison, though the merge now undoes `main`'s change to it.
- **A pull request that edits the script, the workflow or their tests** runs its own copies (inventory 2.5). Such a
  change is a code pull request and is reviewed as a change to the merge gate.
- **The moment after marking ready.** Until the new run registers, the earlier green `Required` is the newest result
  on that head. Merging in that moment needs two steps of Land skipped: waiting for CI, and
  `task merge:check -- <n>`, which is a pull-request run on a pull request that is no longer a draft.

## Invariants and their spec homes

Each holds for every pull request and every set of answers, and each is a sentence of the requirement "Cross-agent
review check" in this change's delta.

| Invariant | Spec home |
| --- | --- |
| I1 One code name among the entries read makes a code pull request, wherever it is in the list and whichever of the two names it is | "Documents only", fourth and fifth sentences |
| I2 Documents only is concluded only from a list that has exactly the number of entries GitHub reports | "Documents only", sixth and seventh sentences |
| I3 No failed, short or misshapen read, and no failed sorting or comparing step, ends in documents only, a carried record or a pass | "Reads" |
| I4 A code pull request that is not a draft passes only with both lines as required and a record by the reviewing agent that names the head or carries over | first paragraph; "The two lines"; "The review record" |
| I5 A record carries over only when all four conditions of D4 hold | "Carrying over" |
| I6 On a draft a finding never changes the exit status, and a failed read always does | "Drafts"; "Reads" |
| I7 Nothing but the kind of run changes which checks apply | first paragraph |

## What a session has to know after this change

The four questions of the adopter seam inventory, for the people of inventory 7.

1. **What must they know?** An implementing session: write `claude` or `codex` in the two lines. A reviewing
   session: start the record with `Review record` and give the two lines. Both: a comment or an edit starts no run.
   That is three facts, one more than the contract's limit of two before it counts as a design finding. The third
   belongs to GitHub and already holds for the known-flake check.
2. **What happens if they do nothing?** A draft goes on as before, with the findings in its log. Marked ready, the
   run fails. Nothing merges silently.
3. **Where do they find out?** From the failed required check, whose output gives the exact line or record to add.
   That is the level of a typed error at run time. The reviewer contract and the `Review request` carry the record's
   format to the reviewing agent before the failure.
4. **What should they have to know?** Nothing. The gap that remains is the fixed text: two words and two lines.
   GitHub offers no identity per agent that would let the script observe the reviewer instead of reading a
   declaration (fact 1).

## Assumptions that are documented and not yet measured

| # | Behaviour | The design rests on it | Where it is measured |
| --- | --- | --- | --- |
| A1 | Listing `ready_for_review` starts a run when a draft is marked ready, and the list is read from the pull request's own copy of the workflow | yes (D5) | task 4.3, on this pull request |
| A2 | The job's token can read a pull request, its files, its comments and a comparison | yes (D7) | tasks 4.2 and 4.4, on this pull request's draft runs |
| A3 | The runner's `gh` prints several pages in one of the two shapes the script handles | yes (D6) | task 4.2 shows one page on the runner, and task 4.1 shows two pages with the local `gh` on #48. Two pages on the runner are first seen on #48's own run after this merges; until then the fake's two shapes stand for it. That half stays open |
| A4 | GitHub refuses to merge a draft | yes (D5) | not planned; open as A15 on #42, and the known-flake exemption already rests on it |
| A5 | A comparison stops at 300 files here as it does on the repository measured | yes (D4) | not planned; nothing here is that large. If the limit were lower, a record could carry over on a list that was cut. Stays open |
| A6 | A pull request's file list stops at 3000 | only for the reason behind D1's third row | not planned |

## Declared costs

- Three more reads on every pull-request run (the pull request, its files, its comments), one more when a record
  names an earlier commit, and one extra page per hundred files or comments. A GitHub failure fails the job.
- One more full CI run each time a pull request is marked ready.
- A code pull request marked ready before its record exists is red until the failed job is re-run.
- A review record needs two fixed lines. A slip is corrected by the reviewing session, which the implementer cannot
  start.
- Every code pull request's two lines must contain `claude` or `codex`. PR #48's line must be reworded. The owner's
  own instructions for Claude sessions name models only.
- No waiver. If the other agent cannot review, a code pull request waits (Q4).
- After a merge of `main` that changes 300 or more files, or a file the pull request changes, the record must be
  renewed. The second is the rule; the first is the price of the limit.
- Each Dependabot pull request, weekly for three ecosystems, waits for a recorded review and for a session to add
  the lines. If Dependabot rewrites its description, the lines are added again (inventory 9 N7).
- Test time. The two existing tests run the script 28 times in about 6 s (measured: 5.06 s and 0.98 s), so about
  0.2 s a run. About 45 more runs add about 10 s to each execution of the unit tests, and `task verify` executes
  them seven times: about 70 s more per `task verify`, by this estimate. Task 2.6 measures it. The cases share no
  state, so the developer may run them in parallel if the measured cost calls for it.

## New surfaces and who uses them

| Surface | Present consumer |
| --- | --- |
| the review check in `scripts/merge-check.sh` | the `merge-check` job; `task merge:check`; the Land step |
| the words `claude` and `codex` in the two lines | the script |
| the lines `reviewed-by:` and `commit:` in a review record | the script |
| `ready_for_review` in the workflow's trigger | the review check on a pull request just marked ready |

Nothing is added for later use. No option, flag, label or variable is added.

## The adoption sweep

Not a pattern. The change adds no primitive meant for reuse: it applies the shape the known-flake check already has
(`read_gh`, a list that may be cut short, a failed `jq` never a pass; inventory 5) to one more rule in the same
script. No other script reads GitHub.

## Questions for the owner

Each is his call. The recommendation is the design above.

- **Q1 How much a code pull request must show.** (a) The two lines only (O1). (b) The two lines and a review record
  that names the head or carries over (O2). Recommended: (b). Its cost is the record's two fixed lines and the
  renewals under "Declared costs".
- **Q2 How the lines name the agents.** (a) Present only. (b) The words `claude` and `codex`. Recommended: (b);
  under (a) a same-agent review passes. Cost: existing habits of writing a model name alone fail until reworded.
- **Q3 Drafts.** (a) Told, not failed, and marking ready starts a run. (b) Failed like any other. Recommended: (a).
- **Q4 No waiver.** As for a known flake, nothing exempts a code pull request, a dependency bump included. One login
  means a waiver comment could not be told from an agent's. Recommended: none.
- **Q5 The reading of "documents only".** Kept as the protocol has it. It counts as documents any file under
  `openspec/`, skill files, the contracts, the protocol and `AGENTS.md`. Say if any of these should count as code.
- **Q6 A pull request written by hand** by neither agent and not by a bot fails until its `implemented-by:` line
  names an agent. No such pull request exists, so no third word is designed. Say if one is wanted.
- **Q7 The push run.** Not applied. Say if a check after the fact on `main` is wanted.

## Premises

| Premise | Measurement |
| --- | --- |
| Nothing reads a description, a file list, comments or a comparison today | inventory 1: the five reads and the empty search |
| One login for the owner and both agents | inventory 2.3: 36 comments and every agent commit by `cglusky` |
| The heading `Review record` is on same-agent reviews | inventory 2.3: comments 5939311446, 5942129322, 5950922103 on #48 |
| Records name commits as short ids, several each | inventory 2.3 |
| No description has a `reviewed-by:` line; four record a same-agent review in `implemented-by:` | inventory 2.2 |
| Codex commits carry no trailer | inventory 2.2: four commits on #59 |
| A bot's pull request is reported as type `Bot` | inventory 2.4: #14 |
| `gh pr view --json files` stops at 100 silently; the paginated read returns every entry | inventory 2.4: 100 of 113, and 101 of 101 |
| A comparison reports `status`, lists files, accepts full ids and answers 404 for an unknown one | inventory 2.4 |
| A comparison stops at 300 files and says nothing | inventory 2.4 (another repository) |
| An edited description starts no run; a re-run reads GitHub again | inventory 2.5 (A11 and A5 on #42) |
| The job has three read permissions and the test pins them | `ci.yml:74-77`; `mergegate_test.go:124` |
| The workflow test does not read the trigger | `mergegate_test.go:264-268` |
| #48 is the only other open pull request and overlaps in four documents | inventory 3 |
