# Plan: Embedding Adapter Layer

## Objective

Refactor Phase 3 semantic indexing to introduce a clean adapter interface for embedding providers, enabling future swap to LiteLLM/Bedrock without code changes.

## Context

- Phase 3 semantic search is implemented using Ollama (`internal/embedding/ollama.go`)
- Current OllamaClient is tightly coupled but well-structured with functional options
- Database schema already tracks `model` field per embedding (good for versioning)
- Need to support: `ollama` (default), `litellm` (OpenAI-compatible), `off` (disabled)

## Approach

### Step 1: Define Embedder Interface

Create `internal/embedding/embedder.go`:

```go
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
```

### Step 2: Adapt OllamaClient to Implement Embedder

Modify `internal/embedding/ollama.go`:
- Add `Embed(ctx, []string)` method (batch wrapper around existing `EmbedBatchWithContext`)
- Add `ProviderID()` returning `"ollama:" + model`
- Add `Dimensions()` returning model-specific size (768 for nomic-embed-text)
- Keep all existing methods for backward compatibility

### Step 3: Create LiteLLM Adapter

Create `internal/embedding/litellm.go`:

```go
type LiteLLMClient struct {
    baseURL    string  // e.g. "http://localhost:4000"
    apiKey     string
    model      string  // e.g. "text-embedding-3-small"
    dimensions int
    httpClient *http.Client
}
```

OpenAI-compatible `/v1/embeddings` endpoint:
- POST `{base_url}/v1/embeddings`
- Headers: `Authorization: Bearer {api_key}`
- Body: `{"model": "...", "input": ["text1", "text2"]}`
- Response: `{"data": [{"embedding": [...]}]}`

### Step 4: Create Provider Factory

Create `internal/embedding/provider.go`:

```go
type ProviderConfig struct {
    Provider   string // "ollama", "litellm", "off"
    OllamaURL  string // default: http://localhost:11434
    LiteLLMURL string // default: http://localhost:4000
    LiteLLMKey string // API key for LiteLLM
    Model      string // model name (provider-specific default if empty)
}

func NewEmbedder(cfg ProviderConfig) (Embedder, error)
func LoadConfigFromEnv() ProviderConfig
```

Environment variables:
- `CODETECT_EMBEDDING_PROVIDER` (ollama|litellm|off, default: ollama)
- `CODETECT_OLLAMA_URL` (default: http://localhost:11434)
- `CODETECT_LITELLM_URL` (default: http://localhost:4000)
- `CODETECT_LITELLM_API_KEY`
- `CODETECT_EMBEDDING_MODEL` (overrides provider default)

### Step 5: Update SemanticSearcher

Modify `internal/embedding/search.go`:
- Change `client *OllamaClient` → `embedder Embedder`
- Update `NewSemanticSearcher(db, embedder Embedder)`
- Use `embedder.ProviderID()` when storing/querying embeddings
- Keep all method signatures unchanged

### Step 6: Update Integration Points

**`cmd/codetect-index/main.go`**:
```go
func runEmbed(args []string) {
    cfg := embedding.LoadConfigFromEnv()
    // Override from flags if provided
    embedder, err := embedding.NewEmbedder(cfg)
    searcher, err := embedding.NewSemanticSearcher(db, embedder)
    // Rest unchanged
}
```

**`internal/tools/semantic.go`**:
```go
func openSemanticSearcher() (*embedding.SemanticSearcher, error) {
    cfg := embedding.LoadConfigFromEnv()
    embedder, err := embedding.NewEmbedder(cfg)
    return embedding.NewSemanticSearcher(db, embedder)
}
```

### Step 7: Handle Provider/Model Versioning

The existing schema already supports this via the `model` field:
- Change `model` to store `ProviderID()` (e.g., "ollama:nomic-embed-text")
- Existing `HasEmbedding(chunk, model)` check prevents conflicts
- Different providers create separate embedding entries
- `make doctor` reports active provider

## Files to Modify

| File | Change |
|------|--------|
| `internal/embedding/embedder.go` | NEW: Interface definition |
| `internal/embedding/ollama.go` | Add interface methods |
| `internal/embedding/litellm.go` | NEW: LiteLLM adapter |
| `internal/embedding/provider.go` | NEW: Factory + config |
| `internal/embedding/search.go` | Use Embedder interface |
| `cmd/codetect-index/main.go` | Use provider factory |
| `internal/tools/semantic.go` | Use provider factory |
| `Makefile` | Update doctor to show provider |

## Risks

1. **Dimension mismatch**: Different models have different embedding sizes
   - Mitigation: Store dimensions in DB, validate on query

2. **Breaking existing embeddings**: Changing model field format
   - Mitigation: Migration path for existing "nomic-embed-text" → "ollama:nomic-embed-text"

3. **LiteLLM availability**: Network/auth issues
   - Mitigation: Clear error messages, graceful degradation

## Success Criteria

- [ ] `CODETECT_EMBEDDING_PROVIDER=ollama make embed` works (default)
- [ ] `CODETECT_EMBEDDING_PROVIDER=off` disables semantic search gracefully
- [ ] `CODETECT_EMBEDDING_PROVIDER=litellm` with valid endpoint works
- [ ] Existing tests pass
- [ ] New tests for provider factory and LiteLLM adapter
- [ ] `make doctor` shows active embedding provider
- [ ] MCP tool contracts unchanged

## Review Checklist

- [ ] Interface is minimal and stable
- [ ] Ollama behavior unchanged when no config set
- [ ] LiteLLM follows OpenAI embedding API spec
- [ ] Environment variables documented in README
- [ ] No breaking changes to MCP tools
- [ ] Provider ID stored correctly in database
