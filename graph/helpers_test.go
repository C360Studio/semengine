package graph

import (
	"testing"

	"github.com/c360studio/semengine/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
