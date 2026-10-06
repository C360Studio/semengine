package plant

import "testing"

func TestInside(t *testing.T) {
	if got := Pick(5); got != 1 {
		t.Fatalf("Pick(5) = %d, want 1", got)
	}
	if got := Lab(-1); got != -1 {
		t.Fatalf("Lab(-1) = %d, want -1", got)
	}
}
