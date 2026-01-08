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
	@echo "=== Embedding Provider ==="
	@PROVIDER=$${REPO_SEARCH_EMBEDDING_PROVIDER:-ollama}; \
	echo "  Provider: $$PROVIDER"; \
	if [ "$$PROVIDER" = "off" ]; then \
		echo "  Status: disabled"; \
	elif [ "$$PROVIDER" = "litellm" ]; then \
		LITELLM_URL=$${REPO_SEARCH_LITELLM_URL:-http://localhost:4000}; \
		echo "  URL: $$LITELLM_URL"; \
		if curl -s "$$LITELLM_URL/health" >/dev/null 2>&1; then \
			echo "✓ litellm: available"; \
		else \
			echo "○ litellm: not available at $$LITELLM_URL"; \
		fi \
	else \
		if command -v ollama >/dev/null 2>&1; then \
			echo "✓ ollama: $$(ollama --version 2>/dev/null || echo 'installed')"; \
			OLLAMA_URL=$${REPO_SEARCH_OLLAMA_URL:-http://localhost:11434}; \
			MODEL=$${REPO_SEARCH_EMBEDDING_MODEL:-nomic-embed-text}; \
			if curl -s "$$OLLAMA_URL/api/tags" 2>/dev/null | grep -q "$$MODEL"; then \
				echo "✓ $$MODEL: model available"; \
			else \
				echo "○ $$MODEL: model not pulled"; \
				echo "  Run: ollama pull $$MODEL"; \
			fi \
		else \
			echo "○ ollama: not found (semantic search disabled)"; \
			echo "  Install from: https://ollama.ai"; \
		fi \
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
