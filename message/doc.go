// Package message provides the core message infrastructure for the SemStreams platform.
// It defines interfaces and types for creating, validating, and processing messages
// that flow through the semantic event mesh.
//
// # Architecture
//
// The package follows a clean, domain-agnostic design with three core concepts:
//
// 1. Messages - Containers that combine typed payloads with metadata
// 2. Payloads - Domain-specific data that may implement behavioral interfaces
// 3. Metadata - Information about message lifecycle and origin
//
// # Message Structure
//
// Every message consists of:
//   - A unique ID for tracking and deduplication
//   - A structured Type (domain, category, version)
//   - A Payload containing the actual data
//   - Metadata about creation time, source, etc.
//   - A content-based hash for integrity
//
// # Behavioral Interfaces
//
// The message package uses runtime capability discovery through optional interfaces.
// Payloads implement only the interfaces relevant to their domain, and services
// discover these capabilities dynamically through type assertions.
//
// ## Primary Entity Interface
//
// Graphable: Declares entities and relationships for knowledge graph storage
//   - EntityID() string - Returns federated entity identifier
//   - Triples() []Triple - Returns semantic facts about the entity
//   - Use when: Payload represents entities that should be stored in the graph
//   - Example: Drone telemetry, sensor readings, IoT device states
//
// ## Rule Interfaces
//
// RuleReadable: Declares the fields rule conditions and `$message.*`
// substitutions may read from this payload
//   - RuleFields() map[string]any - Returns the payload's declared projection
//   - Use when: A rule should be able to condition or substitute on this
//     payload. WITHOUT it the rule engine cannot read the payload at all:
//     every `$message.*` condition against it is false forever, and the engine
//     reports the rule/payload-type pairing once rather than failing silently.
//   - Expose structural facts (ids, roles, outcomes, states, counts, routing);
//     withhold LLM-authored and user content and open caller-populated maps.
//     There is no reflective default, by design — a field reaches a rule only
//     because its author exposed it.
//   - Example: Loop outcome events, tool-call identity, approval decisions.
//     GenericJSONPayload implements it by returning its whole Data map.
//
// ## Runtime Discovery Pattern
//
// Services discover capabilities at runtime through type assertions:
//
//	// Check for entity data
//	if graphable, ok := msg.Payload().(Graphable); ok {
//	    entityID := graphable.EntityID()
//	    triples := graphable.Triples()
//	    // Store in knowledge graph, build relationships, etc.
//	}
//
//	// Check for rule-readable fields
//	if readable, ok := msg.Payload().(RuleReadable); ok {
//	    fields := readable.RuleFields()
//	    // Evaluate conditions, resolve $message.* substitutions, etc.
//	}
//
// This pattern enables services to process any message type without prior knowledge
// of the specific payload structure, discovering capabilities dynamically.
//
// # Type System Hierarchy
//
// The message package uses three related but distinct type representations,
// each serving a specific purpose in the semantic event mesh architecture.
// All three implement the Keyable interface, providing consistent dotted notation
// for NATS routing, storage keys, and entity identification.
//
// ## 1. Type (Message Schema) - "domain.category.version"
//
// Purpose: Identifies message schemas for routing, validation, and evolution.
// Format: Type{Domain: "sensors", Category: "gps", Version: "v1"} -> "sensors.gps.v1"
//
// Use When:
//   - Defining message schemas in payload implementations (Schema() method)
//   - Configuring component input/output types
//   - Routing messages through NATS subjects
//   - Versioning payload schemas for evolution
//
// Example:
//
//	type TemperaturePayload struct { /* fields */ }
//	func (t *TemperaturePayload) Schema() Type {
//	    return Type{Domain: "sensors", Category: "temperature", Version: "v1"}
//	}
//	// Used for: NATS subject "sensors.temperature.v1"
//
// ## 2. EntityType (Graph Classification) - "domain.type"
//
// Purpose: Classifies entities in the property graph for querying and analysis.
// Format: EntityType{Domain: "robotics", Type: "drone"} -> "robotics.drone"
//
// Use When:
//   - Extracting type from EntityID: entityID.EntityType()
//   - Querying the graph for entities by type
//   - Classifying nodes in the knowledge graph
//   - Building semantic relationships between entity types
//
// Example:
//
//	// EntityType is derived from EntityID
//	entityID := EntityID{
//	    Org: "c360", Platform: "platform1",
//	    System: "mav1", Domain: "robotics",
//	    Type: "drone", Instance: "1",
//	}
//	entityType := entityID.EntityType()  // Returns EntityType{Domain: "robotics", Type: "drone"}
//	// Used for: Graph queries like "find all robotics.drone entities"
//
// ## 3. EntityID (Federated Identity) - "org.platform.system.domain.type.instance"
//
// Purpose: Provides globally unique entity identifiers across federated platforms.
// Format: EntityID with 6 parts -> "c360.platform1.mav1.robotics.drone.1"
//
// Use When:
//   - Multi-platform deployments need unique entity identity
//   - Merging data from multiple sources with overlapping IDs
//   - Federating entities across organizational boundaries
//   - Building distributed knowledge graphs
//
// Example:
//
//	entityID := EntityID{
//	    Org:      "c360",      // Organization namespace (platform.org)
//	    Platform: "platform1", // Minting deployment authority (platform.id)
//	    System:   "mav1",      // Source that produced the entity
//	    Domain:   "robotics",  // Delegated taxonomy
//	    Type:     "drone",     // Entity type within the domain
//	    Instance: "1",         // Leaf identifier
//	}
//	// Key(): "c360.platform1.mav1.robotics.drone.1"
//	// Used for: Federated entity resolution and deduplication
//
// ## Type Relationships
//
// These types form a hierarchy:
//
//	EntityID (6 parts) contains → EntityType (2 parts)
//	  entityID.EntityType() returns EntityType{Domain: "robotics", Type: "drone"}
//
//	Type (3 parts) is independent → Used for message schemas, not entity identity
//
// All three implement Keyable:
//
//	type Keyable interface {
//	    Key() string  // Returns dotted notation for NATS subjects and storage
//	}
//
// ## Choosing the Right Type
//
// Ask yourself:
//
//  1. Defining a message schema? → Use Type (Schema() method)
//  2. Providing entity identity? → Use EntityID (Graphable.EntityID() method)
//  3. Extracting entity classification? → Use EntityType (derived from EntityID)
//
// Most payloads only need Type (for Schema()). Only implement Graphable if your
// payload represents entities that should be stored in the knowledge graph.
// EntityType is typically not constructed directly - it's extracted from EntityID
// using the EntityType() method when querying or classifying graph entities.
//
// # Usage Example
//
//	// Define a payload type
//	type TemperaturePayload struct {
//	    SensorID    string    `json:"sensor_id"`
//	    Temperature float64   `json:"temperature"`
//	    Unit        string    `json:"unit"`
//	    Timestamp   time.Time `json:"timestamp"`
//	}
//
//	// Implement required Payload interface
//	func (t *TemperaturePayload) Schema() Type {
//	    return Type{
//	        Domain:   "sensors",
//	        Category: "temperature",
//	        Version:  "v1",
//	    }
//	}
//
//	func (t *TemperaturePayload) Validate() error {
//	    if t.SensorID == "" {
//	        return errors.New("sensor ID required")
//	    }
//	    return nil
//	}
//
//	func (t *TemperaturePayload) MarshalJSON() ([]byte, error) {
//	    // Use alias to avoid infinite recursion
//	    type Alias TemperaturePayload
//	    return json.Marshal((*Alias)(t))
//	}
//
//	func (t *TemperaturePayload) UnmarshalJSON(data []byte) error {
//	    // Use alias to avoid infinite recursion
//	    type Alias TemperaturePayload
//	    return json.Unmarshal(data, (*Alias)(t))
//	}
//
//	// Implement optional behavioral interfaces
//	func (t *TemperaturePayload) Timestamp() time.Time {
//	    return t.Timestamp
//	}
//
//	// Create and use a message
//	payload := &TemperaturePayload{
//	    SensorID:    "temp-001",
//	    Temperature: 22.5,
//	    Unit:        "celsius",
//	    Timestamp:   time.Now(),
//	}
//
//	msg := NewBaseMessage(
//	    payload.Schema(),
//	    payload,
//	    "temperature-monitor",
//	)
//
//	// Services can discover capabilities
//	// Modern approach using Graphable (preferred)
//	if graphable, ok := msg.Payload().(Graphable); ok {
//	    entityID := graphable.EntityID()
//	    triples := graphable.Triples()
//	    fmt.Printf("Entity: %s with %d triples\n", entityID, len(triples))
//	    for _, triple := range triples {
//	        fmt.Printf("  %s: %v\n", triple.Predicate, triple.Object)
//	    }
//	}
//
// # Message Lifecycle
//
// Messages follow a predictable lifecycle from creation through processing:
//
// ## 1. Creation
//
// Messages are created using NewBaseMessage with optional configuration:
//
//	// Simple message with current timestamp
//	msg := NewBaseMessage(payload.Schema(), payload, "my-service")
//
//	// Historical data with specific timestamp
//	msg := NewBaseMessage(payload.Schema(), payload, "my-service",
//	    WithTime(historicalTime))
//
// Messages are immutable after creation - all fields are set during construction
// and cannot be modified. This ensures message integrity throughout processing.
//
// ## 2. Validation
//
// Validation happens at two levels:
//
// Structural Validation (Required):
//   - Message type must be valid (domain, category, version all present)
//   - Payload must not be nil
//   - Metadata must not be nil
//
// Payload Validation (Domain-specific):
//   - Implemented by Payload.Validate() method
//   - Checks domain-specific business rules
//   - May be structural (required fields) or semantic (value ranges)
//
// Example validation:
//
//	if err := msg.Validate(); err != nil {
//	    // Handle validation error
//	    // Check error type: Fatal, Transient, or Invalid
//	}
//
// ## 3. Serialization
//
// Messages are serialized to JSON for transmission over NATS:
//
//	// Serialize message
//	data, err := json.Marshal(msg)
//
//	// Deserialize message: the Decoder resolves the payload type against
//	// the registry it was built with
//	reg := payloadregistry.New()
//	if err := RegisterPayloads(reg); err != nil {
//	    return err
//	}
//	decoded, err := NewDecoder(reg).Decode(data)
//	if err != nil {
//	    return err
//	}
//
// Wire format preserves:
//   - Message ID for deduplication and tracking
//   - Type information for routing and schema validation
//   - Payload data using Payload.MarshalJSON()
//   - Metadata timestamps (millisecond precision) and source
//
// Note: there is no global registry. Deserialization goes through a Decoder
// built with a payloadregistry.Registry that holds the message's payload type;
// decoding a zero-value BaseMessage with json.Unmarshal always fails, because
// it has no registry. For generic JSON processing, register the well-known type
// "core.json.v1" (GenericJSONPayload) with RegisterPayloads.
//
// GenericJSONPayload is the fallback for JSON whose shape is not known when the
// code is written: outside input, or a transform the user configures. Code that
// builds a shape it knows registers a payload type instead. Its Data holds
// JSON-shaped values only (map[string]any, []any, string, Go numbers,
// json.Number, bool and nil); encoding refuses any other value with an error
// naming its type and path, and any string or key that is not valid UTF-8.
//
// ## 4. Transmission
//
// Messages are published to NATS subjects derived from their type:
//
//	subject := msg.Type().Key()  // e.g., "sensors.temperature.v1"
//	nc.Publish(subject, data)
//
// This enables:
//   - Type-based routing using NATS wildcards
//   - Version-specific processing
//   - Domain isolation and security policies
//
// ## 5. Processing
//
// Services receive and process messages based on their subscriptions:
//
//	// Subscribe to specific type
//	nc.Subscribe("sensors.temperature.v1", handler)
//
//	// Subscribe to all versions
//	nc.Subscribe("sensors.temperature.*", handler)
//
//	// Subscribe to entire domain
//	nc.Subscribe("sensors.>", handler)
//
// Services discover payload capabilities at runtime:
//
//	// decoder is built once at startup: NewDecoder(reg)
//	func handler(m *nats.Msg) {
//	    msg, err := decoder.Decode(m.Data)
//	    if err != nil {
//	        return // malformed, or a payload type reg does not hold
//	    }
//
//	    // Discover capabilities
//	    if graphable, ok := msg.Payload().(Graphable); ok {
//	        // Process entity data
//	    }
//	}
//
// # Best Practices
//
// ## Payload Implementation
//
// 1. Implement Required Methods
//   - Schema() Type - Return structured type information
//   - Validate() error - Perform domain-specific validation
//   - MarshalJSON() ([]byte, error) - Use alias pattern to avoid recursion
//   - UnmarshalJSON([]byte) error - Use alias pattern to avoid recursion
//
// 2. Implement Optional Interfaces Thoughtfully
//   - Only implement behavioral interfaces that make semantic sense
//   - Consider whether your payload truly represents an entity before implementing Graphable
//
// 3. Validation Philosophy
//   - Structural validation: Check required fields and basic types
//   - Semantic validation: Check business rules and value ranges (optional)
//   - Fail fast: Return first validation error encountered
//   - Provide clear error messages with context
//
// 4. Use Type Constants
//
//   - Define message types as package constants
//
//   - Don't inline type construction at call sites
//
//   - Example:
//
//     // Good: Type as constant
//     var TemperatureType = message.Type{
//     Domain: "sensors", Category: "temperature", Version: "v1",
//     }
//
//     // Bad: Inline type construction
//     msg := NewBaseMessage(
//     message.Type{Domain: "sensors", Category: "temperature", Version: "v1"},
//     payload, source)
//
// ## Message Creation
//
// 1. Use Functional Options for Configuration
//   - Start with simple NewBaseMessage(type, payload, source)
//   - Add options only when needed: WithTime(), WithMeta()
//   - Don't create custom constructors - use options instead
//
// 2. Set Source Meaningfully
//   - Use service name or component identifier
//   - Be consistent across your application
//   - Example: "temperature-monitor", "gps-processor", "user-service"
//
// 3. Timestamp Considerations
//   - Default timestamp (time.Now()) is correct for most cases
//   - Use WithTime() only for historical data or testing
//
// ## Message Processing
//
// 1. Type Assertion Pattern
//
//   - Use comma-ok idiom for capability discovery
//
//   - Don't assume interfaces are implemented
//
//   - Example:
//
//     if graphable, ok := msg.Payload().(Graphable); ok {
//     // Safe to use graphable methods
//     } else {
//     // Payload doesn't implement Graphable, skip or handle differently
//     }
//
// 2. Error Handling
//   - Always check validation errors before processing
//   - Use errors.As() to detect error types (Fatal, Transient, Invalid)
//   - Don't rely on error message strings for logic
//
// 3. Immutability
//   - Don't modify message fields after creation
//   - Create new messages for transformations
//   - Payload data can be read but not written
//
// ## Testing
//
// 1. Use Real Test Data
//   - Create actual payload instances, don't use mocks
//   - Test with both valid and invalid data
//   - Test serialization round-trips
//
// 2. Test Behavioral Interfaces
//   - Verify interface implementations return correct values
//   - Test type assertions work as expected
//   - Ensure optional interfaces are truly optional
//
// 3. Test Validation
//   - Test both valid and invalid payloads
//   - Verify error messages are clear
//   - Test edge cases and boundary conditions
//
// # Design Philosophy
//
// The messages package embodies several key design principles:
//
// 1. Separation of Concerns: Messages know nothing about routing, storage, or
// entity extraction. They are pure data containers.
//
// 2. Discoverability: Behavioral interfaces allow services to discover and utilize
// message capabilities at runtime without tight coupling.
//
// 3. Domain Agnosticism: The core package contains no domain-specific code.
// All domain logic lives in domain packages that use these interfaces.
//
// 4. Extensibility: New behavioral interfaces can be added without breaking
// existing code. Payloads can implement any combination of interfaces.
//
// 5. Type Safety: Structured Type provides compile-time safety while
// maintaining flexibility.
package message
