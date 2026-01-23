package main

import (
	"os"

	"codetect/internal/logging"
	"codetect/internal/mcp"
	"codetect/internal/tools"
)

const (
	serverName    = "codetect"
	serverVersion = "0.1.0"
)

func main() {
	logger := logging.Default("codetect")

	server := mcp.NewServer(serverName, serverVersion)

	// Register all tools
	tools.RegisterAll(server)

	logger.Info("starting MCP server", "name", serverName, "version", serverVersion)

	if err := server.Run(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
