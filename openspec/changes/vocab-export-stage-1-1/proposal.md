# Proposal: vocab-export-stage-1-1

## Why

Following Stage 1 (`vocab-export-packs`, PR `#235`), the bootstrap design reviews of `semvocab`
(`C360Studio/semvocab#1` Phase 2 and Phase 3) and `semsparql` (`C360Studio/semsparql#1` §2.2.1, §2.3, and Task 0.2)
identified three small, concrete seams in `semengine/vocabulary` and `semengine/vocabulary/export` (`#256`):

1. **Side-effect-free pack and projection validation (`semvocab#1` Phase 2)**: `semvocab/shacl` compiles SHACL shapes
   into `vocabulary.PackSpec` and `vocabulary.ProjectionSpec` artifacts ahead of time and needs to validate them without
   mutating process-global `vocabulary` or `vocabulary/export` registries. Today, `validateProjectionSpec` is unexported
   and `RegisterPack` validates only as part of mutating the global registry.
2. **Exported `export.SubjectIRI` helper (`semsparql#1` §2.2.1 & Task 0.2)**: `semsparql/irimap` must map 6-part entity
   IDs and non-entity subjects to the exact subject IRIs emitted by `vocabulary/export` without duplicating the
   formatting rules.
3. **Blank-node (`_:label`) and full-IRI predicate serialization in `vocabulary/export` (`semsparql#1` §2.3 &
   `semvocab#1` Phase 3)**: when `semsparql` unfolds `ProjectionSpec` chains (`CONSTRUCT`, `DESCRIBE`, `POST /export`)
   for external W3C SHACL validation, it emits `message.Triple` slices whose `Subject` or `Object` is a deterministic
   W3C blank node (`_:b_...`) and whose `Predicate` is a full `http://` or `https://` IRI (`ProjectionStep.PredicateIRI`,
   `ProjectionStep.ValuePredicateIRI`, or `rdf:type`). Today `vocabulary/export` rewrites blank-node subjects to
   `{base}/subjects/_:b_...`, quotes blank-node objects as string literals, and wraps full-IRI predicates under
   `{base}/predicates/...`.

## What changes

- **Export pure `ProjectionSpec` and `PackSpec` validation in `vocabulary`**:
  - Export `(s ProjectionSpec) Validate() error` and `ValidateProjectionSpec(spec ProjectionSpec) error` in
    `vocabulary/projection.go`, enforcing non-empty `Steps`, valid `http://` / `https://` `PredicateIRI` on every step,
    valid `http://` / `https://` `NodeClassIRI` when non-empty (and required on intermediate steps), and
    `ValuePredicateIRI` forbidden on intermediate steps and required (as a valid `http://` / `https://` IRI) on the
    terminal step.
  - Export `(p *PackSpec) Validate() error` and `ValidatePack(pack *PackSpec) error` in `vocabulary/pack.go`, running
    the complete pre-registration validation (non-nil pack, non-empty trimmed `Name`, `PrefixSpec.Validate()`,
    intra-pack duplicate prefix/predicate/entity-type rejection, 3-part predicate syntax, `DataType` set membership,
    `StandardIRI` / `InverseOf` / intra-pack inverse symmetry validation, `AliasType` / `AliasPriority` rules,
    `Projection.Validate()`, and 2-part `EntityTypeSpec` + `ClassIRI` / `Superclasses` validation) with zero global
    state mutation.
  - Route `WithProjection` / `Register` and `RegisterPack` through these validation functions.
- **Export `export.SubjectIRI(subject, base string) string` in `vocabulary/export`**:
  - Normalize `base` by trimming trailing `/` and defaulting empty `base` to `vocabulary.SemEngineBase`; format valid
    6-part canonical entity IDs as `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}` and other
    non-empty subjects as `{base}/subjects/{dots->slashes}`, and use `SubjectIRI` inside `subjectToIRI`.
- **Support W3C `BLANK_NODE_LABEL` blank nodes and full-IRI predicates in `vocabulary/export`**:
  - Validate blank node identifiers (`_:label`) against W3C Turtle / N-Triples `BLANK_NODE_LABEL`.
  - Emit valid `_:label` subjects directly as `_:label` in N-Triples and Turtle (without `<...>` wrapping, prefix
    compaction, or `SubjectIRIFunc` rewriting) and as `"@id": "_:label"` in JSON-LD.
  - Classify valid `_:label` string objects (when `t.Datatype == ""` or `t.Datatype == message.EntityReferenceDatatype`,
    and when a predicate declares `vocabulary.DataTypeEntityID` or falls back to observation) as blank-node resources,
    emitting `_:label` in N-Triples and Turtle and `{"@id": "_:label"}` in JSON-LD.
  - In `resolvePredicateIRI`, when `Triple.Predicate` has no registered `StandardIRI` in `vocabulary` and is already a
    valid `http://` or `https://` IRI (`isHTTPIRI`), return `Triple.Predicate` unchanged instead of wrapping it under
    `{base}/predicates/`.

## Impact

`vocabulary/projection.go`, `vocabulary/pack.go`, `vocabulary/registry.go`, `vocabulary/README.md`,
`vocabulary/projection_test.go`, `vocabulary/pack_test.go`, `vocabulary/export/export.go`,
`vocabulary/export/object.go`, `vocabulary/export/ntriples.go`, `vocabulary/export/turtle.go`,
`vocabulary/export/README.md`, `vocabulary/export/export_test.go`, `docs/admission-ledger.yaml`, and
`openspec/specs/vocabulary-packs/spec.md`.
Closes #256. Addresses #9.
