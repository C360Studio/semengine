package graph

import (
	"testing"

	"github.com/c360studio/semengine/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPropertyValue(t *testing.T) {
	// Create test entity with property and relationship triples
	entity := &EntityState{
		ID: "c360.platform.domain.system.test.entity",
		Triples: []message.Triple{
			{
				Subject:   "test.fixture.graph.helpers.entity.001",
				Predicate: "robotics.battery.level",
				Object:    75.5,
			},
			{
				Subject:   "test.fixture.graph.helpers.entity.001",
				Predicate: "robotics.battery.voltage",
				Object:    12.6,
			},
			{
				Subject:   "test.fixture.graph.helpers.entity.001",
				Predicate: "graph.relation.connected-to",          // Relationship predicate
				Object:    "c360.platform1.robotics.mav1.drone.0", // Valid 6-part EntityID
			},
		},
	}

	tests := []struct {
		name      string
		entity    *EntityState
		predicate string
		wantValue any
		wantFound bool
	}{
		{
			name:      "existing property",
			entity:    entity,
			predicate: "robotics.battery.level",
			wantValue: 75.5,
			wantFound: true,
		},
		{
			name:      "existing property voltage",
			entity:    entity,
			predicate: "robotics.battery.voltage",
			wantValue: 12.6,
			wantFound: true,
		},
		{
			name:      "relationship should not be found",
			entity:    entity,
			predicate: "graph.relation.connected-to",
			wantValue: nil,
			wantFound: false,
		},
		{
			name:      "non-existing property",
			entity:    entity,
			predicate: "non.existing.property",
			wantValue: nil,
			wantFound: false,
		},
		{
			name:      "nil entity",
			entity:    nil,
			predicate: "test.fixture.property",
			wantValue: nil,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := GetPropertyValue(tt.entity, tt.predicate)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantValue, value)
		})
	}
}

// The pin's MergeTriples cases, for statements with one (empty) source: the
// replace rule keeps them.
func TestReplaceBySourceKeepsMergeCases(t *testing.T) {
	t.Run("newer overrides existing property", func(t *testing.T) {
		existing := []message.Triple{
			{
				Subject:   "test.fixture.graph.helpers.entity.001",
				Predicate: "robotics.battery.level",
				Object:    70.0, // Old value
			},
			{
				Subject:   "test.fixture.graph.helpers.entity.001",
				Predicate: "robotics.flight.armed",
				Object:    false, // Old value
			},
		}

		newer := []message.Triple{
			{
				Subject:   "test.fixture.graph.helpers.entity.001",
				Predicate: "robotics.battery.level",
				Object:    85.0, // New value should win
			},
		}

		merged := ReplaceBySource(existing, newer).Triples

		// Should have both properties
		require.Len(t, merged, 2)

		// Newer battery level should win
		batteryLevel, found := findTripleByPredicate(merged, "robotics.battery.level")
		assert.True(t, found)
		assert.Equal(t, 85.0, batteryLevel.Object)

		// Existing armed state should remain
		armedState, found := findTripleByPredicate(merged, "robotics.flight.armed")
		assert.True(t, found)
		assert.Equal(t, false, armedState.Object)
	})

	t.Run("empty slices", func(t *testing.T) {
		result := ReplaceBySource(nil, nil).Triples
		assert.Nil(t, result)

		existing := []message.Triple{{Subject: "test.fixture.graph.helpers.entity.001", Predicate: "test.fixture.property", Object: "value"}}
		result = ReplaceBySource(existing, nil).Triples
		assert.Equal(t, existing, result)

		newer := []message.Triple{{Subject: "test.fixture.graph.helpers.entity.001", Predicate: "test.fixture.property", Object: "new"}}
		result = ReplaceBySource(nil, newer).Triples
		assert.Equal(t, newer, result)
	})

	t.Run("no conflicts", func(t *testing.T) {
		existing := []message.Triple{
			{Subject: "test.fixture.graph.helpers.entity.001", Predicate: "test.fixture.property1", Object: "value1"},
		}
		newer := []message.Triple{
			{Subject: "test.fixture.graph.helpers.entity.001", Predicate: "test.fixture.property2", Object: "value2"},
		}

		merged := ReplaceBySource(existing, newer).Triples
		assert.Len(t, merged, 2)

		// Should contain both
		_, found1 := findTripleByPredicate(merged, "test.fixture.property1")
		_, found2 := findTripleByPredicate(merged, "test.fixture.property2")
		assert.True(t, found1)
		assert.True(t, found2)
	})
}

// Helper function for tests
func findTripleByPredicate(triples []message.Triple, predicate string) (message.Triple, bool) {
	for _, triple := range triples {
		if triple.Predicate == predicate {
			return triple, true
		}
	}
	return message.Triple{}, false
}
