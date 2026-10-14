# Design: vocab-export-packs

## Context

Issue #234 implements Stage 1 of the three-repo ontology and SPARQL architecture (`semengine` -> `semvocab` ->
`semsparql`) confirmed by the owner on 2026-10-14:

1. Full code PR in `semengine` (`Closes #234`, `Closes #237`, `Closes #238`, `Closes #239`, `Addresses #9`).
2. Port `vocabulary/export` and `ADR-107` now rather than waiting for Change 7.
3. Support both JSON and YAML in `vocabulary`'s declarative pack loader (`PackSpec`).

Merge order: independent of open PRs `#230` and `#231` (`inventory.md`).
Adopters of the new primitives: `semvocab` (`RegisterPack`, `LoadPackJSON`, `LoadPackYAML`, `RegisterEntityType`,
`WithProjection`, `export.Register`, `export.RegisterPrefixes`) and `semsparql` (`LookupByIRI`,
`LookupEntityTypesByClassIRI`, `GetPredicateMetadata`, `vocabulary/export`).

## Decisions

### D1 Port `vocabulary/export` and `ADR-107` (`adapt`)

- **Observable behavior**: `export.Serialize(w, triples, format, ...opts)` and
  `export.SerializeToString(triples, format, ...opts)` serialize `[]message.Triple` (such as `entity.Triples`) into
  N-Triples (`export.NTriples`), Turtle (`export.Turtle`), and JSON-LD (`export.JSONLD`), using
  `vocabulary.SemEngineBase` (`https://semengine.semanticstream.ing`) when `WithBaseIRI` is unset.
  `export.Register(prefix, namespace)` and `export.RegisterPrefixes(prefixes)` validate prefix labels against W3C
  Turtle `PN_PREFIX` (`PrefixSpec.Validate`, `#237`) and register namespace prefixes atomically for compact IRIs;
  equal-length namespace aliases break ties deterministically by lexicographically smallest prefix (`#238`); and
  `xsd:dateTime` formatting preserves fractional seconds via `time.RFC3339Nano` (`#239`).
- **Adaptations from pin (`8b99efe9`)**: replace `vocabulary.SemStreamsBase` with `vocabulary.SemEngineBase` in
  `export.go` and `prefix.go` and in 5 test files; add `RegisterPrefixes` and `PN_PREFIX` validation (`#237`),
  deterministic alias tie-breaking in `compact` (`#238`), and `time.RFC3339Nano` in `dateTimeLexical` and
  `classifyByGoType` (`#239`); add `defer vocabulary.SnapshotRegistry()()` before `vocabulary.ClearRegistry()` in test
  files; port `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md` with a SemEngine port header; record
  `vocabulary/bfo` and `vocabulary/cco` as `defer-exclude` (homed in `semvocab`).
- **Proving tests**: `go test -race -count=1 ./vocabulary/export/...`, `task test:repeat`, and `task ledger:check`.

### D2 Reverse predicate IRI lookup (`LookupByIRI`)

- **Observable behavior**: `vocabulary.LookupByIRI(iri string) []string` returns a newly allocated slice of all
  canonical 3-part dotted predicates whose registered `StandardIRI` equals `iri`, sorted lexicographically, or `nil`
  when `iri` is empty or has no registered predicate. Amending a predicate with a new non-empty `WithIRI` updates the
  reverse index; amending without `WithIRI` preserves the existing `StandardIRI` and its reverse mapping.
  `ClearRegistry` and `SnapshotRegistry` clear and restore the reverse index alongside predicates and aliases.
- **Proving tests**: `TestLookupByIRI` (single, one-to-many sorted, amend, unregistered/empty, snapshot/clear) and
  `TestPropLookupByIRIRoundTrip` (Rapid property test).

### D3 Entity-type (`domain.type`) class registration (`RegisterEntityType`)

- **Observable behavior**: `ParseEntityTypeKey(domainType string) (domain, entityType string, err error)` validates a
  two-segment lower-kebab ASCII key `<domain>.<type>` (corresponding to entity ID 0-indexed positions 2 and 4) where
  each segment obeys the narrower vocabulary predicate segment grammar (`[a-z][a-z0-9-]*`, max 64 bytes, no trailing or
  consecutive hyphens via `validatePredicateSegment` in `predicates.go`, which is stricter than
  `pkg/types.ValidateEntityID`'s general segment grammar). `RegisterEntityType(domainType string, opts ...EntityTypeOption)`
  panics on an invalid `domain.type` key or invalid option (empty/whitespace `ClassIRI` or `Superclass`), and otherwise
  registers or amends `EntityTypeMetadata{DomainType, Description, ClassIRI, Superclasses}` (preserving existing fields
  on amend and deduplicating `Superclasses` in registration order). `GetEntityTypeMetadata(domainType)` returns a
  defensive copy (or `nil`), `ListRegisteredEntityTypes()` returns all registered `domain.type` keys sorted, and
  `LookupEntityTypesByClassIRI(classIRI)` returns all `domain.type` keys whose `ClassIRI` or `Superclasses` match
  `classIRI` in sorted order (or `nil`). `ClearRegistry` and `SnapshotRegistry` include entity-type registrations.
- **Proving tests**: `TestParseEntityTypeKey`, `TestRegisterEntityType`, `TestLookupEntityTypesByClassIRI`, and
  `FuzzParseEntityTypeKey`.

### D4 Reified BFO/CCO projection metadata (`ProjectionSpec`, `WithProjection`)

- **Observable behavior**: `ProjectionStep` holds `PredicateIRI`, `NodeClassIRI` (optional intermediate blank/skolem
  node class IRI), and `ValuePredicateIRI` (optional predicate IRI from the intermediate node to the literal/target
  value), with `json` and `yaml` tags. `ProjectionSpec` holds `Steps []ProjectionStep`. `WithProjection(spec)`
  validates that `len(spec.Steps) > 0`, every step has a non-empty trimmed `PredicateIRI`, and any step with
  `NodeClassIRI != ""` (except a terminal link step) or `ValuePredicateIRI != ""` is well-formed (refusing empty/invalid
  steps at registration), and stores a deep copy on `PredicateMetadata.Projection`. `GetPredicateMetadata` returns a
  deep copy of `Projection`, and amending a predicate without `WithProjection` preserves an existing `Projection`.
- **Proving tests**: `TestRegisterWithProjection` (deep copy, amend preservation, invalid step rejection).

### D5 Declarative JSON/YAML vocabulary packs (`PackSpec`, `RegisterPack`)

- **Observable behavior**: `PackSpec` carries `Name`, `Version`, `Prefixes []PrefixSpec`, `Predicates []PredicateSpec`,
  and `EntityTypes []EntityTypeSpec` with `json` and `yaml` tags. `PrefixSpec.Validate()` enforces W3C Turtle
  `PN_PREFIX` syntax on `Prefix` and non-empty whitespace-free `Namespace` (`#237`). `LoadPackJSON(r io.Reader)` and
  `LoadPackYAML(r io.Reader)` decode strictly (refusing unknown fields, trailing documents, or invalid syntax).
  `RegisterPack(pack PackSpec, opts ...PackOption)` validates every prefix (`PrefixSpec.Validate`), predicate
  (`ParsePredicate` and shared `validatePredicateMetadataIn`, including post-staging inverse consistency), and entity
  type (`ParseEntityTypeKey`, class IRI, superclasses) **before** mutating any state, and invokes any configured batch
  prefix registrar (`WithPrefixRegistrar(func([]PrefixSpec) error)`, such as `export.RegisterPrefixes`) before committing
  staged predicates and entity types (fail-closed: a single invalid entry or prefix conflict leaves both the vocabulary
  and prefix registries unchanged). `internal/harness/contract` (`imports_test.go`) and `harness-boundaries` permit
  `vocabulary/pack.go` to import `gopkg.in/yaml.v3` while continuing to reject `gopkg.in/yaml.v3` in other non-test
  production files.
- **Proving tests**: `TestLoadPackJSON`, `TestLoadPackYAML`, `TestRegisterPackAndAtomicOnFailure`,
  `TestPropPackRoundTrip`, `FuzzLoadPackJSON`, `FuzzLoadPackYAML`, and `TestImportGraphSensitivity`.

### D6 Generated checks

Per `docs/testing.md` and `.agents/contracts/semengine-developer.md`:

- Rapid property tests (`TestPropLookupByIRIRoundTrip`, `TestPropPackRoundTrip`) verify round-trip laws over generated
  predicates, IRIs, entity types, and projections.
- Fuzz targets (`FuzzParseEntityTypeKey`, `FuzzLoadPackJSON`, `FuzzLoadPackYAML`) exercise the three new parse/decode
  entry points with valid and invalid seed inputs.
