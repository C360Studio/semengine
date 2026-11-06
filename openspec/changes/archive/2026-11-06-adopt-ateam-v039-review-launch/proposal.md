# Proposal: adopt-ateam-v039-review-launch

Track: quick (one capability, `harness-boundaries`; one requirement modified, the pin).

## Why

SemEngine pins ateam v0.3.7. v0.3.8 adds agent-started review: `review_launch` in `.agents/review-policy.json`
(`manual`, the default, or `agent`), `ateam review request <pr>` (prints the reviewer runs the merge gate accepts, the
prompt and the head SHA) and `ateam review notify` (tells the implementer the record landed, never its text), with host
manifests under `.agents/hosts/` that say how a host starts and messages an agent. ateam's Enjoy manifest was run live.
v0.3.9 is a documentation release. Today the protocol says an agent does not start another platform's agent, so the
owner starts every reviewer by hand.

Approved-by: Coby Leuschke, 2026-11-06, in the Enjoy conversation "ateam update and agent review" ("we need to get
latest and turn on agent review flow").

## What Changes

- The ateam pin moves from `v0.3.7` to `v0.3.9` in both workflows, the merge-gate tool tests and `.agents/README.md`.
- `.agents/skills/{handoff,review-pr}/SKILL.md` take v0.3.9's template text unedited (`TestAteamTemplatesUnchanged`).
- `.agents/review-policy.json` sets `"review_launch": "agent"`.
- `.agents/hosts/enjoy.toml` is ateam v0.3.9's file, unedited. No other host manifest: herdr's and T3 Code's were never
  run live, and Codex or Claude in a terminal needs none (the command prints the runs).
- `.agents/protocol.md` § Cross-agent review: after the `Review request` comment, the implementer runs
  `ateam review request <n>` and starts the reviewer through the host manifest; the reviewer ends with
  `ateam review notify`.

The merge gate does not change: it still requires an approving record by another agent, not weaker, at the head.

## Capabilities

- Modified `harness-boundaries` (the ateam doctor requirement: pin `v0.3.9`).

## Impact

- `.github/workflows/{ci,merge-check}.yml`, `internal/harness/toolcheck/mergegate_test.go`, `.agents/README.md`,
  `.agents/skills/{handoff,review-pr}/SKILL.md`, `.agents/review-policy.json`, `.agents/hosts/enjoy.toml`,
  `.agents/protocol.md`, the spec.
- Closes #537.
