# Tasks: adopt-ateam-v033-bootstrap

## 1. Proposal

- [ ] 1.1 Proposal, tasks and spec delta committed; draft PR opened with `Closes #448` and `Addresses #446`. Gate:
  `task spec:check` and `task docs:check`.

## 2. Bootstrap files

- [ ] 2.1 `ateam init` (v0.3.3) output committed unchanged, including the `.gitignore` entries. Gate: `git diff` shows
  the 28 written files and nothing else changed by the command.
- [ ] 2.2 `preflight`: `.agents/skills/semengine-preflight` renamed to `.agents/skills/preflight` with its content
  kept and `## 1.` and `Record Evidence` section markers; the Claude adapter points at the new path; the Gemini
  command exists. Gate: `ateam doctor --check` lists no `preflight` violation and no alias.
- [ ] 2.3 `pickup` and `handoff`: ateam's text; `semengine-pickup` and `semengine-handoff` removed; their
  SemEngine-specific sentences (primary checkout is discovery-only, write ownership before editing, evidence must
  open on another machine, git-ignored paths are local only, the handoff threshold) are in `.agents/protocol.md`,
  which also names `ateam state handoff` as the session checkpoint home and in the Close step. Gate:
  `ateam doctor --check` lists no protocol violation.
- [ ] 2.4 `.agents/review-policy.json` with SemTeams' tiers (including the resolved Anthropic IDs and
  `claude:fable`), effort bands and `cross_provider: true`. Gate: `ateam doctor --check` passes the review setup.

## 3. Contracts, adapters, gates

- [ ] 3.1 Each `semengine-*` contract opens with the sentence pointing at ateam's base contract (`developer.md` for
  developer and technical writer, `reviewer.md` for reviewer, `architect.md` for architect) and keeps no copy of the
  record grammar; `.claude/agents/*.md` carry `effort:` (`medium` architect and developer, `high` reviewer, `medium`
  technical writer). Gate: `TestAgentSetupParity`.
- [ ] 3.2 `Taskfile.yml`: `ateam:doctor` task (session env vars unset) and `scripts/verify.sh` runs it first;
  `.github/workflows/ci.yml` installs `ateam@v0.3.3` in the verify job; `scripts/doctor.sh` checks the same pin.
  Gate: `task verify` locally, which now fails on any doctor violation.
- [ ] 3.3 `TestAgentSetupParity` and its sensitivity test keep the role-adapter checks (contract pointer, tools,
  sandbox, size, model and effort pins) and the adopter-skill adapter size and checkbox check; the `CLAUDE.md`,
  `AGENTS.md` size, rule-row count and skill-adapter existence checks are deleted with their sensitivity cases.
  `.agents/README.md`: the parity smoke is one line naming `task ateam:doctor` and the test. Gate: `task test:unit`.
- [ ] 3.4 `AGENTS.md`, `.agents/rules-index.md` (same row counts), `docs/testing.md` and the skills name the new
  files and `task ateam:doctor`; `AGENTS.md` stays under 24,000 bytes. Gate: `task verify` on a clean tree.

## 4. Review and archive

- [ ] 4.1 Cross-agent review record on the PR (`reviewed-by:` with `Reviewed at <sha>`); `implemented-by:` and
  `reviewed-by:` in the PR body.
- [ ] 4.2 Archive as the last content commit, syncing the `harness-boundaries` delta. Gate: `task spec:check` and
  `task docs:check`.
