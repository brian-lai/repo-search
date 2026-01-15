BINARY=dist/codetect
INDEXER=dist/codetect-index
DAEMON=dist/codetect-daemon
EVAL=dist/codetect-eval
MIGRATE=dist/migrate-to-postgres

# Installation prefix (default: ~/.local)
PREFIX ?= $(HOME)/.local
BIN_DIR = $(PREFIX)/bin
SHARE_DIR = $(PREFIX)/share/codetect

.PHONY: build mcp index embed doctor clean test bench bench-all install uninstall eval migrate-to-postgres postgres-up postgres-down postgres-logs postgres-shell

# Build all binaries
build:
	@mkdir -p dist
	go build -o $(BINARY) ./cmd/codetect
	go build -o $(INDEXER) ./cmd/codetect-index
	go build -o $(DAEMON) ./cmd/codetect-daemon
	go build -o $(EVAL) ./cmd/codetect-eval
	go build -o $(MIGRATE) ./cmd/migrate-to-postgres

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

# Migrate SQLite embeddings to PostgreSQL
migrate-to-postgres: build
	@if [ -z "$$CODETECT_DB_DSN" ]; then \
		echo "Error: CODETECT_DB_DSN not set"; \
		echo ""; \
		echo "Please configure PostgreSQL:"; \
		echo "  export CODETECT_DB_TYPE=postgres"; \
		echo "  export CODETECT_DB_DSN=\"postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable\""; \
		echo ""; \
		echo "Start PostgreSQL: make postgres-up"; \
		exit 1; \
	fi
	@echo "Migrating SQLite embeddings to PostgreSQL..."
	@./$(MIGRATE)

# PostgreSQL helpers
postgres-up:
	@echo "Starting PostgreSQL with Docker..."
	@docker-compose up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 2
	@docker-compose ps
	@echo ""
	@echo "✓ PostgreSQL is running"
	@echo ""
	@echo "Set environment variables:"
	@echo "  export CODETECT_DB_TYPE=postgres"
	@echo "  export CODETECT_DB_DSN=\"postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable\""

postgres-down:
	@echo "Stopping PostgreSQL..."
	@docker-compose down

postgres-logs:
	@docker-compose logs -f postgres

postgres-shell:
	@docker-compose exec postgres psql -U codetect

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
	@PROVIDER=$${CODETECT_EMBEDDING_PROVIDER:-ollama}; \
	echo "  Provider: $$PROVIDER"; \
	if [ "$$PROVIDER" = "off" ]; then \
		echo "  Status: disabled"; \
	elif [ "$$PROVIDER" = "litellm" ]; then \
		LITELLM_URL=$${CODETECT_LITELLM_URL:-http://localhost:4000}; \
		echo "  URL: $$LITELLM_URL"; \
		if curl -s "$$LITELLM_URL/health" >/dev/null 2>&1; then \
			echo "✓ litellm: available"; \
		else \
			echo "○ litellm: not available at $$LITELLM_URL"; \
		fi \
	else \
		if command -v ollama >/dev/null 2>&1; then \
			echo "✓ ollama: $$(ollama --version 2>/dev/null || echo 'installed')"; \
			OLLAMA_URL=$${CODETECT_OLLAMA_URL:-http://localhost:11434}; \
			MODEL=$${CODETECT_EMBEDDING_MODEL:-nomic-embed-text}; \
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

# Run benchmarks (requires PostgreSQL)
bench:
	@echo "Running vector search benchmarks..."
	@echo "Note: Requires PostgreSQL. Start with: make postgres-up"
	@echo ""
	@POSTGRES_TEST_DSN=$${POSTGRES_TEST_DSN:-"postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"} \
		go test -bench=BenchmarkVectorSearch -benchtime=3s -run=^$$ ./internal/db

bench-all:
	@echo "Running all benchmarks..."
	@echo "Note: Requires PostgreSQL. Start with: make postgres-up"
	@echo ""
	@POSTGRES_TEST_DSN=$${POSTGRES_TEST_DSN:-"postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"} \
		go test -bench=. -benchtime=3s -run=^$$ ./internal/db

# Clean build artifacts
clean:
	rm -rf dist .codetect

# Install globally
install: build
	@echo "Installing to $(PREFIX)..."
	@mkdir -p $(BIN_DIR) $(SHARE_DIR)/templates
	@cp $(BINARY) $(BIN_DIR)/codetect-mcp
	@cp $(INDEXER) $(BIN_DIR)/codetect-index
	@cp $(DAEMON) $(BIN_DIR)/codetect-daemon
	@cp $(EVAL) $(BIN_DIR)/codetect-eval
	@cp $(MIGRATE) $(BIN_DIR)/migrate-to-postgres
	@cp scripts/codetect-wrapper.sh $(BIN_DIR)/codetect
	@chmod +x $(BIN_DIR)/codetect $(BIN_DIR)/codetect-mcp $(BIN_DIR)/codetect-index $(BIN_DIR)/codetect-daemon $(BIN_DIR)/codetect-eval $(BIN_DIR)/migrate-to-postgres
	@cp templates/mcp.json $(SHARE_DIR)/templates/
	@echo ""
	@echo "✓ Installed to $(PREFIX)"
	@echo ""
	@echo "Make sure $(BIN_DIR) is in your PATH:"
	@echo "  export PATH=\"$(BIN_DIR):\$$PATH\""
	@echo ""
	@echo "Quick start:"
	@echo "  cd /path/to/your/project"
	@echo "  codetect init"
	@echo "  codetect index"
	@echo "  codetect daemon start"
	@echo ""
	@echo "Evaluation:"
	@echo "  codetect-eval run --verbose    # Run evaluation tests"
	@echo "  codetect-eval list             # List test cases"
	@echo "  codetect-eval report           # Show latest report"

# Uninstall
uninstall:
	@echo "Uninstalling from $(PREFIX)..."
	@rm -f $(BIN_DIR)/codetect $(BIN_DIR)/codetect-mcp $(BIN_DIR)/codetect-index $(BIN_DIR)/codetect-daemon $(BIN_DIR)/codetect-eval $(BIN_DIR)/migrate-to-postgres
	@rm -rf $(SHARE_DIR)
	@echo "✓ Uninstalled"

# Run MCP evaluation tests
eval: build
	@echo "Running MCP evaluation..."
	@./$(EVAL) run --verbose

# List evaluation test cases
eval-list: build
	@./$(EVAL) list

# Show latest evaluation report
eval-report: build
	@./$(EVAL) report
