package types

import "fmt"

// EntityType represents a structured entity type identifier using dotted notation.
//
// Format: EntityType{Domain: "domain", Type: "type"} -> Key() returns "domain.type"
//
// Examples:
//   - EntityType{Domain: "robotics", Type: "drone"} -> "robotics.drone"
//   - EntityType{Domain: "robotics", Type: "battery"} -> "robotics.battery"
//   - EntityType{Domain: "sensors", Type: "gps"} -> "sensors.gps"
//
// EntityType is used by Identifiable payloads to specify their entity type,
// enabling entity extraction and property graph construction with consistent dotted keys.
type EntityType struct {
	// Domain identifies the business or system domain (lowercase)
	Domain string
	// Type identifies the specific entity type within the domain (lowercase)
	Type string
}

// Key returns the dotted notation representation: "domain.type"
// This implements the Keyable interface for unified semantic keys.
func (et EntityType) Key() string {
	return fmt.Sprintf("%s.%s", et.Domain, et.Type)
}

// String returns the same as Key() for backwards compatibility
func (et EntityType) String() string {
	return et.Key()
}

// IsValid checks if the EntityType has required fields populated
func (et EntityType) IsValid() bool {
	return et.Domain != "" && et.Type != ""
}
