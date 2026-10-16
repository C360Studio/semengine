# Proposal: security-contract-line

## Why

Issue #274, claimed as draft PR #279 on the owner's word (2026-10-16). `pkg/security` is framework API: products reach
`security.Config` through `component.Dependencies.Security` (`component/dependencies.go:51`; owner's amended ruling
on #90, comment 6214503074). No spec requirement or README section offers it, and no test drives it the way a product
would. Nothing says which of its 31 settings any SemEngine code applies: 12 are applied by nothing here (`mode` and
the ACME blocks, whose loaders travel to change 5), and every one of its 31 `schema` tags is refused by SemEngine's
own schema parser, so a schema generated over these types is empty.

## What Changes

- A new capability spec, `security-config`, is the contract line. It offers all seven types and their JSON keys to
  products through `component.Dependencies.Security`, states the fail-closed defaults, and says which settings the
  metrics server applies and which nothing applies yet.
- A product-shaped test, `pkg/security/product_test.go`: a product's `component.Factory` receives a `Dependencies`
  whose `Security` was decoded from an operator's JSON, builds `metric.NewServer` from it, and a real TLS client
  checks what is served.
- Owner question 1 (recommended): `internal/tlsutil` refuses `mode` other than empty or `manual`, and `acme.enabled:
  true`, with an invalid-class error naming the key, until the ACME loaders are ported.
- Owner question 3 (recommended): the `schema` tags on `pkg/security` go; the `json` tags stay.
- `pkg/security/doc.go` loses the ACME usage example and the `pkg/acme` link (#9 comment 5995807082, item 5).
- `metric/tls_test.go:92`'s comment stops naming `security.Config.Validate`, which does not exist.
- Ledger rows `pkg/security` and `internal/tlsutil` name the spec and the new test.

## Capabilities

- Added: `security-config`.

## Impact

- `pkg/security/doc.go`, `config.go` (question 3), new `product_test.go`.
- `internal/tlsutil/tlsutil.go` and a new test (question 1).
- `metric/tls_test.go` (one comment).
- `docs/admission-ledger.yaml`: rows `pkg/security`, `internal/tlsutil`.
- Exported surface: no symbol added, removed or changed. 38 identifiers kept as framework API.
- Not changed: `component/` (no overlap with draft #273). Not decided: anything epic #84 decides.
- A code PR (Go files change): another agent reviews it, and the PR body carries `reviewed-by:`.
- Closes #274.
