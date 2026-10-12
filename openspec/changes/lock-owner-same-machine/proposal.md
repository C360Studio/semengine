# Proposal: lock-owner-same-machine

## Why

Issue #201 is labelled `class:flake`. The integration runner records the lock owner's host with `hostname`
(`scripts/test-integration.sh:99`), and on macOS `hostname` follows the network (`Cobys-MBP.localdomain` on one,
`Cobys-MacBook-Pro.local` on another). `TestR4CanonicalInvocationAndOwnerFormat` calls `hostname` again and failed
once when the name changed mid-run. Worse, both judges of a stale lock refuse to judge an owner whose recorded host
differs from the current name (`scripts/test-integration.sh:176`, `internal/harness/natsfixture/admission.go:93-97`),
so a lock left by a runner that died before a network change is never reclaimed: every later run waits out its
budget and is refused.

The lock is a directory in this machine's `/tmp` (the lock-dir override exists for the runner's own contract tests),
so every owner it can name ran on this machine. The pid and its recorded start time already prove whether that owner
is alive.

## What changes

- Neither judge compares the recorded host with the current host name: an owner is live when its pid is alive with
  the recorded start time, and stale otherwise, as today for a matching host. The host stays in the owner record and
  in the runner's messages, as information.
- `TestR4CanonicalInvocationAndOwnerFormat` checks the recorded host is not empty, instead of calling `hostname`
  again.
- The `integration-test-runner` and `nats-fixture` scenarios that say an owner on another host is respected or
  refused are changed to match.

Owner: #201 is folded into PR #200 because `merge-check` passes only a pull request that closes every open
`class:flake` issue (2026-10-12; the owner was told and did not object).
