# Proposal: lifecycleguard-tidy

## Why

Issue #196 (comment of 2026-10-16, moved from #153 item 6; owner, 2026-10-18: "Claim PR 1, the lifecycleguard tidy
from #196, as a draft and write its design"). Three problems in `internal/lifecycleguard`:

- It refuses the same bad input in different ways. A nil context gets `ErrInvalidConfig` in `Start` and
  `ErrInvalidData` in `Stop`. An ended context is wrapped as invalid in `Start` and returned bare (class transient) in
  `Stop`.
- Nothing tests its refusal of a concurrent `Stop`.
- It takes the cleanup separately in `Start` and in `Stop`, and graph-ingest passes two different values.

This is PR 1 of the order recorded on #282 (comment 6249934379, point 5), ahead of the `Start`-context change (PR 2,
ADR-113). Because PR 2 changes the lifecycle suite, this change keeps the guard's behaviour under today's suite exactly
as it is.

## What Changes

- `Start` and `Stop` refuse a nil context and a context that has already ended through one helper, the same way:
  - an error classified invalid;
  - for a nil context, an error that matches `errs.ErrInvalidData` (Start matched `ErrInvalidConfig`);
  - for an ended context, an error that still matches the context's error under `errors.Is` (Stop returned it bare);
  - the same message in both methods apart from the method's name.
- The cleanup is given once, to `Start`. `Stop` takes no cleanup and runs the one the admitted `Start` was given. A
  refused `Start` records nothing.
  `func (g *Guard) Start(ctx context.Context, start, cleanup func(context.Context) error) (startErr error)` is
  unchanged; `func (g *Guard) Stop(ctx context.Context) error` replaces `Stop(ctx, cleanup)`.
- graph-ingest: `Stop` calls `c.lifecycle.Stop(ctx)`. The "component stopped gracefully" log moves to the end of
  `release`, so it is printed after any cleanup that succeeds. A repeated `Stop` that has nothing left to release runs
  no cleanup and prints nothing.
- New guard tests:
  - the refusal shape, in both methods;
  - the cleanup the admitted `Start` was given is the one every `Stop` runs;
  - a concurrent `Stop` is refused with a transient error.
- Two graph-ingest test helpers now give a cleanup that fails on its first call and runs `release` after that, so
  their tests keep what they assert.
- The guard's package and method doc comments state the one refusal and the one cleanup.

## Capabilities

None changed. No spec names the guard, its refusal classes or how its cleanup is passed. The suite requirements it
satisfies ("Portable floor", "Failed start whose own cleanup fails") are unchanged (`skip_specs: true`).

## Impact

- Files: `internal/lifecycleguard/guard.go`, `guard_test.go`; `processor/graph-ingest/component.go` (`Stop`,
  `release`), `lifecycle_owner_test.go`, `test_owner_support_test.go`.
- No exported signature changes outside `internal/`. What callers of graph-ingest can observe:
  - `Start(nil)` matches `ErrInvalidData`, not `ErrInvalidConfig`, with a new message.
  - `Stop` given an ended context is classified invalid, not transient; `errors.Is` on the context's error still holds.
  - The stop log also follows a failed `Start` whose rollback succeeded.
  - No test, caller or consumer in scope reads any of these.
- No spec, ledger or rule text changes.
- Addresses #196, which stays open as the adoption tracker.
