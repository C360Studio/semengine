package plant

import "testing"

func TestLabel(t *testing.T) {
	hits = 0
	Lab2(-1)
	if hits != 2 {
		t.Fatalf("hits = %d after Lab2(-1), want 2", hits)
	}
}
