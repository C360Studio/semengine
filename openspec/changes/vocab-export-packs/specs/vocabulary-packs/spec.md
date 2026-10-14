# vocabulary-packs

## ADDED Requirements

### Requirement: Reverse predicate IRI lookup returns sorted canonical predicates

`vocabulary.LookupByIRI(iri string)` SHALL return a newly allocated slice containing every canonical 3-part dotted
predicate registered with `WithIRI(iri)`, sorted in lexicographical order, or `nil` when `iri` is empty or no
predicate is registered for `iri`. Amending a predicate with a new non-empty `WithIRI` SHALL update the reverse index,
and amending without `WithIRI` SHALL preserve the existing `StandardIRI` mapping. `ClearRegistry` and `SnapshotRegistry`
SHALL clear and restore the reverse index alongside predicates and aliases.

#### Scenario: One-to-many IRI mapping returns sorted predicates

- **WHEN** `geo.location.latitude` and `air.track.latitude` are both registered with `WithIRI("http://example.org/lat")`
- **THEN** `LookupByIRI("http://example.org/lat")` returns `[]string{"air.track.latitude", "geo.location.latitude"}`

#### Scenario: Amending IRI updates reverse lookup

- **WHEN** `ops.alert.status` is registered with `WithIRI("http://example.org/v1/status")` and later amended with
  `WithIRI("http://example.org/v2/status")`
- **THEN** `LookupByIRI("http://example.org/v1/status")` returns `nil` and
  `LookupByIRI("http://example.org/v2/status")` returns `[]string{"ops.alert.status"}`

### Requirement: Entity-type class metadata registration and reverse lookup

`vocabulary.ParseEntityTypeKey(domainType string)` SHALL accept only a two-segment lower-kebab ASCII string
`<domain>.<type>` where each segment obeys the entity-ID segment grammar, returning an error otherwise.
`vocabulary.RegisterEntityType(domainType string, opts ...EntityTypeOption)` SHALL panic on an invalid `domain.type`
key or an option with an empty or whitespace-containing IRI, and SHALL otherwise register or amend
`EntityTypeMetadata` (preserving existing fields and deduplicating `Superclasses`). `LookupEntityTypesByClassIRI(iri)`
SHALL return all registered `domain.type` keys whose `ClassIRI` or `Superclasses` contain `iri`, sorted
lexicographically, or `nil` when none match.

#### Scenario: Class and superclass lookup

- **WHEN** `uxs.uas` is registered with `WithClassIRI("https://confirm.mil/diekm/UnmannedAerialSystem")` and
  `WithSuperclass("http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft")`
- **THEN** `LookupEntityTypesByClassIRI` returns `[]string{"uxs.uas"}` for both IRIs and `GetEntityTypeMetadata`
  returns a defensive copy of the metadata

#### Scenario: Invalid entity-type key is refused

- **WHEN** `ParseEntityTypeKey` is called with `"onlyone"`, `"three.part.key"`, `"Upper.Case"`, or `"bad..key"`
- **THEN** it returns a non-nil error

### Requirement: Reification and projection path metadata on predicates

`vocabulary.WithProjection(spec ProjectionSpec)` SHALL attach an ordered slice of `ProjectionStep` entries to
`PredicateMetadata.Projection`, refusing an empty `Steps` slice or a step with an empty or whitespace-containing IRI.
`GetPredicateMetadata` SHALL return a deep copy of `Projection`, and amending a predicate without `WithProjection`
SHALL preserve its existing `Projection`.

#### Scenario: Projection metadata survives registration and amend

- **WHEN** a predicate is registered with a two-step `ProjectionSpec` (linking an entity to a `cco:ActOfMeasuring`
  node and a literal value predicate) and then amended with `WithDescription`
- **THEN** `GetPredicateMetadata` returns a deep copy of the two-step `ProjectionSpec` along with the updated
  description

### Requirement: Declarative JSON and YAML vocabulary packs load and register fail-closed

`vocabulary.LoadPackJSON` and `vocabulary.LoadPackYAML` SHALL decode a `PackSpec` from JSON or YAML while refusing
unknown fields, malformed syntax, or trailing data. `vocabulary.RegisterPack` SHALL validate every prefix, predicate,
and entity type in the `PackSpec` before mutating registry state; if any entry is invalid, `RegisterPack` SHALL return
an error and leave the vocabulary and prefix registries unchanged.

#### Scenario: Valid pack registers predicates, entity types, and prefixes

- **WHEN** a `PackSpec` decoded by `LoadPackJSON` or `LoadPackYAML` is passed to `RegisterPack` with
  `WithPrefixRegistrar(export.Register)`
- **THEN** its predicates, aliases, inverses, projections, entity types, and export prefixes are all registered and
  resolvable

#### Scenario: Invalid entry in a pack leaves registry unchanged

- **WHEN** `RegisterPack` is called with a `PackSpec` whose first predicate is valid and whose second predicate or
  entity type is invalid
- **THEN** `RegisterPack` returns an error and the first predicate is not registered in the vocabulary registry
