package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultOllamaURL   = "http://localhost:11434"
	DefaultModel       = "nomic-embed-text"
	DefaultTimeout     = 30 * time.Second
	DefaultBatchSize   = 32
	DefaultDimensions  = 768 // nomic-embed-text dimensions
)

// OllamaClient provides access to the Ollama embedding API
type OllamaClient struct {
	baseURL    string
	model      string
	timeout    time.Duration
	batchSize  int
	httpClient *http.Client
}

// OllamaOption configures the Ollama client
type OllamaOption func(*OllamaClient)

// WithBaseURL sets the Ollama server URL
func WithBaseURL(url string) OllamaOption {
	return func(c *OllamaClient) {
		c.baseURL = url
	}
}

// WithModel sets the embedding model
func WithModel(model string) OllamaOption {
	return func(c *OllamaClient) {
		c.model = model
	}
}

// WithTimeout sets the request timeout
func WithTimeout(timeout time.Duration) OllamaOption {
	return func(c *OllamaClient) {
		c.timeout = timeout
	}
}

// NewOllamaClient creates a new Ollama client
func NewOllamaClient(opts ...OllamaOption) *OllamaClient {
	c := &OllamaClient{
		baseURL:   DefaultOllamaURL,
		model:     DefaultModel,
		timeout:   DefaultTimeout,
		batchSize: DefaultBatchSize,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.httpClient = &http.Client{
		Timeout: c.timeout,
	}

	return c
}

// Available checks if Ollama is running and the model is available
func (c *OllamaClient) Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// ModelAvailable checks if the specific embedding model is available
func (c *OllamaClient) ModelAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	for _, m := range result.Models {
		if m.Name == c.model || m.Name == c.model+":latest" {
			return true
		}
	}

	return false
}

// embedRequest is the request body for the Ollama embedding API
type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// embedResponse is the response from the Ollama embedding API
type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed generates an embedding for a single text
func (c *OllamaClient) Embed(text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.EmbedWithContext(ctx, text)
}

// EmbedWithContext generates an embedding with a custom context
func (c *OllamaClient) EmbedWithContext(ctx context.Context, text string) ([]float32, error) {
	reqBody := embedRequest{
		Model:  c.model,
		Prompt: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding returned")
	}

	return result.Embedding, nil
}

// EmbedBatch generates embeddings for multiple texts
// Processes in batches to avoid overwhelming Ollama
func (c *OllamaClient) EmbedBatch(texts []string) ([][]float32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout*time.Duration(len(texts)/c.batchSize+1))
	defer cancel()

	return c.EmbedBatchWithContext(ctx, texts)
}

// EmbedBatchWithContext generates embeddings for multiple texts with a custom context
func (c *OllamaClient) EmbedBatchWithContext(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))

	for i, text := range texts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emb, err := c.EmbedWithContext(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embedding text %d: %w", i, err)
		}
		embeddings[i] = emb
	}

	return embeddings, nil
}

// Model returns the current model name
func (c *OllamaClient) Model() string {
	return c.model
}

// BaseURL returns the current base URL
func (c *OllamaClient) BaseURL() string {
	return c.baseURL
}
