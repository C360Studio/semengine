package types

// Keyable interface represents types that can be converted to semantic keys
// using dotted notation. This is the foundation of the unified semantic
// architecture enabling NATS wildcard queries and consistent storage patterns.
type Keyable interface {
	// Key returns the dotted notation representation of this semantic type.
	// Examples: "robotics.drone", "telemetry.robotics.drone.1", "robotics.battery.v1"
	Key() string
}
