package embedding

import "context"

// EmbeddingProvider defines the interface for text embedding generation.
type EmbeddingProvider interface {
	// Embed converts text to a vector embedding.
	Embed(ctx context.Context, text string) ([]float32, error)
	// Dim returns the dimensionality of the embedding vectors.
	Dim() int
	// Name returns the provider name.
	Name() string
}

// BatchEmbed converts multiple texts to embeddings.
// Providers that don't support batching will call Embed sequentially.
func BatchEmbed(ctx context.Context, provider EmbeddingProvider, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := provider.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}
