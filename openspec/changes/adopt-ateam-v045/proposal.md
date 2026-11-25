# Proposal: adopt-ateam-v045

Track: quick (one capability, `harness-boundaries`; one requirement modified).

## Decisions you are making by approving

- Every future design-track proposal must list the owner's decisions, or the spec check fails it.
- Gemini reviewers move tiers as ateam's release notes say, which changes which Gemini runs may review Opus work.
- Codex Sol models stay in the top tier here, although ateam's own policy now ranks them one tier lower.
- No paths force the design track, and whole-repository review tags are not used yet.

## Why

SemEngine pins ateam v0.4.0; v0.4.5 was released on 2026-11-24 (issue #748 lists every change from v0.4.1 to v0.4.5
against `main`). The only breaking change is in v0.4.5: `ateam state spec-check` fails an open change that has a
`design.md` and a proposal without a bullet under `## Decisions you are making by approving`. `main` has no open change
today, so nothing turns red on merge. The owner asked for the upgrade on 2026-11-24 in Enjoy ("Start on #748").

Approved-by:

## What Changes

- The pin moves from `v0.4.0` to `v0.4.5` in both workflows, the merge-gate tool tests, `Taskfile.yml`,
  `.agents/ateam.json`, `.agents/README.md` and the `harness-boundaries` spec.
- ateam's base contracts and its `design`, `handoff`, `pickup`, `ponytail` and `review-pr` skills take v0.4.5's
  template text unedited (`TestAteamTemplatesUnchanged`). The base reviewer contract now carries ateam's default
  blocking bar and delta re-review; it lets a repository raise the bar, and `semengine-reviewer.md` keeps its own rules.
- SemEngine's own text: `.agents/protocol.md`, `AGENTS.md` and `.agents/rules-index.md` name the decisions section;
  the architect contract may write its bullets; `docs/testing.md` says a helper that mirrors the code under test is
  not an oracle.
- `.agents/review-policy.json`: `gemini-4-carbon` joins frontier, `gemini-4-argon` moves to workhorse beside the new
  `gemini-4-neon`, `gemini-4-hydrogen` joins light, `gemini-3.1-pro` is dropped, and Gemini's high band adds `xhigh`.

Not taken: ateam's own policy moving `gpt-6-sol` and `gpt-6.1-sol` to workhorse (not in any release note);
`design_paths` and the CI `spec-check --base` step it needs; the `review whole` tag; v0.4.5's `openspec/config.yaml`
rules, which repeat the protocol. The relative `scripts/ateam` hook paths stay: `doctor` warns, it does not fail.

## Capabilities

- Modified `harness-boundaries` (ateam doctor gates verify: pin `v0.4.5`).

## Impact

- `.github/workflows/{ci,merge-check}.yml`, `internal/harness/toolcheck/mergegate_test.go`, `Taskfile.yml`,
  `.agents/{ateam.json,README.md,review-policy.json,protocol.md,rules-index.md}`, `AGENTS.md`, `docs/testing.md`,
  `.agents/contracts/{architect,developer,reviewer,semengine-architect}.md`, five skills, the spec.
- No open PR. Closes #748.
