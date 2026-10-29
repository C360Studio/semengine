# Tasks: adopt-ateam-merge-gate

Track: `design` (adds spec requirements; touches `merge-gate` and `harness-boundaries`). Owner ruling on #447
(2026-10-28): proposal approval and the design track are adopted.

## 1. Proposal and design

- [x] 1.1 Proposal, design, tasks and spec deltas committed; draft PR opened with `Closes #449` and `Addresses #446`.
  Gate: `task spec:check` and `task docs:check`.
  Done in 54d760c; draft PR #469. Owner approved the proposal and design D1 in session on 2026-10-29 (`Approved-by:`
  filled, recorded on #449).

## 2. Pin and templates

- [x] 2.1 The ateam pin is `v0.3.4` in `.github/workflows/ci.yml` (verify job); `scripts/doctor.sh` and
  `TestAteamTemplatesUnchanged` read every install line across `ci.yml` and `merge-check.yml` and require one version.
  Gate: `scripts/doctor.sh` with the v0.3.4 binary on PATH; the test.
  Done in 886c9b9 (pin) and f6b9852 (both readers); `scripts/doctor.sh` prints `ok ateam v0.3.4` with
  `$(go env GOPATH)/bin` first on PATH, and fails naming `v0.3.3 v0.3.4` on a copy whose `merge-check.yml` names
  v0.3.3; `TestAteamTemplatesUnchanged` and the "pins differ" plants of `TestCIWorkflowPinnedSensitivity` pass in
  `task verify` on 8524f60. Notes: `notes/2.1.md`.
- [x] 2.2 The three template files v0.3.4 changed (`.agents/contracts/developer.md`, `.agents/contracts/reviewer.md`,
  `.agents/skills/review-pr/SKILL.md`) are byte-identical to `ateam init` at v0.3.4; the harness-boundaries doctor
  requirement names the 100 ms ceiling. Gate: `go test ./internal/harness/toolcheck -run TestAteamTemplatesUnchanged`.
  Done in 886c9b9 (the three files copied from `ateam init` v0.3.4 into an empty directory holding the review policy;
  the delta already names 100 ms); the test passes with ateam v0.3.4, and in `task verify` on 8524f60.

## 3. Gate and workflow

- [x] 3.1 `scripts/merge-check.sh` is the wrapper `exec "${ATEAM:-ateam}" review gate "$@"` with a header comment
  naming the gate's three rules and the pin; `task merge:check` desc updated. Gate: `task merge:check -- 452` against
  the merged PR passes locally with the v0.3.4 binary.
  Done in 99b8517; `task merge:check -- 452` with ateam v0.3.4 printed `merge-check: ok` and exited 0 (review record
  by codex at d035dfd, strength pass, no open `class:flake` issue).
- [x] 3.2 `.github/workflows/merge-check.yml` as design D1/D2: `pull_request` types `opened`, `synchronize`, `reopened`,
  `ready_for_review`, `edited`; concurrency per PR, `cancel-in-progress: false`; job skipped on a draft; checkout at
  `github.event.pull_request.base.sha` with `persist-credentials: false`; setup-go from `go.mod`; `go install` at the
  pin; env `ATEAM_REQUIRED_CHECKS: Required,Merge check`, the three read permissions, 5-minute limit. `ci.yml` loses
  the `merge-check` job; `required` needs `verify` alone. Gate: `TestCIWorkflowPinned` passes; the PR's checks show
  workflow `Merge check` ran from the PR's own `merge-check.yml` and was skipped on the draft, with no check named
  `Merge check`; `ATEAM_REQUIRED_CHECKS='Required,Merge check' scripts/merge-check.sh 469` with ateam v0.3.4 prints
  `no rule in force on main requires Merge check` and exits 1. (Gate corrected in review round 1: `notes/3.2.md`.)
  Done in f6b9852. `TestCIWorkflowPinned` passes in `task verify` on 8524f60; at 92c1447 `gh pr checks 469` lists
  the `merge-check.yml` job as `skipping` (run 40929694074) and no check named `Merge check`; the local command printed
  `no rule in force on main requires Merge check` and exited 1.
- [x] 3.3 `internal/harness/toolcheck/mergecheck_test.go` deleted; one smoke test shows the wrapper execs `$ATEAM`
  with the arguments and passes exit codes 0, 1, 2 through; `TestCIWorkflowPinned` and its sensitivity test cover
  `ci.yml` (`required` needs `verify`, no write permission, 15-minute verify limit, activity types) and
  `merge-check.yml` (activity types including `edited`, draft condition, base-sha checkout, pin, env, permissions,
  limit, concurrency). Gate: `task test:tools`.
  Done in 99b8517 (`mergecheck_test.go` deleted, `TestMergeCheckWrapper` in `mergewrapper_test.go`) and f6b9852
  (`TestCIWorkflowPinned` over both files); `task test:tools` passed, and again as `test:tools:changed` in
  `task verify` on 8524f60. Notes: `notes/3.3.md`.

## 4. Record grammar and docs

- [x] 4.1 `.agents/contracts/semengine-reviewer.md` § verdict points at ateam's `reviewer.md` § Record; the protocol's
  "Cross-agent review" item is rewritten to the gate (design D4); `AGENTS.md` (Commands block, § 5 merge rows,
  § Working here Merge gate), `.agents/rules-index.md` (same row counts), `.agents/skills/preflight/SKILL.md`
  (`merge:check` row), `docs/testing.md` (PATH note for a development `ateam` build) updated; `AGENTS.md` under
  24,000 bytes. Gate: `task verify` on a clean tree.
  Done in 8524f60; `AGENTS.md` is 23,890 bytes; `task verify` on 8524f60, clean tree, ateam v0.3.4 first on PATH:
  `verify: ok`, exit 0, `ateam doctor --check` violations null. Dropped protocol sentences: `notes/4.1.md`;
  `docs/repository-map.md` fixed in review round 1; the `merge-gate` Purpose paragraph goes with task 5.3 (#470).

## 5. Review, ruleset, archive

- [x] 5.1 Owner adds `Merge check` to the `main` ruleset's required checks. Gate:
  `gh api repos/{owner}/{repo}/rules/branches/main` lists `Required` and `Merge check` with
  `strict_required_status_checks_policy` true, or `ATEAM_REQUIRED_CHECKS='Required,Merge check'
  scripts/merge-check.sh 469` with ateam v0.3.4 passes rule 1. Done 2026-10-29 on the owner's approval (ruleset
  `main-required-checks`, 24272345, edited through the API); the gate query lists both checks, strict on.
- [x] 5.2 Cross-agent review record on the PR in ateam's grammar; `implemented-by:` and `reviewed-by:` with
  `(model, effort)` in the PR body. Record: PR #469 comment 6507637104, `Reviewed at 3f0c800`, `Verdict: APPROVE`,
  `reviewed-by: codex (gpt-6-astra, high)`; its two nonblocking findings: the design wording fixed here, the
  `merge-check.yml` header comment filed as #471 (a workflow edit after the record would void it).
- [x] 5.3 Archive as the last content commit, syncing the `merge-gate` and `harness-boundaries` deltas; the archive
  commit also rewrites the `merge-gate` spec's Purpose paragraph, which still says a code pull request fails
  `merge-check` without the two agent lines (#470). Gate: `task spec:check` and `task docs:check`.
