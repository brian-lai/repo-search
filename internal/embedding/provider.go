package embedding

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Provider is the type of embedding provider
type Provider string

const (
	ProviderOllama  Provider = "ollama"
	ProviderLiteLLM Provider = "litellm"
	ProviderOff     Provider = "off"
)

// ProviderConfig configures the embedding provider
type ProviderConfig struct {
	Provider   Provider // "ollama", "litellm", "off"
	OllamaURL  string   // default: http://localhost:11434
	LiteLLMURL string   // default: http://localhost:4000
	LiteLLMKey string   // API key for LiteLLM
	Model      string   // model name (provider-specific default if empty)
	Dimensions int      // embedding dimensions (0 = auto-detect)
}

// DefaultProviderConfig returns the default provider configuration
func DefaultProviderConfig() ProviderConfig {
	return ProviderConfig{
		Provider:   ProviderOllama,
		OllamaURL:  DefaultOllamaURL,
		LiteLLMURL: DefaultLiteLLMURL,
		Model:      "", // will use provider default
		Dimensions: 0,  // will use provider default
	}
}

// LoadConfigFromEnv loads provider configuration from environment variables
func LoadConfigFromEnv() ProviderConfig {
	cfg := DefaultProviderConfig()

	// Provider selection
	if p := os.Getenv("REPO_SEARCH_EMBEDDING_PROVIDER"); p != "" {
		switch strings.ToLower(p) {
		case "ollama":
			cfg.Provider = ProviderOllama
		case "litellm":
			cfg.Provider = ProviderLiteLLM
		case "off", "disabled", "none":
			cfg.Provider = ProviderOff
		default:
			// Log warning but use default
			fmt.Fprintf(os.Stderr, "warning: unknown embedding provider %q, using ollama\n", p)
		}
	}

	// Ollama configuration
	if url := os.Getenv("REPO_SEARCH_OLLAMA_URL"); url != "" {
		cfg.OllamaURL = url
	}

	// LiteLLM configuration
	if url := os.Getenv("REPO_SEARCH_LITELLM_URL"); url != "" {
		cfg.LiteLLMURL = url
	}
	if key := os.Getenv("REPO_SEARCH_LITELLM_API_KEY"); key != "" {
		cfg.LiteLLMKey = key
	}

	// Model override
	if model := os.Getenv("REPO_SEARCH_EMBEDDING_MODEL"); model != "" {
		cfg.Model = model
	}

	// Dimensions override
	if dim := os.Getenv("REPO_SEARCH_EMBEDDING_DIMENSIONS"); dim != "" {
		if d, err := strconv.Atoi(dim); err == nil && d > 0 {
			cfg.Dimensions = d
		}
	}

	return cfg
}

// NewEmbedder creates an Embedder from the configuration
func NewEmbedder(cfg ProviderConfig) (Embedder, error) {
	switch cfg.Provider {
	case ProviderOff:
		return &NullEmbedder{}, nil

	case ProviderOllama:
		opts := []OllamaOption{
			WithBaseURL(cfg.OllamaURL),
		}
		if cfg.Model != "" {
			opts = append(opts, WithModel(cfg.Model))
		}
		return NewOllamaClient(opts...), nil

	case ProviderLiteLLM:
		opts := []LiteLLMOption{
			WithLiteLLMBaseURL(cfg.LiteLLMURL),
		}
		if cfg.LiteLLMKey != "" {
			opts = append(opts, WithLiteLLMAPIKey(cfg.LiteLLMKey))
		}
		if cfg.Model != "" {
			opts = append(opts, WithLiteLLMModel(cfg.Model))
		}
		if cfg.Dimensions > 0 {
			opts = append(opts, WithLiteLLMDimensions(cfg.Dimensions))
		}
		return NewLiteLLMClient(opts...), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

// NewEmbedderFromEnv creates an Embedder from environment configuration
func NewEmbedderFromEnv() (Embedder, error) {
	return NewEmbedder(LoadConfigFromEnv())
}

// ProviderName returns a human-readable name for the provider
func (p Provider) String() string {
	switch p {
	case ProviderOllama:
		return "Ollama"
	case ProviderLiteLLM:
		return "LiteLLM"
	case ProviderOff:
		return "Disabled"
	default:
		return string(p)
	}
}

// IsEnabled returns true if the provider is not disabled
func (p Provider) IsEnabled() bool {
	return p != ProviderOff
}
