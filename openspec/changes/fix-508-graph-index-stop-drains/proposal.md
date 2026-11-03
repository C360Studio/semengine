# Proposal: fix-508-graph-index-stop-drains

Approved-by:

## Why

Issue #508. When `processor/graph-index` stops, it drains its eight query subscriptions one at a time and returns at the
first drain that fails (`cleanup`, `processor/graph-index/component.go:305-309`). If a handler is still running when
`Stop`'s context ends, the verbs after it are never drained. They keep answering queries while `Stop` reports failure,
until a later `Stop` resumes.

A reproducing test on the base commit, with one stuck handler first in the list, shows all eight verbs still answering
60 seconds after `Stop` returned. `processor/graph-ingest` joins every drain's error (`component.go:575-579`), and the
`processor/graph-query` port in PR #504 does the same.

## What Changes

- `cleanup` begins every drain and joins their errors before it returns. As today, a failed drain returns with the
  handles kept, and the next `Stop` resumes.
- A new integration test, `TestStopWithAStuckHandlerStopsEveryOtherVerb`, holds one handler running past `Stop`'s
  context. It requires `context.Canceled` from `Stop`, requires no graph-index verb to answer afterwards, and requires a
  later `Stop` to finish once the handler returns.

## Not covered

- When a drain fails, the steps after the drains (ending the feeds, draining the index pool) still wait for the next
  `Stop`, as today. Indexing therefore continues until then. #508's done-when asks only for every drain to begin, and
  those steps have their own ordering (ADR-113).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. No requirement in `openspec/specs/graph-index/spec.md` describes this behaviour, and adding one would move this
change to the design track.
