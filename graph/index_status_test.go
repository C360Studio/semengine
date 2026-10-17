package graph

import (
	"slices"
	"testing"
)

// TestIndexStatesReturnsACopy holds design D6: IndexStates lists the four wire
// states of the readiness envelope, and a caller that changes the slice it got
// back cannot change what the next caller gets. The readiness gate's allow-list
// and the one-hot state gauge both read this list.
func TestIndexStatesReturnsACopy(t *testing.T) {
	want := []IndexState{"building", "ready", "degraded", "reset_required"}

	first := IndexStates()
	if !slices.Equal(first, want) {
		t.Fatalf("IndexStates() = %q, want %q", first, want)
	}

	first[0] = "tampered"
	_ = append(first[:1], "appended")

	if second := IndexStates(); !slices.Equal(second, want) {
		t.Fatalf("IndexStates() after a caller changed its copy = %q, want %q", second, want)
	}
}
