package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"codetect/internal/logging"
)

const ProtocolVersion = "2024-11-05"

// ToolHandler is the function signature for handling tool calls
type ToolHandler func(args map[string]interface{}) (*ToolsCallResult, error)

// Server handles MCP JSON-RPC communication over stdio
type Server struct {
	name     string
	version  string
	tools    []Tool
	handlers map[string]ToolHandler
	logger   *slog.Logger
}

// NewServer creates a new MCP server
func NewServer(name, version string) *Server {
	return &Server{
		name:     name,
		version:  version,
		tools:    []Tool{},
		handlers: make(map[string]ToolHandler),
		logger:   logging.Default("mcp"),
	}
}

// RegisterTool adds a tool to the server
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.tools = append(s.tools, tool)
	s.handlers[tool.Name] = handler
}

// Run starts the server and processes stdin/stdout
func (s *Server) Run() error {
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("reading stdin: %w", err)
		}

		if len(line) == 0 || string(line) == "\n" {
			continue
		}

		response := s.handleMessage(line)
		if response != nil {
			if err := s.writeResponse(response); err != nil {
				s.logger.Error("error writing response", "error", err)
			}
		}
	}
}

func (s *Server) handleMessage(data []byte) *Response {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Error("parse error", "error", err)
		return &Response{
			JSONRPC: "2.0",
			Error: &Error{
				Code:    ParseError,
				Message: "Parse error",
				Data:    err.Error(),
			},
		}
	}

	s.logger.Debug("received request", "method", req.Method, "id", req.ID)

	switch req.Method {
	case "initialize":
		return s.handleInitialize(&req)
	case "initialized":
		// Notification, no response needed
		return nil
	case "tools/list":
		return s.handleToolsList(&req)
	case "tools/call":
		return s.handleToolsCall(&req)
	case "ping":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{},
		}
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    MethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
		ServerInfo: ServerInfo{
			Name:    s.name,
			Version: s.version,
		},
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleToolsList(req *Request) *Response {
	result := ToolsListResult{
		Tools: s.tools,
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleToolsCall(req *Request) *Response {
	// Parse params
	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    InvalidParams,
				Message: "Invalid params",
			},
		}
	}

	var params ToolsCallParams
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    InvalidParams,
				Message: "Invalid params",
			},
		}
	}

	handler, ok := s.handlers[params.Name]
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    MethodNotFound,
				Message: fmt.Sprintf("Tool not found: %s", params.Name),
			},
		}
	}

	result, err := handler(params.Arguments)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: &ToolsCallResult{
				Content: []Content{{Type: "text", Text: err.Error()}},
				IsError: true,
			},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) writeResponse(resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "%s\n", data)
	return err
}
