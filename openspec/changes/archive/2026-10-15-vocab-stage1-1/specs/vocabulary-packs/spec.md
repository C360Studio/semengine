# vocabulary-packs Specification Delta

## MODIFIED Requirements

### Requirement: Reification and projection path metadata on predicates

`vocabulary.ProjectionSpec.Validate()` SHALL validate a `ProjectionSpec` without reading or mutating global registry
state, refusing an empty `Steps` slice, any step whose `PredicateIRI` is not a valid `http://` or `https://` IRI, any
step with a non-empty `NodeClassIRI` that is not a valid `http://` or `https://` IRI, any non-terminal step without
`NodeClassIRI` or with `ValuePredicateIRI` set, or a terminal step without a valid `http://` or `https://`
`ValuePredicateIRI`.
`vocabulary.WithProjection(spec ProjectionSpec)` SHALL attach an ordered slice of `ProjectionStep` entries to
`PredicateMetadata.Projection`, panicking at registration if `spec.Validate()` returns an error.
`GetPredicateMetadata` SHALL return a deep copy of `Projection`, and amending a predicate without `WithProjection`
SHALL preserve its existing `Projection`.

#### Scenario: Projection metadata survives registration and amend

- **WHEN** a predicate is registered with a two-step `ProjectionSpec` (linking an entity to a `cco:ActOfMeasuring`
  node and a literal value predicate) and then amended with `WithDescription`
- **THEN** `GetPredicateMetadata` returns a deep copy of the two-step `ProjectionSpec` along with the updated
  description

#### Scenario: Pure ProjectionSpec validation rejects invalid steps

- **WHEN** `ProjectionSpec.Validate()` is called on a `ProjectionSpec` whose terminal step omits `ValuePredicateIRI`
  or uses a non-`http://`/`https://` IRI
- **THEN** it returns a non-nil error without reading or modifying global registry state

### Requirement: Declarative JSON and YAML vocabulary packs load and register fail-closed

`vocabulary.LoadPackJSON` and `vocabulary.LoadPackYAML` SHALL decode a `PackSpec` from JSON or YAML while refusing
unknown fields, malformed syntax, or trailing data. `(*vocabulary.PackSpec).Validate()` SHALL validate a `PackSpec` with
zero global state reads or writes, refusing a nil pack, an empty or whitespace-only `Name`, any invalid or duplicate
`PrefixSpec`, any invalid or duplicate `PredicateSpec` (including 3-part name syntax, datatype, IRI syntax, alias
metadata consistency, `ProjectionSpec.Validate`, and intra-pack `InverseOf` symmetry), or any invalid or duplicate
`EntityTypeSpec`. `vocabulary.RegisterPack` SHALL validate the `PackSpec` via `PackSpec.Validate()`, verify cross-pack
inverse consistency, and run any configured prefix registrar before mutating registry state; if any entry or batch
prefix registration is invalid, `RegisterPack` SHALL return an error and leave the vocabulary and prefix registries
unchanged.

#### Scenario: Valid pack registers predicates, entity types, and prefixes

- **WHEN** a `PackSpec` decoded by `LoadPackJSON` or `LoadPackYAML` is passed to `RegisterPack` with
  `WithPrefixRegistrar(export.RegisterPrefixes)`
- **THEN** its predicates, aliases, inverses, projections, entity types, and export prefixes are all registered and
  resolvable

#### Scenario: Invalid entry in a pack leaves registry unchanged

- **WHEN** `RegisterPack` is called with a `PackSpec` whose first predicate is valid and whose second predicate,
  entity type, or late prefix binding is invalid
- **THEN** `RegisterPack` returns an error and neither the first predicate nor any earlier prefix in the pack is
  registered

#### Scenario: Standalone PackSpec validation has zero global side effects

- **WHEN** `pack.Validate()` is called on a valid or invalid `PackSpec` while the global vocabulary registry is empty
- **THEN** `ListRegisteredPredicates()` and `ListRegisteredEntityTypes()` remain empty and the caller's `PackSpec` is
  not mutated

## ADDED Requirements

### Requirement: SubjectIRI helper and blank-node/IRI-predicate export

`export.SubjectIRI(subject, base string) string` SHALL return `""` when `subject` is empty, normalize `base` by
trimming trailing `'/'` characters and defaulting an empty `base` to `vocabulary.SemEngineBase`, format a valid 6-part
entity ID as `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}`, and format any other non-empty
subject as `{base}/subjects/{dots->slashes}`.
`export.Serialize` and `export.SerializeToString` SHALL render subjects and objects (when `Triple.Datatype` is empty or
`message.EntityReferenceDatatype`) that match W3C Turtle / N-Triples `BLANK_NODE_LABEL` (`_:` followed by a valid
`PN_LOCAL` label) as RDF blank nodes (`_:label` in N-Triples and Turtle without angle brackets or prefix compaction,
and `{"@id": "_:label"}` in JSON-LD), SHALL fall back to `SubjectIRI` or literal serialization for malformed `"_:"`
strings or explicit non-`@id` datatypes rather than emitting invalid raw blank-node tokens, and SHALL pass through
unregistered `Triple.Predicate` values that are valid `http://` or `https://` IRIs unchanged across N-Triples, Turtle,
and JSON-LD.

#### Scenario: SubjectIRI matches default exporter subject IRI construction

- **WHEN** `export.SubjectIRI("acme.ops.robotics.gcs.drone.001", "")` and
  `export.SubjectIRI("some.topic.name", "https://example.org/")` are called
- **THEN** they return `"https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001"` and
  `"https://example.org/subjects/some/topic/name"` respectively, matching `export.Serialize`

#### Scenario: Unfolded reified BFO/CCO blank-node chain serializes cleanly

- **WHEN** `export.SerializeToString` is called in `NTriples`, `Turtle`, and `JSONLD` with a 3-triple reified
  measurement chain connecting entity `"acme.ops.robotics.gcs.drone.001"` to blank node `"_:b_1"` via full-IRI
  predicate `"http://purl.obolibrary.org/obo/RO_0000056"`, typing `"_:b_1"` with `rdf:type`
  `"http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring"`, and attaching literal `85.5` via
  `"http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value"`
- **THEN** `"_:b_1"` is serialized as a blank node in both subject and object position (never wrapped in `<...>` or
  quotes), and the unregistered `http://` predicates are emitted as full or prefix-compacted IRIs (never rewritten
  under `/predicates/`)

#### Scenario: Malformed blank-node labels and explicit literal datatypes

- **WHEN** `export.SerializeToString` is called with malformed blank-node strings (`"_:"`, `"_:bad."`, `"_:has space"`)
  or with a valid blank-node string carrying `Datatype: "xsd:string"`
- **THEN** it never emits a raw unquoted `_:` blank-node token for those values, rendering malformed object strings and
  `xsd:string`-typed objects as quoted string literals and malformed subject strings via `SubjectIRI`
