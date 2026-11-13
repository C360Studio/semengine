package graphquery

import "testing"

// TestEntityTypesRefusesAnIDThatDoesNotParse: the aggregation refuses such an ID rather than skip
// it. The page read refuses it first (TestSummaryRefusesABrokenPage); refusing here too means no
// answer can silently undercount if that check ever moves.
func TestEntityTypesRefusesAnIDThatDoesNotParse(t *testing.T) {
	types, err := entityTypes([]string{"acme.dep1.src.git.commit.a1", "acme.dep1.src.git.commit.a3.extra"}, 2)
	if err == nil {
		t.Fatalf("entityTypes = %+v, want an error for the seven-part ID", types)
	}
}
