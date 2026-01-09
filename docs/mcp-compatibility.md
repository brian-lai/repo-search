# MCP Compatibility & Multi-Tool Support

This document outlines repo-search's compatibility with MCP (Model Context Protocol) clients and plans for supporting other CLI-based LLM tools.

## What is MCP?

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/) is an open standard developed by Anthropic for connecting AI assistants to external tools and data sources. It provides a standardized way for LLM applications to:

- Discover available tools
- Call tools with structured inputs
- Receive structured outputs

repo-search implements an MCP server that exposes code search capabilities via the stdio transport.

## Current MCP Client Support

| Tool | Support Status | Notes |
|------|----------------|-------|
| **Claude Code** | Fully supported | Primary target, tested extensively |
| **Cursor** | Should work | MCP support added late 2024 |
| **Cline (VS Code)** | Should work | Full MCP support |
| **Continue** | Should work | Open source IDE extension with MCP |
| **Zed** | Should work | Built-in MCP support |

### Tested Configurations

- **Claude Code** on macOS with stdio transport

### Community-Reported Configurations

We welcome reports of repo-search working with other MCP clients. Please open an issue or PR to add your configuration.

## Configuration for MCP Clients

repo-search uses the standard `.mcp.json` configuration format:

```json
{
  "mcpServers": {
    "repo-search": {
      "command": "repo-search-mcp",
      "args": [],
      "cwd": "/path/to/your/project"
    }
  }
}
```

Most MCP clients will automatically discover this configuration when placed in the project root.

### Client-Specific Setup

#### Claude Code

```bash
cd /path/to/project
repo-search init    # Creates .mcp.json
repo-search index   # Index symbols
claude              # Start Claude Code
```

#### Cursor

Cursor reads `.mcp.json` from the workspace root. After running `repo-search init`, restart Cursor to pick up the configuration.

#### Cline / Continue

These VS Code extensions typically read MCP configuration from:
- `.mcp.json` in workspace root
- VS Code settings

## Non-MCP Tool Support

### Current Status

repo-search is currently MCP-only. Tools that don't support MCP cannot directly use it.

### Tools Without MCP Support

| Tool | MCP Support | Alternative Approach |
|------|-------------|---------------------|
| **OpenAI Codex CLI** | No | Planned: HTTP API mode |
| **Google Gemini CLI** | No | Planned: HTTP API mode |
| **Aider** | No | Planned: CLI mode |
| **GPT-Engineer** | No | Planned: CLI mode |

## Roadmap: Multi-Tool Support

We plan to expand repo-search to support non-MCP tools through additional interfaces:

### Phase 1: HTTP API Mode (Planned)

Add an HTTP server mode that exposes the same tools via REST API:

```bash
# Start HTTP server
repo-search serve --port 8080

# Query via curl
curl -X POST http://localhost:8080/search_keyword \
  -H "Content-Type: application/json" \
  -d '{"query": "func main", "top_k": 5}'
```

This would enable integration with any tool that can make HTTP requests.

### Phase 2: CLI Query Mode (Planned)

Add direct CLI commands for querying:

```bash
# Direct CLI usage
repo-search search "func main" --limit 5
repo-search symbol Server --kind struct
repo-search semantic "error handling logic"
```

This would allow tools to shell out to repo-search directly.

### Phase 3: Language Server Protocol (Considered)

LSP integration could provide IDE-native symbol search and navigation, complementing the MCP interface.

## Contributing

We welcome contributions to expand tool support:

1. **Testing with MCP clients**: Report which clients work with repo-search
2. **HTTP API implementation**: Help build the REST interface
3. **CLI query mode**: Help build direct CLI commands
4. **Documentation**: Add setup guides for specific tools

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.

## Resources

- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [MCP TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
- [MCP Go SDK](https://github.com/mark3labs/mcp-go)
- [Claude Code Documentation](https://docs.anthropic.com/claude-code)
