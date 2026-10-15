# Tasks: vocab-stage1-1

## 1. Pure `ProjectionSpec` and `PackSpec` validation in `vocabulary`

- [x] 1.1 Write failing unit, property, and fuzz tests in `vocabulary/projection_test.go` and `vocabulary/pack_test.go`
  for `ProjectionSpec.Validate()` (non-empty steps, valid `http://`/`https://` IRIs, required intermediate
  `NodeClassIRI`, forbidden intermediate `ValuePredicateIRI`, required terminal `ValuePredicateIRI`) and
  `PackSpec.Validate()` (`TestPackSpecValidate_PureNoSideEffects` verifying zero global registry reads/writes and no
  caller mutation, nil/empty-name rejection, duplicate prefix/predicate/entity type rejection, intra-pack inverse
  symmetry, and `FuzzValidatePack`).
- [x] 1.2 Implement `(s ProjectionSpec) Validate() error` and `(p *PackSpec) Validate() error` in
  `vocabulary/projection.go` and `vocabulary/pack.go`, and wire `RegisterPack` and `WithProjection` to delegate to them.

## 2. `export.SubjectIRI` and blank-node / full-IRI predicate export in `vocabulary/export`

- [x] 2.1 Write failing unit and fuzz tests in `vocabulary/export/export_test.go` for `export.SubjectIRI`
  (`TestSubjectIRI_MatchesDefaultExporter`, trailing-slash base normalization, empty-base default, and `FuzzSubjectIRI`),
  blank-node subjects and objects (`TestSerialize_UnfoldedReifiedBlankNodeChain` across N-Triples, Turtle, and JSON-LD,
  `@id`-typed blank-node objects, `DataTypeEntityID` blank-node objects, and `TestBlankNodeValidation_MalformedFallback`),
  and unregistered `http://`/`https://` predicate pass-through (`TestResolvePredicateIRI_FullIRIPassThrough`).
- [x] 2.2 Implement `export.SubjectIRI`, `isValidBlankNodeLabel` + `objectBlankNode` handling across
  `vocabulary/export/export.go`, `vocabulary/export/object.go`, `vocabulary/export/ntriples.go`,
  `vocabulary/export/turtle.go`, and `vocabulary/export/jsonld.go`, and full `http://`/`https://` predicate
  pass-through in `resolvePredicateIRI`.

## 3. Documentation, provenance, and verification

- [x] 3.1 Update `vocabulary/doc.go`, `vocabulary/README.md`, `vocabulary/export/doc.go`, `vocabulary/export/README.md`,
  `docs/admission-ledger.yaml`, and developer notes `openspec/changes/vocab-stage1-1/notes/1.1.md`.
- [x] 3.2 Run `task mutate:check` on the new validation guards and run `task verify` on a clean worktree.
