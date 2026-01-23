# Installation Guide

This guide covers all installation methods for codetect.

## Requirements

### Required

- **Go 1.21+** - for building from source
- **[ripgrep](https://github.com/BurntSushi/ripgrep)** (`rg`) - for keyword search

### Optional

- **[universal-ctags](https://github.com/universal-ctags/ctags)** - for symbol indexing
- **[Ollama](https://ollama.ai)** - for semantic search (default embedding provider)
- **[LiteLLM](https://github.com/BerriAI/litellm)** - alternative embedding provider
- **[Docker](https://www.docker.com/get-started)** - for running PostgreSQL + pgvector (recommended)
- **[PostgreSQL 12+](https://www.postgresql.org)** + **[pgvector](https://github.com/pgvector/pgvector)** - for scalable vector search (alternative to SQLite)

## Quick Start (Recommended)

The interactive installer provides a guided setup experience:

```bash
git clone https://github.com/brian-lai/codetect.git
cd codetect
./install.sh
```

The installer will:
1. **Check dependencies** - Verify Go and ripgrep are installed
2. **Optional features setup**:
   - **Symbol indexing** - Ask if you want ctags and offer to install it automatically
   - **Semantic search** - Ask if you want semantic search and guide Ollama setup
3. **Database setup** - Choose between SQLite (simple) or PostgreSQL+pgvector (scalable)
4. **Build and install** - Compile binaries and optionally install globally to `~/.local/bin`
5. **Configure PATH** - Automatically add `~/.local/bin` to your shell profile if needed
6. **Initial indexing** - Optionally index the codetect codebase itself

### What the Installer Does

**Automatic Dependency Installation:**
- Detects your platform (macOS, Linux) and package manager (brew, apt, dnf, pacman)
- If you enable symbol indexing but don't have ctags, it will install it for you
- Supports: Homebrew (macOS), apt (Ubuntu/Debian), dnf (Fedora), pacman (Arch)

**Smart Ollama Detection:**
- Checks if Ollama is installed when you enable semantic search
- Shows a prominent warning if Ollama is missing with installation instructions
- If Ollama is installed, checks if it's running and if the embedding model is available
- Offers to download the default embedding model during installation
  - **Recommended:** `bge-m3` (1024 dims, ~2.2GB) - 47% better retrieval than default
  - **Default:** `nomic-embed-text` (768 dims, ~274MB) - smaller but lower quality
  - See [Embedding Model Comparison](./embedding-model-comparison.md) for detailed analysis

**Global Installation:**
- Installs binaries to `~/.local/bin` for easy access from anywhere
- Creates global config at `~/.config/codetect/config.env`
- Automatically updates your shell profile ($PATH) if needed

### Installation Flow

The installer runs in 6 steps:

**Step 1: Checking Required Dependencies**
- Verifies Go 1.21+ is installed
- Verifies ripgrep is installed
- Exits with helpful error messages if either is missing

**Step 2: Optional Features Setup**
- **Symbol Indexing**:
  - Checks if universal-ctags is installed
  - If not, explains what symbol indexing does
  - Asks if you want to enable it
  - If yes, automatically installs ctags using your package manager
- **Semantic Search**:
  - Explains semantic search capabilities
  - Asks if you want to enable it
  - If yes, shows provider options (Ollama or LiteLLM)
  - For Ollama: shows prominent red warning box if not installed
  - Checks if Ollama is running and if models are available
  - Offers to download the embedding model

**Step 3: Database Setup**
- **Database Choice**:
  - Shows two options:
    - **SQLite** (default): Simple, local, good for up to ~10k embeddings
    - **PostgreSQL + pgvector**: Scalable, good for large codebases and teams
- **PostgreSQL Installation Method**:
  - **Docker** (recommended if available):
    - Checks if Docker is installed and running
    - Checks if port 5432 is available (or prompts for alternative)
    - Starts PostgreSQL + pgvector container automatically
    - Auto-enables pgvector extension
    - Shows container management commands
  - **System Installation** (fallback):
    - Checks if PostgreSQL is installed
    - Offers to install automatically (brew, apt, dnf supported)
    - Checks for pgvector extension
    - Offers to install pgvector
    - Checks port availability before setup
    - Collects connection details
    - Tests connection and enables pgvector extension
  - Falls back to SQLite if PostgreSQL setup fails

**Step 4: Build and Install**
- Builds all binaries (codetect-mcp, codetect-index, codetect-daemon)
- Asks if you want to install globally
- If yes:
  - Installs to `~/.local/bin`
  - Checks if `~/.local/bin` is in PATH
  - Offers to add to PATH automatically by updating shell profile
  - Creates global config at `~/.config/codetect/config.env`

**Step 5: Configuration**
- Generates configuration file with your selected options
- For SQLite: uses default local storage
- For PostgreSQL: saves DSN connection string
- For Ollama: saves URL and model name
- For LiteLLM: saves URL, API key, and model name

**Step 6: Initial Setup (Optional)**
- If ctags is available, offers to index the codetect codebase
- If semantic search is enabled, offers to generate embeddings
- Shows final summary with:
  - Database backend selection
  - Configuration file location
  - Features enabled/disabled
  - Quick start commands
  - Usage instructions for Claude Code

## Manual Installation

### 1. Clone and Build

```bash
git clone https://github.com/brian-lai/codetect.git
cd codetect
make install
```

### 2. Add to PATH

Add to your shell profile (`~/.zshrc`, `~/.bashrc`, etc.):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then reload your shell:

```bash
source ~/.zshrc  # or ~/.bashrc
```

### 3. Verify Installation

```bash
codetect doctor
```

## Installing Dependencies

**Note:** The `./install.sh` script can automatically install ctags for you on supported platforms (macOS, Ubuntu/Debian, Fedora, Arch Linux). You only need to manually install dependencies if you skip the installer.

### ripgrep (Required)

ripgrep must be installed manually before running the installer:

```bash
# macOS
brew install ripgrep

# Ubuntu/Debian
sudo apt install ripgrep

# Fedora
sudo dnf install ripgrep

# Arch Linux
sudo pacman -S ripgrep

# Windows (with Chocolatey)
choco install ripgrep
```

### universal-ctags (Optional, for symbol indexing)

The installer will offer to install this automatically. Manual installation:

```bash
# macOS
brew install universal-ctags

# Ubuntu/Debian
sudo apt install universal-ctags

# Fedora
sudo dnf install ctags

# Arch Linux
sudo pacman -S ctags
```

### Ollama (Optional, for semantic search)

The installer will detect if Ollama is missing and show prominent warnings. Manual installation:

```bash
# Install from https://ollama.ai
# Download the installer for your platform and run it

# After installation, pull the recommended embedding model
ollama pull bge-m3  # Recommended: best quality for code search

# Or use the smaller default model
# ollama pull nomic-embed-text  # Smaller but lower quality (-47% retrieval)

# See docs/embedding-model-comparison.md for detailed comparison
```

**Important:** The installer will display a prominent red warning box if you enable semantic search but Ollama is not installed. You have two options:
1. Cancel installation, install Ollama, then re-run `./install.sh`
2. Continue without semantic search (you can enable it later)

### LiteLLM (Alternative embedding provider)

```bash
# Install
pip install litellm

# Start the proxy server
litellm --model text-embedding-3-small
```

### PostgreSQL + pgvector (Optional, for scalable vector search)

The installer will offer to install these automatically. Manual installation:

**PostgreSQL:**

```bash
# macOS
brew install postgresql@16
brew services start postgresql@16

# Ubuntu/Debian
sudo apt-get install -y postgresql postgresql-contrib
sudo systemctl start postgresql
sudo systemctl enable postgresql

# Fedora
sudo dnf install -y postgresql-server postgresql-contrib
sudo postgresql-setup --initdb
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

**pgvector extension:**

```bash
# macOS
brew install pgvector

# Ubuntu/Debian (requires PostgreSQL apt repository)
sudo apt-get install -y postgresql-16-pgvector

# From source (all platforms)
git clone https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install
```

**Enable the extension in your database:**

```sql
CREATE EXTENSION IF NOT EXISTS vector;
```

**Note:** The installer handles extension setup automatically when you choose PostgreSQL.

### Docker (Recommended for PostgreSQL)

The installer can automatically set up PostgreSQL + pgvector in Docker. Manual setup:

**Start PostgreSQL:**

```bash
docker-compose up -d postgres
```

**Stop PostgreSQL:**

```bash
docker-compose stop postgres
```

**View logs:**

```bash
docker-compose logs -f postgres
```

**Remove completely (including data):**

```bash
docker-compose down -v
```

**Custom port (if 5432 is in use):**

```bash
POSTGRES_PORT=5433 docker-compose up -d postgres
```

**Connection details:**
- Host: `localhost`
- Port: `5432` (or custom)
- User: `codetect`
- Password: `codetect`
- Database: `codetect`
- DSN: `postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable`

The Docker setup automatically:
- Uses the official `pgvector/pgvector:pg16` image
- Enables the pgvector extension on startup
- Persists data in a named Docker volume (`codetect-pgdata`)
- Includes health checks
- Restarts automatically unless stopped

## Per-Project Setup

After installing codetect globally, set it up in each project:

```bash
cd /path/to/your/project

# Initialize (creates .mcp.json)
codetect init

# Index symbols (requires ctags)
codetect index

# Generate embeddings (requires Ollama or LiteLLM)
codetect embed

# Verify setup
codetect doctor
```

## CLI Commands Reference

### Main Commands

| Command | Description |
|---------|-------------|
| `codetect init` | Initialize codetect in current directory |
| `codetect index` | Index symbols (requires ctags) |
| `codetect embed` | Generate embeddings for semantic search |
| `codetect doctor` | Check installation and dependencies |
| `codetect stats` | Show index statistics |
| `codetect migrate` | Discover existing indexes and register them |
| `codetect update` | Update to latest version from GitHub |
| `codetect help` | Show all commands |

### Daemon Commands

| Command | Description |
|---------|-------------|
| `codetect daemon start` | Start background indexing daemon |
| `codetect daemon stop` | Stop daemon |
| `codetect daemon status` | Show daemon status |
| `codetect daemon logs` | View daemon logs |

### Registry Commands

| Command | Description |
|---------|-------------|
| `codetect registry list` | List registered projects |
| `codetect registry add` | Add current project to registry |
| `codetect registry remove` | Remove a project from registry |
| `codetect registry stats` | Show aggregate statistics |

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CODETECT_DB_TYPE` | Database backend: `sqlite` or `postgres` | `sqlite` |
| `CODETECT_DB_DSN` | PostgreSQL connection string (required if type=postgres) | (none) |
| `CODETECT_DB_PATH` | SQLite database path (used if type=sqlite) | `.codetect/symbols.db` |
| `CODETECT_EMBEDDING_PROVIDER` | Provider: `ollama`, `litellm`, or `off` | `ollama` |
| `CODETECT_OLLAMA_URL` | Ollama server URL | `http://localhost:11434` |
| `CODETECT_LITELLM_URL` | LiteLLM server URL | `http://localhost:4000` |
| `CODETECT_LITELLM_API_KEY` | API key for LiteLLM | (none) |
| `CODETECT_EMBEDDING_MODEL` | Override the embedding model | (provider default) |
| `CODETECT_EMBEDDING_DIMENSIONS` | Override embedding dimensions | (model default) |

### Examples

**Using SQLite (default):**
```bash
codetect embed  # Uses SQLite at .codetect/symbols.db
```

**Using PostgreSQL:**
```bash
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://user:password@localhost/codetect?sslmode=disable"
codetect embed
```

**Using Ollama (default):**
```bash
codetect embed  # Uses Ollama at localhost:11434 with default model (nomic-embed-text)

# To use the recommended model:
CODETECT_EMBEDDING_MODEL=bge-m3 CODETECT_VECTOR_DIMENSIONS=1024 codetect embed
```

**Using a custom Ollama model:**
```bash
# Recommended: Use bge-m3 for best code search quality
CODETECT_EMBEDDING_MODEL=bge-m3 CODETECT_VECTOR_DIMENSIONS=1024 codetect embed

# Other options (see docs/embedding-model-comparison.md):
# CODETECT_EMBEDDING_MODEL=snowflake-arctic-embed CODETECT_VECTOR_DIMENSIONS=1024 codetect embed
# CODETECT_EMBEDDING_MODEL=jina-embeddings-v3 CODETECT_VECTOR_DIMENSIONS=1024 codetect embed
```

**Using LiteLLM with OpenAI:**
```bash
export CODETECT_EMBEDDING_PROVIDER=litellm
export CODETECT_LITELLM_API_KEY=sk-...
export CODETECT_EMBEDDING_MODEL=text-embedding-3-small
codetect embed
```

**Using PostgreSQL + LiteLLM:**
```bash
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://user:password@localhost/codetect?sslmode=disable"
export CODETECT_EMBEDDING_PROVIDER=litellm
export CODETECT_LITELLM_API_KEY=sk-...
codetect embed
```

**Disabling semantic search:**
```bash
CODETECT_EMBEDDING_PROVIDER=off codetect embed  # Skips embedding
```

## Installed Files

After `make install`, files are placed at:

```
~/.local/
├── bin/
│   ├── codetect          # Main CLI (wrapper script)
│   ├── codetect-mcp      # MCP server binary
│   ├── codetect-index    # Indexer binary
│   └── codetect-daemon   # Background daemon binary
└── share/
    └── codetect/
        └── templates/
            └── mcp.json     # Template for new projects
```

Configuration and registry are stored at:

```
~/.config/codetect/
├── config.env              # Global configuration
└── registry.json           # Project registry
```

## Per-Project Files

When you run `codetect init` in a project:

```
your-project/
├── .mcp.json                # MCP server registration
└── .codetect/            # Index storage (gitignored)
    └── symbols.db           # SQLite database
```

## Uninstalling

```bash
cd /path/to/codetect
make uninstall
```

This removes:
- `~/.local/bin/codetect*`
- `~/.local/share/codetect/`

Project-specific files (`.mcp.json`, `.codetect/`) are not removed.

## Troubleshooting

### `codetect: command not found`

Ensure `~/.local/bin` is in your PATH:
```bash
echo $PATH | grep -q "$HOME/.local/bin" || echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

### `ctags not found` warning

Symbol indexing requires universal-ctags. Install it or ignore if you don't need symbol search.

### `Ollama connection refused`

Ensure Ollama is running:
```bash
ollama serve
```

### Embeddings not working

Check provider configuration:
```bash
codetect doctor
```

Try with explicit provider:
```bash
CODETECT_EMBEDDING_PROVIDER=ollama codetect embed
```

### Port 5432 already in use

The installer automatically detects port conflicts. If manually setting up:

**Check what's using the port:**
```bash
lsof -i :5432
# or
netstat -an | grep 5432
```

**For Docker, use a different port:**
```bash
POSTGRES_PORT=5433 docker-compose up -d postgres

# Update your DSN
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5433/codetect?sslmode=disable"
```

**Stop conflicting PostgreSQL:**
```bash
# macOS
brew services stop postgresql@16

# Linux
sudo systemctl stop postgresql
```

### PostgreSQL container won't start

**Check Docker is running:**
```bash
docker ps
```

**View container logs:**
```bash
docker-compose logs postgres
```

**Remove and recreate:**
```bash
docker-compose down -v
docker-compose up -d postgres
```
