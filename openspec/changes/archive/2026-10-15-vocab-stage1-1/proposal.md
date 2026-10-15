# Proposal: vocab-stage1-1

## Why

Stage 1 (`#235`) delivered `vocabulary/export`, `LookupByIRI`, `RegisterEntityType`, `ProjectionSpec`, and declarative
JSON/YAML `PackSpec` loading. During the sister-repo architecture reviews for `C360Studio/semvocab#1` and
`C360Studio/semsparql#1`, three upstream Stage 1.1 seams were identified in `C360Studio/semengine#256`:

1. **Pure validation seam on `ProjectionSpec` and `PackSpec` (`semvocab#1`)**: `semvocab/shacl` and pack authors need
   to validate a `PackSpec` or `ProjectionSpec` in unit tests and SHACL compilation without mutating global
   `vocabulary` or `vocabulary/export` registries. Today, `validateProjectionSpec` and `stagePackLocked` are unexported
   and `RegisterPack` couples validation to `registryMu` and registry mutation. Moreover, `validateProjectionSpec` does
   not currently enforce `http://`/`https://` scheme syntax or require `ValuePredicateIRI` on the terminal step, and
   `stagePackLocked` does not reject an empty pack `Name` or intra-pack duplicate predicate/entity-type keys.
2. **Exported `export.SubjectIRI(subject, base string) string` (`semsparql#1`)**: `semsparql/irimap` must mint canonical
   subject IRIs identical to `vocabulary/export`'s internal `subjectToIRI` without duplicating the
   `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}` formatting logic.
3. **Blank-node (`_:label`) and full-IRI predicate pass-through in `vocabulary/export` (`semsparql#1`)**: When
   `semsparql` unfolds reified BFO/CCO `ProjectionSpec` chains for `CONSTRUCT` / `DESCRIBE` queries, the unfolded
   `[]message.Triple` slice contains deterministic blank-node subjects and objects (`_:b_...`) and full ontology
   predicate IRIs (`http://...`, `https://...`) that are not registered as top-level 3-part dotted predicates. Today,
   `vocabulary/export` rewrites `_:label` subjects to `<.../subjects/_:label>`, serializes `_:label` objects as string
   literals `"_:label"`, and rewrites unregistered full-IRI predicates to `<.../predicates/http://...>`.

## What changes

- **Export pure `ProjectionSpec.Validate()` and `PackSpec.Validate()` in `vocabulary`**:
  - `(s ProjectionSpec) Validate() error` validates a projection specification without touching any global state:
    non-empty `Steps`, valid `http://` or `https://` IRI on every step's `PredicateIRI`, valid `http://` or `https://`
    `NodeClassIRI` when set (and required on non-terminal steps in multi-step chains), and `ValuePredicateIRI`
    forbidden on non-terminal steps and required (as a valid `http://` or `https://` IRI) on the terminal step.
  - `(p *PackSpec) Validate() error` validates an entire `PackSpec` with zero global registry reads or writes:
    non-nil pack with non-empty trimmed `Name`, valid `PrefixSpec` entries with no intra-pack duplicate prefix labels,
    valid `PredicateSpec` entries (3-part name, valid datatype, valid IRI when set, alias metadata consistency,
    projection validation via `ProjectionSpec.Validate()`, no intra-pack duplicate predicate names, and intra-pack
    `InverseOf` consistency), and valid `EntityTypeSpec` entries (2-part `domain.type` key, valid class/superclass
    IRIs, and no intra-pack duplicate `domain_type` keys).
  - `RegisterPack` calls `pack.Validate()` before acquiring `registryMu` and staging into the global registries.
- **Export `export.SubjectIRI(subject, base string) string` in `vocabulary/export`**:
  - Converts a 6-part entity ID to `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}` (`ADR-102`)
    and a non-entity subject to `{base}/subjects/{dots->slashes}`, trimming trailing `/` on `base` and defaulting an
    empty `base` to `vocabulary.SemEngineBase`. Returns `""` when `subject == ""`.
- **Support blank-node (`_:label`) subjects/objects and full-IRI (`http://`, `https://`) predicates in
  `vocabulary/export`**:
  - Blank-node subjects and objects matching W3C Turtle / N-Triples `BLANK_NODE_LABEL` (`_:` followed by a valid
    `PN_LOCAL` body without `:`) are emitted as blank nodes (`_:label` in N-Triples and Turtle without `<...>` or
    prefix compaction; `{"@id": "_:label"}` in JSON-LD) when `Triple.Datatype` is empty or
    `message.EntityReferenceDatatype` (`"@id"`). Malformed `"_:"` strings are not emitted as raw blank-node tokens.
  - Unregistered `Triple.Predicate` values that are already valid `http://` or `https://` IRIs pass through unchanged
    in `resolvePredicateIRI` after checking `vocabulary.GetPredicateMetadata(predicate)`.

## Impact

`vocabulary/projection.go`, `vocabulary/pack.go`, `vocabulary/projection_test.go`, `vocabulary/pack_test.go`,
`vocabulary/doc.go`, `vocabulary/README.md`, `vocabulary/export/export.go`, `vocabulary/export/object.go`,
`vocabulary/export/ntriples.go`, `vocabulary/export/turtle.go`, `vocabulary/export/export_test.go`,
`vocabulary/export/doc.go`, `vocabulary/export/README.md`, `docs/admission-ledger.yaml`, and
`openspec/specs/vocabulary-packs/spec.md`.
Closes #256. Addresses #9.
