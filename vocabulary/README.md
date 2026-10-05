# Vocabulary Package

**Purpose**: Semantic vocabulary management with dotted notation predicates and optional IRI mappings for standards compliance.

## Design Philosophy: Pragmatic Semantic Web

The vocabulary package follows a pragmatic semantic web approach that balances clean internal architecture with customer
requirements for standards compliance.

### Core Principles

**Internal**: Always use dotted notation (domain.category.property)

- Clean, human-readable predicates
- NATS wildcard query support
- No URI/IRI complexity in internal code

**External**: Optional IRI mappings at API boundaries

- RDF/Turtle export with standard vocabularies
- OGC compliance (GeoSPARQL, SSN/SOSA)
- Integration with existing semantic systems

**No Leakage**: Standards complexity does NOT leak inward

- Internal code never sees IRIs
- Triples always use dotted predicates
- NATS subjects use dotted notation

## Predicate Structure

All predicates follow three-level dotted notation:

```text
domain.category.property
```

**Examples**:

- `sensor.temperature.celsius` - Sensor domain, temperature category
- `geo.location.latitude` - Geospatial domain, location category
- `time.lifecycle.created` - Temporal domain, lifecycle category
- `robotics.battery.level` - Robotics domain, battery category

**Naming Conventions**:

- **domain**: lowercase, business domain (sensors, geo, time, robotics)
- **category**: lowercase, groups related properties (temperature, location, lifecycle)
- **property**: lowercase, specific property name (celsius, latitude, created)
- No underscores or special characters (dots only for level separation)

**Why This Works**:

- ✅ NATS wildcards: `robotics.battery.*` finds all battery predicates
- ✅ Human-readable: Clear semantic meaning without URIs
- ✅ Consistent: Matches EntityID.Key() and Type.Key() patterns
- ✅ Simple: No namespace prefixes or IRI complexity

## Quick Start

### 1. Define Domain Predicates

```go
package robotics

const (
    BatteryLevel   = "robotics.battery.level"
    BatteryVoltage = "robotics.battery.voltage"
    FlightModeArmed = "robotics.flight.armed"
)
```

### 2. Register with Metadata

```go
func init() {
    vocabulary.Register(BatteryLevel,
        vocabulary.WithDescription("Battery charge level percentage"),
        vocabulary.WithDataType(vocabulary.DataTypeFloat),
        vocabulary.WithUnits("percent"),
        vocabulary.WithRange("0-100"),
        vocabulary.WithIRI("http://schema.org/batteryLevel"))

    vocabulary.Register(BatteryVoltage,
        vocabulary.WithDescription("Battery voltage"),
        vocabulary.WithDataType(vocabulary.DataTypeFloat),
        vocabulary.WithUnits("volts"))

    vocabulary.Register(FlightModeArmed,
        vocabulary.WithDescription("Flight mode armed status"),
        vocabulary.WithDataType(vocabulary.DataTypeBool))
}
```

### 3. Use in Messages

```go
// Create triples - always dotted notation
triple := message.Triple{
    Subject:   entityID,
    Predicate: robotics.BatteryLevel,  // "robotics.battery.level"
    Object:    85.5,
}

// NATS queries work naturally
nc.Subscribe("robotics.battery.*", handler)  // All battery predicates
```

## Functional Options API

The `Register()` function uses functional options for clean, composable configuration:

### Available Options

#### `WithDescription(desc string)`

Human-readable description of the predicate.

```go
vocabulary.Register("sensor.temperature.celsius",
    vocabulary.WithDescription("Temperature in degrees Celsius"))
```

#### `WithDataType(dataType string)`

The **pragmatic type** of the object value — one of seven canonical constants. Not the Go type of the value: the
serializer observes that directly, and the authoritative `ENTITY_STATES` JSON round trip erases it anyway (a declared
whole number arrives back as `float64` every time). Declare only what observation cannot recover.

```go
vocabulary.Register("sensor.temperature.celsius",
    vocabulary.WithDataType(vocabulary.DataTypeFloat))
```

One declaration renders three ways. The Go column is what the serializer typically sees, **not** what you declare;
the RDF column is owned by `vocabulary/export` (ADR-107 keeps semantic-web vocabulary at the export edge, never on
the declaration surface):

| Constant | Value | Typical Go value | RDF (Turtle / N-Triples) | JSON Schema |
|---|---|---|---|---|
| `DataTypeString` | `string` | `string` | `"text"` (`xsd:string`, omitted) | `{"type":"string"}` |
| `DataTypeEntityID` | `entity_id` | `string` (6-part ID) | `<iri>` — a resource, never a literal | `{"type":"string","format":"semstreams-entity-id"}` |
| `DataTypeInt` | `int` | `float64` after the round trip | `"5"^^xsd:integer` | `{"type":"integer"}` |
| `DataTypeFloat` | `float` | `float64` | `"5.5"^^xsd:double` | `{"type":"number"}` |
| `DataTypeBool` | `bool` | `bool` | `"true"^^xsd:boolean` | `{"type":"boolean"}` |
| `DataTypeDateTime` | `datetime` | `time.Time`, or RFC 3339 `string` | `"…Z"^^xsd:dateTime` | `{"type":"string","format":"date-time"}` |
| `DataTypeJSON` | `json` | `string` | `"…"^^rdf:JSON` | `{"type":"string","contentMediaType":"application/json"}` |

Declaring nothing is legal. **Anything else panics at registration**, naming the offending value and the accepted
vocabulary. The framework translates nothing — there is no legacy-spelling map, so `float64`, `number`, `double`,
`time.Time`, `timestamp`, `int64`, `array`, `entity_ref`, `reference` and `boolean` all halt the binary rather than
becoming their canonical equivalent. If you declare any of them today, see
[`docs/operations/migration-predicate-datatype.md`](../docs/operations/migration-predicate-datatype.md), which is
the whole migration.

Export honors the declaration, and **ignores it for any triple whose observed value contradicts it** — a fractional
value under an `int` declaration serializes as `xsd:double`, not as a rounded integer.

#### `WithUnits(units string)`

Measurement units, as free-form **documentation**. No framework path validates, normalizes, interprets, or honors it
— unlike `WithDataType`, which is honored at export.

```go
vocabulary.Register("sensor.temperature.celsius",
    vocabulary.WithUnits("celsius"))
```

#### `WithRange(valueRange string)`

Valid value ranges, as free-form **documentation**. `"0-100"`, `"-90 to 90"` and `"positive"` are three incompatible
grammars, which is why nothing parses it. No framework path validates, normalizes, interprets, or honors it.

```go
vocabulary.Register("robotics.battery.level",
    vocabulary.WithRange("0-100"))
```

#### `WithIRI(iri string)`

W3C/RDF equivalent IRI for standards compliance.

Use constants from `standards.go` for common vocabularies.

```go
vocabulary.Register("entity.label.preferred",
    vocabulary.WithIRI(vocabulary.DcTitle))
```

#### `WithAlias(aliasType, priority int)`

Mark as entity alias for resolution.

```go
vocabulary.Register("robotics.communication.callsign",
    vocabulary.WithAlias(vocabulary.AliasTypeCommunication, 0))  // Priority 0 = highest
```

### Complete Example

```go
vocabulary.Register("robotics.battery.level",
    vocabulary.WithDescription("Battery charge level percentage"),
    vocabulary.WithDataType(vocabulary.DataTypeFloat),
    vocabulary.WithUnits("percent"),
    vocabulary.WithRange("0-100"),
    vocabulary.WithIRI("http://schema.org/batteryLevel"))
```

## Alias Predicates for Entity Resolution

Some predicates represent entity aliases (identifiers, labels, call signs). The registry tracks these for entity
resolution and correlation.

### Alias Types

**AliasTypeIdentity** - Entity equivalence

- RDF equivalents: owl:sameAs, schema:sameAs
- Use for: Federated entity IDs, external system UUIDs
- Resolution: ✅ Can resolve to entity IDs

**AliasTypeAlternate** - Secondary unique identifiers

- RDF equivalents: schema:alternateName, dc:alternative
- Use for: Model numbers, registration IDs
- Resolution: ✅ Can resolve to entity IDs

**AliasTypeExternal** - External system identifiers

- Go constants: `vocabulary.DcIdentifier`
- RDF equivalents: dc:identifier, schema:identifier
- Use for: Manufacturer serial numbers, legacy system IDs
- Resolution: ✅ Can resolve to entity IDs

**AliasTypeCommunication** - Communication identifiers

- RDF equivalent: foaf:accountName
- Use for: Radio call signs, network hostnames, MQTT client IDs
- Resolution: ✅ Can resolve to entity IDs

**AliasTypeLabel** - Display names

- RDF equivalents: rdfs:label, skos:prefLabel, schema:name
- Use for: Human-readable display names
- Resolution: ❌ NOT for resolution (ambiguous - many entities share labels)

**Note**: The RDF notation shown (e.g., `owl:sameAs`) is shorthand for full IRIs. In Go code, always use the provided
constants from `standards.go` (e.g., `vocabulary.DcIdentifier`).

### Example: Registering Aliases

```go
// Communication identifier - highest priority
vocabulary.Register("robotics.communication.callsign",
    vocabulary.WithDescription("Radio call sign for ATC"),
    vocabulary.WithDataType(vocabulary.DataTypeString),
    vocabulary.WithAlias(vocabulary.AliasTypeCommunication, 0))

// External identifier
vocabulary.Register("robotics.identifier.serial",
    vocabulary.WithDescription("Manufacturer serial number"),
    vocabulary.WithDataType(vocabulary.DataTypeString),
    vocabulary.WithAlias(vocabulary.AliasTypeExternal, 1),
    vocabulary.WithIRI(vocabulary.DcIdentifier))

// Display label - NOT used for resolution
vocabulary.Register("entity.label.display",
    vocabulary.WithDescription("Human-readable display name"),
    vocabulary.WithDataType(vocabulary.DataTypeString),
    vocabulary.WithAlias(vocabulary.AliasTypeLabel, 10))  // Low priority
```

### Discovering Aliases

```go
// Get all alias predicates with priorities
aliases := vocabulary.DiscoverAliasPredicates()
// Returns: map["robotics.communication.callsign"]int(0), etc.
```

## Standards Mappings

Common standard vocabulary IRIs are provided in `standards.go`:

### SKOS (Simple Knowledge Organization System)

```go
const (
    SkosBroader  = "http://www.w3.org/2004/02/skos/core#broader"
    SkosNarrower = "http://www.w3.org/2004/02/skos/core#narrower"
)
```

### Dublin Core

```go
const (
    DcIdentifier = "http://purl.org/dc/terms/identifier"
    DcTitle      = "http://purl.org/dc/terms/title"
)
```

See `standards.go` for the complete list of standard vocabulary IRIs.

## Registry API

### Registration

```go
// Register with functional options
vocabulary.Register(name string, opts ...Option)
```

### Retrieval

```go
// Get metadata for a predicate
meta := vocabulary.GetPredicateMetadata("robotics.battery.level")
if meta != nil {
    fmt.Println(meta.Description)
    fmt.Println(meta.StandardIRI)
}

// List all registered predicates
predicates := vocabulary.ListRegisteredPredicates()

// Discover alias predicates
aliases := vocabulary.DiscoverAliasPredicates()
```

### Testing

```go
// Clear registry (testing only)
vocabulary.ClearRegistry()
```

## Internal vs External Usage

### Internal: Always Dotted Notation

```go
// Creating triples
triple := message.Triple{
    Subject:   "c360.platform1.robotics.drone.001",
    Predicate: "robotics.battery.level",  // Dotted, not IRI
    Object:    85.5,
}

// NATS subscriptions
nc.Subscribe("robotics.battery.*", handler)

// Entity properties
entityState.SetProperty("geo.location.latitude", 37.7749)

// NO IRIs in internal code!
```

## Best Practices

### Predicate Definition

1. **Use Package Constants**
   - Define predicates as package constants
   - Don't inline predicate strings
   - Group by domain and category

2. **Register in init()**
   - Register all domain predicates during package initialization
   - Use functional options for clarity
   - Include IRI mappings for customer-facing predicates

3. **Consistent Naming**
   - Follow `domain.category.property` strictly
   - Use lowercase throughout
   - Choose semantic, descriptive names

### IRI Mappings

1. **Only When Needed**
   - Map to standard IRIs for customer integrations
   - Skip IRI for internal-only predicates
   - Use well-known standards (Schema.org, OWL, SKOS)

2. **At API Boundaries**
   - Translate dotted → IRI in RDF export
   - Translate IRI → dotted in RDF import
   - Keep internal code IRI-free

3. **Document Mappings**
   - Explain why specific IRI chosen
   - Reference standard vocabulary documentation
   - Note any semantic differences

### Entity Resolution

1. **Mark Alias Predicates**
   - Use `WithAlias()` for identity-related predicates
   - Set appropriate priority for conflict resolution
   - Choose correct AliasType for semantics

2. **Avoid Label Confusion**
   - Don't use AliasTypeLabel for resolution
   - Labels are for display only (ambiguous)
   - Use identity/alternate/external for unique IDs

## Graph Domain Predicates

The vocabulary package provides standard relationship predicates for linking entities in the semantic graph. These
`graph.rel.*` predicates enable rich semantic relationships with mappings to standard vocabularies.

### Relationship Types

All relationship predicates follow the pattern `graph.rel.*` and are registered with PROV-O
mappings where applicable.

#### Hierarchical Relationships

```go
vocabulary.GraphRelContains     // Parent contains child
```

- `graph.rel.contains` → `prov:hadMember` - Hierarchical containment (platform contains sensors)

### Usage Example

```go
import "github.com/c360studio/semengine/vocabulary"

// Create containment relationship between platform and sensor
triple := message.Triple{
    Subject:   "platform-001",
    Predicate: vocabulary.GraphRelContains,  // "graph.rel.contains"
    Object:    "sensor-001",
}

// Query all relationships using NATS wildcards
nc.Subscribe("graph.rel.*", handler)  // All relationship types
nc.Subscribe("graph.rel.contains", handler)  // Only containment
```

### Standards Compliance

Relationship predicates map to established semantic web vocabularies:

- **PROV-O** - Provenance and membership relationships

See `relationships.go` for the complete registration and `standards.go` for IRI constants.

## Framework Predicates

The vocabulary package provides framework predicates in `predicates.go`.

Applications should define their own domain-specific vocabularies.

## Related Documentation

- `doc.go` - Comprehensive package documentation
- `standards.go` - Standard vocabulary IRI constants (SKOS, Dublin Core, PROV-O)
- `message/triple.go` - Triple structure for semantic facts
- `message/types.go` - EntityID, EntityType, Type patterns

The vocabulary package focuses on **predicate management**, not IRI generation. For entity-level IRI needs, use the
entity's own methods.
