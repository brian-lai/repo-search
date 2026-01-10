#!/bin/bash
#
# repo-search development setup script
#
# This script is for DEVELOPERS working on the repo-search codebase itself.
# It builds repo-search and configures it to index this repository.
#
# For END USERS installing repo-search:
#   Run: make install
#   Then: cd /path/to/your/project && repo-search init
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Config file location
CONFIG_FILE=".env.repo-search"

echo -e "${CYAN}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║              repo-search development setup                    ║"
echo "║         Configure repo-search to index itself                 ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"
echo ""
warn "This script is for developers working on repo-search itself."
info "End users should run: make install"
echo ""

#
# Helper functions
#
prompt() {
    echo -e "${BLUE}?${NC} $1"
}

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
# Check required dependencies
#
echo -e "\n${CYAN}Checking dependencies...${NC}\n"

# Go
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | cut -d' ' -f3)
    success "Go: $GO_VERSION"
else
    error "Go is not installed"
    info "Install from: https://go.dev/dl/"
    exit 1
fi

# ripgrep
if command -v rg &> /dev/null; then
    RG_VERSION=$(rg --version | head -1)
    success "ripgrep: $RG_VERSION"
else
    error "ripgrep (rg) is not installed"
    info "Install with: brew install ripgrep (macOS) or apt install ripgrep (Ubuntu)"
    exit 1
fi

# ctags (optional)
CTAGS_AVAILABLE=false
if command -v ctags &> /dev/null && ctags --version 2>&1 | grep -q "Universal Ctags"; then
    CTAGS_VERSION=$(ctags --version | head -1 | cut -d',' -f1)
    success "ctags: $CTAGS_VERSION"
    CTAGS_AVAILABLE=true
else
    warn "universal-ctags not found (symbol indexing will be disabled)"
    info "Install with: brew install universal-ctags (macOS)"
fi

#
# Embedding provider selection
#
echo -e "\n${CYAN}Embedding Provider Setup${NC}"
echo -e "Semantic search requires an embedding provider.\n"

echo "Select an embedding provider:"
echo -e "  ${GREEN}1)${NC} Ollama (local, free, recommended)"
echo -e "  ${GREEN}2)${NC} LiteLLM (OpenAI, Azure, Bedrock, etc.)"
echo -e "  ${GREEN}3)${NC} None (disable semantic search)"
echo ""

read -p "$(echo -e ${BLUE}?${NC}) Your choice [1]: " PROVIDER_CHOICE
PROVIDER_CHOICE=${PROVIDER_CHOICE:-1}

EMBEDDING_PROVIDER=""
OLLAMA_URL=""
LITELLM_URL=""
LITELLM_API_KEY=""
EMBEDDING_MODEL=""

case $PROVIDER_CHOICE in
    1)
        EMBEDDING_PROVIDER="ollama"
        echo ""

        # Check if Ollama is installed
        if command -v ollama &> /dev/null; then
            success "Ollama is installed"

            # Check if Ollama is running
            if curl -s http://localhost:11434/api/tags &> /dev/null; then
                success "Ollama is running"

                # Check for nomic-embed-text model
                if curl -s http://localhost:11434/api/tags | grep -q "nomic-embed-text"; then
                    success "nomic-embed-text model is available"
                else
                    warn "nomic-embed-text model not found"
                    read -p "$(echo -e ${BLUE}?${NC}) Pull nomic-embed-text now? [Y/n]: " PULL_MODEL
                    PULL_MODEL=${PULL_MODEL:-Y}
                    if [[ $PULL_MODEL =~ ^[Yy] ]]; then
                        echo "Pulling nomic-embed-text..."
                        ollama pull nomic-embed-text
                        success "Model pulled successfully"
                    fi
                fi
            else
                warn "Ollama is not running"
                info "Start it with: ollama serve"
            fi
        else
            warn "Ollama is not installed"
            info "Install from: https://ollama.ai"
            info "Then run: ollama pull nomic-embed-text"
        fi

        # Custom Ollama URL?
        read -p "$(echo -e ${BLUE}?${NC}) Ollama URL [http://localhost:11434]: " OLLAMA_URL
        OLLAMA_URL=${OLLAMA_URL:-http://localhost:11434}

        # Custom model?
        read -p "$(echo -e ${BLUE}?${NC}) Embedding model [nomic-embed-text]: " EMBEDDING_MODEL
        EMBEDDING_MODEL=${EMBEDDING_MODEL:-nomic-embed-text}
        ;;

    2)
        EMBEDDING_PROVIDER="litellm"
        echo ""

        info "LiteLLM provides a unified API for multiple embedding providers."
        info "See: https://github.com/BerriAI/litellm"
        echo ""

        read -p "$(echo -e ${BLUE}?${NC}) LiteLLM URL [http://localhost:4000]: " LITELLM_URL
        LITELLM_URL=${LITELLM_URL:-http://localhost:4000}

        read -p "$(echo -e ${BLUE}?${NC}) API Key (leave empty if not required): " LITELLM_API_KEY

        read -p "$(echo -e ${BLUE}?${NC}) Embedding model [text-embedding-3-small]: " EMBEDDING_MODEL
        EMBEDDING_MODEL=${EMBEDDING_MODEL:-text-embedding-3-small}

        # Test connection
        echo ""
        if curl -s "${LITELLM_URL}/health" &> /dev/null; then
            success "LiteLLM is reachable at $LITELLM_URL"
        else
            warn "Could not connect to LiteLLM at $LITELLM_URL"
            info "Make sure the server is running before using 'make embed'"
        fi
        ;;

    3)
        EMBEDDING_PROVIDER="off"
        echo ""
        warn "Semantic search will be disabled"
        info "You can enable it later by setting REPO_SEARCH_EMBEDDING_PROVIDER"
        ;;

    *)
        error "Invalid choice"
        exit 1
        ;;
esac

#
# Generate config file
#
echo -e "\n${CYAN}Generating configuration...${NC}\n"

cat > "$CONFIG_FILE" << EOF
# repo-search configuration
# Source this file or add to your shell profile:
#   source .env.repo-search

# Embedding provider: ollama, litellm, or off
export REPO_SEARCH_EMBEDDING_PROVIDER="$EMBEDDING_PROVIDER"
EOF

if [[ $EMBEDDING_PROVIDER == "ollama" ]]; then
    cat >> "$CONFIG_FILE" << EOF

# Ollama configuration
export REPO_SEARCH_OLLAMA_URL="$OLLAMA_URL"
export REPO_SEARCH_EMBEDDING_MODEL="$EMBEDDING_MODEL"
EOF
elif [[ $EMBEDDING_PROVIDER == "litellm" ]]; then
    cat >> "$CONFIG_FILE" << EOF

# LiteLLM configuration
export REPO_SEARCH_LITELLM_URL="$LITELLM_URL"
export REPO_SEARCH_LITELLM_API_KEY="$LITELLM_API_KEY"
export REPO_SEARCH_EMBEDDING_MODEL="$EMBEDDING_MODEL"
EOF
fi

success "Created $CONFIG_FILE"

#
# Build
#
echo -e "\n${CYAN}Building repo-search...${NC}\n"

make build
success "Build complete"

#
# Initial indexing
#
echo -e "\n${CYAN}Initial Setup${NC}\n"

read -p "$(echo -e ${BLUE}?${NC}) Run symbol indexing now? [Y/n]: " RUN_INDEX
RUN_INDEX=${RUN_INDEX:-Y}

if [[ $RUN_INDEX =~ ^[Yy] ]]; then
    echo ""
    make index
    success "Symbol indexing complete"
fi

if [[ $EMBEDDING_PROVIDER != "off" ]]; then
    echo ""
    read -p "$(echo -e ${BLUE}?${NC}) Generate embeddings now? [Y/n]: " RUN_EMBED
    RUN_EMBED=${RUN_EMBED:-Y}

    if [[ $RUN_EMBED =~ ^[Yy] ]]; then
        echo ""
        # Source the config to use the settings
        source "$CONFIG_FILE"
        make embed
        success "Embedding complete"
    fi
fi

#
# Summary
#
echo -e "\n${CYAN}╔═══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║                    Installation Complete                       ║${NC}"
echo -e "${CYAN}╚═══════════════════════════════════════════════════════════════╝${NC}\n"

echo -e "Configuration saved to: ${GREEN}$CONFIG_FILE${NC}"
echo ""
echo "To use these settings in future sessions, add to your shell profile:"
echo -e "  ${YELLOW}echo 'source $(pwd)/$CONFIG_FILE' >> ~/.zshrc${NC}"
echo ""
echo "Quick reference:"
echo -e "  ${GREEN}make doctor${NC}    - Check dependencies"
echo -e "  ${GREEN}make index${NC}     - Index symbols"
echo -e "  ${GREEN}make embed${NC}     - Generate embeddings"
echo -e "  ${GREEN}make stats${NC}     - Show index statistics"
echo ""

if [[ $EMBEDDING_PROVIDER == "ollama" ]]; then
    echo -e "Embedding provider: ${GREEN}Ollama${NC} ($EMBEDDING_MODEL)"
elif [[ $EMBEDDING_PROVIDER == "litellm" ]]; then
    echo -e "Embedding provider: ${GREEN}LiteLLM${NC} ($EMBEDDING_MODEL)"
else
    echo -e "Embedding provider: ${YELLOW}Disabled${NC}"
fi

echo ""
echo -e "To use with Claude Code, the ${GREEN}.mcp.json${NC} file is already configured."
echo "Just run Claude Code in this directory!"
echo ""
