# Design: simplify-04a-03-errs-retry

## Context

Issue #218 is part of Epic #223 (the pre-Change 3 simplification wave). Before claiming work, the owner confirmed two
scoping decisions under the Clarify gate:

1. **Scope**: keep this PR scoped to Issue #218 (`pkg/errs` and `pkg/retry` cleanup: nil-safe `WrapTransient`,
   `WrapFatal`, and `WrapInvalid`; delete duplicate `errs.RetryConfig` and `pkg/errs`'s import of `pkg/retry`; replace
   mutex-guarded `math/rand` jitter in `pkg/retry` with `math/rand/v2`), leaving companion Issue #167
   (`errs.Classify` fallback for unclassified errors and `MutationCommitUnknown` non-retryability) for its own PR under
   Bounded port scope.
2. **Stop boundary**: run autonomously through worktree claim, OpenSpec change, implementation, local `task verify`,
   push, and posting the cross-agent `Review request` comment.

### Overlap inventory

`gh pr list --state open --json number,title,headRefName` shows two open PRs:

- **#230 (`claude/test-infra-smaller-fixture`)**: touches `internal/harness/natsfixture/*`, `natsclient/*_test.go`,
  `pkg/lifecycle/harness_gate_integration_test.go`, `processor/graph-ingest/*_integration_test.go`, and the
  `internal/harness/natsfixture` section of `docs/admission-ledger.yaml`.
- **#235 (`gemini/vocab-export-packs`)**: touches `vocabulary/*` and the `vocabulary` / `vocabulary/export` rows in
  `docs/admission-ledger.yaml`.

Neither open PR overlaps with `pkg/errs/*`, `pkg/retry/*`, or the `pkg/errs` / `pkg/retry` /
`internal/harness/semantictest` rows in `docs/admission-ledger.yaml`. These PRs can merge in any order.

### Open-issue inventory for `pkg/errs` and `pkg/retry`

- **#218 (`port-refactor: pkg/errs & pkg/retry — nil-safe Wrap*, drop duplicate RetryConfig, and math/rand/v2 jitter`)**:
  retired by this PR (`Closes #218`).
- **#167 (`defect: errs.Classify defaults an unclassified error to Transient, so a commit-unknown is spelled retryable`)**:
  stays open for its own PR per owner Clarify selection (`docs/admission-ledger.yaml` `pkg/errs` `known_risks` cites
  `#167`).

### Caller inventory (SemEngine, SemStreams pin `8b99efe9`, and sister repositories)

- **`errs.WrapTransient`, `errs.WrapFatal`, `errs.WrapInvalid` with `err == nil`**:
  - In SemEngine (`main` at `f2cc323`): zero non-test callers pass a `nil` error to `WrapTransient`, `WrapFatal`, or
    `WrapInvalid`.
  - At the SemStreams pin (`8b99efe9`): only unported `graph/inference/applier.go:65-75` called
    `errs.WrapInvalid(nil, "AnomalyOverlayApplier", "ApplyDismissed", "anomaly cannot be nil")`; when `graph/inference`
    is ported, those validation sites will pass `errs.ErrInvalidData` (matching the rest of the codebase).
- **`errs.RetryConfig`, `errs.DefaultRetryConfig()`, `ShouldRetry`, `ToRetryConfig`, `BackoffDelay`**:
  - Zero callers exist outside `pkg/errs/errs_test.go` and `pkg/errs/doc.go` in SemEngine (`main` at `f2cc323`), at the
    SemStreams pin (`8b99efe9`), or across sister repositories (`semsource`, `semconnect`, `semteams`, `semboids`).
    (`natsclient` defines and uses its own `natsclient.RetryConfig` in `natsclient/request.go:491`.)

## Decisions

### D1 Make `WrapTransient`, `WrapFatal`, and `WrapInvalid` unconditionally nil-safe

- **Pin defect**: In `pkg/errs/errs.go:359-402`, `WrapTransient`, `WrapFatal`, and `WrapInvalid` return `nil` only when
  `err == nil && action == ""`, and synthesize `errors.New(action)` when `err == nil && action != ""`. By contrast,
  `Wrap(nil, ...)` (`:349-351`) and `Classified(..., nil)` (`:287-289`) unconditionally return `nil`.
- **Change**: Replace the `if err == nil { if action == "" { return nil } err = errors.New(action) }` block in
  `WrapTransient`, `WrapFatal`, and `WrapInvalid` with `if err == nil { return nil }`, and update their doc comments and
  `pkg/errs/doc.go`.
- **Proving test**: `TestWrapClassified_NilInnerAlwaysReturnsNil` (and `TestWrapErrors` nil checks) in
  `pkg/errs/errs_test.go`.

### D2 Delete duplicate `errs.RetryConfig` and `pkg/errs`'s import of `pkg/retry`

- **Change**: Delete `RetryConfig`, `DefaultRetryConfig()`, `ShouldRetry`, `ToRetryConfig`, and `BackoffDelay` from
  `pkg/errs/errs.go` (`:404-481`), delete `TestRetryConfig_ShouldRetry`,
  `TestRetryConfig_ShouldRetry_WithSpecificErrors`, `TestRetryConfig_BackoffDelay`, and `TestRetryConfig_ToRetryConfig`
  from `pkg/errs/errs_test.go`, update `pkg/errs/doc.go` examples to use `pkg/retry` directly, and drop `"time"` and
  `"github.com/c360studio/semengine/pkg/retry"` from `pkg/errs/errs.go` imports so `pkg/errs` is a pure stdlib leaf
  package.
- **Proving tests**: `pkg/errs` unit tests (`go test -race -cpu=1 ./pkg/errs/...`) and `task build` / `task verify`.

### D3 Switch `pkg/retry` jitter to lock-free `math/rand/v2`

- **Pin defect**: `pkg/retry/retry.go:11-17, 167-175` guards a package-global `math/rand.Rand` with `randMu sync.Mutex`
  on every jittered backoff computation.
- **Change**: Import `"math/rand/v2"`, remove `"math/rand"`, `"sync"`, `randMu`, and `randSource`, and call
  `rand.Int64N(limit)` in `jitter(delay time.Duration)`.
- **Proving tests**: `TestJitter` and `TestConcurrentRetries` in `pkg/retry/retry_test.go` (`go test -race -cpu=1 ./pkg/retry/...`).

### D4 Generated checks

`WrapTransient`, `WrapFatal`, and `WrapInvalid` nil handling is a total two-branch check (`err == nil` vs. non-nil) and
`jitter(delay)` is covered by deterministic range bounds across 100 iterations in `TestJitter` plus concurrent
`synctest` execution in `TestConcurrentRetries`; table-driven unit tests suffice (`docs/testing.md`, "Decide whether
generated checks are needed").
