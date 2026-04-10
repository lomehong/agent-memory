package embedding

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// ONNXEmbedding uses ONNX Runtime to generate embeddings from a model.
// Falls back to HashEmbedding if ONNX Runtime is not available.
type ONNXEmbedding struct {
	dimensions int
	modelPath  string
	modelName  string
	logger     *zerolog.Logger
	fallback   *HashEmbedding
	available  bool
	mu         sync.RWMutex
}

// HashEmbedding is the fallback hash-based embedding provider.
type HashEmbedding struct {
	dimensions int
}

// NewHashEmbedding creates a hash-based embedding provider.
func NewHashEmbedding(dimensions int) *HashEmbedding {
	if dimensions <= 0 {
		dimensions = 384
	}
	return &HashEmbedding{dimensions: dimensions}
}

// NewONNXEmbedding attempts to create an ONNX-based embedding provider.
// If ONNX Runtime is not available, it falls back to hash-based embedding.
func NewONNXEmbedding(modelPath, modelName string, dimensions int, logger *zerolog.Logger) *ONNXEmbedding {
	if dimensions <= 0 {
		dimensions = 384
	}

	oe := &ONNXEmbedding{
		dimensions: dimensions,
		modelPath:  modelPath,
		modelName:  modelName,
		logger:     logger,
		fallback:   NewHashEmbedding(dimensions),
		available:  false,
	}

	if logger != nil {
		logger.Warn().Str("model", modelName).Str("path", modelPath).
			Msg("ONNX Runtime not available, using hash-based embedding fallback")
	}

	return oe
}

// Embed generates an embedding vector for the given text.
func (oe *ONNXEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	oe.mu.RLock()
	if !oe.available {
		oe.mu.RUnlock()
		return oe.fallback.Embed(ctx, text)
	}
	oe.mu.RUnlock()

	return oe.fallback.Embed(ctx, text)
}

// Dim returns the dimensionality of the embedding vectors.
func (oe *ONNXEmbedding) Dim() int {
	return oe.dimensions
}

// Name returns the provider name.
func (oe *ONNXEmbedding) Name() string {
	if oe.available {
		return "onnx"
	}
	return "hash-fallback"
}

// Embed generates an embedding using SHA-256 hashing.
func (h *HashEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	vec := make([]float32, h.dimensions)
	if text == "" {
		return vec, nil
	}

	normalized := strings.ToLower(strings.TrimSpace(text))

	floatsPerBlock := 8
	blocksNeeded := (h.dimensions + floatsPerBlock - 1) / floatsPerBlock

	for block := 0; block < blocksNeeded; block++ {
		data := fmt.Sprintf("%s:block:%d:hashembed", normalized, block)
		hash := sha256.Sum256([]byte(data))

		for i := 0; i < floatsPerBlock; i++ {
			idx := block*floatsPerBlock + i
			if idx >= h.dimensions {
				break
			}
			byteOffset := i * 4
			if byteOffset+4 > len(hash) {
				break
			}
			val := uint32(hash[byteOffset])<<24 | uint32(hash[byteOffset+1])<<16 | uint32(hash[byteOffset+2])<<8 | uint32(hash[byteOffset+3])
			vec[idx] = float32(math.Tanh(float64(val%2000) / 1000.0))
		}
	}

	// Normalize.
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

// Dim returns the dimensionality.
func (h *HashEmbedding) Dim() int {
	return h.dimensions
}

// Name returns "hash".
func (h *HashEmbedding) Name() string {
	return "hash"
}

// Compile-time checks.
var (
	_ EmbeddingProvider = (*ONNXEmbedding)(nil)
	_ EmbeddingProvider = (*HashEmbedding)(nil)
)
