package embedding

import (
	"math"
)

// CosineSimilarity computes the cosine similarity between two vectors
// Returns a value between -1 and 1, where 1 means identical direction
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct float64
	var normA float64
	var normB float64

	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// DotProduct computes the dot product of two vectors
func DotProduct(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		sum += float64(a[i]) * float64(b[i])
	}
	return float32(sum)
}

// Magnitude computes the magnitude (L2 norm) of a vector
func Magnitude(v []float32) float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return float32(math.Sqrt(sum))
}

// Normalize returns a unit vector in the same direction
func Normalize(v []float32) []float32 {
	mag := Magnitude(v)
	if mag == 0 {
		return v
	}

	result := make([]float32, len(v))
	for i, x := range v {
		result[i] = x / mag
	}
	return result
}

// EuclideanDistance computes the Euclidean distance between two vectors
func EuclideanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		diff := float64(a[i]) - float64(b[i])
		sum += diff * diff
	}
	return float32(math.Sqrt(sum))
}

// ScoredItem represents a result with a similarity score
type ScoredItem struct {
	Index int
	Score float32
}

// TopKByCosineSimilarity finds the top-k most similar vectors to query
// Returns indices and scores sorted by similarity (highest first)
func TopKByCosineSimilarity(query []float32, vectors [][]float32, k int) []ScoredItem {
	if k <= 0 || len(vectors) == 0 {
		return nil
	}

	// Calculate all similarities
	similarities := make([]ScoredItem, len(vectors))
	for i, v := range vectors {
		similarities[i] = ScoredItem{
			Index: i,
			Score: CosineSimilarity(query, v),
		}
	}

	// Simple selection sort for top-k (efficient for small k)
	// For large k or vectors, consider using a heap
	for i := 0; i < k && i < len(similarities); i++ {
		maxIdx := i
		for j := i + 1; j < len(similarities); j++ {
			if similarities[j].Score > similarities[maxIdx].Score {
				maxIdx = j
			}
		}
		similarities[i], similarities[maxIdx] = similarities[maxIdx], similarities[i]
	}

	if k > len(similarities) {
		k = len(similarities)
	}
	return similarities[:k]
}
