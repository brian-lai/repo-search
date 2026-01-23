# Plan: Global Installation for codetect

## Objective

Enable codetect to be used in **any project** with minimal setup:
1. Install once globally (`make install`)
2. Initialize any project (`codetect init`)
3. Index before Claude starts (`codetect index`)
4. Claude Code has access to search tools via `.mcp.json`

## User Workflow (After Implementation)

```bash
# One-time setup
cd /path/to/codetect
make install

# In any project
cd /path/to/my-backend-service
codetect init      # Creates .mcp.json
codetect index     # Index symbols
codetect embed     # Optional: generate embeddings
claude                # Start Claude Code (MCP auto-starts)
```

## Implementation

### 1. Create Wrapper Script
**File:** `scripts/codetect-wrapper.sh` → installed as `~/.local/bin/codetect`

Commands:
- `codetect mcp` - Start MCP server (for .mcp.json)
- `codetect index [path]` - Index symbols
- `codetect embed [path]` - Generate embeddings
- `codetect init` - Create .mcp.json in current directory
- `codetect doctor` - Check installation & dependencies
- `codetect stats` - Show index stats

### 2. Create Template `.mcp.json`
**File:** `templates/mcp.json`

```json
{
  "mcpServers": {
    "codetect": {
      "command": "codetect",
      "args": ["mcp"]
    }
  }
}
```

### 3. Add Makefile Targets
**File:** `Makefile` (modify)

```makefile
PREFIX ?= $(HOME)/.local

install: build
    # Install binaries
    cp dist/codetect $(PREFIX)/bin/codetect-mcp
    cp dist/codetect-index $(PREFIX)/bin/codetect-index
    # Install wrapper
    cp scripts/codetect-wrapper.sh $(PREFIX)/bin/codetect
    # Install templates
    cp templates/mcp.json $(PREFIX)/share/codetect/templates/

uninstall:
    rm -f $(PREFIX)/bin/codetect*
    rm -rf $(PREFIX)/share/codetect
```

### 4. Global Config Location
**File:** `~/.config/codetect/config.env`

Stores embedding provider settings (shared across all projects):
```bash
export CODETECT_EMBEDDING_PROVIDER=ollama
export CODETECT_OLLAMA_URL=http://localhost:11434
```

## Directory Structure After Install

```
~/.local/
├── bin/
│   ├── codetect          # Wrapper script (main entry point)
│   ├── codetect-mcp      # MCP server binary
│   └── codetect-index    # Indexer binary
└── share/
    └── codetect/
        └── templates/
            └── mcp.json     # Template for new projects

~/.config/
└── codetect/
    └── config.env           # Global embedding config
```

## Per-Project Structure

```
/path/to/any-project/
├── .mcp.json                # Created by `codetect init`
└── .codetect/            # Created by `codetect index`
    └── symbols.db           # SQLite database (gitignored)
```

## Files to Create/Modify

| File | Action |
|------|--------|
| `scripts/codetect-wrapper.sh` | CREATE - Main wrapper script |
| `templates/mcp.json` | CREATE - Template for projects |
| `Makefile` | MODIFY - Add install/uninstall targets |
| `README.md` | MODIFY - Add global installation docs |

## Verification

After implementation:
```bash
# Install
make install

# Test in a different directory
cd /tmp
mkdir test-project && cd test-project
codetect init
codetect doctor
cat .mcp.json  # Should show codetect config
```

## Future Enhancements (Not in Scope)

- Homebrew formula
- Auto-index git hook template
- `codetect-claude` wrapper that auto-indexes before launching Claude
