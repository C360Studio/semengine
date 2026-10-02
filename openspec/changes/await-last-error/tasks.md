# Await last-observation tasks

## 1. Leg 1: Codex

- [x] 1.1 Commit this OpenSpec change first, including the complete MODIFIED requirement; install the pinned
      dependencies with `npm ci` and run `task spec:check`.
- [x] 1.2 Add one deterministic error-then-clean observation test beside `TestAwaitReportsLastObservation`;
      end the wait through test-owned cancellation and keep Await unchanged.
- [x] 1.3 Run the new test verbosely under the race detector and confirm it passes on the existing implementation.
- [x] 1.4 Perform the earlier-error-retention mutation check; record baseline, intended failure and restored pass,
      and confirm the runtime file is restored exactly.
- [x] 1.5 Run the required preflight before implementation publication, including the repeated unit gate;
      record commands, tested state, results and limits.
- [x] 1.6 Run semengine-handoff; record the validated checkpoint, explicit leg-2 stop point and pickup evidence
      in draft PR #59, with these tasks reconciled to the recorded evidence.

Leg-1 evidence: test commit `ca3eaf46d9068b0d9794e33aae80ac0165be2695`; focused race-test
baseline, mutation and restored exits 0/1/0 in `.evidence/await-last-error/`; `task verify` exit 0 on
that clean commit, including five shuffled unit runs. Draft PR #59 records the commands, full focused
outputs, runtime-file checksum, verification limits and pickup fields. The coordinator publishes this
task update and releases write ownership in the PR description after publication.

## 2. Hold: fresh Claude pickup for leg 2

The owner assigns the remaining steps to a fresh Claude session taking the draft PR alone.
Codex stops after section 1.

- [ ] 2.1 Hold: fresh Claude leg-2 pickup. Use semengine-pickup to verify the worktree, branch, CI,
      evidence and stop point.
- [ ] 2.2 Obtain independent implementation review and resolve its findings.
- [ ] 2.3 Archive and synchronize the current spec as the final content commit; obtain the archive/spec-sync check.
- [ ] 2.4 Continue the owner's leg-2 landing instructions through the repository merge gates.
