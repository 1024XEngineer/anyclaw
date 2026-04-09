package memory

import (
	"fmt"
	"math"
)

// CosineSimilarity computes the cosine similarity between two vectors
func CosineSimilarity(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimension mismatch: %d vs %d", len(a), len(b))
	}
	if len(a) == 0 {
		return 0, fmt.Errorf("empty vectors")
	}

	var dotProduct float64
	var normA float64
	var normB float64

	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0, nil
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}

// EuclideanDistance computes the Euclidean distance between two vectors
func EuclideanDistance(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimension mismatch: %d vs %d", len(a), len(b))
	}
	if len(a) == 0 {
		return 0, fmt.Errorf("empty vectors")
	}

	var sum float64
	for i := range a {
		diff := float64(a[i] - b[i])
		sum += diff * diff
	}

	return math.Sqrt(sum), nil
}

// DotProduct computes the dot product of two vectors
func DotProduct(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimension mismatch: %d vs %d", len(a), len(b))
	}

	var sum float64
	for i := range a {
		sum += float64(a[i] * b[i])
	}

	return sum, nil
}

// Normalize normalizes a vector to unit length
func Normalize(vec []float32) ([]float32, error) {
	if len(vec) == 0 {
		return nil, fmt.Errorf("empty vector")
	}

	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	norm = math.Sqrt(norm)

	if norm == 0 {
		return nil, fmt.Errorf("cannot normalize zero vector")
	}

	normalized := make([]float32, len(vec))
	for i, v := range vec {
		normalized[i] = float32(float64(v) / norm)
	}

	return normalized, nil
}

// Magnitude computes the magnitude (length) of a vector
func Magnitude(vec []float32) (float64, error) {
	if len(vec) == 0 {
		return 0, fmt.Errorf("empty vector")
	}

	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}

	return math.Sqrt(sum), nil
}

// Add adds two vectors
func Add(a, b []float32) ([]float32, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("vector dimension mismatch: %d vs %d", len(a), len(b))
	}

	result := make([]float32, len(a))
	for i := range a {
		result[i] = a[i] + b[i]
	}

	return result, nil
}

// Subtract subtracts vector b from vector a
func Subtract(a, b []float32) ([]float32, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("vector dimension mismatch: %d vs %d", len(a), len(b))
	}

	result := make([]float32, len(a))
	for i := range a {
		result[i] = a[i] - b[i]
	}

	return result, nil
}

// Multiply multiplies a vector by a scalar
func Multiply(vec []float32, scalar float64) ([]float32, error) {
	if len(vec) == 0 {
		return nil, fmt.Errorf("empty vector")
	}

	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(float64(v) * scalar)
	}

	return result, nil
}

// Mean computes the mean vector of multiple vectors
func Mean(vectors [][]float32) ([]float32, error) {
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no vectors provided")
	}

	dim := len(vectors[0])
	for _, vec := range vectors {
		if len(vec) != dim {
			return nil, fmt.Errorf("vector dimension mismatch")
		}
	}

	result := make([]float32, dim)
	for _, vec := range vectors {
		for i, v := range vec {
			result[i] += v
		}
	}

	count := float32(len(vectors))
	for i := range result {
		result[i] /= count
	}

	return result, nil
}

// ChunkText splits text into chunks suitable for embedding
func ChunkText(text string, maxTokens int) []string {
	// Simple chunking by character count
	// A more sophisticated implementation would use a tokenizer
	const approxCharsPerToken = 4
	maxChars := maxTokens * approxCharsPerToken

	if len(text) <= maxChars {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxChars {
			chunks = append(chunks, text)
			break
		}

		// Try to split at a sentence boundary
		splitPos := maxChars
		for i := maxChars; i > maxChars/2; i-- {
			if text[i] == '.' || text[i] == '!' || text[i] == '?' {
				splitPos = i + 1
				break
			}
		}

		// If no sentence boundary found, try to split at a word boundary
		if splitPos == maxChars {
			for i := maxChars; i > maxChars/2; i-- {
				if text[i] == ' ' || text[i] == '\n' {
					splitPos = i + 1
					break
				}
			}
		}

		chunks = append(chunks, text[:splitPos])
		text = text[splitPos:]
	}

	return chunks
}

// ComputeHash computes a simple hash of a vector for caching purposes
func ComputeHash(vec []float32) uint64 {
	// Simple hash function for demonstration
	// In production, use a proper hash like xxhash or fnv
	var h uint64 = 14695981039346656037
	for _, v := range vec {
		h ^= uint64(v)
		h *= 1099511628211
	}
	return h
}
