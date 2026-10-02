# merge-gate

## ADDED Requirements

### Requirement: Cross-agent review check

On a pull-request run `scripts/merge-check.sh` SHALL apply a review check: a code pull request that is not a draft
passes only when its description and its comments record a review by the agent that did not implement it, of the
commit that is its head or of a commit whose record carries over to the head. A documents-only pull request passes
without one. The check SHALL be applied whatever the base branch of the pull request and whatever the known-flake
check and the up-to-date rule find, and the script SHALL exit zero only when none of the three fails. There SHALL be no
waiver: no label, no comment other than a review record, and no environment variable SHALL change the result, other
than the two variables that select the kind of run. On a push run the script SHALL NOT apply the check, SHALL say so,
and SHALL NOT read a pull request, its files or its comments.

**Documents only.** The script SHALL read every page of the pull request's list of changed files and the number of
changed files GitHub reports for it. Each entry gives one name, and a renamed file gives two: its new name and its
previous name. A name is a document name when it ends in `.md` or starts with `openspec/`, and does not start with
`.claude/agents/`. Every other name is a code name, including the name of a file the pull request deletes. A pull
request with at least one code name among the entries read is a code pull request. A pull request is documents only
when the entries read number exactly what GitHub reports and none of their names is a code name; a pull request that
changes no file is documents only. When the entries read are fewer or more than GitHub reports and none of their
names is a code name, the script SHALL treat the pull request as a code pull request and say the list may be
incomplete.

**The two lines.** For a code pull request the description SHALL have exactly one line that starts `implemented-by:`
and exactly one line that starts `reviewed-by:`. A carriage return at the end of a line is ignored. An agent name is
the word `claude` or the word `codex`, in any letter case, with no letter or digit directly before or after it. The
`implemented-by:` line SHALL name at least one agent, unless GitHub reports the pull request's author as a bot. The
`reviewed-by:` line SHALL name exactly one agent, the reviewing agent. The reviewing agent SHALL NOT be one the
`implemented-by:` line names, unless that line names both agents.

**The review record.** A review record is a comment on the pull request's conversation whose first line, after any
`#` characters and spaces, starts with `Review record`, and which has a line that starts `reviewed-by:` and names
exactly one agent, and a line that starts `commit:` followed by the 40 characters of a commit id in lower case, with
or without backticks around them. The first such line of each kind is the one read. The check passes when a review
record that names the reviewing agent names the head commit. Failing that, the script SHALL consider only the most
recently created review record that names the reviewing agent, and the check passes when that record carries over.

**Carrying over.** A record that names a commit other than the head carries over when all of these hold, and not
otherwise: GitHub's comparison of that commit with the head reports the head as ahead of it; the comparison lists
fewer than 300 files; the list of the pull request's changed files is complete; and no name in the comparison, new or
previous, is a name in the pull request's list.

**Drafts.** While the pull request is a draft the script SHALL make the same reads and print the same findings, SHALL
say that they do not fail a draft, and SHALL NOT exit non-zero because of them.

**Reads.** The script SHALL exit non-zero, naming the read, when a read of the pull request, its files, its comments
or the comparison fails or returns something other than what was asked for, and when a step that sorts or compares
names fails. This holds for a draft too. None of these is ever treated as documents only, as a record that carries
over, or as a pass.

**What is printed.** The script SHALL print whether the pull request is documents only or a code pull request, and
for a code pull request at least one of its code names. For each finding it SHALL print what is missing and the line
or record to add. When the check passes on a record it SHALL print the commit the record names and whether that is
the head or was carried over.

#### Scenario: Documents only

- **WHEN** a pull request that is not a draft changes `docs/a.md` and `openspec/config.yaml`, and its description has
  neither line
- **THEN** the script exits zero and says the pull request is documents only

#### Scenario: One code file among documents

- **WHEN** a pull request that is not a draft changes `docs/a.md` and `scripts/x.sh`, and its description has no
  `reviewed-by:` line
- **THEN** the script exits non-zero, names `scripts/x.sh`, and says a `reviewed-by:` line is missing

#### Scenario: Role adapter

- **WHEN** the only file a pull request changes is `.claude/agents/semengine-reviewer.md`
- **THEN** the script treats it as a code pull request and names that file

#### Scenario: Names near the rule

- **WHEN** a pull request changes exactly one file, and that file is in turn `README.MD`, `docs/a.md.txt`,
  `openspecs/a.yaml`, `docs/openspec/a.yaml`, `Taskfile.yml`, `docs/admission-ledger.yaml`, `AGENTS.md`,
  `.claude/skills/preflight/SKILL.md`, `x/.claude/agents/a.md` and `openspec/changes/x/.openspec.yaml`
- **THEN** the script treats the first six as code pull requests and the last four as documents only

#### Scenario: Renamed file

- **WHEN** the only entry in the list renames `scripts/x.sh` to `docs/x.md`
- **THEN** the script treats the pull request as a code pull request and names `scripts/x.sh`

#### Scenario: Deleted file

- **WHEN** the only entry in the list deletes `scripts/x.sh`
- **THEN** the script treats the pull request as a code pull request and names `scripts/x.sh`

#### Scenario: Code file on the second page

- **WHEN** a pull request changes 101 files, the first 100 listed are Markdown files and the last is `go.mod`
- **THEN** the script treats it as a code pull request and names `go.mod`

#### Scenario: File list cut short

- **WHEN** GitHub reports 101 changed files, the list read has 100 entries, and every one is a Markdown file
- **THEN** the script does not treat the pull request as documents only, and says the list has 100 of 101 files and
  may be incomplete

#### Scenario: No changed file

- **WHEN** GitHub reports no changed file and the list read is empty
- **THEN** the script exits zero and says the pull request is documents only

#### Scenario: No reviewed-by line

- **WHEN** a code pull request that is not a draft has the line `implemented-by: claude (opus)` and no `reviewed-by:`
  line
- **THEN** the script exits non-zero and says to add a line `reviewed-by: codex`

#### Scenario: No implemented-by line

- **WHEN** a code pull request that is not a draft has no `implemented-by:` line
- **THEN** the script exits non-zero and says an `implemented-by:` line is missing

#### Scenario: Implementer line names no agent

- **WHEN** a code pull request whose author is a user has the line `implemented-by: opus (semengine-developer)`
- **THEN** the script exits non-zero and says the line names neither `claude` nor `codex`

#### Scenario: Same agent implements and reviews

- **WHEN** a code pull request has the lines `implemented-by: claude-fable-5-1` and `reviewed-by: Claude (opus)`,
  and a review record that names the head with `reviewed-by: claude`
- **THEN** the script exits non-zero and says the other agent reviews

#### Scenario: Reviewer line names no agent or both

- **WHEN** a code pull request has the line `reviewed-by: gpt-6-astra`, or the line `reviewed-by: codex, then claude`
- **THEN** the script exits non-zero and says the line must name exactly one of `claude` and `codex`

#### Scenario: Longer word that holds an agent name

- **WHEN** a code pull request has the lines `implemented-by: claude` and `reviewed-by: codex2`
- **THEN** the script exits non-zero and says the `reviewed-by:` line names neither `claude` nor `codex`

#### Scenario: Both agents implemented

- **WHEN** a code pull request has the lines `implemented-by: codex (gpt-6-sol); claude (review fixes)` and
  `reviewed-by: claude`, and a review record that names the head with `reviewed-by: claude`
- **THEN** the script exits zero

#### Scenario: Pull request by a bot

- **WHEN** a code pull request whose author GitHub reports as a bot has the lines `implemented-by: dependabot` and
  `reviewed-by: codex`, and a review record that names the head with `reviewed-by: codex`
- **THEN** the script exits zero

#### Scenario: Line written twice

- **WHEN** the description of a code pull request has two lines that start `reviewed-by:`
- **THEN** the script exits non-zero and says there are two

#### Scenario: Line that does not start the line

- **WHEN** the description of a code pull request has `- reviewed-by: codex` as a list item, or names `reviewed-by:`
  in the middle of a sentence, and has no line that starts with it
- **THEN** the script exits non-zero and says a `reviewed-by:` line is missing

#### Scenario: Carriage returns

- **WHEN** every line of the description ends in a carriage return and a line feed, the two lines are otherwise as
  required, and a review record names the head
- **THEN** the script exits zero

#### Scenario: Record names the head

- **WHEN** a code pull request that is not a draft has the lines `implemented-by: claude` and `reviewed-by: codex`,
  and a comment whose first line is `## Review record: implementation review` with the lines `reviewed-by: codex` and
  `commit:` followed by the head commit's 40 characters
- **THEN** the script exits zero and prints that commit and that it is the head

#### Scenario: Commit in backticks

- **WHEN** the same record writes the commit id between backticks
- **THEN** the script exits zero

#### Scenario: No record

- **WHEN** a code pull request that is not a draft has both lines as required and no comment whose first line starts
  with `Review record`
- **THEN** the script exits non-zero and says a review record by `codex` is missing and which two lines it must
  carry

#### Scenario: Record by the implementing agent's own reviewer

- **WHEN** the lines are `implemented-by: claude` and `reviewed-by: codex`, and the only review record names the head
  with `reviewed-by: claude`
- **THEN** the script exits non-zero and says a review record by `codex` is missing

#### Scenario: Record without a full commit id

- **WHEN** the only comment headed `Review record` has `reviewed-by: codex` and either no `commit:` line or a
  `commit:` line with a seven-character id
- **THEN** the script exits non-zero and says the record must name a commit by its 40 characters

#### Scenario: Heading not on the first line

- **WHEN** the only comment that has both lines mentions `Review record` in its text but starts with another line
- **THEN** the script exits non-zero and says a review record by `codex` is missing

#### Scenario: Earlier record and a later one that names the head

- **WHEN** two review records by `codex` exist, the earlier names an earlier commit and the later names the head
- **THEN** the script exits zero and makes no comparison

#### Scenario: Newest record is the one carried over

- **WHEN** two review records by `codex` name two earlier commits, neither is the head, and the comparison of the
  later record's commit with the head lists only files the pull request does not change
- **THEN** the script exits zero, and the one comparison it reads is that of the later record's commit

#### Scenario: Main merged and none of the pull request's files touched

- **WHEN** the record names an earlier commit, the comparison reports the head as ahead of it, and the comparison
  lists `pkg/other.go` and `docs/other.md`, neither of which the pull request changes
- **THEN** the script exits zero and prints the commit the record names and that the record was carried over

#### Scenario: A file the pull request changes differs

- **WHEN** the record names an earlier commit and the comparison lists `scripts/merge-check.sh`, which the pull
  request changes
- **THEN** the script exits non-zero, names `scripts/merge-check.sh`, and says the head needs a review record

#### Scenario: Renamed file in the comparison

- **WHEN** the record names an earlier commit and the only entry of the comparison renames `scripts/x.sh`, which the
  pull request changes, to `scripts/y.sh`
- **THEN** the script exits non-zero and names `scripts/x.sh`

#### Scenario: Head does not contain the recorded commit

- **WHEN** the record names a commit and the comparison reports the head as `diverged` from it or `behind` it
- **THEN** the script exits non-zero and says the head does not contain the commit the record names

#### Scenario: Comparison may be cut short

- **WHEN** the record names an earlier commit and the comparison lists 300 files, none of which the pull request
  changes
- **THEN** the script exits non-zero and says the comparison may be incomplete and the record must name the head

#### Scenario: More files than GitHub lists, record names the head

- **WHEN** GitHub reports 3001 changed files, the list read has 3000 entries of which one is `go.mod`, the two lines
  are as required, and a review record by the reviewing agent names the head
- **THEN** the script exits zero

#### Scenario: More files than GitHub lists, record names an earlier commit

- **WHEN** the same pull request has a review record that names an earlier commit and none that names the head
- **THEN** the script exits non-zero and says the record cannot be carried over because the file list is incomplete

#### Scenario: Draft

- **WHEN** a draft code pull request has no `reviewed-by:` line, no known flake is open and the up-to-date rule
  holds
- **THEN** the script exits zero, prints that a `reviewed-by:` line is missing, and says this does not fail a draft

#### Scenario: Draft and a failed read

- **WHEN** the pull request is a draft and the read of its files fails
- **THEN** the script exits non-zero naming the read

#### Scenario: A read fails

- **WHEN** the read of the pull request, of its files, of its comments or of the comparison fails, or returns
  something other than what was asked for
- **THEN** the script exits non-zero naming the read, and does not say documents only, carried over or ok

#### Scenario: Recorded commit that GitHub does not have

- **WHEN** the record names 40 characters that are no commit of the repository, so that the comparison is refused
- **THEN** the script exits non-zero and names the commit the record gave

#### Scenario: Sorting or comparing names fails

- **WHEN** the step that sorts the changed files into code names, or the step that compares the comparison's names
  with the pull request's, fails
- **THEN** the script exits non-zero, and does not say documents only or carried over

#### Scenario: Open flake and no review

- **WHEN** issue 40 is open with the label `class:flake`, and a code pull request that is not a draft closes no
  flake and has no `reviewed-by:` line
- **THEN** the script exits non-zero and its output names issue 40 and says a `reviewed-by:` line is missing

#### Scenario: Pull request on another base

- **WHEN** a code pull request that is not a draft, whose base is not the default branch, has no `reviewed-by:` line
- **THEN** the script exits non-zero and says a `reviewed-by:` line is missing

#### Scenario: Push run

- **WHEN** `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` is `push`
- **THEN** the script says the review check does not apply and reads no pull request, no file list and no comments

### Requirement: A run when a pull request is marked ready

The CI workflow SHALL start a run for a pull request when it is opened, when it is reopened, when it is pushed to
(`synchronize`) and when a draft is marked ready for review (`ready_for_review`). The review check does not fail a
draft, so the run that starts when the draft is marked ready is the one that applies it to that head. A contract test
SHALL fail when the workflow's `pull_request` trigger is missing, lists no activity types, or lists types without any
one of these four.

#### Scenario: Ready type dropped

- **WHEN** the `pull_request` trigger in `.github/workflows/ci.yml` lists `opened`, `synchronize` and `reopened`
- **THEN** the contract test fails naming `ready_for_review`

#### Scenario: No types listed

- **WHEN** the `pull_request` trigger lists no activity types
- **THEN** the contract test fails naming `ready_for_review`

#### Scenario: Push type dropped

- **WHEN** the `pull_request` trigger lists `opened`, `reopened` and `ready_for_review`
- **THEN** the contract test fails naming `synchronize`

#### Scenario: No pull-request trigger

- **WHEN** the workflow starts on `push` only
- **THEN** the contract test fails saying the `pull_request` trigger is missing
