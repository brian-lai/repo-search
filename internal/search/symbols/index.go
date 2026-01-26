package symbols

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codetect/internal/config"
	"codetect/internal/db"
)

// Index is the database-backed symbol index.
// Uses the adapter pattern to support multiple database backends (SQLite, PostgreSQL).
// All database operations go through the adapter interface for database portability.
type Index struct {
	sqlDB      *sql.DB           // Raw SQL connection (deprecated, for legacy compatibility only)
	adapter    db.DB             // Adapter interface - use this for all database operations
	dialect    db.Dialect        // SQL dialect for database-specific syntax (placeholders, etc.)
	dbPath     string
	root       string
	indexCfg   config.IndexConfig // Indexing backend configuration
}

// NewIndex creates or opens a symbol index at the given path.
// Uses SQLite by default. Sets repoRoot to current working directory.
//
// Deprecated: Use NewIndexWithConfig for new code. This constructor is
// maintained for backward compatibility with existing SQLite-based workflows.
func NewIndex(dbPath string) (*Index, error) {
	sqlDB, err := OpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	// Default to current working directory for repo root
	cwd, _ := os.Getwd()

	return &Index{
		sqlDB:    sqlDB,
		adapter:  db.WrapSQL(sqlDB),
		dialect:  db.GetDialect(db.DatabaseSQLite),
		dbPath:   dbPath,
		root:     cwd,
		indexCfg: config.LoadIndexConfigFromEnv(),
	}, nil
}

// NewIndexWithConfig creates a symbol index using the provided configuration.
// repoRoot is the absolute path to the repository root, used for multi-repo isolation.
// This is the preferred constructor for new code as it supports multiple
// database backends (SQLite, PostgreSQL) through the adapter pattern.
//
// All Index methods use the adapter interface and dialect-aware SQL,
// enabling seamless database backend switching without code changes.
func NewIndexWithConfig(cfg db.Config, repoRoot string) (*Index, error) {
	database, err := db.Open(cfg)
	if err != nil {
		return nil, err
	}

	dialect := cfg.Dialect()

	// Initialize schema using dialect-aware DDL
	if err := initSchemaWithAdapter(database, dialect); err != nil {
		database.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return &Index{
		adapter:  database,
		dialect:  dialect,
		dbPath:   cfg.Path,
		root:     repoRoot,
		indexCfg: config.LoadIndexConfigFromEnv(),
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

// FindSymbol searches for symbols by name (supports LIKE patterns) within this repo
func (idx *Index) FindSymbol(name string, kind string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any

	// Use LIKE for partial matching
	pattern := "%" + name + "%"

	// Build query with dialect-aware placeholders, filtering by repo_root
	if kind != "" {
		query = fmt.Sprintf(`SELECT name, kind, path, line, language, pattern, scope
				 FROM symbols
				 WHERE repo_root = %s AND name LIKE %s AND kind = %s
				 ORDER BY
					CASE WHEN name = %s THEN 0
						 WHEN name LIKE %s THEN 1
						 ELSE 2 END,
					name
				 LIMIT %s`,
			idx.dialect.Placeholder(1),
			idx.dialect.Placeholder(2),
			idx.dialect.Placeholder(3),
			idx.dialect.Placeholder(4),
			idx.dialect.Placeholder(5),
			idx.dialect.Placeholder(6))
		args = []any{idx.root, pattern, kind, name, name + "%", limit}
	} else {
		query = fmt.Sprintf(`SELECT name, kind, path, line, language, pattern, scope
				 FROM symbols
				 WHERE repo_root = %s AND name LIKE %s
				 ORDER BY
					CASE WHEN name = %s THEN 0
						 WHEN name LIKE %s THEN 1
						 ELSE 2 END,
					name
				 LIMIT %s`,
			idx.dialect.Placeholder(1),
			idx.dialect.Placeholder(2),
			idx.dialect.Placeholder(3),
			idx.dialect.Placeholder(4),
			idx.dialect.Placeholder(5))
		args = []any{idx.root, pattern, name, name + "%", limit}
	}

	rows, err := idx.adapter.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying symbols: %w", err)
	}
	defer rows.Close()

	var symbols []Symbol
	for rows.Next() {
		var s Symbol
		var language, patternStr, scope sql.NullString
		if err := rows.Scan(&s.Name, &s.Kind, &s.Path, &s.Line, &language, &patternStr, &scope); err != nil {
			return nil, fmt.Errorf("scanning symbol: %w", err)
		}
		s.Language = language.String
		s.Pattern = patternStr.String
		s.Scope = scope.String
		symbols = append(symbols, s)
	}

	return symbols, rows.Err()
}

// ListDefsInFile returns all symbol definitions in a file within this repo
func (idx *Index) ListDefsInFile(path string) ([]Symbol, error) {
	query := fmt.Sprintf(`SELECT name, kind, path, line, language, pattern, scope
			  FROM symbols
			  WHERE repo_root = %s AND path = %s
			  ORDER BY line`, idx.dialect.Placeholder(1), idx.dialect.Placeholder(2))

	rows, err := idx.adapter.Query(query, idx.root, path)
	if err != nil {
		return nil, fmt.Errorf("querying symbols: %w", err)
	}
	defer rows.Close()

	var symbols []Symbol
	for rows.Next() {
		var s Symbol
		var language, patternStr, scope sql.NullString
		if err := rows.Scan(&s.Name, &s.Kind, &s.Path, &s.Line, &language, &patternStr, &scope); err != nil {
			return nil, fmt.Errorf("scanning symbol: %w", err)
		}
		s.Language = language.String
		s.Pattern = patternStr.String
		s.Scope = scope.String
		symbols = append(symbols, s)
	}

	return symbols, rows.Err()
}

// Update re-indexes files that have changed since last index
func (idx *Index) Update(root string) error {
	idx.root = root

	// Get list of files that need reindexing
	filesToIndex, err := idx.getFilesToIndex(root)
	if err != nil {
		return fmt.Errorf("scanning files: %w", err)
	}

	if len(filesToIndex) == 0 {
		return nil // Nothing to do
	}

	// Collect all symbols based on configured backend
	var allSymbols []Symbol

	// Decide which indexer(s) to use based on configuration
	useAstGrep := idx.indexCfg.UseAstGrep() && AstGrepAvailable()
	useCtags := idx.indexCfg.UseCtags() && CtagsAvailable()

	// If ast-grep is required but not available, error
	if idx.indexCfg.RequireAstGrep() && !AstGrepAvailable() {
		return fmt.Errorf("ast-grep backend required but not available")
	}

	var unsupportedFiles []string

	// Try ast-grep for supported languages (if configured)
	if useAstGrep {
		filesByLang := make(map[string][]string)

		for path := range filesToIndex {
			lang := LanguageFromExtension(path)
			if lang != "" {
				filesByLang[lang] = append(filesByLang[lang], filepath.Join(root, path))
			} else {
				unsupportedFiles = append(unsupportedFiles, path)
			}
		}

		// Run ast-grep for each language
		for lang, files := range filesByLang {
			symbols, err := RunAstGrep(root, files, lang)
			if err != nil {
				// If ast-grep fails and ctags is allowed, fall back
				if useCtags {
					unsupportedFiles = append(unsupportedFiles, files...)
					continue
				}
				return fmt.Errorf("ast-grep failed for %s: %w", lang, err)
			}
			allSymbols = append(allSymbols, symbols...)
		}
	} else {
		// Not using ast-grep, mark all files as unsupported
		for path := range filesToIndex {
			unsupportedFiles = append(unsupportedFiles, path)
		}
	}

	// Run ctags for unsupported files (if configured and available)
	if len(unsupportedFiles) > 0 && useCtags {
		// Convert relative paths to absolute for ctags
		var absUnsupportedFiles []string
		for _, path := range unsupportedFiles {
			if filepath.IsAbs(path) {
				absUnsupportedFiles = append(absUnsupportedFiles, path)
			} else {
				absUnsupportedFiles = append(absUnsupportedFiles, filepath.Join(root, path))
			}
		}

		entries, err := RunCtags(root, absUnsupportedFiles)
		if err != nil {
			// Only error if both indexers failed and we have no symbols
			if len(allSymbols) == 0 {
				return fmt.Errorf("running ctags: %w", err)
			}
		} else {
			for _, entry := range entries {
				allSymbols = append(allSymbols, entry.ToSymbol())
			}
		}
	}

	// Begin transaction for bulk insert
	tx, err := idx.adapter.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing symbols for files being reindexed within this repo
	deleteQuery := fmt.Sprintf("DELETE FROM symbols WHERE repo_root = %s AND path = %s",
		idx.dialect.Placeholder(1), idx.dialect.Placeholder(2))
	for path := range filesToIndex {
		if _, err := tx.Exec(deleteQuery, idx.root, path); err != nil {
			return fmt.Errorf("clearing symbols for %s: %w", path, err)
		}
	}

	// Batch insert symbols (500 at a time for performance)
	if err := idx.batchInsertSymbols(tx, allSymbols, 500); err != nil {
		return fmt.Errorf("inserting symbols: %w", err)
	}

	// Build dialect-aware upsert statement for files with repo_root
	fileUpsertSQL := idx.dialect.UpsertSQL(
		"files",
		[]string{"repo_root", "path", "mtime", "size", "indexed_at"},
		[]string{"repo_root", "path"},
		[]string{"mtime", "size", "indexed_at"},
	)
	fileStmt, err := tx.Prepare(fileUpsertSQL)
	if err != nil {
		return fmt.Errorf("preparing file insert: %w", err)
	}
	defer fileStmt.Close()

	// Update file tracking with repo_root
	now := time.Now().Unix()
	for path, info := range filesToIndex {
		if _, err := fileStmt.Exec(idx.root, path, info.mtime, info.size, now); err != nil {
			return fmt.Errorf("updating file record for %s: %w", path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// batchInsertSymbols inserts symbols in batches to reduce DB round-trips
func (idx *Index) batchInsertSymbols(tx db.Tx, symbols []Symbol, batchSize int) error {
	if len(symbols) == 0 {
		return nil
	}

	// Build dialect-aware upsert statement
	symbolUpsertSQL := idx.dialect.UpsertSQL(
		"symbols",
		[]string{"repo_root", "name", "kind", "path", "line", "language", "pattern", "scope", "signature"},
		[]string{"repo_root", "name", "path", "line"},
		[]string{"kind", "language", "pattern", "scope", "signature"},
	)
	stmt, err := tx.Prepare(symbolUpsertSQL)
	if err != nil {
		return fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	// Insert in batches
	for i := 0; i < len(symbols); i += batchSize {
		end := i + batchSize
		if end > len(symbols) {
			end = len(symbols)
		}

		for _, sym := range symbols[i:end] {
			_, err := stmt.Exec(
				idx.root, sym.Name, sym.Kind, sym.Path, sym.Line,
				nullString(sym.Language), nullString(sym.Pattern),
				nullString(sym.Scope), nullString(""), // signature empty for now
			)
			if err != nil {
				// Log but continue on duplicate/constraint errors
				continue
			}
		}
	}

	return nil
}

// FullReindex clears all data for this repo and reindexes from scratch
func (idx *Index) FullReindex(root string) error {
	// Set root for scoped operations
	idx.root = root

	// Clear all existing data for this repo using the adapter
	deleteSymbolsQuery := fmt.Sprintf("DELETE FROM symbols WHERE repo_root = %s", idx.dialect.Placeholder(1))
	if _, err := idx.adapter.Exec(deleteSymbolsQuery, idx.root); err != nil {
		return fmt.Errorf("clearing symbols: %w", err)
	}
	deleteFilesQuery := fmt.Sprintf("DELETE FROM files WHERE repo_root = %s", idx.dialect.Placeholder(1))
	if _, err := idx.adapter.Exec(deleteFilesQuery, idx.root); err != nil {
		return fmt.Errorf("clearing files: %w", err)
	}

	return idx.Update(root)
}

type fileInfo struct {
	mtime int64
	size  int64
}

// getFilesToIndex returns files that need reindexing (new or modified) within this repo
func (idx *Index) getFilesToIndex(root string) (map[string]fileInfo, error) {
	// Get currently indexed files for this repo using the adapter
	indexed := make(map[string]fileInfo)
	query := fmt.Sprintf("SELECT path, mtime, size FROM files WHERE repo_root = %s", idx.dialect.Placeholder(1))
	rows, err := idx.adapter.Query(query, idx.root)
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

// Stats returns statistics about the index for this repo
func (idx *Index) Stats() (symbolCount int, fileCount int, err error) {
	// Use the adapter for database-agnostic queries, scoped by repo_root
	symbolQuery := fmt.Sprintf("SELECT COUNT(*) FROM symbols WHERE repo_root = %s", idx.dialect.Placeholder(1))
	if err := idx.adapter.QueryRow(symbolQuery, idx.root).Scan(&symbolCount); err != nil {
		return 0, 0, err
	}
	fileQuery := fmt.Sprintf("SELECT COUNT(*) FROM files WHERE repo_root = %s", idx.dialect.Placeholder(1))
	if err := idx.adapter.QueryRow(fileQuery, idx.root).Scan(&fileCount); err != nil {
		return 0, 0, err
	}
	return symbolCount, fileCount, nil
}
