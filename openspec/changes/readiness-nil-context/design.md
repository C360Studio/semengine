# Design: readiness-nil-context

Depends on `INVENTORY PASS` for `inventory.md`; pins are at the base commit.

## D1. Refuse nil first, in Run's words

- `Publish`: `if ctx == nil { return errors.New("readiness: nil context") }` as its first statement, before the
  timestamp, marshal and `Put` (`publisher.go:71-82`). Doc comment: a nil context is refused before any write.
- `WaitForFirst`: the same check before the `select` (`watcher.go:252-258`), so nil is refused even after an
  envelope has arrived; a check inside the select cannot be written, since `ctx.Done()` is evaluated first. Doc
  comment: a nil context returns an error.
- Wording and shape copy `watcher.go:196-197` (inventory § 5). No sentinel, no classified error, no helper: the
  package's other refusals are plain `errors.New`, and no caller tells this error apart.
- Options: (a) chosen. (b) a shared `errNilContext` sentinel for three sites: no reader would test it with
  `errors.Is`. (c) a classified error like `storagereport`'s: `graph/readiness` has no classified vocabulary.
  (d) do nothing: a nil context keeps panicking in the caller's goroutine.

## D2. Tests (public API, independent oracle)

- `TestPublishRefusesNilContext` (`publisher_test.go`): rows nil context (want an error naming `nil context`,
  `fakeWriter.calls == 0`) and `t.Context()` (want nil, one `Put`). On the base the nil row panics.
- `TestWaitForFirstRefusesNilContext` (`watcher_test.go`, `synctest` harness): after `deliver(readyEntry(t))`, nil
  returns an error naming `nil context` and `t.Context()` returns nil; before any delivery, nil returns an error
  without blocking. On the base both nil rows panic.
- Sensitivity, `task mutate:check` with `-expect-text` on each test's failure text: the check replaced by
  `ctx = context.Background()` (the forbidden nil default) is detected by the Publish nil row (error nil, one `Put`)
  and by the WaitForFirst after-delivery row (returns nil). A panic or deadlock reading is inconclusive, not detection.

## D3. Spec and ledger

- MODIFIED requirement (delta `specs/graph-readiness/spec.md`) adds both methods and one scenario.
- Ledger `contract` gains `(10) Publish and WaitForFirst return an error for a nil ctx, Publish before any write
  (readiness-nil-context, #297)`. Merge order with #292: either first; whichever lands second merges `origin/main`
  and resolves the adjacent-line ledger hunk and the `watcher.go` hunks (`:252` here, `:303-310` there) by hand.

Owner questions: none.
