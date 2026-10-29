# Tasks: adopt-ateam-merge-gate

Track: `design` (adds spec requirements; touches `merge-gate` and `harness-boundaries`). Owner ruling on #447
(2026-10-28): proposal approval and the design track are adopted.

## 1. Proposal and design

- [ ] 1.1 Proposal, design, tasks and spec deltas committed; draft PR opened with `Closes #449` and `Addresses #446`.
  Hold: OPEN-Q proposal awaits owner approval (D1 and the ruleset edit it needs). Gate: `task spec:check` and
  `task docs:check`.

## 2. Pin and templates

- [ ] 2.1 The ateam pin is `v0.3.4` in `.github/workflows/ci.yml` (verify job); `scripts/doctor.sh` and
  `TestAteamTemplatesUnchanged` read every install line across `ci.yml` and `merge-check.yml` and require one version.
  Gate: `scripts/doctor.sh` with the v0.3.4 binary on PATH; the test.
- [ ] 2.2 The three template files v0.3.4 changed (`.agents/contracts/developer.md`, `.agents/contracts/reviewer.md`,
  `.agents/skills/review-pr/SKILL.md`) are byte-identical to `ateam init` at v0.3.4; the harness-boundaries doctor
  requirement names the 100 ms ceiling. Gate: `go test ./internal/harness/toolcheck -run TestAteamTemplatesUnchanged`.

## 3. Gate and workflow

- [ ] 3.1 `scripts/merge-check.sh` is the wrapper `exec "${ATEAM:-ateam}" review gate "$@"` with a header comment
  naming the gate's three rules and the pin; `task merge:check` desc updated. Gate: `task merge:check -- 452` against
  the merged PR passes locally with the v0.3.4 binary.
- [ ] 3.2 `.github/workflows/merge-check.yml` as design D1/D2: `pull_request` types `opened`, `synchronize`, `reopened`,
  `ready_for_review`, `edited`; concurrency per PR, `cancel-in-progress: false`; job skipped on a draft; checkout at
  `github.event.pull_request.base.sha` with `persist-credentials: false`; setup-go from `go.mod`; `go install` at the
  pin; env `ATEAM_REQUIRED_CHECKS: Required,Merge check`, the three read permissions, 5-minute limit. `ci.yml` loses
  the `merge-check` job; `required` needs `verify` alone. Gate: the PR's own `Merge check` run reads the forge and
  reports (rule 1 red until the ruleset edit, by design).
- [ ] 3.3 `internal/harness/toolcheck/mergecheck_test.go` deleted; one smoke test shows the wrapper execs `$ATEAM`
  with the arguments and passes exit codes 0, 1, 2 through; `TestCIWorkflowPinned` and its sensitivity test cover
  `ci.yml` (`required` needs `verify`, no write permission, 15-minute verify limit, activity types) and
  `merge-check.yml` (activity types including `edited`, draft condition, base-sha checkout, pin, env, permissions,
  limit, concurrency). Gate: `task test:tools`.

## 4. Record grammar and docs

- [ ] 4.1 `.agents/contracts/semengine-reviewer.md` § verdict points at ateam's `reviewer.md` § Record; the protocol's
  "Cross-agent review" item is rewritten to the gate (design D4); `AGENTS.md` (Commands block, § 5 merge rows,
  § Working here Merge gate), `.agents/rules-index.md` (same row counts), `.agents/skills/preflight/SKILL.md`
  (`merge:check` row), `docs/testing.md` (PATH note for a development `ateam` build) updated; `AGENTS.md` under
  24,000 bytes. Gate: `task verify` on a clean tree.

## 5. Review, ruleset, archive

- [ ] 5.1 Owner adds `Merge check` to the `main` ruleset's required checks; the PR's `Merge check` run turns green on
  rule 1. Hold: OPEN-Q owner action on GitHub after 3.2 is pushed.
- [ ] 5.2 Cross-agent review record on the PR in ateam's grammar; `implemented-by:` and `reviewed-by:` with
  `(model, effort)` in the PR body.
- [ ] 5.3 Archive as the last content commit, syncing the `merge-gate` and `harness-boundaries` deltas. Gate:
  `task spec:check` and `task docs:check`.
