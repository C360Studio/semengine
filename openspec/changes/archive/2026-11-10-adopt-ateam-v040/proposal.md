# Proposal: adopt-ateam-v040

Track: quick (two capabilities, `harness-boundaries` and `merge-gate`; one requirement modified in each).

## Why

SemEngine pins ateam v0.3.9; v0.4.0 was released on 2026-11-10 (issue #585 measured each release note against
`main`). Three of its changes reach this repository. The probe in `merge-check.yml` now asks whether a successful
`Merge check` already stands at the head (ateam #359), which reads check runs, a permission our probe job lacks; without
it the probe fails and the full check runs red on a pull request still awaiting review. `Approved-by:` now carries a
`sha256:` digest of `proposal.md` and `design.md`, and `task spec:check` fails when either changes after approval
(ateam #272). The base contracts and skills now cite ateam's numbered protocol sections (§ 1 to § 4, ateam #200),
which SemEngine's own protocol does not number.

The owner asked for it on 2026-11-10 in the Enjoy conversation "ateam v0.4.0 upgrade issue" ("Claim #585 and do the
ateam v0.4.0 upgrade"); the approval below, with its digest, is the owner's to fill.

Approved-by: Coby Leuschke (2026-11-10) sha256:0c6a799383b3

## What Changes

- The ateam pin moves from `v0.3.9` to `v0.4.0` in both workflows, the merge-gate tool tests, `Taskfile.yml`,
  `.agents/README.md` and the `harness-boundaries` spec.
- ateam's base contracts and its `design`, `handoff`, `pickup` and `review-pr` skills take v0.4.0's template text
  unedited, plus the new `.agents/skills/handoff/t3-card.html` (`TestAteamTemplatesUnchanged`).
- The `Merge check probe` job adds `checks: read`; the contract test holds the probe to four reads and the
  `merge-check` job to its three.
- `.agents/protocol.md`: the `Approved-by:` form with the digest, and one paragraph mapping ateam's section numbers to
  this file's sections.

Not taken: `.agents/ateam.json` with `ateam_version`. Doctor would then fail on any binary that is not exactly the
pin, but the pin would live in a second place beside the workflow install lines that `scripts/doctor.sh`,
`ateam:bin` and the tool tests read. Making it the one source is its own change. The Ponytail alias removal, the
`required_checks` rename and the gate-files trigger move touch nothing here.

## Capabilities

- Modified `harness-boundaries` (ateam doctor gates verify: pin `v0.4.0`).
- Modified `merge-gate` (the merge-check workflow: probe permissions and the standing-pass rule).

## Impact

- `.github/workflows/{ci,merge-check}.yml`, `internal/harness/toolcheck/mergegate_test.go`, `Taskfile.yml`,
  `.agents/README.md`, `.agents/contracts/{architect,developer,reviewer}.md`,
  `.agents/skills/{design,handoff,pickup,review-pr}/SKILL.md`, `.agents/skills/handoff/t3-card.html`,
  `.agents/protocol.md`, the two specs.
- Draft PR #582 shares no file. Its `Approved-by:` line has no digest, so `task spec:check` warns on it after this
  merges; re-approving with the digest is the owner's.
- Closes #585.
