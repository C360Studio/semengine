# Proposal: graph-ingest-metering

## Why

This is the small follow-on PR to simplification wave PR 10 (#299). It covers `processor/graph-ingest` only. Owner,
2026-10-18 (#300 comment 6250544834): section B of #300 "is the one item with a standing owner ruling (L, #91 comment
6062681355) still unimplemented, and it is the root cause of #303. It goes into one small follow-on PR right
after #299 lands, together with the two defects #302 and #303."

Today:

- A revision mismatch logs two WARN lines for one request (#303).
- Every mutation reply is counted by decoding the reply the handler has just encoded, behind two stacked wrappers
  (#300 B).
- `DataFlow`'s messages-per-second counts entity writes, misses appends entirely, and counts a stream message that
  wrote nothing (#302).
- Query refusals spell "invalid" and "internal" several ways, some with no code at all (#300 B).

The outcome label already is the classified code: PR #286 did that part of ruling L. What remains is ruling L's "the
re-decode layer go[es]" and the second wrapper.

## What Changes

- **One wrapper per mutation request.** It records the rejection counter, the outcome counter and one WARN line for
  a refused request. Each handler records its own outcome where it decides it. The re-decode goes. (#303, #300 B)
- **`DataFlow` counts input messages:** stream deliveries and mutation requests. `BytesPerSecond` counts their bytes.
  `entities_updated_total` also counts appended entities. (#302, owner question 1)
- **Query refusals carry a code without changing class**, and natsclient's `response_too_large` gets one exported
  spelling. (#300 B, owner questions 2 and 3)

## Capabilities

- `graph-transport-boundary`:
  - MODIFIED "Mutation replies carry four outcomes": one count per reply, one WARN per refusal.
  - ADDED "Query refusals carry a code" (if owner question 2 is answered as recommended).
- `graph-ingest-recovery`: ADDED "graph-ingest's data flow counts input messages".

## Impact

- **Files:**
  - `processor/graph-ingest`: `canonical_mutations.go`, `mutation_runtime.go`, `query.go`, `component.go`, tests,
    `README.md`.
  - `natsclient/request.go` (question 3).
  - `docs/admission-ledger.yaml` (`processor/graph-ingest` row; `natsclient` row if question 3 is answered yes).
- **Metrics:**
  - `graph_mutation_outcomes_total`: label values unchanged. The `internal` count on a reply that fails to re-decode
    disappears; that path is unreachable.
  - `entities_updated_total` rises on append. semboids reads it as its ingest rate (`cmd/sweep/main.go:466-467`); it
    does not append, so its numbers do not move.
- **Logs:** one WARN per refused mutation. It carries the error's detail, so the expected revision of a revision
  mismatch is still logged.
- **`DataFlow`:** no reader exists in this tree or at the pin (inventory § 1, #302).
- **Wire:** mutation replies unchanged. Query replies gain codes and keep their classes (question 2).
- **Issues:** closes #302 and #303. Part of #300 (section B only).
