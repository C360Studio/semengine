# Tasks: security-contract-line

Each task names the outcome and the gate that proves it. Tests come first. Record each new test failing on the base
commit in `notes/<task>.md`, or, for a test of behaviour that already exists, the mutation runs that show it can fail.
Tasks marked (Q1) or (Q3) follow the owner's answer to that question; a different answer rewrites them before work
starts.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and the owner
  answers questions 1 to 3 on #274. Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [ ] 2.1 `TestProductServesTLSFromDependenciesSecurity` in `pkg/security/product_test.go` (D5): a no-certificate
  client is refused, a signed client is served 200, with `client_cert_optional` the no-certificate client is
  served, and `min_version` `"1.4"` makes `Start` fail naming `min_version`. Gate: passes on the base commit;
  `task mutate:check` runs on mutants A and B (D5) are recorded in `notes/2.1.md` as detected, surviving or
  inconclusive.
- [ ] 2.2 (Q1) `TestLoadersRefuseSettingsNotApplied` in `internal/tlsutil`: `mode` `"acme"` and `"amce"`, and
  `acme.enabled` true, are refused with an invalid-class error naming the key, for both loaders; `mode` empty and
  `"manual"` are accepted. Plus the scenario "ACME mode on the metrics server" in `metric/tls_test.go`. Gate: both
  fail on the base commit.

## 3. Implementation

- [ ] 3.1 (Q1) The two loaders refuse what 2.2 names. Gate: 2.2 passes; one `task mutate:check` run deleting the
  `mode` check fails it.
- [ ] 3.2 (Q3) The `schema` tags on `pkg/security`'s 31 fields are removed; the `json` tags are unchanged. Gate:
  `TestConfigDecodesEveryKey` and `TestConfigDocumentCoversEveryField` pass unchanged.
- [ ] 3.3 `pkg/security/doc.go`: the ACME usage example and the `pkg/acme` link go; the package comment names the
  `security-config` spec and `component.Dependencies.Security`. Gate: `task lint`, `task docs:check`.
- [ ] 3.4 `metric/tls_test.go:92`: the comment stops naming `security.Config.Validate`. Gate: `task fmt:check`.

## 4. Docs and ledger

- [ ] 4.1 `docs/admission-ledger.yaml`, row `pkg/security`: `contract` names `openspec/specs/security-config/spec.md`
  beside the type list; `proving_tests` adds `pkg/security/product_test.go`; `known_risks` records the removed
  `schema` tags (Q3). Row `internal/tlsutil`: `contract` adds the refusal (Q1). Gate: `task ledger:check`.
- [ ] 4.2 File the issue owner question 2 asks for (re-judge `internal/tlsutil` under #190), and the pin's ACME
  `renew_before` defect (default 8h against the documented 24h; an unparsable value silently becomes 8h, pin
  `pkg/tlsutil/tlsutil.go:350-354`) for change 5. Gate: both issue numbers in a PR comment, read back.

## 5. Verify and archive

- [ ] 5.1 `task verify` passes on a clean tree at the PR head. Gate: `task verify`.
- [ ] 5.2 Another agent reviews the code PR; the body carries `implemented-by:` and `reviewed-by:`. Gate:
  `task merge:check -- 279`.
- [ ] 5.3 Archive commit: the `security-config` spec is synced into `openspec/specs/`. Gate: `task spec:check`,
  `task docs:check`.
