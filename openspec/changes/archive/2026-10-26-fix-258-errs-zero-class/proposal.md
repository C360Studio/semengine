# Proposal: fix-258-errs-zero-class

## Why

`errs.ErrorClass`'s zero value is `ErrorTransient` (`pkg/errs/errs.go:17`). A `*errs.ClassifiedError` built as a
struct literal without `Class` therefore reads as retryable through `errs.IsTransient`, `errs.Classify`, the
`X-Error-Class` header and the append-result wire (#258). That is the fail-open default #167 removed from
`errs.Classify` for unclassified errors. No production literal in this repository or in the four starter consumers
omits `Class` today (`inventory.md` §2b and the adopter seam), so this closes a trap for future code, not a live
failure.

The issue's `projection.MutationError` half is out of scope: nothing outside `pkg/projection` reads
`MutationError.Class`, and the predicates read its `Err` instead (`inventory.md` §1).

The owner chose the one-line fix with a light process on 2026-10-26: tests and code, then Codex's code review; no
separate design document.

## What Changes

- `ErrorTransient` is numbered from one (`iota + 1`), leaving the zero value unnamed, as `config/key_family.go:37`
  does. No class is added: the set stays transient, invalid and fatal (pin ADR-060 `:394`).
- A `*errs.ClassifiedError` with an unset class is none of the three to the predicates, so `errs.Classify` returns
  `errs.ErrorFatal`, and `ErrorClass.String()` already answers `"unknown"` for it.
- Tests in `pkg/errs` pin both.

## Capabilities

### Modified Capabilities

- `error-classification`: a new requirement, "A classified error built without a class is fatal".

## Impact

- Code: `pkg/errs/errs.go`; `pkg/errs/errs_test.go`.
- Docs: the `pkg/errs` row of `docs/admission-ledger.yaml` (`contract` and `known_risks`).
- Behaviour: an unset-class `ClassifiedError` reads fatal, not transient. That includes one wrapping a transient
  sentinel such as `context.DeadlineExceeded`, because `errors.As` stops at the first `ClassifiedError`. Named
  classes keep their string spellings and the class is never encoded as a number (`inventory.md` §2c); a classless
  error's `X-Error-Class` header changes from `transient` to `fatal`, which is the intended fix. graph-ingest's keyed ingest
  still retries an error that is neither invalid nor fatal (`processor/graph-ingest/keyed_ingest.go:322`), as ADR-111
  Decision 3 allows; that path reads the predicates, not `Classify`.
