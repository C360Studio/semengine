package embedding

import (
	"math"
	"testing"
)

// TestCosineSimilarity checks CosineSimilarity against values worked by hand from
// cos(θ) = (A · B) / (||A|| × ||B||), and its documented 0 for vectors it cannot compare.
// The pin had no test of vector.go; its BM25 tests only call it.
func TestCosineSimilarity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 2, 3}, []float32{1, 2, 3}, 1},
		{"same direction, other length", []float32{1, 2, 3}, []float32{2, 4, 6}, 1},
		{"orthogonal", []float32{1, 0}, []float32{0, 5}, 0},
		{"opposite", []float32{1, -2}, []float32{-1, 2}, -1},
		// (3·4 + 4·3) / (5 × 5) = 24/25.
		{"worked value", []float32{3, 4}, []float32{4, 3}, 0.96},
		{"length mismatch", []float32{1, 2}, []float32{1, 2, 3}, 0},
		{"empty", []float32{}, []float32{}, 0},
		{"zero vector", []float32{0, 0}, []float32{1, 1}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CosineSimilarity(tt.a, tt.b); math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("CosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
