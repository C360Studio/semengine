# Tasks: vocab-export-stage-1-1

Read `design.md`, `proposal.md`, `inventory.md`, and `specs/vocabulary-packs/spec.md`. Run `task test:unit` after each
code task and `task verify` on a clean tree before requesting cross-agent review. Record mutation checks, Rapid seeds,
and fuzz runs in `notes/<task>.md`.

## 1. Claim and design (orchestrator / architect)

- [x] 1.1 Move to worktree `gemini/vocab-export-stage-1-1`, author `inventory.md`, `proposal.md`, `design.md`,
  `tasks.md`, and `specs/vocabulary-packs/spec.md`, and open the claiming draft PR (`Closes #256`, `Addresses #9`).

## 2. Pure `PackSpec` and `ProjectionSpec` validation in `vocabulary` (developer)

- [ ] 2.1 Write failing unit tests (`TestPackSpecValidate_PureNoSideEffects` and terminal/scheme projection checks in
  `vocabulary/pack_test.go` and `vocabulary/projection_test.go`), then implement `(s ProjectionSpec) Validate() error`,
  `ValidateProjectionSpec(spec ProjectionSpec) error`, `(p *PackSpec) Validate() error`, and
  `ValidatePack(pack *PackSpec) error` in `vocabulary/projection.go` and `vocabulary/pack.go` (design D1).
  Checks:
  - `go test -race -count=1 ./vocabulary/...`;
  - `go test -fuzz=FuzzLoadPackJSON -fuzztime=5s ./vocabulary/` (and seed replay);
  - `go test -fuzz=FuzzLoadPackYAML -fuzztime=5s ./vocabulary/` (and seed replay);
  - `task mutate:check` on `vocabulary/projection.go` and `vocabulary/pack.go`.
  Record commands and outputs in `notes/2.1.md`.

## 3. `export.SubjectIRI`, blank nodes, and IRI predicates (`vocabulary/export`)

- [ ] 3.1 Write failing unit tests (`TestSubjectIRI_MatchesDefaultExporter` and
  `TestSerialize_UnfoldedReifiedBlankNodeChain` in `vocabulary/export/export_test.go`), then implement
  `export.SubjectIRI`, `isBlankNode` / `objectBlankNode` handling across N-Triples, Turtle, and JSON-LD, and `isHTTPIRI`
  passthrough in `resolvePredicateIRI` (design D2, D3).
  Checks:
  - `go test -race -count=1 ./vocabulary/export/...`;
  - `task test:unit`;
  - `task test:repeat`;
  - `task mutate:check` on `vocabulary/export/export.go` and `vocabulary/export/object.go`.
  Record commands and outputs in `notes/3.1.md`.

## 4. Update documentation and run full verification (technical writer / developer)

- [ ] 4.1 Update `vocabulary/README.md`, `vocabulary/export/README.md`, and `docs/admission-ledger.yaml` (`vocabulary`
  and `vocabulary/export` rows). Run `task verify` on a clean tree.

## 5. Review and archive

- [ ] 5.1 Cross-agent review of record on the implementation commit.
- [ ] 5.2 Archive `vocab-export-stage-1-1` and sync `openspec/specs/vocabulary-packs/spec.md` as the final content
  commit.
