# merge-gate

## ADDED Requirements

### Requirement: Cross-agent review check

On a pull-request run `scripts/merge-check.sh` SHALL apply a review check. A code pull request that is not a draft
passes only when its description says which agent implemented it and names the other agent as its reviewer. A
documents-only pull request passes without that. The script SHALL exit zero only when the review check, the
known-flake check and the up-to-date rule all pass. There SHALL be no waiver: no label, no comment and no
environment variable SHALL change the result, other than the two variables that select the kind of run. On a push
run the script SHALL NOT apply the check, SHALL say so, and SHALL NOT read a pull request or its files.

**Documents only.** The script SHALL read every page of the pull request's list of changed files, and the number of
changed files GitHub reports for it. Each entry gives one name, and a renamed file gives two: its new name and its
previous name. A name is a document name when it ends in `.md` or starts with `openspec/`, and does not start with
`.claude/agents/`. Every other name is a code name, including the name of a file the pull request deletes. A pull
request with at least one code name is a code pull request. Every other pull request is documents only, one that
changes no file included. When the entries read are not as many as GitHub reports, the script SHALL exit non-zero,
say the list is incomplete and give both numbers, whatever the names it read.

**The two lines.** The description of a code pull request SHALL have exactly one line that starts `reviewed-by:`
and, unless GitHub reports the pull request's author as a bot, exactly one line that starts `implemented-by:`. A
line starts in the first column. An agent name is the word `claude` or the word `codex`, in any letter case, with no
letter or digit directly before or after it. The `implemented-by:` line SHALL name at least one agent. The
`reviewed-by:` line SHALL name exactly one agent, and not an agent the `implemented-by:` line names, unless that
line names both. For a bot's pull request the `implemented-by:` line is not read, and the `reviewed-by:` line may
name either agent. Nothing else in either line is read. A carriage return at the end of a line SHALL NOT change any
of this.

**Drafts.** While the pull request is a draft the script SHALL make the same reads and print the same findings,
SHALL say that they do not fail a draft, and SHALL NOT exit non-zero because of them.

**Reads.** The script SHALL exit non-zero, naming the read, when the read of the pull request or of its files fails
or returns something other than what was asked for, and when the step that sorts the names fails. This holds for a
draft too. None of these is ever treated as documents only or as a pass.

**What is printed.** The script SHALL print whether the pull request is documents only or a code pull request, and
for a code pull request its code names, up to ten of them. For each finding it SHALL print what is missing and the
line to write. When a pull request that is not a draft has a finding, it SHALL also print that editing the
description starts no run, so the job has to be run again. When a code pull request passes it SHALL print the
reviewing agent.

#### Scenario: Documents only

- **WHEN** a pull request that is not a draft changes `docs/a.md` and `openspec/config.yaml`, and its description
  has neither line
- **THEN** the script exits zero and says the pull request is documents only

#### Scenario: One code file among documents

- **WHEN** a pull request that is not a draft changes `docs/a.md` and `scripts/x.sh`, and its description has no
  `reviewed-by:` line
- **THEN** the script exits non-zero, names `scripts/x.sh`, and says a `reviewed-by:` line is missing

#### Scenario: Names near the rule

- **WHEN** one pull request changes `AGENTS.md`, `.claude/skills/preflight/SKILL.md`, `x/.claude/agents/a.md`,
  `openspec/changes/x/.openspec.yaml` and `openspec/x.sh`, and a second changes those five and also `README.MD`,
  `docs/a.md.txt`, `openspecs/a.yaml`, `docs/openspec/a.yaml`, `Taskfile.yml`, `docs/admission-ledger.yaml`,
  `.claude/agents/semengine-reviewer.md`, `.codex/agents/semengine-reviewer.toml`, and deletes `scripts/x.sh`
- **THEN** the script says the first is documents only, and treats the second as a code pull request and names each
  of the nine files that the first does not change

#### Scenario: Renamed file

- **WHEN** the only entry in the list renames `scripts/x.sh` to `docs/x.md`
- **THEN** the script treats the pull request as a code pull request and names `scripts/x.sh`

#### Scenario: Code file on the second page

- **WHEN** a pull request changes 101 files, the first 100 listed are Markdown files and the last is `go.mod`, and
  GitHub's two pages reach the script either joined into one list or as two lists one after the other
- **THEN** the script treats it as a code pull request and names `go.mod`, in both cases

#### Scenario: File list incomplete

- **WHEN** GitHub reports 101 changed files, the list read has 100 entries, and every one is a Markdown file
- **THEN** the script exits non-zero, says the list has 100 of 101 files, and does not say documents only

#### Scenario: No changed file

- **WHEN** GitHub reports no changed file and the list read is empty
- **THEN** the script exits zero and says the pull request is documents only

#### Scenario: The other agent is named

- **WHEN** a code pull request that is not a draft has the line `implemented-by: claude (opus)` and the line
  `reviewed-by: codex semengine-reviewer — targeted (3.1–3.3 fixes) APPROVE at 0a86a9a; full-diff review pending.`
- **THEN** the script exits zero and prints `codex` as the reviewing agent; what follows the agent's name is not
  read

#### Scenario: No reviewed-by line

- **WHEN** a code pull request that is not a draft has the line `implemented-by: claude (opus)` and no
  `reviewed-by:` line
- **THEN** the script exits non-zero, says to add a line `reviewed-by: codex`, and says that editing the description
  starts no run

#### Scenario: Implementer line missing or naming no agent

- **WHEN** a code pull request whose author is a user has no `implemented-by:` line, or has the line
  `implemented-by: opus (semengine-developer); fable (orchestrating session)`
- **THEN** the script exits non-zero and says the `implemented-by:` line must name `claude` or `codex`

#### Scenario: Same agent implements and reviews

- **WHEN** a code pull request has the lines `implemented-by: claude-fable-5-1` and `reviewed-by: Claude (opus)`
- **THEN** the script exits non-zero and says the other agent, `codex`, reviews

#### Scenario: Reviewer line names no agent, both, or a longer word

- **WHEN** a code pull request has the line `implemented-by: claude`, and its `reviewed-by:` line reads
  `gpt-6-astra`, or `codex, then claude`, or `codex2`
- **THEN** the script exits non-zero each time and says the line must name exactly one of `claude` and `codex`

#### Scenario: Both agents implemented

- **WHEN** a code pull request has the lines `implemented-by: codex (gpt-6-sol); claude (review fixes)` and
  `reviewed-by: claude`
- **THEN** the script exits zero

#### Scenario: Pull request by a bot

- **WHEN** a code pull request whose author GitHub reports as a bot has no `implemented-by:` line and has the line
  `reviewed-by: codex`
- **THEN** the script exits zero

#### Scenario: Line not written once at the start of a line

- **WHEN** the description of a code pull request has two lines that start `reviewed-by:`, or has
  `- reviewed-by: codex` as a list item and no line that starts with it, or has one `reviewed-by:` line and two
  lines that start `implemented-by:`
- **THEN** the script exits non-zero each time, and says there are two `reviewed-by:` lines in the first case, none
  in the second, and two `implemented-by:` lines in the third

#### Scenario: Carriage returns

- **WHEN** every line of the description ends in a carriage return and a line feed, and the two lines are otherwise
  as required
- **THEN** the script exits zero

#### Scenario: Draft

- **WHEN** a draft code pull request has no `reviewed-by:` line, no known flake is open and the up-to-date rule
  holds
- **THEN** the script exits zero, prints that a `reviewed-by:` line is missing, and says this does not fail a draft

#### Scenario: A read for the review check fails

- **WHEN** the read of the pull request or of its files fails, or returns something other than what was asked for,
  on a pull request that is a draft or is not
- **THEN** the script exits non-zero naming the read, and does not say documents only or ok

#### Scenario: Sorting names fails

- **WHEN** the step that sorts the changed files into code names fails
- **THEN** the script exits non-zero, and does not say documents only

#### Scenario: Push run and the review check

- **WHEN** `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` is `push`
- **THEN** the script says the review check does not apply and reads no pull request and no file list

### Requirement: A run when a pull request is marked ready

The `pull_request` trigger of the CI workflow SHALL list the activity types `opened`, `synchronize`, `reopened` and
`ready_for_review`, so that a run starts when a pull request is opened, pushed to or reopened, and when a draft is
marked ready for review. The review check does not fail a draft, so the run that marking it ready starts is the one
that applies the check to the head that can now merge. A contract test SHALL fail when the workflow has no
`pull_request` trigger, when the trigger lists no activity types, and when it lacks any one of the four.

#### Scenario: Ready type missing

- **WHEN** the `pull_request` trigger in `.github/workflows/ci.yml` lists `opened`, `synchronize` and `reopened`, or
  lists no activity types, or the workflow has no `pull_request` trigger
- **THEN** the contract test fails each time naming `ready_for_review`

#### Scenario: A default type missing

- **WHEN** the `pull_request` trigger lists `opened`, `reopened` and `ready_for_review`
- **THEN** the contract test fails naming `synchronize`
