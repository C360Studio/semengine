# Shared work protocol (Claude, Codex, and Gemini)

Canonical. `CLAUDE.md` and `AGENTS.md` point to this file and carry the four gates (clarify, claim, merge, close)
inline; edit the protocol here only. Read it before taking, landing, or closing work; the pickup and handoff skills
read it.

State that all agents must see lives in the repository's tools, never in a prose document or an agent's private
memory. Each question has one home, and each home is a `gh` or `task` query. There is no `/tickets` state.

- **What is wanted, what kind, is it decided:** a GitHub issue with labels `type:epic`, `status:needs-decision`,
  `status:blocked`. `status:needs-decision` is the owner's docket; a ruling is posted as an issue comment and the
  label removed. `status:blocked` names its blocker in a comment.
- **An epic:** a tracking issue labeled `type:epic` whose body carries a task list of `#n` children. GitHub renders
  the progress; there is no separate epic document.
- **What gates a release:** a GitHub milestone. Setup work (SETUP 01 to 03B) shares one milestone; each provisional
  tier release (SETUP 04A, 04B, 04C) has its own. An epic sits in exactly one milestone.
- **Who has claimed what:** a **draft PR** on an agent-prefixed branch (`claude/…`, `codex/…`, `gemini/…`) opened at the
  start of the work, with `Closes #n` for a leaf issue or `Addresses #n` for an epic. No draft PR, no claim.
  Design-phase work claims the same way; the OpenSpec change is its
  first content commit. A stop-point goes in the PR description.
- **Target state, task truth, holds:** the OpenSpec change inside that PR; `task spec:queue`, run in the claim's
  worktree, shows its holds. The archive (`openspec archive <id>` + spec sync) is the landing PR's last commit,
  reviewed with the code. Every task can be ticked in or before the archive commit. No task may assert a post-merge
  fact ("CI green", "merge-ready") or wait on a step that follows the archive (the check of the archive, undraft, the
  final CI run, the merge): such a task strands the change, and those steps are recorded on the PR. A hold is written
  in the unticked task it stops, on any of its lines, as `Hold:` followed by what it waits for (an issue or PR number,
  or the owner's ruling). The queue reads every line of an unticked task and prints each hold from `Hold:` on. A hold
  in a heading or a paragraph outside every task does not show, and `task spec:check` fails on it. Proposal approval
  (owner ruling of 2026-10-28 on #447, items 1 to 3): `proposal.md` stays on one page (60 lines or fewer;
  `task spec:check` warns past it) and carries `Approved-by:`, empty until the owner fills
  `<name> (<YYYY-MM-DD>)`; the line may link the issue comment where the owner ruled. The first task holds
  `Hold: OPEN-Q proposal awaits owner approval` until the owner approves, and the hold comes off in the same edit that
  fills `Approved-by:`. The one hold covers proposal and design. A `design.md` past 120 lines draws a warning, which is
  accepted for port designs: they keep the shape sweep, pin probes and ADR listing their rules require.
- **Session checkpoint:** `ateam state handoff` (the [handoff](skills/handoff/SKILL.md) skill) writes
  `.ateam/state.json`, which git ignores, and posts the five-field checkpoint (`Goal`, `Addresses`, `Constraints`,
  `Done when`, `Checks to run`) to the open PR; [pickup](skills/pickup/SKILL.md) reconciles it against live state.
  Hand off at 450K tokens of context: `.claude/settings.json` sets auto-compaction at 500K, which a session never
  reaches. Ride past 450K only when staying on the same plan in the same context is worth more than a clean pickup,
  and say so in the checkpoint. The checkpoint records every pending paid run, background job or automation with its
  identifier and owner, and whether it keeps running, so the next session neither duplicates nor silently cancels one
  in flight. An unpushed commit makes a checkpoint local; it does not erase a draft-PR claim.
- **Why:** an ADR, or the owner's ruling comment on the issue. Owner rulings of 2026-09-30 on the plan are recorded
  on PR #1.

## Bounded port scope

A port implements the admitted capability and its accepted contract. Its reviewed design establishes the work to
deliver. A finding adds required work only when the correction is necessary to satisfy a binding requirement, an
admission or verification gate, an explicit owner ruling, an accepted design commitment, or the dependency and
packaging changes needed to build the admitted package. Name that obligation and the concrete failure, conflicting
implementation, or missing required evidence. A maintenance preference alone does not establish required work.

Correct broken integrity, silent-loss prevention, context ownership, completed joins, authority and readiness,
acknowledged durability, and metadata or content preservation. All other explicit correctness, language, surface,
testing, and repository requirements remain binding, except the generic inherited-cleanup mandates this section
expressly supersedes. Missing proof of a required guarantee calls for targeted qualification; it is neither a
passing gate nor automatic evidence that a rewrite is necessary. A scope freeze cannot excuse a broken contract, a
newly introduced defect, or an unproved admission gate. A failure that needs a condition the code does not choose and
names no real-world trigger is not required work under this list; the reviewer contract § Finding and verdict format
(#185) sets its severity and its fix.

Use the retained contract, code path, existing reproduction, failing regression, or required qualification result.
State what it proves and what remains unknown. Whether a defect began at the source pin or during the port may remain
unknown; attribution is not a prerequisite to correcting a demonstrated defect. This rule adds no pin experiment per
finding. Existing pin-probe obligations and evidence requirements still apply to claims about pin behavior.

Inherited duplication, copied scaffolding, unnecessary indirection, naming, and similar maintenance findings are
optional unless they meet the required-work rule above. "Inherited" means structure retained from the source package,
not every addition first made in this repository. This exception does not permit newly introduced duplicate owners,
new exports with no reader and no contract line, or parallel spellings. The surface audit remains required: truly dead
surface is not protected by this exception, and admitted packages, consumers, and the pin's consumer adapters (pin
packages outside both halves, `AGENTS.md` § What this is for, or named consumer-owned by a ruling, as SETUP 03B design
D4 excludes `gateway` and `gateway/graph-gateway` under D5's layer 4:
`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:223`) still count when identifying readers.
A symbol with a production caller at the pin in a consumer adapter is framework API, offered to products built on
SemEngine: it is kept with its contract line, and a defect in it is fixed or tracked, not removed with it (owner ruling
of 2026-10-12, #190).

The inventory records every shape-sweep finding. For each, the design records either the required correction and its
binding basis, or the retained structure and why changing it is optional. Required corrections receive the
appropriate ledger disposition; an optional finding does not force a refactor or an `adapt` item. Record retained
costs through existing design and ledger fields where applicable. Route findings under Work lifecycle, "File"; do
not create a separate cleanup ledger.

An optional finding cannot veto design or implementation approval, become a blocking or high finding merely by
invoking a generic consolidation rule, or add a prerequisite to the MVP path. Present optional work as optional
before design acceptance. Scheduling it as additional work requires an explicit owner decision identifying that
work; a general instruction to keep code simple is insufficient. Required corrections use the smallest complete
change that meets the obligation; unrelated cleanup does not join that correction automatically.

This rule supersedes generic wording that makes every inherited shape finding a mandatory consolidation, port
refactor, or `adapt` item, including that part of issue #159. The sweep, package-size rules, and specifically directed
adoption work from #159 remain binding. Stronger explicit requirements, package-specific ADR decisions, other owner
rulings, and accepted tasks remain binding; only the owner can change those commitments. Apply this rule to future
port designs and newly raised findings on open ports. Existing approved tasks, including PR #93's tasks, the #170
prerequisite, and package-specific ADR adoption, remain commitments; changing them requires a separate explicit owner
ruling. This rule adds no prerequisite to PR #93. Enforcement is review only: the independent reviewer checks the
obligation and evidence supporting required work within the existing finding and verdict format and does not block
on optional maintenance.

## Work lifecycle

- **Start:** `gh issue list --state open` · `gh pr list` (drafts are claims; skip them) · `task spec:queue` ·
  `gh run list --branch main --limit 3` · `gh issue list --label status:needs-decision`.
- **Clarify:** before claiming work, mutating state, or spawning role subagents, read any named issue, PR, or file so
  you never ask a question the repository already answers, then verify four boundaries are clear: (1) **Deliverable**
  (answer/audit, issue, OpenSpec design, docs-only PR, or code PR), (2) **Scope and non-goals** (in-bounds
  packages/files and what must not change), (3) **Definition of Done and verification** (observable completion criteria
  and `task` gates), and (4) **Stop boundary** (pause after inventory/design for an owner decision vs run through to a
  ready PR). When a prompt leaves any of the four ambiguous, stop first, propose a concise 3–4 bullet recommended
  Definition of Done the owner can approve or adjust in one reply, and ask at most 1–3 targeted questions; once set,
  run autonomously to that stop boundary without pausing on routine steps. Then declare the change's track: `quick`
  (proposal and tasks) or `design` (plus `design.md`, drafted through the [design](skills/design/SKILL.md) skill).
  `design` is required when the change adds a spec Requirement or touches more than one capability under
  `openspec/specs/`.
- **Take work:** an unclaimed issue, then a dedicated worktree on an agent-prefixed branch (`claude/<topic>`,
  `codex/<topic>`, `gemini/<topic>`; the prefix names who claimed it), then push, then a draft PR with `Closes #n` or
  `Addresses #n`, then work. One claimed PR owns one worktree. When multiple agents share a host, the primary checkout
  is discovery-only and no agent commits from it; its `task spec:queue` does not list unmerged changes elsewhere. The
  previous writer stops or releases ownership before a pickup edits; never seize an active worktree. Immediately
  before every commit and push, verify that the worktree's current branch is the draft PR head; a mismatch stops the
  operation. The prefix says who claimed the work, never
  whether that agent's worktree is idle.
- **Bootstrap exception:** while the repository has no base protocol, SETUP 01 (epic #5, claim PR #12, branch
  `setup-01-foundation`, unprefixed) is claimed under the plan's bounded exception. The exception ends when that PR
  merges; every later claim follows the rules above.
- **Worktree hygiene:** for standalone CLI sessions, the claim's worktree lives at a durable sibling path
  (`git worktree add ../semengine-wt/<branch> -b <branch> origin/main`), never under `/private/tmp`, which a reboot
  purges; Enjoy desktop sessions use `move_to_worktree`, which creates `.worktrees/<branch>` (ignored in `.gitignore`
  alongside `.enjoy-logs/`). Close hygiene (owner ruling of 2026-10-28 on #447, item 6): after the squash merge the
  claiming agent removes its worktree and deletes the remote branch. `ateam state prune` removes every clean, merged
  worktree of its own and their local branches; GitHub deletes the remote branch on merge. Never remove another
  agent's worktree, or one that is dirty or unpushed. Heavy local gates run one agent at a time on a shared host:
  worktrees fix the git collision, not the CPU one. CI is the arbiter; a local red under contention is not a finding.
- **File:** a defect in code in this repository that the change does not fix is always filed as an issue, whether
  the code was carried or ported (brought over from the SemStreams pin, unchanged or adapted) or written here, such as
  the test harness or a script (two owner rulings of 2026-10-13 on #212, comments 6156218594 and 6156601031: the
  first writes down the owner's word of 2026-10-12 on carried code, quoted on #193 and #212; the second extends it to
  code written here). A defect is a behaviour that breaks what the code promises: a contract line (a spec
  requirement, a package `README.md` or its admission-ledger `contract` field), a doc comment, or a reasonable
  reading of the code. It is filed whatever found it: a test, a review, a port or a coverage pass. In carried or
  ported code, whether the defect began at the pin or during the port may stay unknown; attribution is not a
  prerequisite ("Bounded port scope" above). Check for an existing issue first (`gh issue list --search`). #189 and
  #193 bundle one package's defects as numbered items in one issue; a defect that a pull request can retire on its
  own is better filed alone, so `Closes #n` can close it. When the defect's code exists only on a pull request's
  branch, the issue is labelled `status:blocked` with that pull request named as its blocker, and the pull request
  body links it as a follow-up with no closing keyword (`Follow-up after this pull request merges: #n`): a `Closes`,
  `Fixes` or `Resolves` there would close the issue when the pull request merges. The label stays until that pull
  request merges and then comes off, removed by whoever merges it or by the next session that reads the issue; after
  PR #93 merged, #189 kept the label until the owner's word removed it (#189 comment 6152552748). Filing tracks a
  defect; it does not satisfy a guarantee the pull request claims, and a finding about the defect keeps the severity
  the reviewer contract gives it (#185).

  Route every other finding before opening an issue. A defect is not a residual: a residual is a cost or loose end of
  a decision this change just made deliberately, not code that behaves wrongly, and it belongs in a doc comment at
  the line or in the change's `design.md`. A consequence an owner ruling already states needs nothing; the ruling is
  the record. An unmeasured cost becomes a *Declared cost* section in the design. Of the rest, only an architectural
  finding (it crosses files, would need its evidence re-collected to re-derive, or changes what someone should not do
  next) becomes an issue. Ask the owner before filing when placement is a genuine scheduling call.
- **Land:** implementation review (another agent's for a code pull request: "Cross-agent review" below), then, for a
  documents-only change, the owner-run cross-agent round where the owner asks for it, then fixes and re-review, then
  archive as the final content commit, then a narrow reviewer check of the archive/spec sync (any agent's reviewer),
  then undraft, then `Required` and `Merge check` green on a head that is up to date with `main` and with **no known
  flake open** ("Known flakes" below), then `task merge:check -- <n>` immediately before merging, in a shell where
  `GITHUB_ACTIONS` is not set, then squash merge. A green run while a known flake is open is not a fix: a re-run or a
  new push only rolls the dice again. Fix the flake; there is no other way past it. A correction after archive re-enters
  reconciliation and final review; no later content commit bypasses the archive/spec-sync check. State `implemented-by:`
  in the PR body, naming the implementing agent with the word `claude`, `codex`, or `gemini` and its model and effort as
  `.agents/review-policy.json` lists them (`implemented-by: claude (opus, medium)`). Bring a pushed branch up to date by
  merging `origin/main` into it; do not rebase or force-push it. A review record names the commit it read, and a rebase
  leaves that record pointing at a commit the branch no longer has. The squash merge keeps `main` linear either way.
  The developer's notes for an implementation review, the choices a task made that its design does not spell out, are
  a file per task, `openspec/changes/<id>/notes/<task>.md`, with a one-line pull request comment that links it
  (developer contract § Handoff); they archive with the change. Evidence cited in a PR or task opens on another
  machine: output pasted into the PR, or a CI run. A path git ignores (such as `coverage/` or `.ateam/`) exists only
  on the host that wrote it: name it as local only, and never make it a task's only proof.
- **Report:** a conclusion or a handoff ends with one or two lines on what was skipped or not checked and what risk
  remains (the Ponytail footer; the session-start hook injects the ladder, [ponytail](skills/ponytail/SKILL.md)). A
  deliberate limit in code is marked `// shortcut: <the limit>, <when to upgrade>`, which `ateam state debt` lists.
  Both apply the rule that every skip, drop or degrade is declared (`AGENTS.md` § 1, last row; owner ruling of
  2026-10-28 on #447, item 8).
- **Cross-agent review:** on a code pull request the implementation review, and the re-review of a fix that changes
  code, are done by an agent that wrote none of the commits under review, as the pull request's `implemented-by:` line
  records; the re-review of a fix that changes documents only, and the check of the archive/spec sync, are any agent's
  reviewer's (the bullet "A record covers the commit it names" below). The owner ruled on 2026-10-02 (issue #64) that
  Codex's `semengine-reviewer` reviews what Claude sessions implement, that it works both ways (Claude's reviewer for
  what Codex implements), and that the rule is enforced for code and waived for a documents-only pull request. A pull
  request is documents only when every file it changes ends in `.md`, or is a `.yml`, `.yaml` or `.json` file under
  `openspec/`, and none is a role adapter under `.claude/agents/` or a canonical role contract under
  `.agents/contracts/`: the classifier of `ateam review gate`, which the merge check runs. Any other changed file makes
  it a code pull request: a `.go` file, the harness included; `go.mod`; a script; `Taskfile.yml`; a CI workflow;
  `docs/admission-ledger.yaml`; `.agents/review-policy.json`; `.claude/agents/*`; `.codex/agents/*`;
  `.agents/contracts/*`. If multiple agents wrote commits, the owner names the reviewer on the issue. A pull request no
  agent wrote (a dependency bot's) is reviewed by any agent's reviewer, and the record names which; the gate provides
  for this only when GitHub reports the author as a bot. A code change the owner would write by hand is given to an
  agent session to make: the commits are then that agent's, `implemented-by:` names it truthfully, and another agent
  reviews. The ruleset on `main` lets nobody merge past a failed `Merge check`, and editing the ruleset is a waiver this
  rule does not give. For a code pull request this replaces the owner-run round; the owner need not ask. A
  documents-only pull request keeps SemEngine's own reviewer. The rule applies to every pull request not yet merged, one
  already open included. The implementing session may run its own reviewer as it works; those reviews find defects early
  and are not the gate. An agent does not start another platform's agent, so the pull request carries both halves:
  - The implementer asks with a PR comment headed `Review request`. It names the kind of review (the lane), the commit
    SHA to read, the diff range, the implementer's `task verify` result on that commit (run on a clean tree), and the
    issue or ruling the change answers. It does not say what the reviewer should conclude.
  - The reviewing session posts one PR comment, the review record, in ateam's grammar
    ([`contracts/reviewer.md`](contracts/reviewer.md) § Record; [review-pr](skills/review-pr/SKILL.md)):

    ```text
    reviewed-by: <agent> (<model>, <effort>)
    Reviewed at <commit-sha>
    Verdict: APPROVE | REQUEST CHANGES
    <findings in the semengine-reviewer format, checks run, what could not be run>
    ```

    `Reviewed at` names one commit of the pull request; the model is named as `.agents/review-policy.json` lists it.
  - Evidence comes from the tree and the reviewer's own runs, not from the implementer's account. For anything
    `task verify` runs, the evidence is the reviewer's run on the named commit; without one, the implementer's result
    is recorded as reported by the implementer, never as passed. The implementer does not wait on or watch a CI run
    before asking, and the record does not rest on one: the CI run before merge is the check (owner ruling of
    2026-10-10, issue #171). A check the reviewer could not run and CI does not run is recorded as not run.
  - A record covers the commit it names. A later commit that changes code (any file the classifier above does not
    call a document) needs a new record from the reviewing agent. A later commit that changes documents only, and a
    merge of `origin/main` that touches no file the pull request changes, carry the record over; the documents-only
    re-review and the check of the archive/spec sync are any agent's reviewer's, and the record names which (owner
    ruling of 2026-10-03, #66 comment 5968737070: cross-agent review is for code, not for process). When `main`
    changed a file the pull request also changes, the merge needs a new record.
  - The reviewer does not write on the branch. Findings go back to the implementer, who keeps write ownership. If
    the agents disagree on a finding, it goes to the owner on the issue, labelled `status:needs-decision`.
  - The PR body carries `implemented-by: <agent> (<model>, <effort>)`, written by the implementer, and, after an
    approving record, `reviewed-by: <agent> (<model>, <effort>)`, added by the reviewer (review-pr § 5). That
    description edit reruns `Merge check` on the `edited` event; no manual rerun is needed.
  - The check: `scripts/merge-check.sh` runs `ateam review gate` at the release pinned in the workflows, from
    `.github/workflows/merge-check.yml` (on the base commit, so a pull request cannot loosen the gate judging it) and
    from `task merge:check -- <n>`. On a code pull request that is not a draft it fails unless the description has
    both lines, the reviewer is another agent and not weaker than every implementer (model tier, then effort band, in
    `.agents/review-policy.json`), and the latest record approves a commit no later code commit follows. A draft
    carries no `Merge check` run.
  - What stays review only: whether the lines and the record are true (the owner and the agents use one GitHub login,
    so the gate cannot tell who wrote a commit, a description or a comment); the record's scope and content; that the
    owner named the reviewer when multiple agents wrote commits; and a pull request that changes `ci.yml` or
    `merge-check.yml`, which runs its own copy.
- **Known flakes:** a known flake is an open issue labelled `class:flake`. The label is for a failure that a pull
  request in this repository can end: a test or check that passes and fails on the same tree. A network fetch that did
  not answer is not one. Record it as a comment on the pull request it hit, with the run's link, and re-run when the
  remote answers; the second failure of the same fetch gets an ordinary issue, without the label. A red you cannot
  explain by your diff is filed before the next push. While a known flake is open, `Merge check` fails on every pull
  request except one whose description closes every open flake (`Closes #n` for each). No comment or label on a pull
  request exempts it. A pull request that closes a flake shows the reproduction: the command, and how often it failed
  before the fix and after. A flake that comes back after its fix reopens its issue. A property-based test that fails on
  one run and passes on the next is a known flake too: the issue records the seed, and the fix keeps the failing input
  as a named test (`docs/testing.md`, "Running and replaying a Rapid test").
- **Close:** at a session boundary, before context compaction, run `ateam state handoff` ("Session checkpoint" above).
  After the merge, remove the claim's worktree ("Worktree hygiene" above). The squash merge of a PR that declared
  `Closes #n` at review time closes that issue; the merge is the authorization. The declaration must predate the review
  rounds that cover it. A PR that only `Addresses` an epic closes nothing. A close with no merged PR behind it
  (duplicate, stale, fixed elsewhere) takes the owner's word on the issue itself; an approval of adjacent work (a PR, a
  review round, a design, a waiver) never widens into a close, and a bare "approved" closes nothing. A `class:flake`
  issue is closed, or its label removed or renamed, only by a merged fix or on the owner's word: any of the three lifts
  the `Merge check` stop for every pull request.

## Verification

Two workflows report the checks the ruleset on `main` requires. In `ci.yml`, `verify` runs `task verify`, the same
commands as a local run, and `required` (`Required`) fails if `verify` failed, is missing, or was skipped or cancelled.
`merge-check.yml` (`Merge check`) runs `scripts/merge-check.sh`, which is `ateam review gate`: it fails unless the rules
on `main` require `Required` and `Merge check` with an up-to-date head, fails a code pull request that is not a draft
unless its attestation lines and review record satisfy "Cross-agent review", and fails while a known flake is open that
the pull request does not close. It reads GitHub, so it is not part of `task verify`. Run locally,
`task merge:check -- <n>` checks that the rules on `main` require `Required` alone unless `ATEAM_REQUIRED_CHECKS` is set
(`ATEAM_REQUIRED_CHECKS='Required,Merge check'` checks both, as the workflow does). Gate selection and evidence
recording are in [preflight](skills/preflight/SKILL.md).

ateam's default test-to-spec attribution, a `// spec: openspec/specs/<cap>/spec.md#Requirement: ...` line on every Go
`Test`, `Fuzz` and `Example` checked by `ateam test-spec`, is deferred until
[C360Studio/ateam#237](https://github.com/C360Studio/ateam/issues/237) ships a brownfield mode (owner ruling of
2026-10-28 on #447, item 5); no test here carries the marker yet.

There is no program baton document. Agents mutate only this repository: SemStreams and other sister repositories are
read-only inventory.
