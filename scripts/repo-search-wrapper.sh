#!/bin/bash
#
# repo-search - Global wrapper script for repo-search MCP server
#
# Commands:
#   mcp            Start MCP server (used by .mcp.json)
#   index          Index symbols in current directory
#   embed          Generate embeddings for semantic search
#   init           Initialize repo-search in current directory
#   doctor         Check installation and dependencies
#   stats          Show index statistics
#   daemon         Manage background indexing daemon
#   registry       Manage project registry
#   help           Show this help message
#

set -e

# Installation paths
INSTALL_PREFIX="${REPO_SEARCH_PREFIX:-$HOME/.local}"
BIN_DIR="$INSTALL_PREFIX/bin"
SHARE_DIR="$INSTALL_PREFIX/share/repo-search"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/repo-search"
CONFIG_FILE="$CONFIG_DIR/config.env"
REGISTRY_FILE="$CONFIG_DIR/registry.json"
PID_FILE="$CONFIG_DIR/daemon.pid"
LOG_FILE="$CONFIG_DIR/daemon.log"
SOCKET_PATH="/tmp/repo-search-$(id -u).sock"

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

    # Register in central registry
    registry_add "$(pwd)" 2>/dev/null || true

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

#
# Daemon commands
#
cmd_daemon() {
    local subcmd="${1:-status}"
    shift || true

    case "$subcmd" in
        start)
            daemon_start "$@"
            ;;
        stop)
            daemon_stop
            ;;
        status)
            daemon_status
            ;;
        logs)
            daemon_logs "$@"
            ;;
        help|--help|-h)
            daemon_help
            ;;
        *)
            error "Unknown daemon command: $subcmd"
            daemon_help
            exit 1
            ;;
    esac
}

daemon_start() {
    # Check if already running
    if [[ -S "$SOCKET_PATH" ]]; then
        if daemon_is_running; then
            warn "Daemon is already running"
            return 0
        else
            # Stale socket
            rm -f "$SOCKET_PATH"
        fi
    fi

    # Ensure config directory exists
    mkdir -p "$CONFIG_DIR"

    # Check if daemon binary exists
    if [[ ! -x "$BIN_DIR/repo-search-daemon" ]]; then
        error "repo-search-daemon not found at $BIN_DIR/repo-search-daemon"
        info "Run 'make install' from the repo-search directory"
        return 1
    fi

    echo -e "${CYAN}Starting daemon...${NC}"

    # Start daemon in background
    nohup "$BIN_DIR/repo-search-daemon" start --foreground >> "$LOG_FILE" 2>&1 &
    local pid=$!

    # Wait for socket to be ready
    local tries=0
    while [[ ! -S "$SOCKET_PATH" && $tries -lt 20 ]]; do
        sleep 0.1
        ((tries++))
    done

    if [[ -S "$SOCKET_PATH" ]]; then
        success "Daemon started (PID: $pid)"
        info "Log file: $LOG_FILE"
    else
        error "Daemon failed to start"
        info "Check logs: repo-search daemon logs"
        return 1
    fi
}

daemon_stop() {
    if ! daemon_is_running; then
        warn "Daemon is not running"
        return 0
    fi

    echo -e "${CYAN}Stopping daemon...${NC}"

    # Send stop command via socket
    echo '{"action":"stop"}' | nc -U "$SOCKET_PATH" 2>/dev/null || true

    # Wait for socket to be removed
    local tries=0
    while [[ -S "$SOCKET_PATH" && $tries -lt 20 ]]; do
        sleep 0.1
        ((tries++))
    done

    if [[ ! -S "$SOCKET_PATH" ]]; then
        success "Daemon stopped"
    else
        # Force kill if still running
        if [[ -f "$PID_FILE" ]]; then
            local pid=$(cat "$PID_FILE")
            kill -9 "$pid" 2>/dev/null || true
            rm -f "$PID_FILE" "$SOCKET_PATH"
        fi
        success "Daemon stopped (forced)"
    fi
}

daemon_status() {
    if daemon_is_running; then
        echo -e "${GREEN}Daemon is running${NC}"
        echo ""

        # Get status from daemon
        local response=$(echo '{"action":"status"}' | nc -U "$SOCKET_PATH" 2>/dev/null)
        if [[ -n "$response" ]]; then
            echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
        fi
    else
        echo -e "${YELLOW}Daemon is not running${NC}"
        info "Start with: repo-search daemon start"
    fi
}

daemon_logs() {
    local lines="${1:-50}"

    if [[ ! -f "$LOG_FILE" ]]; then
        info "No log file found"
        return 0
    fi

    echo -e "${CYAN}Daemon logs (last $lines lines):${NC}"
    echo ""
    tail -n "$lines" "$LOG_FILE"
}

daemon_is_running() {
    [[ -S "$SOCKET_PATH" ]] && echo '{"action":"status"}' | nc -U "$SOCKET_PATH" &>/dev/null
}

daemon_help() {
    echo "Usage: repo-search daemon <command>"
    echo ""
    echo "Commands:"
    echo "  start       Start the background daemon"
    echo "  stop        Stop the daemon"
    echo "  status      Show daemon status"
    echo "  logs [n]    Show last n lines of logs (default: 50)"
    echo "  help        Show this help"
}

#
# Registry commands
#
cmd_registry() {
    local subcmd="${1:-list}"
    shift || true

    case "$subcmd" in
        list)
            registry_list
            ;;
        add)
            registry_add "$@"
            ;;
        remove)
            registry_remove "$@"
            ;;
        stats)
            registry_stats
            ;;
        help|--help|-h)
            registry_help
            ;;
        *)
            error "Unknown registry command: $subcmd"
            registry_help
            exit 1
            ;;
    esac
}

registry_list() {
    if [[ ! -f "$REGISTRY_FILE" ]]; then
        info "No projects registered"
        info "Run 'repo-search init' in a project to register it"
        return 0
    fi

    echo -e "${CYAN}Registered Projects${NC}"
    echo ""

    # Parse JSON and display projects
    python3 -c "
import json
import sys
from datetime import datetime

try:
    with open('$REGISTRY_FILE') as f:
        data = json.load(f)

    projects = data.get('projects', [])
    if not projects:
        print('  No projects registered')
        sys.exit(0)

    for p in projects:
        path = p.get('path', 'unknown')
        name = p.get('name', 'unknown')
        watch = '✓' if p.get('watch_enabled') else '○'
        stats = p.get('index_stats', {})
        symbols = stats.get('symbols', 0)
        embeddings = stats.get('embeddings', 0)

        last_indexed = p.get('last_indexed')
        if last_indexed:
            dt = datetime.fromisoformat(last_indexed.replace('Z', '+00:00'))
            last_indexed = dt.strftime('%Y-%m-%d %H:%M')
        else:
            last_indexed = 'never'

        print(f'  {watch} {name}')
        print(f'    Path: {path}')
        print(f'    Symbols: {symbols}, Embeddings: {embeddings}')
        print(f'    Last indexed: {last_indexed}')
        print()
except FileNotFoundError:
    print('  No projects registered')
except Exception as e:
    print(f'  Error reading registry: {e}')
"
}

registry_add() {
    local path="${1:-.}"
    path=$(cd "$path" && pwd)

    if [[ ! -d "$path" ]]; then
        error "Directory not found: $path"
        return 1
    fi

    # Ensure config directory exists
    mkdir -p "$CONFIG_DIR"

    # Add to registry using Python (simple JSON manipulation)
    python3 -c "
import json
import os
from datetime import datetime

registry_file = '$REGISTRY_FILE'
project_path = '$path'
project_name = os.path.basename(project_path)

# Load or create registry
try:
    with open(registry_file) as f:
        data = json.load(f)
except FileNotFoundError:
    data = {'version': 1, 'projects': [], 'settings': {'auto_watch': True, 'debounce_ms': 500, 'max_projects': 50}}

# Check if already registered
for p in data['projects']:
    if p['path'] == project_path:
        print(f'Project already registered: {project_name}')
        exit(0)

# Add project
data['projects'].append({
    'path': project_path,
    'name': project_name,
    'added_at': datetime.utcnow().isoformat() + 'Z',
    'last_indexed': None,
    'index_stats': {'symbols': 0, 'embeddings': 0, 'db_size_bytes': 0},
    'watch_enabled': data['settings']['auto_watch']
})

with open(registry_file, 'w') as f:
    json.dump(data, f, indent=2)

print(f'Added project: {project_name}')
"
    success "Project registered"

    # If daemon is running, tell it to watch this project
    if daemon_is_running; then
        echo "{\"action\":\"add\",\"path\":\"$path\"}" | nc -U "$SOCKET_PATH" &>/dev/null || true
        info "Daemon will watch this project"
    fi
}

registry_remove() {
    local path="${1:-.}"
    path=$(cd "$path" 2>/dev/null && pwd) || path="$1"

    if [[ ! -f "$REGISTRY_FILE" ]]; then
        error "No registry found"
        return 1
    fi

    # Remove from registry using Python
    python3 -c "
import json
import os

registry_file = '$REGISTRY_FILE'
project_path = '$path'

with open(registry_file) as f:
    data = json.load(f)

original_len = len(data['projects'])
data['projects'] = [p for p in data['projects'] if p['path'] != project_path]

if len(data['projects']) == original_len:
    print(f'Project not found in registry: {project_path}')
    exit(1)

with open(registry_file, 'w') as f:
    json.dump(data, f, indent=2)

print(f'Removed project: {os.path.basename(project_path)}')
"
    success "Project removed"

    # If daemon is running, tell it to stop watching
    if daemon_is_running; then
        echo "{\"action\":\"remove\",\"path\":\"$path\"}" | nc -U "$SOCKET_PATH" &>/dev/null || true
    fi
}

registry_stats() {
    if [[ ! -f "$REGISTRY_FILE" ]]; then
        info "No projects registered"
        return 0
    fi

    echo -e "${CYAN}Registry Statistics${NC}"
    echo ""

    python3 -c "
import json

with open('$REGISTRY_FILE') as f:
    data = json.load(f)

projects = data.get('projects', [])
total_symbols = sum(p.get('index_stats', {}).get('symbols', 0) for p in projects)
total_embeddings = sum(p.get('index_stats', {}).get('embeddings', 0) for p in projects)
total_size = sum(p.get('index_stats', {}).get('db_size_bytes', 0) for p in projects)
watched = sum(1 for p in projects if p.get('watch_enabled'))

print(f'Total projects: {len(projects)}')
print(f'Watched projects: {watched}')
print(f'Total symbols: {total_symbols}')
print(f'Total embeddings: {total_embeddings}')
print(f'Total index size: {total_size / 1024 / 1024:.2f} MB')
"
}

registry_help() {
    echo "Usage: repo-search registry <command>"
    echo ""
    echo "Commands:"
    echo "  list        List all registered projects"
    echo "  add [path]  Add project to registry (default: current directory)"
    echo "  remove <path>  Remove project from registry"
    echo "  stats       Show aggregate statistics"
    echo "  help        Show this help"
}

cmd_update() {
    local source_dir="${REPO_SEARCH_SOURCE:-$HOME/dev/repo-search}"

    if [[ ! -f "$source_dir/scripts/update.sh" ]]; then
        error "Update script not found"
        info "Set REPO_SEARCH_SOURCE to the location of your repo-search clone"
        info "Default: $source_dir"
        return 1
    fi

    exec "$source_dir/scripts/update.sh"
}

cmd_help() {
    echo -e "${CYAN}repo-search${NC} - MCP server for codebase search & navigation"
    echo ""
    echo "Usage: repo-search <command> [options]"
    echo ""
    echo "Commands:"
    echo "  mcp             Start MCP server (used by .mcp.json)"
    echo "  index [path]    Index symbols (default: current directory)"
    echo "  embed [path]    Generate embeddings for semantic search"
    echo "  init [-f]       Create .mcp.json in current directory"
    echo "  doctor          Check installation and dependencies"
    echo "  stats           Show index statistics"
    echo "  daemon <cmd>    Manage background indexing daemon"
    echo "  registry <cmd>  Manage project registry"
    echo "  update          Update to latest version from GitHub"
    echo "  help            Show this help message"
    echo ""
    echo "Daemon Commands:"
    echo "  daemon start    Start background daemon"
    echo "  daemon stop     Stop daemon"
    echo "  daemon status   Show daemon status"
    echo "  daemon logs     View daemon logs"
    echo ""
    echo "Registry Commands:"
    echo "  registry list     List registered projects"
    echo "  registry add      Add current project"
    echo "  registry remove   Remove a project"
    echo "  registry stats    Show aggregate stats"
    echo ""
    echo "Configuration:"
    echo "  Global config: $CONFIG_FILE"
    echo "  Registry:      $REGISTRY_FILE"
    echo "  Per-project:   .mcp.json (created by 'init')"
    echo ""
    echo "Quick Start:"
    echo "  repo-search init          # Initialize project"
    echo "  repo-search index         # Index symbols"
    echo "  repo-search daemon start  # Start background daemon"
    echo "  claude                    # Start Claude Code"
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
        daemon)
            cmd_daemon "$@"
            ;;
        registry)
            cmd_registry "$@"
            ;;
        update)
            cmd_update "$@"
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
