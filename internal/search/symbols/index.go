package symbols

import (
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"codetect/internal/db"
)

// Index is the database-backed symbol index.
// Currently uses SQLite but the adapter layer allows future migration to other databases.
type Index struct {
	sqlDB   *sql.DB     // Raw SQL connection (for legacy compatibility)
	adapter db.DB       // Adapter interface for new code
	dialect db.Dialect  // SQL dialect for database-specific syntax
	dbPath  string
	root    string
}

// NewIndex creates or opens a symbol index at the given path.
// Uses SQLite by default.
func NewIndex(dbPath string) (*Index, error) {
	sqlDB, err := OpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	return &Index{
		sqlDB:   sqlDB,
		adapter: db.WrapSQL(sqlDB),
		dialect: db.GetDialect(db.DatabaseSQLite),
		dbPath:  dbPath,
	}, nil
}

// NewIndexWithConfig creates a symbol index using the provided configuration.
// This allows using different database types in the future.
func NewIndexWithConfig(cfg db.Config) (*Index, error) {
	database, err := db.Open(cfg)
	if err != nil {
		return nil, err
	}

	return &Index{
		adapter: database,
		dialect: cfg.Dialect(),
		dbPath:  cfg.Path,
	}, nil
}

// Close closes the index database
func (idx *Index) Close() error {
	if idx.adapter != nil {
		return idx.adapter.Close()
	}
	return nil
}

// DB returns the underlying database connection.
// Deprecated: Use DBAdapter() for new code to get the db.DB interface.
// Returns nil if the index was created with NewIndexWithConfig (non-SQLite).
func (idx *Index) DB() *sql.DB {
	return idx.sqlDB
}

// DBAdapter returns the database adapter interface.
// Use this for interoperability with packages that use the adapter interface.
func (idx *Index) DBAdapter() db.DB {
	return idx.adapter
}

// Dialect returns the SQL dialect used by this index.
func (idx *Index) Dialect() db.Dialect {
	return idx.dialect
}

// FindSymbol searches for symbols by name (supports LIKE patterns)
func (idx *Index) FindSymbol(name string, kind string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any

	// Use LIKE for partial matching
	pattern := "%" + name + "%"

	if kind != "" {
		query = `SELECT name, kind, path, line, language, pattern, scope
				 FROM symbols
				 WHERE name LIKE ? AND kind = ?
				 ORDER BY
					CASE WHEN name = ? THEN 0
						 WHEN name LIKE ? THEN 1
						 ELSE 2 END,
					name
				 LIMIT ?`
		args = []any{pattern, kind, name, name + "%", limit}
	} else {
		query = `SELECT name, kind, path, line, language, pattern, scope
				 FROM symbols
				 WHERE name LIKE ?
				 ORDER BY
					CASE WHEN name = ? THEN 0
						 WHEN name LIKE ? THEN 1
						 ELSE 2 END,
					name
				 LIMIT ?`
		args = []any{pattern, name, name + "%", limit}
	}

	rows, err := idx.sqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying symbols: %w", err)
	}
	defer rows.Close()

	var symbols []Symbol
	for rows.Next() {
		var s Symbol
		var language, pattern, scope sql.NullString
		if err := rows.Scan(&s.Name, &s.Kind, &s.Path, &s.Line, &language, &pattern, &scope); err != nil {
			return nil, fmt.Errorf("scanning symbol: %w", err)
		}
		s.Language = language.String
		s.Pattern = pattern.String
		s.Scope = scope.String
		symbols = append(symbols, s)
	}

	return symbols, rows.Err()
}

// ListDefsInFile returns all symbol definitions in a file
func (idx *Index) ListDefsInFile(path string) ([]Symbol, error) {
	query := `SELECT name, kind, path, line, language, pattern, scope
			  FROM symbols
			  WHERE path = ?
			  ORDER BY line`

	rows, err := idx.sqlDB.Query(query, path)
	if err != nil {
		return nil, fmt.Errorf("querying symbols: %w", err)
	}
	defer rows.Close()

	var symbols []Symbol
	for rows.Next() {
		var s Symbol
		var language, pattern, scope sql.NullString
		if err := rows.Scan(&s.Name, &s.Kind, &s.Path, &s.Line, &language, &pattern, &scope); err != nil {
			return nil, fmt.Errorf("scanning symbol: %w", err)
		}
		s.Language = language.String
		s.Pattern = pattern.String
		s.Scope = scope.String
		symbols = append(symbols, s)
	}

	return symbols, rows.Err()
}

// Update re-indexes files that have changed since last index
func (idx *Index) Update(root string) error {
	if !CtagsAvailable() {
		return fmt.Errorf("universal-ctags not available")
	}

	idx.root = root

	// Get list of files that need reindexing
	filesToIndex, err := idx.getFilesToIndex(root)
	if err != nil {
		return fmt.Errorf("scanning files: %w", err)
	}

	if len(filesToIndex) == 0 {
		return nil // Nothing to do
	}

	// Run ctags on all files
	entries, err := RunCtags(root, nil) // Recursive scan
	if err != nil {
		return fmt.Errorf("running ctags: %w", err)
	}

	// Begin transaction for bulk insert
	tx, err := idx.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing symbols for files being reindexed
	for path := range filesToIndex {
		if _, err := tx.Exec("DELETE FROM symbols WHERE path = ?", path); err != nil {
			return fmt.Errorf("clearing symbols for %s: %w", path, err)
		}
	}

	// Prepare insert statement
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO symbols
		(name, kind, path, line, language, pattern, scope, signature)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	// Insert new symbols
	for _, entry := range entries {
		sym := entry.ToSymbol()
		_, err := stmt.Exec(
			sym.Name, sym.Kind, sym.Path, sym.Line,
			nullString(sym.Language), nullString(sym.Pattern),
			nullString(sym.Scope), nullString(""), // signature empty for now
		)
		if err != nil {
			// Log but continue on duplicate/constraint errors
			continue
		}
	}

	// Update file tracking
	now := time.Now().Unix()
	fileStmt, err := tx.Prepare(`INSERT OR REPLACE INTO files (path, mtime, size, indexed_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing file insert: %w", err)
	}
	defer fileStmt.Close()

	for path, info := range filesToIndex {
		if _, err := fileStmt.Exec(path, info.mtime, info.size, now); err != nil {
			return fmt.Errorf("updating file record for %s: %w", path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// FullReindex clears all data and reindexes from scratch
func (idx *Index) FullReindex(root string) error {
	// Clear all existing data
	if err := ClearAllSymbols(idx.sqlDB); err != nil {
		return fmt.Errorf("clearing symbols: %w", err)
	}
	if _, err := idx.sqlDB.Exec("DELETE FROM files"); err != nil {
		return fmt.Errorf("clearing files: %w", err)
	}

	return idx.Update(root)
}

type fileInfo struct {
	mtime int64
	size  int64
}

// getFilesToIndex returns files that need reindexing (new or modified)
func (idx *Index) getFilesToIndex(root string) (map[string]fileInfo, error) {
	// Get currently indexed files
	indexed := make(map[string]fileInfo)
	rows, err := idx.sqlDB.Query("SELECT path, mtime, size FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		var info fileInfo
		if err := rows.Scan(&path, &info.mtime, &info.size); err != nil {
			return nil, err
		}
		indexed[path] = info
	}

	// Walk directory and find files needing indexing
	needsIndex := make(map[string]fileInfo)

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable files
		}

		// Skip hidden directories and common non-code directories
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || isIgnoredDir(name) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only index code files
		if !isCodeFile(path) {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		// Check if file needs indexing
		info, err := d.Info()
		if err != nil {
			return nil
		}

		current := fileInfo{
			mtime: info.ModTime().Unix(),
			size:  info.Size(),
		}

		if prev, exists := indexed[relPath]; !exists || prev.mtime != current.mtime || prev.size != current.size {
			needsIndex[relPath] = current
		}

		return nil
	})

	return needsIndex, err
}

// isIgnoredDir returns true for directories that should not be indexed
func isIgnoredDir(name string) bool {
	ignored := map[string]bool{
		"node_modules": true,
		"vendor":       true,
		"dist":         true,
		"build":        true,
		"target":       true,
		"__pycache__":  true,
		".git":         true,
		".svn":         true,
		".hg":          true,
	}
	return ignored[name]
}

// isCodeFile returns true for files that should be indexed
func isCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	codeExts := map[string]bool{
		".go":    true,
		".js":    true,
		".ts":    true,
		".tsx":   true,
		".jsx":   true,
		".py":    true,
		".rb":    true,
		".java":  true,
		".c":     true,
		".cpp":   true,
		".h":     true,
		".hpp":   true,
		".rs":    true,
		".swift": true,
		".kt":    true,
		".scala": true,
		".php":   true,
		".cs":    true,
		".sh":    true,
		".bash":  true,
		".zsh":   true,
		".sql":   true,
		".lua":   true,
		".vim":   true,
		".el":    true,
	}
	return codeExts[ext]
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// Stats returns statistics about the index
func (idx *Index) Stats() (symbolCount int, fileCount int, err error) {
	if err := idx.sqlDB.QueryRow("SELECT COUNT(*) FROM symbols").Scan(&symbolCount); err != nil {
		return 0, 0, err
	}
	if err := idx.sqlDB.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount); err != nil {
		return 0, 0, err
	}
	return symbolCount, fileCount, nil
}
