// Package registry manages the central project registry for codetect.
// It tracks all indexed projects, their statistics, and watch status.
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	// RegistryVersion is the current schema version
	RegistryVersion = 1
	// DefaultDebounceMs is the default debounce time for file watching
	DefaultDebounceMs = 500
	// DefaultMaxProjects is the default maximum number of projects
	DefaultMaxProjects = 50
)

// IndexStats holds statistics about a project's index
type IndexStats struct {
	Symbols     int   `json:"symbols"`
	Embeddings  int   `json:"embeddings"`
	DBSizeBytes int64 `json:"db_size_bytes"`
}

// Project represents a registered project in the registry
type Project struct {
	Path         string     `json:"path"`
	Name         string     `json:"name"`
	AddedAt      time.Time  `json:"added_at"`
	LastIndexed  *time.Time `json:"last_indexed,omitempty"`
	IndexStats   IndexStats `json:"index_stats"`
	WatchEnabled bool       `json:"watch_enabled"`
}

// Settings holds global registry settings
type Settings struct {
	AutoWatch   bool `json:"auto_watch"`
	DebounceMs  int  `json:"debounce_ms"`
	MaxProjects int  `json:"max_projects"`
}

// RegistryData is the top-level structure stored in registry.json
type RegistryData struct {
	Version  int       `json:"version"`
	Projects []Project `json:"projects"`
	Settings Settings  `json:"settings"`
}

// Registry manages the central project registry
type Registry struct {
	path string
	data *RegistryData
	mu   sync.RWMutex
}

// DefaultConfigDir returns the default config directory path
func DefaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "codetect")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "codetect")
}

// DefaultRegistryPath returns the default registry file path
func DefaultRegistryPath() string {
	return filepath.Join(DefaultConfigDir(), "registry.json")
}

// NewRegistry creates or loads the registry from the default location
func NewRegistry() (*Registry, error) {
	return NewRegistryAt(DefaultRegistryPath())
}

// NewRegistryAt creates or loads the registry from a specific path
func NewRegistryAt(path string) (*Registry, error) {
	r := &Registry{
		path: path,
		data: &RegistryData{
			Version:  RegistryVersion,
			Projects: []Project{},
			Settings: Settings{
				AutoWatch:   true,
				DebounceMs:  DefaultDebounceMs,
				MaxProjects: DefaultMaxProjects,
			},
		},
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Load existing registry if it exists
	if _, err := os.Stat(path); err == nil {
		if err := r.load(); err != nil {
			return nil, fmt.Errorf("failed to load registry: %w", err)
		}
	}

	return r, nil
}

// load reads the registry from disk
func (r *Registry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, r.data)
}

// save writes the registry to disk
func (r *Registry) save() error {
	data, err := json.MarshalIndent(r.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0644)
}

// Add registers a new project in the registry
func (r *Registry) Add(projectPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Normalize path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if already registered
	for _, p := range r.data.Projects {
		if p.Path == absPath {
			return nil // Already registered
		}
	}

	// Check max projects
	if len(r.data.Projects) >= r.data.Settings.MaxProjects {
		return fmt.Errorf("maximum number of projects (%d) reached", r.data.Settings.MaxProjects)
	}

	// Add new project
	project := Project{
		Path:         absPath,
		Name:         filepath.Base(absPath),
		AddedAt:      time.Now(),
		WatchEnabled: r.data.Settings.AutoWatch,
	}
	r.data.Projects = append(r.data.Projects, project)

	return r.save()
}

// Remove unregisters a project from the registry
func (r *Registry) Remove(projectPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Find and remove project
	for i, p := range r.data.Projects {
		if p.Path == absPath {
			r.data.Projects = slices.Delete(r.data.Projects, i, i+1)
			return r.save()
		}
	}

	return fmt.Errorf("project not found: %s", projectPath)
}

// List returns all registered projects
func (r *Registry) List() []Project {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]Project, len(r.data.Projects))
	copy(result, r.data.Projects)
	return result
}

// Get returns a specific project by path
func (r *Registry) Get(projectPath string) (*Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	for _, p := range r.data.Projects {
		if p.Path == absPath {
			// Return a copy
			proj := p
			return &proj, nil
		}
	}

	return nil, fmt.Errorf("project not found: %s", projectPath)
}

// UpdateStats updates the index statistics for a project
func (r *Registry) UpdateStats(projectPath string, stats IndexStats) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	for i, p := range r.data.Projects {
		if p.Path == absPath {
			r.data.Projects[i].IndexStats = stats
			return r.save()
		}
	}

	return fmt.Errorf("project not found: %s", projectPath)
}

// SetLastIndexed updates the last indexed timestamp for a project
func (r *Registry) SetLastIndexed(projectPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	now := time.Now()
	for i, p := range r.data.Projects {
		if p.Path == absPath {
			r.data.Projects[i].LastIndexed = &now
			return r.save()
		}
	}

	return fmt.Errorf("project not found: %s", projectPath)
}

// SetWatchEnabled enables or disables watching for a project
func (r *Registry) SetWatchEnabled(projectPath string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	for i, p := range r.data.Projects {
		if p.Path == absPath {
			r.data.Projects[i].WatchEnabled = enabled
			return r.save()
		}
	}

	return fmt.Errorf("project not found: %s", projectPath)
}

// GetWatchedProjects returns all projects with watching enabled
func (r *Registry) GetWatchedProjects() []Project {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Project
	for _, p := range r.data.Projects {
		if p.WatchEnabled {
			result = append(result, p)
		}
	}
	return result
}

// Settings returns the current registry settings
func (r *Registry) Settings() Settings {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.data.Settings
}

// AggregateStats returns combined statistics across all projects
func (r *Registry) AggregateStats() IndexStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var total IndexStats
	for _, p := range r.data.Projects {
		total.Symbols += p.IndexStats.Symbols
		total.Embeddings += p.IndexStats.Embeddings
		total.DBSizeBytes += p.IndexStats.DBSizeBytes
	}
	return total
}

// Path returns the registry file path
func (r *Registry) Path() string {
	return r.path
}
