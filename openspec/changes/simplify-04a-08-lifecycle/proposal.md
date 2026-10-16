# Proposal: simplify-04a-08-lifecycle

## Why

PR 9b of epic #223 (owner, 2026-10-16: "Pick up 9b (pkg/lifecycle) next"). `pkg/lifecycle`'s graph emitter keeps its
own copy of the error-to-commit-state mapping. It reports a write that may be stored as retryable (#257), and a
request cancelled before it was sent as commit-unknown. PR 9a (#265) exported `projection.ClassifyMutationError` for
this. The package also refuses "no NATS client" in five places with four messages, wraps its watch loop in layers
whose reason went with the channel API, matches entity IDs two ways, documents `Fail`, `Complete` and `Children`
differently from what they do, and tests against doubles that accept stored values graph-ingest refuses
(#154, #193, #199).

## What Changes

- The emitter reports every write failure through `projection.ClassifyMutationError`, still wrapping
  `ErrEntityNotFound` and `ErrAlreadyExists`. `lifecycleMutationOperation` is removed: each write passes its own
  operation.
- A manager built without a NATS client registers workflows and answers definition queries. Every call that needs
  NATS fails with one error naming the missing client (owner question 1).
- One watch loop delivers `Event`s; `Watch` is that loop without deletes. A failure is returned, not also logged.
- One matcher: `semtypes.MatchEntityIDPattern`.
- `Complete` picks the first reachable terminal in the current phase's declared edge order (owner question 2). The
  `Fail` doc states the edge it needs.
- `Children` reads children in link order until the page is full, so reads stay bounded by the page (ADR-049). It
  skips and slices as `List` does, and owner question 3 adds List's rule for read errors.
- Test doubles refuse what graph-ingest's stored-value rule refuses.
- Docs say what the code does, not what it used to do.

## Capabilities

- Modified: `projection-mutation` (the one mapping also covers lifecycle's emitter).

## Impact

- `pkg/lifecycle` (`graph_emit.go`, `manager.go`, `manager_query.go`, `errors.go`, tests);
  `docs/admission-ledger.yaml` (`pkg/lifecycle` contract).
- Exported surface: no symbol added or removed. Behaviour changes for callers: write errors (#257), Complete's pick,
  Children's pages (a non-nil empty result; no panic on a negative Offset), and the Watch logs.
- Closes #154, #193, #199, #257. Addresses #223.
