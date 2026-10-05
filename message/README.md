# Message Package

Polymorphic message handling for SemStreams data flows with behavior-based processing and type-safe payload management.

## Overview

The message package provides the foundation for type-safe, extensible message processing in SemStreams. Messages flow
through components as polymorphic envelopes (BaseMessage) containing typed payloads that can be type-asserted to
behavioral interfaces for specialized processing.

Key design principles:

- **Polymorphic by default**: A `Decoder` deserializes messages to the concrete payload types its registry holds
- **Behavior-based processing**: Type-assert to capabilities (RuleReadable, Storable, etc.) not concrete types
- **Registry-driven**: A payload registry, created by the application and passed to `message.NewDecoder`, adds
  payload types without changing this package
- **Type-safe**: Compile-time type checking with runtime polymorphism
- **Immutable**: Messages are read-only after creation (safe for concurrent access)

## Installation

```go
import "github.com/c360studio/semengine/message"
```

## Core Concepts

### Message Structure

Every SemStreams message consists of:

1. **Envelope** (`BaseMessage`): Polymorphic wrapper with type metadata
2. **Payload**: Concrete data implementing the Payload interface
3. **Behavioral Interfaces**: Optional capabilities for specialized processing

```text
┌─ BaseMessage ────────────────────────────┐
│ Type: "core.json.v1"                     │
│ PayloadData: <raw JSON>                  │
│ ├─ payloadInstance ─────────────────┐    │
│ │  GenericJSONPayload               │    │
│ │  ├─ Data: map[string]any          │    │
│ │  ├─ Implements: Payload           │    │
│ │  └─ Optionally: RuleReadable,     │    │
│ │                 Storable, etc.    │    │
│ └───────────────────────────────────┘    │
└──────────────────────────────────────────┘
```

### Type System

Payloads are identified by "domain.category.version" strings:

- `core.json.v1` - Generic JSON (built-in SemStreams type)
- `robotics.mavlink.v1` - MAVLink messages (domain-specific)
- `iot.sensor.v2` - IoT sensor data (domain-specific)

Components declare accepted types in their configuration, and the flow engine validates compatibility before deployment.

### Behavioral Interfaces

Optional capabilities that payloads can implement:

| Interface | Purpose | Methods | Use Case |
|-----------|---------|---------|----------|
| **IndexingProfiler** | Indexing profile hint | `IndexingProfile() string` | Embedding and clustering eligibility |
| **RuleReadable** | Fields rules may read | `RuleFields() map[string]any` | Rule conditions and substitutions |
| **Storable** | Entity with a storage reference | `EntityID()`, `Triples()`, `StorageRef()` | Graph storage of large payloads |

Processors type-assert to the specific behaviors they need, making components reusable across different payload types.

## Quick Start

### Creating Messages

```go
// Create a GenericJSON payload
payload := message.NewGenericJSON(map[string]any{
    "sensor_id": "temp-001",
    "temperature": 23.5,
    "unit": "celsius",
})

// Wrap in BaseMessage: type, payload, source
msg := message.NewBaseMessage(payload.Schema(), payload, "temperature-monitor")

// Serialize to JSON
data, err := json.Marshal(msg)
// Result: {"id":"…","type":{"domain":"core","category":"json","version":"v1"},
//          "payload":{"data":{"sensor_id":"temp-001"…}},
//          "meta":{"created_at":…,"received_at":…,"source":"temperature-monitor"}}
```

### Deserializing Messages

```go
// A Decoder resolves the "type" field against the registry it was built
// with; there is no global registry. Build the registry once at startup.
reg := payloadregistry.New()
if err := message.RegisterPayloads(reg); err != nil { // core.json.v1
    return err
}
decoder := message.NewDecoder(reg)

msg, err := decoder.Decode(data)
if err != nil {
    return err // malformed JSON, unregistered type, or a payload that does not fit it
}

// json.Unmarshal into a zero-value message.BaseMessage always fails: it has
// no registry to resolve the payload type.

// Access the payload
payload := msg.Payload()
if genericJSON, ok := payload.(*message.GenericJSONPayload); ok {
    // A decoded number is a json.Number holding its literal, so no integer is rounded.
    temperature, err := genericJSON.Data["temperature"].(json.Number).Float64()
    if err != nil {
        return err
    }
    fmt.Printf("Temperature: %.1f°C\n", temperature)
}
```

### Using Behavioral Interfaces

```go
// Check if payload declares the fields rules may read
if readable, ok := msg.Payload().(message.RuleReadable); ok {
    fields := readable.RuleFields()
    fmt.Printf("Rule fields: %v\n", fields)
}
```

## Architecture

### Message Flow

```text
┌─────────────┐     ┌────────────────┐     ┌─────────────┐     ┌─────────────┐
│   Input     │────▶│   Processor    │────▶│  Processor  │────▶│   Output    │
│ (UDP/File)  │     │  (Filter/Map)  │     │(Transform)  │     │(File/WS)    │
└─────────────┘     └────────────────┘     └─────────────┘     └─────────────┘
      │                     │                      │                    │
      ▼                     ▼                      ▼                    ▼
 BaseMessage          BaseMessage            BaseMessage          BaseMessage
  (Payload)            (Payload)              (Payload)            (Payload)
```

Each component:

1. Receives `BaseMessage` from NATS
2. Type-checks the payload type
3. Type-asserts to required behavioral interfaces
4. Processes the payload
5. Creates new `BaseMessage` with transformed payload
6. Publishes to next component

### Payload Registry

There is no global registry. The application creates a `payloadregistry.Registry` (`payloadregistry.New()`),
registers its payload types on it, and builds a `message.Decoder` from it. The registry maps type identifiers to
factory functions:

```go
Registry: {
  "core.json.v1" → func() any { return &GenericJSONPayload{} }
  "robotics.position.v1" → func() any { return &PositionPayload{} }
  "iot.sensor.v2" → func() any { return &SensorPayload{} }
}
```

When `Decoder.Decode` reads a message whose type is `core.json.v1`, it:

1. Looks up `core.json.v1` in the decoder's registry, and fails if it is not registered
2. Calls the factory to create an empty payload instance
3. Unmarshals the payload JSON into that instance, and fails if it does not fit
4. Returns a `*BaseMessage` holding the typed payload

This enables polymorphic deserialization without reflection or code generation.

### Integration Points

**NATS**: Messages serialize to JSON for NATS pub/sub:

- Subjects: `sensors.temperature`, `robots.position`, etc.
- Payload: Complete `BaseMessage` JSON

**Component Registry**: Components declare type compatibility:

- `input_types`: ["core.json.v1", "iot.sensor.v2"]
- `output_types`: ["core.json.v1"]

**Flow Engine**: Validates type compatibility in flow graphs before deployment

**Metrics**: Message counts tracked by type (via `Schema().String()`)

## Usage Patterns

### Creating Custom Payload Types

```go
package myapp

import (
    "time"
    "github.com/c360studio/semengine/component"
    "github.com/c360studio/semengine/message"
)

// Define your payload
type RobotPositionPayload struct {
    RobotID   string    `json:"robot_id"`
    Latitude  float64   `json:"latitude"`
    Longitude float64   `json:"longitude"`
    Timestamp time.Time `json:"timestamp"`
}

// Implement Payload interface
// PayloadType() is not part of the Payload interface
// Use Schema().String() to get the type identifier
// Schema() method is implemented above

func (p *RobotPositionPayload) Validate() error {
    // Structural validation: required fields
    if p.RobotID == "" {
        return errors.WrapInvalid(errors.ErrInvalidData, "RobotPositionPayload", "Validate", "robot_id is required")
    }

    // Semantic validation: value ranges
    if p.Latitude < -90 || p.Latitude > 90 {
        return errors.WrapInvalid(errors.ErrInvalidData, "RobotPositionPayload", "Validate",
            fmt.Sprintf("latitude must be between -90 and 90, got: %.6f", p.Latitude))
    }
    if p.Longitude < -180 || p.Longitude > 180 {
        return errors.WrapInvalid(errors.ErrInvalidData, "RobotPositionPayload", "Validate",
            fmt.Sprintf("longitude must be between -180 and 180, got: %.6f", p.Longitude))
    }
    return nil
}

// Implement behavioral interfaces
func (p *RobotPositionPayload) EntityID() string {
    return p.RobotID
}

func (p *RobotPositionPayload) EntityType() string {
    return "robot"
}

// RegisterPayloads registers the type with the application's registry, the
// pattern message.RegisterPayloads follows; the application calls it at startup
// on the registry it passes to message.NewDecoder.
func RegisterPayloads(reg *payloadregistry.Registry) error {
    return reg.Register(&payloadregistry.Registration{
        Domain:      "robotics",
        Category:    "position",
        Version:     "v1",
        Description: "Robot geographic position with timestamp",
        Factory: func() any {
            return &RobotPositionPayload{}
        },
        Example: RobotPositionPayload{
            RobotID:   "robot-001",
            Latitude:  40.7128,
            Longitude: -74.0060,
            Timestamp: time.Now(),
        },
    })
}
```

### GenericJSON as the Fallback

`GenericJSONPayload` (`core.json.v1`) is the fallback for JSON whose shape is not known when the code is written:
outside input, or a transform the user configures. Code that builds a shape it knows registers a payload type
through the registry instead.

`Data` holds JSON-shaped values only: `map[string]any`, `[]any`, `string`, Go numbers, `json.Number`, `bool` and
`nil`, nested to any depth. Encoding refuses any other value (a struct, `time.Time`, a pointer, a typed map or
slice such as `map[string]string`) with an error naming its type and path, such as `data.a[2].b`. It also refuses a
string or key that is not valid UTF-8.

```go
// Quick prototype - no custom types needed
func createTestMessage() *message.BaseMessage {
    payload := message.NewGenericJSON(map[string]any{
        "test_id": "test-001",
        "status": "running",
        "metrics": map[string]any{
            "cpu": 45.2,
            "memory": 67.8,
        },
    })
    return message.NewBaseMessage(payload)
}

// Process in a flow
func processTestData(msg *message.BaseMessage) {
    if genericJSON, ok := msg.Payload().(*message.GenericJSONPayload); ok {
        metrics := genericJSON.Data["metrics"].(map[string]any)
        n, _ := metrics["cpu"].(json.Number) // a decoded number is a json.Number
        if cpu, err := n.Float64(); err == nil && cpu > 80.0 {
            // Alert high CPU
        }
    }
}
```

**When to use GenericJSON**:

- ✅ JSON from outside whose shape the code does not know
- ✅ User-configured transforms (filter, map) over such JSON
- ✅ Tests and prototypes

**When NOT to use GenericJSON**:

- ❌ Any shape the code builds from fields it knows: register a payload type
- ❌ Type-safe domain models
- ❌ Performance-critical paths (use typed payloads)

## Testing

### Testing Round-Trip Serialization

```go
func TestMessageRoundTrip(t *testing.T) {
    // Create original message
    original := message.NewGenericJSON(map[string]any{
        "test": "value",
        "number": 42,
    })
    originalMsg := message.NewBaseMessage(original.Schema(), original, "test")

    // Marshal to JSON
    data, err := json.Marshal(originalMsg)
    require.NoError(t, err)

    // Decode back through a registry holding core.json.v1
    reg := payloadregistry.New()
    require.NoError(t, message.RegisterPayloads(reg))
    reconstructed, err := message.NewDecoder(reg).Decode(data)
    require.NoError(t, err)

    // Verify type and payload
    assert.Equal(t, "core.json.v1", reconstructed.Type().Key())
    payload := reconstructed.Payload().(*message.GenericJSONPayload)
    assert.Equal(t, "value", payload.Data["test"])
    assert.Equal(t, json.Number("42"), payload.Data["number"])
}
```

## Implementation Notes

### Thread Safety

**Messages are immutable after creation**:

- `BaseMessage` fields set once during construction or unmarshaling
- Payload instances should be immutable or use copy-on-write
- Safe for concurrent reads from multiple goroutines
- **NOT safe** for concurrent modification

**The payload registry is thread-safe**:

- `payloadregistry.Registry` uses internal synchronization
- Safe to register types concurrently (at startup, before decoding)
- Safe to create payloads, and to share one `Decoder`, across goroutines

### Performance

**Polymorphic deserialization overhead**:

- Registry lookup: O(1) map access (~10ns)
- Type assertion: Zero-cost Go interface check
- JSON unmarshaling: Standard library performance

**For high-throughput scenarios** (>10K msg/sec):

- Use typed payloads (avoid `map[string]any`)
- Pool `BaseMessage` instances if needed
- Consider binary serialization (protobuf, msgpack)

**Benchmarks** (typical hardware):

```shell
BenchmarkBaseMessage_Marshal       1000000   1200 ns/op
BenchmarkBaseMessage_Unmarshal     800000    1500 ns/op
BenchmarkTypedPayload_Marshal      2000000    600 ns/op
BenchmarkTypedPayload_Unmarshal    1500000    900 ns/op
```

### Error Handling

Uses SemStreams error classification:

```go
import "github.com/c360studio/semengine/pkg/errs"

// Invalid input (malformed JSON, unknown type)
errors.WrapInvalid(err, "BaseMessage", "UnmarshalJSON", "validate type format")

// Programming errors (factory returns wrong type)
errors.WrapFatal(err, "BaseMessage", "UnmarshalJSON", "factory contract violation")

// General errors (JSON marshaling)
errors.Wrap(err, "BaseMessage", "MarshalJSON", "marshal payload")
```

Error classification enables retry logic and proper error handling in components.

### Security Considerations

**Input Validation**:

- Payload types validated against "domain.category.version" format
- Unknown types rejected (must be registered)
- Factory contract enforced (must return Payload instance)
- Payload-specific validation via `Validate()` method

**GenericJSON Limitations**:

- No built-in size limits (enforce at transport layer)
- No depth limits (enforce at transport layer)
- No schema validation (use typed payloads for strict schemas)

**Best Practices**:

- Validate payloads in components before processing
- Enforce message size limits at NATS/transport level
- Use typed payloads for production systems
- Implement `Validate()` thoroughly for custom payloads

## Common Patterns

### Enrichment Pattern

```go
// Add metadata to existing message
func enrichMessage(msg *message.BaseMessage) (*message.BaseMessage, error) {
    // Extract original data
    original := msg.Payload().(*message.GenericJSONPayload)

    // Create enriched payload
    enriched := message.NewGenericJSON(map[string]any{
        "original": original.Data,
        "enriched_at": time.Now().UTC().Format(time.RFC3339Nano), // time.Time is not JSON-shaped
        "enrichment_version": "v1",
    })

    return message.NewBaseMessage(enriched), nil
}
```

### Filtering Pattern

```go
// Filter messages based on behavioral capabilities
func filterStorableOnly(msgs []*message.BaseMessage) []*message.BaseMessage {
    var result []*message.BaseMessage
    for _, msg := range msgs {
        if _, ok := msg.Payload().(message.Storable); ok {
            result = append(result, msg)
        }
    }
    return result
}
```

## Migration from Legacy Systems

If migrating from string-based message types:

```go
// Old: Type in payload
type OldMessage struct {
    Type    string
    Payload json.RawMessage
}

// New: Use BaseMessage
// decoder is built from a registry holding the new payload types
func migrateOldMessage(decoder *message.Decoder, old *OldMessage) (*message.BaseMessage, error) {
    // Map old types to new type identifiers
    typeMap := map[string]message.Type{
        "temperature": {Domain: "iot", Category: "sensor", Version: "v1"},
        "position":    {Domain: "robotics", Category: "position", Version: "v1"},
    }

    newType, ok := typeMap[old.Type]
    if !ok {
        // Fall back to GenericJSON
        var data map[string]any
        if err := json.Unmarshal(old.Payload, &data); err != nil {
            return nil, err
        }
        payload := message.NewGenericJSON(data)
        return message.NewBaseMessage(payload.Schema(), payload, "migration"), nil
    }

    // Reconstruct as BaseMessage: the wire's type is an object, not a dotted string
    envelope, err := json.Marshal(map[string]any{"type": newType, "payload": old.Payload})
    if err != nil {
        return nil, err
    }
    return decoder.Decode(envelope)
}
```

## Troubleshooting

### "unregistered payload type" Error

**Problem**: `Decoder.Decode` fails with "unregistered payload type: X.Y.Z"

**Solution**: Register the payload type on the registry the decoder was built from, before decoding:

```go
reg := payloadregistry.New()
if err := reg.Register(&payloadregistry.Registration{
    Domain:   "X",
    Category: "Y",
    Version:  "Z",
    Factory:  func() any { return &MyPayload{} },
}); err != nil {
    return err
}
decoder := message.NewDecoder(reg)
```

### Type Assertion Fails

**Problem**: Type assertion returns `ok=false`

**Debugging**:

```go
// Check actual type
payload := msg.Payload()
fmt.Printf("Payload type: %T\n", payload)
fmt.Printf("Payload type ID: %s\n", payload.Schema().String())

// Check behavioral interface
if _, ok := payload.(message.Storable); ok {
    fmt.Println("Payload IS Storable")
} else {
    fmt.Println("Payload is NOT Storable")
    // Check what interfaces it does implement
}
```

### JSON Deserialization Fails

**Problem**: `Decoder.Decode` succeeds but payload fields are zero-valued

**Cause**: JSON field names don't match struct tags

**Solution**: Ensure struct tags match JSON field names:

```go
// Wrong
type MyPayload struct {
    Value string // Missing json tag
}

// Correct
type MyPayload struct {
    Value string `json:"value"`
}
```

## Examples

See `*_test.go` files for comprehensive examples:

- `base_message_test.go` - Round-trip serialization, behavioral interfaces
- `generic_json_test.go` - GenericJSON usage, nested structures
- `behaviors.go` - Behavioral interface definitions with documentation

## References

- [NATS Client](../natsclient/) - Message transport layer

## Contributing

When adding new behavioral interfaces:

1. Keep interfaces small (1-3 methods)
2. Document use cases clearly
3. Provide example implementations
4. Add tests demonstrating usage
5. Update this README with the new interface

When adding new payload types:

1. Implement `Payload` interface
2. Implement relevant behavioral interfaces
3. Add a `RegisterPayloads(reg *payloadregistry.Registry) error` function that registers it
4. Add comprehensive tests
5. Document type identifier format
