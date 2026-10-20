# Tasks: port-composition

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each new
test fails on the base commit. Each stop's task ends only when `task verify` passes on a clean tree at its commit
(#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner. Gate: `task spec:check` and `task docs:check`. Done: `INVENTORY PASS` at `336db07`, `DESIGN REVIEW PASS` at
  `d6ee74d` (premise citation fixed at `7af29f9`); owner rulings #9 comments 6295357983 and 6295807962 (Q1 yes, Q2 yes).

## 2. Port

- [x] 2.1 `component`, tests first:
  - `TestDeclarationConflictWithMatchesAdmission`, including the case with two claims on each side;
  - `TestDeclarationConflictWithAdmitsNonClashingClaims`;
  - the field readers in `registry_boot_admission_test.go` and `registry_declare_test.go` moved to `ResourceID()`.

  Then add `Declaration.ConflictWith` (design O3), make the registry's admission check call it, remove
  `ExclusiveResources`, and update the `component` ledger row. Gate: `go test -race ./component/`, with the existing
  refusal-text tests unchanged; `task verify`. Done at `f9323f8`: `task verify` passed on a clean tree there;
  failing-first record and choices in `notes/2.1.md`.
- [x] 2.2 `composition`, tests first:
  - the pin's tests, minus `assert_test.go` and `shipped_configs_test.go`, with helpers adapted;
  - `TestValidateExclusiveClashMatchesAdmission` and `TestValidateSelfClashIsPortDeclarationError`.

  Then the package as in design "Decision", with the `repair-before-port` ledger row, a `docs/repository-map.md` row,
  `"composition unit"` in `scripts/cover-check.sh`, and ADR-100 carried with its port note and ledger row. Gate:
  `go test -race ./composition/`, `task ledger:check`, `task cover:check`, `task verify`. Done at `1b7406f`:
  `task verify` passed on a clean tree there (`composition` 90.3%, unit profile); failing-first record and choices in
  `notes/2.2.md`.
- [x] 2.2a Codex round 1 (design "Codex round 1"; #346, #347, #348). First merge origin/main into the branch (#341).
  Tests first, each failing on the base:
  - `TestValidateKeepsSameNamedLanesApart` (Codex's #346 probe as the oracle, with the renamed-output control) and
    `TestValidateReportsOrphanBesideSameNamedOutput`;
  - `TestValidateReadsNoTLSFile`, `TestValidateDocumentReadsNoFile` and `TestConfigValidateRefusesMissingTLSFiles`;
  - `TestValidateConfigInvalidIsDeterministic`, `TestConfigValidateNamesFirstInvalidComponentByName` and
    `TestRegistryValidateNamesFirstInvalidEndpointByName`.

  Then:
  - `ComponentPortRef.Direction`, used by the contract map and the orphan check;
  - `Config.ValidateDocument`, with `Validate` composed from it and the host checks;
  - composition calling `ValidateDocument`;
  - sorted loops in `Config.ValidateDocument` and `model.Registry.Validate`;
  - the `config`, `model` (now `adapt`) and `component/flowgraph` ledger rows updated.

  After the fix commit, the PR body carries `Closes #346`, `Closes #347` and `Closes #348`, and `status:blocked` is
  removed from those three issues.

  Gate: `go test -race ./config/ ./model/ ./internal/flowgraph/ ./composition/`, `task verify`.
- [x] 2.3 #16 refusal, tests first: `TestValidateRefusesStreamCapturingReservedSubject`,
  `TestValidateDoesNotRefuseNeighbouringSubjects` and `TestReservedSubjectOverlapMatchesEnumeration` (design
  "Generated checks"; record the counts). Then:
  - `SubjectsOverlap` in `internal/flowgraph`, with its ledger contract line;
  - the check in `Analyze`, naming derived streams with `config.DeriveStreamName`;
  - the type constant, with `TestValidateFindingsVocabularyIsClosed` updated to the fourteen types.

  Gate: `go test -race ./composition/ ./internal/flowgraph/`, `TestReservedSubjectsDeclaredOnce`, `task verify`.
- [ ] 2.4 Example config (owner Q1), tests first: `TestValidateExampleConfigHasNoErrorFindings`. Then the rewritten
  `config/example_config.json`, with `TestLoader_ExampleConfig` updated. Gate:
  `go test -race ./config/ ./composition/`, `task verify`.
- [ ] 2.5 `task mutate:check` for M1-M18 (detections; for M16 and M17 record the run count) and S1 (survivor
  recorded) in `notes/2.5.md`. Gate: `task verify` passes on a clean tree.
- [ ] 2.6 Cross-agent review stop (Codex) over 2.2a-2.5; `reviewed-by:` in the PR body.

## 3. Close

- [ ] 3.1 Archive (`openspec archive port-composition`) as the last content commit. It syncs
  `composition-validation` (new; give it its Purpose section at archive, as `rule-pack-identity` got one),
  `graph-transport-boundary`, `component-registration` and `config-loading` into `openspec/specs/`. Gate:
  `task spec:check` and `task docs:check`.
