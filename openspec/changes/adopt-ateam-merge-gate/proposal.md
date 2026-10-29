# Proposal: adopt-ateam-merge-gate

## Why

Issue #449, PR B of epic #446. The merge gate on `main` is still SemEngine's own `scripts/merge-check.sh` (270 lines)
with a 1,400-line fixture matrix in `internal/harness/toolcheck`. ateam's `review gate` was ported from that script
and adds what the script cannot do: a commit-bound review record (`Reviewed at <sha>`, `Verdict:`) and a strength
comparison of reviewer against implementer by model tier, then effort band, read from `.agents/review-policy.json`,
which PR A (#452) already committed. ateam v0.3.4 (tagged 2026-10-29) is the first release carrying the resolved
Claude model IDs in its tier ranking (ateam #209) and the 100 ms doctor ceiling (ateam #241); this change moves the
pin there and re-syncs the three template files v0.3.4 changed.

Owner ruling on this epic: match ateam unless there is a very good reason not to, and record the divergence.

Approved-by: Coby Leuschke (2026-10-29), in session, recorded on #449.

## What Changes

- `scripts/merge-check.sh` becomes `exec "${ATEAM:-ateam}" review gate "$@"`; `task merge:check -- <n>` is unchanged
  for callers.
- The merge check moves out of `ci.yml` into its own workflow `.github/workflows/merge-check.yml`, as ateam runs it:
  triggered on `opened`, `synchronize`, `reopened`, `ready_for_review` and `edited` (a reviewer's description edit
  reaches it without rerunning Verify), queued never cancelled, skipped on a draft, checking out the **base** commit
  and installing the pinned binary so a pull request cannot loosen the gate judging it, with
  `ATEAM_REQUIRED_CHECKS=Required,Merge check`. `Required` in `ci.yml` then needs `verify` alone.
- The `main` ruleset gains `Merge check` as a second required status check (owner action on GitHub; the gate fails
  closed until it is there).
- The ateam pin becomes `v0.3.4` in both workflows; the three template files v0.3.4 changed (`developer.md`,
  `reviewer.md`, `review-pr`) are re-synced byte for byte; the doctor ceiling sentence becomes 100 ms.
- `internal/harness/toolcheck/mergecheck_test.go` and the gate matrix go; one smoke test shows the wrapper execs
  `$ATEAM` and passes exit codes through; `TestCIWorkflowPinned` covers both workflows.
- Record grammar: the `semengine-reviewer` contract's verdict words point at ateam's `reviewer.md` § Record
  (`Verdict: APPROVE | REQUEST CHANGES`), and the protocol's "Cross-agent review" item says what the gate checks.

## Capabilities

### Modified Capabilities

- `merge-gate`: the script's requirements are replaced by the gate at its pin and the merge-check workflow.
- `harness-boundaries`: the doctor gate names the v0.3.4 pin and its 100 ms ceiling.
