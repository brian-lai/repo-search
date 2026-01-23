// Package daemon provides background indexing for codetect.
// It watches registered projects for file changes and triggers re-indexing.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"codetect/internal/logging"
	"codetect/internal/registry"

	"github.com/fsnotify/fsnotify"
	ignore "github.com/sabhiram/go-gitignore"
)

// Daemon manages background file watching and indexing
type Daemon struct {
	registry    *registry.Registry
	watcher     *fsnotify.Watcher
	indexQueue  chan string
	debounceMap map[string]*time.Timer
	debounceMu  sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *slog.Logger
	logFile     *os.File
}

// DaemonStatus represents the current state of the daemon
type DaemonStatus struct {
	Running         bool      `json:"running"`
	PID             int       `json:"pid"`
	StartedAt       time.Time `json:"started_at"`
	WatchedProjects int       `json:"watched_projects"`
	TotalWatches    int       `json:"total_watches"`
}

// Config holds daemon configuration
type Config struct {
	DebounceMs int
	LogPath    string
	PIDPath    string
	SocketPath string
}

// DefaultConfig returns the default daemon configuration
func DefaultConfig() Config {
	configDir := registry.DefaultConfigDir()
	uid := os.Getuid()
	return Config{
		DebounceMs: 500,
		LogPath:    filepath.Join(configDir, "daemon.log"),
		PIDPath:    filepath.Join(configDir, "daemon.pid"),
		SocketPath: fmt.Sprintf("/tmp/codetect-%d.sock", uid),
	}
}

// New creates a new daemon instance
func New(reg *registry.Registry, cfg Config) (*Daemon, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Setup logging - use file if configured, otherwise use logging package defaults
	var logFile *os.File
	var logger *slog.Logger
	if cfg.LogPath != "" {
		logFile, err = os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			watcher.Close()
			cancel()
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		// Create slog logger with file output
		logCfg := logging.LoadConfigFromEnv("codetect-daemon")
		logCfg.Output = logFile
		logger = logging.New(logCfg)
	} else {
		logger = logging.Default("codetect-daemon")
	}

	return &Daemon{
		registry:    reg,
		watcher:     watcher,
		indexQueue:  make(chan string, 100),
		debounceMap: make(map[string]*time.Timer),
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
		logFile:     logFile,
	}, nil
}

// Run starts the daemon and blocks until shutdown
func (d *Daemon) Run(cfg Config) error {
	d.logger.Info("daemon starting")

	// Write PID file
	if err := d.writePIDFile(cfg.PIDPath); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	defer os.Remove(cfg.PIDPath)

	// Setup signal handlers
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Start watching registered projects
	if err := d.watchAllProjects(); err != nil {
		d.logger.Warn("error watching projects", "error", err)
	}

	// Start IPC server
	ipcServer, err := NewIPCServer(cfg.SocketPath, d)
	if err != nil {
		return fmt.Errorf("failed to start IPC server: %w", err)
	}
	defer ipcServer.Close()

	go ipcServer.Serve(d.ctx)

	// Start index worker
	go d.indexWorker()

	// Start watcher event handler
	go d.watcherLoop()

	d.logger.Info("daemon started", "pid", os.Getpid())

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		d.logger.Info("received signal", "signal", sig)
	case <-d.ctx.Done():
		d.logger.Info("context cancelled")
	}

	// Cleanup
	d.logger.Info("daemon shutting down")
	d.cancel()
	d.watcher.Close()
	if d.logFile != nil {
		d.logFile.Close()
	}

	return nil
}

// Stop signals the daemon to shut down
func (d *Daemon) Stop() {
	d.cancel()
}

// Status returns the current daemon status
func (d *Daemon) Status() DaemonStatus {
	return DaemonStatus{
		Running:         true,
		PID:             os.Getpid(),
		StartedAt:       time.Now(), // TODO: track actual start time
		WatchedProjects: len(d.registry.GetWatchedProjects()),
		TotalWatches:    len(d.watcher.WatchList()),
	}
}

// watchAllProjects adds watches for all registered projects
func (d *Daemon) watchAllProjects() error {
	projects := d.registry.GetWatchedProjects()
	d.logger.Info("watching projects", "count", len(projects))

	for _, p := range projects {
		if err := d.watchProject(p.Path); err != nil {
			d.logger.Error("failed to watch project", "path", p.Path, "error", err)
		}
	}
	return nil
}

// maxWatchesPerProject limits file watchers to prevent file descriptor exhaustion
const maxWatchesPerProject = 1000

// watchProject adds watches for all directories in a project
func (d *Daemon) watchProject(projectPath string) error {
	count := 0
	var limitReached bool

	// Load gitignore patterns for this project
	gi := loadGitignore(projectPath)

	err := filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if entry.IsDir() {
			// Check hardcoded ignore list first
			if isIgnoredDir(entry.Name()) {
				return filepath.SkipDir
			}

			// Check gitignore patterns
			if gi != nil {
				relPath, err := filepath.Rel(projectPath, path)
				if err == nil && gi.MatchesPath(relPath+"/") {
					return filepath.SkipDir
				}
			}

			if count >= maxWatchesPerProject {
				if !limitReached {
					d.logger.Warn("reached max watches limit", "limit", maxWatchesPerProject, "project", projectPath)
					limitReached = true
				}
				return filepath.SkipDir
			}
			if err := d.watcher.Add(path); err != nil {
				return nil // Skip errors
			}
			count++
		}
		return nil
	})
	d.logger.Debug("added watches", "count", count, "project", projectPath)
	return err
}

// unwatchProject removes watches for a project
func (d *Daemon) unwatchProject(projectPath string) error {
	watchList := d.watcher.WatchList()
	for _, path := range watchList {
		if isSubpath(path, projectPath) {
			d.watcher.Remove(path)
		}
	}
	return nil
}

// watcherLoop handles fsnotify events
func (d *Daemon) watcherLoop() {
	debounceMs := d.registry.Settings().DebounceMs
	if debounceMs <= 0 {
		debounceMs = 500
	}
	debounceDuration := time.Duration(debounceMs) * time.Millisecond

	for {
		select {
		case <-d.ctx.Done():
			return
		case event, ok := <-d.watcher.Events:
			if !ok {
				return
			}
			d.handleEvent(event, debounceDuration)
		case err, ok := <-d.watcher.Errors:
			if !ok {
				return
			}
			d.logger.Error("watcher error", "error", err)
		}
	}
}

// handleEvent processes a file system event with debouncing
func (d *Daemon) handleEvent(event fsnotify.Event, debounceDuration time.Duration) {
	// Skip non-code files
	if !isCodeFile(event.Name) && !event.Has(fsnotify.Create) {
		return
	}

	// Handle new directories
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if !isIgnoredDir(filepath.Base(event.Name)) {
				d.watcher.Add(event.Name)
			}
		}
	}

	// Find which project this event belongs to
	project := d.findProjectForPath(event.Name)
	if project == "" {
		return
	}

	// Debounce: reset timer for this project
	d.debounceMu.Lock()
	if timer, ok := d.debounceMap[project]; ok {
		timer.Stop()
	}
	d.debounceMap[project] = time.AfterFunc(debounceDuration, func() {
		d.debounceMu.Lock()
		delete(d.debounceMap, project)
		d.debounceMu.Unlock()

		select {
		case d.indexQueue <- project:
			d.logger.Debug("queued reindex", "project", project)
		default:
			d.logger.Warn("index queue full, skipping", "project", project)
		}
	})
	d.debounceMu.Unlock()
}

// findProjectForPath returns the project path that contains the given path
func (d *Daemon) findProjectForPath(path string) string {
	projects := d.registry.GetWatchedProjects()
	for _, p := range projects {
		if isSubpath(path, p.Path) {
			return p.Path
		}
	}
	return ""
}

// indexWorker processes the index queue
func (d *Daemon) indexWorker() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case projectPath := <-d.indexQueue:
			d.runIndex(projectPath)
		}
	}
}

// runIndex executes the indexer for a project
func (d *Daemon) runIndex(projectPath string) {
	d.logger.Info("indexing", "project", projectPath)

	// Run codetect-index
	cmd := exec.CommandContext(d.ctx, "codetect-index", "index", projectPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		d.logger.Error("index failed", "project", projectPath, "error", err, "output", string(output))
		return
	}

	d.logger.Info("index completed", "project", projectPath)

	// Update registry
	if err := d.registry.SetLastIndexed(projectPath); err != nil {
		d.logger.Error("failed to update registry", "error", err)
	}
}

// AddProject adds a project to the watch list
func (d *Daemon) AddProject(projectPath string) error {
	if err := d.registry.Add(projectPath); err != nil {
		return err
	}
	return d.watchProject(projectPath)
}

// RemoveProject removes a project from the watch list
func (d *Daemon) RemoveProject(projectPath string) error {
	if err := d.unwatchProject(projectPath); err != nil {
		return err
	}
	return d.registry.Remove(projectPath)
}

// TriggerReindex queues a project for immediate reindexing
func (d *Daemon) TriggerReindex(projectPath string) error {
	select {
	case d.indexQueue <- projectPath:
		return nil
	default:
		return fmt.Errorf("index queue full")
	}
}

// writePIDFile writes the daemon PID to a file
func (d *Daemon) writePIDFile(path string) error {
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

// isIgnoredDir returns true if the directory should be skipped
func isIgnoredDir(name string) bool {
	ignored := map[string]bool{
		// Version control
		".git": true,
		".svn": true,
		".hg":  true,

		// IDE/Editor
		".idea":   true,
		".vscode": true,

		// Build outputs
		"dist":   true,
		"build":  true,
		"target": true,
		"out":    true,

		// Dependencies
		"node_modules": true,
		"vendor":       true,
		".bundle":      true,
		"Pods":         true,

		// Python
		"__pycache__": true,
		".venv":       true,
		"venv":        true,
		"env":         true,
		".tox":        true,
		".pytest_cache": true,

		// Ruby/Rails
		"tmp":      true,
		"log":      true,
		"coverage": true,
		"sorbet":   true,

		// Generated/Cache
		".cache":       true,
		".codetect": true,
		".next":        true,
		".nuxt":        true,
		".turbo":       true,
		".parcel-cache": true,

		// Assets (often generated)
		"public/assets": true,
		"public/packs":  true,
	}
	return ignored[name]
}

// isCodeFile returns true if the file is a code file
func isCodeFile(path string) bool {
	ext := filepath.Ext(path)
	codeExts := map[string]bool{
		".go":    true,
		".js":    true,
		".ts":    true,
		".tsx":   true,
		".jsx":   true,
		".py":    true,
		".java":  true,
		".c":     true,
		".cpp":   true,
		".h":     true,
		".hpp":   true,
		".rs":    true,
		".rb":    true,
		".php":   true,
		".swift": true,
		".kt":    true,
		".scala": true,
		".cs":    true,
		".md":    true,
		".json":  true,
		".yaml":  true,
		".yml":   true,
		".toml":  true,
	}
	return codeExts[ext]
}

// isSubpath returns true if child is under parent
func isSubpath(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return len(rel) > 0 && rel[0] != '.'
}

// loadGitignore loads gitignore patterns from local .gitignore and global ~/.gitignore
func loadGitignore(rootPath string) *ignore.GitIgnore {
	var patterns []string

	// Load global gitignore (~/.gitignore)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		globalGitignore := filepath.Join(homeDir, ".gitignore")
		if content, err := os.ReadFile(globalGitignore); err == nil {
			for _, line := range splitLines(string(content)) {
				if line != "" && !isComment(line) {
					patterns = append(patterns, line)
				}
			}
		}
	}

	// Load local .gitignore
	localGitignore := filepath.Join(rootPath, ".gitignore")
	if content, err := os.ReadFile(localGitignore); err == nil {
		for _, line := range splitLines(string(content)) {
			if line != "" && !isComment(line) {
				patterns = append(patterns, line)
			}
		}
	}

	if len(patterns) == 0 {
		return nil
	}

	return ignore.CompileIgnoreLines(patterns...)
}

// splitLines splits content into lines
func splitLines(content string) []string {
	return strings.Split(content, "\n")
}

// isComment returns true if line is a gitignore comment
func isComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	return len(trimmed) > 0 && trimmed[0] == '#'
}
