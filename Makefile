BINARY=dist/repo-search
INDEXER=dist/repo-search-index

.PHONY: build mcp index doctor clean test

# Build both binaries
build:
	@mkdir -p dist
	go build -o $(BINARY) ./cmd/repo-search
	go build -o $(INDEXER) ./cmd/repo-search-index

# Run MCP server (used by .mcp.json)
mcp: build
	@./$(BINARY)

# Run indexer
index: build
	@./$(INDEXER) index .

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
	@echo "All required dependencies satisfied ✓"

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf dist .repo_search
