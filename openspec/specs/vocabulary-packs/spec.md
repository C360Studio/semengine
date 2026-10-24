# vocabulary-packs Specification

## Purpose

Vocabulary packs and ontology alignment primitives let `vocabulary` register and query reverse predicate IRI mappings,
`domain.type` entity class metadata, reified projection path specifications, and declarative JSON/YAML ontology packs
fail-closed while keeping domain and upper ontologies (`semvocab`) and SPARQL query execution (`semsparql`) outside the
engine core.

## Requirements

### Requirement: Reverse predicate IRI lookup returns sorted canonical predicates

`vocabulary.LookupByIRI(iri string)` SHALL return a newly allocated slice containing every canonical 3-part dotted
predicate registered with `WithIRI(iri)`, sorted in lexicographical order, or `nil` when `iri` is empty or no
predicate is registered for `iri`.

#### Scenario: One-to-many IRI mapping returns sorted predicates

- **WHEN** `geo.location.latitude` and `air.track.latitude` are both registered with `WithIRI("http://example.org/lat")`
- **THEN** `LookupByIRI("http://example.org/lat")` returns `[]string{"air.track.latitude", "geo.location.latitude"}`

### Requirement: Reverse IRI index follows predicate amendment

Amending a predicate with a new non-empty `WithIRI` SHALL update the reverse index, and amending without `WithIRI`
SHALL preserve the existing `StandardIRI` mapping.

#### Scenario: Amending IRI updates reverse lookup

- **WHEN** `ops.alert.status` is registered with `WithIRI("http://example.org/v1/status")` and later amended with
  `WithIRI("http://example.org/v2/status")`
- **THEN** `LookupByIRI("http://example.org/v1/status")` returns `nil` and
  `LookupByIRI("http://example.org/v2/status")` returns `[]string{"ops.alert.status"}`

### Requirement: Reverse IRI index follows registry clear and snapshot

`ClearRegistry` and `SnapshotRegistry` SHALL clear and restore the reverse index alongside predicates and aliases.

#### Scenario: Clearing the registry clears the reverse index

- **WHEN** a predicate is registered with `WithIRI(iri)` and `ClearRegistry` is then called
- **THEN** `LookupByIRI(iri)` returns `nil`

#### Scenario: Restoring a snapshot restores the reverse index

- **WHEN** the function returned by `SnapshotRegistry` is called after the registry changed
- **THEN** `LookupByIRI` answers as it did when the snapshot was taken

### Requirement: Entity-type class metadata registration and reverse lookup

`vocabulary.ParseEntityTypeKey(domainType string)` SHALL accept only a two-segment lower-kebab ASCII string
`<domain>.<type>` where each segment obeys the vocabulary predicate segment grammar (`[a-z][a-z0-9-]*`, maximum 64
bytes, no trailing or consecutive hyphens), returning an error otherwise.

#### Scenario: Invalid entity-type key is refused

- **WHEN** `ParseEntityTypeKey` is called with `"onlyone"`, `"three.part.key"`, `"Upper.Case"`, or `"bad..key"`
- **THEN** it returns a non-nil error

### Requirement: Entity-type registration panics on bad input and merges on amend

`vocabulary.RegisterEntityType(domainType string, opts ...EntityTypeOption)` SHALL panic on an invalid `domain.type`
key or an option with an empty or whitespace-containing IRI, and SHALL otherwise register or amend
`EntityTypeMetadata` (preserving existing fields and deduplicating `Superclasses`).

#### Scenario: Invalid registration input panics

- **WHEN** `RegisterEntityType` is called with the key `"onlyone"` or with an option whose IRI is empty
- **THEN** it panics

### Requirement: Entity types are found by class IRI

`LookupEntityTypesByClassIRI(iri)` SHALL return all registered `domain.type` keys whose `ClassIRI` or `Superclasses`
contain `iri`, sorted lexicographically, or `nil` when none match.

#### Scenario: Class and superclass lookup

- **WHEN** `uxs.uas` is registered with `WithClassIRI("https://confirm.mil/diekm/UnmannedAerialSystem")` and
  `WithSuperclass("http://www.ontologyrepository.com/CommonCoreOntologies/Aircraft")`
- **THEN** `LookupEntityTypesByClassIRI` returns `[]string{"uxs.uas"}` for both IRIs and `GetEntityTypeMetadata`
  returns a defensive copy of the metadata

### Requirement: Reification and projection path metadata on predicates

`vocabulary.ProjectionSpec.Validate()` SHALL validate a `ProjectionSpec` without reading or mutating global registry
state, refusing an empty `Steps` slice, any step whose `PredicateIRI` is not a valid `http://` or `https://` IRI, any
step with a non-empty `NodeClassIRI` that is not a valid `http://` or `https://` IRI, any non-terminal step without
`NodeClassIRI` or with `ValuePredicateIRI` set, or a terminal step without a valid `http://` or `https://`
`ValuePredicateIRI`.

#### Scenario: Pure ProjectionSpec validation rejects invalid steps

- **WHEN** `ProjectionSpec.Validate()` is called on a `ProjectionSpec` whose terminal step omits `ValuePredicateIRI`
  or uses a non-`http://`/`https://` IRI
- **THEN** it returns a non-nil error without reading or modifying global registry state

### Requirement: Projection metadata attaches to predicates by deep copy

`vocabulary.WithProjection(spec ProjectionSpec)` SHALL attach an ordered slice of `ProjectionStep` entries to
`PredicateMetadata.Projection`, panicking at registration if `spec.Validate()` returns an error.
`GetPredicateMetadata` SHALL return a deep copy of `Projection`, and amending a predicate without `WithProjection`
SHALL preserve its existing `Projection`.

#### Scenario: Projection metadata survives registration and amend

- **WHEN** a predicate is registered with a two-step `ProjectionSpec` (linking an entity to a `cco:ActOfMeasuring`
  node and a literal value predicate) and then amended with `WithDescription`
- **THEN** `GetPredicateMetadata` returns a deep copy of the two-step `ProjectionSpec` along with the updated
  description

### Requirement: Declarative JSON and YAML vocabulary packs load and register fail-closed

`vocabulary.LoadPackJSON` and `vocabulary.LoadPackYAML` SHALL decode a `PackSpec` from JSON or YAML while refusing
unknown fields, malformed syntax, or trailing data.

#### Scenario: Valid pack registers predicates, entity types, and prefixes

- **WHEN** a `PackSpec` decoded by `LoadPackJSON` or `LoadPackYAML` is passed to `RegisterPack` with
  `WithPrefixRegistrar(export.RegisterPrefixes)`
- **THEN** its predicates, aliases, inverses, projections, entity types, and export prefixes are all registered and
  resolvable

### Requirement: PackSpec validation refuses invalid packs without global state

`(*vocabulary.PackSpec).Validate()` SHALL validate a `PackSpec` with zero global state reads or writes, refusing a nil
pack, an empty or whitespace-only `Name`, any invalid or duplicate `PrefixSpec`, any invalid or duplicate
`PredicateSpec` (including 3-part name syntax, datatype, IRI syntax, alias metadata consistency,
`ProjectionSpec.Validate`, and intra-pack `InverseOf` symmetry), or any invalid or duplicate `EntityTypeSpec`.

#### Scenario: Standalone PackSpec validation has zero global side effects

- **WHEN** `pack.Validate()` is called on a valid or invalid `PackSpec` while the global vocabulary registry is empty
- **THEN** `ListRegisteredPredicates()` and `ListRegisteredEntityTypes()` remain empty and the caller's `PackSpec` is
  not mutated

### Requirement: Pack registration fails closed

`vocabulary.RegisterPack` SHALL validate the `PackSpec` via `PackSpec.Validate()`, verify cross-pack inverse
consistency, and run any configured prefix registrar before mutating registry state; if any entry or batch prefix
registration is invalid, `RegisterPack` SHALL return an error and leave the vocabulary and prefix registries unchanged.

#### Scenario: Invalid entry in a pack leaves registry unchanged

- **WHEN** `RegisterPack` is called with a `PackSpec` whose first predicate is valid and whose second predicate,
  entity type, or late prefix binding is invalid
- **THEN** `RegisterPack` returns an error and neither the first predicate nor any earlier prefix in the pack is
  registered

### Requirement: SubjectIRI helper and blank-node/IRI-predicate export

`export.SubjectIRI(subject, base string) string` SHALL return `""` when `subject` is empty, normalize `base` by
trimming trailing `'/'` characters and defaulting an empty `base` to `vocabulary.SemEngineBase`, format a valid 6-part
entity ID as `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}`, and format any other non-empty
subject as `{base}/subjects/{dots->slashes}`.

#### Scenario: SubjectIRI matches default exporter subject IRI construction

- **WHEN** `export.SubjectIRI("acme.ops.robotics.gcs.drone.001", "")` and
  `export.SubjectIRI("some.topic.name", "https://example.org/")` are called
- **THEN** they return `"https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001"` and
  `"https://example.org/subjects/some/topic/name"` respectively, matching `export.Serialize`

### Requirement: Blank-node subjects and objects export as RDF blank nodes

`export.Serialize` and `export.SerializeToString` SHALL render subjects and objects (when `Triple.Datatype` is empty or
`message.EntityReferenceDatatype`) that match W3C Turtle / N-Triples `BLANK_NODE_LABEL` (`_:` followed by a valid
`PN_LOCAL` label) as RDF blank nodes (`_:label` in N-Triples and Turtle without angle brackets or prefix compaction,
and `{"@id": "_:label"}` in JSON-LD).

#### Scenario: Unfolded reified BFO/CCO blank-node chain serializes cleanly

- **WHEN** `export.SerializeToString` is called in `NTriples`, `Turtle`, and `JSONLD` with a 3-triple reified
  measurement chain connecting entity `"acme.ops.robotics.gcs.drone.001"` to blank node `"_:b_1"` via full-IRI
  predicate `"http://purl.obolibrary.org/obo/RO_0000056"`, typing `"_:b_1"` with `rdf:type`
  `"http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring"`, and attaching literal `85.5` via
  `"http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value"`
- **THEN** `"_:b_1"` is serialized as a blank node in both subject and object position (never wrapped in `<...>` or
  quotes), and the unregistered `http://` predicates are emitted as full or prefix-compacted IRIs (never rewritten
  under `/predicates/`)

### Requirement: Malformed blank-node strings never export as raw blank-node tokens

`export.Serialize` and `export.SerializeToString` SHALL fall back to `SubjectIRI` or literal serialization for
malformed `"_:"` strings or explicit non-`@id` datatypes rather than emitting invalid raw blank-node tokens.

#### Scenario: Malformed blank-node labels and explicit literal datatypes

- **WHEN** `export.SerializeToString` is called with malformed blank-node strings (`"_:"`, `"_:bad."`, `"_:has space"`)
  or with a valid blank-node string carrying `Datatype: "xsd:string"`
- **THEN** it never emits a raw unquoted `_:` blank-node token for those values, rendering malformed object strings and
  `xsd:string`-typed objects as quoted string literals and malformed subject strings via `SubjectIRI`

### Requirement: Unregistered IRI predicates export unchanged

`export.Serialize` and `export.SerializeToString` SHALL pass through unregistered `Triple.Predicate` values that are
valid `http://` or `https://` IRIs unchanged across N-Triples, Turtle, and JSON-LD.

#### Scenario: Unregistered IRI predicate passes through

- **WHEN** a triple whose `Predicate` is an unregistered `http://` or `https://` IRI is serialized as `NTriples`,
  `Turtle`, and `JSONLD`
- **THEN** the predicate IRI appears unchanged in each output, never rewritten under `/predicates/`

### Requirement: One predicate parser, with no shared state

`vocabulary.ParsePredicate` SHALL accept a predicate exactly when it has three segments separated by `.`, each segment
matches `[a-z][a-z0-9]*(-[a-z0-9]+)*` and is at most `MaxPredicateSegmentBytes` (64) bytes, and the whole is at most
`MaxPredicateBytes` (194) bytes (pin ADR-074). It SHALL return the three segments as `PredicateParts`.

#### Scenario: Agreement with the grammar

- **WHEN** any string is parsed
- **THEN** it is accepted exactly when it matches the grammar above, with the same parts

### Requirement: Predicate refusals name the first fault

`vocabulary.ParsePredicate` SHALL refuse a predicate with a `*PredicateValidationError` naming the first fault in this
order: empty, length, arity, then each segment from the first, checked for empty, length, first byte, then each later
byte (character or hyphen).

#### Scenario: A refusal carries the first fault

- **WHEN** a string that breaks the grammar is parsed
- **THEN** the error carries the reason and segment of the first fault in the order above

### Requirement: The predicate parser keeps no state and is the only whole-predicate check

`vocabulary.ParsePredicate` SHALL keep no state between calls and SHALL NOT allocate when it accepts a predicate. Every
check in this repository of a whole predicate's syntax SHALL call it; a namespace or entity-type key parser shares its
segment rule.

#### Scenario: A predicate never seen before

- **WHEN** a valid predicate not parsed before in the process is parsed
- **THEN** the parse returns its three parts and makes no heap allocation
