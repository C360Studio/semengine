package plant

import "testing"

func TestBranches(t *testing.T) {
	if got := Sign(5); got != 1 {
		t.Fatalf("Sign(5) = %d, want 1", got)
	}
	if got := Name(2); got != "two" {
		t.Fatalf("Name(2) = %q, want two", got)
	}
}
