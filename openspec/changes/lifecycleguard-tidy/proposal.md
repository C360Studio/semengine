# Proposal: lifecycleguard-tidy

## Why

Issue #196 (comment of 2026-10-16, moved from #153 item 6; owner, 2026-10-18: "Claim PR 1, the lifecycleguard tidy
from #196, as a draft and write its design"). Three problems in `internal/lifecycleguard`:

- It refuses a nil context two ways: `ErrInvalidConfig`, with one message, in `Start`, and `ErrInvalidData`, with
  another, in `Stop`. #153 item 6 also named the ended context, wrapped as invalid in `Start` and returned bare in
  `Stop`. This change keeps that difference on purpose: `Stop` then returns the same error whether its context ended
  before the call, during its wait or inside the cleanup, as every stop method in the tree does (design D1).
- Nothing tests its refusal of a concurrent `Stop`.
- It takes the cleanup separately in `Start` and in `Stop`, and graph-ingest passes two different values.
- A cleanup that panics can never run again. Inside `Stop`, every later `Stop` is refused as concurrent; inside a
  failed `Start`'s rollback, every later `Stop` waits until its own context ends. Nothing triggers this today; it was
  found in this change's review (design D7).

This is PR 1 of the order recorded on #282 (comment 6249934379, point 5), ahead of the `Start`-context change (PR 2,
ADR-113). Because PR 2 changes the lifecycle suite, this change keeps the guard's behaviour under today's suite exactly
as it is.

## What Changes

- `Start` and `Stop` refuse a nil context through one helper, the same way: an error classified invalid that matches
  `errs.ErrInvalidData` (Start matched `ErrInvalidConfig`), with the same message apart from the method's name.
- An ended context is handled as today, now on purpose and tested. `Start` refuses it as invalid, still matching the
  context's error under `errors.Is`. `Stop` returns the context's own error, the same one it returns when its context
  ends during its wait or inside the cleanup.
- The cleanup is given once, to `Start`. `Stop` takes no cleanup and runs the one the admitted `Start` was given. The
  admitted `Start` is one that passed the nil-context, ended-context and second-`Start` refusals. A refused `Start`
  records nothing.
  `func (g *Guard) Start(ctx context.Context, start, cleanup func(context.Context) error) (startErr error)` is
  unchanged; `func (g *Guard) Stop(ctx context.Context) error` replaces `Stop(ctx, cleanup)`.
- graph-ingest: `Stop` calls `c.lifecycle.Stop(ctx)`. The "component stopped gracefully" log moves to the end of
  `release`, so it is printed after any cleanup that succeeds. A repeated `Stop` that has nothing left to release runs
  no cleanup and prints nothing.
- A cleanup that panics, in a `Stop` or in a failed `Start`'s rollback, stays pending like one that failed, and the
  panic continues to the caller; a later `Stop` runs the cleanup again.
- New guard tests:
  - the nil-context refusal in both methods, and the ended context in each;
  - the cleanup the admitted `Start` was given is the one every `Stop` runs;
  - a concurrent `Stop` is refused with a transient error;
  - a `Stop`, and a failed `Start`, whose cleanup panics leave that cleanup for the next `Stop`.
- Two graph-ingest test helpers now give a cleanup that fails on its first call and runs `release` after that, so
  their tests keep what they assert.
- The guard's package and method doc comments state the one nil-context refusal, the two answers to an ended
  context, the one cleanup and the panicking cleanup.

## Capabilities

None changed. No spec names the guard, its refusal classes or how its cleanup is passed. The suite requirements it
satisfies ("Portable floor", "Failed start whose own cleanup fails") are unchanged (`skip_specs: true`).

## Impact

- Files: `internal/lifecycleguard/guard.go`, `guard_test.go`; `processor/graph-ingest/component.go` (`Stop`,
  `release`), `lifecycle_owner_test.go`, `test_owner_support_test.go`.
- No exported signature changes outside `internal/`. What callers of graph-ingest can observe:
  - `Start(nil)` matches `ErrInvalidData`, not `ErrInvalidConfig`, with a new message.
  - The stop log also follows a failed `Start` whose rollback succeeded.
  - A `release` that panics would no longer leave every later `Stop` refused or waiting (none panics today).
  - No test, caller or consumer in scope reads any of these.
- No spec, ledger or rule text changes.
- Addresses #196, which stays open as the adoption tracker.
