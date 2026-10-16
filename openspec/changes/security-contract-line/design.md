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

All 38 identifiers: the seven types by name, and their JSON keys as the key document `configDocument` in
`pkg/security/config_test.go` spells them (`TestConfigDecodesEveryKey` holds it; the ledger row says only "as at the
pin"). The spec states what is true today, in three requirements (spec delta): (1) a factory receives
`deps.Security` unchanged, SemEngine fills that field nowhere yet, and the zero value is TLS off and fail-closed
under mTLS; (2) what `metric.NewServer` applies, with server TLS enabled; (3, Q1) what nothing applies yet.
`TLSConfig.Client`, its four fields and its three mTLS fields (8 identifiers) are offered and applied by no exported
call (inventory § 2); requirement 3 says so instead of promising behaviour.

With server TLS disabled, `metric.NewServer` reads no other server setting except the mTLS switch, which it refuses
(`metric/handler.go:103`, `:149`; `internal/tlsutil/tlsutil.go:23-25` returns before `parseTLSVersion` at `:27`).
A `min_version` or certificate path under a disabled server bounds nothing that is served, so requirement 2 scopes
its clauses to an enabled server rather than refusing them.

## D3. Settings nothing applies (owner question 1)

`mode` and the ACME blocks have readers at the pin (inventory § 4) and their loaders travel to change 5 (#9 comment
5950752741), so they stay. The question is only what happens meanwhile.

- Option 1 (recommended): `internal/tlsutil.LoadServerTLSConfig` and `LoadClientTLSConfig` refuse `mode` other than
  empty or `manual`, and `acme.enabled: true`, with an invalid-class error naming the key. This mirrors the MinVersion
  refusal already at `internal/tlsutil/tlsutil.go:158-166`, and meets the surface audit's rule that "a config field
  that stays has a test that fails when the field is ignored" (`.agents/contracts/semengine-architect.md:72-75`):
  the refusal test fails if `mode` or `acme.enabled` is ignored. The seven other `ACMEConfig` fields act only under
  `acme.enabled` (pin `pkg/tlsutil/tlsutil.go:197`), so refusing it covers them. Change 5 replaces the refusal with
  the loader. Cost: about 15 lines and one table test in `internal/tlsutil`, and one clause on its ledger row.
  Real-world trigger: an operator moving a pin config that sets `mode: "acme"` with old cert files is served the
  files with no error.
- Option 2: declare only. Requirement 3 is rewritten to say the settings are accepted and have no effect. No code.
  The trigger above stays silent, and 12 fields stay with no test that fails when they are ignored, against the
  surface audit rule above; the design would have to state that exception for the owner.

Requirement 3 in the spec delta is written as option 1 and marked (Q1), as tasks 2.2 and 3.1 are.

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

- A product type implementing `component.Discoverable` with exactly the methods `main` requires when this change
  is written (see Merge order), and a
  `var factory component.Factory = func(raw json.RawMessage, deps component.Dependencies)
  (component.Discoverable, error)` whose body builds `metric.NewServer(0, "/metrics", metric.NewMetricsRegistry(),
  deps.Security)`. The test calls the factory directly, as the host does. It calls no `component.Registry` method, so
  it is independent of #273's `Register` change and of the rule that consumers reach `CreateComponent` only through
  `service.ComponentManager`.
- Arrival of all 31 fields: a first case calls the factory with `Dependencies{Security: <configDocument decoded>}`
  (the key document `config_test.go` already holds, same `security_test` package), and the factory records the
  `deps.Security` it received; the test requires it to equal `wantConfig`, the hand-written value in
  `config_test.go`. This is the product-shaped proof for the 20 fields the server cases do not exercise; their
  decoding stays proven by `TestConfigDecodesEveryKey`, and what they do (nothing yet, or refused under Q1) by
  requirement 3's tests.
- For the server cases, `Dependencies.Security` is decoded, with `DisallowUnknownFields`, from an operator JSON
  document naming a
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

Already proven, named by the spec, not duplicated: `TestServerRequiredMTLSRejectsClientWithoutCertificate`
(`metric/tls_test.go:60`, the no-certificate refusal called on `metric.NewServer` directly; the new test adds the
`Dependencies` hop and the operator JSON), `TestServerRefusesMTLSWithoutTLS` (`metric/tls_test.go:94`),
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
`pkg/security`. The issue names #84, whose scope includes "an authenticate/authorize hook for the HTTP servers the
engine runs (for example, `metric.NewServer`)", so making the loaders public does not decide the shape of
authentication. Boot filling `Dependencies.Security` from configuration is the `config`/`service` port's work.

## Open issues

| Issue | Disposition |
| --- | --- |
| #274 | retired: `Closes #274`; tasks 2.1, 3.1 to 3.4, 4.1 |
| new: `GenerateConfigSchema` drops a field whose tag it refuses (`component/schema_tags.go:404-409`) | stays: a defect in `component`, outside this change; filed as #281. Question 3 removes the symptom only in `pkg/security`; `natsclient/storage_pressure.go:82`, `:86`, `:91` (`type:number`) have the same symptom |
| #84 | stays: owner decision on the security capability; optional to this change, which fixes no part of it. This spec may change when #84 is designed |
| #9, #223 | stay: epics; no closing keyword (#274 is outside the wave) |
| #161 | stays: its check is not built; when built, `pkg/security`'s seven types take `contract: security-config` as their allowlist reason |

## Merge order

PR #273 first (inventory § 3). It is further along, and it changes `component.Discoverable`: it deletes
`type Metadata` and removes `Meta()` and `ConfigSchema()` (`component/discovery.go` at `bbb0c44`). This PR merges
`origin/main` after #273 lands, and its developer writes the product type with the four methods left (`InputPorts`,
`OutputPorts`, `Health`, `DataFlow`). If this PR is ready first and lands first, its product type carries `Meta()`
and `ConfigSchema()`, and #273 deletes those two methods from `pkg/security/product_test.go` when it merges main.
Both PRs edit `docs/admission-ledger.yaml`, at `:390` and `:675` here and `:1872` and later in #273, so no textual
conflict is expected; the second to land re-runs `task ledger:check`.

## Size and kind

About 30 non-test lines change (`doc.go` about 15; `internal/tlsutil` about 15 under question 1; tag removal under
question 3 is 31 lines rewritten, no logic), plus about 120 test lines. One package's contract with one cross-package
test, well inside #159's bound. A code PR: another agent's review and `reviewed-by:` are required. The claim's title
prefix `docs(security)` may become `test(security)` or `fix(security)`; `merge-check` reads files, not the title.

## Owner questions

1. **Should SemEngine refuse `mode: acme` and `acme.enabled` until it can do ACME?** Recommended yes (D3 option 1):
   about 15 lines and a test in `internal/tlsutil`; an operator is told at start instead of being served the wrong
   certificate. Option 2 costs no code, but leaves that case silent and keeps 12 settings with no test that fails when
   they are ignored, which the surface audit rule forbids unless you waive it.
2. **Should the TLS loaders products need become public?** Recommended not in this change: file an issue to re-judge
   `internal/tlsutil` under #190, naming #84 so the re-judgment does not decide how authentication hooks onto the
   engine's servers. Until then a product that reads `deps.Security` rebuilds the fail-closed mTLS rule
   itself. Doing it here roughly doubles the change (a package move, its row, its own contract line).
3. **Should the `schema` tags on `pkg/security` go?** Recommended yes (D4 option 1): SemEngine's own parser refuses
   all 31, so they describe nothing and one is wrong. Correcting them instead costs 31 edits plus a test for text about
   settings that do nothing yet.
