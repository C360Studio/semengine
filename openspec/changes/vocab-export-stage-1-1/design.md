# Design: vocab-export-stage-1-1

## Context

Issue `#256` delivers the Stage 1.1 follow-up seams in `semengine/vocabulary` and `semengine/vocabulary/export` for
`semvocab` (`C360Studio/semvocab#1` Phase 2 & Phase 3) and `semsparql` (`C360Studio/semsparql#1` §2.2.1, §2.3 & Task
0.2), confirmed by the owner's Clarify decisions on 2026-10-15:

1. Full code PR in `semengine` (`Closes #256`, `Addresses #9`).
2. Enforce all `#256` §1 validation rules in `ProjectionSpec.Validate()` / `ValidateProjectionSpec` and
   `PackSpec.Validate()` / `ValidatePack`, including required terminal `ValuePredicateIRI` and `http://` / `https://`
   projection step IRIs, required non-empty `PackSpec.Name`, and intra-pack duplicate prefix, predicate, and entity-type
   rejection.

Merge order: independent of open PR `#255` (`inventory.md`).
Adopters: `semvocab` (`PackSpec.Validate`, `ValidatePack`, `ProjectionSpec.Validate`, `ValidateProjectionSpec`,
`vocabulary/export`) and `semsparql` (`export.SubjectIRI`, `vocabulary/export` blank-node and full-IRI predicate
serialization).

## Decisions

### D1 Pure `ProjectionSpec` and `PackSpec` validation (`vocabulary/`)

- **Observable behavior**:
  - `(s ProjectionSpec) Validate() error` and `ValidateProjectionSpec(spec ProjectionSpec) error` validate `s` with zero
    global state mutation:
    - `len(s.Steps) > 0`;
    - every step has a non-empty valid `http://` or `https://` `PredicateIRI` (`validateHTTPIRI`);
    - every step with non-empty `NodeClassIRI` has a valid `http://` or `https://` IRI, and every non-terminal step
      (`i < len(s.Steps)-1`) requires a non-empty `NodeClassIRI`;
    - `ValuePredicateIRI` is forbidden on non-terminal steps (`i < len(s.Steps)-1`) and required as a valid `http://` or
      `https://` IRI on the terminal step (`i == len(s.Steps)-1`).
  - `(p *PackSpec) Validate() error` and `ValidatePack(pack *PackSpec) error` validate `p` with zero global state
    mutation (reading neither `predicateRegistry` nor `entityTypeRegistry`):
    - `p != nil` and `strings.TrimSpace(p.Name) != ""`;
    - every `PrefixSpec` passes `PrefixSpec.Validate()` and no two entries in `p.Prefixes` share the same `Prefix`;
    - every `PredicateSpec` has a unique `Name` within `p.Predicates`, satisfies `ParsePredicate(spec.Name)`, rejects
      `spec.AliasPriority != 0` when `!spec.IsAlias && spec.AliasType == ""`, validates `DataType`, `IRI`, `AliasType`,
      `InverseOf` / `Symmetric`, and `Projection.Validate()` (when `spec.Projection != nil`), and satisfies intra-pack
      inverse symmetry;
    - every `EntityTypeSpec` has a unique `DomainType` within `p.EntityTypes`, satisfies `ParseEntityTypeKey`, and has
      valid `ClassIRI` and `Superclasses` IRIs.
  - `Register` (via `validatePredicateMetadataIn`) and `RegisterPack` call `ProjectionSpec.Validate()` and
    `PackSpec.Validate()` so validation rules have one implementation.
- **Proving tests**: `TestPackSpecValidate_PureNoSideEffects`, `TestRegisterWithProjection`,
  `TestRegisterPackAndAtomicOnFailure`, `TestPropPackRoundTrip`, `FuzzLoadPackJSON`, and `FuzzLoadPackYAML`.

### D2 Exported `export.SubjectIRI` helper (`vocabulary/export/export.go`)

- **Observable behavior**:
  - `export.SubjectIRI(subject, base string) string` returns `""` when `subject == ""`. Otherwise it normalizes `base`
    by trimming trailing `/` (`strings.TrimRight(base, "/")`) and defaulting an empty normalized `base` to
    `vocabulary.SemEngineBase`.
  - When `subject` is a valid 6-part canonical entity ID (`message.ParseEntityID`), `SubjectIRI` returns
    `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}`; otherwise it returns
    `{base}/subjects/{dots->slashes}`.
  - `subjectToIRI(subject, base)` delegates directly to `SubjectIRI(subject, base)` so `semsparql` and
    `vocabulary/export` share one implementation.
- **Proving tests**: `TestSubjectIRI_MatchesDefaultExporter`.

### D3 Blank-node (`_:label`) and full-IRI predicate serialization (`vocabulary/export/`)

- **Observable behavior**:
  - `isBlankNode(s string) bool` validates `s` against W3C Turtle / N-Triples `BLANK_NODE_LABEL`: prefix `_:`, followed
    by `[A-Za-z0-9_:]` (or non-ASCII `PN_CHARS_U`) at the first character after `_:`, `[A-Za-z0-9_:.-]` (or `PN_CHARS`)
    in subsequent positions, and not ending in `.`.
  - When `Triple.Subject` satisfies `isBlankNode`:
    - `resolveSubjectIRI` returns `t.Subject` (`_:label`) unchanged without invoking `SubjectIRIFunc` or `SubjectIRI`;
    - N-Triples (`writeNTriples`) and Turtle (`writeTurtleBody` / `formatTurtleTerm`) emit `_:label` directly without
      `<...>` wrapping, and `scanPrefixes` skips prefix compaction on `_:label`;
    - JSON-LD (`buildJSONLDGraph`) emits `"@id": "_:label"`.
  - When `Triple.Object` is a string satisfying `isBlankNode` (with `t.Datatype == ""` or
    `t.Datatype == message.EntityReferenceDatatype`, including when a predicate declares `vocabulary.DataTypeEntityID`):
    - `classifyObject` returns `classifiedObject{kind: objectBlankNode, iri: s}`;
    - N-Triples (`formatNTriplesObject`) and Turtle (`formatTurtleObject`) emit `_:label` without `<...>` wrapping, and
      `scanPrefixes` skips prefix compaction;
    - JSON-LD (`jsonldObjectValue`) emits `map[string]any{"@id": s}`.
  - In `resolvePredicateIRI(predicate, base)`:
    - after checking `vocabulary.GetPredicateMetadata(predicate)` for a registered `StandardIRI`, if `predicate` is
      already a valid `http://` or `https://` IRI (`isHTTPIRI`), `resolvePredicateIRI` returns `predicate` unchanged
      instead of wrapping it under `{base}/predicates/`.
- **Proving tests**: `TestSerialize_UnfoldedReifiedBlankNodeChain` and unit tests for invalid blank-node labels and
  full-IRI predicate compaction across N-Triples, Turtle, and JSON-LD.
