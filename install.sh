#!/bin/bash
#
# codetect installation script
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
CONFIG_FILE=".env.codetect"

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
check_port() {
    local port=$1
    if command -v lsof &> /dev/null; then
        lsof -i ":$port" &> /dev/null
        return $?
    elif command -v netstat &> /dev/null; then
        netstat -an | grep -q ":$port.*LISTEN"
        return $?
    else
        # Can't check, assume available
        return 1
    fi
}

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
║                        codetect installer                       ║
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
if command -v codetect &> /dev/null; then
    EXISTING_VERSION=$(codetect --version 2>/dev/null || echo "unknown")
    warn "codetect is already installed ($EXISTING_VERSION)"
    echo ""
    info "This will reinstall/update codetect."
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
        info "You can install universal-ctags later and run 'codetect index'"
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
                    "  • Run: ${BOLD}ollama pull bge-m3${NC} (or your preferred model)" \
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
                # Model Selection Menu
                print_section "Embedding Model Selection"

                echo "Select an embedding model for code search:"
                echo ""
                echo -e "  ${GREEN}${BOLD}1)${NC} bge-m3 ${YELLOW}(RECOMMENDED)${NC}"
                info "Best overall performance (+47% vs nomic)"
                info "Dimensions: 1024, Memory: 2.2 GB, Context: 8K tokens"
                echo ""
                echo -e "  ${GREEN}${BOLD}2)${NC} snowflake-arctic-embed-l-v2.0"
                info "Highest retrieval quality (+57% vs nomic)"
                info "Dimensions: 1024, Memory: 2.2 GB, Context: 8K tokens"
                echo ""
                echo -e "  ${GREEN}${BOLD}3)${NC} jina-embeddings-v3"
                info "Best semantic similarity (+50% vs nomic)"
                info "Dimensions: 1024, Memory: 1.1 GB, Context: 8K tokens"
                echo ""
                echo -e "  ${GREEN}${BOLD}4)${NC} nomic-embed-text ${YELLOW}(legacy)${NC}"
                info "Backward compatibility, smallest footprint"
                info "Dimensions: 768, Memory: 522 MB, Context: 8K tokens"
                echo ""
                echo -e "  ${GREEN}${BOLD}5)${NC} Custom model"
                info "Specify your own Ollama-compatible model"
                echo ""
                info "See docs/embedding-model-comparison.md for detailed comparison"
                echo ""

                read -p "$(prompt "Your choice [1]")" MODEL_CHOICE
                MODEL_CHOICE=${MODEL_CHOICE:-1}

                # Set model variables based on choice
                case $MODEL_CHOICE in
                    1)
                        EMBEDDING_MODEL="bge-m3"
                        OLLAMA_MODEL_NAME="bge-m3"
                        VECTOR_DIMENSIONS="1024"
                        MODEL_SIZE="2.2 GB"
                        ;;
                    2)
                        EMBEDDING_MODEL="snowflake-arctic-embed"
                        OLLAMA_MODEL_NAME="snowflake-arctic-embed"
                        VECTOR_DIMENSIONS="1024"
                        MODEL_SIZE="2.2 GB"
                        ;;
                    3)
                        EMBEDDING_MODEL="jina-embeddings-v3"
                        OLLAMA_MODEL_NAME="jina/jina-embeddings-v3"
                        VECTOR_DIMENSIONS="1024"
                        MODEL_SIZE="1.1 GB"
                        ;;
                    4)
                        EMBEDDING_MODEL="nomic-embed-text"
                        OLLAMA_MODEL_NAME="nomic-embed-text"
                        VECTOR_DIMENSIONS="768"
                        MODEL_SIZE="274 MB"
                        ;;
                    5)
                        read -p "$(prompt "Enter Ollama model name")" EMBEDDING_MODEL
                        OLLAMA_MODEL_NAME="$EMBEDDING_MODEL"
                        read -p "$(prompt "Enter vector dimensions [1024]")" VECTOR_DIMENSIONS
                        VECTOR_DIMENSIONS=${VECTOR_DIMENSIONS:-1024}
                        MODEL_SIZE="unknown"
                        warn "Custom models are not validated - ensure compatibility"
                        ;;
                    *)
                        error "Invalid choice"
                        exit 1
                        ;;
                esac

                success "Selected: $EMBEDDING_MODEL (dimensions: $VECTOR_DIMENSIONS)"
                echo ""

                # Check if Ollama is running
                if curl -s http://localhost:11434/api/tags &> /dev/null; then
                    success "Ollama is running"

                    # Check for selected model
                    if curl -s http://localhost:11434/api/tags | grep -q "$EMBEDDING_MODEL"; then
                        success "$EMBEDDING_MODEL model is available"
                    else
                        warn "$EMBEDDING_MODEL model not found"
                        echo ""
                        info "The $EMBEDDING_MODEL model is required for semantic search."
                        info "Download size: $MODEL_SIZE"
                        echo ""
                        read -p "$(prompt "Pull $EMBEDDING_MODEL now? [Y/n]")" PULL_MODEL
                        PULL_MODEL=${PULL_MODEL:-Y}
                        if [[ $PULL_MODEL =~ ^[Yy] ]]; then
                            echo ""
                            info "Downloading model (this may take several minutes)..."
                            if ollama pull "$OLLAMA_MODEL_NAME"; then
                                success "Model downloaded successfully"
                                success "Vector dimensions: $VECTOR_DIMENSIONS"
                            else
                                error "Failed to download model"
                                warn "You can download it later with: ${BOLD}ollama pull $OLLAMA_MODEL_NAME${NC}"
                            fi
                        fi
                    fi
                else
                    warn "Ollama is not running"
                    info "Start it with: ${BOLD}ollama serve${NC}"
                    info "Or it will start automatically when you run 'codetect embed'"
                fi

                # Custom Ollama URL?
                echo ""
                read -p "$(prompt "Ollama URL [http://localhost:11434]")" OLLAMA_URL
                OLLAMA_URL=${OLLAMA_URL:-http://localhost:11434}
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
                info "Make sure the server is running before using 'codetect embed'"
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
    info "You can enable it later by setting CODETECT_EMBEDDING_PROVIDER"
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
info "Storage: Local .codetect/symbols.db file"
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

        # Check if Docker is available
        DOCKER_AVAILABLE=false
        if command -v docker &> /dev/null && docker ps &> /dev/null; then
            success "Docker is available"
            DOCKER_AVAILABLE=true
        fi

        # Offer Docker vs System installation
        echo "Choose installation method:"
        echo ""
        if [[ $DOCKER_AVAILABLE == true ]]; then
            echo -e "  ${GREEN}${BOLD}1)${NC} Docker (recommended - easy to manage)"
            info "Isolated container, easy start/stop, no system conflicts"
        else
            echo -e "  ${YELLOW}${BOLD}1)${NC} Docker (not available)"
            info "Install Docker from: ${BOLD}https://www.docker.com/get-started${NC}"
        fi
        echo ""
        echo -e "  ${GREEN}${BOLD}2)${NC} System installation (PostgreSQL + pgvector)"
        info "Installs directly on your system via package manager"
        echo ""

        read -p "$(prompt "Your choice [1]")" INSTALL_METHOD
        INSTALL_METHOD=${INSTALL_METHOD:-1}

        if [[ $INSTALL_METHOD == "1" && $DOCKER_AVAILABLE == true ]]; then
            # Docker installation
            echo ""
            print_section "Docker PostgreSQL Setup"

            # Check if container exists and its state
            CONTAINER_EXISTS=false
            CONTAINER_RUNNING=false

            if docker ps -a --format '{{.Names}}' | grep -q '^codetect-postgres$'; then
                CONTAINER_EXISTS=true
                if docker ps --format '{{.Names}}' | grep -q '^codetect-postgres$'; then
                    CONTAINER_RUNNING=true
                fi
            fi

            if [[ $CONTAINER_EXISTS == true && $CONTAINER_RUNNING == true ]]; then
                success "PostgreSQL container already running"

                # Get the port it's running on
                PG_PORT=$(docker inspect -f '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' codetect-postgres 2>/dev/null || echo "5432")
                info "Container: codetect-postgres"
                info "Port: $PG_PORT"

                # Verify it's healthy
                if docker-compose exec -T postgres pg_isready -U codetect &>/dev/null; then
                    success "PostgreSQL is healthy"
                else
                    warn "Container running but PostgreSQL not ready"
                    info "Attempting restart..."
                    docker-compose restart postgres

                    # Wait for PostgreSQL to be ready
                    info "Waiting for PostgreSQL to be ready..."
                    for i in {1..30}; do
                        if docker-compose exec -T postgres pg_isready -U codetect &>/dev/null; then
                            success "PostgreSQL is ready"
                            break
                        fi
                        sleep 1
                    done
                fi

                # Check if pgvector is enabled
                if docker-compose exec -T postgres psql -U codetect -d codetect -c "SELECT extname FROM pg_extension WHERE extname='vector'" | grep -q vector; then
                    success "pgvector extension is enabled"
                else
                    warn "pgvector extension not enabled"
                    info "Enabling pgvector..."
                    docker-compose exec -T postgres psql -U codetect -d codetect -c "CREATE EXTENSION IF NOT EXISTS vector"
                fi

                # Set connection details
                PG_HOST="localhost"
                PG_USER="codetect"
                PG_PASSWORD="codetect"
                PG_DBNAME="codetect"
                POSTGRES_DSN="postgres://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}?sslmode=disable"
                POSTGRES_INSTALLED=true

            elif [[ $CONTAINER_EXISTS == true && $CONTAINER_RUNNING == false ]]; then
                warn "PostgreSQL container exists but is stopped"

                # Get the port it was configured with
                PG_PORT=$(docker inspect -f '{{(index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort}}' codetect-postgres 2>/dev/null || echo "5432")
                info "Starting existing container on port $PG_PORT..."

                # Set environment for docker-compose
                export POSTGRES_PORT=$PG_PORT

                if docker-compose start postgres; then
                    success "PostgreSQL container started successfully"

                    # Wait for PostgreSQL to be ready
                    info "Waiting for PostgreSQL to be ready..."
                    for i in {1..30}; do
                        if docker-compose exec -T postgres pg_isready -U codetect &>/dev/null; then
                            success "PostgreSQL is ready"
                            break
                        fi
                        sleep 1
                    done

                    # Verify pgvector
                    if docker-compose exec -T postgres psql -U codetect -d codetect -c "SELECT extname FROM pg_extension WHERE extname='vector'" | grep -q vector; then
                        success "pgvector extension is enabled"
                    else
                        warn "pgvector extension not enabled"
                        info "Enabling pgvector..."
                        docker-compose exec -T postgres psql -U codetect -d codetect -c "CREATE EXTENSION IF NOT EXISTS vector"
                    fi

                    # Set connection details
                    PG_HOST="localhost"
                    PG_USER="codetect"
                    PG_PASSWORD="codetect"
                    PG_DBNAME="codetect"
                    POSTGRES_DSN="postgres://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}?sslmode=disable"
                    POSTGRES_INSTALLED=true
                else
                    error "Failed to start PostgreSQL container"
                    warn "Falling back to SQLite"
                    DB_TYPE="sqlite"
                fi

            else
                # Container doesn't exist - create it
                echo ""
                info "Creating new PostgreSQL container..."

                # Check port availability
                PG_PORT=5432
                while check_port $PG_PORT; do
                    warn "Port $PG_PORT is already in use"
                    echo ""
                    read -p "$(prompt "Enter alternative port [5433]")" ALT_PORT
                    PG_PORT=${ALT_PORT:-5433}
                done

                success "Port $PG_PORT is available"

                # Set environment for docker-compose
                export POSTGRES_PORT=$PG_PORT

                info "Starting PostgreSQL with pgvector in Docker..."
                info "Container: codetect-postgres"
                info "Port: $PG_PORT"
                echo ""

                if docker-compose up -d postgres; then
                    success "PostgreSQL container started successfully"

                    # Wait for PostgreSQL to be ready
                    info "Waiting for PostgreSQL to be ready..."
                    for i in {1..30}; do
                        if docker-compose exec -T postgres pg_isready -U codetect &>/dev/null; then
                            success "PostgreSQL is ready"
                            break
                        fi
                        sleep 1
                    done

                    # Check if pgvector is enabled
                    if docker-compose exec -T postgres psql -U codetect -d codetect -c "SELECT extname FROM pg_extension WHERE extname='vector'" | grep -q vector; then
                        success "pgvector extension is enabled"
                    else
                        warn "pgvector extension not enabled automatically"
                        info "Enabling pgvector..."
                        docker-compose exec -T postgres psql -U codetect -d codetect -c "CREATE EXTENSION IF NOT EXISTS vector"
                    fi

                    # Set connection details
                    PG_HOST="localhost"
                    PG_USER="codetect"
                    PG_PASSWORD="codetect"
                    PG_DBNAME="codetect"
                    POSTGRES_DSN="postgres://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}?sslmode=disable"
                    POSTGRES_INSTALLED=true

                    echo ""
                    print_box "$GREEN" \
                        "PostgreSQL is running in Docker" \
                        "" \
                        "Start:   docker-compose up -d postgres" \
                        "Stop:    docker-compose stop postgres" \
                        "Logs:    docker-compose logs -f postgres" \
                        "Remove:  docker-compose down -v"
                else
                    error "Failed to start PostgreSQL container"
                    warn "Falling back to SQLite"
                    DB_TYPE="sqlite"
                fi
            fi

        elif [[ $INSTALL_METHOD == "2" || ($INSTALL_METHOD == "1" && $DOCKER_AVAILABLE == false) ]]; then
            # System installation
            echo ""
            print_section "System PostgreSQL Installation"

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

            # Platform-specific defaults
            if [[ $PLATFORM == "Mac" ]]; then
                DEFAULT_PG_USER="$USER"
                info "On macOS, Homebrew PostgreSQL uses your username by default"
            else
                DEFAULT_PG_USER="postgres"
            fi

            read -p "$(prompt "PostgreSQL host [localhost]")" PG_HOST
            PG_HOST=${PG_HOST:-localhost}

            # Check port availability
            PG_PORT=5432
            if check_port $PG_PORT; then
                warn "Port $PG_PORT is already in use by another service"
                echo ""
                read -p "$(prompt "Enter alternative port [5433]")" ALT_PORT
                PG_PORT=${ALT_PORT:-5433}

                # Check alternative port
                if check_port $PG_PORT; then
                    error "Port $PG_PORT is also in use"
                    warn "Please stop the conflicting service or choose a different port"
                    read -p "$(prompt "Enter port number")" PG_PORT
                fi
            fi

            success "Using port $PG_PORT"

            read -p "$(prompt "PostgreSQL user [$DEFAULT_PG_USER]")" PG_USER
            PG_USER=${PG_USER:-$DEFAULT_PG_USER}

            read -sp "$(prompt "PostgreSQL password (leave empty if not required)")" PG_PASSWORD
            echo ""

            read -p "$(prompt "Database name [codetect]")" PG_DBNAME
            PG_DBNAME=${PG_DBNAME:-codetect}

            # Test connection to postgres database (always exists)
            echo ""
            info "Testing PostgreSQL connection..."

            if [[ -z "$PG_PASSWORD" ]]; then
                TEST_DSN="postgres://${PG_USER}@${PG_HOST}:${PG_PORT}/postgres?sslmode=disable"
            else
                TEST_DSN="postgres://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/postgres?sslmode=disable"
            fi

            if psql "$TEST_DSN" -c "SELECT 1" &>/dev/null; then
                success "Successfully connected to PostgreSQL"

                # Create database if it doesn't exist
                info "Creating database '$PG_DBNAME' if it doesn't exist..."
                if psql "$TEST_DSN" -c "SELECT 1 FROM pg_database WHERE datname='$PG_DBNAME'" | grep -q 1; then
                    success "Database '$PG_DBNAME' already exists"
                else
                    if psql "$TEST_DSN" -c "CREATE DATABASE $PG_DBNAME" &>/dev/null; then
                        success "Database '$PG_DBNAME' created"
                    else
                        error "Failed to create database '$PG_DBNAME'"
                        warn "You may need to create it manually with: createdb $PG_DBNAME"
                    fi
                fi

                # Build DSN for target database
                if [[ -z "$PG_PASSWORD" ]]; then
                    POSTGRES_DSN="postgres://${PG_USER}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}?sslmode=disable"
                else
                    POSTGRES_DSN="postgres://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DBNAME}?sslmode=disable"
                fi

                # Enable pgvector extension if available
                if [[ $HAS_PGVECTOR == true ]]; then
                    # Check if extension already enabled
                    if psql "$POSTGRES_DSN" -c "SELECT 1 FROM pg_extension WHERE extname='vector'" 2>/dev/null | grep -q 1; then
                        success "pgvector extension already enabled"
                    else
                        info "Enabling pgvector extension..."
                        if psql "$POSTGRES_DSN" -c "CREATE EXTENSION IF NOT EXISTS vector" &>/dev/null; then
                            success "pgvector extension enabled"
                        else
                            warn "Could not enable pgvector extension"
                            info "You may need to enable it manually with: CREATE EXTENSION vector;"
                        fi
                    fi
                fi
            else
                warn "Could not connect to PostgreSQL"
                info "Make sure:"
                info "  1. PostgreSQL is running (${BOLD}brew services start postgresql@16${NC} or ${BOLD}pg_ctl -D /opt/homebrew/var/postgresql@16 start${NC})"
                info "  2. User '$PG_USER' exists and has access"
                info "  3. Connection credentials are correct"
                echo ""
                info "Debug tips:"
                info "  • Check if PostgreSQL is running: ${BOLD}brew services list${NC}"
                info "  • Try connecting manually: ${BOLD}psql postgres${NC}"
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
        else
            error "Invalid installation method"
            warn "Falling back to SQLite"
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
print_header "Step 5/6: Building codetect"

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
                echo "# Added by codetect installer" >> "$SHELL_RC"
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
    CONFIG_DIR="$HOME/.config/codetect"
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

# Helper function to load existing config
load_existing_config() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        return 1
    fi

    if source "$CONFIG_FILE" 2>/dev/null; then
        return 0
    else
        warn "Existing config file appears corrupted"
        return 1
    fi
}

# Check if config exists and load it
BACKUP_FILE=""
if [[ -f "$CONFIG_FILE" ]]; then
    info "Configuration file already exists"

    if load_existing_config; then
        info "Preserving existing configuration values..."

        # Use existing values if set, otherwise use newly selected values
        # This allows script to update with new selections while preserving user customizations
        PREVIOUS_DB_TYPE="${CODETECT_DB_TYPE:-}"
        DB_TYPE="${CODETECT_DB_TYPE:-$DB_TYPE}"
        POSTGRES_DSN="${CODETECT_DB_DSN:-$POSTGRES_DSN}"
        EMBEDDING_PROVIDER="${CODETECT_EMBEDDING_PROVIDER:-$EMBEDDING_PROVIDER}"
        OLLAMA_URL="${CODETECT_OLLAMA_URL:-$OLLAMA_URL}"
        EMBEDDING_MODEL="${CODETECT_EMBEDDING_MODEL:-$EMBEDDING_MODEL}"
        LITELLM_URL="${CODETECT_LITELLM_URL:-$LITELLM_URL}"
        LITELLM_API_KEY="${CODETECT_LITELLM_API_KEY:-$LITELLM_API_KEY}"

        # Detect database backend change
        if [[ -n "$PREVIOUS_DB_TYPE" && "$PREVIOUS_DB_TYPE" != "$DB_TYPE" ]]; then
            echo ""
            warn "Database backend change detected!"
            info "Previous: $PREVIOUS_DB_TYPE"
            info "New:      $DB_TYPE"
            echo ""
            warn "Changing database backends requires migration."
            info "Existing indexes in $PREVIOUS_DB_TYPE will not be accessible."
            echo ""
            read -p "$(prompt "Continue with database change? [y/N]")" CONFIRM_DB_CHANGE
            CONFIRM_DB_CHANGE=${CONFIRM_DB_CHANGE:-N}

            if [[ ! $CONFIRM_DB_CHANGE =~ ^[Yy] ]]; then
                error "Database change cancelled"
                info "Re-run installer and select '$PREVIOUS_DB_TYPE' to keep existing setup"
                exit 1
            fi

            info "To migrate data later, use: make migrate-to-postgres"
            echo ""
        fi

        # Create backup before regenerating
        BACKUP_FILE="$CONFIG_FILE.backup.$(date +%Y%m%d-%H%M%S)"
        cp "$CONFIG_FILE" "$BACKUP_FILE"
        info "Backed up old config to $BACKUP_FILE"
    else
        # Corrupted config, back it up and create fresh
        BACKUP_FILE="$CONFIG_FILE.corrupted.$(date +%Y%m%d-%H%M%S)"
        mv "$CONFIG_FILE" "$BACKUP_FILE"
        info "Backed up corrupted config to $BACKUP_FILE"
        info "Creating fresh configuration..."
    fi
fi

cat > "$CONFIG_FILE" << EOF
# codetect configuration
# Auto-generated by installer on $(date)

# Database backend: sqlite or postgres
export CODETECT_DB_TYPE="$DB_TYPE"
EOF

if [[ $DB_TYPE == "postgres" && -n "$POSTGRES_DSN" ]]; then
    cat >> "$CONFIG_FILE" << EOF

# PostgreSQL configuration
export CODETECT_DB_DSN="$POSTGRES_DSN"
EOF
fi

cat >> "$CONFIG_FILE" << EOF

# Embedding provider: ollama, litellm, or off
export CODETECT_EMBEDDING_PROVIDER="$EMBEDDING_PROVIDER"
EOF

if [[ $EMBEDDING_PROVIDER == "ollama" ]]; then
    cat >> "$CONFIG_FILE" << EOF

# Ollama configuration
export CODETECT_OLLAMA_URL="$OLLAMA_URL"
export CODETECT_EMBEDDING_MODEL="$EMBEDDING_MODEL"
export CODETECT_VECTOR_DIMENSIONS="$VECTOR_DIMENSIONS"
EOF
elif [[ $EMBEDDING_PROVIDER == "litellm" ]]; then
    cat >> "$CONFIG_FILE" << EOF

# LiteLLM configuration
export CODETECT_LITELLM_URL="$LITELLM_URL"
export CODETECT_LITELLM_API_KEY="$LITELLM_API_KEY"
export CODETECT_EMBEDDING_MODEL="$EMBEDDING_MODEL"
EOF
fi

if [[ -n "$BACKUP_FILE" ]]; then
    success "Configuration updated (previous backed up)"
else
    success "Configuration saved to $CONFIG_FILE"
fi

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
        read -p "$(prompt "Add codetect config to shell profile? [Y/n]")" ADD_CONFIG
        ADD_CONFIG=${ADD_CONFIG:-Y}

        if [[ $ADD_CONFIG =~ ^[Yy] ]]; then
            echo "" >> "$SHELL_RC"
            echo "# Source codetect configuration" >> "$SHELL_RC"
            echo "$SOURCE_LINE" >> "$SHELL_RC"
            success "Added config sourcing to $SHELL_RC"
            info "New shells will automatically have codetect configuration"
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
    CODETECT_CMD="codetect"
else
    CODETECT_CMD="./dist/codetect"
fi

if [[ $CTAGS_AVAILABLE == true ]]; then
    echo ""
    read -p "$(prompt "Index symbols in this repository now? [Y/n]")" RUN_INDEX
    RUN_INDEX=${RUN_INDEX:-Y}

    if [[ $RUN_INDEX =~ ^[Yy] ]]; then
        echo ""
        info "Running symbol indexing..."
        if [[ $INSTALLED_GLOBALLY == true ]]; then
            codetect index .
        else
            ./dist/codetect-index .
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
            codetect embed .
        else
            ./dist/codetect-index -embed .
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
        "  Templates: ~/.local/share/codetect/" \
        "  Registry: ~/.config/codetect/registry.json"
fi

print_box "$MAGENTA" \
    "${BOLD}Features Enabled${NC}" \
    "  Database:        ${GREEN}✓${NC}  ($DB_TYPE)" \
    "  Keyword Search:  ${GREEN}✓${NC}  (ripgrep)" \
    "  Symbol Indexing: $(if [[ $CTAGS_AVAILABLE == true ]]; then echo "${GREEN}✓${NC}  (universal-ctags)"; else echo "${YELLOW}✗${NC}  (not installed)"; fi)" \
    "  Semantic Search: $(if [[ $EMBEDDING_PROVIDER != "off" ]]; then echo "${GREEN}✓${NC}  ($EMBEDDING_PROVIDER)"; else echo "${YELLOW}✗${NC}  (disabled)"; fi)"

print_box "$BLUE" \
    "${BOLD}Quick Start${NC}" \
    "  Check setup:        $CODETECT_CMD doctor" \
    "  Index project:      $CODETECT_CMD index <path>" \
    "  Generate embeddings: $CODETECT_CMD embed <path>" \
    "  View statistics:    $CODETECT_CMD stats" \
    "  Start daemon:       $CODETECT_CMD daemon start" \
    "  Update:             $CODETECT_CMD update"

print_box "$YELLOW" \
    "${BOLD}Using with Claude Code${NC}" \
    "  1. cd /path/to/your/project" \
    "  2. $CODETECT_CMD init" \
    "  3. $CODETECT_CMD index" \
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
    info "To enable later, set CODETECT_EMBEDDING_PROVIDER in $CONFIG_FILE"
fi

echo ""
echo -e "${GREEN}${BOLD}Happy coding with codetect! 🚀${NC}"
echo ""
