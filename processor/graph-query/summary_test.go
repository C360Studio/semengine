package graphquery

import "testing"

// TestEntityTypesRefusesAnIDThatDoesNotParse: the pin's aggregation skipped such an ID, which its own
// page check made unreachable. Here the page read refuses it first (TestSummaryRefusesABrokenPage), and
// no answer can silently undercount if that check ever moves.
func TestEntityTypesRefusesAnIDThatDoesNotParse(t *testing.T) {
	types, err := entityTypes([]string{"acme.dep1.src.git.commit.a1", "acme.dep1.src.git.commit.a3.extra"}, 2)
	if err == nil {
		t.Fatalf("entityTypes = %+v, want an error for the seven-part ID", types)
	}
}
