package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// Command represents a request from the CLI to the daemon
type Command struct {
	Action string `json:"action"` // status, stop, reindex, add, remove
	Path   string `json:"path,omitempty"`
}

// Response represents a response from the daemon to the CLI
type Response struct {
	Status  string `json:"status"` // ok, error
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// IPCServer handles communication between CLI and daemon
type IPCServer struct {
	socketPath string
	listener   net.Listener
	daemon     *Daemon
}

// NewIPCServer creates a new IPC server
func NewIPCServer(socketPath string, daemon *Daemon) (*IPCServer, error) {
	// Remove stale socket if it exists
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on socket: %w", err)
	}

	return &IPCServer{
		socketPath: socketPath,
		listener:   listener,
		daemon:     daemon,
	}, nil
}

// Close shuts down the IPC server
func (s *IPCServer) Close() error {
	os.Remove(s.socketPath)
	return s.listener.Close()
}

// Serve handles incoming connections
func (s *IPCServer) Serve(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		go s.handleConnection(ctx, conn)
	}
}

// handleConnection processes a single client connection
func (s *IPCServer) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}

	var cmd Command
	if err := json.Unmarshal(line, &cmd); err != nil {
		s.sendResponse(conn, Response{Status: "error", Message: "invalid command"})
		return
	}

	resp := s.handleCommand(ctx, cmd)
	s.sendResponse(conn, resp)
}

// handleCommand processes a command and returns a response
func (s *IPCServer) handleCommand(ctx context.Context, cmd Command) Response {
	switch cmd.Action {
	case "status":
		status := s.daemon.Status()
		return Response{Status: "ok", Data: status}

	case "stop":
		s.daemon.Stop()
		return Response{Status: "ok", Message: "daemon stopping"}

	case "reindex":
		if cmd.Path == "" {
			return Response{Status: "error", Message: "path required"}
		}
		if err := s.daemon.TriggerReindex(cmd.Path); err != nil {
			return Response{Status: "error", Message: err.Error()}
		}
		return Response{Status: "ok", Message: "reindex queued"}

	case "add":
		if cmd.Path == "" {
			return Response{Status: "error", Message: "path required"}
		}
		if err := s.daemon.AddProject(cmd.Path); err != nil {
			return Response{Status: "error", Message: err.Error()}
		}
		return Response{Status: "ok", Message: "project added"}

	case "remove":
		if cmd.Path == "" {
			return Response{Status: "error", Message: "path required"}
		}
		if err := s.daemon.RemoveProject(cmd.Path); err != nil {
			return Response{Status: "error", Message: err.Error()}
		}
		return Response{Status: "ok", Message: "project removed"}

	default:
		return Response{Status: "error", Message: "unknown action"}
	}
}

// sendResponse sends a JSON response to the client
func (s *IPCServer) sendResponse(conn net.Conn, resp Response) {
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}

// IPCClient is used by CLI to communicate with daemon
type IPCClient struct {
	socketPath string
}

// NewIPCClient creates a new IPC client
func NewIPCClient(socketPath string) *IPCClient {
	return &IPCClient{socketPath: socketPath}
}

// DefaultSocketPath returns the default socket path for the current user
func DefaultSocketPath() string {
	return fmt.Sprintf("/tmp/codetect-%d.sock", os.Getuid())
}

// Send sends a command to the daemon and returns the response
func (c *IPCClient) Send(cmd Command) (*Response, error) {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer conn.Close()

	// Send command
	data, _ := json.Marshal(cmd)
	conn.Write(append(data, '\n'))

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// IsRunning checks if the daemon is running
func (c *IPCClient) IsRunning() bool {
	resp, err := c.Send(Command{Action: "status"})
	return err == nil && resp.Status == "ok"
}

// Stop tells the daemon to shut down
func (c *IPCClient) Stop() error {
	_, err := c.Send(Command{Action: "stop"})
	return err
}

// Status returns the daemon status
func (c *IPCClient) Status() (*DaemonStatus, error) {
	resp, err := c.Send(Command{Action: "status"})
	if err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("%s", resp.Message)
	}

	// Convert Data to DaemonStatus
	data, _ := json.Marshal(resp.Data)
	var status DaemonStatus
	json.Unmarshal(data, &status)
	return &status, nil
}

// Reindex triggers reindexing for a project
func (c *IPCClient) Reindex(path string) error {
	resp, err := c.Send(Command{Action: "reindex", Path: path})
	if err != nil {
		return err
	}
	if resp.Status != "ok" {
		return fmt.Errorf("%s", resp.Message)
	}
	return nil
}
