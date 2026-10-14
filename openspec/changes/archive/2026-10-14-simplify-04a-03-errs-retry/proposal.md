# Proposal: simplify-04a-03-errs-retry

## Why

1. `errs.WrapTransient`, `errs.WrapFatal`, and `errs.WrapInvalid` (`pkg/errs/errs.go:359-402`) return `nil` when
   `err == nil && action == ""`, but synthesize a brand-new `errors.New(action)` when `err == nil && action != ""`
   (carried from SemStreams `#94` for `graph/inference/applier.go`). That violates Go's universal `Wrap*(nil, ...) == nil`
   convention, disagrees with `errs.Wrap(nil, ...)` and `errs.Classified(..., nil)` in the same file, and has zero
   callers in SemEngine.
2. `pkg/errs` defines a second `errs.RetryConfig` (`pkg/errs/errs.go:404-481`) alongside `retry.Config` in `pkg/retry`
   and `natsclient.RetryConfig` in `natsclient/request.go`, complete with `DefaultRetryConfig()`, `ShouldRetry`,
   `ToRetryConfig()`, and `BackoffDelay()`. Zero callers in SemEngine, at the SemStreams pin (`8b99efe9`), or across
   sister repositories (`semsource`, `semconnect`, `semteams`, `semboids`) reference `errs.RetryConfig` or its methods,
   and carrying it forces `pkg/errs` to import `pkg/retry`.
3. `pkg/retry` (`pkg/retry/retry.go:11-17, 167-175`) maintains a package-global `randMu sync.Mutex` guarding a
   `math/rand` `randSource` seeded with `time.Now().UnixNano()` at `init` time. Go 1.22+ provides `math/rand/v2` with a
   lock-free per-thread top-level generator (`rand.Int64N`).

Issue #218 (part of the pre-Change 3 simplification wave, Epic #223) makes `WrapTransient`, `WrapFatal`, and
`WrapInvalid` unconditionally nil-safe, deletes the dead `errs.RetryConfig` surface and `pkg/retry` import from
`pkg/errs`, and switches `pkg/retry` jitter to `math/rand/v2`.

## What Changes

- **Nil-safe `WrapTransient`, `WrapFatal`, and `WrapInvalid` (`pkg/errs/errs.go`)**:
  - Return `nil` whenever `err == nil` in `WrapTransient`, `WrapFatal`, and `WrapInvalid`, matching `Wrap(nil, ...)` and
    `Classified(..., nil)`.
  - Update `pkg/errs/errs_test.go` to assert that `WrapTransient(nil, ...)`, `WrapFatal(nil, ...)`, and
    `WrapInvalid(nil, ...)` return `nil` both when `action == ""` and when `action != ""`.
- **Delete dead `errs.RetryConfig` and `pkg/retry` import (`pkg/errs/errs.go`, `pkg/errs/doc.go`)**:
  - Remove `RetryConfig`, `DefaultRetryConfig()`, `ShouldRetry`, `ToRetryConfig`, and `BackoffDelay` from
    `pkg/errs/errs.go`, along with the `"time"` and `"github.com/c360studio/semengine/pkg/retry"` imports, making
    `pkg/errs` a pure stdlib leaf package.
  - Remove the corresponding `TestRetryConfig_*` tests from `pkg/errs/errs_test.go` and update `pkg/errs/doc.go`
    examples to use `pkg/retry` directly instead of `errs.DefaultRetryConfig()`.
- **Lock-free `math/rand/v2` jitter in `pkg/retry` (`pkg/retry/retry.go`)**:
  - Replace `"math/rand"` and `"sync"` (`randMu sync.Mutex`, `randSource`) in `pkg/retry/retry.go` with
    `"math/rand/v2"` and `rand.Int64N(limit)`.
- **Admission-ledger sync (`docs/admission-ledger.yaml`)**:
  - Update the `pkg/errs` and `pkg/retry` rows (and `internal/harness/semantictest`'s closure note) in
    `docs/admission-ledger.yaml`.

## Capabilities

No normative capability spec requirements change (`skip_specs: true`).

## Impact

- `pkg/errs/errs.go`, `pkg/errs/errs_test.go`, `pkg/errs/doc.go`.
- `pkg/retry/retry.go`, `pkg/retry/retry_test.go`.
- `docs/admission-ledger.yaml`.
- Closes #218; addresses #223.
