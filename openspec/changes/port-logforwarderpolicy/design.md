# Design: port-logforwarderpolicy

Pin `8b99efe9`. Ledger row: `internal/logforwarderpolicy` (below).

## Options

- A. Carry as-is (recommended). Both files byte-identical, so `task ledger:check` holds `policy.go` to the pin. Cost:
  the five survivors below stay uncovered and are recorded on the PR.
- B. Carry both files and add one test file written here for the survivors. The ledger then takes two file rows
  (`policy.go` and `policy_test.go`, both carry; `internal/harness/pindiff/compare.go:57-125` compares a file row to
  a file) plus the new test, so `task ledger:check` still holds `policy.go` to the pin. Cost: two rows instead of one
  and a test file for branches nothing reads until `service` lands.
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
  is `service`'s log-forwarder PR; raised with the owner in Q1, not fixed here.
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
Predicted survivors, recorded as uncovered cases: S1 `:38` `err != io.EOF` to `err != nil && err != io.EOF` (a
second JSON value accepted; deleting `:37-43` does not build, `io` unused), S2 delete `:77-79` (empty
source), S3 drop `TrimSpace` `:76`, S4 `:31` `len(bytes.TrimSpace(raw))` to `len(raw)`, S5 drop `TrimSpace` `:49`.
Task 2.2 runs them and reports survivors as survivors.

## Admission-ledger row

```yaml
- source_path: internal/logforwarderpolicy
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the log-forwarder service's inner configuration policy; its reader is service/log_forwarder.go
    (pin :17, :58, :72), ported later in change 3 (setup-04a-foundation design D2 row 3); carried ahead of its
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
    source of the retired flow-service; five behaviours are untested at the pin: a second JSON value,
    empty sources dropped, sources trimmed, whitespace-only input, the level trimmed (S1-S5, notes/2.2.md)
  proving_tests: >-
    internal/logforwarderpolicy/policy_test.go (TestResolveOwnsDefaultsNormalizationValidationAndSafetyExclusion,
    TestValidateFieldsPreservesPublicConfigValidationSemantics)
  owner: port-logforwarderpolicy developer
  disposition: carry
```

## Findings not filed

Neither is a defect in this repository's code under `.agents/protocol.md` "File": the code meets its ledger contract.
Both go to the owner as Q1: the log-forwarder forwarding nothing in the port set, and the mandatory exclusion naming
the retired flow-service source (`policy.go:14`, pin `service-composition` spec `:167-168`).

## Merge order

Planned: the health port (#318) first. Done: the owner asked for this PR's merge on 2026-10-19 while #318 was still
a draft, so this PR merged first and #318 resolves the ledger append when it merges `origin/main`.

## Owner question

Q1. At the pin, logs reach NATS only through `pkg/logging.NATSLogHandler`, built in `internal/bootstrapobservability`.
Neither is in the port set. So once `service` lands, SemEngine's `log-forwarder` would accept and validate
`min_level` and `exclude_sources` and forward nothing. Which way should `service`'s log-forwarder PR go: (a) admit
`pkg/logging` and the boot wiring; (b) port `LogForwarder` and say it forwards nothing; or (c) exclude `LogForwarder`
and this package (the `internal/componentadmission` precedent)? The same PR decides whether the mandatory
`flow-service.websocket` exclusion stays. (b) and (c) touch ADR-058's service example list
(`058-boot-lifecycle-phases.md:56`) and ADR-098 decision 3 (logs carry substrate observability). Answers (a) and (b)
keep Decision A. Answer (c) switches this PR to option D before task 2.1: a `defer-exclude` ledger row, as
`model/wire` has, and no code lands.

Owner answer, 2026-10-19: design accepted (Decision A); Q1 answered (b): `service`'s log-forwarder PR ports
`LogForwarder` and says plainly that it forwards nothing yet.
