package inference

import (
	"context"

	"github.com/c360studio/semengine/message"
)

// TripleAdder adds one statement to an entity that already exists.
// HierarchyInference writes the inverse edges onto containers through it, as
// part of its EntityStore.
type TripleAdder interface {
	AddTriple(ctx context.Context, triple message.Triple) error
}
