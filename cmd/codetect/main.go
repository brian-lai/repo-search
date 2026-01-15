package main

import (
	"log"
	"os"

	"codetect/internal/mcp"
	"codetect/internal/tools"
)

const (
	serverName    = "codetect"
	serverVersion = "0.1.0"
)

func main() {
	// Log to stderr so stdout is clean for MCP JSON-RPC
	log.SetOutput(os.Stderr)
	log.SetPrefix("[codetect] ")

	server := mcp.NewServer(serverName, serverVersion)

	// Register all tools
	tools.RegisterAll(server)

	log.Println("starting MCP server")

	if err := server.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
