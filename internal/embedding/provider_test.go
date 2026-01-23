package embedding

import (
	"os"
	"testing"
)

func TestDefaultProviderConfig(t *testing.T) {
	cfg := DefaultProviderConfig()

	if cfg.Provider != ProviderOllama {
		t.Errorf("expected Provider=%s, got %s", ProviderOllama, cfg.Provider)
	}
	if cfg.OllamaURL != DefaultOllamaURL {
		t.Errorf("expected OllamaURL=%s, got %s", DefaultOllamaURL, cfg.OllamaURL)
	}
	if cfg.LiteLLMURL != DefaultLiteLLMURL {
		t.Errorf("expected LiteLLMURL=%s, got %s", DefaultLiteLLMURL, cfg.LiteLLMURL)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Save original env and restore after test
	origProvider := os.Getenv("CODETECT_EMBEDDING_PROVIDER")
	origOllamaURL := os.Getenv("CODETECT_OLLAMA_URL")
	origLiteLLMURL := os.Getenv("CODETECT_LITELLM_URL")
	origLiteLLMKey := os.Getenv("CODETECT_LITELLM_API_KEY")
	origModel := os.Getenv("CODETECT_EMBEDDING_MODEL")
	origDimensions := os.Getenv("CODETECT_EMBEDDING_DIMENSIONS")

	defer func() {
		os.Setenv("CODETECT_EMBEDDING_PROVIDER", origProvider)
		os.Setenv("CODETECT_OLLAMA_URL", origOllamaURL)
		os.Setenv("CODETECT_LITELLM_URL", origLiteLLMURL)
		os.Setenv("CODETECT_LITELLM_API_KEY", origLiteLLMKey)
		os.Setenv("CODETECT_EMBEDDING_MODEL", origModel)
		os.Setenv("CODETECT_EMBEDDING_DIMENSIONS", origDimensions)
	}()

	t.Run("defaults when no env vars set", func(t *testing.T) {
		os.Unsetenv("CODETECT_EMBEDDING_PROVIDER")
		os.Unsetenv("CODETECT_OLLAMA_URL")
		os.Unsetenv("CODETECT_LITELLM_URL")
		os.Unsetenv("CODETECT_LITELLM_API_KEY")
		os.Unsetenv("CODETECT_EMBEDDING_MODEL")
		os.Unsetenv("CODETECT_EMBEDDING_DIMENSIONS")

		cfg := LoadConfigFromEnv()

		if cfg.Provider != ProviderOllama {
			t.Errorf("expected default Provider=%s, got %s", ProviderOllama, cfg.Provider)
		}
	})

	t.Run("reads ollama provider", func(t *testing.T) {
		os.Setenv("CODETECT_EMBEDDING_PROVIDER", "ollama")
		os.Setenv("CODETECT_OLLAMA_URL", "http://custom:11434")

		cfg := LoadConfigFromEnv()

		if cfg.Provider != ProviderOllama {
			t.Errorf("expected Provider=%s, got %s", ProviderOllama, cfg.Provider)
		}
		if cfg.OllamaURL != "http://custom:11434" {
			t.Errorf("expected OllamaURL=http://custom:11434, got %s", cfg.OllamaURL)
		}
	})

	t.Run("reads litellm provider", func(t *testing.T) {
		os.Setenv("CODETECT_EMBEDDING_PROVIDER", "litellm")
		os.Setenv("CODETECT_LITELLM_URL", "http://litellm:4000")
		os.Setenv("CODETECT_LITELLM_API_KEY", "test-key")

		cfg := LoadConfigFromEnv()

		if cfg.Provider != ProviderLiteLLM {
			t.Errorf("expected Provider=%s, got %s", ProviderLiteLLM, cfg.Provider)
		}
		if cfg.LiteLLMURL != "http://litellm:4000" {
			t.Errorf("expected LiteLLMURL=http://litellm:4000, got %s", cfg.LiteLLMURL)
		}
		if cfg.LiteLLMKey != "test-key" {
			t.Errorf("expected LiteLLMKey=test-key, got %s", cfg.LiteLLMKey)
		}
	})

	t.Run("reads off provider", func(t *testing.T) {
		os.Setenv("CODETECT_EMBEDDING_PROVIDER", "off")

		cfg := LoadConfigFromEnv()

		if cfg.Provider != ProviderOff {
			t.Errorf("expected Provider=%s, got %s", ProviderOff, cfg.Provider)
		}
	})

	t.Run("reads model override", func(t *testing.T) {
		os.Setenv("CODETECT_EMBEDDING_PROVIDER", "ollama")
		os.Setenv("CODETECT_EMBEDDING_MODEL", "custom-model")

		cfg := LoadConfigFromEnv()

		if cfg.Model != "custom-model" {
			t.Errorf("expected Model=custom-model, got %s", cfg.Model)
		}
	})

	t.Run("reads dimensions override", func(t *testing.T) {
		os.Setenv("CODETECT_EMBEDDING_PROVIDER", "litellm")
		os.Setenv("CODETECT_EMBEDDING_DIMENSIONS", "1024")

		cfg := LoadConfigFromEnv()

		if cfg.Dimensions != 1024 {
			t.Errorf("expected Dimensions=1024, got %d", cfg.Dimensions)
		}
	})

	t.Run("ignores invalid dimensions", func(t *testing.T) {
		os.Setenv("CODETECT_EMBEDDING_DIMENSIONS", "invalid")

		cfg := LoadConfigFromEnv()

		if cfg.Dimensions != 0 {
			t.Errorf("expected Dimensions=0 for invalid input, got %d", cfg.Dimensions)
		}
	})
}

func TestNewEmbedder(t *testing.T) {
	t.Run("creates NullEmbedder for off", func(t *testing.T) {
		cfg := ProviderConfig{Provider: ProviderOff}
		embedder, err := NewEmbedder(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, ok := embedder.(*NullEmbedder)
		if !ok {
			t.Errorf("expected *NullEmbedder, got %T", embedder)
		}

		if embedder.Available() {
			t.Error("NullEmbedder should not be available")
		}
		if embedder.ProviderID() != "off" {
			t.Errorf("expected ProviderID=off, got %s", embedder.ProviderID())
		}
	})

	t.Run("creates OllamaClient for ollama", func(t *testing.T) {
		cfg := ProviderConfig{
			Provider:  ProviderOllama,
			OllamaURL: "http://test:11434",
			Model:     "test-model",
		}
		embedder, err := NewEmbedder(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		client, ok := embedder.(*OllamaClient)
		if !ok {
			t.Errorf("expected *OllamaClient, got %T", embedder)
		}
		if client.BaseURL() != "http://test:11434" {
			t.Errorf("expected BaseURL=http://test:11434, got %s", client.BaseURL())
		}
		if client.Model() != "test-model" {
			t.Errorf("expected Model=test-model, got %s", client.Model())
		}
		if embedder.ProviderID() != "ollama:test-model" {
			t.Errorf("expected ProviderID=ollama:test-model, got %s", embedder.ProviderID())
		}
	})

	t.Run("creates LiteLLMClient for litellm", func(t *testing.T) {
		cfg := ProviderConfig{
			Provider:   ProviderLiteLLM,
			LiteLLMURL: "http://litellm:4000",
			LiteLLMKey: "test-key",
			Model:      "text-embedding-ada-002",
			Dimensions: 1536,
		}
		embedder, err := NewEmbedder(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		client, ok := embedder.(*LiteLLMClient)
		if !ok {
			t.Errorf("expected *LiteLLMClient, got %T", embedder)
		}
		if client.BaseURL() != "http://litellm:4000" {
			t.Errorf("expected BaseURL=http://litellm:4000, got %s", client.BaseURL())
		}
		if client.Model() != "text-embedding-ada-002" {
			t.Errorf("expected Model=text-embedding-ada-002, got %s", client.Model())
		}
		if client.Dimensions() != 1536 {
			t.Errorf("expected Dimensions=1536, got %d", client.Dimensions())
		}
		if embedder.ProviderID() != "litellm:text-embedding-ada-002" {
			t.Errorf("expected ProviderID=litellm:text-embedding-ada-002, got %s", embedder.ProviderID())
		}
	})

	t.Run("uses default model for ollama", func(t *testing.T) {
		cfg := ProviderConfig{
			Provider:  ProviderOllama,
			OllamaURL: DefaultOllamaURL,
		}
		embedder, err := NewEmbedder(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		client := embedder.(*OllamaClient)
		if client.Model() != DefaultModel {
			t.Errorf("expected default model %s, got %s", DefaultModel, client.Model())
		}
	})

	t.Run("uses default model for litellm", func(t *testing.T) {
		cfg := ProviderConfig{
			Provider:   ProviderLiteLLM,
			LiteLLMURL: DefaultLiteLLMURL,
		}
		embedder, err := NewEmbedder(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		client := embedder.(*LiteLLMClient)
		if client.Model() != DefaultLiteLLMModel {
			t.Errorf("expected default model %s, got %s", DefaultLiteLLMModel, client.Model())
		}
	})
}

func TestProviderString(t *testing.T) {
	tests := []struct {
		provider Provider
		want     string
	}{
		{ProviderOllama, "Ollama"},
		{ProviderLiteLLM, "LiteLLM"},
		{ProviderOff, "Disabled"},
		{Provider("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			got := tt.provider.String()
			if got != tt.want {
				t.Errorf("Provider.String() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestProviderIsEnabled(t *testing.T) {
	if !ProviderOllama.IsEnabled() {
		t.Error("ProviderOllama should be enabled")
	}
	if !ProviderLiteLLM.IsEnabled() {
		t.Error("ProviderLiteLLM should be enabled")
	}
	if ProviderOff.IsEnabled() {
		t.Error("ProviderOff should not be enabled")
	}
}
