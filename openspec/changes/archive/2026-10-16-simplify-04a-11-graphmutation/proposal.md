# Proposal: simplify-04a-11-graphmutation

## Why

PR 8a of epic #223 (owner, 2026-10-16: "Pick up PR 8 next, including #268 and #276"; "Five PRs, one per package, bugs
first"; #223 comment 6216623583).

`internal/graphmutation` labels a write as "commit unknown" even when natsclient refused it before sending, because
the broker is reconnecting or the circuit breaker is open. Callers then never retry a write that was never sent
(#268). The client's own refusals (no context, a value that will not encode as JSON) read the same way. Separately,
`ResolveSubject` takes a family that can have only one value, and graph-ingest builds that value from config and then
checks it twice (#137).

## What Changes

- natsclient's `ErrNotConnected` and `ErrCircuitOpen` are reported as `TransportUnavailable`: nothing was sent.
  `projection.ClassifyMutationError`, and so `pkg/lifecycle`, read them as not committed, unavailable, transient,
  with no new rule.
- Requests the client refuses itself before sending are class invalid with code `invalid_request`, and no request
  goes out. This covers a nil client, a nil context, an operation not admitted, a payload that does not encode, and
  a Create with no entity (now checked before sending).
- `ResolveSubject(family, op)` becomes `Subject(op)`. graph-ingest checks its provider port once and builds one route
  table.
- The spec, the projection doc comment, the tests, the ledger and `docs/contract.md` drop "until #268".

## Capabilities

- Modified: `projection-mutation` (the requirement "One mapping from a mutation request's error to its commit state").

## Impact

- Files: `internal/graphmutation` (`client.go`, `protocol.go`, tests);
  `processor/graph-ingest/canonical_mutations.go`, `component.go:312` and two tests;
  `pkg/projection/mutation_client.go` (doc comment) and two tests;
  `pkg/lifecycle/manager_integration_test.go` (the rename); `docs/admission-ledger.yaml`; `docs/contract.md`.
- Exported surface: `graphmutation.ResolveSubject` is replaced by `graphmutation.Subject`. The package is internal,
  so no consumer imports it.
- Behaviour changes for callers: write errors on reconnect or with the breaker open, and the client's own refusals.
- Closes #268, #137. Addresses #223.
