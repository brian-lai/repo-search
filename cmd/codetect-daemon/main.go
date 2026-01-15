// codetect-daemon is the background indexing daemon for codetect.
// It watches registered projects for file changes and triggers re-indexing.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"codetect/internal/daemon"
	"codetect/internal/registry"
)

func main() {
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
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
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
}

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	foreground := fs.Bool("foreground", false, "Run in foreground (don't daemonize)")
	fs.Parse(args)

	// Check if already running
	client := daemon.NewIPCClient(daemon.DefaultSocketPath())
	if client.IsRunning() {
		fmt.Println("Daemon is already running")
		os.Exit(1)
	}

	// Load registry
	reg, err := registry.NewRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load registry: %v\n", err)
		os.Exit(1)
	}

	// Create daemon config
	cfg := daemon.DefaultConfig()

	// Create and run daemon
	d, err := daemon.New(reg, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create daemon: %v\n", err)
		os.Exit(1)
	}

	if *foreground {
		fmt.Printf("Starting daemon in foreground (PID: %d)\n", os.Getpid())
		if err := d.Run(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Daemon error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// For now, just run in foreground
		// TODO: proper daemonization using fork or service manager
		fmt.Printf("Daemon started (PID: %d)\n", os.Getpid())
		fmt.Println("Note: Run with --foreground or use '&' to background")
		if err := d.Run(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Daemon error: %v\n", err)
			os.Exit(1)
		}
	}
}

func cmdStop() {
	client := daemon.NewIPCClient(daemon.DefaultSocketPath())
	if !client.IsRunning() {
		fmt.Println("Daemon is not running")
		os.Exit(1)
	}

	if err := client.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stop daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Daemon stopped")
}

func cmdStatus() {
	client := daemon.NewIPCClient(daemon.DefaultSocketPath())

	status, err := client.Status()
	if err != nil {
		fmt.Println("Daemon is not running")
		os.Exit(1)
	}

	// Print status as JSON
	data, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(data))
}
