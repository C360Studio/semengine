# Inventory: review-gate-check

- base: `94ccedd11dbe1ad57ed2dc8507206c97430ead59` (`origin/main`). The claim branch `claude/review-gate-check` is
  this commit plus one empty claim commit, `4e35ce0`, so every pin below holds on both.
- measured: 2026-10-02; issue #66, claim PR #67; `gh` 2.97.0, `jq` 1.7.1.
- repositories read: this one only. `docs/inventory-scope.md` rule 1 asks an inventory to name its repositories; the
  question below needs no sister repository.
- GitHub state (pull requests, comments, API answers) was read live on the same day. PR #48 was pushed to twice while
  this was written (head `cc55af6`, 101 files, then `8a6f7ce`, 113 files); each measurement names the head it saw.

This is the architect's inventory-only deliverable (contract step 2). It holds no target state, no options and no
recommendation. It has not had its independent review. `design.md` in the same directory is a draft that depends on
this file and is not valid until this file has `INVENTORY PASS`.

A pin is written `path:line` with the start of the line's text; a long line is cut with `…`. Text from a Markdown
file is shown as it is written there, so its own code spans show as code. Text from a script, a test or a workflow
is shown as code.

## 0. The question and the reading scope

**Question.** What does this repository already hold that records, decides or checks three facts about a pull
request, and what can `scripts/merge-check.sh` read from GitHub about each?

1. Whether it is a code pull request or documents only.
2. Which agent implemented it.
3. Which agent reviewed it, and which commit that review read.

Terms used below, as `.agents/protocol.md` uses them. A *code pull request* changes at least one file that is not
Markdown and not under `openspec/`, or changes a role adapter under `.claude/agents/`. *Documents only* is every other
pull request. A *review record* is the pull request comment in which the reviewing session posts its report. The
*head* is the newest commit of the pull request's branch.

**Read in full:** `.agents/protocol.md`; `scripts/merge-check.sh`; `openspec/specs/merge-gate/spec.md`;
`internal/harness/contract/mergecheck_test.go`; `.github/workflows/ci.yml`; `Taskfile.yml`; `.github/dependabot.yml`;
`docs/testing.md`; `docs/inventory-scope.md`; `.agents/contracts/semengine-architect.md`; issues #66 and #64 with
comments. **Read in the parts that bear on the question:** `docs/setup-plan.md` ("Proposed foundation", "Shared state
and working process"; the rest is engine scope and names no merge rule); `internal/harness/contract/mergegate_test.go`
(lines 1 to 366, the workflow test); the archived `flake-defense` design, decision D8 and its assumptions table;
`AGENTS.md`; `.agents/README.md`; the reviewer and developer contracts; the preflight skill.

## 1. The claimed gap (category 1)

The claim (issue #66): nothing fails when a code pull request merges without the other agent's review.

| Pin | Text |
| --- | --- |
| `AGENTS.md:90` | \| A code pull request (any changed file that is not Markdown or under `openspec/`, …` ending `review only until issue #66 puts a check in `scripts/merge-check.sh` (owner ruling of 2026-10-02: enforce it for code) \| |
| `scripts/merge-check.sh:73` | `read_gh "the rules in force on main" 'type == "array" and all(.[]; type == "object")' \` |
| `scripts/merge-check.sh:87` | `read_gh "ruleset ${id}" 'type == "object" and (.enforcement \| type == "string")' \` |
| `scripts/merge-check.sh:108` | `read_gh "labels" 'type == "array" and all(.[]; (.name \| type) == "string")' \` |
| `scripts/merge-check.sh:119` | `read_gh "open ${label} issues" 'type == "array" and all(.[]; (.url \| type) == "string" …` |
| `scripts/merge-check.sh:134` | `read_gh "pull request #${pr}" '(.closingIssuesReferences \| type == "array") and …` |
| `scripts/merge-check.sh:135` | `gh pr view "$pr" --json closingIssuesReferences` |
| `internal/harness/contract/mergecheck_test.go:37` | `*) echo "fake gh: unexpected call: $*" >&2; exit 3 ;;` |

Those five are every read the script makes. Searches that came up empty:

- `git grep -n -E 'comments|--json [a-zA-Z,]*body|pulls/|compare/' -- scripts .github Taskfile.yml internal`: two
  hits, both prose about Go comments in `internal/harness/pindiff/` (`rewrite.go:17`, `rewrite_test.go:8`). No script,
  job or test reads a pull request's description, its changed files, its comments or a comparison of two commits.
- `grep -n -i review openspec/specs/merge-gate/spec.md`: lines 63 and 71, both the phrase "checked in review only".
  The `merge-gate` capability has no requirement about review.
- GitHub's own review state is unused: `gh api repos/C360Studio/semengine/pulls/<n>/reviews --jq length` is `0` for
  #48, #54, #59, #63 and #65, and `gh pr view 48 --json reviewDecision` is empty.
- No pull request description has ever had a line that starts `reviewed-by:` (all 20 pull requests, section 2.2).

The gap is as claimed.

## 2. Every current spelling of the three facts (category 2)

### 2.1 Code or documents only

| Pin | Text |
| --- | --- |
| `.agents/protocol.md:67` | `Codex implements), and that the rule is enforced for code and waived for a documents-only pull request. What` |
| `.agents/protocol.md:68` | `"documents only" means is this repository's reading of that ruling, not the owner's words: every file the pull` |
| `.agents/protocol.md:69` | request changes is a Markdown file or is under `openspec/`, and none is a role adapter under `.claude/agents/` |
| `.agents/protocol.md:71` | file makes it a code pull request: a `.go` file, the harness included; `go.mod`; a script; `Taskfile.yml`; the CI |
| `AGENTS.md:125` | `content commit; squash merge. A code pull request (any changed file that is not a Markdown file or under` |
| `.agents/contracts/semengine-reviewer.md:14` | On a code pull request (any changed file that is not a Markdown file or under `openspec/`, or that is a role adapter |
| `.agents/README.md:46` | `code pull request the reviewer of record is the other agent's reviewer from the table above; a documents-only pull` |
| `.agents/contracts/semengine-developer.md:46` | `pull request ("Cross-agent review"), and for a documents-only one SemEngine's reviewer and any cross-agent round` |
| `.agents/skills/semengine-preflight/SKILL.md:56` | - **Documentation or skill instructions only:** `task fmt:check`, `task docs:check`, and `git diff --check`; add |

The definition has one canonical home (`protocol.md:67-72`) and is restated in `AGENTS.md` (row 90 and the Merge
gate, 125 to 128) and in the reviewer contract. All of it is prose: nothing in the tree sorts a list of changed files.
The preflight skill sorts a diff for a different purpose (which local checks to run) and by a different rule.

Facts about the tree that the definition meets:

- Every tracked Markdown file ends in `.md`: `git ls-files | grep -i -E '\.(md|markdown|mdx|mdown)$'` lists 63
  files, all `.md`.
- Files under `openspec/` that are not Markdown: three `.gitkeep`, six `.openspec.yaml`, and `openspec/config.yaml`.
  No `.go`, `.sh` or workflow file is there today. The definition would call one a document if it were.
- `.claude/agents/` holds four `.md` files whose front matter sets `tools` and `model` (for example
  `.claude/agents/semengine-reviewer.md:4-5`). The six `SKILL.md` files under `.claude/skills/` and
  `.agents/skills/` carry only `name`, `description` and, once, `argument-hint`. The definition counts skills,
  contracts, `AGENTS.md` and the protocol itself as documents.
- What the definition gives for every merged pull request, sorted by hand from each file list (new and previous
  names; `gh api --paginate repos/C360Studio/semengine/pulls/<n>/files`): documents only for #1, #23, #39, #47, #55,
  #56, #58 and #65; code for #12, #13, #14, #21, #43, #44, #54, #59 and #63. #21 is code by one file,
  `docs/admission-ledger.yaml`. Open: #48 is code; #67 changes no file yet.

### 2.2 Which agent implemented

| Pin | Text |
| --- | --- |
| `.agents/protocol.md:59` | review; no later content commit bypasses the archive/spec-sync check. State `implemented-by: <model or persona>` in |
| `.agents/protocol.md:65` | the pull request's `implemented-by:` line records. The owner ruled on 2026-10-02 (issue #64) that Codex's |
| `.agents/protocol.md:72` | workflow; `docs/admission-ledger.yaml`. If both agents wrote commits, the owner names the reviewer on the issue. A |
| `.agents/protocol.md:73` | `pull request neither agent wrote (a dependency bot's) is reviewed by either agent's reviewer, and the record names` |
| `.agents/protocol.md:16` | - **Who has claimed what:** a **draft PR** on an agent-prefixed branch (`claude/…`, `codex/…`) opened at the start |
| `AGENTS.md:124` | `GITHUB_ACTIONS` unset; `implemented-by: <model or persona>` in the PR body; the archive/spec sync is the last |

Four things say who implemented a pull request. Measured on all 20 pull requests
(`gh pr list --state all --json number,author,headRefName,body`) and on the commits of #48 and #59:

1. **The `implemented-by:` line**, free text. Values seen: `opus`; `fable (orchestrating session)`;
   `opus (semengine-developer)`; `dependabot` (PR #14, added by a session, not by Dependabot); on #59
   `gpt-6-sol (…); gpt-6-astra (…); Codex (orchestration). Leg 2: claude-fable-5-1 (review fixes: …), reviewed by
   semengine-reviewer (opus)`; on #48 `design, opus role agents (architect, reviewer, technical writer), orchestrated
   by fable then opus; …`. The word `claude` appears in one of them (#59) and `Codex` in two (#59, #1). On #1 the
   line is a list item (`- implemented-by: …`), everywhere else it starts the line.
2. **The branch prefix** (`claude/`, `codex/`, `dependabot/`): names who claimed the work (`protocol.md:34-39`),
   not who wrote each commit. #59 is `codex/await-last-error` and has Claude commits.
3. **Commit trailers.** Commits by Claude sessions carry `Co-Authored-By: Claude … <noreply@anthropic.com>`.
   Commits by Codex sessions carry no trailer: #59 has four commits with none and five with Claude's. The commit
   author is `cglusky` on every one. A commit with no trailer is therefore not known to be Codex's.
4. **The pull request's author.** `cglusky`, type `User`, on every pull request an agent opened; `dependabot[bot]`,
   type `Bot`, on #14.

The owner's global instructions for Claude sessions (outside this repository, so not a pin) ask for
`implemented-by: sonnet|opus|fable`: a model name, with no agent name.

### 2.3 Which agent reviewed, and which commit

| Pin | Text |
| --- | --- |
| `.agents/protocol.md:63` | `- **Cross-agent review:** on a code pull request the reviews in "Land" (the implementation review, the re-review of` |
| `.agents/protocol.md:64` | `fixes, the check of the archive/spec sync) are done by the agent that wrote none of the commits under review, as` |
| `.agents/protocol.md:78` | - The implementer asks with a PR comment headed `Review request`. It names the kind of review, the commit to read, |
| `.agents/protocol.md:81` | - The reviewing session answers with a PR comment headed `Review record`: its reviewer's report as written, in the |
| `.agents/protocol.md:82` | `reviewer contract's format, not a summary. It names the commit read, what was run, and what could not be run.` |
| `.agents/protocol.md:87` | `- A record covers the commit it names. A later content commit, one that changes the pull request's own diff against` |
| `.agents/protocol.md:88` | `main`, needs a re-review, and the archive commit needs the archive check. A merge of `origin/main` is not a |
| `.agents/protocol.md:89` | content commit when `main` changed no file the pull request changes: the record carries over, and CI on the |
| `.agents/protocol.md:90` | merged head is the check. When `main` changed a file the pull request also changes, that file needs a re-review. |
| `.agents/protocol.md:93` | - The implementer writes `reviewed-by: <model or persona>` in the PR body beside `implemented-by:`, taken from the |
| `.agents/protocol.md:60` | the PR body. Bring a pushed branch up to date by merging `origin/main` into it; do not rebase or force-push it. A |
| `.agents/contracts/semengine-reviewer.md:17` | `review is asked for and answered on the pull request. A review record there names the commit it read, what it ran and` |
| `.agents/contracts/semengine-reviewer.md:300` | style only. End with `APPROVE` when there are no blocking/high findings, otherwise `CHANGES REQUESTED` and the exact |

How the record looks in practice (measured: `gh api --paginate repos/C360Studio/semengine/issues/<n>/comments`):

- **One login.** All 33 comments then on #48 and all three on #59 are by `cglusky`, type `User`. The owner and both
  agents post under it. The archived `flake-defense` design already records this ("One login", decision D8). No
  field of a comment says which agent wrote it.
- **The heading `Review record` is already used by the implementing agent's own reviewer.** #48 has three comments
  headed `## Review record…` (ids 5939311446, 5942129322, 5950922103). Each is the report of the Claude session's own
  `semengine-reviewer` on a Claude pull request, which the protocol calls an early check, not the gate
  (`protocol.md:75-77`). A fourth verdict on #48 has another heading (5954106306, `## Task 2.9: harness review
  verdict, PASS …`).
- **The one review of a Codex pull request by Claude** (#59) is three comments headed `## Leg 2: …`, none headed
  `Review record`. They predate PR #65.
- **How commits are named.** As short ids in prose and tables, several in one comment (comment 5954106306 names
  seven, among them `912d82a` and `030e85e`). The one `Review request` so far (#48, 5955724059) names the head by
  its full 40 characters, `0a86a9a6a9853e427bee9aab5c52e609b9a4c0ea`, on a line that starts "Head:".
- **Verdict words** vary by review mode: `APPROVE`, `PASS`, `DESIGN PASS`, `CHANGES REQUESTED`, often one row per
  round in a table.
- **A third home for "a review happened":** OpenSpec task truth. Tasks are ticked with a comment id, for example
  `openspec/changes/archive/2026-10-02-carry-check/tasks.md:15-17` ("DESIGN PASS … recorded on this pull request
  (comment 5952034096)").
- **PR #48 today** carries an unanswered `Review request` to Codex and, in its description, a list headed "Not yet
  reviewed" of five fix commits. The fact "these commits are not reviewed" lives in prose in the description.

### 2.4 What GitHub answers to a read (measured)

| Read | Answer | Limit |
| --- | --- | --- |
| `gh api repos/C360Studio/semengine/pulls/48` | one object: `draft` (true), `changed_files` (101, later 113), `head.sha` (40 characters), `base.ref`, `user.type` (`User`), `body` (a string; no carriage return in #48 or #14) | none |
| the same for #14 | `user.login` `dependabot[bot]`, `user.type` `Bot`, `draft` false | none |
| `gh api --paginate 'repos/…/pulls/48/files?per_page=100'` at head `cc55af6` | `gh` 2.97 joins the pages into one array of 101 entries; with `--slurp`, an array of two pages (100 and 1). Each entry has `filename` and `status` (84 `added`, 17 `modified`); none had `previous_filename`, which GitHub documents for a renamed file | documented: 3000 files; not measured |
| the same for #67 (no changed file) with `--slurp` | `[[]]` | none |
| `gh pr view 48 --json changedFiles,files` at head `8a6f7ce` | `changedFiles` 113, `files` lists 100 and says nothing | 100, silent |
| `gh api --paginate 'repos/…/issues/48/comments?per_page=10'` | one array of 33 (four pages joined); each entry has `id`, `body`, `user`, `created_at`, `updated_at`, `html_url` | none seen |
| `gh api repos/…/compare/0a86a9a6…c0ea...cc55af64…0003` | `status` `ahead`, `ahead_by` 3, `total_commits` 3, `files` 9 entries with `filename`; all nine are files #48 changes. The answer has no field that says the file list was cut | 300 files, measured outside the project (see the note under this table); the largest comparison here, the first commit against `main`, has 155 |
| the same with short ids `0a86a9a...cc55af6` | the same answer | none |
| the same with a commit id that does not exist | HTTP 404, `gh` exits 1 | none |

The 300-file limit was measured on a large public repository that is not part of this project and was read for no
other purpose: `gh api 'repos/golang/go/compare/go1.21.0...go1.22.0'` answers with `total_commits` 1712, 250 commits
listed and exactly 300 files listed, and the same keys as above. Nothing in the answer says the file list stopped.

Two reads of #48 a few minutes apart gave 95 and then 101 changed files, because the branch was pushed to between
them. Two reads inside one run can disagree for the same reason.

### 2.5 When a CI run starts (pins and prior measurements)

| Pin | Text |
| --- | --- |
| `.github/workflows/ci.yml:7` | `pull_request:` (no activity types listed) |
| `.github/workflows/ci.yml:13` | `cancel-in-progress: ${{ github.event_name == 'pull_request' }}` |
| `.github/workflows/ci.yml:74` | `permissions:` followed by `contents: read`, `issues: read`, `pull-requests: read` (75 to 77) |
| `.github/workflows/ci.yml:85` | `PR_NUMBER: ${{ github.event.pull_request.number }}` |
| `scripts/merge-check.sh:48` | `echo "merge-check: push run (${how}); the known-flake check does not apply, the up-to-date rule is checked"` |

- With no types listed, GitHub starts a run when a pull request is opened, reopened or pushed to (documented).
  Editing the description starts none: measured on PR #44 (comment 5939718900; assumption A11 of `flake-defense`,
  recorded on #42). A new comment starts none (documented; no workflow here listens for comments).
- Marking a draft ready for review is not among the default types (documented, not measured).
- A re-run of a failed job reads GitHub again (measured; assumption A5, recorded on #42).
- GitHub refuses to merge a draft (documented, not measured; assumption A15, still open on #42).
- A pull request runs its own copy of the script, the workflow and their tests (`flake-defense` design, D8, "A pull
  request runs its own copy").

## 3. Adjacent claims on the territory (category 3)

**Current specs.** `openspec/specs/merge-gate/spec.md` holds every requirement on the script and the workflow.

| Pin | Text |
| --- | --- |
| `openspec/specs/merge-gate/spec.md:5` | that defend against flaky tests: how often and under which settings `task verify` runs the unit tests, that a |
| `openspec/specs/merge-gate/spec.md:57` | `every pull-request run except that of a pull request whose closing references include every open one. There SHALL be` |
| `openspec/specs/merge-gate/spec.md:58` | `no waiver: no comment and no label on the pull request SHALL exempt it, and no environment variable SHALL change which` |
| `openspec/specs/merge-gate/spec.md:77` | `the list asked for, naming the read, and when the issue list reaches the number asked for; none of these is ever` |
| `openspec/specs/merge-gate/spec.md:79` | `SHALL NOT apply this check and SHALL say so.` |
| `openspec/specs/merge-gate/spec.md:201` | pull-request run, under a token limited to `contents: read`, `issues: read` and `pull-requests: read`. No job in |
| `openspec/specs/merge-gate/spec.md:244` | - **WHEN** the `merge-check` job is granted a permission other than its three reads |

The Purpose (lines 4 to 7) says the capability covers the parts of the merge gate "that defend against flaky tests".
Scenarios that end "the script exits zero" (lines 86 to 91, 99 to 103, 117 to 120, 192 to 196) say nothing about
the pull request's review.

**What holds the script and the workflow today.**

| Pin | Text |
| --- | --- |
| `internal/harness/contract/mergecheck_test.go:18` | ``const fakeGH = `#!/bin/sh`` (answers five reads from files, refuses any other call) |
| `internal/harness/contract/mergecheck_test.go:81` | `func healthy() ghState {` |
| `internal/harness/contract/mergecheck_test.go:176` | `func TestMergeCheckKnownFlake(t *testing.T) {` |
| `internal/harness/contract/mergecheck_test.go:283` | `if strings.Contains(r.argv, "issue list") \|\| strings.Contains(r.argv, "pr view") {` |
| `internal/harness/contract/mergegate_test.go:124` | `mergeCheckPermissions = map[string]string{"contents": "read", "issues": "read", "pull-requests": "read"}` |
| `internal/harness/contract/mergegate_test.go:138` | ``const good = `on: [push]`` (the clean fixture of the workflow test) |
| `internal/harness/contract/mergegate_test.go:264` | `var wf struct {` (reads `permissions`, `defaults` and `jobs`; it does not read `on`) |

Every existing case of `TestMergeCheckKnownFlake` and `TestMergeCheckUpToDateRule` runs the script for pull request
12 against that fake. A read the fake does not know exits 3, so a new read in the script fails all of them until the
fake answers it.

**Prose that names the script and would go stale.** `AGENTS.md:45`, `:52`, `:79`, `:90`, `:121-128`;
`.agents/protocol.md:115-119`; `.agents/skills/semengine-preflight/SKILL.md:45`, `:48-49`, `:67`;
`docs/repository-map.md:46`; `docs/testing.md:160`; `Taskfile.yml:96`; `.github/workflows/ci.yml:65-69`, `:81`;
`scripts/merge-check.sh:2-20`.

**ADRs.** None exist (`git ls-files | grep -i adr`: no result).

**Active OpenSpec changes on `main`.** None (`ls openspec/changes`: `archive`).

**Issues.** #66 is this work. #64 (closed by PR #65) holds the rulings. #61 (open): `ubuntu-latest` moves to Ubuntu
26 from 2026-10-19 and the runner image is not pinned, so the `gh` and `jq` versions under the `merge-check` job can
change between two runs. No other open issue names the merge check
(`gh issue list --state open --limit 100`: 31 issues read).

**Admission ledger.** Not a port; no row.

**Open pull requests** (`gh pr list --state open --json number,title,changedFiles,files`; #48 has more than 100
files, so its list was read with `gh api --paginate repos/C360Studio/semengine/pulls/48/files`).

| PR | State | Files | Overlap with the files this question touches |
| --- | --- | --- | --- |
| #48 `claude/setup-04a-01-floor` | draft, code, 113 files at `8a6f7ce` | port of the floor | `AGENTS.md`, `docs/testing.md`, `.agents/contracts/semengine-developer.md`, `.agents/contracts/semengine-reviewer.md`. It does not change `scripts/merge-check.sh`, either merge-gate test file, `.github/`, `Taskfile.yml`, `.agents/protocol.md`, the preflight skill or `docs/repository-map.md`, and it has no delta under `openspec/changes/setup-04a-01-floor/specs/merge-gate/` (its deltas: `background-work`, `harness-boundaries`, `lifecycle-suite`, `nats-fixture`, `process-host`, `transport-client`) |
| #67 `claude/review-gate-check` | draft, no changed file | this claim | itself |

PR #48 is also the first pull request the rule will meet: it is a code pull request by Claude sessions, its
`implemented-by:` line names no agent, and it has no review by Codex yet.

## 4. Present consumers (category 4)

What reads or runs the surfaces in question today:

| Surface | Present consumer |
| --- | --- |
| `scripts/merge-check.sh <n>` | `.github/workflows/ci.yml:86` (`run: scripts/merge-check.sh "$PR_NUMBER"`); `Taskfile.yml:98`; the Land step, `.agents/protocol.md:56` |
| the job `merge-check` | `.github/workflows/ci.yml:92` (`needs: [verify, merge-check]`) |
| the line `implemented-by:` | readers of the pull request; `.agents/protocol.md:65` names it as the record of who wrote the commits. No command reads it |
| the line `reviewed-by:` | none yet: no pull request has it, no command reads it |
| a comment headed `Review record` | the implementing session and the owner, by eye. No command reads it |

Anything a design adds beyond these needs a consumer from this list or a named new one.

## 5. The problem shape (category 5)

The shape: admit or refuse at a gate, on facts read from another system at the moment of the run, where a read that
fails or is cut short must never count as "allowed".

Closest instance, in the same script:

| Pin | Text |
| --- | --- |
| `scripts/merge-check.sh:56` | `read_gh() {` (runs one read; exits 2 naming it when the read fails or `jq` rejects its shape) |
| `scripts/merge-check.sh:111` | `if [ "$(printf '%s' "$answer" \| jq 'length')" -ge "$limit" ]; then` (a full page may be cut short) |
| `scripts/merge-check.sh:136` | `# A failed jq leaves an empty result, which would read as "every flake is closed".` |
| `scripts/merge-check.sh:156` | `echo "::warning::pull request #${pr} is exempt from the known-flake check by closing ${exempting}; …` |

Second instance: `scripts/openspec-queue.sh:95` (`# An unavailable read is unavailable, never an empty queue: …`).

The paginated list is named in the architect contract
(`.agents/contracts/semengine-architect.md:103-105`: the `gh pr list` file listing "stops at 100 files for each pull
request and says nothing when it does").

Whether a design adopts the first instance is for the design to say.

## 6. Same-class collision table

The semantic class: the recorded fact that a pull request's content was reviewed by an agent other than the one that
wrote it.

| Dimension | Evidence |
| --- | --- |
| Semantic class | "Reviewed by the other agent, at this commit": `.agents/protocol.md:63-94` |
| Owners | The protocol (the rule, the two comment headings, the `reviewed-by:` line); the reviewer contract (`semengine-reviewer.md:14-20`, verdict words at `:300`); OpenSpec task truth (review tasks ticked with a comment id); GitHub's review state (unused, section 1) |
| Catalogs | None. No file or registry lists reviews. The pull request's conversation is the only store. `git grep -n -i 'review record'` outside the archive: `protocol.md:61`, `:81`, `semengine-reviewer.md:17` |
| Status | The check `Required` (`ci.yml:90-105`) and the job `merge-check`, neither of which reads review state today; `reviewDecision` is empty on every pull request |
| Lifecycle | Created by a comment; "covers the commit it names" (`protocol.md:87`); kept across a merge of `main` that changed none of the pull request's files (`:88-90`); a later content commit needs a new one; a rebase strands it (`:60-62`). Nothing says what an edited comment means |
| Ownership | One GitHub login for the owner and both agents (section 2.3). The branch prefix names the claimer (`protocol.md:34-39`). No per-agent identity exists on GitHub |
| Readers | People and agent sessions, by eye. No command (section 1 searches) |
| Writers | The implementing session (request, description lines); the reviewing session (record); the owner (rulings on the issue). All under one login |
| Recovery | For the script's existing reads: exit 2 naming the read (`merge-check.sh:56-67`), and a re-run reads again. For review records: nothing defined |

## 7. Adopter seam inventory

No consumer repository reaches this surface. The people who carry it work in this repository or act on it from
outside:

- **An implementing session** (Claude or Codex) that has never opened `scripts/merge-check.sh`.
- **The other agent's reviewing session**, which the owner starts and which cannot be started or corrected by the
  implementer (`protocol.md:77`).
- **Dependabot**, which opens pull requests weekly for three ecosystems (`.github/dependabot.yml:3-15`), writes its
  own description, and opens them ready for review, not as drafts (PR #14: `draft` false).
- **The owner**, who merges on his word and, by the protocol, names the reviewer when both agents wrote commits.

What each must know today, with the rule as prose: the definition of documents only; that the review of record is
the other agent's; the two comment headings; that a record names its commit; that a merge of `main` may or may not
keep the record; that `reviewed-by:` goes in the description. If they do nothing: the pull request merges, and
nothing says the rule was skipped. Where they find out: a document (`protocol.md`), which is the level the architect
contract counts as a finding for a correctness fact. What they should have to know is for the design.

Two facts that any check will meet, whatever its shape:

- A check that reads the description or the comments runs only when a run starts (section 2.5). The review is
  always written after the push it reviews.
- The reviewing agent reads the reviewer contract, not the implementer's instructions. A format the record must
  follow reaches that agent only through the contract, the protocol or the text of the `Review request`.

## 8. Intent check and extraction obligations

No boundary is set or moved (no port set, tier, package or capability), so the intent table is not triggered. This
is not a port: no ledger row, no pin probe, no surface audit and no returning guidance apply. No Go production code
exists or is touched, so the context rules have nothing to inventory.

## 9. Not established

Each is a fact a design might rest on that this inventory could not measure.

| # | Fact | Why it is open |
| --- | --- | --- |
| N1 | That the 300-file limit of a comparison (measured, section 2.4) is the same for this repository and stays so | one measurement, on another repository |
| N2 | The list of a pull request's files stops at 3000 | documented; no pull request here is near it |
| N3 | Listing `ready_for_review` under `pull_request` starts a run when a draft is marked ready, and the list is taken from the pull request's own copy of the workflow | documented; never tried here |
| N4 | GitHub refuses to merge a draft | documented; open as A15 on #42 |
| N5 | The `gh` on the CI runner joins paginated pages into one array as 2.97 does locally, and accepts `--slurp` | the runner's version is not recorded; the image is unpinned (#61) |
| N6 | The job's token (`contents`, `issues` and `pull-requests`, all read) may read a pull request, its files, its comments and a comparison of two commits | documented per endpoint; the job has made none of these reads |
| N7 | Whether Dependabot rewrites a description a session has added lines to | #14 kept its added line until merge; no later Dependabot update of that pull request was seen |
| N8 | Whether a description edited in the browser carries carriage returns | documented GitHub behaviour; both bodies measured were written through `gh` and have none |
| N9 | Whether anyone will open a pull request written by hand, by neither agent and not by a bot | none of the 20 is one |
