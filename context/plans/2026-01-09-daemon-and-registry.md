# Plan: Background Indexing Daemon & Central Index Registry

**Status: COMPLETE** - PR #4: https://github.com/brian-lai/codetect/pull/4

## Objective

Add two features to improve codetect performance and usability across multiple projects:

1. **Background Indexing Daemon** - Watches for file changes and re-indexes automatically [DONE]
2. **Central Index Registry** - Tracks all indexed projects and enables shared configuration [DONE]

## User Workflow (After Implementation)

```bash
# Start daemon (watches all registered projects)
codetect daemon start

# In any project
cd /path/to/my-project
codetect init              # Registers project in central registry
codetect index             # Initial index (daemon watches after this)

# Daemon automatically re-indexes on file changes
# No manual re-indexing needed!

# Check status
codetect daemon status     # Show watched projects, last index times
codetect registry list     # List all registered projects
```

---

## Part 1: Background Indexing Daemon

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    codetect daemon                        │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │   fsnotify   │  │   Debouncer  │  │   Indexer    │       │
│  │   Watcher    │─▶│   (500ms)    │─▶│   Worker     │       │
│  └──────────────┘  └──────────────┘  └──────────────┘       │
│         │                                    │               │
│         │ watches                            │ calls         │
│         ▼                                    ▼               │
│  ┌──────────────┐                   ┌──────────────┐        │
│  │  Registered  │                   │ codetect  │        │
│  │   Projects   │                   │    index     │        │
│  └──────────────┘                   └──────────────┘        │
│                                                              │
│  ┌──────────────────────────────────────────────────┐       │
│  │              Unix Socket IPC                      │       │
│  │   /tmp/codetect-<uid>.sock                    │       │
│  │   Commands: status, stop, reindex, add, remove   │       │
│  └──────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────┘
```

### New Commands

| Command | Description |
|---------|-------------|
| `codetect daemon start` | Start background daemon |
| `codetect daemon stop` | Stop daemon gracefully |
| `codetect daemon status` | Show daemon status and watched projects |
| `codetect daemon logs` | Tail daemon log file |

### Implementation

#### 1.1 Create Daemon Package

**File:** `internal/daemon/daemon.go`

```go
type Daemon struct {
    registry    *Registry
    watcher     *fsnotify.Watcher
    indexQueue  chan string        // Projects needing reindex
    debounceMap map[string]*time.Timer
    socket      net.Listener
    ctx         context.Context
    cancel      context.CancelFunc
}

func (d *Daemon) Start() error
func (d *Daemon) Stop() error
func (d *Daemon) AddProject(path string) error
func (d *Daemon) RemoveProject(path string) error
func (d *Daemon) Status() *DaemonStatus
```

#### 1.2 File Watching with Debouncing

**File:** `internal/daemon/watcher.go`

- Use `fsnotify` for cross-platform file watching
- Watch directories (not files) for reliable atomic update handling
- Skip ignored directories: `.git`, `node_modules`, `vendor`, `.codetect`
- Debounce changes: Wait 500ms after last change before triggering reindex
- Only watch code files (reuse `isCodeFile()` from symbols package)

```go
func (d *Daemon) watchProject(projectPath string) {
    // Walk and add all directories
    filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, err error) error {
        if entry.IsDir() && !isIgnored(path) {
            d.watcher.Add(path)
        }
        return nil
    })
}

func (d *Daemon) handleEvent(event fsnotify.Event) {
    project := d.findProjectForPath(event.Name)

    // Debounce: reset timer on each event
    if timer, ok := d.debounceMap[project]; ok {
        timer.Stop()
    }
    d.debounceMap[project] = time.AfterFunc(500*time.Millisecond, func() {
        d.indexQueue <- project
    })
}
```

#### 1.3 Unix Socket IPC

**File:** `internal/daemon/ipc.go`

- Socket location: `/tmp/codetect-<uid>.sock` (user-specific)
- JSON-based command/response protocol
- Commands: `status`, `stop`, `reindex <path>`, `add <path>`, `remove <path>`

```go
type Command struct {
    Action string `json:"action"`
    Path   string `json:"path,omitempty"`
}

type Response struct {
    Status  string `json:"status"` // "ok" or "error"
    Message string `json:"message,omitempty"`
    Data    any    `json:"data,omitempty"`
}
```

#### 1.4 Daemon Lifecycle

**File:** `internal/daemon/lifecycle.go`

- PID file at `~/.config/codetect/daemon.pid`
- Log file at `~/.config/codetect/daemon.log`
- Graceful shutdown on SIGTERM/SIGINT
- Auto-cleanup stale socket on startup

```go
func (d *Daemon) Run() error {
    // Write PID file
    // Setup signal handlers
    // Start watcher goroutine
    // Start IPC server goroutine
    // Start index worker goroutine
    // Block until shutdown
}
```

#### 1.5 Update Wrapper Script

**File:** `scripts/codetect-wrapper.sh`

Add daemon commands:

```bash
cmd_daemon() {
    local subcmd="${1:-status}"
    case "$subcmd" in
        start)  daemon_start ;;
        stop)   daemon_stop ;;
        status) daemon_status ;;
        logs)   daemon_logs ;;
        *)      error "Unknown daemon command: $subcmd" ;;
    esac
}

daemon_start() {
    # Check if already running
    # Start daemon in background
    # Wait for socket to be ready
}

daemon_stop() {
    # Send stop command via socket
    # Or send SIGTERM to PID
}
```

---

## Part 2: Central Index Registry

### Architecture

```
~/.config/codetect/
├── config.env              # Global embedding config (existing)
├── registry.json           # Project registry
├── daemon.pid              # Daemon PID file
└── daemon.log              # Daemon log file
```

### Registry Schema

**File:** `~/.config/codetect/registry.json`

```json
{
  "version": 1,
  "projects": [
    {
      "path": "/Users/brian/dev/backend-service",
      "name": "backend-service",
      "added_at": "2026-01-09T10:00:00Z",
      "last_indexed": "2026-01-09T11:30:00Z",
      "index_stats": {
        "symbols": 1205,
        "embeddings": 651,
        "db_size_bytes": 2048576
      },
      "watch_enabled": true
    }
  ],
  "settings": {
    "auto_watch": true,
    "debounce_ms": 500,
    "max_projects": 50
  }
}
```

### Implementation

#### 2.1 Create Registry Package

**File:** `internal/registry/registry.go`

```go
type Registry struct {
    path     string
    data     *RegistryData
    mu       sync.RWMutex
}

type RegistryData struct {
    Version  int        `json:"version"`
    Projects []Project  `json:"projects"`
    Settings Settings   `json:"settings"`
}

type Project struct {
    Path        string     `json:"path"`
    Name        string     `json:"name"`
    AddedAt     time.Time  `json:"added_at"`
    LastIndexed time.Time  `json:"last_indexed"`
    IndexStats  IndexStats `json:"index_stats"`
    WatchEnabled bool      `json:"watch_enabled"`
}

func NewRegistry() (*Registry, error)
func (r *Registry) Add(projectPath string) error
func (r *Registry) Remove(projectPath string) error
func (r *Registry) List() []Project
func (r *Registry) UpdateStats(projectPath string, stats IndexStats) error
func (r *Registry) SetLastIndexed(projectPath string) error
```

#### 2.2 New Commands

| Command | Description |
|---------|-------------|
| `codetect registry list` | List all registered projects |
| `codetect registry add <path>` | Add project to registry |
| `codetect registry remove <path>` | Remove project from registry |
| `codetect registry stats` | Show aggregate stats across all projects |

#### 2.3 Update `codetect init`

Modify `cmd_init()` in wrapper to:
1. Create `.mcp.json` (existing)
2. Register project in central registry (new)
3. Enable watch if daemon is running (new)

#### 2.4 Update Indexer to Report Stats

**File:** `cmd/codetect-index/main.go`

After indexing completes:
1. Update registry with last_indexed timestamp
2. Update registry with index_stats (symbol count, embedding count, db size)

---

## Files to Create/Modify

| File | Action | Status |
|------|--------|--------|
| `internal/daemon/daemon.go` | CREATE | DONE |
| `internal/daemon/ipc.go` | CREATE | DONE |
| `internal/registry/registry.go` | CREATE | DONE |
| `cmd/codetect-daemon/main.go` | CREATE | DONE |
| `scripts/codetect-wrapper.sh` | MODIFY | DONE |
| `Makefile` | MODIFY | DONE |
| `go.mod` | MODIFY | DONE |

---

## Dependencies

```bash
go get github.com/fsnotify/fsnotify
```

---

## Verification

### Test Daemon

```bash
# Start daemon
codetect daemon start
# Expected: "Daemon started (PID: XXXXX)"

# Check status
codetect daemon status
# Expected: Shows running status, watched projects

# Make a file change in a watched project
echo "// test" >> /path/to/project/main.go
# Expected: Daemon logs show reindex triggered after 500ms

# Stop daemon
codetect daemon stop
# Expected: "Daemon stopped"
```

### Test Registry

```bash
# List projects
codetect registry list
# Expected: Shows all registered projects with stats

# Add project
cd /path/to/new-project
codetect init
codetect registry list
# Expected: New project appears in list

# Remove project
codetect registry remove /path/to/old-project
codetect registry list
# Expected: Project no longer in list
```

---

## Future Enhancements (Not in Scope)

- Shared embedding cache across projects (dedup identical code)
- Git hook integration for auto-indexing on commit
- Web UI for daemon status
- Remote daemon mode (index on a different machine)
