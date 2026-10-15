# Proposal: simplify-04a-04-errs-classify

## Why

`errs.Classify` (`pkg/errs/errs.go:218-236`) gives an error that carries no class the transient class ("Default to
transient for unknown errors to allow retry"). An error nobody classified is therefore retryable by default, which is
the fail-open direction. Two places then build a commit-unknown `projection.MutationError` with
`Class: errs.Classify(err)` (`pkg/lifecycle/graph_emit.go:110`, `pkg/projection/mutation_client.go:467`), so a write
that may already be stored is spelled retryable. ADR-111 Decision 3 says a commit-unknown result "is never retried",
and names this defect's home as #167.

Changing the default is not enough by itself. `errs.IsTransient` reads through `MutationError` to the error it wraps,
so a commit-unknown error wrapping `context.DeadlineExceeded` still reads transient whatever its `Class` field says.

## What Changes

- `errs.Classify` returns `ErrorFatal` for an unclassified error: one with no `*errs.ClassifiedError` in its chain and
  none of the listed sentinels. The two other places that turn an unrecognised class into transient follow it:
  `natsclient`'s `classForHeader` default branch and `pkg/projection`'s `classifiedAppendFailure`.
- Every commit-unknown `projection.MutationError` from `pkg/projection` and `pkg/lifecycle` carries `ErrorFatal`, and
  `errs.IsTransient` reports false for it whatever it wraps. `errors.Is`/`errors.As` still reach the cause; `Code` and
  `Detail` stay the reply's.
- Errors that relied on the old default to stay retryable are classified explicitly: `projection`'s no-responders
  error (as `pkg/lifecycle` already does) and `lifecycle.ErrUpdateRetriesExhausted`.
- `pkg/errs/doc.go`, `docs/contract.md`, the `pkg/errs` admission-ledger row and ADR-112 § 3 say the new default.

No exported symbol is added or removed.

## Capabilities

- New: `error-classification` (the unclassified-error default).
- Modified: `projection-mutation` (commit-unknown is not retryable; no-responders stays transient), `transport-client`
  (an unclassified handler error crosses the wire as fatal).

## Impact

- `pkg/errs/errs.go`, `pkg/errs/errs_test.go`, `pkg/errs/doc.go`.
- `natsclient/errors.go`, `natsclient/errors_test.go`.
- `pkg/projection/mutation_client.go`, `pkg/projection/commit_ambiguity_test.go`.
- `pkg/lifecycle/graph_emit.go`, `pkg/lifecycle/errors.go`, `pkg/lifecycle/graph_emit_contract_test.go`,
  `pkg/lifecycle/manager_test.go`.
- `docs/admission-ledger.yaml`, `docs/contract.md`, `docs/adr/112-natsclient-contract.md`.
- Wire behaviour: a `SubscribeForRequests` handler error with no class is sent as `fatal`, not `transient`.
- Closes #167; addresses #223.
