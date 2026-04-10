package embedding

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
)

// MockEmbedding provides a deterministic hash-based embedding for testing and development.
type MockEmbedding struct {
	dimensions int
}

// NewMockEmbedding creates a new mock embedding provider.
func NewMockEmbedding(dimensions int) *MockEmbedding {
	if dimensions <= 0 {
		dimensions = 384
	}
	return &MockEmbedding{dimensions: dimensions}
}

// Embed generates a deterministic embedding based on text content.
// Uses SHA-256 hash to produce consistent vectors for the same input.
func (m *MockEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	vec := make([]float32, m.dimensions)
	if text == "" {
		return vec, nil
	}

	// Normalize text for consistent hashing.
	normalized := strings.ToLower(strings.TrimSpace(text))

	// Generate hash blocks to fill the dimensions.
	// SHA-256 produces 32 bytes = 8 float32 values per block.
	floatsPerBlock := 8
	blocksNeeded := (m.dimensions + floatsPerBlock - 1) / floatsPerBlock

	for block := 0; block < blocksNeeded; block++ {
		data := fmt.Sprintf("%s:block:%d:mock", normalized, block)
		hash := sha256.Sum256([]byte(data))

		for i := 0; i < floatsPerBlock; i++ {
			idx := block*floatsPerBlock + i
			if idx >= m.dimensions {
				break
			}

			// Convert 4 bytes (not 8) to a float32 in [-1, 1] range.
			byteOffset := i * 4
			if byteOffset+4 > len(hash) {
				break
			}
			val := uint32(hash[byteOffset])<<24 | uint32(hash[byteOffset+1])<<16 | uint32(hash[byteOffset+2])<<8 | uint32(hash[byteOffset+3])
			// Map uint32 to [-1, 1] using tanh
			vec[idx] = float32(math.Tanh(float64(val%2000) / 1000.0))
		}
	}

	// Normalize the vector.
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}

	return vec, nil
}

// Dim returns the dimensionality of the mock embeddings.
func (m *MockEmbedding) Dim() int {
	return m.dimensions
}

// Name returns "mock".
func (m *MockEmbedding) Name() string {
	return "mock"
}

// Compile-time check.
var _ EmbeddingProvider = (*MockEmbedding)(nil)
