package graphindex

import (
	"errors"

	"github.com/c360studio/semengine/graph"
)

// Predicate tokens in the INCOMING and NAME composite keys are hex-encoded through the
// shared graph.EncodePredicateToken codec, so every reverse-index reader decodes them the
// same way.
func encodePredicateToken(predicate string) string { return graph.EncodePredicateToken(predicate) }

// errIndexWritePartial is returned when one or more required index writes or retractions
// fail during an entity update. It propagates so readiness is withheld: an index that
// drops writes and reports itself caught up is the failure ADR-066 exists to prevent.
var errIndexWritePartial = errors.New("graph-index: one or more required index writes failed")
