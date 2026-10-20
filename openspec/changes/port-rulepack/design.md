# Design: port-rulepack

Pin `8b99efe9`. Ledger row: `pkg/rulepack` (below). This design depends on `INVENTORY PASS` for `inventory.md`.

## Options

- A. Carry as-is (`carry`, byte-equal). Not available: the owner's 2026-10-19 ruling makes the alphabet and text
  fixes below required, so the package is `adapt` whatever Q1 says. It would also land `ValidateConfig` and
  `ValidateRuntimeUpdate` with no reader, contract line or local test (coverage 19.2%; their tests live in
  unadmitted `frameworkcapabilities/rulepacks`), and keep `config` and `types` (in-tree closure 19 semengine packages
  against 7 for `natsclient`).
- B. Adapt: keep `PackIDCharset`, `MaxPackIDBytes`, `ValidateID`; drop `ValidateConfig`, `ValidateRuntimeUpdate`,
  `packIdentity`, `decodePackID`; one alphabet spelling; true text; a small spec (recommended).
- C. Keep `ValidateConfig` as plumbing for `service`'s rule-pack PR, drop `ValidateRuntimeUpdate`. Cost: no admitted
  pin package calls it, so a later PR would add a caller the pin never had, beside a second duplicate check
  (`service/rule_pack_bind.go:99-101`); the `config` import stays.
- D. Defer the package to the rule core (change 6). Cost: departs from the owner's order; `service`'s rule-pack
  binding (change 3) reads `ValidateID`.

## Decision (B)

- **Dropped as dead surface** (inventory sweep item 4; surface audit, #9 comment 5968830525; #190): `ValidateConfig`
  (only reader unadmitted; no consumer imports the package) and `ValidateRuntimeUpdate` (no production reader at the
  pin; guards hot reload ADR-094 retired), with `packIdentity` and `decodePackID`. The package then imports `fmt`,
  `regexp` and `natsclient`. The surface audit requires the drop; Q1 names it because it departs from 03B E5's
  accepted "carries (no edit)", which the alphabet and text fixes below already change on their own.
- **One alphabet spelling** (owner, 2026-10-19: fix simple leftovers): `ValidateID` refuses an ID that does not match
  a pattern compiled once from `PackIDCharset` (`"^" + PackIDCharset + "+$"`), replacing the loop at pin `:45-55`.
  The validator and the rule processor's schema pattern (pin `processor/rule/schema.go:18`) then share one source,
  which `graph-events/spec.md:135-145` (pin) requires to agree. The `ValidateKVLiteralToken` call stays first and
  wrapped with `%w`: it is ADR-076's "one literal KV token" and gives `.`, wildcards, spaces and non-ASCII their
  reason. Order and every accept/refuse outcome stay as at the pin; only the `/` message loses "offending char".
- **True text** (surface audit (c)): the package comment, the `MaxPackIDBytes` comment and the messages at pin
  `:35`, `:41`, `:51` stop naming the owner id `rule-pack.<pack_id>`. The bound stays 246, cited to the spec below.
  The messages keep the substrings the pin test asserts (`pack_id is required`, `maximum is 246`, `invalid pack_id`,
  `one literal KV token`).
- **Spec** (Q2): new capability `rule-pack-identity`, one requirement carried from pin `graph-events/spec.md:65-66`
  restricted to `ValidateID`: 1–246 bytes matching `^[A-Za-z0-9_=-]+$`, refused without normalization otherwise; a
  non-KV-token value carries natsclient's `kv_token_invalid` code. The duplicate and static-reload clauses stay with
  `service` and `processor/rule`.
- `t.Parallel` stays: the rule bans it only in integration-tagged files (tree precedent
  `natsclient/kv_key_contract_test.go`). Public-package rule: `ValidateID(string) error` names no `internal/` type.
- ADRs: ADR-076 decision 4 binds. Its grammar, bound and one-token clauses are kept unchanged here. Its duplicate
  clause ("Duplicate enabled PackIDs are one semantic producer collision → composition rejection before binding",
  `076-...md:36-37`) leaves this package with `ValidateConfig` and is kept in the port set by `service`'s binding
  (pin `rule_pack_bind.go:99`, before clients are built at `:72-79`). What loosens is pin `graph-events/spec.md:75-77`
  ("before projection validation"); Q1 names it. The record is not copied now: decisions 1–3 and 5 govern
  `processor/rule` event identity and come with the rule core, as ADR-055 does (config design). ADR-094 is kept (it
  is why `ValidateRuntimeUpdate` goes). ADR-056 is superseded; ADR-058 governs `service`. No guidance to carry.
- Context and lifecycle rules do not apply: no context, goroutine or I/O.

## Ruling to implementation

| Ruling | Implementation | Proving test |
| --- | --- | --- |
| #159, one package per PR | `pkg/rulepack/` plus ledger row, map row, spec | review of the file list |
| Surface audit (#9 comment 5968830525, #190); Q1 | `identity.go` without the four symbols | build; `go list -deps ./pkg/rulepack` names no `config`, `types` (task notes) |
| Owner 2026-10-19, simple leftovers | `ValidateID` matches the `PackIDCharset` pattern | `TestValidateIDEveryByte` |
| Surface audit (c) | package comment, `MaxPackIDBytes` comment, three messages | review; `TestValidateID` substrings |
| ADR-076 decision 4 | `MaxPackIDBytes`, `PackIDCharset`, `ValidateID` | `TestValidateID`, `TestValidateIDBounds`, `TestValidateIDKVTokenRefusal` |
| ADR-094 | `ValidateRuntimeUpdate` not ported | build (no caller) |

## Generated checks

Needed by `docs/testing.md` (an input format with a size limit); decided: an exhaustive per-byte table, no random
generator. Every refusal rule is per byte and position-free (`ValidateKVLiteralToken` and the pattern) or depends on
length alone, so placing each of the 256 byte values in `"a"+b+"a"` decides every alphabet case and 246/247 decides
the bound. The oracle is the spec's literal `^[A-Za-z0-9_=-]+$`, written in the test; the test also counts 65
accepted bytes, so its assertions are shown to run.

## Proving tests and mutants

`TestValidateID` (pin, unchanged); new `TestValidateIDEveryByte`, `TestValidateIDBounds` (literals 246 and 247), and
`TestValidateIDKVTokenRefusal` (`pack.one` gives a `*errs.ClassifiedError` with `Code` `kv_token_invalid` via
`errors.As`). Predicted detections: M1 delete the length check (pin `:33-38`), M2 `>` to `>=` `:33`, M3 246 to 247
`:21`, M4 delete the empty check `:30-32`, M5 delete the KV call `:39-44`, M6 `%w` to `%v` `:43`, M7 `PackIDCharset`
to `[A-Za-z0-9_=/-]` (a `/` appended after `-` makes the invalid range `=-/`, so `MustCompile` panics at init and
the run is inconclusive, not a detection), M8 drop `=` from it, M9 delete the pattern check, M10 drop `$` from the
pattern. Predicted survivor:
S1 the `/` message reworded (anything keeping `invalid pack_id`). At the pin, `TestValidateID` alone lets every
`PackIDCharset` edit survive, and loop edits adding a byte outside the KV alphabet are equivalent (unreachable).

## Admission-ledger row

```yaml
- source_path: pkg/rulepack
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the rule-pack identity contract (PackID grammar and byte bound) read by processor/rule (pin config.go:151-177,
    schema.go:13,18) and service's rule-pack binding (pin service/rule_pack_bind.go:96), both admitted (03B E5) and
    ported later; carried ahead of its importers in the owner's order (#9 comment 6270875286)
  destination: pkg/rulepack
  contract: >-
    03B retained-contract-matrix row (design.md:716, "rule-pack IDs validated by one contract package"): the ID
    validation stays here; binding projection contracts at boot is service's (rule_pack_bind.go). ValidateID
    accepts 1-246 bytes matching ^[A-Za-z0-9_=-]+$ and refuses anything else without normalizing; a value that is
    not one literal NATS KV token is refused with natsclient's kv_token_invalid error wrapped; PackIDCharset and
    MaxPackIDBytes are the grammar and bound the rule processor's schema publishes (pin ADR-076 decision 4); stated
    as the rule-pack-identity spec if Q2 is answered yes
  dependencies_and_side_effects: natsclient (ValidateKVLiteralToken), fmt, regexp; no goroutine, no I/O
  known_risks: >-
    dropped as unread in the port set: ValidateConfig (only pin reader frameworkcapabilities/rulepacks, reached from
    internal/boot and internal/bootstrapobservability, none admitted), ValidateRuntimeUpdate (no production reader
    at the pin; ADR-094 retired component hot reload), packIdentity, decodePackID; duplicate enabled pack IDs are
    then refused only by service's rule-pack binding, after its projection preflight (pin rule_pack_bind.go:89-101);
    a disabled rule processor's pack_id is checked only when a later boot enables it; the alphabet loop is
    replaced by a pattern built from PackIDCharset; text naming the retired owner id
    rule-pack.<pack_id> reworded; no importer until service or processor/rule lands
  proving_tests: >-
    pkg/rulepack/identity_test.go (TestValidateID, TestValidateIDEveryByte, TestValidateIDBounds,
    TestValidateIDKVTokenRefusal)
  owner: port-rulepack developer
  disposition: adapt
```

`docs/repository-map.md` gains a `pkg/rulepack/` row. Coverage is recorded on the PR, not gated: `pkg/rulepack` is
not on 03B D10's ruled or extended critical lists (`.../setup-03b-contract-boundary/design.md:454-467`) nor in
`scripts/cover-check.sh`; predicted 100%. No `class:port-refactor` item: a dead-surface drop and text fixes, as for
`health`'s row.

**Issues:** none names the package; #9 stays (`Addresses #9`). **Merge order:** independent of #329 and #332 (no
file overlap); whichever merges second keeps both ledger and map appends. `composition` follows this PR.

## Owner questions

- **Q1. Drop `ValidateConfig` and `ValidateRuntimeUpdate`?** Recommended: yes. Nothing in the port set calls either,
  and the second guards a hot-reload path the pin itself retired (ADR-094). The surface-audit rule requires the drop;
  it is asked because it departs from the 03B design's accepted "carries (no edit)" for this package. What gets
  looser: the pin spec says a duplicate pack ID "MUST fail the complete composition before projection validation"
  (`graph-events/spec.md:75-77`), and ADR-076 decision 4 rejects duplicates "before binding" (`:36-37`). After the
  drop only `service`'s rule-pack binding refuses duplicates: still before binding (`rule_pack_bind.go:99`; clients
  at `:72-79`), but after the projection check (`:89-93`), so a config with both faults reports the projection
  fault. The pin ran `ValidateConfig` first only in its own boot (`internal/boot/run.go:90`), which no consumer uses
  (04A `inventory.md:527`): for every consumer, the pin already behaved this way. `service`'s PR decides whether to
  move its duplicate check ahead. Answer no: decision B still holds, but both functions stay, each with a contract
  line in the ledger row, their tests rewritten here (0% locally today; the pin's live in
  `frameworkcapabilities/rulepacks`), the `config` and `types` imports kept, and the hot-reload messages at pin
  `:102` and `:124` reworded, since they describe a path ADR-094 retired.
- **Q2. Add a small `rule-pack-identity` spec in this PR?** Recommended: yes, though optional: the ledger `contract`
  field is already a valid contract line, since `ValidateID` and the constants are ported and their readers are
  admitted (precedent: `health` got a spec, `internal/logforwarderpolicy` did not). The `pack_id` rules are public
  and no spec in this tree states them. Cost of yes: when the rule core ports the pin's `graph-events` capability,
  the PackID rules would be written in two specs, and that change must fold them into one. Answer no: the ledger
  row is the contract until the rule core.
