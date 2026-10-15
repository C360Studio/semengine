# Design: vocab-stage1-1

## Context

Issue `#256` resolves the Stage 1.1 follow-up seams in `vocabulary` and `vocabulary/export` discovered during the
architecture reviews of `C360Studio/semvocab#1` and `C360Studio/semsparql#1`.

Merge order: independent of open PR `#255` (`claude/simplify-errs-classify`), which touches `natsclient/`, `pkg/errs/`,
`pkg/lifecycle/`, `pkg/projection/`, `processor/graph-ingest/commit_ambiguity_integration_test.go`, and
`openspec/changes/simplify-04a-04-errs-classify/` with zero file overlap (`inventory.md`).

Governing ADRs (`inventory.md` Category 6):

- `ADR-102` (`docs/adr/102-entity-id-segment-semantics.md`): kept unchanged. Canonical 6-part entity IDs
  `{org}.{platform}.{domain}.{system}.{type}.{instance}` continue to map to the published IRI path
  `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}` via `message.ParseEntityID`'s named fields
  (owner item O-10).
- `ADR-107` (`docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md`): kept unchanged. Three-part dotted
  predicates remain canonical in `vocabulary`, ontology packs and SHACL live in `semvocab`, and RDF blank-node and
  full-IRI predicate rendering live exclusively at the `vocabulary/export` serialization edge.

Adopters:

- `C360Studio/semvocab` (`shacl.CompileShapes`, pack unit tests): calls `pack.Validate()` and `spec.Validate()`.
- `C360Studio/semsparql` (`irimap`, `CONSTRUCT` / `DESCRIBE` reified projection unfolding): calls
  `export.SubjectIRI(subject, base)` and passes unfolded `[]message.Triple` containing `_:label` blank nodes and full
  `http://`/`https://` predicate IRIs to `export.Serialize`.

## Options Considered

1. **Option A — Do nothing in `semengine` and work around in `semvocab` / `semsparql`**:
   - *Cost*: `semvocab/shacl` would either mutate global `vocabulary` state under `SnapshotRegistry()` during pure
     shape compilation or duplicate `vocabulary`'s predicate, entity-type, prefix, and projection validation rules;
     `semsparql/irimap` would maintain a second copy of `subjectToIRI` (`ADR-102` O-10); and `semsparql` could not use
     `vocabulary/export.Serialize` to serialize unfolded reified BFO/CCO blank-node graphs. Rejected by `#256`.
2. **Option B — Export both `Validate() error` methods and top-level `ValidatePack` / `ValidateProjectionSpec` wrapper
   functions**:
   - *Cost*: Introduces two exported spellings for the same validation check in `vocabulary`, diverging from
     `PrefixSpec.Validate() error` (`vocabulary/pack.go:23`) and working against open issue `#219` (`unify Validate /
     IsValid`). Rejected.
3. **Option C (Recommended) — Extend existing owners with a single canonical spelling per seam**:
   - Export `(s ProjectionSpec) Validate() error` and `(p *PackSpec) Validate() error` in `vocabulary` (matching
     `(p PrefixSpec) Validate() error`), export `export.SubjectIRI(subject, base string) string` in `vocabulary/export`
     as the single implementation behind `subjectToIRI`, and extend `vocabulary/export` to recognize W3C Turtle /
     N-Triples `BLANK_NODE_LABEL` subjects/objects and unregistered `http://`/`https://` predicates.

## Measured Premises & Invariants

- **Measured premises (`inventory.md`)**:
  - `vocabulary/projection.go:58-89` (`validateProjectionSpec`) is unexported, accepts arbitrary IRI schemes (such as
    `urn:`), and allows a terminal step with `ValuePredicateIRI == ""`. Every valid projection in the existing test
    suite (`vocabulary/projection_test.go:14-26`, `vocabulary/pack_test.go:34-43,163-171,325-334,396`) already uses
    `http://` or `https://` IRIs and sets `ValuePredicateIRI` on the terminal step.
  - `vocabulary/pack.go:183-249` (`RegisterPack` / `stagePackLocked`) acquires `registryMu`, reads `predicateRegistry`
    and `entityTypeRegistry`, does not reject an empty `PackSpec.Name`, and allows intra-pack duplicate predicate or
    entity-type keys to overwrite earlier entries in the same slice.
  - `vocabulary/export/export.go:122-136` (`subjectToIRI`) is unexported and does not trim trailing `'/'` on `base` or
    default an empty `base` to `vocabulary.SemEngineBase`.
  - `vocabulary/export/export.go:139-144,203-217` and `vocabulary/export/object.go:124-137,152-157,330-348` have zero
    `_:` blank-node branches and rewrite unregistered `http://`/`https://` predicates under `{base}/predicates/...`.
- **Invariants for generated property and fuzz checks**:
  1. **Standalone `PackSpec.Validate()` purity invariant** (`TestPackSpecValidate_PureNoSideEffects`,
     `FuzzValidatePack` -> Requirement *Declarative JSON and YAML vocabulary packs load and register fail-closed*,
     Scenario *Standalone PackSpec validation has zero global side effects* `THEN` clause): for any `PackSpec` input
     (valid or invalid), calling `pack.Validate()` leaves `ListRegisteredPredicates()` and
     `ListRegisteredEntityTypes()` unchanged and does not mutate the caller's `PackSpec`.
  2. **Pack round-trip & validation consistency invariant** (`TestPropPackRoundTrip`, `FuzzLoadPackJSON`,
     `FuzzLoadPackYAML` -> Requirement *Declarative JSON and YAML vocabulary packs load and register fail-closed*,
     Scenarios *Valid pack registers predicates, entity types, and prefixes* and *Invalid entry in a pack leaves
     registry unchanged* `THEN` clauses): every generated valid `PackSpec` passes `pack.Validate()` and round-trips
     through JSON and YAML into `RegisterPack`, and any decoded `PackSpec` rejected by `pack.Validate()` or
     `RegisterPack` leaves the predicate and entity-type registries unchanged.
  3. **`SubjectIRI` exporter equivalence & base-normalization invariant** (`TestSubjectIRI_MatchesDefaultExporter`,
     `FuzzSubjectIRI` -> Requirement *SubjectIRI helper and blank-node/IRI-predicate export*, Scenario *SubjectIRI
     matches default exporter subject IRI construction* `THEN` clause): for any subject and base string,
     `SubjectIRI(subject, base)` equals `SubjectIRI(subject, strings.TrimRight(base, "/"))`, returns `""` iff
     `subject == ""`, uses `vocabulary.SemEngineBase` when trimmed `base` is empty, and matches the subject IRI emitted
     by `SerializeToString` for non-blank-node subjects.

## Decisions

### D1 Pure validation on `ProjectionSpec` and `PackSpec` in `vocabulary` (`ADR-107`)

- **Observable behavior**:
  - `(s ProjectionSpec) Validate() error` validates `s` with zero global state reads or writes:
    1. `len(s.Steps) > 0`.
    2. Every step's `PredicateIRI` is a non-empty valid `http://` or `https://` IRI (no whitespace, control characters,
       or Turtle `IRIREF`-forbidden characters `<>"{}|^\` and backtick).
    3. Every non-empty `step.NodeClassIRI` is a valid `http://` or `https://` IRI, and `step.NodeClassIRI` is required
       on every non-terminal step (`i < len(s.Steps)-1`).
    4. `step.ValuePredicateIRI` is forbidden on non-terminal steps (`i < len(s.Steps)-1`) and required as a valid
       `http://` or `https://` IRI on the terminal step (`i == len(s.Steps)-1`).
    - `WithProjection` / `validateProjectionSpec` delegates to `spec.Validate()`.
  - `(p *PackSpec) Validate() error` validates a standalone `PackSpec` without acquiring `registryMu`, reading or
    writing `predicateRegistry` or `entityTypeRegistry`, invoking any prefix registrar, or mutating `*p`:
    1. `p != nil` and `strings.TrimSpace(p.Name) != ""`.
    2. Every `PrefixSpec` passes `pfx.Validate()`, and duplicate `pfx.Prefix` entries within `p.Prefixes` are rejected.
    3. Every `PredicateSpec` has a valid 3-part `Name` (`ParsePredicate`), no duplicate `Name` within `p.Predicates`,
       valid `IRI` when non-empty (`validateNonEmptyIRI`), valid `DataType` when non-empty (`validateDataType`), valid
       alias metadata (refusing non-zero `AliasPriority` without `IsAlias` or `AliasType`, and validating `AliasType`),
       valid `InverseOf` syntax (`ParsePredicate`) and no `Symmetric && InverseOf != ""`, valid `Projection` via
       `spec.Projection.Validate()` when `spec.Projection != nil`, and intra-pack inverse symmetry when the inverse
       target is also declared in `p.Predicates`.
    4. Every `EntityTypeSpec` has a valid 2-part `DomainType` (`ParseEntityTypeKey`), no duplicate `DomainType` within
       `p.EntityTypes`, and valid `ClassIRI` / `Superclasses` IRIs (`validateNonEmptyIRI`).
  - `RegisterPack` calls `pack.Validate()` first, then under `registryMu` stages into clones of `predicateRegistry`
    (checking cross-pack inverse consistency against already-registered predicates) and `entityTypeRegistry`, runs any
    configured `WithPrefixRegistrar`, and commits.
- **Proving tests**: `TestProjectionSpecValidate`, `TestPackSpecValidate_PureNoSideEffects`,
  `TestRegisterPackAndAtomicOnFailure`, `TestPropPackRoundTrip`, `FuzzLoadPackJSON`, `FuzzLoadPackYAML`, and
  `FuzzValidatePack`.

### D2 Export `export.SubjectIRI` in `vocabulary/export` (`ADR-102`)

- **Observable behavior**:
  - `export.SubjectIRI(subject, base string) string` returns `""` when `subject == ""`.
  - It normalizes `base` by stripping trailing `'/'` characters (`strings.TrimRight(base, "/")`) and defaulting an
    empty `base` to `vocabulary.SemEngineBase`.
  - When `message.ParseEntityID(subject)` succeeds, it returns
    `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}` per `ADR-102` O-10.
  - Otherwise it returns `{base}/subjects/{dots->slashes}`.
  - `subjectToIRI` delegates directly to `SubjectIRI`.
- **Proving tests**: `TestSubjectIRI_MatchesDefaultExporter`, `TestSubjectToIRI_EntityID`,
  `TestSubjectToIRI_NonEntityFallback`, `TestSubjectToIRI_Empty`, and `FuzzSubjectIRI`.

### D3 Blank-node (`_:label`) and full-IRI predicates in `export` (`ADR-107`)

- **Observable behavior**:
  - `isValidBlankNodeLabel(s string) bool` reports whether `s` starts with `"_:"` followed by a valid W3C Turtle /
    N-Triples `BLANK_NODE_LABEL` local part (`isValidPNLocal(s[2:])`, which enforces `PN_CHARS_U | [0-9]` at start,
    `PN_CHARS | '.'` in the middle, not ending in `.`, and no `:` or whitespace).
  - **Blank-node subjects**: When `isValidBlankNodeLabel(t.Subject)` is true, `resolveSubjectIRI` returns `t.Subject`
    verbatim (`_:label`) without invoking `opts.subjectIRIFn` or `SubjectIRI`. `writeNTriples` emits `%s <%s> %s .`
    without `<...>` around a blank-node subject; `scanPrefixes` skips `pm.compact(subIRI)` for blank-node subjects;
    `formatTurtleTerm` returns `iri` directly without `<...>` or prefix compaction when `isValidBlankNodeLabel(iri)` is
    true; and `buildJSONLDGraph` sets `"@id": "_:label"`.
  - **Blank-node objects**: `objectBlankNode` is recognized in `classifyWithExplicitDatatype` (when
    `t.Datatype == message.EntityReferenceDatatype` and `t.Object` is a valid blank node label),
    `classifyByDeclaredDataType` (when `declared == vocabulary.DataTypeEntityID` and `obj` is a valid blank node label),
    and `classifyString` (when `isValidBlankNodeLabel(s)` is true). N-Triples and Turtle emit `_:label` without `<...>`
    or prefix compaction (`scanPrefixes` does not compact `objectBlankNode`), and JSON-LD emits `{"@id": "_:label"}`.
    An explicit non-`@id` datatype (such as `xsd:string`) still forces a literal, and malformed `"_:"` strings (such as
    `"_:"`, `"_:bad."`, `"_:has space"`) fall back to `SubjectIRI` in subject position or literal classification in
    object position rather than emitting invalid unquoted RDF tokens.
  - **Full-IRI predicates**: In `resolvePredicateIRI(predicate, base string) string`, after checking
    `vocabulary.GetPredicateMetadata(predicate)` for a registered `StandardIRI`, if `isHTTPIRI(predicate)` is true
    (starts with `"http://"` or `"https://"` with a non-empty remainder and no whitespace, control characters, or
    Turtle `IRIREF`-forbidden characters `<>"{}|^\` and backtick), `resolvePredicateIRI` returns `predicate` unchanged
    (enabling N-Triples `<IRI>`, Turtle prefix compaction, and JSON-LD `@context` compaction on intermediate projection
    hops). Otherwise it normalizes `base` (`strings.TrimRight(base, "/")`, defaulting empty to
    `vocabulary.SemEngineBase`) and returns `{base}/predicates/{dots->slashes}`.
- **Proving tests**: `TestSerialize_UnfoldedReifiedBlankNodeChain` (across `NTriples`, `Turtle`, and `JSONLD`),
  `TestBlankNodeValidation_MalformedFallback`, and `TestResolvePredicateIRI_FullIRIPassThrough`.
