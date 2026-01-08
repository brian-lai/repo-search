package embedding

import "context"

// Embedder is the interface for embedding providers
type Embedder interface {
	// Embed generates embeddings for multiple texts
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Available checks if the provider is ready
	Available() bool

	// ProviderID returns a unique identifier for index segregation
	// Format: "provider:model" e.g. "ollama:nomic-embed-text", "litellm:text-embedding-3-small"
	ProviderID() string

	// Dimensions returns the embedding vector size
	Dimensions() int
}

// NullEmbedder is a no-op embedder for when embedding is disabled
type NullEmbedder struct{}

func (n *NullEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, nil
}

func (n *NullEmbedder) Available() bool {
	return false
}

func (n *NullEmbedder) ProviderID() string {
	return "off"
}

func (n *NullEmbedder) Dimensions() int {
	return 0
}
