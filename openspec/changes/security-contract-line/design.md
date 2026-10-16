# Design: security-contract-line

Status: draft. Not for design review until the inventory has `INVENTORY PASS` (architect contract, Required workflow,
step 3). Inventory: `inventory.md` (base `28454d4`).

## Context

Issue #274's "Done when": a contract line offers `pkg/security`'s exported surface to products (a spec requirement, or a
README section the ledger row's `contract` field names); every export is named by it or dropped as dead; a test drives
the offered surface through `component.Dependencies`. Not in scope: what the security capability becomes (epic #84).
Owner intent (#190, 2026-10-12): "if it's an API we are providing for people to use then the only callers might be
tests in our code."

Measured (inventory § 4): 38 exported identifiers, all framework API, none plumbing, none a back door, none dead.
Nothing is dropped or unexported. The ledger row already lists the seven types (inventory § 1), so the new line
extends that one; it does not add a second list.

## D1. Where the contract line lives

| Option | What it is | Cost |
| --- | --- | --- |
| A. Spec requirement (recommended) | `openspec/specs/security-config/spec.md`, three requirements; the ledger `contract` field names it | one new capability; scenarios bind the test by name; `task spec:check` checks its form; it is the `contract:` reason #161's allowlist takes |
| B. `pkg/security/README.md` section | a README the ledger names | a third prose home beside `doc.go` (131 lines) and the ledger; no gate checks it against the test |
| C. Do nothing: the ledger field is the line | `docs/admission-ledger.yaml:396-401` already | fails #274's "Done when" and the owner's 2026-10-16 bar (spec or README); says nothing about `Dependencies` or what is applied |

Recommended A. It mirrors `pkg/errs` (`openspec/specs/error-classification/spec.md`) and keeps one prose home for
"what the types do" that a reviewer can hold code to. `doc.go` keeps the field list and points at the spec.

## D2. What the line offers

All 38 identifiers, by name: the seven types and their JSON keys, as the ledger row already lists them. The spec
states what is true today, in three requirements (spec delta): (1) a factory receives `deps.Security` and the zero
value is TLS off and fail-closed under mTLS; (2) what `metric.NewServer` applies; (3) what nothing applies yet.
`TLSConfig.Client` and its eight fields are offered and applied by no exported call (inventory § 2); requirement 3
says so instead of promising behaviour.

## D3. Settings nothing applies (owner question 1)

`mode` and the ACME blocks have readers at the pin (inventory § 4) and their loaders travel to change 5 (#9 comment
5950752741), so they stay. The question is only what happens meanwhile.

- Option 1 (recommended): `internal/tlsutil.LoadServerTLSConfig` and `LoadClientTLSConfig` refuse `mode` other than
  empty or `manual`, and `acme.enabled: true`, with an invalid-class error naming the key. This mirrors the MinVersion
  refusal already at `internal/tlsutil/tlsutil.go:158-166` and #74's rule, never accept a key and ignore it. Change 5
  replaces the refusal with the loader. Cost: about 15 lines and one table test in `internal/tlsutil`, and one clause
  on its ledger row. Real-world trigger: an operator moving a pin config that sets `mode: "acme"` with old cert files
  is served the files with no error.
- Option 2: declare only. Requirement 3 says the settings are accepted and have no effect. No code. The trigger above
  stays silent.

## D4. Schema tags (owner question 3)

Every `schema` tag on the 31 fields is refused by `component.ParseSchemaTag` (`category:security`; `type:boolean` on
the seven bools), so `component.GenerateConfigSchema` returns no property for any `security` type (probe, inventory
§ 2). Nothing here or at the pin reads them otherwise, and `renew_before`'s `default:24h` disagrees with the pin's 8h.

- Option 1 (recommended): remove the `schema` tags; keep the `json` tags. No behaviour changes. The JSON keys stay held
  by `TestConfigDecodesEveryKey`.
- Option 2: correct them (`type:bool`, a valid category, true defaults), and add a test that a generated schema has a
  property for each field. Cost: 31 tag edits and text to keep true for settings that do nothing yet.
- Option 3: leave them, and file the defect.

## D5. The product-shaped test

`pkg/security/product_test.go`, package `security_test` (external, so it compiles only against exported API).

- A product type implementing `component.Discoverable` (the methods `main` requires; it still satisfies #273's
  narrower interface) and a `var factory component.Factory = func(raw json.RawMessage, deps component.Dependencies)
  (component.Discoverable, error)` whose body builds `metric.NewServer(0, "/metrics", metric.NewMetricsRegistry(),
  deps.Security)`. The test calls the factory directly, as the host does. It calls no `component.Registry` method, so
  it is independent of #273's `Register` change and of the rule that consumers reach `CreateComponent` only through
  `service.ComponentManager`.
- `Dependencies.Security` is decoded, with `DisallowUnknownFields`, from an operator JSON document naming a
  self-signed certificate written to `t.TempDir()`: server TLS on, server mTLS on, `client_ca_files` set,
  `client_cert_optional` absent.
- The server starts with `StartWithListener` on `net.Listen("tcp", "127.0.0.1:0")`; cleanup stops it under a
  bounded context.
- Oracle: `crypto/tls` and `net/http` clients, independent of `internal/tlsutil`, and the expectations in the spec
  scenarios: a client with no certificate is refused; a client presenting a certificate the client CA signed gets
  200; with `client_cert_optional: true` the client with no certificate gets 200; with `min_version` `"1.4"`,
  `Start` returns an error naming `min_version`.
- Shown able to fail: two `task mutate:check` runs recorded in `notes/2.1.md`. Mutant A flips the branch at
  `internal/tlsutil/tlsutil.go:122` (`if mtlsCfg.ClientCertOptional` to `if !mtlsCfg.ClientCertOptional`): the
  no-certificate case must fail. Mutant B makes `metric/handler.go:149` `if false`: the served case must fail. Each is
  reported detected, surviving or inconclusive as it is.
- Generated checks: not used. The inputs are one switch (`client_cert_optional`), one client choice and one
  version string; four examples cover the table.

Already proven, named by the spec, not duplicated: `TestServerRefusesMTLSWithoutTLS` (`metric/tls_test.go:94`),
`TestServerMTLSRequiresClientCertificateByDefault` (`internal/tlsutil/mtls_default_test.go:21`),
`TestLoadServerTLSConfigRefusesUnknownMinVersion` (`internal/tlsutil/minversion_test.go:38`, the loader-level
refusal), and
`TestConfigDecodesEveryKey` (`pkg/security/config_test.go`).

Error class at `Start`: `metric/handler.go:150-153` wraps a loader refusal with `errs.WrapFatal`, so `errs.Classify`
reports fatal there (`pkg/errs/errs.go:223-238`). The spec promises the invalid class only at the loaders, and at
`Start` only an error naming the key.

## D6. Not in this change (owner question 2)

The loaders a product needs to apply `deps.Security` are in `internal/tlsutil` (floor design D5, before #190). At the
pin they had production callers in consumer adapters (`input/websocket`, `output/httppost`), which #190 counts as
framework API evidence. Recommended: file an issue to re-judge `internal/tlsutil` under #190, and keep this change to
`pkg/security`. Boot filling `Dependencies.Security` from configuration is the `config`/`service` port's work.

## Open issues

| Issue | Disposition |
| --- | --- |
| #274 | retired: `Closes #274`; tasks 2.1, 3.1 to 3.4, 4.1 |
| #84 | stays: owner decision on the security capability; optional to this change, which fixes no part of it. This spec may change when #84 is designed |
| #9, #223 | stay: epics; no closing keyword (#274 is outside the wave) |
| #161 | stays: its check is not built; when built, `pkg/security`'s seven types take `contract: security-config` as their allowlist reason |

## Merge order

No file or capability overlap with any open PR if the change keeps out of `component/` (inventory § 3). Either order
with #273 works; if #273 lands first, nothing here changes.

## Size and kind

About 30 non-test lines change (`doc.go` about 15; `internal/tlsutil` about 15 under question 1; tag removal under
question 3 is 31 lines rewritten, no logic), plus about 120 test lines. One package's contract with one cross-package
test, well inside #159's bound. A code PR: another agent's review and `reviewed-by:` are required. The claim's title
prefix `docs(security)` may become `test(security)` or `fix(security)`; `merge-check` reads files, not the title.

## Owner questions

1. **Should SemEngine refuse `mode: acme` and `acme.enabled` until it can do ACME?** Recommended yes (D3 option 1):
   about 15 lines in `internal/tlsutil`; an operator is told at start instead of being served the wrong certificate.
   Option 2 costs nothing now and leaves that case silent.
2. **Should the TLS loaders products need become public?** Recommended not in this change: file an issue to re-judge
   `internal/tlsutil` under #190. Until then a product that reads `deps.Security` rebuilds the fail-closed mTLS rule
   itself. Doing it here roughly doubles the change (a package move, its row, its own contract line).
3. **Should the `schema` tags on `pkg/security` go?** Recommended yes (D4 option 1): SemEngine's own parser refuses
   all 31, so they describe nothing and one is wrong. Correcting them instead costs 31 edits plus a test for text about
   settings that do nothing yet.
