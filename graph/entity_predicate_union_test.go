package graph

// A noncanonical predicate resident among valid ones (the resident-poison shape of the
// no-laundering matrix) is refused by the ENTITY_STATES contract seam and by the
// MarshalEntityState write gate; valid triples riding with it never launder it.

import (
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

func TestEntityStateContractRefusesNoncanonicalPredicateAmongValid(t *testing.T) {
	const entityID = "acme.ops.test.system.widget.001"
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	poisoned := &EntityState{ID: entityID, Triples: []message.Triple{
		{
			Subject: entityID, Predicate: semantictest.Predicate(t, "test", "state", "value"), Object: "v",
			Timestamp: now, Confidence: 1.0,
		},
		{
			Subject: entityID, Predicate: "Test.State.Value", Object: "v", // predicate-audit:invalid {"kind":"stored-predicate","value":"Test.State.Value","reason":"segment_start"}
			Timestamp: now, Confidence: 1.0,
		},
		{
			Subject: entityID, Predicate: semantictest.Predicate(t, "test", "state", "phase"), Object: "v",
			Timestamp: now, Confidence: 1.0,
		},
	}}

	err := ValidateEntityStateContract(poisoned)
	var predicateErr *EntityPredicateContractError
	if !errors.As(err, &predicateErr) {
		t.Fatalf("ValidateEntityStateContract() error = %T %v, want *EntityPredicateContractError", err, err)
	}
	if len(predicateErr.Violations) != 1 || predicateErr.Violations[0].Predicate != "Test.State.Value" {
		t.Fatalf("violations = %+v, want exactly the noncanonical predicate", predicateErr.Violations)
	}

	if _, err := MarshalEntityState(poisoned); err == nil {
		t.Fatal("MarshalEntityState serialized a noncanonical predicate among valid ones")
	}
}
