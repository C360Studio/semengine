# Proposal: readiness-nil-context

## Why

Issue #297 (owner, 2026-10-17: "Take #297: refuse a nil context in Publish and WaitForFirst"). `Publisher.Publish`
and `Watcher.WaitForFirst` panic when given a nil context, while `Watcher.Run` in the same package returns an error.
Exported calls that take a context and return an error refuse nil (architect and reviewer contracts, Context
ownership). Found in the review of #289; inherited from the SemStreams pin.

## What Changes

- `Publish(nil, ...)` returns `readiness: nil context` and writes nothing.
- `WaitForFirst(nil)` returns `readiness: nil context`, whether or not an envelope has arrived.
- Doc comments of both say so; the `graph-readiness` spec names both; the ledger row records the change from the pin.

## Capabilities

- Modified: `graph-readiness` ("Readiness wiring is refused, not ignored").

## Impact

- Files: `graph/readiness/publisher.go`, `watcher.go`, `publisher_test.go`, `watcher_test.go`;
  `docs/admission-ledger.yaml` (`graph/readiness` row, `contract`); `openspec/specs/graph-readiness/spec.md` at archive.
- No API, wire or metric change. Callers passing a real context see no difference.
- Closes #297.
