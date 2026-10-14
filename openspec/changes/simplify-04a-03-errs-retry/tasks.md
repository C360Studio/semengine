# Tasks: simplify-04a-03-errs-retry

Each task names the outcome and the gate that proves it.

## 1. Design

- [x] 1.1 Clarify scope (`#218` only vs. `#167`), caller inventory, and overlap boundaries with the owner, and scaffold
  `.openspec.yaml`, `proposal.md`, `design.md`, and `tasks.md`. Gate: `task spec:check` and `task docs:check`.

## 2. Implementation and tests

- [x] 2.1 Make `WrapTransient`, `WrapFatal`, and `WrapInvalid` return `nil` whenever `err == nil` in `pkg/errs/errs.go`,
  delete `RetryConfig`, `DefaultRetryConfig()`, `ShouldRetry`, `ToRetryConfig`, `BackoffDelay`, and the `pkg/retry` /
  `time` imports from `pkg/errs/errs.go`, switch `pkg/retry/retry.go` jitter from mutex-guarded `math/rand` to
  `math/rand/v2`, and update `pkg/errs/errs_test.go` and `pkg/retry/retry_test.go` (`TestRetry_JitterBounds`,
  `TestRetry_ConcurrentWithJitter`). Gate: `task test:unit` and `task test:repeat`.
- [x] 2.2 Update `pkg/errs/doc.go` and the `pkg/errs`, `pkg/retry`, and `internal/harness/semantictest` rows in
  `docs/admission-ledger.yaml`. Gate: `task docs:check`, `task ledger:check`, and `task verify`.

## 3. Review and close

- [ ] 3.1 Cross-agent review of record on the implementation commit; record `reviewed-by:` in the PR body once the
  reviewing agent's newest review record approves.
- [ ] 3.2 Archive the change (`openspec archive simplify-04a-03-errs-retry`) as the last content commit. Gate:
  `task spec:check`.
