# Tasks: security-contract-line

Each task names the outcome and the gate that proves it. Tests come first. Record each new test failing on the base
commit in `notes/<task>.md`, or, for a test of behaviour that already exists, the mutation runs that show it can fail.
Tasks marked (Q1) or (Q3) follow the owner's answer to that question; a different answer rewrites them before work
starts.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and the owner
  answers questions 1 to 3 on #274. Gate: `task spec:check` and `task docs:check`.
  Done: review PASS at `323ce3a`; owner accepted all three on #274 (comment 6223764193).

## 2. Failing tests

- [x] 2.1 `TestProductServesTLSFromDependenciesSecurity` in `pkg/security/product_test.go` (D5): a no-certificate
  client is refused, a signed client is served 200, with `client_cert_optional` the no-certificate client is
  served, and `min_version` `"1.4"` makes `Start` fail naming `min_version`; and a first case requires the
  `deps.Security` the factory receives, from the decoded `configDocument`, to equal `wantConfig`. The product type
  implements the `Discoverable` methods `main` has when the PR merges main (design, Merge order).
  `notes/2.1.md` says the arrival case has no mutant: only the decode can change it, and `TestConfigDecodesEveryKey`
  covers that.
  Gate: passes on the base commit;
  `task mutate:check` runs on mutants A and B (D5) are recorded in `notes/2.1.md` as detected, surviving or
  inconclusive.
  Done: passes on `3aab575`; mutants A and B both detection at `9d7f118` (`notes/2.1.md`). After main merged
  (#273), the product type keeps the four `Discoverable` methods (`25f4416`); the CN-allowlist case (Codex
  batch-1 finding) passes and mutant C is a detection at `7750191` (`notes/2.1.md`).
- [x] 2.2 (Q1) `TestLoadersRefuseSettingsNotApplied` in `internal/tlsutil`: `mode` `"acme"` and `"amce"`, and
  `acme.enabled` true, are refused with an invalid-class error naming the key, for both loaders; `mode` empty and
  `"manual"` are accepted. Plus the scenario "ACME mode on the metrics server" in `metric/tls_test.go`. Gate: both
  fail on the base commit.
  Done: both fail on the base code, recorded at `f548aeb` (`notes/2.2.md`).

## 3. Implementation

- [x] 3.1 (Q1) The two loaders refuse what 2.2 names, and the spec delta's "This requirement follows the owner's
  answer" paragraph is removed so it does not sync into the published spec. Gate: 2.2 passes; one
  `task mutate:check` run deleting the `mode` check fails it.
  Done: 2.2 passes and the mutation check is a detection at `52e6c18` (`notes/3.1.md`).
- [x] 3.2 (Q3) The `schema` tags on `pkg/security`'s 31 fields are removed; the `json` tags are unchanged. Gate:
  `TestConfigDecodesEveryKey` and `TestConfigDocumentCoversEveryField` pass unchanged.
  Done: both pass, `config_test.go` unchanged, at `f7c842c`.
- [x] 3.3 `pkg/security/doc.go`: the ACME usage example (`:65-78`) and the `pkg/acme` link (`:130`) go; `:83`,
  `:101-106` and `:120` stop advertising ACME as working and say what the spec's requirement 3 says; `:129` stops
  pointing products at `internal/tlsutil` and names `metric.NewServer` as the call that applies the server block;
  the package comment names the `security-config` spec and `component.Dependencies.Security`. Gate: `task lint`,
  `task docs:check`.
  Done: both exit 0 at `5be39ce` (`notes/3.3.md`).
- [x] 3.4 `metric/tls_test.go:92`: the comment stops naming `security.Config.Validate`. Gate: `task fmt:check`.
  Done: exit 0 at `6544c8f`.

## 4. Docs and ledger

- [x] 4.1 `docs/admission-ledger.yaml`, row `pkg/security`: `contract` names `openspec/specs/security-config/spec.md`
  beside the type list; `proving_tests` adds `pkg/security/product_test.go`; `known_risks` records the removed
  `schema` tags (Q3). Row `internal/tlsutil`: `contract` adds the refusal (Q1). Gate: `task ledger:check`.
  Done: exit 0 at `8edf346`; the `internal/tlsutil` row's `proving_tests` also names `notapplied_test.go`.
- [x] 4.2 File the issue owner question 2 asks for (re-judge `internal/tlsutil` under #190, naming #84 so it does
  not decide how authentication hooks onto the engine's servers); cite the `GenerateConfigSchema` silent-skip issue
  (#281) on the PR without a closing keyword; and file the pin's ACME
  `renew_before` defect (default 8h against the documented 24h; an unparsable value silently becomes 8h, pin
  `pkg/tlsutil/tlsutil.go:350-354`) for change 5. Gate: all three issue numbers in a PR comment, read
  back.
  Done: #284 (tlsutil), #281 (schema skip), #285 (renew_before); PR #279 comment 6225757109, read back.

## 5. Verify and archive

- [x] 5.1 `task verify` passes on a clean tree at the PR head. Gate: `task verify`.
  Done: exit 0, `verify: ok`, at `285fc03`; the tree was clean before and after.
- [ ] 5.2 Another agent reviews the code PR; the body carries `implemented-by:` and `reviewed-by:`. Gate:
  `task merge:check -- 279`.
- [ ] 5.3 Archive commit: the `security-config` spec is synced into `openspec/specs/`. Gate: `task spec:check`,
  `task docs:check`.
