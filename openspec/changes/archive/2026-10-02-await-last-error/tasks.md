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
baseline, mutation and restored exits 0/1/0; `task verify` exit 0 on that clean commit, including five
shuffled unit runs. Draft PR #59 records the commands, full focused outputs, runtime-file checksum,
verification limits and pickup fields, and CI run 37009998371 passed on the published head `2365c45`.
The local logs are under `.evidence/await-last-error/` in the claim's worktree, which git ignores; they
are not part of the repository. The coordinator publishes this task update and releases write ownership
in the PR description after publication.

## 2. Leg 2: Claude

The owner assigns the remaining steps to a fresh Claude session taking the draft PR alone.
Codex stops after section 1. What follows the archive (the check of the archive and spec sync, undraft,
CI, `task merge:check` and the merge) is recorded on PR #59, not here: a task cannot be ticked for
something that happens after the last content commit.

- [x] 2.1 Picked up with semengine-pickup at `2365c45`: worktree, branch, upstream, CI and the stop point
      verified, and Codex's release of write ownership read from PR #59. `main` (`39badc4`) merged in as
      `f39ead2`.
- [x] 2.2 Independent implementation review at `f39ead2` (changes requested, nothing blocking), and its re-check
      of the fixes at `474aa38` (approve), both recorded on PR #59.
- [x] 2.3 Known flake #62 was fixed on `main` by PR #63 (`e69b59a`), and `main` merged in as `8e38e89`.
      `task verify` passed on `8e38e89`, the branch's last commit before the archive commit, with the branch
      up to date with `main`.
- [x] 2.4 The change is archived and `openspec/specs/lifecycle-suite/spec.md` synced as the last content
      commit; `task spec:check` passes and `task spec:queue` shows no open hold.

Leg-2 evidence: the implementation review at `f39ead2` found the test and the delta correct,
reproduced the mutation check (three mutants, all detected) and asked for three changes, made in the
commit after it: tasks 2.3 and 2.4 as first written could not be ticked before the archive; the doc
comment on `Await` kept the phrase the ruling resolved; and the leg-1 evidence pointed at local files.
`task verify` on `f39ead2` failed in `test:repeat` on `TestSignalDuringPullReapsThePull`, a runner test
this change does not touch. That was known flake #62; PR #63 fixed it. The run on `8e38e89` started
2026-10-02T15:06:29Z and exited 0, every step passing, `test:repeat` included.
