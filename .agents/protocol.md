# Shared work protocol (Claude and Codex)

Canonical. `CLAUDE.md` and `AGENTS.md` point to this file and carry the three gates (claim, merge, close) inline; edit
the protocol here only. Read it before taking, landing, or closing work; the pickup and handoff skills read it.

State that both agents must see lives in the repository's tools, never in a prose document or either agent's private
memory. Each question has one home, and each home is a `gh` or `task` query. There is no `/tickets` state.

- **What is wanted, what kind, is it decided:** a GitHub issue with labels `type:epic`, `status:needs-decision`,
  `status:blocked`. `status:needs-decision` is the owner's docket; a ruling is posted as an issue comment and the
  label removed. `status:blocked` names its blocker in a comment.
- **An epic:** a tracking issue labeled `type:epic` whose body carries a task list of `#n` children. GitHub renders
  the progress; there is no separate epic document.
- **What gates a release:** a GitHub milestone. Setup work (SETUP 01 to 03B) shares one milestone; each provisional
  tier release (SETUP 04A, 04B, 04C) has its own. An epic sits in exactly one milestone.
- **Who has claimed what:** a **draft PR** on an agent-prefixed branch (`claude/…`, `codex/…`) opened at the start
  of the work, with `Closes #n` for a leaf issue or `Addresses #n` for an epic. No draft PR, no claim. Design-phase
  work claims the same way; the OpenSpec change is its
  first content commit. A stop-point goes in the PR description.
- **Target state, task truth, holds:** the OpenSpec change inside that PR; `task spec:queue`, run in the claim's
  worktree, shows its holds. The archive (`openspec archive <id>` + spec sync) is the landing PR's last commit,
  reviewed with the code. Every task can be ticked in or before the archive commit. No task may assert a post-merge
  fact ("CI green", "merge-ready") or wait on a step that follows the archive (the check of the archive, undraft, the
  final CI run, the merge): such a task strands the change, and those steps are recorded on the PR. A hold is written
  on the unticked task it stops, as `Hold:` followed by what it waits for (an issue or PR number, or the owner's
  ruling). The queue reads unticked task lines only: a hold in a heading or a paragraph does not show.
- **Why:** an ADR, or the owner's ruling comment on the issue. Owner rulings of 2026-09-30 on the plan are recorded
  on PR #1.

## Work lifecycle

- **Start:** `gh issue list --state open` · `gh pr list` (drafts are claims; skip them) · `task spec:queue` ·
  `gh run list --branch main --limit 3` · `gh issue list --label status:needs-decision`.
- **Take work:** an unclaimed issue, then a dedicated worktree on an agent-prefixed branch (`claude/<topic>`,
  `codex/<topic>`; the prefix names who claimed it), then push, then a draft PR with `Closes #n` or `Addresses #n`,
  then work. One claimed PR owns one worktree. When multiple agents share a host, the primary checkout is
  discovery-only and no agent commits from it. Immediately before every commit and push, verify that the worktree's
  current branch is the draft PR head; a mismatch stops the operation. The prefix says who claimed the work, never
  whether that agent's worktree is idle.
- **Bootstrap exception:** while the repository has no base protocol, SETUP 01 (epic #5, claim PR #12, branch
  `setup-01-foundation`, unprefixed) is claimed under the plan's bounded exception. The exception ends when that PR
  merges; every later claim follows the rules above.
- **Worktree hygiene:** the claim's worktree lives at a durable sibling path
  (`git worktree add ../semengine-wt/<branch> -b <branch> origin/main`), never under `/private/tmp`, which a reboot
  purges; `git worktree remove` it when the PR merges. Heavy local gates run one agent at a time on a shared host:
  worktrees fix the git collision, not the CPU one. CI is the arbiter; a local red under contention is not a finding.
- **File:** before opening an issue, route the finding. A residual of a decision this change just made deliberately
  belongs in a doc comment at the line or in the change's `design.md`. A consequence an owner ruling already states
  needs nothing; the ruling is the record. An unmeasured cost becomes a *Declared cost* section in the design. Only an
  architectural finding (it crosses files, would need its evidence re-collected to re-derive, or changes what someone
  should not do next) becomes an issue. Ask the owner before filing when placement is a genuine scheduling call.
- **Land:** implementation review (the other agent's for a code pull request: "Cross-agent review" below), then,
  for a documents-only change, the owner-run cross-agent round where the owner asks for it, then fixes and
  re-review, then archive as the final content commit, then a narrow reviewer check of the archive/spec sync (either
  agent's reviewer), then undraft, then CI green on a head that is up to date with `main` and with **no known flake
  open** ("Known flakes"
  below), then `task merge:check -- <n>` immediately before merging, in a shell where `GITHUB_ACTIONS` is not set,
  then squash merge. A green run while a known flake is open is not a fix: a re-run or a new push only rolls the dice
  again. Fix the flake; there is no other way past it. A correction after archive re-enters reconciliation and final
  review; no later content commit bypasses the archive/spec-sync check. State `implemented-by:` in the PR body, naming
  the implementing agent with the word `claude` or `codex` beside the model or persona
  (`implemented-by: claude (opus semengine-developer)`). Bring a pushed branch up to date by merging `origin/main`
  into it; do not rebase or force-push it. A review record names the commit it read, and a rebase leaves that record
  pointing at a commit the branch no longer has. The squash merge keeps `main` linear either way.
- **Cross-agent review:** on a code pull request the reviews in "Land" (the implementation review, the re-review of
  fixes, the check of the archive/spec sync) are done by the agent that wrote none of the commits under review, as
  the pull request's `implemented-by:` line records. The owner ruled on 2026-10-02 (issue #64) that Codex's
  `semengine-reviewer` reviews what Claude sessions implement, that it works both ways (Claude's reviewer for what
  Codex implements), and that the rule is enforced for code and waived for a documents-only pull request. What
  "documents only" means is this repository's reading of that ruling, not the owner's words: every file the pull
  request changes is a Markdown file or is under `openspec/`, and none is a role adapter under `.claude/agents/`
  (Markdown that sets a role's model and tools, as the `.toml` files under `.codex/agents/` do). Any other changed
  file makes it a code pull request: a `.go` file, the harness included; `go.mod`; a script; `Taskfile.yml`; the CI
  workflow; `docs/admission-ledger.yaml`. If both agents wrote commits, the owner names the reviewer on the issue. A
  pull request neither agent wrote (a dependency bot's) is reviewed by either agent's reviewer, and the record names
  which. The check below provides for this only when GitHub reports the author as a bot. A code change the owner
  would write by hand is given to an agent session to make: the commits are then that agent's, `implemented-by:`
  names it truthfully, and the other agent reviews. A code pull request by a user with no `implemented-by:` line
  naming an agent fails the check once it is not a draft; the ruleset on `main` lets nobody merge past the failed
  check, and editing the ruleset is a waiver this rule does not give. For a code pull request this replaces the
  owner-run round; the owner need not ask. A documents-only pull request keeps SemEngine's own reviewer. The rule
  applies to every pull request not yet merged, one already open included. The implementing session may run its own
  reviewer as it works; those reviews, past or future, find defects early and are not the gate. Neither agent can
  start the other, so the pull request carries both halves:
  - The implementer asks with a PR comment headed `Review request`. It names the kind of review, the commit to read,
    the diff range, that commit's CI run, and the issue or ruling the change answers. It does not say what the
    reviewer should conclude.
  - The reviewing session answers with a PR comment headed `Review record`: its reviewer's report as written, in the
    reviewer contract's format, not a summary. It names the commit read, what was run, and what could not be run.
  - Evidence comes from the tree and from CI, not from the implementer's account. For anything `task verify` runs,
    the CI run of the named commit is the evidence. A check the reviewer could not run and CI does not run is
    recorded as not run; output the implementer supplies for it is recorded as reported by the implementer, never as
    passed.
  - A record covers the commit it names. A later content commit, one that changes the pull request's own diff against
    `main`, needs a re-review, and the archive commit needs the archive check. The other agent's review covers code:
    a later commit that changes code (any file that is not Markdown or under `openspec/`, or a role adapter under
    `.claude/agents/`) goes back to the other agent for re-review. A later commit that changes documents only, and
    the check of the archive/spec sync, are reviewed by either agent's reviewer; the record names which (owner
    ruling of 2026-10-03, #66 comment 5968737070: cross-agent review is for code, not for process). A merge of
    `origin/main` is not a content commit when `main` changed no file the pull request changes: the record carries
    over, and CI on the merged head is the check. When `main` changed a file the pull request also changes, that
    file needs a re-review.
  - The reviewer does not write on the branch. Findings go back to the implementer, who keeps write ownership. If
    the two agents disagree on a finding, it goes to the owner on the issue, labelled `status:needs-decision`.
  - The implementer writes one `reviewed-by:` line in the PR body, beside `implemented-by:`, once the reviewing
    agent's newest review record approves; it names that agent with the word `claude` or `codex`, beside the model or
    persona taken from the record.
  - The check: `scripts/merge-check.sh`, run by CI's `merge-check` job and by `task merge:check -- <n>`, reads the
    pull request's description and its list of changed files. On a code pull request the description must have
    exactly one line starting `implemented-by:` that names at least one agent (not read when GitHub reports the
    author as a bot) and exactly one line starting `reviewed-by:` that names exactly one agent: the one
    `implemented-by:` does not name, or either when it names both or the author is a bot. A line starts in the first
    column; an agent name is `claude` or `codex` in any letter case, not part of a longer word such as `codex2`;
    nothing else in either line is read. On a draft the findings are printed and fail nothing; on a pull request that
    is not a draft they fail `merge-check`, and `Required` with it. Marking a draft ready starts the CI run that
    applies the check; editing the description or adding a comment starts no run, so after the edit run the failed
    `merge-check` job again.
  - What stays review only: whether the lines are true (the owner and both agents use one GitHub login, so the script
    cannot tell who wrote a commit, a description or a comment); that a review record exists, the commit it read,
    and the re-review after a later content commit (once written, the line satisfies the check through every later
    push); the record's verdict, scope and content; that the owner named the reviewer when both agents wrote
    commits; a pull request that changes the script, the workflow or their tests, which runs its own copies; and the
    moment after marking ready before the new run registers, when a draft's green `Required` may still be the newest
    result, which the Land steps of waiting for CI and `task merge:check -- <n>` cover.
- **Known flakes:** a known flake is an open issue labelled `class:flake`. The label is for a failure that a pull
  request in this repository can end: a test or check that passes and fails on the same tree. A network fetch that
  did not answer is not one. Record it as a comment on the pull request it hit, with the run's link, and re-run when
  the remote answers; the second failure of the same fetch gets an ordinary issue, without the label. A red you
  cannot explain by your diff is filed before the next push. While a known flake is open, CI's `merge-check` job,
  and `Required` with it, fails on every pull request except one whose description closes every open flake
  (`Closes #n` for each). No comment or label on a pull request exempts it. A pull request that closes a flake shows
  the reproduction: the command, and how often it failed before the fix and after. A flake that comes back after its
  fix reopens its issue. A property-based test that fails on one run and passes on the next is a known flake too: the
  issue records the seed, and the fix keeps the failing input as a named test (`docs/testing.md`, "Running and
  replaying a Rapid test").
- **Close:** the squash merge of a PR that declared `Closes #n` at review time closes that issue; the merge is the
  authorization. The declaration must predate the review rounds that cover it. A PR that only `Addresses` an epic
  closes nothing. A close with no merged PR behind it (duplicate, stale, fixed elsewhere) takes the owner's word on
  the issue itself; an approval of adjacent work (a PR, a review round, a design, a waiver) never widens into a
  close, and a bare "approved" closes nothing. A `class:flake` issue is closed, or its label removed or renamed, only
  by a merged fix or on the owner's word: any of the three lifts the `merge-check` stop for every pull request.

## Verification

CI has three jobs. `verify` runs `task verify`, the same commands as a local run. `merge-check` runs
`scripts/merge-check.sh`: it fails while a known flake is open that the pull request does not close, fails unless
the rules on `main` require an up-to-date head, and fails a code pull request that is not a draft unless its
`implemented-by:` and `reviewed-by:` lines name the agents as "Cross-agent review" says. It reads GitHub, so it is
not part of `task verify`. `required` fails if `verify` or `merge-check` failed, is missing, or was skipped or
cancelled; it is the check the ruleset on `main` requires. Gate selection and evidence recording are in
[semengine-preflight](skills/semengine-preflight/SKILL.md).

There is no program baton document. Agents mutate only this repository: SemStreams and other sister repositories are
read-only inventory.
