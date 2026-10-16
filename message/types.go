package message

import "github.com/c360studio/semengine/pkg/types"

// Type provides structured type information for messages.
// It enables type-safe routing and processing by clearly identifying
// the domain, category, and version of each message.
// It is an alias: the definition lives in pkg/types, so payload code can say
// message.Type without importing that package.
type Type = types.Type

// ParseEntityID delegates canonical parsing to pkg/types.
func ParseEntityID(value string) (types.EntityID, error) {
	return types.ParseEntityID(value)
}
