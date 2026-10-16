package message

import "github.com/c360studio/semengine/pkg/types"

// Type aliases for backwards compatibility.
// The canonical definitions are now in pkg/types.
// New code should import pkg/types directly.

// Type provides structured type information for messages.
// It enables type-safe routing and processing by clearly identifying
// the domain, category, and version of each message.
type Type = types.Type

// ParseEntityID delegates canonical parsing to pkg/types.
func ParseEntityID(value string) (types.EntityID, error) {
	return types.ParseEntityID(value)
}
