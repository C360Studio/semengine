# Tasks: fix-724-retry-worker-provider-failures

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first
output in `notes/<task>.md`. One pull request (#733), `Closes #724`, `Closes #732`.

## 1. Design

- [ ] 1.1 Inventory and design: INVENTORY PASS, then pre-owner design review passes; the owner fills `Approved-by:`
  and rules on design questions 1 to 4.
  Hold: OPEN-Q proposal awaits owner approval

## 2. graph/embedding and httpembedder: name the provider's unavailability

- [ ] 2.1 Tests first, shown failing (`notes/2.1.md`):
  - `TestClassifyEmbedErr` gains: `errs.ErrRateLimited` wrapped by `WrapTransient` → `rate_limited`; a
    `WrapTransient` error → `provider_unavailable`; a refused connection wrapped by `WrapTransient` →
    `connection_refused`; a plain error → `embedder_error`.
  - `TestRetryableReason` covers every stored reason, `unknown`, and hop 1's three (none retried).
  - A `ScanFailed` round trip keeps `rate_limited` and `provider_unavailable`.
  - `TestProviderUnavailableIsMarked` (httpembedder): 429 with `Retry-After` matches `errs.ErrRateLimited`, one
    request, returned at once; 500, 502, 503, 504 and a closed server are `errs.IsTransient`; 400, 401, 404, 413,
    422, 501, an untrusted reply, and an `httptest.NewTLSServer` the client does not trust are not (this last is the
    probe of the TLS error at the pinned Go).
  Premises: `worker.go:786-805,827-830`, `storage.go:513-535`, `httpembedder.go:204-219`, `pkg/errs/errs.go:154-171`.
- [ ] 2.2 Implement D1 and D2 (A): two reasons, `RetryableReason`, the classifier order, httpembedder marking, the
  package doc. Accept: 2.1 passes, `task verify`.

## 3. graph-embedding: the retry lane

- [ ] 3.1 Tests first, shown failing (`notes/3.1.md`):
  - `TestRetryTargets` (unit, exact sets, outcomes recorded through the same `failedMap` methods `onTerminal` calls,
    with the outcome type): only retryable failures that are not waiting; one at a time, the next after
    the cursor, wrapping; all after a stored vector for a failed entity; one, not all, after a skipped or deleted
    outcome; seeded failures waiting; back to one on the next pass.
  - `TestRepairRetriesProviderFailures` (integration, invariants): five entities fail while the embedder is down
    (three `provider_unavailable`, one `rate_limited` from a product-style embedder returning `errs.ErrRateLimited`,
    one `dimension_mismatch`); after restore and passes, the four get vectors, the failed map is
    `{dimension_mismatch: 1}`, and the embedder is never called again for that entity.
  - `TestRepairWaitsForRedrivenOutcome`: with the embedder held, further passes make no second call for it.
  - `TestStartSeedsFailedMap`: a pass while the gated store holds the Worker's snapshot retry leaves the seeded entity
    failed `{timeout: 1}`; a re-drive would find it absent and clear it.
  - `TestRepairConvergesStranded`: `gone` fails `dimension_mismatch` and stays failed.
  Premises: `failed.go:44-50,100-121,209-220`, `hop1.go:253-262,281-314`, `component.go:346-350`,
  `repair_integration_test.go:180,237-246`.
- [ ] 3.2 Implement D3 (B) and D4 (A) in `failed.go` and `hop1.go`, with the re-check under `hop1Mu`. Update the
  README sections "Failed entities and the repair lane" (with D5's declared limit in plain words) and "Known limits", the
  `graph/embedding` README paragraph on reasons and `pkg/errs` marking, and `docs/contract.md:34` (tier 1 returns
  without a restart). Accept: 3.1 passes, `task verify`.

## 4. End to end

- [ ] 4.1 `TestLosingTheProviderKeepsWordSearch`: a switchable fake provider (503 while down); `failed_reasons`
  `{provider_unavailable: 1}`; after restore, `semantic` answers B and graph-embedding's status reads ready with no
  failures. Shown failing on main (`notes/4.1.md`). Accept: `task verify`.

## 5. Close

- [ ] 5.1 Spec sync and archive as the last content commit; PR body `Closes #724`, `Closes #732`, `implemented-by:`,
  `reviewed-by:`.
