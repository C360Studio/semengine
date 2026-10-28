# Proposal: adopt-ateam-v033-bootstrap

## Why

Issue #448, the first child of epic #446. SemEngine adopts [ateam](https://github.com/C360Studio/ateam) v0.3.3 in
full. SemTeams adopted it first by layering it on its existing process and then needed a second epic to remove the
duplicates (semteams #363, #375); this change goes to that end state directly for the bootstrap: the files
`ateam init` writes, `ateam doctor --check` as the one oracle for agent-setup parity, and the removal of the local
checks that duplicate it.

Measured on 2026-10-28 on a scratch copy of `main` at 12543dc, and re-done in review on the PR base 2d6640a with the
same result: `ateam init` at v0.3.3 wrote 28 files and collided with none; `ateam doctor --check` then reported two
violations, the protocol's missing `ateam state handoff` marker and the `preflight` alias target's missing `## 1.`
and `Record Evidence` markers. Everything else here is removal or a pointer.

Owner rulings on #447 (2026-10-28): `task mutate:check` keeps `internal/harness/mutcheck`; port designs keep their
depth and the 120-line `spec-check` warning is accepted. The pickup and handoff replacement (item 7) follows the
recommendation and is corrected if the ruling differs.

Approved-by: Coby Leuschke (2026-10-28), in session; recorded on #447.

## What Changes

- The `ateam init` output at v0.3.3 is committed unchanged: hooks and MCP registration for Claude, Codex, Gemini and
  Antigravity, the six canonical skills with their adapters, the base contracts `architect.md`, `developer.md` and
  `reviewer.md`, and `.ateam/` and `benchmarks/runs/` in `.gitignore`.
- `semengine-preflight` becomes the canonical `preflight` with its content kept and the two section markers added;
  `semengine-pickup` and `semengine-handoff` are replaced by ateam's `pickup` and `handoff`, and their
  SemEngine-specific sentences move into `.agents/protocol.md`, which also gains the `ateam state handoff` marker.
- `.agents/review-policy.json` declares the model tiers and effort bands (SemTeams' file) with `cross_provider: true`.
- The four `semengine-*` contracts open by pointing at ateam's base contract; `.claude/agents/*.md` gain `effort:`.
- `task ateam:doctor` joins `task verify`; the ateam pin is `v0.3.3` in `.github/workflows/ci.yml` and
  `scripts/doctor.sh`.
- `TestAgentSetupParity` keeps only what doctor does not check; the parity smoke in `.agents/README.md` becomes one
  line.

## Capabilities

### Modified Capabilities

- `harness-boundaries`: the "Agent setup" requirements are split between `ateam doctor --check` and the contract
  test; two requirements are added for the doctor gate and the template files.
