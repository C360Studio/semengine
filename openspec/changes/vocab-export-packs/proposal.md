# Proposal: vocab-export-packs

## Why

An early-adopter Defense and Intelligence workload requires upper/mid-level ontology alignment (BFO 2020, Common Core
Ontologies / CCO, and Defense and Intelligence Enterprise Knowledge Model / DIEKM) and a W3C SPARQL 1.1 Protocol
endpoint (`/sparql`), while `semengine` core must stay pragmatic, 3-part dotted-predicate-first, and free of Semantic
Web query machinery and domain ontologies (`ADR-107`).

The owner ruled (`#234`) on a three-repository architecture:

1. **`semengine`** owns the generic vocabulary registry, bidirectional IRI lookup, entity-type (`domain.type`) class
   metadata, reified projection path metadata (`ProjectionSpec`), declarative pack loading (JSON and YAML), and the
   RDF/JSON-LD export serializer (`vocabulary/export`).
2. **`semvocab`** owns ontology packs (`bfo`, `cco`, `diekm`) as both Go packages and declarative JSON/YAML files.
3. **`semsparql`** owns the read-only W3C SPARQL 1.1 Protocol projection adapter over `semengine`.

Today `vocabulary` in `semengine` maps `dotted -> IRI` (`GetPredicateMetadata`) and `alias -> canonical`
(`ResolveAlias`), but has no reverse `IRI -> []dotted` index, no entity-type (`domain.type`) class IRI registration for
`rdf:type` matching, no reification/projection path metadata (`ProjectionSpec`) to project flat 3-part triples into
multi-node BFO/CCO measurement subgraphs at the SPARQL/export boundary, and no declarative JSON/YAML pack loader. In
addition, `vocabulary/export` was scheduled for Slice 04A Change 7 even though all of its dependencies (`message`,
`pkg/types`, `vocabulary`) are already in the floor.

## What changes

- **Port `vocabulary/export` and `ADR-107` now**: bring `vocabulary/export` (N-Triples, Turtle, JSON-LD, prefix
  registry, and `PredicateDatatype` literal formatting) and `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md`
  from the SemStreams pin (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`), adapted to `vocabulary.SemEngineBase`.
- **Add reverse IRI lookup (`LookupByIRI`)**: maintain a reverse `IRI -> []string` index in `vocabulary` so
  `LookupByIRI(iri)` returns all canonical dotted predicates registered with `WithIRI(iri)` in deterministic sorted
  order.
- **Add entity-type (`domain.type`) class registration**: add `EntityTypeMetadata`, `EntityTypeOption`
  (`WithEntityTypeDescription`, `WithClassIRI`, `WithSuperclass`), `ParseEntityTypeKey`, `RegisterEntityType`,
  `GetEntityTypeMetadata`, `ListRegisteredEntityTypes`, and `LookupEntityTypesByClassIRI` so `semvocab` can map entity
  `domain.type` pairs to ontology class IRIs and `semsparql` can resolve `?s rdf:type <ClassIRI>` patterns.
- **Add reification/projection path metadata (`ProjectionSpec`)**: add `ProjectionStep`, `ProjectionSpec`, and
  `WithProjection(spec ProjectionSpec)` on `PredicateMetadata` so a flat SemEngine triple can declare the multi-hop
  BFO/CCO pattern it projects to at the SPARQL/export edge.
- **Add declarative JSON/YAML pack loader (`PackSpec`)**: add `PackSpec`, `PrefixSpec`, `PredicateSpec`, and
  `EntityTypeSpec` (with `json` and `yaml` tags), `LoadPackJSON`, `LoadPackYAML` (refusing unknown keys), and
  `RegisterPack` (validating the entire pack fail-closed before mutating registry state).
- **Update portfolio scope and docs**: add `semvocab` and `semsparql` to `docs/inventory-scope.md`; record
  `vocabulary/bfo` and `vocabulary/cco` as `defer-exclude` (moved to `semvocab`) and add `vocabulary/export` and
  `ADR-107` in `docs/admission-ledger.yaml`, `docs/tier1-cross-check.md`, `docs/repository-map.md`,
  `docs/adr/102-entity-id-segment-semantics.md`, and `vocabulary/README.md`.

## Impact

`vocabulary/` (new `entity_type.go`, `pack.go`, `projection.go`, updates to `predicates.go`, `registry.go`, `doc.go`,
`README.md`, and tests), `vocabulary/export/` (ported package), `internal/harness/contract/imports_test.go` (permitting
`vocabulary/pack.go` to import `gopkg.in/yaml.v3`), `docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md`,
`docs/adr/102-entity-id-segment-semantics.md`, `docs/admission-ledger.yaml`, `docs/inventory-scope.md`,
`docs/tier1-cross-check.md`, `docs/repository-map.md`, new capability spec `openspec/specs/vocabulary-packs/spec.md`,
and modified `openspec/specs/harness-boundaries/spec.md`.
Closes #234, #237, #238, #239. Addresses #9.
