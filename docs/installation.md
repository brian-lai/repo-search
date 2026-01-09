# Installation Guide

This guide covers all installation methods for repo-search.

## Requirements

### Required

- **Go 1.21+** - for building from source
- **[ripgrep](https://github.com/BurntSushi/ripgrep)** (`rg`) - for keyword search

### Optional

- **[universal-ctags](https://github.com/universal-ctags/ctags)** - for symbol indexing
- **[Ollama](https://ollama.ai)** - for semantic search (default embedding provider)
- **[LiteLLM](https://github.com/BerriAI/litellm)** - alternative embedding provider

## Quick Start (Recommended)

The interactive installer handles everything:

```bash
git clone https://github.com/brian-lai/repo-search.git
cd repo-search
./install.sh
```

The installer will:
1. Check dependencies (Go, ripgrep, ctags)
2. Let you choose an embedding provider (Ollama, LiteLLM, or none)
3. Configure provider settings
4. Build and optionally install globally
5. Optionally index the current codebase

## Manual Installation

### 1. Clone and Build

```bash
git clone https://github.com/brian-lai/repo-search.git
cd repo-search
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
repo-search doctor
```

## Installing Dependencies

### ripgrep (Required)

```bash
# macOS
brew install ripgrep

# Ubuntu/Debian
apt install ripgrep

# Fedora
dnf install ripgrep

# Arch Linux
pacman -S ripgrep

# Windows (with Chocolatey)
choco install ripgrep
```

### universal-ctags (Optional, for symbol indexing)

```bash
# macOS
brew install universal-ctags

# Ubuntu/Debian
apt install universal-ctags

# Fedora
dnf install ctags

# Arch Linux
pacman -S ctags
```

### Ollama (Optional, for semantic search)

```bash
# Install from https://ollama.ai

# Pull the embedding model
ollama pull nomic-embed-text
```

### LiteLLM (Alternative embedding provider)

```bash
# Install
pip install litellm

# Start the proxy server
litellm --model text-embedding-3-small
```

## Per-Project Setup

After installing repo-search globally, set it up in each project:

```bash
cd /path/to/your/project

# Initialize (creates .mcp.json)
repo-search init

# Index symbols (requires ctags)
repo-search index

# Generate embeddings (requires Ollama or LiteLLM)
repo-search embed

# Verify setup
repo-search doctor
```

## CLI Commands Reference

| Command | Description |
|---------|-------------|
| `repo-search init` | Initialize repo-search in current directory |
| `repo-search index` | Index symbols (requires ctags) |
| `repo-search embed` | Generate embeddings for semantic search |
| `repo-search doctor` | Check installation and dependencies |
| `repo-search stats` | Show index statistics |
| `repo-search update` | Update to latest version from GitHub |
| `repo-search help` | Show all commands |

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REPO_SEARCH_EMBEDDING_PROVIDER` | Provider: `ollama`, `litellm`, or `off` | `ollama` |
| `REPO_SEARCH_OLLAMA_URL` | Ollama server URL | `http://localhost:11434` |
| `REPO_SEARCH_LITELLM_URL` | LiteLLM server URL | `http://localhost:4000` |
| `REPO_SEARCH_LITELLM_API_KEY` | API key for LiteLLM | (none) |
| `REPO_SEARCH_EMBEDDING_MODEL` | Override the embedding model | (provider default) |
| `REPO_SEARCH_EMBEDDING_DIMENSIONS` | Override embedding dimensions | (model default) |

### Examples

**Using Ollama (default):**
```bash
repo-search embed  # Uses Ollama at localhost:11434 with nomic-embed-text
```

**Using a custom Ollama model:**
```bash
REPO_SEARCH_EMBEDDING_MODEL=mxbai-embed-large repo-search embed
```

**Using LiteLLM with OpenAI:**
```bash
export REPO_SEARCH_EMBEDDING_PROVIDER=litellm
export REPO_SEARCH_LITELLM_API_KEY=sk-...
export REPO_SEARCH_EMBEDDING_MODEL=text-embedding-3-small
repo-search embed
```

**Disabling semantic search:**
```bash
REPO_SEARCH_EMBEDDING_PROVIDER=off repo-search embed  # Skips embedding
```

## Installed Files

After `make install`, files are placed at:

```
~/.local/
├── bin/
│   ├── repo-search          # Main CLI (wrapper script)
│   ├── repo-search-mcp      # MCP server binary
│   └── repo-search-index    # Indexer binary
└── share/
    └── repo-search/
        └── templates/
            └── mcp.json     # Template for new projects
```

## Per-Project Files

When you run `repo-search init` in a project:

```
your-project/
├── .mcp.json                # MCP server registration
└── .repo_search/            # Index storage (gitignored)
    └── symbols.db           # SQLite database
```

## Uninstalling

```bash
cd /path/to/repo-search
make uninstall
```

This removes:
- `~/.local/bin/repo-search*`
- `~/.local/share/repo-search/`

Project-specific files (`.mcp.json`, `.repo_search/`) are not removed.

## Troubleshooting

### `repo-search: command not found`

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
repo-search doctor
```

Try with explicit provider:
```bash
REPO_SEARCH_EMBEDDING_PROVIDER=ollama repo-search embed
```
