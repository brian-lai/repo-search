package symbols

import (
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

// Index is the SQLite-backed symbol index
type Index struct {
	db     *sql.DB
	dbPath string
	root   string
}

// NewIndex creates or opens a symbol index at the given path
func NewIndex(dbPath string) (*Index, error) {
	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	return &Index{
		db:     db,
		dbPath: dbPath,
	}, nil
}

// Close closes the index database
func (idx *Index) Close() error {
	if idx.db != nil {
		return idx.db.Close()
	}
	return nil
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

	rows, err := idx.db.Query(query, args...)
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

	rows, err := idx.db.Query(query, path)
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
	tx, err := idx.db.Begin()
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
	if err := ClearAllSymbols(idx.db); err != nil {
		return fmt.Errorf("clearing symbols: %w", err)
	}
	if _, err := idx.db.Exec("DELETE FROM files"); err != nil {
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
	rows, err := idx.db.Query("SELECT path, mtime, size FROM files")
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
	if err := idx.db.QueryRow("SELECT COUNT(*) FROM symbols").Scan(&symbolCount); err != nil {
		return 0, 0, err
	}
	if err := idx.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount); err != nil {
		return 0, 0, err
	}
	return symbolCount, fileCount, nil
}
