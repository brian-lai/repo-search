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
	DefaultLiteLLMURL        = "http://localhost:4000"
	DefaultLiteLLMModel      = "text-embedding-3-small"
	DefaultLiteLLMDimensions = 1536 // OpenAI text-embedding-3-small default
	DefaultLiteLLMTimeout    = 30 * time.Second
)

// LiteLLMClient provides access to OpenAI-compatible embedding APIs (LiteLLM, AWS Bedrock, etc.)
type LiteLLMClient struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	timeout    time.Duration
	httpClient *http.Client
}

// LiteLLMOption configures the LiteLLM client
type LiteLLMOption func(*LiteLLMClient)

// WithLiteLLMBaseURL sets the LiteLLM server URL
func WithLiteLLMBaseURL(url string) LiteLLMOption {
	return func(c *LiteLLMClient) {
		c.baseURL = url
	}
}

// WithLiteLLMAPIKey sets the API key
func WithLiteLLMAPIKey(key string) LiteLLMOption {
	return func(c *LiteLLMClient) {
		c.apiKey = key
	}
}

// WithLiteLLMModel sets the embedding model
func WithLiteLLMModel(model string) LiteLLMOption {
	return func(c *LiteLLMClient) {
		c.model = model
	}
}

// WithLiteLLMDimensions sets the expected embedding dimensions
func WithLiteLLMDimensions(dim int) LiteLLMOption {
	return func(c *LiteLLMClient) {
		c.dimensions = dim
	}
}

// WithLiteLLMTimeout sets the request timeout
func WithLiteLLMTimeout(timeout time.Duration) LiteLLMOption {
	return func(c *LiteLLMClient) {
		c.timeout = timeout
	}
}

// NewLiteLLMClient creates a new LiteLLM client
func NewLiteLLMClient(opts ...LiteLLMOption) *LiteLLMClient {
	c := &LiteLLMClient{
		baseURL:    DefaultLiteLLMURL,
		model:      DefaultLiteLLMModel,
		dimensions: DefaultLiteLLMDimensions,
		timeout:    DefaultLiteLLMTimeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.httpClient = &http.Client{
		Timeout: c.timeout,
	}

	return c
}

// openAIEmbeddingRequest is the request body for OpenAI-compatible embedding API
type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// openAIEmbeddingResponse is the response from OpenAI-compatible embedding API
type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Embed implements Embedder.Embed - generates embeddings for multiple texts
func (c *LiteLLMClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := openAIEmbeddingRequest{
		Model: c.model,
		Input: texts,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LiteLLM returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("LiteLLM error: %s", result.Error.Message)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("unexpected response: got %d embeddings for %d texts", len(result.Data), len(texts))
	}

	// Sort by index to ensure correct order
	embeddings := make([][]float32, len(texts))
	for _, item := range result.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("invalid index %d in response", item.Index)
		}
		embeddings[item.Index] = item.Embedding
	}

	return embeddings, nil
}

// Available implements Embedder.Available - checks if the provider is ready
func (c *LiteLLMClient) Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try a simple health check or model list
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		// Fall back to checking /v1/models
		req, err = http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/models", nil)
		if err != nil {
			return false
		}
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Accept 200 or 401 (means server is running, just needs auth)
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized
}

// ProviderID implements Embedder.ProviderID - returns unique identifier
func (c *LiteLLMClient) ProviderID() string {
	return "litellm:" + c.model
}

// Dimensions implements Embedder.Dimensions - returns embedding vector size
func (c *LiteLLMClient) Dimensions() int {
	return c.dimensions
}

// Model returns the current model name
func (c *LiteLLMClient) Model() string {
	return c.model
}

// BaseURL returns the current base URL
func (c *LiteLLMClient) BaseURL() string {
	return c.baseURL
}

// Ensure LiteLLMClient implements Embedder
var _ Embedder = (*LiteLLMClient)(nil)
