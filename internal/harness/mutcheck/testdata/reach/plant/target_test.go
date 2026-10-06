package plant

import "testing"

func TestClamp(t *testing.T) {
	if got := Clamp(3); got != 3 {
		t.Fatalf("Clamp(3) = %d, want 3", got)
	}
}
