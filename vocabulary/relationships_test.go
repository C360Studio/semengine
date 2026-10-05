package vocabulary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelationshipPredicates(t *testing.T) {
	tests := []struct {
		name             string
		predicate        string
		expectedDomain   string
		expectedCategory string
		expectedIRI      string
	}{
		{
			name:             "GraphRelContains",
			predicate:        GraphRelContains,
			expectedDomain:   "graph",
			expectedCategory: "rel",
			expectedIRI:      ProvHadMember,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify predicate is valid
			assert.True(t, IsValidPredicate(tt.predicate),
				"Predicate %s should be valid", tt.predicate)

			// Verify predicate is registered
			meta := GetPredicateMetadata(tt.predicate)
			require.NotNil(t, meta,
				"Predicate %s should be registered", tt.predicate)

			// Verify metadata
			assert.NotEmpty(t, meta.Description,
				"Predicate %s should have a description", tt.predicate)
			assert.Equal(t, tt.expectedDomain, meta.Domain,
				"Predicate %s should have domain %s", tt.predicate, tt.expectedDomain)
			assert.Equal(t, tt.expectedCategory, meta.Category,
				"Predicate %s should have category %s", tt.predicate, tt.expectedCategory)

			// Verify IRI mapping if expected
			if tt.expectedIRI != "" {
				assert.Equal(t, tt.expectedIRI, meta.StandardIRI,
					"Predicate %s should map to IRI %s", tt.predicate, tt.expectedIRI)
			}
		})
	}
}

func TestGetRelationshipPredicates(t *testing.T) {
	// Get all registered predicates and filter for graph.rel.*
	allPredicates := ListRegisteredPredicates() // predicate-audit:unrelated {"column":19,"surface":"go-assignment:allPredicates","value":"","basis":"reviewed:runtime-registry-output"}
	relPredicates := make([]string, 0)          // predicate-audit:unrelated {"column":19,"surface":"go-assignment:relPredicates","value":"","basis":"reviewed:runtime-filtered-predicate-collection"}
	for _, pred := range allPredicates {
		meta := GetPredicateMetadata(pred)
		if meta != nil && meta.Domain == "graph" && meta.Category == "rel" {
			relPredicates = append(relPredicates, pred) // predicate-audit:unrelated {"column":20,"surface":"go-assignment:relPredicates","value":"","basis":"reviewed:runtime-filtered-predicate-collection"}
		}
	}

	// graph.rel.contains is the one relationship predicate the framework
	// registers; the eleven others were left behind as dead surface.
	assert.Equal(t, []string{GraphRelContains}, relPredicates,
		"graph.rel.contains should be the only graph.rel.* predicate")

	// Verify specific predicates exist
	predicateMap := make(map[string]bool)
	for _, pred := range relPredicates {
		predicateMap[pred] = true
	}

	expectedPredicates := []string{
		GraphRelContains,
	}

	for _, pred := range expectedPredicates {
		assert.True(t, predicateMap[pred],
			"Predicate %s should be in graph.rel category", pred)
	}
}

func TestRelationshipIRIMappings(t *testing.T) {
	tests := []struct {
		predicate   string
		expectedIRI string
	}{
		{GraphRelContains, ProvHadMember},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			meta := GetPredicateMetadata(tt.predicate)
			require.NotNil(t, meta, "Predicate should be registered")

			assert.Equal(t, tt.expectedIRI, meta.StandardIRI,
				"Predicate %s should map to IRI %s", tt.predicate, tt.expectedIRI)
		})
	}
}

func TestRelationshipPredicateFormat(t *testing.T) {
	// All relationship predicates should follow graph.rel.* pattern
	allRelPredicates := []string{
		GraphRelContains,
	}

	for _, pred := range allRelPredicates {
		assert.Contains(t, pred, "graph.rel.",
			"Relationship predicate %s should start with graph.rel.", pred)
	}
}
