#!/bin/bash
#
# repo-search - Global wrapper script for repo-search MCP server
#
# Commands:
#   mcp       Start MCP server (used by .mcp.json)
#   index     Index symbols in current directory
#   embed     Generate embeddings for semantic search
#   init      Initialize repo-search in current directory
#   doctor    Check installation and dependencies
#   stats     Show index statistics
#   help      Show this help message
#

set -e

# Installation paths
INSTALL_PREFIX="${REPO_SEARCH_PREFIX:-$HOME/.local}"
BIN_DIR="$INSTALL_PREFIX/bin"
SHARE_DIR="$INSTALL_PREFIX/share/repo-search"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/repo-search"
CONFIG_FILE="$CONFIG_DIR/config.env"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

#
# Helper functions
#
success() {
    echo -e "${GREEN}✓${NC} $1"
}

warn() {
    echo -e "${YELLOW}!${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

info() {
    echo -e "  $1"
}

#
# Load global config if exists
#
load_config() {
    if [[ -f "$CONFIG_FILE" ]]; then
        source "$CONFIG_FILE"
    fi
}

#
# Commands
#

cmd_mcp() {
    load_config
    exec "$BIN_DIR/repo-search-mcp" "$@"
}

cmd_index() {
    load_config
    local target_dir="${1:-.}"

    echo -e "${CYAN}Indexing symbols in: ${target_dir}${NC}"
    "$BIN_DIR/repo-search-index" index "$target_dir"
    success "Symbol indexing complete"
}

cmd_embed() {
    load_config
    local target_dir="${1:-.}"

    if [[ "$REPO_SEARCH_EMBEDDING_PROVIDER" == "off" ]]; then
        warn "Embedding provider is disabled"
        info "Set REPO_SEARCH_EMBEDDING_PROVIDER in $CONFIG_FILE to enable"
        return 0
    fi

    echo -e "${CYAN}Generating embeddings in: ${target_dir}${NC}"
    "$BIN_DIR/repo-search-index" embed "$target_dir"
    success "Embedding complete"
}

cmd_init() {
    local force=false
    if [[ "$1" == "-f" || "$1" == "--force" ]]; then
        force=true
    fi

    if [[ -f ".mcp.json" && "$force" != "true" ]]; then
        warn ".mcp.json already exists"
        info "Use 'repo-search init --force' to overwrite"
        return 1
    fi

    # Check if template exists
    local template="$SHARE_DIR/templates/mcp.json"
    if [[ ! -f "$template" ]]; then
        # Fallback: create inline
        cat > .mcp.json << 'EOF'
{
  "mcpServers": {
    "repo-search": {
      "command": "repo-search",
      "args": ["mcp"]
    }
  }
}
EOF
    else
        cp "$template" .mcp.json
    fi

    success "Created .mcp.json"
    info "Run 'repo-search index' to index this codebase"

    # Add to .gitignore if exists
    if [[ -f ".gitignore" ]]; then
        if ! grep -q "^\.repo_search/$" .gitignore 2>/dev/null; then
            echo "" >> .gitignore
            echo "# repo-search index" >> .gitignore
            echo ".repo_search/" >> .gitignore
            success "Added .repo_search/ to .gitignore"
        fi
    fi
}

cmd_doctor() {
    load_config

    echo -e "${CYAN}repo-search Installation Check${NC}"
    echo ""

    # Check binaries
    echo "Binaries:"
    if [[ -x "$BIN_DIR/repo-search-mcp" ]]; then
        success "repo-search-mcp: $BIN_DIR/repo-search-mcp"
    else
        error "repo-search-mcp not found at $BIN_DIR/repo-search-mcp"
    fi

    if [[ -x "$BIN_DIR/repo-search-index" ]]; then
        success "repo-search-index: $BIN_DIR/repo-search-index"
    else
        error "repo-search-index not found at $BIN_DIR/repo-search-index"
    fi
    echo ""

    # Check dependencies
    echo "Dependencies:"
    if command -v rg &> /dev/null; then
        RG_VERSION=$(rg --version | head -1)
        success "ripgrep: $RG_VERSION"
    else
        error "ripgrep (rg) not found"
    fi

    if command -v ctags &> /dev/null && ctags --version 2>&1 | grep -q "Universal Ctags"; then
        CTAGS_VERSION=$(ctags --version | head -1 | cut -d',' -f1)
        success "ctags: $CTAGS_VERSION"
    else
        warn "universal-ctags not found (symbol indexing will be limited)"
    fi
    echo ""

    # Check embedding provider
    echo "Embedding Provider:"
    local provider="${REPO_SEARCH_EMBEDDING_PROVIDER:-ollama}"

    case "$provider" in
        ollama)
            echo -e "  Provider: ${GREEN}Ollama${NC}"
            local ollama_url="${REPO_SEARCH_OLLAMA_URL:-http://localhost:11434}"
            if curl -s "$ollama_url/api/tags" &> /dev/null; then
                success "Ollama is running at $ollama_url"
                local model="${REPO_SEARCH_EMBEDDING_MODEL:-nomic-embed-text}"
                if curl -s "$ollama_url/api/tags" | grep -q "$model"; then
                    success "Model '$model' is available"
                else
                    warn "Model '$model' not found"
                fi
            else
                warn "Ollama not running at $ollama_url"
            fi
            ;;
        litellm)
            echo -e "  Provider: ${GREEN}LiteLLM${NC}"
            local litellm_url="${REPO_SEARCH_LITELLM_URL:-http://localhost:4000}"
            if curl -s "$litellm_url/health" &> /dev/null; then
                success "LiteLLM is running at $litellm_url"
            else
                warn "LiteLLM not running at $litellm_url"
            fi
            ;;
        off)
            echo -e "  Provider: ${YELLOW}Disabled${NC}"
            info "Semantic search is disabled"
            ;;
        *)
            error "Unknown provider: $provider"
            ;;
    esac
    echo ""

    # Check config
    echo "Configuration:"
    if [[ -f "$CONFIG_FILE" ]]; then
        success "Config file: $CONFIG_FILE"
    else
        info "No global config (using defaults)"
        info "Create with: mkdir -p $CONFIG_DIR && touch $CONFIG_FILE"
    fi
    echo ""

    # Check current directory
    echo "Current Directory:"
    if [[ -f ".mcp.json" ]]; then
        success ".mcp.json exists"
    else
        info "No .mcp.json (run 'repo-search init' to create)"
    fi

    if [[ -d ".repo_search" ]]; then
        success ".repo_search/ index exists"
        if [[ -f ".repo_search/symbols.db" ]]; then
            local size=$(du -h ".repo_search/symbols.db" | cut -f1)
            info "Database size: $size"
        fi
    else
        info "No index (run 'repo-search index' to create)"
    fi
}

cmd_stats() {
    load_config

    if [[ ! -f ".repo_search/symbols.db" ]]; then
        error "No index found. Run 'repo-search index' first."
        return 1
    fi

    echo -e "${CYAN}Index Statistics${NC}"
    echo ""

    # Symbol count
    local symbols=$(sqlite3 ".repo_search/symbols.db" "SELECT COUNT(*) FROM symbols" 2>/dev/null || echo "0")
    echo "Symbols: $symbols"

    # File count
    local files=$(sqlite3 ".repo_search/symbols.db" "SELECT COUNT(DISTINCT path) FROM symbols" 2>/dev/null || echo "0")
    echo "Files with symbols: $files"

    # Embedding count
    local embeddings=$(sqlite3 ".repo_search/symbols.db" "SELECT COUNT(*) FROM embeddings" 2>/dev/null || echo "0")
    echo "Embedded chunks: $embeddings"

    # Database size
    local size=$(du -h ".repo_search/symbols.db" | cut -f1)
    echo "Database size: $size"
}

cmd_help() {
    echo -e "${CYAN}repo-search${NC} - MCP server for codebase search & navigation"
    echo ""
    echo "Usage: repo-search <command> [options]"
    echo ""
    echo "Commands:"
    echo "  mcp           Start MCP server (used by .mcp.json)"
    echo "  index [path]  Index symbols (default: current directory)"
    echo "  embed [path]  Generate embeddings for semantic search"
    echo "  init [-f]     Create .mcp.json in current directory"
    echo "  doctor        Check installation and dependencies"
    echo "  stats         Show index statistics"
    echo "  help          Show this help message"
    echo ""
    echo "Configuration:"
    echo "  Global config: $CONFIG_FILE"
    echo "  Per-project:   .mcp.json (created by 'init')"
    echo ""
    echo "Quick Start:"
    echo "  repo-search init      # Initialize project"
    echo "  repo-search index     # Index symbols"
    echo "  repo-search embed     # Generate embeddings (optional)"
    echo "  claude                # Start Claude Code"
}

#
# Main
#
main() {
    local cmd="${1:-help}"
    shift || true

    case "$cmd" in
        mcp)
            cmd_mcp "$@"
            ;;
        index)
            cmd_index "$@"
            ;;
        embed)
            cmd_embed "$@"
            ;;
        init)
            cmd_init "$@"
            ;;
        doctor)
            cmd_doctor "$@"
            ;;
        stats)
            cmd_stats "$@"
            ;;
        help|--help|-h)
            cmd_help
            ;;
        *)
            error "Unknown command: $cmd"
            echo ""
            cmd_help
            exit 1
            ;;
    esac
}

main "$@"
