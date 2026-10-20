# Tasks: port-composition

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each new
test fails on the base commit. Each stop's task ends only when `task verify` passes on a clean tree at its commit
(#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner. Gate: `task spec:check` and `task docs:check`.

## 2. Port

- [ ] 2.1 `component`: tests first (`TestDeclarationConflictWithMatchesAdmission`; the field readers in
  `registry_boot_admission_test.go` and `registry_declare_test.go` move to `ResourceID()`). Then
  `Declaration.ConflictWith` (design O3), the registry's admission check calling it, and `ExclusiveResources`
  removed. Update the `component` ledger row. Gate: `go test -race ./component/`, with the existing refusal-text tests
  unchanged; `task verify`.
- [ ] 2.2 `composition`: the pin's tests, minus `assert_test.go` and `shipped_configs_test.go`, with helpers adapted,
  plus `TestValidateExclusiveClashMatchesAdmission` and `TestValidateSelfClashIsPortDeclarationError`. Then the
  package as in design "Decision". Add the `repair-before-port` ledger row, a `docs/repository-map.md` row and
  `"composition unit"` in `scripts/cover-check.sh`. Carry ADR-100 with its port note and ledger row. Gate:
  `go test -race ./composition/`, `task ledger:check`, `task cover:check`, `task verify`.
- [ ] 2.3 #16 refusal: `TestValidateRefusesStreamCapturingReservedSubject` and
  `TestReservedSubjectOverlapMatchesEnumeration` first (design "Generated checks"; record the counts). Then the check
  in `Analyze`, the type constant and the overlap function in `internal/flowgraph`. Gate:
  `go test -race ./composition/ ./internal/flowgraph/`, `TestReservedSubjectsDeclaredOnce`, `task verify`.
- [ ] 2.4 Example config (owner Q1): `TestValidateExampleConfigHasNoErrorFindings` first. Then the rewritten
  `config/example_config.json` and `TestLoader_ExampleConfig` updated. Gate: `go test -race ./config/
  ./composition/`, `task verify`.
- [ ] 2.5 `task mutate:check` for M1-M10 (detections) and S1 (survivor recorded) in `notes/2.5.md`. Gate:
  `task verify` passes on a clean tree.
- [ ] 2.6 Cross-agent review stop (Codex) over 2.1-2.5; `reviewed-by:` in the PR body.

## 3. Close

- [ ] 3.1 Archive (`openspec archive port-composition`) as the last content commit, syncing `composition-validation`,
  `graph-transport-boundary` and `component-registration` into `openspec/specs/`. Gate: `task spec:check` and
  `task docs:check`.
