#!/bin/bash
#
# repo-search installation script
# Interactive setup for the MCP server
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Config file location
CONFIG_FILE=".env.repo-search"

# Detect platform
OS="$(uname -s)"
case "${OS}" in
    Linux*)     PLATFORM=Linux;;
    Darwin*)    PLATFORM=Mac;;
    *)          PLATFORM="UNKNOWN";;
esac

# Detect package manager
if command -v brew &> /dev/null; then
    PKG_MGR="brew"
elif command -v apt-get &> /dev/null; then
    PKG_MGR="apt"
elif command -v dnf &> /dev/null; then
    PKG_MGR="dnf"
elif command -v pacman &> /dev/null; then
    PKG_MGR="pacman"
else
    PKG_MGR="unknown"
fi

#
# Helper functions
#
print_header() {
    echo -e "\n${CYAN}${BOLD}"
    echo "╔════════════════════════════════════════════════════════════════════╗"
    printf "║ %-66s ║\n" "$1"
    echo "╚════════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

print_section() {
    echo -e "\n${MAGENTA}${BOLD}▸ $1${NC}\n"
}

print_box() {
    local color=$1
    shift
    echo -e "\n${color}┌────────────────────────────────────────────────────────────────────┐${NC}"
    for line in "$@"; do
        printf "${color}│${NC} %-66s ${color}│${NC}\n" "$line"
    done
    echo -e "${color}└────────────────────────────────────────────────────────────────────┘${NC}\n"
}

prompt() {
    echo -e "${BLUE}${BOLD}?${NC} $1"
}

success() {
    echo -e "${GREEN}${BOLD}✓${NC} $1"
}

warn() {
    echo -e "${YELLOW}${BOLD}!${NC} $1"
}

error() {
    echo -e "${RED}${BOLD}✗${NC} $1"
}

info() {
    echo -e "  ${CYAN}→${NC} $1"
}

step() {
    echo -e "${CYAN}${BOLD}[$1/$2]${NC} $3"
}

# Clear screen for a clean start
clear

# Welcome banner
echo -e "${CYAN}${BOLD}"
cat << 'EOF'
╔════════════════════════════════════════════════════════════════════╗
║                                                                    ║
║                        repo-search installer                       ║
║                                                                    ║
║            MCP server for codebase search & navigation             ║
║                                                                    ║
╚════════════════════════════════════════════════════════════════════╝
EOF
echo -e "${NC}"

echo -e "${CYAN}Platform:${NC} $PLATFORM    ${CYAN}Package Manager:${NC} $PKG_MGR"
echo ""

# Check if this is a re-install
REINSTALL=false
if command -v repo-search &> /dev/null; then
    EXISTING_VERSION=$(repo-search --version 2>/dev/null || echo "unknown")
    warn "repo-search is already installed ($EXISTING_VERSION)"
    echo ""
    info "This will reinstall/update repo-search."
    info "Your existing configuration will be preserved."
    echo ""
    read -p "$(prompt "Continue with installation? [Y/n]")" CONTINUE_INSTALL
    CONTINUE_INSTALL=${CONTINUE_INSTALL:-Y}
    if [[ ! $CONTINUE_INSTALL =~ ^[Yy] ]]; then
        echo ""
        info "Installation cancelled"
        exit 0
    fi
    REINSTALL=true
fi

#
# Step 1: Check required dependencies
#
print_header "Step 1/6: Checking Required Dependencies"

# Go
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | cut -d' ' -f3)
    success "Go $GO_VERSION"
else
    error "Go is not installed"
    info "Install from: ${BOLD}https://go.dev/dl/${NC}"
    exit 1
fi

# ripgrep
if command -v rg &> /dev/null; then
    RG_VERSION=$(rg --version | head -1 | awk '{print $2}')
    success "ripgrep $RG_VERSION"
else
    error "ripgrep (rg) is not installed"
    echo ""
    info "ripgrep is required for keyword search."

    case $PKG_MGR in
        brew)
            info "Install with: ${BOLD}brew install ripgrep${NC}"
            ;;
        apt)
            info "Install with: ${BOLD}sudo apt install ripgrep${NC}"
            ;;
        dnf)
            info "Install with: ${BOLD}sudo dnf install ripgrep${NC}"
            ;;
        pacman)
            info "Install with: ${BOLD}sudo pacman -S ripgrep${NC}"
            ;;
        *)
            info "Install from: ${BOLD}https://github.com/BurntSushi/ripgrep${NC}"
            ;;
    esac
    exit 1
fi

#
# Step 2: Optional Features Setup
#
print_header "Step 2/6: Optional Features Setup"

# Symbol Indexing
print_section "Symbol Indexing (enables find_symbol, list_defs_in_file)"

ENABLE_SYMBOLS=false
CTAGS_AVAILABLE=false

if command -v ctags &> /dev/null && ctags --version 2>&1 | grep -q "Universal Ctags"; then
    CTAGS_VERSION=$(ctags --version | head -1 | cut -d',' -f1)
    success "universal-ctags already installed: $CTAGS_VERSION"
    CTAGS_AVAILABLE=true
    ENABLE_SYMBOLS=true
else
    warn "universal-ctags is not installed"
    echo ""
    info "Symbol indexing allows you to search for functions, types, classes,"
    info "and other code symbols by name. This enables fast navigation in large"
    info "codebases."
    echo ""

    read -p "$(prompt "Enable symbol indexing? [Y/n]")" INSTALL_CTAGS
    INSTALL_CTAGS=${INSTALL_CTAGS:-Y}

    if [[ $INSTALL_CTAGS =~ ^[Yy] ]]; then
        echo ""
        case $PKG_MGR in
            brew)
                info "Installing universal-ctags via Homebrew..."
                if brew install universal-ctags; then
                    success "universal-ctags installed successfully"
                    CTAGS_AVAILABLE=true
                    ENABLE_SYMBOLS=true
                else
                    error "Failed to install universal-ctags"
                    warn "Symbol indexing will be disabled"
                fi
                ;;
            apt)
                info "Installing universal-ctags..."
                info "This requires sudo access."
                if sudo apt-get update && sudo apt-get install -y universal-ctags; then
                    success "universal-ctags installed successfully"
                    CTAGS_AVAILABLE=true
                    ENABLE_SYMBOLS=true
                else
                    error "Failed to install universal-ctags"
                    warn "Symbol indexing will be disabled"
                fi
                ;;
            dnf)
                info "Installing ctags..."
                info "This requires sudo access."
                if sudo dnf install -y ctags; then
                    success "ctags installed successfully"
                    CTAGS_AVAILABLE=true
                    ENABLE_SYMBOLS=true
                else
                    error "Failed to install ctags"
                    warn "Symbol indexing will be disabled"
                fi
                ;;
            pacman)
                info "Installing ctags..."
                info "This requires sudo access."
                if sudo pacman -S --noconfirm ctags; then
                    success "ctags installed successfully"
                    CTAGS_AVAILABLE=true
                    ENABLE_SYMBOLS=true
                else
                    error "Failed to install ctags"
                    warn "Symbol indexing will be disabled"
                fi
                ;;
            *)
                warn "Automatic installation not supported on this platform"
                info "Install manually from: ${BOLD}https://github.com/universal-ctags/ctags${NC}"
                info "Symbol indexing will be disabled for now"
                ;;
        esac
    else
        warn "Skipping symbol indexing setup"
        info "You can install universal-ctags later and run 'repo-search index'"
    fi
fi

#
# Step 3: Semantic Search Setup
#
print_header "Step 3/6: Semantic Search Setup"

print_section "Semantic Search (enables search_semantic, hybrid_search)"

echo "Semantic search allows natural language queries like:"
info "\"error handling logic\""
info "\"authentication and authorization\""
info "\"database connection pooling\""
echo ""

read -p "$(prompt "Enable semantic search? [Y/n]")" ENABLE_SEMANTIC
ENABLE_SEMANTIC=${ENABLE_SEMANTIC:-Y}

EMBEDDING_PROVIDER=""
OLLAMA_URL=""
LITELLM_URL=""
LITELLM_API_KEY=""
EMBEDDING_MODEL=""

if [[ $ENABLE_SEMANTIC =~ ^[Yy] ]]; then
    echo ""
    echo "Select an embedding provider:"
    echo -e "  ${GREEN}${BOLD}1)${NC} Ollama (local, free, recommended)"
    echo -e "  ${GREEN}${BOLD}2)${NC} LiteLLM (OpenAI, Azure, Bedrock, etc.)"
    echo ""

    read -p "$(prompt "Your choice [1]")" PROVIDER_CHOICE
    PROVIDER_CHOICE=${PROVIDER_CHOICE:-1}

    case $PROVIDER_CHOICE in
        1)
            EMBEDDING_PROVIDER="ollama"
            echo ""

            # Check if Ollama is installed
            OLLAMA_INSTALLED=false
            if command -v ollama &> /dev/null; then
                success "Ollama is installed"
                OLLAMA_INSTALLED=true
            else
                print_box "$RED" \
                    "┃  OLLAMA NOT FOUND  ┃" \
                    "" \
                    "Semantic search requires Ollama to be installed on your system." \
                    "" \
                    "Ollama is a free, local embedding server that runs AI models" \
                    "on your machine without sending data to external services." \
                    "" \
                    "${BOLD}Installation:${NC}" \
                    "  • Visit: ${BOLD}https://ollama.ai${NC}" \
                    "  • Download and install for your platform" \
                    "  • Run: ${BOLD}ollama pull nomic-embed-text${NC}" \
                    "" \
                    "Without Ollama, semantic search features will be disabled."

                read -p "$(prompt "Continue installation without semantic search? [Y/n]")" CONTINUE_WITHOUT
                CONTINUE_WITHOUT=${CONTINUE_WITHOUT:-Y}

                if [[ ! $CONTINUE_WITHOUT =~ ^[Yy] ]]; then
                    echo ""
                    error "Installation cancelled"
                    info "Install Ollama from ${BOLD}https://ollama.ai${NC} and re-run this script"
                    exit 1
                fi

                EMBEDDING_PROVIDER="off"
            fi

            if [[ $OLLAMA_INSTALLED == true ]]; then
                # Check if Ollama is running
                if curl -s http://localhost:11434/api/tags &> /dev/null; then
                    success "Ollama is running"

                    # Check for nomic-embed-text model
                    if curl -s http://localhost:11434/api/tags | grep -q "nomic-embed-text"; then
                        success "nomic-embed-text model is available"
                    else
                        warn "nomic-embed-text model not found"
                        echo ""
                        info "The nomic-embed-text model is recommended for code embeddings."
                        info "Size: ~274MB"
                        echo ""
                        read -p "$(prompt "Pull nomic-embed-text now? [Y/n]")" PULL_MODEL
                        PULL_MODEL=${PULL_MODEL:-Y}
                        if [[ $PULL_MODEL =~ ^[Yy] ]]; then
                            echo ""
                            info "Downloading model (this may take a few minutes)..."
                            if ollama pull nomic-embed-text; then
                                success "Model downloaded successfully"
                            else
                                error "Failed to download model"
                                warn "You can download it later with: ${BOLD}ollama pull nomic-embed-text${NC}"
                            fi
                        fi
                    fi
                else
                    warn "Ollama is not running"
                    info "Start it with: ${BOLD}ollama serve${NC}"
                    info "Or it will start automatically when you run 'repo-search embed'"
                fi

                # Custom Ollama URL?
                echo ""
                read -p "$(prompt "Ollama URL [http://localhost:11434]")" OLLAMA_URL
                OLLAMA_URL=${OLLAMA_URL:-http://localhost:11434}

                # Custom model?
                read -p "$(prompt "Embedding model [nomic-embed-text]")" EMBEDDING_MODEL
                EMBEDDING_MODEL=${EMBEDDING_MODEL:-nomic-embed-text}
            fi
            ;;

        2)
            EMBEDDING_PROVIDER="litellm"
            echo ""

            info "LiteLLM provides a unified API for multiple embedding providers."
            info "Supports: OpenAI, Azure, AWS Bedrock, Google Vertex AI, and more"
            info "Documentation: ${BOLD}https://github.com/BerriAI/litellm${NC}"
            echo ""

            read -p "$(prompt "LiteLLM URL [http://localhost:4000]")" LITELLM_URL
            LITELLM_URL=${LITELLM_URL:-http://localhost:4000}

            read -p "$(prompt "API Key (leave empty if not required)")" LITELLM_API_KEY

            read -p "$(prompt "Embedding model [text-embedding-3-small]")" EMBEDDING_MODEL
            EMBEDDING_MODEL=${EMBEDDING_MODEL:-text-embedding-3-small}

            # Test connection
            echo ""
            if curl -s "${LITELLM_URL}/health" &> /dev/null; then
                success "LiteLLM is reachable at $LITELLM_URL"
            else
                warn "Could not connect to LiteLLM at $LITELLM_URL"
                info "Make sure the server is running before using 'repo-search embed'"
            fi
            ;;

        *)
            error "Invalid choice"
            exit 1
            ;;
    esac
else
    EMBEDDING_PROVIDER="off"
    warn "Semantic search will be disabled"
    info "You can enable it later by setting REPO_SEARCH_EMBEDDING_PROVIDER"
fi

#
# Step 4: Database Setup
#
print_header "Step 4/6: Database Setup"

print_section "Vector Database Backend"

echo "Choose a database backend for storing embeddings:"
echo ""
echo -e "  ${GREEN}${BOLD}1)${NC} SQLite (local, simple, recommended for most users)"
info "Good for: Up to ~10k embeddings, single-user, simple setup"
info "Storage: Local .repo_search/symbols.db file"
echo ""
echo -e "  ${GREEN}${BOLD}2)${NC} PostgreSQL + pgvector (scalable, recommended for teams)"
info "Good for: Large codebases, team deployments, millions of embeddings"
info "Storage: PostgreSQL database with native vector search"
echo ""

read -p "$(prompt "Your choice [1]")" DB_CHOICE
DB_CHOICE=${DB_CHOICE:-1}

DB_TYPE="sqlite"
POSTGRES_DSN=""
POSTGRES_INSTALLED=false

case $DB_CHOICE in
    1)
        DB_TYPE="sqlite"
        success "Using SQLite (default)"
        ;;

    2)
        DB_TYPE="postgres"
        echo ""
        print_section "PostgreSQL + pgvector Setup"

        # Check if PostgreSQL is installed
        if command -v psql &> /dev/null; then
            PG_VERSION=$(psql --version | awk '{print $3}')
            success "PostgreSQL $PG_VERSION is installed"
            POSTGRES_INSTALLED=true
        else
            warn "PostgreSQL is not installed"
            echo ""
            info "PostgreSQL with pgvector extension is required for vector database."
            echo ""

            case $PKG_MGR in
                brew)
                    info "Install with: ${BOLD}brew install postgresql@16${NC}"
                    read -p "$(prompt "Install PostgreSQL now? [Y/n]")" INSTALL_PG
                    INSTALL_PG=${INSTALL_PG:-Y}

                    if [[ $INSTALL_PG =~ ^[Yy] ]]; then
                        echo ""
                        info "Installing PostgreSQL via Homebrew..."
                        if brew install postgresql@16; then
                            success "PostgreSQL installed successfully"
                            info "Starting PostgreSQL service..."
                            brew services start postgresql@16
                            POSTGRES_INSTALLED=true
                        else
                            error "Failed to install PostgreSQL"
                        fi
                    fi
                    ;;
                apt)
                    info "Install with:"
                    info "  ${BOLD}sudo apt-get install -y postgresql postgresql-contrib${NC}"
                    read -p "$(prompt "Install PostgreSQL now? [Y/n]")" INSTALL_PG
                    INSTALL_PG=${INSTALL_PG:-Y}

                    if [[ $INSTALL_PG =~ ^[Yy] ]]; then
                        echo ""
                        info "Installing PostgreSQL..."
                        if sudo apt-get update && sudo apt-get install -y postgresql postgresql-contrib; then
                            success "PostgreSQL installed successfully"
                            info "Starting PostgreSQL service..."
                            sudo systemctl start postgresql
                            sudo systemctl enable postgresql
                            POSTGRES_INSTALLED=true
                        else
                            error "Failed to install PostgreSQL"
                        fi
                    fi
                    ;;
                dnf)
                    info "Install with: ${BOLD}sudo dnf install -y postgresql-server postgresql-contrib${NC}"
                    read -p "$(prompt "Install PostgreSQL now? [Y/n]")" INSTALL_PG
                    INSTALL_PG=${INSTALL_PG:-Y}

                    if [[ $INSTALL_PG =~ ^[Yy] ]]; then
                        echo ""
                        info "Installing PostgreSQL..."
                        if sudo dnf install -y postgresql-server postgresql-contrib; then
                            success "PostgreSQL installed successfully"
                            info "Initializing PostgreSQL..."
                            sudo postgresql-setup --initdb
                            sudo systemctl start postgresql
                            sudo systemctl enable postgresql
                            POSTGRES_INSTALLED=true
                        else
                            error "Failed to install PostgreSQL"
                        fi
                    fi
                    ;;
                *)
                    warn "Automatic installation not supported on this platform"
                    info "Install PostgreSQL manually from:"
                    info "  • ${BOLD}https://www.postgresql.org/download/${NC}"
                    ;;
            esac
        fi

        if [[ $POSTGRES_INSTALLED == true ]]; then
            # Check for pgvector extension
            echo ""
            info "Checking for pgvector extension..."

            # Try to check if pgvector is available (this will fail gracefully)
            HAS_PGVECTOR=false
            if psql -U postgres -c "SELECT * FROM pg_available_extensions WHERE name='vector'" 2>/dev/null | grep -q vector; then
                success "pgvector extension is available"
                HAS_PGVECTOR=true
            else
                warn "pgvector extension not found"
                echo ""
                info "pgvector adds native vector similarity search to PostgreSQL."
                info "It's required for efficient semantic search at scale."
                echo ""

                case $PKG_MGR in
                    brew)
                        info "Install with: ${BOLD}brew install pgvector${NC}"
                        read -p "$(prompt "Install pgvector now? [Y/n]")" INSTALL_PGVECTOR
                        INSTALL_PGVECTOR=${INSTALL_PGVECTOR:-Y}

                        if [[ $INSTALL_PGVECTOR =~ ^[Yy] ]]; then
                            echo ""
                            info "Installing pgvector..."
                            if brew install pgvector; then
                                success "pgvector installed successfully"
                                HAS_PGVECTOR=true
                            else
                                error "Failed to install pgvector"
                                warn "You'll need to install pgvector manually"
                            fi
                        fi
                        ;;
                    apt)
                        info "Install with:"
                        info "  ${BOLD}sudo apt-get install -y postgresql-16-pgvector${NC}"
                        read -p "$(prompt "Install pgvector now? [Y/n]")" INSTALL_PGVECTOR
                        INSTALL_PGVECTOR=${INSTALL_PGVECTOR:-Y}

                        if [[ $INSTALL_PGVECTOR =~ ^[Yy] ]]; then
                            echo ""
                            info "Installing pgvector..."
                            if sudo apt-get install -y postgresql-16-pgvector; then
                                success "pgvector installed successfully"
                                HAS_PGVECTOR=true
                            else
                                error "Failed to install pgvector"
                                warn "You may need to add the PostgreSQL apt repository first"
                                info "See: ${BOLD}https://github.com/pgvector/pgvector${NC}"
                            fi
                        fi
                        ;;
                    *)
                        warn "Automatic installation not supported"
                        info "Install manually from: ${BOLD}https://github.com/pgvector/pgvector${NC}"
                        ;;
                esac
            fi

            # Get database connection details
            echo ""
            info "PostgreSQL connection configuration:"
            echo ""
            read -p "$(prompt "PostgreSQL host [localhost]")" PG_HOST
            PG_HOST=${PG_HOST:-localhost}

            read -p "$(prompt "PostgreSQL port [5432]")" PG_PORT
            PG_PORT=${PG_PORT:-5432}

            read -p "$(prompt "PostgreSQL user [postgres]")" PG_USER
            PG_USER=${PG_USER:-postgres}

            read -sp "$(prompt "PostgreSQL password (leave empty if not required)")" PG_PASSWORD
            echo ""

            read -p "$(prompt "Database name [repo_search]")" PG_DBNAME
            PG_DBNAME=${PG_DBNAME:-repo_search}

            # Build DSN
            if [[ -z "$PG_PASSWORD" ]]; then
                POSTGRES_DSN="postgres://${PG_USER}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}?sslmode=disable"
            else
                POSTGRES_DSN="postgres://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}?sslmode=disable"
            fi

            # Test connection
            echo ""
            info "Testing connection..."
            if psql "$POSTGRES_DSN" -c "SELECT 1" &>/dev/null; then
                success "Successfully connected to PostgreSQL"

                # Enable pgvector extension if available
                if [[ $HAS_PGVECTOR == true ]]; then
                    info "Enabling pgvector extension..."
                    if psql "$POSTGRES_DSN" -c "CREATE EXTENSION IF NOT EXISTS vector" &>/dev/null; then
                        success "pgvector extension enabled"
                    else
                        warn "Could not enable pgvector extension"
                        info "You may need to enable it manually with: CREATE EXTENSION vector;"
                    fi
                fi
            else
                warn "Could not connect to PostgreSQL"
                info "Make sure:"
                info "  1. PostgreSQL is running"
                info "  2. Database '$PG_DBNAME' exists (or user has CREATE DATABASE permission)"
                info "  3. Connection credentials are correct"
                echo ""
                read -p "$(prompt "Continue anyway? [Y/n]")" CONTINUE_ANYWAY
                CONTINUE_ANYWAY=${CONTINUE_ANYWAY:-Y}
                if [[ ! $CONTINUE_ANYWAY =~ ^[Yy] ]]; then
                    error "Installation cancelled"
                    exit 1
                fi
            fi
        else
            warn "PostgreSQL is not installed"
            info "Falling back to SQLite"
            DB_TYPE="sqlite"
        fi
        ;;

    *)
        error "Invalid choice"
        exit 1
        ;;
esac

#
# Step 5: Build and Install
#
print_header "Step 5/6: Building repo-search"

step 1 3 "Building binaries..."
if make build > /dev/null 2>&1; then
    success "Build complete"
else
    error "Build failed"
    exit 1
fi

echo ""
read -p "$(prompt "Install globally to ~/.local/bin? [Y/n]")" INSTALL_GLOBAL
INSTALL_GLOBAL=${INSTALL_GLOBAL:-Y}

if [[ $INSTALL_GLOBAL =~ ^[Yy] ]]; then
    echo ""
    step 2 3 "Installing globally..."
    if make install > /dev/null 2>&1; then
        success "Installed to ~/.local/bin"

        # Check if ~/.local/bin is in PATH
        if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
            echo ""
            warn "~/.local/bin is not in your PATH"
            info "Add this to your shell profile (~/.zshrc or ~/.bashrc):"
            echo ""
            echo -e "  ${YELLOW}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
            echo ""
            read -p "$(prompt "Add to PATH now? [Y/n]")" ADD_PATH
            ADD_PATH=${ADD_PATH:-Y}

            if [[ $ADD_PATH =~ ^[Yy] ]]; then
                # Detect shell
                if [[ $SHELL == *"zsh"* ]]; then
                    SHELL_RC="$HOME/.zshrc"
                else
                    SHELL_RC="$HOME/.bashrc"
                fi

                echo "" >> "$SHELL_RC"
                echo "# Added by repo-search installer" >> "$SHELL_RC"
                echo "export PATH=\"\$HOME/.local/bin:\$PATH\"" >> "$SHELL_RC"
                success "Added to $SHELL_RC"
                info "Run: ${BOLD}source $SHELL_RC${NC} to apply changes"
            fi
        fi
    else
        error "Installation failed"
        exit 1
    fi

    # Generate global config
    CONFIG_DIR="$HOME/.config/repo-search"
    mkdir -p "$CONFIG_DIR"
    CONFIG_FILE="$CONFIG_DIR/config.env"
    INSTALLED_GLOBALLY=true
else
    warn "Skipping global installation"
    info "Binaries are in ./dist/"
    INSTALLED_GLOBALLY=false
fi

#
# Generate config file
#
step 3 3 "Generating configuration..."

cat > "$CONFIG_FILE" << EOF
# repo-search configuration
# Auto-generated by installer on $(date)

# Database backend: sqlite or postgres
export REPO_SEARCH_DB_TYPE="$DB_TYPE"
EOF

if [[ $DB_TYPE == "postgres" && -n "$POSTGRES_DSN" ]]; then
    cat >> "$CONFIG_FILE" << EOF

# PostgreSQL configuration
export REPO_SEARCH_DB_DSN="$POSTGRES_DSN"
EOF
fi

cat >> "$CONFIG_FILE" << EOF

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

success "Configuration saved to $CONFIG_FILE"

# Add config sourcing to shell profile
if [[ $INSTALLED_GLOBALLY == true ]]; then
    # Detect shell
    if [[ $SHELL == *"zsh"* ]]; then
        SHELL_RC="$HOME/.zshrc"
    elif [[ $SHELL == *"bash"* ]]; then
        SHELL_RC="$HOME/.bashrc"
    else
        SHELL_RC="$HOME/.profile"
    fi

    # Check if sourcing line already exists (idempotency)
    SOURCE_LINE="[ -f \"$CONFIG_FILE\" ] && source \"$CONFIG_FILE\""
    if grep -qF "$CONFIG_FILE" "$SHELL_RC" 2>/dev/null; then
        info "Config already sourced in $SHELL_RC"
    else
        echo ""
        echo -e "  ${YELLOW}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
        echo ""
        read -p "$(prompt "Add repo-search config to shell profile? [Y/n]")" ADD_CONFIG
        ADD_CONFIG=${ADD_CONFIG:-Y}

        if [[ $ADD_CONFIG =~ ^[Yy] ]]; then
            echo "" >> "$SHELL_RC"
            echo "# Source repo-search configuration" >> "$SHELL_RC"
            echo "$SOURCE_LINE" >> "$SHELL_RC"
            success "Added config sourcing to $SHELL_RC"
            info "New shells will automatically have repo-search configuration"
            info "For current shell, run: ${BOLD}source $SHELL_RC${NC}"
        else
            info "Skipped adding to shell profile"
            info "To use config, run: ${BOLD}source $CONFIG_FILE${NC}"
        fi
    fi
fi

#
# Step 6: Initial Setup
#
print_header "Step 6/6: Initial Setup (Optional)"

if [[ $INSTALLED_GLOBALLY == true ]]; then
    REPO_SEARCH_CMD="repo-search"
else
    REPO_SEARCH_CMD="./dist/repo-search"
fi

if [[ $CTAGS_AVAILABLE == true ]]; then
    echo ""
    read -p "$(prompt "Index symbols in this repository now? [Y/n]")" RUN_INDEX
    RUN_INDEX=${RUN_INDEX:-Y}

    if [[ $RUN_INDEX =~ ^[Yy] ]]; then
        echo ""
        info "Running symbol indexing..."
        if [[ $INSTALLED_GLOBALLY == true ]]; then
            repo-search index .
        else
            ./dist/repo-search-index .
        fi
        success "Symbol indexing complete"
    fi
fi

if [[ $EMBEDDING_PROVIDER != "off" ]]; then
    echo ""
    read -p "$(prompt "Generate embeddings for this repository now? [Y/n]")" RUN_EMBED
    RUN_EMBED=${RUN_EMBED:-Y}

    if [[ $RUN_EMBED =~ ^[Yy] ]]; then
        echo ""
        info "Generating embeddings (this may take a few minutes)..."
        # Source the config to use the settings
        source "$CONFIG_FILE"
        if [[ $INSTALLED_GLOBALLY == true ]]; then
            repo-search embed .
        else
            ./dist/repo-search-index -embed .
        fi
        success "Embedding generation complete"
    fi
fi

#
# Final Summary
#
clear
echo -e "${GREEN}${BOLD}"
cat << 'EOF'
╔════════════════════════════════════════════════════════════════════╗
║                                                                    ║
║                   ✓  Installation Complete!                       ║
║                                                                    ║
╚════════════════════════════════════════════════════════════════════╝
EOF
echo -e "${NC}"

print_box "$CYAN" \
    "${BOLD}Configuration${NC}" \
    "  Location: $CONFIG_FILE" \
    ""

if [[ $INSTALLED_GLOBALLY == true ]]; then
    print_box "$GREEN" \
        "${BOLD}Global Installation${NC}" \
        "  Binaries: ~/.local/bin/" \
        "  Templates: ~/.local/share/repo-search/" \
        "  Registry: ~/.config/repo-search/registry.json"
fi

print_box "$MAGENTA" \
    "${BOLD}Features Enabled${NC}" \
    "  Database:        ${GREEN}✓${NC}  ($DB_TYPE)" \
    "  Keyword Search:  ${GREEN}✓${NC}  (ripgrep)" \
    "  Symbol Indexing: $(if [[ $CTAGS_AVAILABLE == true ]]; then echo "${GREEN}✓${NC}  (universal-ctags)"; else echo "${YELLOW}✗${NC}  (not installed)"; fi)" \
    "  Semantic Search: $(if [[ $EMBEDDING_PROVIDER != "off" ]]; then echo "${GREEN}✓${NC}  ($EMBEDDING_PROVIDER)"; else echo "${YELLOW}✗${NC}  (disabled)"; fi)"

print_box "$BLUE" \
    "${BOLD}Quick Start${NC}" \
    "  Check setup:        $REPO_SEARCH_CMD doctor" \
    "  Index project:      $REPO_SEARCH_CMD index <path>" \
    "  Generate embeddings: $REPO_SEARCH_CMD embed <path>" \
    "  View statistics:    $REPO_SEARCH_CMD stats" \
    "  Start daemon:       $REPO_SEARCH_CMD daemon start" \
    "  Update:             $REPO_SEARCH_CMD update"

print_box "$YELLOW" \
    "${BOLD}Using with Claude Code${NC}" \
    "  1. cd /path/to/your/project" \
    "  2. $REPO_SEARCH_CMD init" \
    "  3. $REPO_SEARCH_CMD index" \
    "  4. claude"

echo -e "${CYAN}Database Backend:${NC} $DB_TYPE"
if [[ $DB_TYPE == "postgres" ]]; then
    echo -e "${CYAN}PostgreSQL:${NC} $PG_HOST:$PG_PORT/$PG_DBNAME"
fi
echo ""

if [[ $EMBEDDING_PROVIDER == "ollama" ]]; then
    echo -e "${CYAN}Embedding Provider:${NC} Ollama ($EMBEDDING_MODEL)"
    echo -e "${CYAN}Server URL:${NC} $OLLAMA_URL"
elif [[ $EMBEDDING_PROVIDER == "litellm" ]]; then
    echo -e "${CYAN}Embedding Provider:${NC} LiteLLM ($EMBEDDING_MODEL)"
    echo -e "${CYAN}Server URL:${NC} $LITELLM_URL"
else
    echo -e "${CYAN}Embedding Provider:${NC} ${YELLOW}Disabled${NC}"
    info "To enable later, set REPO_SEARCH_EMBEDDING_PROVIDER in $CONFIG_FILE"
fi

echo ""
echo -e "${GREEN}${BOLD}Happy coding with repo-search! 🚀${NC}"
echo ""
