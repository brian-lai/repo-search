BINARY=dist/repo-search
INDEXER=dist/repo-search-index

.PHONY: build mcp index embed doctor clean test

# Build both binaries
build:
	@mkdir -p dist
	go build -o $(BINARY) ./cmd/repo-search
	go build -o $(INDEXER) ./cmd/repo-search-index

# Run MCP server (used by .mcp.json)
mcp: build
	@./$(BINARY)

# Run symbol indexer
index: build
	@./$(INDEXER) index .

# Generate embeddings (requires Ollama)
embed: build
	@./$(INDEXER) embed .

# Run both index and embed
index-all: index embed

# Check dependencies
doctor:
	@echo "Checking dependencies..."
	@echo ""
	@echo "=== Required ==="
	@command -v go >/dev/null 2>&1 || { echo "❌ missing: go"; exit 1; }
	@echo "✓ go: $$(go version | cut -d' ' -f3)"
	@command -v rg >/dev/null 2>&1 || { echo "❌ missing: ripgrep (rg)"; exit 1; }
	@echo "✓ ripgrep: $$(rg --version | head -1)"
	@echo ""
	@echo "=== Optional (for symbol indexing) ==="
	@if command -v ctags >/dev/null 2>&1 && ctags --version 2>&1 | grep -q "Universal Ctags"; then \
		echo "✓ ctags: $$(ctags --version | head -1)"; \
	else \
		echo "○ ctags: not found (symbol indexing disabled)"; \
		echo "  Install with: brew install universal-ctags"; \
	fi
	@echo ""
	@echo "=== Optional (for semantic search) ==="
	@if command -v ollama >/dev/null 2>&1; then \
		echo "✓ ollama: $$(ollama --version 2>/dev/null || echo 'installed')"; \
		if curl -s http://localhost:11434/api/tags 2>/dev/null | grep -q "nomic-embed-text"; then \
			echo "✓ nomic-embed-text: model available"; \
		else \
			echo "○ nomic-embed-text: model not pulled"; \
			echo "  Run: ollama pull nomic-embed-text"; \
		fi \
	else \
		echo "○ ollama: not found (semantic search disabled)"; \
		echo "  Install from: https://ollama.ai"; \
	fi
	@echo ""
	@echo "All required dependencies satisfied ✓"

# Show index stats
stats: build
	@./$(INDEXER) stats .

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf dist .repo_search
