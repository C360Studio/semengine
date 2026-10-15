# Proposal: simplify-04a-05-prochost-helper-exit

## Why

A `prochost` helper keeps running after the test process that started it has ended without running its cleanup. A
helper runs in a process group of its own (`internal/harness/prochost/prochost.go:91`), and the only thing that ends a
helper the test did not end itself is the test's cleanup (`:98-107`). A test process interrupted with Ctrl-C, killed,
panicking, or hitting its `-timeout` skips that cleanup. On 2026-10-12 a Ctrl-C to an integration run left
graph-ingest's settlement helper running under PID 1, connected to a leftover NATS container, until it was killed by
hand (PR #206 comment 6136509092). A probe at go1.26.9 shows the helper survives every way its parent can end
without cleanup (`inventory.md`, P1).

## What Changes

- A helper started by `prochost.Start` exits on its own once the test process that started it has ended, however that
  process ended. It writes a line to its stderr saying why and exits non-zero. While the test process runs, nothing
  changes.
- A helper's standard input belongs to `prochost`; helper functions do not read it. No helper reads it today.
- The helper keeps its own process group, so `Signal`, `Pause`, `Resume` and `Kill` keep their meaning.
- The `prochost` doc comment and `docs/testing.md` say this and name #208. They stop citing Go's deadlock detector as
  the reason a helper parks on SIGTERM, because the new watch keeps that detector from firing in a helper.

No exported symbol is added or removed.

## Capabilities

- Modified: `process-host` (added requirement: a helper ends when its test process ends).

## Impact

- `internal/harness/prochost/prochost.go`, `internal/harness/prochost/prochost_test.go`.
- `docs/testing.md` (the `prochost` paragraph).
- `processor/graph-ingest/settlement_integration_test.go`: unchanged; its integration run is part of the gate.
- Closes #208; addresses #223.
