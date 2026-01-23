// codetect-daemon is the background indexing daemon for codetect.
// It watches registered projects for file changes and triggers re-indexing.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"codetect/internal/daemon"
	"codetect/internal/logging"
	"codetect/internal/registry"
)

var logger *slog.Logger

func main() {
	logger = logging.Default("codetect-daemon")

	// Subcommands
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		cmdStart(os.Args[2:])
	case "stop":
		cmdStop()
	case "status":
		cmdStatus()
	case "help", "--help", "-h":
		printUsage()
	default:
		logger.Error("unknown command", "command", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("codetect-daemon - Background indexing daemon")
	fmt.Println()
	fmt.Println("Usage: codetect-daemon <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  start     Start the daemon")
	fmt.Println("  stop      Stop the daemon")
	fmt.Println("  status    Show daemon status")
	fmt.Println("  help      Show this help")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  CODETECT_LOG_LEVEL   Log level (debug, info, warn, error) [default: info]")
	fmt.Println("  CODETECT_LOG_FORMAT  Output format (text, json) [default: text]")
}

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	foreground := fs.Bool("foreground", false, "Run in foreground (don't daemonize)")
	fs.Parse(args)

	// Check if already running
	client := daemon.NewIPCClient(daemon.DefaultSocketPath())
	if client.IsRunning() {
		logger.Error("daemon is already running")
		os.Exit(1)
	}

	// Load registry
	reg, err := registry.NewRegistry()
	if err != nil {
		logger.Error("failed to load registry", "error", err)
		os.Exit(1)
	}

	// Create daemon config
	cfg := daemon.DefaultConfig()

	// Create and run daemon
	d, err := daemon.New(reg, cfg)
	if err != nil {
		logger.Error("failed to create daemon", "error", err)
		os.Exit(1)
	}

	if *foreground {
		logger.Info("starting daemon in foreground", "pid", os.Getpid())
		if err := d.Run(cfg); err != nil {
			logger.Error("daemon error", "error", err)
			os.Exit(1)
		}
	} else {
		// For now, just run in foreground
		// TODO: proper daemonization using fork or service manager
		logger.Info("daemon started", "pid", os.Getpid())
		logger.Info("note: Run with --foreground or use '&' to background")
		if err := d.Run(cfg); err != nil {
			logger.Error("daemon error", "error", err)
			os.Exit(1)
		}
	}
}

func cmdStop() {
	client := daemon.NewIPCClient(daemon.DefaultSocketPath())
	if !client.IsRunning() {
		logger.Error("daemon is not running")
		os.Exit(1)
	}

	if err := client.Stop(); err != nil {
		logger.Error("failed to stop daemon", "error", err)
		os.Exit(1)
	}

	logger.Info("daemon stopped")
}

func cmdStatus() {
	client := daemon.NewIPCClient(daemon.DefaultSocketPath())

	status, err := client.Status()
	if err != nil {
		logger.Info("daemon is not running")
		os.Exit(1)
	}

	// Print status as JSON to stdout (data output)
	data, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(data))
}
