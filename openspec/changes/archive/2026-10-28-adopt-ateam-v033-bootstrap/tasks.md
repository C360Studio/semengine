# Tasks: adopt-ateam-v033-bootstrap

## 1. Proposal

- [x] 1.1 Proposal, tasks and spec delta committed; draft PR opened with `Closes #448` and `Addresses #446`. Gate:
  `task spec:check` and `task docs:check`.
  Done in 25281f2; draft PR #452 carries both lines; `task spec:check` and `task docs:check` pass on it.

## 2. Bootstrap files

- [x] 2.1 `ateam init` (v0.3.3) output committed unchanged, including the `.gitignore` entries. Gate: `git diff` shows
  the 28 written files and nothing else changed by the command.
  Done in f28724f: `ateam init` wrote 28 files and appended the two `.gitignore` lines; `git status` showed nothing else.
- [x] 2.2 `preflight`: `.agents/skills/semengine-preflight` renamed to `.agents/skills/preflight` with its content
  kept and `## 1.` and `Record Evidence` section markers; the Claude adapter points at the new path; the Gemini
  command exists. Gate: `ateam doctor --check` lists no `preflight` violation and no alias.
  Done in 85361eb (notes/2.2.md): `ateam doctor --check` at 51207ef reports `"violations": null` and
  `"skill_aliases": []`.
- [x] 2.3 `pickup` and `handoff`: ateam's text; `semengine-pickup` and `semengine-handoff` removed; their
  SemEngine-specific sentences (primary checkout is discovery-only, write ownership before editing, evidence must
  open on another machine, git-ignored paths are local only, the handoff threshold) are in `.agents/protocol.md`,
  which also names `ateam state handoff` as the session checkpoint home and in the Close step. Gate:
  `ateam doctor --check` lists no protocol violation.
  Done in 5b49229 (notes/2.3.md): `ateam doctor --check` at 51207ef reports no protocol violation.
- [x] 2.4 `.agents/review-policy.json` with SemTeams' tiers (including the resolved Anthropic IDs and
  `claude:fable`), effort bands and `cross_provider: true`. Gate: `ateam doctor --check` passes the review setup.
  Done in a5a7cea: `ateam doctor --check` at 51207ef passes (`"passed": true`).

## 3. Contracts, adapters, gates

- [x] 3.1 Each `semengine-*` contract opens with the sentence pointing at ateam's base contract (`developer.md` for
  developer and technical writer, `reviewer.md` for reviewer, `architect.md` for architect) and keeps no copy of the
  record grammar; `.claude/agents/*.md` carry `effort:` (`medium` architect and developer, `high` reviewer, `medium`
  technical writer). Gate: `TestAgentSetupParity`.
  Done in 3165744 (notes/3.1.md): `TestAgentSetupParity` passes in `task test:unit` at 51207ef.
- [x] 3.2 `Taskfile.yml`: `ateam:doctor` task (session env vars unset) and `scripts/verify.sh` runs it first;
  `.github/workflows/ci.yml` installs `ateam@v0.3.3` in the verify job; `scripts/doctor.sh` checks the same pin.
  Gate: `task verify` locally, which now fails on any doctor violation.
  Done in d741d59 (notes/3.2.md): `task verify` at 51207ef on a clean tree ran `ateam:doctor` first and
  exited 0.
- [x] 3.3 `TestAgentSetupParity` and its sensitivity test keep the role-adapter checks (contract pointer, base-contract
  pointer, tools, sandbox, size, model and effort pins) and, for every adopter skill (each skill under
  `.agents/skills/` that is not one of ateam's six), that its Claude adapter exists, links to it, says to read it
  fully, and stays within 25 lines with no checkbox; the `CLAUDE.md`, `AGENTS.md` size and rule-row count checks are
  deleted with their sensitivity cases, since `ateam doctor --check` owns them and ateam's six canonical skills.
  `.agents/README.md`: the parity smoke is one paragraph naming the three oracles. Gate: `task test:unit`.
  Done in 91b6822, amended in e362556 and 6e4720f after review round 1 (notes/3.3.md): `task test:unit` at d035dfd
  passes, including `TestAgentSetupParitySensitivity`. Codex review MEDIUM 2 (PR comment 6473373835) asked for this
  wording; the retained empty-skill check it also asked for is #460.
- [x] 3.4 `AGENTS.md`, `.agents/rules-index.md` (same row counts), `docs/testing.md` and the skills name the new
  files and `task ateam:doctor`; `AGENTS.md` stays under 24,000 bytes. Gate: `task verify` on a clean tree.
  Done in 51207ef (notes/3.4.md): `AGENTS.md` is 23,968 bytes; `task verify` at 51207ef on a clean tree exited 0.

## 4. Review and archive

- [x] 4.1 Cross-agent review record on the PR (`reviewed-by:` with `Reviewed at <sha>`); `implemented-by:` and
  `reviewed-by:` in the PR body.
  Done: Codex record on PR #452, comment 6473373835 (2026-10-28T15:36:57Z): `reviewed-by: codex (gpt-6-astra, high)`,
  `Reviewed at d035dfd694d55fbab418147fe292ad0e8c62f7e1`, `Verdict: APPROVE`, two nonblocking MEDIUMs (one applied
  in task 3.3's wording, one filed as #460). The PR body carries `implemented-by: claude (opus, medium)` and
  `reviewed-by: codex (gpt-6-astra, high)`.
- [x] 4.2 Archive as the last content commit, syncing the `harness-boundaries` and `merge-gate` deltas. Gate:
  `task spec:check` and `task docs:check`.
  Done in the archive commit: `openspec archive adopt-ateam-v033-bootstrap`; `task spec:check` and `task docs:check`
  pass on it.
