package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewLiteLLMClient(t *testing.T) {
	t.Run("creates client with defaults", func(t *testing.T) {
		client := NewLiteLLMClient()

		if client.baseURL != DefaultLiteLLMURL {
			t.Errorf("expected baseURL=%s, got %s", DefaultLiteLLMURL, client.baseURL)
		}
		if client.model != DefaultLiteLLMModel {
			t.Errorf("expected model=%s, got %s", DefaultLiteLLMModel, client.model)
		}
		if client.dimensions != DefaultLiteLLMDimensions {
			t.Errorf("expected dimensions=%d, got %d", DefaultLiteLLMDimensions, client.dimensions)
		}
	})

	t.Run("creates client with custom options", func(t *testing.T) {
		client := NewLiteLLMClient(
			WithLiteLLMBaseURL("http://custom:8080"),
			WithLiteLLMAPIKey("test-key"),
			WithLiteLLMModel("custom-model"),
			WithLiteLLMDimensions(768),
		)

		if client.baseURL != "http://custom:8080" {
			t.Errorf("expected baseURL=http://custom:8080, got %s", client.baseURL)
		}
		if client.apiKey != "test-key" {
			t.Errorf("expected apiKey=test-key, got %s", client.apiKey)
		}
		if client.model != "custom-model" {
			t.Errorf("expected model=custom-model, got %s", client.model)
		}
		if client.dimensions != 768 {
			t.Errorf("expected dimensions=768, got %d", client.dimensions)
		}
	})
}

func TestLiteLLMClient_ProviderID(t *testing.T) {
	client := NewLiteLLMClient(WithLiteLLMModel("text-embedding-3-small"))

	expected := "litellm:text-embedding-3-small"
	if got := client.ProviderID(); got != expected {
		t.Errorf("ProviderID() = %s, want %s", got, expected)
	}
}

func TestLiteLLMClient_Dimensions(t *testing.T) {
	client := NewLiteLLMClient(WithLiteLLMDimensions(512))

	if got := client.Dimensions(); got != 512 {
		t.Errorf("Dimensions() = %d, want 512", got)
	}
}

func TestLiteLLMClient_Embed(t *testing.T) {
	t.Run("successful embedding", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/embeddings" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != "POST" {
				t.Errorf("unexpected method: %s", r.Method)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
			}

			// Parse request
			var req openAIEmbeddingRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}

			if len(req.Input) != 2 {
				t.Errorf("expected 2 inputs, got %d", len(req.Input))
			}

			// Send response
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float32 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
					{Embedding: []float32{0.4, 0.5, 0.6}, Index: 1},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLiteLLMClient(
			WithLiteLLMBaseURL(server.URL),
			WithLiteLLMAPIKey("test-key"),
		)

		embeddings, err := client.Embed(context.Background(), []string{"hello", "world"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(embeddings) != 2 {
			t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
		}
		if len(embeddings[0]) != 3 {
			t.Errorf("expected 3 dimensions, got %d", len(embeddings[0]))
		}
		if embeddings[0][0] != 0.1 {
			t.Errorf("expected first value 0.1, got %f", embeddings[0][0])
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		client := NewLiteLLMClient()
		embeddings, err := client.Embed(context.Background(), []string{})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if embeddings != nil {
			t.Errorf("expected nil for empty input, got %v", embeddings)
		}
	})

	t.Run("handles server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		_, err := client.Embed(context.Background(), []string{"test"})

		if err == nil {
			t.Error("expected error for server error")
		}
	})

	t.Run("handles API error in response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := openAIEmbeddingResponse{
				Error: &struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				}{
					Message: "rate limit exceeded",
					Type:    "rate_limit_error",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		_, err := client.Embed(context.Background(), []string{"test"})

		if err == nil {
			t.Error("expected error for API error response")
		}
	})

	t.Run("handles response with out-of-order indices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := openAIEmbeddingResponse{
				Data: []struct {
					Embedding []float32 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: []float32{0.4, 0.5}, Index: 1},
					{Embedding: []float32{0.1, 0.2}, Index: 0},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		embeddings, err := client.Embed(context.Background(), []string{"first", "second"})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should be reordered by index
		if embeddings[0][0] != 0.1 {
			t.Errorf("expected first embedding to have value 0.1, got %f", embeddings[0][0])
		}
		if embeddings[1][0] != 0.4 {
			t.Errorf("expected second embedding to have value 0.4, got %f", embeddings[1][0])
		}
	})
}

func TestLiteLLMClient_Available(t *testing.T) {
	t.Run("returns true when health check succeeds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		if !client.Available() {
			t.Error("expected Available() = true")
		}
	})

	t.Run("returns true for 401 (server running but needs auth)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		client := NewLiteLLMClient(WithLiteLLMBaseURL(server.URL))
		if !client.Available() {
			t.Error("expected Available() = true for 401")
		}
	})

	t.Run("returns false when server not available", func(t *testing.T) {
		client := NewLiteLLMClient(WithLiteLLMBaseURL("http://localhost:59999"))
		if client.Available() {
			t.Error("expected Available() = false")
		}
	})
}

func TestNullEmbedder(t *testing.T) {
	embedder := &NullEmbedder{}

	t.Run("Embed returns nil", func(t *testing.T) {
		result, err := embedder.Embed(context.Background(), []string{"test"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("Available returns false", func(t *testing.T) {
		if embedder.Available() {
			t.Error("expected Available() = false")
		}
	})

	t.Run("ProviderID returns off", func(t *testing.T) {
		if embedder.ProviderID() != "off" {
			t.Errorf("expected ProviderID() = off, got %s", embedder.ProviderID())
		}
	})

	t.Run("Dimensions returns 0", func(t *testing.T) {
		if embedder.Dimensions() != 0 {
			t.Errorf("expected Dimensions() = 0, got %d", embedder.Dimensions())
		}
	})
}
