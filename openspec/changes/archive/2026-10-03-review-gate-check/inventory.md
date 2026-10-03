# Inventory: review-gate-check

- base: `94ccedd11dbe1ad57ed2dc8507206c97430ead59` (`origin/main`, still its head at 2026-10-02T16:28Z). The claim
  branch `claude/review-gate-check` is this commit plus an empty claim commit, `4e35ce0`, and `0f5e63b`, which adds
  only `openspec/changes/review-gate-check/`, so every pin below holds on both.
- revision 2, measured 2026-10-02; issue #66, claim PR #67; `gh` 2.97.0, `jq` 1.7.1. Revision 1 (sha256
  `511bfd6b9b5abf825b7a335adc24a3352ffa8d8db3485f099f487b81f516f30f`) had its independent review, which asked for
  changes (PR #67, comment 5956546196). This revision answers its eight findings.
- repositories read: this one only. `docs/inventory-scope.md` rule 1 asks an inventory to name its repositories; the
  question below needs no sister repository. Revision 1 also read `golang/go`, a public repository that file does
  not list, to measure a GitHub limit. That measurement is withdrawn as out of scope, and nothing below rests on it
  (section 2.4; section 9, N1).
- GitHub's documentation (docs.github.com, a website, not a repository) was read for this revision on 2026-10-02
  between 16:23Z and 16:29Z: the REST reference pages for commits, pull requests, pull request reviews and
  repository rules; the guides "Available rules for rulesets" and "Approving a pull request with required reviews";
  and "Events that trigger workflows". Below, "documented" means one of those pages says it, unless the entry says
  the page was not re-read.
- GitHub state (pull requests, comments, API answers) was read live. Another session pushes to PR #48 often. Unless
  a line says otherwise, a statement about #48 is of head `6bf98097f03ebb4dc701728505ebc37fafc113f0` (`6bf9809`),
  read between 2026-10-02T16:21Z and 16:33Z; its head, file count and 35 comments were the same at 16:37Z.
  Revision 1 read it at `cc55af6` (101 files) and `8a6f7ce` (113 files). A second review record from Codex arrived
  on #48 at 16:30:22Z, while this revision was being written; it is included.

This is the architect's inventory-only deliverable (contract step 2). It holds no target state, no options and no
recommendation. It does not have `INVENTORY PASS` yet. `design.md` in the same directory is a draft that depends on
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

That list is revision 1's reading. Revision 2 read, besides the review of revision 1: `scripts/merge-check.sh`;
`.agents/protocol.md`, lines 59 to 94; the eight role adapters under `.claude/agents/` and `.codex/agents/`;
`.agents/README.md`, lines 1 to 56; the archived `flake-defense` inventory, section 3.3; and PR #48's diff against
the base for the five files named in section 3.

## 1. The claimed gap (category 1)

The claim (issue #66): nothing fails when a code pull request merges without the other agent's review.

| Pin | Text |
| --- | --- |
| `AGENTS.md:90` | \| A code pull request (any changed file that is not Markdown or under `openspec/`, …` ending `review only until issue #66 puts a check in `scripts/merge-check.sh` (owner ruling of 2026-10-02: enforce it for code) \| |
| `scripts/merge-check.sh:73` | `read_gh "the rules in force on main" 'type == "array" and all(.[]; type == "object")' \` |
| `scripts/merge-check.sh:77` | `\| select(.type == "required_status_checks")` (the one rule type the script keeps from the answer to its read at line 73) |
| `scripts/merge-check.sh:87` | `read_gh "ruleset ${id}" 'type == "object" and (.enforcement \| type == "string")' \` |
| `scripts/merge-check.sh:108` | `read_gh "labels" 'type == "array" and all(.[]; (.name \| type) == "string")' \` |
| `scripts/merge-check.sh:119` | `read_gh "open ${label} issues" 'type == "array" and all(.[]; (.url \| type) == "string" …` |
| `scripts/merge-check.sh:134` | `read_gh "pull request #${pr}" '(.closingIssuesReferences \| type == "array") and …` |
| `scripts/merge-check.sh:135` | `gh pr view "$pr" --json closingIssuesReferences` |
| `internal/harness/contract/mergecheck_test.go:37` | `*) echo "fake gh: unexpected call: $*" >&2; exit 3 ;;` |
| `openspec/changes/archive/2026-10-01-flake-defense/inventory.md:331` | \| `pull_request` \| `required_approving_review_count: 0`; `allowed_merge_methods: ["squash"]` \| |

The five `read_gh` lines are every read the script makes. Searches and what they found:

- `git grep -n -E 'comments|--json [a-zA-Z,]*body|pulls/|compare/' -- scripts .github Taskfile.yml internal`: two
  hits, both prose about Go comments in `internal/harness/pindiff/` (`rewrite.go:17`, `rewrite_test.go:8`). No script,
  job or test reads a pull request's description, its changed files, its comments or a comparison of two commits.
- `grep -n -i review openspec/specs/merge-gate/spec.md`: lines 63 and 71, both the phrase "checked in review only".
  The `merge-gate` capability has no requirement about review.
- GitHub has its own home for "a review is needed before a merge", and today it asks for none. The ruleset on `main`
  (`24272345`, `main-required-checks`, `active`, the only ruleset; `main` has no classic branch protection) holds a
  `pull_request` rule beside its `required_status_checks` rule
  (`gh api repos/C360Studio/semengine/rules/branches/main`, read 2026-10-02T16:21Z). Its parameters:
  `required_approving_review_count: 0`, `required_reviewers: []`, `require_code_owner_review: false`,
  `require_last_push_approval: false`, `dismiss_stale_reviews_on_push: false`,
  `required_review_thread_resolution: false`, `dismissal_restriction.enabled: false`,
  `require_extra_approval_for_unattributed_changes: true` and `allowed_merge_methods: ["squash"]`. The script
  already receives this rule in the answer to its read at line 73 and drops it at line 77. An approval count of 0
  asks for no review: none of the 20 pull requests has a GitHub review, `reviewDecision` is null on each, and 17 of
  them merged (GraphQL `reviews.totalCount` and `reviewDecision`, read 16:24Z). Sections 6 and 9 say what the
  parameters mean.
- No tracked file is a `CODEOWNERS` file or a pull request template
  (`git ls-tree -r --name-only 94ccedd | grep -i -E 'codeowners|pull_request_template'`: no result).
- One pull request description has a line that starts `reviewed-by:`: #48's, added by an edit of the description at
  2026-10-02T16:11:00Z (section 2.3). The other 19 have none (`gh pr list --state all`, 20 read at 16:22Z; #65 and
  #67 name the line inside a sentence).

The gap is as claimed: no command fails such a merge, and GitHub's rule does not hold it back.

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
- `.claude/agents/` holds four `.md` files whose front matter sets `tools` and, in three of the four, `model` (for
  example `.claude/agents/semengine-reviewer.md:4-5`; the architect's has no `model` line). The six `SKILL.md`
  files under `.claude/skills/` and `.agents/skills/` carry only `name`, `description` and, once, `argument-hint`.
  The definition counts skills, contracts, `AGENTS.md` and the protocol itself as documents.
- What the definition gives for every merged pull request, sorted by hand from each file list (new and previous
  names; `gh api --paginate repos/C360Studio/semengine/pulls/<n>/files`): documents only for #1, #23, #39, #47, #55,
  #56, #58 and #65; code for #12, #13, #14, #21, #43, #44, #54, #59 and #63. #21 is code by one file,
  `docs/admission-ledger.yaml`. Open: #48 is code; #67 changes six files at `0f5e63b`, all under `openspec/`, so it
  is documents only so far.

### 2.2 Which agent implemented

| Pin | Text |
| --- | --- |
| `.agents/protocol.md:59` | review; no later content commit bypasses the archive/spec-sync check. State `implemented-by: <model or persona>` in |
| `.agents/protocol.md:65` | the pull request's `implemented-by:` line records. The owner ruled on 2026-10-02 (issue #64) that Codex's |
| `.agents/protocol.md:72` | workflow; `docs/admission-ledger.yaml`. If both agents wrote commits, the owner names the reviewer on the issue. A |
| `.agents/protocol.md:73` | `pull request neither agent wrote (a dependency bot's) is reviewed by either agent's reviewer, and the record names` |
| `.agents/protocol.md:16` | - **Who has claimed what:** a **draft PR** on an agent-prefixed branch (`claude/…`, `codex/…`) opened at the start |
| `AGENTS.md:124` | `GITHUB_ACTIONS` unset; `implemented-by: <model or persona>` in the PR body; the archive/spec sync is the last |
| `.claude/agents/semengine-reviewer.md:5` | `model: opus` |
| `.codex/agents/semengine-reviewer.toml:3` | `model = "gpt-6-astra"` |
| `.codex/agents/semengine-developer.toml:3` | `model = "gpt-6-sol"` |
| `.agents/README.md:39` | \| Reviewer \| `opus` \| `gpt-6-astra`, `high` \| |
| `.agents/README.md:45` | `The table does not rank Claude's models against Codex's; across agents the pairing is the owner's decision. For a` |

Five things say who implemented a pull request. Measured on all 20 pull requests
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
   author is `cglusky` on every one. A commit with no trailer is therefore not known to be Codex's: on #48, 58 of
   the 63 commits carry Claude's trailer, and the five without are its merges of `origin/main`, among them the head
   and the commit the first Codex record read.
4. **The pull request's author.** `cglusky`, type `User`, on every pull request an agent opened; `dependabot[bot]`,
   type `Bot`, on #14.
5. **A model name, which only the role adapters tie to an agent.** Most `implemented-by:` values are model names:
   `opus`, `gpt-6-sol`, `gpt-6-astra`, and `fable` in 17 of the 20. The files that say which model an agent's role
   runs on are the eight adapters (`.claude/agents/*.md`, `.codex/agents/*.toml`) and the routing table at
   `.agents/README.md:34-40`; the list at `.agents/README.md:9-24` says which adapter is which agent's. They give
   `opus` to three of Claude's roles (its architect adapter has no `model` line) and `gpt-6-sol` or `gpt-6-astra`
   to Codex's four. They map a role to a model, not a model to an agent, and `fable` is in no tracked file outside
   the archive (`git grep -n -i fable 94ccedd -- . ':!openspec/changes/archive'`: no result).

An agent's own name is written where the reviewer is meant, not the implementer: `codex` in #48's `reviewed-by:`
line, "Codex" in the headings of #48's two requests and in Codex's two records (section 2.3). In `implemented-by:`
lines it appears only on #59 and #1.

The owner's global instructions for Claude sessions (outside this repository, so not a pin) ask for
`implemented-by: sonnet|opus|fable`: like the adapters, a model name, with no agent name.

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

How the record looks in practice (measured: `gh api --paginate repos/C360Studio/semengine/issues/<n>/comments` for
each of the 20 pull requests, read 2026-10-02T16:25Z; #48 read again at 16:32Z, head `6bf9809`):

- **One login.** All 89 comments, 35 of them on #48, are by `cglusky`, type `User`. The owner and both agents post
  under it. The archived `flake-defense` design already records this ("One login", decision D8). No field of a
  comment says which agent wrote it. None of the 89 has been edited (`created_at` equals `updated_at`).
- **The heading `Review record` is used by the implementing agent's own reviewer and by the other agent.** 19
  comments have it: one each on #13, #21, #63 and #67, two on #47, three on #65, and five each on #48 and #54. Two
  are the other agent's (next item). The other 17 are on pull requests from `claude/` branches, start
  `## Review record`, and do not name Codex; 15 say "independent `semengine-reviewer`" in the heading. By their text
  they are reports of the implementing session's own reviewer, which the protocol calls an early check, not the
  gate (`protocol.md:75-77`). On #48 those are 5939311446, 5942129322 and 5950922103; a fourth verdict there has
  another heading (5954106306, `## Task 2.9: harness review verdict, PASS …`). After the two words comes nothing,
  an opening parenthesis, a dash or a colon.
- **The two records by the other agent** are Codex's, both on #48, and they differ in shape.
  - Comment 5956023752 (15:48:41Z) starts "## Review record — Codex `semengine-reviewer`". Its first paragraph
    names the commit it read in full ("at `0a86a9a6a9853e427bee9aab5c52e609b9a4c0ea`") and that commit's CI run
    (37026066644). It holds two more 40-character commit ids that are not the commit read: the SemStreams pin
    `8b99efe9…`, which is not a commit of this repository, and `cc55af64…`, a later head it says it does not
    cover. It covers six named commits, not the whole diff. Its first paragraph says "PASS with two non-blocking
    corrections" and its last line "APPROVE — targeted scope only."
  - Comment 5956732582 (16:30:22Z) answers the re-check request below. Its first line is "Reviewer: Codex
    `semengine-reviewer`."; the heading is its third line and one level higher, `# Review record`. It names a
    range in full, `0a86a9a6…c0ea..6bf98097…13f0`, of which the second id is the commit read, and CI run
    37031603674. Its verdict is `CHANGES REQUESTED` (three BLOCKING findings, two HIGH). It says it "does not
    satisfy full-diff hold 7.1". It is 263 lines long and includes Go sources in fenced blocks.
- **Requests.** #48 has two. Comment 5955724059 (15:32:14Z) is headed `## Review request for Codex …` and names the
  head of that time in full, on a line that starts "Head:". Comment 5956384842 (16:11:02Z) is headed "## Re-check
  request for Codex `semengine-reviewer`, at `6bf98097f03ebb4dc701728505ebc37fafc113f0`", so it does not carry the
  words `Review request` (`protocol.md:78`).
- **Reviews across agents before the rule.** #59 and #1 are from `codex/` branches and were reviewed by Claude
  sessions. #59 has three comments headed `## Leg 2: …` (5954707106, 5954724646, 5955366913), each naming its commit
  as a short id in the heading. #1 has comment 5915195795, with no heading, which names the reviewed head and the
  base in full. None is headed `Review record`; all predate PR #65.
- **How commits are named.** As short ids in prose, headings and tables, several in one comment (comment 5954106306
  names seven, among them `912d82a` and `030e85e`). Of the 19 records only Codex's two name the commit read in
  full. A run of 40 hexadecimal characters in a record is not always that commit: the first Codex record holds
  three, the second a range, and three records (two on #47, one on #67) hold a sha256, whose first 40 characters
  look the same.
- **Verdict words** vary by review mode: `APPROVE`, `PASS`, `DESIGN PASS`, `CHANGES REQUESTED`,
  `INVENTORY CHANGES REQUESTED`, `APPROVE both`, and the two phrases of the first Codex record; often one row per
  round in a table.
- **The one `reviewed-by:` line** is in #48's description, added by the edit of 16:11:00Z (GraphQL
  `userContentEdits`: the eighth edit and, at 16:32Z, the last). It reads: "reviewed-by: codex semengine-reviewer —
  targeted (3.1–3.3 fixes, 2.11 re-check) APPROVE at `0a86a9a` (comment 5956023752); full-diff review (hold 7.1)
  pending." It starts the line, and names the agent in lower case, a short commit id that is not the head, a
  comment id, a partial scope and the word "pending". At 16:32Z it still said so, two minutes after the second
  record said `CHANGES REQUESTED` at the head: the implementer writes the line after a record, so the two can
  differ. Before the rule, four `implemented-by:` lines also said who reviewed (#12, #55, #56 and #59; on #55
  "…; reviewed by fable (orchestrating session)").
- **A third home for "a review happened":** OpenSpec task truth. Tasks are ticked with a comment id, for example
  `openspec/changes/archive/2026-10-02-carry-check/tasks.md:15-17` ("DESIGN PASS … recorded on this pull request
  (comment 5952034096)").
- **PR #48 at `6bf9809`** is the first live case of `protocol.md:87-90`. Its head is a merge of `main` (`94ccedd`,
  PR #65) made after the first Codex record. The description lists, under "Awaiting Codex re-check", the fix
  commits, tasks 3.4 and 3.5 and that merge; the list "Not yet reviewed" that revision 1 saw was replaced by the
  edit of 16:11:00Z. The description and the re-check request say the merge touched two files #48 also changes,
  `AGENTS.md` and the reviewer contract. Measured here there are three; the developer contract is the third
  (`git diff --name-only cd7c49d 94ccedd`, the eight files the merge brought from `main`, set against #48's 114).
  What is reviewed, what is not, and which files need a re-review live in prose in the description.

### 2.4 What GitHub answers to a read (measured)

| Read | Answer | Limit |
| --- | --- | --- |
| `gh api repos/C360Studio/semengine/pulls/48` | one object: `draft` (true), `changed_files` (114), `head.sha` (40 characters), `base.ref`, `user.type` (`User`), `body` (a string; no carriage return in any of the 20 descriptions) | none |
| the same for #14 | `user.login` `dependabot[bot]`, `user.type` `Bot`, `draft` false | none |
| `gh api --paginate 'repos/…/pulls/48/files?per_page=100'` | `gh` 2.97 joins the pages into one array of 114 entries; with `--slurp`, an array of two pages (100 and 14). Each entry has `filename` and `status` (96 `added`, 18 `modified`); none has `previous_filename`, which GitHub documents for a renamed file. The 114 names are those of `git diff --name-only 94ccedd...6bf9809` | documented: 3000 files; not measured |
| the same with `--slurp` for a pull request that changes no file | `[[]]` (revision 1's read of #67 at `4e35ce0`; not repeated, because #67 now changes six files and no other pull request is empty) | none |
| `gh pr view 48 --json changedFiles,files` | `changedFiles` 114, `files` lists 100 and says nothing | 100, silent |
| `gh api --paginate 'repos/…/issues/48/comments?per_page=10'` | one array of 34 at 16:27Z (four pages joined; the 35th comment came at 16:30Z); each entry has `id`, `body`, `user`, `created_at`, `updated_at`, `html_url` | none seen |
| `gh api repos/…/compare/0a86a9a6…c0ea...6bf98097…13f0` (the commit the first Codex record read, against the head) | `status` `ahead`, `ahead_by` 12, `total_commits` 12, `files` 34 entries with `filename`. 29 are files #48 changes, three of them changed on `main` too (section 2.3); the other five came in with the merge of `main` and are not #48's. No field says which side changed a file, and none says the file list was cut | documented: 300 files, on the first page only; not measured (see the note under this table) |
| the same with short ids `0a86a9a...6bf9809` | the same answer | none |
| the same with a commit id that does not exist | HTTP 404, `gh` exits 1 | none |
| GraphQL `pullRequest { viewerDidAuthor reviewDecision reviews { totalCount } userContentEdits { … } }` for all 20 | for the login the sessions use (`cglusky`), `viewerDidAuthor` is true on 19 and false on #14; `reviewDecision` is null and `reviews.totalCount` 0 on all 20; `userContentEdits` gives the time, the editor and the text of each edit of a description (eight on #48) | none seen |

The 300-file limit of a comparison is documented, not measured. GitHub's page for the read says the list of changed
files "is only shown on the first page of results, and it includes up to 300 changed files for the entire
comparison". Revision 1 measured the limit on `golang/go`, a repository `docs/inventory-scope.md` does not list;
that measurement is withdrawn as out of scope and its result is not used here. The limit cannot be measured in this
repository today: its largest tree, #48's head, has 251 files, and the first commit against that head differs in all
251 (`git diff --name-only cff360f 6bf9809 | wc -l`; 155 against `main`). So whether an answer that was cut says so
is not known (section 9, N1).

The count of #48's changed files was 95, 101 and 113 in revision 1's reads and 114 in this one's, because the
branch is pushed to between them. Two reads inside one run can disagree for the same reason.

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
- GitHub refuses to merge a draft (documented, not measured). This is assumption A15 of `flake-defense`, which that
  change left unmeasured and recorded so on #42 (comment 5939821065); #42 is closed.
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
| `internal/harness/contract/mergecheck_test.go:78` | ``otherRules = `{"type":"deletion","ruleset_id":24272345},{"type":"non_fast_forward",…`` (the fake's answer for the rules on `main` holds no `pull_request` rule; the live answer holds one) |
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
change between two runs. No other open issue names the merge check or the review rule
(`gh issue list --state open --limit 200 --json number,title,body`: 30 issues read at 16:32Z).

**Admission ledger.** Not a port; no row.

**Open pull requests** (two, from `gh pr list --state all --json number,state,isDraft,changedFiles,…` at 16:22Z.
PR #48 has more than 100 files, so its list was read with
`gh api --paginate repos/C360Studio/semengine/pulls/48/files`).

| PR | State | Files | Overlap with the files this question touches |
| --- | --- | --- | --- |
| #48 `claude/setup-04a-01-floor` | draft, code, 114 files at `6bf9809` | port of the floor | `AGENTS.md`, `docs/testing.md`, `docs/repository-map.md`, `.agents/contracts/semengine-developer.md`, `.agents/contracts/semengine-reviewer.md`; and three other test files of the Go package that holds the two merge-gate tests (`boundaries_test.go`, `signatures_test.go`, `testtext_test.go` in `internal/harness/contract/`). It does not change `scripts/merge-check.sh`, either merge-gate test file, `.github/`, `Taskfile.yml`, `.agents/protocol.md`, `.agents/README.md`, a role adapter or the preflight skill, and it has no delta under `openspec/changes/setup-04a-01-floor/specs/merge-gate/` (its deltas: `background-work`, `harness-boundaries`, `lifecycle-suite`, `nats-fixture`, `process-host`, `transport-client`) |
| #67 `claude/review-gate-check` | draft, six files at `0f5e63b`, all under `openspec/changes/review-gate-check/` | this claim | itself |

**What PR #48 claims on this territory** (pins at `6bf9809`; the first four texts were already in #48, a few lines
higher, at `8a6f7ce`, the head revision 1 read):

| Pin, at `6bf9809` | Text |
| --- | --- |
| `.agents/contracts/semengine-reviewer.md:314` | A PASS or `APPROVE` names the CI run and the commit it rests on: a step is done only when the CI run for its pushed |
| `.agents/contracts/semengine-reviewer.md:316` | `green.` (the end of the paragraph #48 adds to "Finding and verdict format", after base line 301) |
| `AGENTS.md:83` | \| A step is done only when its pushed commit's CI run has passed, and a reviewer's PASS names that run and commit \| developer contract § Handoff; reviewer contract § Finding and verdict format \| CI job `required`; naming the run in a verdict is checked in review only \| |
| `.agents/contracts/semengine-developer.md:231` | A step is done only when the CI run for its pushed commit has passed. A local `task verify` is evidence, not the gate. |
| `AGENTS.md:96` | \| A code pull request (any changed file that is not Markdown or under `openspec/`, … (the review rule's row, base line 90) |

- **A second rule about what a review verdict names.** #48 adds to the reviewer contract that a PASS or `APPROVE`
  names the CI run and the commit it rests on, and indexes it in `AGENTS.md` as checked "in review only". That is a
  rule on the content of a review record (fact 3 of this inventory), held in an open pull request and not on
  `main`. Section 7 notes that a record's format reaches the reviewing agent through this contract. Both Codex
  records on #48 name a CI run and a commit (section 2.3).
- **It moves the lines this change must edit.** #48 adds six lines to `AGENTS.md` above them: the review rule's row
  moves from `:90` to `:96` and the Merge gate from `:121-128` to `:127-134` (`:45`, `:52` and `:79` become `:46`,
  `:53` and `:85`). Issue #66, "What closes this", asks for that row to change. In the reviewer contract `:300`
  becomes `:311`; `:14` and `:17` do not move.
- **File overlap only:** `docs/repository-map.md` (line 58, the ledger row; line 46, the merge-check row, is
  untouched), `docs/testing.md` (no hunk moves or changes line 160) and the developer contract (line 46 is
  untouched).

Which of #48 and this change merges first is for the design to say.

PR #48 is also the first pull request the rule will meet. It is a code pull request by Claude sessions whose
`implemented-by:` line names no agent. It has two records by Codex, an approval of six commits at a commit 12 behind
the head and `CHANGES REQUESTED` on the range up to the head, and a `reviewed-by:` line that names only the first
(section 2.3).

## 4. Present consumers (category 4)

What reads or runs the surfaces in question today:

| Surface | Present consumer |
| --- | --- |
| `scripts/merge-check.sh <n>` | `.github/workflows/ci.yml:86` (`run: scripts/merge-check.sh "$PR_NUMBER"`); `Taskfile.yml:98`; the Land step, `.agents/protocol.md:56` |
| the job `merge-check` | `.github/workflows/ci.yml:92` (`needs: [verify, merge-check]`) |
| the line `implemented-by:` | readers of the pull request; `.agents/protocol.md:65` names it as the record of who wrote the commits. No command reads it |
| the line `reviewed-by:` | one instance, in #48's description (section 2.3). Its readers are the owner and the sessions, by eye. No command reads it |
| a comment headed `Review record` | 19 instances, two from the other agent (section 2.3). Its readers are the implementing session, which copies the result into the description and the task list, and the owner, by eye. No command reads it |
| the `pull_request` rule of ruleset `24272345` | GitHub, when a merge is asked for: it allows only a squash merge and asks for no review. `scripts/merge-check.sh:73` receives the rule and `:77` drops it |

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
| Owners | The protocol (the rule, the two comment headings, the `reviewed-by:` line); the reviewer contract (`semengine-reviewer.md:14-20`, verdict words at `:300`); pending in open PR #48, the reviewer contract's rule that a PASS or `APPROVE` names the CI run and the commit, with its `AGENTS.md` row (section 3); OpenSpec task truth (review tasks ticked with a comment id); on GitHub, the `pull_request` rule of ruleset `24272345` on `main`, in force and asking for no review (section 1), and GitHub's review objects, of which none exists |
| Catalogs | No file or registry lists reviews; the pull request's conversation is the only store, and GitHub's own list of a pull request's reviews (`pulls/<n>/reviews`) is empty on all 20. `git grep -n -i 'review record'` outside the archive: `protocol.md:61`, `:81`, `semengine-reviewer.md:17`. For agent identity: the eight role adapters and `.agents/README.md:9-24` and `:34-40`, which map a role to a model and not a model to an agent (section 2.2) |
| Status | The check `Required` (`ci.yml:90-105`) and the job `merge-check`, neither of which reads review state today. GitHub's own status for the class is `reviewDecision`, null on every pull request, and the merge button, which the `pull_request` rule does not hold back while its count is 0. In prose: the `reviewed-by:` line and the "Awaiting Codex re-check" item in #48's description |
| Lifecycle | Created by a comment; "covers the commit it names" (`protocol.md:87`); kept across a merge of `main` that changed none of the pull request's files (`:88-90`); a later content commit needs a new one; a rebase strands it (`:60-62`). Nothing says what an edited comment means (none has been edited so far). GitHub's rule has two lifecycle settings of its own, both off: `dismiss_stale_reviews_on_push` (documented: "New, reviewable commits pushed will dismiss previous pull request review approvals") and `require_last_push_approval` (documented: "the most recent reviewable push must be approved by someone other than the person who pushed it"). A GitHub review records the commit it was made on (`commit_id`, documented) |
| Ownership | One GitHub login for the owner and both agents (section 2.3). The branch prefix names the claimer (`protocol.md:34-39`). No per-agent identity exists on GitHub. To GitHub that login is the author of every pull request an agent opened (`viewerDidAuthor`, section 2.4), and GitHub documents that "Pull request authors cannot approve their own pull requests" (not measured: section 9, N10). The rule names no reviewer (`required_reviewers: []`, `require_code_owner_review: false`, no `CODEOWNERS` file), and the ruleset has no bypass actor (`bypass_actors: []`, `current_user_can_bypass: "never"`, read 16:21Z) |
| Readers | People and agent sessions, by eye: the owner; the reviewing session, which reads the request; the implementing session, which copies a record's result into the `reviewed-by:` line and the task list. No command (section 1 searches). GitHub reads its own rule when a merge is asked for |
| Writers | The implementing session (requests; the `implemented-by:` and `reviewed-by:` lines of the description); the reviewing session (records: Codex's two on #48 are the first under the rule); the owner (rulings on the issue). All under one login. The ruleset was last changed at 2026-10-01T19:51Z (`updated_at`), the day a session set its strict setting on the owner's ruling (`flake-defense` design, Q3, `design.md:1084`) |
| Recovery | For the script's existing reads: exit 2 naming the read (`merge-check.sh:56-67`), and a re-run reads again. For review records: nothing defined. For GitHub's rule: nothing here uses it; GitHub documents that, with stale-review dismissal on, a dismissed approval must be given again before the merge |

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

The one live case shows two of these carried by hand. #48's implementer listed two of the three files `main` also
changed, and the second Codex record has its heading on the third line, one level higher than the first (section
2.3).

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
| N1 | A comparison of two commits lists at most 300 files, and an answer that was cut does not say so | the limit is documented (section 2.4); the silence is not on the page read. Neither is measured: revision 1's measurement on another repository is withdrawn, and no comparison in this repository reaches 300 files (its largest tree has 251) |
| N2 | The list of a pull request's files stops at 3000 | documented; no pull request here is near it |
| N3 | Listing `ready_for_review` under `pull_request` starts a run when a draft is marked ready, and the list is taken from the pull request's own copy of the workflow | the activity type is documented; the second half is revision 1's label and was not re-read; never tried here |
| N4 | GitHub refuses to merge a draft | documented; not measured. It is assumption A15 of `flake-defense`, left unmeasured there and recorded so on #42 (comment 5939821065), which is closed |
| N5 | The `gh` on the CI runner joins paginated pages into one array as 2.97 does locally, and accepts `--slurp` | the runner's version is not recorded; the image is unpinned (#61) |
| N6 | The job's token (`contents`, `issues` and `pull-requests`, all read) may read a pull request, its files, its comments and a comparison of two commits | documented per endpoint by revision 1's label; the permission sections were not re-read; the job has made none of these reads |
| N7 | Whether Dependabot rewrites a description a session has added lines to | not seen either way: a session added #14's line at 11:26:54Z on 2026-10-02 and #14 merged three seconds later. Dependabot had edited that description four times before, last at 11:23:19Z (GraphQL `userContentEdits`) |
| N8 | Whether a description edited in the browser carries carriage returns | revision 1 called this documented; no page read for this revision says it. None of the 20 descriptions and none of the 89 comments has one |
| N9 | Whether anyone will open a pull request written by hand, by neither agent and not by a bot | none of the 20 is one |
| N10 | Whether GitHub would refuse an approving review of a pull request from the login that opened it, the one login the owner and both agents use | documented (section 6, Ownership); not measured, because the measurement is to post a review |
| N11 | What `require_extra_approval_for_unattributed_changes: true` does | the REST reference page for rules does not list the parameter. The rulesets guide describes a setting "Require an additional approval for unattributed Copilot pull requests", on by default, which "has no effect if the ruleset requires zero approvals". That the parameter is that setting is inferred from its name |
| N12 | Whether the next record from the other agent will have its heading on the first line, at which level, and with one commit or a range | the two so far differ (section 2.3). The tree sets the heading's words and what a record names (`protocol.md:81-82`), not where the heading stands or how a commit is written |
