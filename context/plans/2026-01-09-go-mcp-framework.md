# Plan: Go MCP Framework (mcp-go)

**Status:** Ready for implementation
**Target Repo:** New repo (e.g., `github.com/brian-lai/mcp-go`)

## Objective

Create an open-source Go framework for building MCP (Model Context Protocol) servers. Fill the gap in the ecosystem - there's no mature Go MCP SDK yet (TypeScript and Python have official SDKs).

## Why This Matters

- **Go is ideal for CLI tools** - fast startup, single binary, cross-platform
- **MCP adoption is growing** - Claude Code, Cursor, and other AI tools use it
- **No good Go option exists** - developers are rolling their own (like we did in codetect)

## Design Goals

1. **Simple API** - Register a tool in 5 lines of code
2. **Type-safe** - Use Go generics for typed handlers
3. **Zero dependencies** - Only stdlib (maybe `golang.org/x/` for extras)
4. **Spec-compliant** - Full MCP 2024-11-05 protocol support
5. **Extensible** - Support resources, prompts, sampling (future MCP features)

---

## API Design

### Basic Usage

```go
package main

import "github.com/brian-lai/mcp-go"

func main() {
    server := mcp.NewServer("my-server", "1.0.0")

    server.Tool("greet", "Greet a user by name",
        mcp.Param("name", mcp.String, "The name to greet", mcp.Required),
        func(args mcp.Args) (string, error) {
            name := args.String("name")
            return fmt.Sprintf("Hello, %s!", name), nil
        },
    )

    server.Run()
}
```

### Advanced Usage

```go
// Typed handlers with structs
type SearchArgs struct {
    Query string `mcp:"query,required" desc:"Search query"`
    Limit int    `mcp:"limit" desc:"Max results" default:"10"`
}

type SearchResult struct {
    Results []Result `json:"results"`
    Total   int      `json:"total"`
}

server.TypedTool("search", "Search the database",
    func(args SearchArgs) (SearchResult, error) {
        // Type-safe args, no casting needed
        results := db.Search(args.Query, args.Limit)
        return SearchResult{Results: results, Total: len(results)}, nil
    },
)
```

### With Context & Middleware

```go
server := mcp.NewServer("my-server", "1.0.0",
    mcp.WithLogger(slog.Default()),
    mcp.WithMiddleware(loggingMiddleware),
)

// Handler receives context for cancellation
server.Tool("slow_operation", "Long running task",
    mcp.Param("id", mcp.String, "Task ID", mcp.Required),
    func(ctx context.Context, args mcp.Args) (string, error) {
        select {
        case <-ctx.Done():
            return "", ctx.Err()
        case result := <-doWork(args.String("id")):
            return result, nil
        }
    },
)
```

---

## Package Structure

```
mcp-go/
├── mcp.go              # Main package, NewServer, Run
├── server.go           # Server struct, message loop
├── tool.go             # Tool registration, handlers
├── types.go            # MCP protocol types (Request, Response, Tool, etc.)
├── schema.go           # JSON Schema builders
├── args.go             # Args helper for type-safe argument access
├── transport.go        # Stdio transport (extensible for HTTP later)
├── errors.go           # MCP error codes and helpers
├── options.go          # Server options (WithLogger, WithMiddleware)
│
├── typed/              # Optional: reflection-based typed handlers
│   └── typed.go        # Struct-to-schema generation
│
├── examples/
│   ├── minimal/        # Simplest possible server
│   ├── calculator/     # Multi-tool example
│   └── file-server/    # Resource example
│
└── internal/
    └── jsonrpc/        # JSON-RPC 2.0 implementation
```

---

## Implementation Phases

### Phase 1: Core Protocol (MVP)

**Goal:** Working MCP server that can register and execute tools.

| File | Description |
|------|-------------|
| `types.go` | JSON-RPC 2.0 + MCP types (Request, Response, Tool, etc.) |
| `server.go` | Server struct, stdio message loop |
| `tool.go` | `Tool()` registration, handler execution |
| `args.go` | `Args` helper with `String()`, `Int()`, `Bool()`, etc. |
| `schema.go` | `Param()` builder for input schemas |
| `errors.go` | Standard JSON-RPC error codes |
| `mcp.go` | Public API: `NewServer()`, `Run()` |

**Methods to implement:**
- `initialize` / `initialized`
- `tools/list`
- `tools/call`
- `ping`

**Deliverable:** Can build the "greet" example above.

### Phase 2: Developer Experience

**Goal:** Make it pleasant to use with better ergonomics.

| Feature | Description |
|---------|-------------|
| `WithLogger()` | Structured logging option |
| `WithMiddleware()` | Pre/post handler hooks |
| Context support | Pass `context.Context` to handlers |
| Validation | Auto-validate required params before handler |
| Error wrapping | `mcp.Errorf()` for structured errors |

**Deliverable:** Can build production-quality tools with logging and error handling.

### Phase 3: Typed Handlers (Optional)

**Goal:** Zero-boilerplate tool definitions using reflection.

| Feature | Description |
|---------|-------------|
| `TypedTool()` | Register handler with struct args |
| Struct tags | `mcp:"name,required"` for schema generation |
| Auto-marshal | JSON results without manual serialization |

**Deliverable:** Can define tools with just a struct and function.

### Phase 4: Extended MCP Features

**Goal:** Full MCP spec compliance.

| Feature | Description |
|---------|-------------|
| Resources | `server.Resource()` for file/data serving |
| Prompts | `server.Prompt()` for pre-configured prompts |
| Capabilities | Proper capability negotiation |
| Notifications | `tools/list_changed`, etc. |

### Phase 5: Alternative Transports

**Goal:** Support beyond stdio.

| Transport | Description |
|-----------|-------------|
| HTTP/SSE | For web-based MCP clients |
| WebSocket | Bidirectional communication |

---

## Type Definitions (Core)

```go
// types.go

// Server is an MCP server instance
type Server struct {
    name     string
    version  string
    tools    []Tool
    handlers map[string]Handler
    options  serverOptions
}

// Tool represents an MCP tool definition
type Tool struct {
    Name        string      `json:"name"`
    Description string      `json:"description"`
    InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema is a JSON Schema for tool inputs
type InputSchema struct {
    Type       string              `json:"type"`
    Properties map[string]Property `json:"properties,omitempty"`
    Required   []string            `json:"required,omitempty"`
}

// Property describes a single tool parameter
type Property struct {
    Type        string `json:"type"`
    Description string `json:"description"`
}

// Handler is the function signature for tool handlers
type Handler func(ctx context.Context, args Args) (any, error)

// Args provides type-safe access to tool arguments
type Args struct {
    raw map[string]any
}

func (a Args) String(key string) string
func (a Args) Int(key string) int
func (a Args) Float(key string) float64
func (a Args) Bool(key string) bool
func (a Args) StringOr(key, def string) string
func (a Args) IntOr(key string, def int) int
```

---

## Schema Builder API

```go
// schema.go

type ParamType string

const (
    String  ParamType = "string"
    Number  ParamType = "number"
    Integer ParamType = "integer"
    Boolean ParamType = "boolean"
    Array   ParamType = "array"
    Object  ParamType = "object"
)

type ParamOption func(*paramConfig)

func Required(p *paramConfig)          { p.required = true }
func Default(v any) ParamOption        { return func(p *paramConfig) { p.defaultValue = v } }
func Enum(values ...string) ParamOption { return func(p *paramConfig) { p.enum = values } }

// Param creates a parameter definition
func Param(name string, typ ParamType, description string, opts ...ParamOption) ParamDef
```

---

## Error Handling

```go
// errors.go

// Standard JSON-RPC 2.0 error codes
const (
    ErrParse          = -32700
    ErrInvalidRequest = -32600
    ErrMethodNotFound = -32601
    ErrInvalidParams  = -32602
    ErrInternal       = -32603
)

// ToolError wraps errors returned from tool handlers
type ToolError struct {
    Message string
    Data    any
}

func (e ToolError) Error() string { return e.Message }

// Errorf creates a structured tool error
func Errorf(format string, args ...any) error {
    return ToolError{Message: fmt.Sprintf(format, args...)}
}
```

---

## Testing Strategy

### Unit Tests
- `server_test.go` - Message handling, routing
- `tool_test.go` - Tool registration, schema generation
- `args_test.go` - Argument parsing, type coercion

### Integration Tests
- Spawn server, send JSON-RPC messages via stdin, verify stdout
- Test full MCP handshake flow

### Example-Based Tests
- Each example should be runnable and testable
- `go test ./examples/...`

---

## Documentation

### README.md
- Quick start (5-minute guide)
- Installation
- Basic example
- API reference links

### Examples
- `examples/minimal/` - Hello world
- `examples/calculator/` - Multiple tools
- `examples/database/` - Real-world tool with external deps

### GoDoc
- Full API documentation
- Usage examples in doc comments

---

## Verification

After implementation, verify with:

```bash
# Build minimal example
cd examples/minimal
go build -o minimal-server

# Test with manual JSON-RPC
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./minimal-server

echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | ./minimal-server

echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"greet","arguments":{"name":"World"}}}' | ./minimal-server

# Run tests
go test ./...

# Test with Claude Code
# Create .mcp.json pointing to the example server
```

---

## Success Criteria

- [ ] `go get github.com/brian-lai/mcp-go` works
- [ ] Minimal example compiles and runs
- [ ] Works with Claude Code via `.mcp.json`
- [ ] Zero external dependencies (stdlib only)
- [ ] <500 lines of code for core package
- [ ] GoDoc documentation complete
- [ ] README with quick start guide

---

## Reference

- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [MCP TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
- [MCP Python SDK](https://github.com/modelcontextprotocol/python-sdk)
- [codetect implementation](https://github.com/brian-lai/codetect/tree/main/internal/mcp)

---

## Future Enhancements (Out of Scope)

- Code generation from OpenAPI/JSON Schema
- MCP client library (for calling other MCP servers)
- Visual tool builder / playground
- Homebrew formula
