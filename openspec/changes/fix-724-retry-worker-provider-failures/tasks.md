# Tasks: fix-724-retry-worker-provider-failures

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first
output in `notes/<task>.md`. One pull request (#733), `Closes #724`, `Closes #732`.

## 1. Design

- [ ] 1.1 Inventory and design: INVENTORY PASS, then pre-owner design review passes; the owner fills `Approved-by:`
  and rules on design questions 1 to 4.
  Hold: OPEN-Q proposal awaits owner approval

## 2. graph/embedding and httpembedder: name the provider's unavailability

- [ ] 2.1 Tests first, shown failing (`notes/2.1.md`): `TestClassifyEmbedErr` gains rate-limited, provider-unavailable
  and a wrapped refused connection (still `connection_refused`); `TestRetryableReason` covers every stored reason,
  `unknown`, and hop 1's three (none retried); a `ScanFailed` round trip keeps `rate_limited` and
  `provider_unavailable`; `TestProviderUnavailableIsMarked` (httpembedder): 429 with a `Retry-After` is
  `ErrRateLimited`, one request, returned at once; 500/502/503/504 and a closed server are `ErrProviderUnavailable`;
  400/401/404/413/422 and an untrusted reply are neither.
  Premises: `worker.go:786-805`, `storage.go:513-535`, `httpembedder.go:204-219`.
- [ ] 2.2 Implement D1 and D2 (A): the two sentinels, two reasons, `RetryableReason`, classifier order, httpembedder
  wrapping, package doc. Accept: 2.1 passes, `task verify`.

## 3. graph-embedding: the retry lane

- [ ] 3.1 Tests first, shown failing (`notes/3.1.md`): `TestRepairRetriesProviderFailures`: five entities fail while
  the embedder is down (three `provider_unavailable`, one `rate_limited` through a product-style embedder wrapping
  `ErrRateLimited`, one `dimension_mismatch`); a pass while down makes exactly one embedder call, the next pass
  another entity's; after restore, one pass gets a vector for one entity, the next for the other three; the
  `dimension_mismatch` entity is never called again and stays failed; ErrorCount stays 5.
  `TestRepairWaitsForRedrivenOutcome`: with the embedder held, a second pass makes no second call for an entity
  still waiting. `TestRepairConvergesStranded`: `gone` fails `dimension_mismatch` and stays failed.
  Premises: `failed.go:44-50,113-121,209-220`, `hop1.go:253-262,281-314`, `component.go:346-350`.
- [ ] 3.2 Implement D3 (B) and D4 (A) in `failed.go` and `hop1.go`. Update the README sections "Failed entities and
  the repair lane" and "Known limits", plus the `graph/embedding` README paragraph on reasons and the two sentinels.
  Accept: 3.1 passes, `task verify`.

## 4. End to end

- [ ] 4.1 `TestLosingTheProviderKeepsWordSearch`: a switchable fake provider (503 while down); `failed_reasons`
  `{provider_unavailable: 1}`; after restore, `semantic` answers B and graph-embedding's status reads ready with no
  failures. Shown failing on main (`notes/4.1.md`). Accept: `task verify`.

## 5. Close

- [ ] 5.1 Spec sync and archive as the last content commit; PR body `Closes #724`, `Closes #732`, `implemented-by:`,
  `reviewed-by:`.
