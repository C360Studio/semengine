package embedding

import "math"

// CosineSimilarity computes the cosine similarity between two vectors.
//
// Returns a value between -1 and 1, where:
//   - 1 means vectors are identical
//   - 0 means vectors are orthogonal (unrelated)
//   - -1 means vectors are opposite
//
// Formula: cos(θ) = (A · B) / (||A|| × ||B||)
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	if len(a) == 0 {
		return 0.0
	}

	var dotProduct float64
	var magnitudeA float64
	var magnitudeB float64

	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		magnitudeA += float64(a[i]) * float64(a[i])
		magnitudeB += float64(b[i]) * float64(b[i])
	}

	// Avoid division by zero
	if magnitudeA == 0.0 || magnitudeB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(magnitudeA) * math.Sqrt(magnitudeB))
}
