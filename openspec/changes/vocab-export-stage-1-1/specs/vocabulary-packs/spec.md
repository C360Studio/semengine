# vocabulary-packs Specification

## MODIFIED Requirements

### Requirement: Reification and projection path metadata on predicates

`vocabulary.ProjectionSpec.Validate()` and `vocabulary.ValidateProjectionSpec(spec ProjectionSpec)` SHALL validate a
`ProjectionSpec` without mutating global registry state, refusing an empty `Steps` slice, any step whose `PredicateIRI`
is not a valid `http://` or `https://` IRI, any step whose non-empty `NodeClassIRI` is not a valid `http://` or
`https://` IRI, any intermediate step with an empty `NodeClassIRI` or non-empty `ValuePredicateIRI`, and any terminal
step whose `ValuePredicateIRI` is empty or not a valid `http://` or `https://` IRI.
`vocabulary.WithProjection(spec ProjectionSpec)` SHALL attach an ordered slice of `ProjectionStep` entries to
`PredicateMetadata.Projection`, panicking at registration when `spec.Validate()` returns an error.
`GetPredicateMetadata` SHALL return a deep copy of `Projection`, and amending a predicate without `WithProjection`
SHALL preserve its existing `Projection`.

#### Scenario: Projection metadata survives registration and amend

- **WHEN** a predicate is registered with a two-step `ProjectionSpec` (linking an entity to a `cco:ActOfMeasuring`
  node and a literal value predicate) and then amended with `WithDescription`
- **THEN** `GetPredicateMetadata` returns a deep copy of the two-step `ProjectionSpec` along with the updated
  description

#### Scenario: Pure ProjectionSpec validation rejects invalid step IRIs

- **WHEN** `ProjectionSpec.Validate()` or `ValidateProjectionSpec` is called on a `ProjectionSpec` whose terminal step
  omits `ValuePredicateIRI` or whose step IRI does not use `http://` or `https://`
- **THEN** it returns a non-nil error without mutating the predicate or entity-type registries

### Requirement: Declarative JSON and YAML vocabulary packs load and register fail-closed

`vocabulary.LoadPackJSON` and `vocabulary.LoadPackYAML` SHALL decode a `PackSpec` from JSON or YAML while refusing
unknown fields, malformed syntax, or trailing data. `vocabulary.PackSpec.Validate()` and
`vocabulary.ValidatePack(pack *PackSpec)` SHALL validate a `PackSpec` without mutating global registry state, refusing a
`nil` pack, an empty or whitespace-only `Name`, any invalid or duplicate `PrefixSpec`, any duplicate or malformed
`PredicateSpec` (including a non-zero `alias_priority` when neither `is_alias` nor `alias_type` is set, an invalid
`ProjectionSpec`, or an inconsistent intra-pack `inverse_of` declaration), and any duplicate or malformed
`EntityTypeSpec`. `vocabulary.RegisterPack` SHALL validate the `PackSpec` via `PackSpec.Validate()` and against existing
registered inverses before mutating registry state; if any entry or batch prefix registration is invalid, `RegisterPack`
SHALL return an error and leave the vocabulary and prefix registries unchanged.

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

#### Scenario: Pure PackSpec validation mutates no global state

- **WHEN** `PackSpec.Validate()` or `ValidatePack` is called on valid and invalid `PackSpec` values
- **THEN** it returns `nil` for valid packs and an error for invalid packs while `ListRegisteredPredicates()`,
  `ListRegisteredEntityTypes()`, and `export`'s prefix table remain unchanged

## ADDED Requirements

### Requirement: Exported SubjectIRI and blank-node / IRI-predicate serialization

`export.SubjectIRI(subject, base string)` SHALL normalize `base` by trimming trailing `/` characters and defaulting an
empty `base` to `vocabulary.SemEngineBase`, returning `{base}/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}`
when `subject` is a valid 6-part canonical entity ID and `{base}/subjects/{dots->slashes}` otherwise (or `""` when
`subject` is empty). `export.Serialize` and `export.SerializeToString` SHALL use `export.SubjectIRI` for default
non-blank-node subject IRI resolution, SHALL emit W3C `BLANK_NODE_LABEL` identifiers (`_:label`) in `Triple.Subject` and
string `Triple.Object` (when `Triple.Datatype` is empty or `"@id"`) as blank nodes (`_:label` in N-Triples and Turtle
without `<...>` wrapping or `SubjectIRIFunc` rewriting, and `"@id": "_:label"` in JSON-LD), and SHALL preserve
unregistered `http://` and `https://` `Triple.Predicate` IRIs unchanged instead of rewriting them under
`{base}/predicates/`.

#### Scenario: SubjectIRI matches default exporter subject resolution

- **WHEN** `export.SubjectIRI` and `export.SerializeToString` are called with a 6-part canonical entity ID and with a
  non-entity fallback subject under both the default base IRI and a custom base IRI with a trailing `/`
- **THEN** `export.SubjectIRI` returns the exact subject IRI emitted by `export.SerializeToString`

#### Scenario: Unfolded reified blank-node chain serializes in all formats

- **WHEN** an unfolded `ProjectionSpec` triple chain containing an entity subject, intermediate blank nodes (`_:b_1`,
  `_:b_2`) in `Triple.Subject` and `Triple.Object`, and full `http://` / `https://` predicate IRIs (`rdf:type`,
  `cco:is_measured_by`, `cco:has_decimal_value`) is serialized in `NTriples`, `Turtle`, and `JSONLD`
- **THEN** all three formats emit valid blank-node subjects and objects (`_:b_1`, `_:b_2`) and unmangled predicate IRIs
