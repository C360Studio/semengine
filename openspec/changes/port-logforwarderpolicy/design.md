# Design: port-logforwarderpolicy

Pin `8b99efe9`. Ledger row: `internal/logforwarderpolicy` (below).

## Options

- A. Carry as-is (recommended). Both files byte-identical, so `task ledger:check` holds `policy.go` to the pin. Cost:
  the five survivors below stay uncovered and are recorded on the PR.
- B. Adapt, tests only: add table rows for the survivors. Cost: an `adapt` row is printed by `ledger:diff` and never
  failed, so the automatic pin-fidelity check on `policy.go` is lost for a 105-line file.
- C. Adapt and fix the optional findings (one level switch, drop the `flow-service.websocket` default, fix the doc
  comment). This changes a pin spec requirement (`openspec/specs/service-composition/spec.md:167-168` at the pin)
  that belongs to `service`'s PR; doing it here pre-empts that decision.
- D. Do nothing yet: exclude the package until `service` decides owner question Q1 (the `model/wire` precedent).
  Cost: departs from the owner's order.

## Decision (A)

- Carry `internal/logforwarderpolicy/policy.go` and `policy_test.go` unchanged. No module path appears in either file;
  testify is already in `go.mod`.
- Ahead of its importer: allowed by the architect contract, "One package per pull request" (carried in the tree, it
  builds, keeps its ledger row and tests, and waits for its importer). Admission is the 03B design `:229` and the
  foundation design's D2 row 3. The caller to come is `service/log_forwarder.go` (pin `:17`, `:58`, `:72`).
- Shape findings, all optional under `.agents/protocol.md` "Bounded port scope" (no binding requirement, gate or
  ruling names them): the two level spellings, the copied strict decode, the ignored `ValidateFields` parameter and
  the doc comment naming boot composition. Retained; the cost is recorded here.
- The `flow-service.websocket` default is dead behaviour. It is carried because the pin spec requires it and its home
  is `service`'s log-forwarder PR; filed as an issue, not fixed here.
- No spec capability: the package has no observable surface outside the module until `service` lands. The contract
  line is the ledger row's `contract` field; the requirement belongs in `service`'s PR (the pin keeps it in
  `service-composition`). Examples suffice over generated checks: the code is carried with its tests unchanged, and
  the grammar is four words plus an order-preserving dedup.
- ADRs: none binds; nothing is changed or dropped.
- Context and lifecycle rules do not apply: no context, goroutine or I/O.

## Proving tests and mutants (task 2.2)

The oracle is the literal `slog` constants and string slices in the pin's table. Predicted detections: M1 delete
`policy.go:87` (the mandatory append), M2 delete `:33` (`DisallowUnknownFields`), M3 drop `ToUpper` `:49`, M4 delete
the `seen` check `:80-82`, M5 `:47` "INFO" to "WARN", M6 add "TRACE" at `:65`, M7 `:97` `LevelDebug` to `LevelInfo`.
Predicted survivors, recorded as uncovered cases: S1 delete `:37-43` (second JSON value), S2 delete `:77-79` (empty
source), S3 drop `TrimSpace` `:76`, S4 `:31` `len(bytes.TrimSpace(raw))` to `len(raw)`, S5 drop `TrimSpace` `:49`.
Task 2.2 runs them and reports survivors as survivors.

## Admission-ledger row

```yaml
- source_path: internal/logforwarderpolicy
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the log-forwarder service's inner configuration policy; its reader is service/log_forwarder.go
    (pin :17, :58, :72), ported later in change 3 (setup-04a-03-boot D2 row 3); carried ahead of its
    importer in the owner's order (#9 comment 6270875286; architect contract, One package per pull request)
  destination: internal/logforwarderpolicy
  contract: >-
    as at the pin; every .go file matches it (task ledger:check). Resolve decodes one JSON object strictly
    (unknown keys and a second value refused; empty, whitespace or null input gives the defaults), defaults
    min_level to INFO, trims and upper-cases it, accepts DEBUG, INFO, WARN or ERROR, and returns ExcludeSources
    with flow-service.websocket first, then the configured sources trimmed, empties dropped and duplicates
    removed in first-seen order; ValidateFields accepts exactly the four upper-case levels and ignores its
    second argument
  dependencies_and_side_effects: stdlib only (bytes, encoding/json, fmt, io, log/slog, strings); no goroutine, no I/O
  known_risks: >-
    no importer until service's log-forwarder lands; the policy's behaviour readers at the pin
    (internal/bootstrapobservability, pkg/logging) are not in the port set; the mandatory exclusion names a
    source of the retired flow-service; the second-value and empty-source branches are untested at the pin
  proving_tests: >-
    internal/logforwarderpolicy/policy_test.go (TestResolveOwnsDefaultsNormalizationValidationAndSafetyExclusion,
    TestValidateFieldsPreservesPublicConfigValidationSemantics)
  owner: port-logforwarderpolicy developer
  disposition: carry
```

## Issues to file (`status:blocked`, linked from #319 without a closing keyword)

1. log-forwarder: in the port set the service validates its policy but nothing forwards logs (`pkg/logging` and
   `internal/bootstrapobservability` are not ported). Needs an owner ruling before `service`'s log-forwarder design
   (Q1).
2. logforwarderpolicy: the mandatory exclusion names the retired flow-service source (`policy.go:14`); decide it with
   the `service-composition` requirement in `service`'s log-forwarder PR.

## Merge order

The health port (#318) merges first, per the owner's order. This PR then merges `origin/main` and resolves the ledger append.

## Owner question

Q1. At the pin, logs reach NATS only through `pkg/logging.NATSLogHandler`, built in `internal/bootstrapobservability`.
Neither is in the port set. So once `service` lands, SemEngine's `log-forwarder` would accept and validate
`min_level` and `exclude_sources` and forward nothing. Which way should `service`'s log-forwarder PR go: (a) admit
`pkg/logging` and the boot wiring; (b) port `LogForwarder` and say it forwards nothing; or (c) exclude `LogForwarder`
and this package (the `internal/componentadmission` precedent)? This PR proceeds under the existing admission
whichever is chosen; under (c), deleting its 159 lines undoes it.
