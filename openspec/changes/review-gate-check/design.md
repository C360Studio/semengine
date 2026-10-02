# Design: review-gate-check

Status: **revision 3; `DESIGN REVIEW PASS` in round 2 of three (PR #67, comment 5957614491); accepted by the owner
(#66, comment 5959125319), who answered the three questions at the end with the recommendation each time.**
Revision 2 had `DESIGN CHANGES REQUESTED` in round 1 (PR #67, comment 5957401025); this revision answers its eight
findings, and its recommendation is smaller than revision 2's. It rests on `inventory.md` in this directory:
revision 2, which has `INVENTORY PASS` (PR #67, comment 5956907408), sha256
`725b11e7a060092d72c73b5fc98216f85f826f0764d5531053497993fa8fbe8f`, committed at `5861fa5`. That file is the
accepted inventory and is not repeated here; "inventory 2.3" is its section 2.3 and `inventory.md:212` is one of its
lines.

## Context

`.agents/protocol.md`, "Cross-agent review" (`:63-94`), says that on a code pull request the reviews that gate the
merge are done by the other agent: Codex reviews what Claude implemented, and the reverse. Nothing fails when that
is skipped (inventory 1). The owner ruled on 2026-10-02 (issue #64): "agree - we should enforce it for code and can
waive it for docs only", then "merge 65 and yes it should work both ways". His reason is the SemStreams record:
rules policed by review prose drifted, and rules with a command behind them held. His standing rule for the work
itself is "Write the minimum code that solves the stated problem".

Issue #66 is titled "fail a code pull request that has no cross-agent review recorded". It states the floor: "At
the least `reviewed-by:` in the pull request body, beside `implemented-by:`." It asks the design to say whether the
check can also tell that the review read the head, "or a commit the head differs from only by a merge of `main`
that touched none of the pull request's files, and what it does when it cannot tell". Options O4 and O5 answer that.

Terms used below.

- A *code pull request* changes at least one file that is not Markdown and not under `openspec/`, or changes a role
  adapter under `.claude/agents/`. Every other pull request is *documents only* (`.agents/protocol.md:67-72`).
- The *head* is the newest commit of the pull request's branch.
- A *review record* is the pull request comment in which the reviewing session posts its report.
- A *finding* is something the pull request lacks: a line, or an agent's name in a line.
- The *fake `gh`* is the stand-in for the GitHub command line that the existing test puts first on `PATH`; it
  answers from files and refuses any call it does not expect.

## What the inventory found, and what this design does with each

| Finding | Inventory | What the design does |
| --- | --- | --- |
| The owner and both agents use one GitHub login; no field says which agent wrote a comment, a commit or a description | 2.3 (`:209-211`); 6, Ownership | The check reads what is written and never claims to know who wrote it. Every option below is limited by this, and "What the check does not see" says how |
| GitHub's own home for a required review exists: the `pull_request` rule of ruleset `24272345` on `main`. It asks for no review, and `scripts/merge-check.sh` receives it at `:73` and drops it at `:77` | 1 (`:83-94`); 4; 6 | Framed as option O1 and not extended; the reasons are there. The rule and the script's handling of it stay as they are |
| PR #48 adds a rule that a PASS or `APPROVE` names the CI run and the commit, in the reviewer contract and an `AGENTS.md` row | 3 (`:373-388`); 6, Owners | No second rule about what a record holds is added. This design reads no record and edits neither contract. The order of the two merges is under "PR #48" |
| PR #48 moves the `AGENTS.md` lines this change must edit | 3 (`:389-392`) | "PR #48" says which merges first and what each order costs |
| Codex's two review records differ in shape: heading on line 1 or line 3, one commit or a range, and other 40-character ids beside the one read | 2.3 (`:220-231`, `:240-244`); 9, N12 | No comment is read, under the recommendation or under O4 and O5. O6, which reads them, is not chosen for this reason |
| The heading `Review record` is on 19 comments; 17 are an implementing session's own reviewer | 2.3 (`:212-219`) | The heading decides nothing. It is the evidence for D2 |
| Most `implemented-by:` values are model names; `fable` is in 17 of 20 and in no tracked file; the adapters map a role to a model, not a model to an agent | 2.2 (`:173-179`); 6, Catalogs | No model name is read and no catalog is added. The line names the agent in a word (D2) |
| An agent's own name is written for the reviewer and, in `implemented-by:`, only on #59 and #1 | 2.2 (`:181-183`) | D2 changes that habit, and Q1 tells the owner what it costs him |
| The one `reviewed-by:` line, on #48, holds a seven-character id and a comment id, and stood unchanged while a later record asked for changes | 2.3 (`:248-255`) | Under the recommendation the script reads only the agent's name in it, so that line passes; the spec uses it as its example of what is not read. O4 and O5 read more, and "PR #48 through each option" walks it |
| #48's implementer listed two of the three files `main` also changed | 2.3 (`:259-265`); 7 (`:476-478`) | Stays a step done by hand and checked in review. O4 would work it out from GitHub's comparison |
| A check runs only when a run starts; a comment or an edited description starts none; the review comes after the push | 2.5; 7 (`:480-485`) | D3: a draft is told and not failed, and marking it ready starts a run |
| The closest existing shape is in the same script: `read_gh`, a list that may be cut short, a failed `jq` never a pass | 5 | Adopted (D4) |
| The fake `gh` refuses a read it does not know; its answer for the rules on `main` holds no `pull_request` rule | 3 (`:339`, `:344-346`) | D6: the fake gains two answers. Its rules answer is left alone, because the script does not read that rule |
| A third home for "a review happened": OpenSpec tasks ticked with a comment id | 2.3 (`:256-258`) | Untouched. It is what records how far a review got, since the recommended check does not |
| Dependabot opens pull requests that are not drafts and writes its own description | 7 (`:466-467`); 9, N7 | D2, "Declared costs" and Q1 |

## Goals and non-goals

Goals: a code pull request that is ready to merge cannot reach a green `Required` unless its description names the
other agent as its reviewer; a documents-only pull request is untouched; a read that fails or is cut short fails the
check; the output says what to write.

Non-goals: proving who wrote anything; reading a review record, its verdict or the commit it read; a waiver; a
second script, job or required check; any change to the known-flake check or the up-to-date rule; a catalog of
models or agents.

## Options

What is common to O2 to O6. `scripts/merge-check.sh` reads the pull request and its complete list of changed
files, and sorts it into code or documents only (D1). A draft is told what it lacks and not failed, and the workflow
starts a run when a draft is marked ready (D3). A failed or short read fails the check (D4). These parts are needed
whichever lines are asked for, because every option has to know which pull requests it applies to, and none can
pass before the review it asks for exists.

What limits O2 to O6 alike. With one login, each option makes the implementing session state something in the
description and checks that the statement is there. None can check that it is true.

| Option | What a caller sees | What it does not stop | Cost | Chosen? |
| --- | --- | --- | --- | --- |
| O0 Do nothing | Nothing fails; the rule stays prose | everything | none now | No. The owner ruled it out on #64 |
| O1 Extend the existing owner on GitHub: raise `required_approving_review_count` in ruleset `24272345` | GitHub's merge button refuses a pull request with no approving review | see "O1 in plain words" | one ruleset setting; no script | No |
| O2 The floor of #66: a code pull request must have a `reviewed-by:` line | `Merge check` fails a ready code pull request with no such line and prints the line to add | a review by the implementing agent's own reviewer: `reviewed-by: opus (semengine-reviewer)` on a Claude pull request passes | two more reads per run; 17 scenarios; about 21 more script runs per test execution | No |
| O3 O2, and both lines name the agents: the reviewer is not the implementer | also fails when the line names the implementing agent, or no agent | a review that is old, partial or rejecting. The line, once written, satisfies the check for the rest of the pull request's life | the word `claude` or `codex` in two lines; 22 scenarios; about 29 runs | **Recommended** |
| O4 O3, and the `reviewed-by:` line holds the 40-character commit the review read, which must be the head or carry over | also fails when one of the pull request's files changed after the commit the line names, and names the file | a line that names the head. It passes whether the review of the head approved, asked for changes, or does not exist; and the failure it prints asks for "the head's commit" in the line | one more read when that commit is not the head; the carry-over rule; a 300-file limit that cannot be measured here (inventory 9, N1); a 40-character id pasted by hand; 6 more scenarios; about 12 more runs; more protocol text | No |
| O5 O4, and the line also holds the verdict word `APPROVE` | also fails when the line names a commit and does not say it was approved | whether a record says so, or exists. One login: the word is the implementer's statement | O4's cost; one more fixed word, and a choice between the two words reviews end with today, `APPROVE` and `PASS` (`inventory.md:245-247`); 2 more scenarios | No |
| O6 Revision 1 of this design: O3, and the script reads the comments for a `Review record` that carries two fixed lines, `reviewed-by:` and `commit:` | the same as O4, and also fails when no comment has that form | a comment in that form posted by the implementing session | one more paged read; 47 scenarios; a fixed form in the reviewer's comment | No. Both of Codex's real records lack the form (`inventory.md:220-231`), and the implementer cannot correct them |
| O7 A new script, job or required check | a second check on the pull request | as the option it carries | a ruleset change and a new job for the workflow test to pin | No. #66 names `scripts/merge-check.sh`, which already reads GitHub and fails closed |

**O1 in plain words.** With one login, GitHub's rule cannot express this rule. Every pull request an agent opens is
authored by `cglusky`, the login that would also have to approve it, and GitHub documents that an author cannot
approve their own pull request (inventory 6, Ownership; not measured: inventory 9, N10). If that holds, a count of 1
stops every merge, documents-only ones included: the rule's parameters, as read (`inventory.md:83-94`), hold nothing
about which files a pull request changes. If it does not hold, an approval says only that `cglusky` approved, never
which agent. The rule becomes the right owner on the day each agent has its own GitHub account: then GitHub can see
who approved, and its two settings that are off today (`dismiss_stale_reviews_on_push`,
`require_last_push_approval`) do the work O4 does. Creating accounts is the owner's to decide and is not proposed.

**PR #48 through each option.** #48 at head `6bf9809`, as read for the inventory at 2026-10-02T16:32Z
(`inventory.md:220-231`, `:248-255`, `:277`): Codex approved six commits at `0a86a9a`; the head is 12 commits
later; Codex's newest record read the head and says `CHANGES REQUESTED`; the description says "reviewed-by: codex
… APPROVE at `0a86a9a` … full-diff review (hold 7.1) pending". Suppose it were marked ready.

| Option | Result for #48 as it stands | What a session would have to write to pass |
| --- | --- | --- |
| O2 | passes | nothing |
| O3 | fails: `implemented-by:` names no agent | add `claude` to that line. Nothing about the review changes |
| O4 | fails: the same, and the line holds no 40-character id | add `claude`, and a commit. `0a86a9a…` in full fails, because 29 of #48's files differ since (`inventory.md:277`). `6bf9809…` passes: Codex did read that commit, and asked for changes |
| O5 | fails: the same, and no verdict word beside a commit that carries over | nothing true passes until Codex approves the head or a commit that carries over to it |

So the state #48 showed, a head that the other agent has read and rejected, is stopped only by O5, and only for a
session that writes the line truthfully.

**What each step above the floor stops, and what it costs.**

| Step | The failure it stops that the step below lets through | Evidence that the failure happens | Added cost |
| --- | --- | --- | --- |
| O2 to O3: the other agent | A session records its own reviewer in `reviewed-by:` and the check passes, though the rule is about the other agent | 17 of the 19 `Review record` comments are an implementing session's own reviewer; three `implemented-by:` lines name a reviewer of the implementing agent (#12, #55 and #56, among the four at `inventory.md:254-255`; the 17 are at `:212-219`) | the word `claude` or `codex` in two lines; five scenarios; about eight runs |
| O3 to O4: the commit | The line is left naming an older commit after one of the pull request's files changed, and nobody notices | #48's line has named a commit that is not the head since it was written (`inventory.md:248-255`); no pull request has merged in that state, and the rule is one day old | the comparison read, the carry-over rule and its unmeasured limit; six scenarios; about twelve runs; a 40-character id |
| O4 to O5: the verdict word | The line names the head after a review that asked for changes | #48 at 16:32Z, had its session followed "the commit the newest record read" | one more fixed word and the choice of word; two scenarios |
| O5 to O6: the record's form | A session writes the line though no record comment exists. That is a false statement, not an omission | none in the 20 pull requests | a form the reviewing session must follow and the implementer cannot fix; a paged read of the comments; about 17 more scenarios |

**Recommendation: O3.** It is the least that puts a command behind what the rule is about, the other agent, and
that is the problem #66 states: a code pull request with no cross-agent review recorded. O4 alone adds less than
revision 2 of this design claimed: it catches a line left on an older commit, not a head that was rejected or never
read, and it costs the comparison, its limit and a third more test cases. O5 would close that for a truthful
session, at O4's whole cost and one more word, and still proves nothing about the record. No pull request has yet
merged on a stale line, so by the owner's minimum rule O4 and O5 wait until one does or until he asks for them in
Q1.

What O3 leaves to prose, in plain words: which commit the review read, whether it approved, and whether anything
changed afterwards. Those stay with the protocol, the review tasks of each change (`inventory.md:256-258`) and the
owner's word for the merge, and the rule's row in `AGENTS.md` will list them as review only.

If the owner rules O2, D2's scenarios drop out. If he rules O4 or O5, the design, the delta and the tasks are
extended and re-reviewed before any code (task 1.3); what O4 adds is described in its row above and in revision 2 of
this file.

## Decisions

Each decision says what a caller can observe. The scenarios named are those of the requirement "Cross-agent review
check" in this change's `merge-gate` delta; each is one case of the test in D6.

### D1 Documents only is decided from the complete list of changed files

The script reads the pull request (its draft flag, description, author type and number of changed files) and every
page of its list of changed files. Each entry gives one name; a renamed file gives two, the new and the previous. A
name is a *document name* when it ends in `.md` or starts with `openspec/`, and does not start with
`.claude/agents/`. Every other name is a *code name*. That is the protocol's definition
(`.agents/protocol.md:67-72`), with "a Markdown file" read as a name that ends in `.md`, as every tracked Markdown
file does (`inventory.md:125-126`).

- At least one code name: a code pull request. The output lists the code names, up to ten.
- No code name: documents only. The check passes and reads nothing more. A pull request that changes no file is
  documents only, as every claim is at first.
- The entries read are not as many as GitHub reports: the script stops with "unavailable" and both numbers. This is
  the script's existing answer to a list that may be incomplete (`scripts/merge-check.sh:111-114`).

GitHub lists at most 3000 files for a pull request (documented; inventory 9, N2), so a larger pull request cannot
pass. None is near that: the largest so far has 114.

The definition counts as documents any file under `openspec/` (a script there too), skill files, the contracts, the
protocol and `AGENTS.md` (inventory 2.1). The scenario "Names near the rule" pins that reading, so a change to it
is a change to the protocol and to this spec together.

Scenarios: "Documents only", "One code file among documents", "Names near the rule", "Renamed file", "Code file on
the second page", "File list incomplete", "No changed file".

### D2 The two lines name the agents with two fixed words

A code pull request's description has exactly one line that starts `reviewed-by:` and, unless GitHub reports its
author as a bot, exactly one that starts `implemented-by:`. An *agent name* is the word `claude` or `codex`, in any
letter case, with no letter or digit directly before or after it: `Codex (orchestration)` and `claude-fable-5-1`
name an agent; `opus`, `gpt-6-sol` and `codex2` do not.

| `implemented-by:` names | `reviewed-by:` must name |
| --- | --- |
| `claude` only | `codex` only |
| `codex` only | `claude` only |
| both | exactly one of them. The protocol says the owner names which (`.agents/protocol.md:72`); the script cannot read that |
| neither, and the author is a user | fails: the line must say which agent wrote the commits. Q3 asks the owner about a pull request he writes himself |
| not read: the author is a bot | exactly one of them (`.agents/protocol.md:72-74`: either agent's reviewer) |

Nothing else in either line is read, so the model or persona the protocol asks for still fits:
`implemented-by: claude (opus semengine-developer; fable orchestrating)`.

The third row is kept, with its scenario, because without it a pull request both agents wrote, as #59 was
(`inventory.md:160-165`), would have no truthful line that passes.

Why the line and not something the script could observe. The protocol names the `implemented-by:` line as the record
of who wrote the commits (`.agents/protocol.md:64-65`), so the script reads that one home. Three other signs exist
and none is used (inventory 2.2). The branch prefix records who claimed the work, not who wrote each commit: #59 is
on a `codex/` branch and has Claude commits, and #12 has no prefix. Commit trailers are on Claude's commits only,
and not on its merges. A model name cannot be turned into an agent without a catalog that does not exist. Reading
the branch prefix would make a second home for the fact, tied to the first by a naming habit.

Scenarios: "The other agent is named", "No reviewed-by line", "Implementer line missing or naming no agent", "Same
agent implements and reviews", "Reviewer line names no agent, both, or a longer word", "Both agents implemented",
"Pull request by a bot", "Line not written once at the start of a line", "Carriage returns". Whether a description
edited in the browser carries carriage returns is not established (inventory 9, N8), so the design does not rest on
it either way: the scenario requires that they change nothing.

### D3 A draft is told, not failed; marking it ready starts a run

On a draft the script makes the same reads and prints the same findings, says they do not fail a draft, and does not
fail because of them. A failed read still fails.

Why a draft cannot be failed. The review always comes after the push it reads, and its evidence is that push's CI
run (`.agents/protocol.md:78-79`, `:83-84`). If the check failed drafts, `Required` would be red on every push to a
code pull request until the review existed, and the reviewer would never see a green run to rest on. PR #48's
pending rule sharpens this: "a step is done only when the CI run for its pushed commit has passed"
(`inventory.md:378-381`).

Why a run on ready is needed. The Land order is review, fixes, re-review, archive, the check of the archive, *then*
undraft, then CI green (`.agents/protocol.md:52-57`). The `reviewed-by:` line is written after a push, and an edited
description starts no run (inventory 2.5). Without a run at undraft, the newest `Required` on the head could be a
draft's green one from before the line existed, and the check would not have been applied in CI. So the workflow's
`pull_request` trigger lists `opened`, `synchronize`, `reopened` and `ready_for_review`. The first three are
GitHub's defaults (`.github/workflows/ci.yml:7` lists none today); a list of types takes their place, so all four
are listed and `TestCIWorkflowPinned` fails when one is missing. GitHub refuses to merge a draft (documented, not
measured: inventory 9, N4), which is what makes the draft's green harmless.

`edited` is not listed: it would start the whole workflow on every edit of a description and cancel a run in
progress (`ci.yml:13`). A pull request that is not a draft and gains its line by an edit needs its failed job run
again, and the output says so.

Scenarios: "Draft"; and the requirement "A run when a pull request is marked ready" with "Ready type missing" and "A
default type missing".

### D4 What fails, and what is printed

A finding fails a pull request that is not a draft and is printed for a draft. A read that fails, or returns
something other than what was asked for, ends the script at once naming the read, as the known-flake check does
through `read_gh` (`scripts/merge-check.sh:56-67`). So does a failed `jq` step where an empty result would read as a
pass: sorting the names, where empty would mean documents only, as `merge-check.sh:136-140` guards today. The spec
says "non-zero"; exit 1 for findings and 2 for "unavailable" is the script's existing habit.

The order of the review check and the known-flake check is not specified, and no existing behaviour of the
known-flake check or the up-to-date rule changes. The known-flake check ends the script at several places today
(`merge-check.sh:115-116`, `:124-125`, `:151`); a run that ends there may not print a review finding, and the next
run does.

The script does not depend on how `gh` prints several pages. `gh` 2.97 joins them into one list; whether the CI
runner's `gh` does is not established (inventory 9, N5). Both forms are accepted, and "Code file on the second page"
serves both.

Scenarios: "A read for the review check fails", "Sorting names fails", "Push run and the review check".

### D5 Where it runs, and under which permissions

- **CI, pull-request run:** the `merge-check` job. Its step's name and comment change; nothing else in the job does.
- **Locally:** `task merge:check -- <n>`, called as today.
- **Push to `main`:** the check does not apply and the script says so, as it does for the known-flake check
  (`scripts/merge-check.sh:48`).

No new permission. The job has `contents: read`, `issues: read` and `pull-requests: read` (`ci.yml:74-77`), and the
spec's two permission scenarios stay as they are. That they cover a pull request and its files is documented per
endpoint and not yet seen under the job's token (inventory 9, N6); this pull request's first run with the new
script makes both reads (task 4.2).

### D6 Tests

Driven as today: a copy of the script run from a throwaway directory with the fake `gh` first on `PATH`
(`internal/harness/contract/mergecheck_test.go:18-41`). The fake gains two answers (the pull request and its files)
and keeps its rule that any other call exits 3.

- **`TestMergeCheckReview`** has one case per scenario of "Cross-agent review check". Each case writes the answers by
  hand and states the expected exit and words, taken from the scenario and never from running the script.
- **The existing cases** of `TestMergeCheckKnownFlake` and `TestMergeCheckUpToDateRule` keep their expectations. The
  shared healthy state becomes a documents-only pull request, so the review check passes in them.
- **`TestCIWorkflowPinned`** checks the trigger, and `TestCIWorkflowPinnedSensitivity` gains one planted workflow
  for each case of the two trigger scenarios. Its clean fixture starts on `push` only today
  (`mergegate_test.go:138`) and gains the trigger.

**Shown able to fail.**

- Written first: each new case is run against the script as it is today and seen to fail for its stated reason.
- The experiment of `docs/testing.md`, "Show that the test can fail", applies, because the check allows or forbids a
  merge. One wrong change at a time, each with its three runs recorded on the pull request: (1) treat a name under
  `.claude/agents/` as a document; (2) ignore a renamed file's previous name; (3) stop after the first page of
  files; (4) take an incomplete list as documents only; (5) accept a reviewer the `implemented-by:` line names; (6)
  treat every pull request as a draft; (7) let a failed sorting step read as documents only. A wrong change the
  tests let through is reported as a survivor.
- Against real GitHub, read-only, with the expected result worked out by hand from each pull request's page first:
  #65 (documents only), #63 (code, no `reviewed-by:` line), #14 (code, by a bot), #48 (code, draft, two pages of
  files, an `implemented-by:` line that names no agent), #67. This is the check on the fake that the fake cannot
  give.
- On this pull request, on the owner's word: marked ready once before any review exists, the run that starts fails
  `Merge check` with the findings; then it is returned to draft (task 4.3).

**Generated checks: examples are enough** (`docs/testing.md`, "Decide whether generated checks are needed"). Two
inputs have interacting cases.

- *Names.* Three conditions on a path (ends in `.md`; starts with `openspec/`; starts with `.claude/agents/`), of
  which the last two cannot both hold. "Names near the rule" gives each combination with its near misses (another
  letter case, a longer suffix, a longer or nested directory name). The rule over a list is "one code name decides",
  checked alone, among documents, on the second page, and as a previous name.
- *Lines.* A fixed prefix at the start of a line and two fixed words. The cases are listed: absent, twice, not at
  the start, carriage returns, and a line naming neither agent, one, the other, both, or a longer word.

The limits are explicit examples at their edges: 100 and 101 files, and a count that does not match. Each case
costs a process, and a generator would need a second implementation of the same path and line rules as its oracle.
So no generated check is written.

### D7 Documents changed with the code

- `.agents/protocol.md`:
  - "Land" (`:59`), which says "State `implemented-by: <model or persona>` in the PR body": the line names the
    implementing agent with the word `claude` or `codex`, beside the model or persona.
  - "Cross-agent review" (`:93-94`): the `reviewed-by:` line names the reviewing agent with the same words, and is
    written once that agent's newest review record approves. That one sentence is the definition used in tasks 5.4
    and 7.2 as well. The section also says what the script checks and what stays review only, and that marking
    ready starts the run that applies the check while an edit or a comment starts none.
  - "Verification" (`:115-119`): what `merge-check` now fails on.
  - If the owner answers Q3 with "not now", the sentence about a pull request neither agent wrote (`:72-74`) says
    that the check provides only for a bot's, and that a code change the owner would write by hand is given to an
    agent session to make: the commits are then that agent's, `implemented-by:` names it truthfully, and the other
    agent reviews. It also says that the ruleset lets nobody merge past the failed check, and that editing the
    ruleset is a waiver this rule does not give.
- `AGENTS.md`: the rule's row (`:90`) names the command and the tests under "Enforced by" and lists what stays
  review only; the command comment (`:45`), the CI paragraph (`:52`) and the Merge bullet follow. The Merge bullet
  is `:121-128` at the base, and its sentence about a code pull request is `:125-128`; `inventory.md:119-120` cites
  the sentence and `inventory.md:390` the bullet.
- One line each: `.agents/skills/semengine-preflight/SKILL.md` (`:45`, `:48-49`, `:67`), `docs/repository-map.md`
  (`:46`), `docs/testing.md` (`:160`), the description of `merge:check` in `Taskfile.yml` (`:96`), the workflow's
  comment (`ci.yml:65-69`, `:81`) and the script's header.
- The reviewer and developer contracts are not edited: a review record's form does not change.
- The Purpose of `openspec/specs/merge-gate/spec.md` says the capability covers "the parts of it that defend against
  flaky tests". A delta cannot change a Purpose, so it is edited in the archive commit (task 8.1).

## PR #48 and the other open pull requests

Two pull requests are open, both drafts (inventory 3).

**This change (#67) merges first if it is ready first; neither waits for the other.** #67 is the smaller change:
PR #48 is at 24 of 45 tasks (its description at `6bf9809`, read 2026-10-02T16:21Z), and has five findings from
Codex to fix and a full-diff review still to come (`inventory.md:227-231`).

| Order | What it costs |
| --- | --- |
| #67 first | #48 merges `main`, which it must do in any case. `AGENTS.md`, `docs/repository-map.md` and `docs/testing.md` are then changed on both sides; the two changes touch different lines of each, and by the protocol those three files need Codex's re-review on #48, which its pending full-diff review covers if the merge comes before it. #48's next run uses the new script. As a draft it is told what it lacks: its `implemented-by:` line names no agent. Before it is marked ready it needs `claude` in that line; its `reviewed-by:` line already names `codex`. The first port then merges with the command behind the rule |
| #48 first | The first port merges with the rule policed by prose only; Codex is in fact reviewing it. #67 then merges `main` and edits the moved `AGENTS.md` lines (`:96`, and `:127-134` for the Merge bullet, at #48's head `6bf9809`). If the documents of section 5 are already written, those files need Codex's re-review on #67 |

**This pull request (#67)** is a code pull request by Claude sessions, checked by its own copy of the script. Its
review of record is Codex's. It is the first use of the two words.

**Dependabot** opens pull requests that are not drafts, so each is red on `Merge check` until an agent has reviewed
it, a session has added the `reviewed-by:` line, and the job has been run again.

## What the check does not see

These parts of the rule stay review only, and the rule's row in `AGENTS.md` will say so.

- **Who wrote anything.** One login. The script cannot tell that the two lines are true, that a review record
  exists, or who posted it. It stops an omission and an honest slip, not a false statement.
- **Which commit the review read, and what changed since.** The line, once written, satisfies the check through
  every later push. The protocol's rules that a record covers the commit it names and that a later content commit
  needs a re-review (`.agents/protocol.md:87-90`) are not checked.
- **The verdict and the scope of the review.** A line written after a review that asked for changes passes, and so
  does one written after a review of six commits out of sixty. PR #48's line as it stands is the example
  (`inventory.md:248-255`).
- **That the owner named the reviewer** when both agents wrote commits.
- **The record itself:** that it is the reviewer's report as written, what was run, and that the evidence came from
  the tree and from CI.
- **A pull request that edits the script, the workflow or their tests** runs its own copies (inventory 2.5). It is a
  code pull request and is reviewed as a change to the merge gate.
- **The moment after marking ready.** Until the new run registers, a draft's green `Required` may be the newest
  result on the head. Merging in that moment needs two Land steps skipped: waiting for CI, and
  `task merge:check -- <n>`.
- **A code pull request written by hand** by neither agent and not by a bot: Q3.

## Invariants and their spec homes

Each holds for every pull request and every set of answers, and each is a sentence of the requirement "Cross-agent
review check" in this change's delta.

| Invariant | Spec home |
| --- | --- |
| I1 One code name makes a code pull request, wherever it is in the list and whichever of an entry's two names it is | "Documents only", second to fifth sentences |
| I2 Documents only is concluded only from a list with exactly as many entries as GitHub reports | "Documents only", last sentence |
| I3 No failed, short or misshapen read, and no failed sorting step, ends in documents only or a pass | "Reads" |
| I4 A code pull request that is not a draft passes only with the two lines as required | first paragraph; "The two lines" |
| I5 On a draft a finding never changes the exit status, and a failed read always does | "Drafts"; "Reads" |

## What a session has to know after this change

The four questions of the adopter seam inventory, for the people of inventory 7.

1. **What must they know?** An implementing session: its `implemented-by:` line carries its agent's name; the
   `reviewed-by:` line carries the other agent's; an edit starts no run. That is three facts, one over the
   contract's limit of two. The third is GitHub's, already holds for the known-flake check, and is printed by the
   failing run. A reviewing session: nothing new. The owner: nothing new, apart from Q1's note on his own
   instructions.
2. **What happens if they do nothing?** A draft goes on as before, with the findings in its log. Marked ready, the
   run fails. Nothing merges in silence.
3. **Where do they find out?** From the failed required check, whose output is the line to write. The session that
   sees the failure is the one that can fix it.
4. **What should they have to know?** Nothing. What remains is two words. The script observes what GitHub can show
   it: the draft flag and the files. Who implemented and who reviewed cannot be observed while there is one login.

## Assumptions that are documented and not yet measured

| # | Behaviour | Inventory | The design rests on it | Where it is measured |
| --- | --- | --- | --- | --- |
| A1 | Listing `ready_for_review` starts a run when a draft is marked ready, from the pull request's own copy of the workflow | N3 | yes (D3) | task 4.3, on this pull request. If no run starts, D3 goes back to design review |
| A2 | The job's token can read a pull request and its files | N6 | yes (D5) | task 4.2, on this pull request's first run with the new script |
| A3 | The runner's `gh` prints several pages in one of the two forms the script accepts | N5 | no: both are accepted | task 4.2 shows one page on the runner and task 4.1 two pages locally on #48; two pages on the runner are first seen on #48's own run. Stays open |
| A4 | GitHub refuses to merge a draft | N4 | yes (D3) | not planned; the known-flake check already rests on it |
| A5 | A pull request's file list stops at 3000 | N2 | only for the limit stated in D1 | not planned |

## Declared costs

- Two more reads on every pull-request run, and one more page per hundred files. A GitHub failure fails the job, as
  it does today.
- One more full CI run each time a pull request is marked ready.
- A code pull request that is not a draft is red from the moment it lacks its line until the job is run again.
- Both lines of every code pull request carry `claude` or `codex`. Eighteen of the 20 `implemented-by:` lines so far
  name no agent (`inventory.md:181-183`). The owner's instructions for Claude sessions, in a file outside this
  repository that agents may not edit, ask for `implemented-by: sonnet|opus|fable`; a session that follows only that
  writes a line the check fails (Q1).
- No waiver. If the other agent cannot review, a code pull request waits: #64 waives documents only.
- A pull request that changes more than 3000 files cannot pass (D1).
- Each Dependabot pull request, weekly for three ecosystems, is red until an agent has reviewed it, a session has
  edited its description and the job has been run again. If Dependabot rewrites its description, the line is added
  again (inventory 9, N7).
- **Test time: measured.** The new test runs the script for each of its cases in every execution of the unit
  tests, and `task verify` executes them seven times (`test:unit`, `test:integration` and five times in
  `test:repeat`).
  - On CI the `contract` package took 21.0 s, 14.1 s and 31.5 s in those three steps on `main` (run 37028259077,
    `94ccedd`), and 24.5 s, 18.4 s and 48.6 s with this change (run 37052400417, `69443ed`): 24.9 s more for the
    three together. The three steps took 9 s more and the `Verify` job 7 s more (197 s, then 204 s), because other
    packages run beside this one.
  - One run on each side. Variation between runners is not measured.
  - Before the code existed this design estimated 14 to 41 s and proposed a budget of 30 s for the three together
    (the architect's figure; nothing in the tree sets one). The measured cost is under it, so the cases are not
    run side by side and none is removed. The record is PR #67 comment 5959662131 (task 2.5).

## New surfaces and who uses them

| Surface | Present consumer |
| --- | --- |
| the review check in `scripts/merge-check.sh` | the `merge-check` job; `task merge:check`; the Land step |
| the words `claude` and `codex` in the two lines | the script |
| `ready_for_review` in the workflow's trigger | the review check on a pull request just marked ready |

Nothing is added for later use: no option, flag, label, variable, file or catalog.

## The adoption sweep

Not a pattern. The change adds no primitive meant for reuse: it applies the shape the known-flake check already has
(`read_gh`, a list that may be cut short, a failed `jq` never a pass; inventory 5) to one more rule in the same
script. No other script reads GitHub.

## Questions for the owner

Three. Each is written to be answered without reading the rest of this file, and says what it costs you.

**Q1. What must a code pull request's description say before it can merge?**

- (a) That it was reviewed: a `reviewed-by:` line. This is the least #66 asks for.
- (b) That the other agent reviewed it: `reviewed-by:` names `codex` for what Claude implemented and `claude` for
  what Codex implemented. For that, `implemented-by:` must name the implementing agent as well.
- (c) Also which commit the review read, as 40 characters in the line. The check then fails if a file of the pull
  request changed after that commit.
- (d) Also the word `APPROVE` in the line.

Recommendation: (b).

Reasons. (a) passes when a session records its own reviewer, which is the old habit: 17 of the 19 review comments
so far are an agent's own reviewer. (b) is the least that checks what the rule is about, the other agent. (c)
catches one more slip: the line left pointing at an older commit while work went on. It does not catch a line that
names the newest commit, whether the review of that commit approved, asked for changes, or never happened. (d)
closes that for a session that writes the line truthfully. Neither (c) nor (d) can prove a review exists, because
you and both agents use one GitHub login. They cost a comparison read against GitHub, a rule for when an older
review still counts, about a third more test cases, and a commit id pasted by hand. No pull request has merged on a
stale line yet.

What (b) leaves unchecked, so you can weigh it: once the line is written, the check stays satisfied. PR #48 today
would pass (b) as soon as its `implemented-by:` line names `claude`, although Codex's newest review asks for changes
and its full-diff review is pending. Under (b) that is held by the review tasks in each change and by your word for
the merge, not by the command. If you want the command to hold it, the answer is (d), not (c).

What every answer from (b) up costs you.

- Your own instruction file for Claude sessions says `implemented-by: sonnet|opus|fable`. Agents may not edit that
  file. A session that follows only it writes a line the check fails on a code pull request, until the session adds
  `claude` after reading the failure or you change the wording to, for example,
  `implemented-by: claude (sonnet|opus|fable)`.
- Each Dependabot pull request (weekly, three ecosystems) turns red on `Merge check` and stays red until an agent
  has reviewed it, a session has added `reviewed-by:` to its description, and the job has been run again. This
  holds under (a) as well.
- One more full CI run each time a pull request is marked ready, and an estimated 14 to 41 s more for each
  `task verify`.

**Q2. Should this change merge before PR #48, and should either wait for the other?**

Recommendation: this change first if it is ready first, and neither waits.

Reasons. This change is small and #48 is about half done. If it lands first, #48, the first port, merges with the
command behind the rule; its session adds `claude` to one line. Holding #48 for it would spend port time on a gate
that Codex's reviews already honour there.

What the other answers cost you. "#48 first, always" delays the check for as long as the port takes, and every pull
request merged meanwhile is policed by prose. "Hold #48 until this lands" stops the port for the length of this
change's design review, implementation and Codex review.

**Q3. Do you ever open a code pull request that you wrote by hand?**

As designed, such a pull request cannot pass truthfully. GitHub shows you and the agents as the same user, so the
check asks every user's code pull request to name `claude` or `codex` in `implemented-by:`. The protocol you merged
with #65 says a pull request neither agent wrote "is reviewed by either agent's reviewer", and gives a dependency
bot's as its example; the check as designed provides for the bot only.

Recommendation: not now. None of the 20 pull requests so far was written by hand (inventory 9, N9), and the protocol
will say that the check does not yet provide for one.

What the other answer costs. If you do write them, the design adds a third word: `implemented-by: owner` makes
either agent's review count, as for a bot. That is one more scenario, and a word that any session could write to
get past the "other agent" rule.

What "not now" costs you, and what you do meanwhile. A code pull request you write yourself stays red on
`Merge check`, and you cannot merge past it: the ruleset on `main` lets nobody bypass a required check
(`bypass_actors: []`, `current_user_can_bypass: "never"`; `inventory.md:453`). The path that stays inside the rule
is to give the change to an agent session to make: the commits are then that agent's, `implemented-by:` names it
truthfully, and the other agent reviews. The only other way is to edit ruleset `24272345` yourself. That is a waiver
outside the script, it lifts the required check from every pull request while it is off, and this design does not
propose it. Adding the third word later is a design change, an implementation and a Codex review.

**Not asked, because it is already ruled.**

- A waiver: #64 waives documents only, and the `merge-gate` spec has none for known flakes. None is designed.
- What "documents only" means: `.agents/protocol.md:67-72`, merged with #65. The design implements it as written.
- That Dependabot's pull requests are checked: `.agents/protocol.md:72-74` gives them to either agent's reviewer.
  What that costs is in Q1.
- Pull requests already open: `.agents/protocol.md:75-76`. #48 is checked once it is marked ready.

## Premises

| Premise | Measurement |
| --- | --- |
| Nothing reads a description or a file list today | inventory 1: the five reads (`merge-check.sh:73`, `:87`, `:108`, `:119`, `:134`) and the search at `inventory.md:78-80` |
| One login for the owner and both agents | `inventory.md:209-211`: 89 comments, all by `cglusky` |
| GitHub's rule asks for no review, and the script drops it | `inventory.md:83-94`; `merge-check.sh:77` |
| An author cannot approve their own pull request | documented, not measured: inventory 6, Ownership; 9, N10 |
| The heading `Review record` is on same-agent reviews; Codex's two records differ in form | `inventory.md:212-231` |
| #48's `reviewed-by:` line names `codex` and a short id, and stood while a later record asked for changes | `inventory.md:248-255` |
| `implemented-by:` names an agent on two pull requests of 20; `fable` is in 17 and in no tracked file | `inventory.md:173-183` |
| A bot's pull request is reported as type `Bot` and is not a draft; a user's, an agent's included, as type `User` | `inventory.md:171-172`, `:272` |
| One read gives the draft flag, description, author type and file count | `inventory.md:271` |
| `gh pr view --json files` stops at 100 silently; the paged read returns every entry | `inventory.md:273`, `:275`: 100 of 114, and 114 of 114 |
| The comparison of Codex's first reviewed commit with #48's head lists 34 files, 29 of them #48's (used for O4's row only) | `inventory.md:277` |
| An edited description starts no run; a re-run reads GitHub again | inventory 2.5 (`:303-307`) |
| Marking a draft ready is not among the default activity types | documented, not measured: `inventory.md:306`; 9, N3 |
| The job has three read permissions and a test pins them | `ci.yml:74-77`; `mergegate_test.go:124` |
| The fake refuses a read it does not know, so a new read fails every existing case until the fake answers it | `mergecheck_test.go:37`; `inventory.md:344-346` |
| The known-flake check ends the script at several places today | `merge-check.sh:115-116`, `:124-125`, `:151` |
| #48 is the only other open pull request; it changes `AGENTS.md`, `docs/repository-map.md`, `docs/testing.md` and both contracts | `inventory.md:370` |
| `task verify` runs the `contract` package's tests seven times | `scripts/verify.sh:12`; `Taskfile.yml:81`, `:88`; CI run 37028259077 |
| No budget for `task verify`'s time is set in the tree | `git grep -n -i budget 94ccedd -- docs/testing.md openspec/specs .agents AGENTS.md Taskfile.yml scripts/verify.sh`: timeouts for cleanup, the fixture and the integration runner, and no limit on a step or on the whole; `docs/testing.md:287-292` lists what is enforced, and of `task verify` says only that it prints each step's wall time |
