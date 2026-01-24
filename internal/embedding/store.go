package embedding

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codetect/internal/db"
)

// EmbeddingRecord represents a stored embedding
type EmbeddingRecord struct {
	ID          int64     `json:"id"`
	RepoRoot    string    `json:"repo_root,omitempty"` // Populated for cross-repo queries
	Path        string    `json:"path"`
	StartLine   int       `json:"start_line"`
	EndLine     int       `json:"end_line"`
	ContentHash string    `json:"content_hash"`
	Embedding   []float32 `json:"embedding"`
	Model       string    `json:"model"`
	CreatedAt   time.Time `json:"created_at"`
}

// EmbeddingStore manages embedding storage in the database.
// Supports multiple database types via the dialect abstraction.
// For PostgreSQL, uses dimension-grouped tables (embeddings_768, embeddings_1024, etc.)
// to support multiple embedding models with different dimensions across repositories.
type EmbeddingStore struct {
	db           db.DB
	dialect      db.Dialect
	schema       *db.SchemaBuilder
	vectorDim    int    // Vector dimensions (e.g., 768 for nomic-embed-text)
	useNativeVec bool   // True if using PostgreSQL native vector type
	repoRoot     string // Absolute path to repo root for multi-repo isolation
}

// tableNameForDimensions returns the table name for a given vector dimension.
// PostgreSQL uses dimension-grouped tables (embeddings_768, embeddings_1024, etc.)
// to support multiple embedding models with different dimensions.
// SQLite uses a single "embeddings" table since it stores vectors as JSON text.
func tableNameForDimensions(dialect db.Dialect, dim int) string {
	if dialect.Name() == "postgres" {
		return fmt.Sprintf("embeddings_%d", dim)
	}
	return "embeddings"
}

// tableName returns the table name for this store's vector dimensions.
func (s *EmbeddingStore) tableName() string {
	return tableNameForDimensions(s.dialect, s.vectorDim)
}

// embeddingColumnsForDialect returns the column definitions for the embeddings table
// based on the database dialect. PostgreSQL uses native vector type, SQLite uses TEXT.
func embeddingColumnsForDialect(dialect db.Dialect, vectorDim int) []db.ColumnDef {
	embeddingCol := db.ColumnDef{
		Name:     "embedding",
		Nullable: false,
	}

	// Use native vector type for PostgreSQL, TEXT for SQLite
	if dialect.Name() == "postgres" {
		embeddingCol.Type = db.ColTypeVector
		embeddingCol.VectorDimension = vectorDim
	} else {
		embeddingCol.Type = db.ColTypeText // JSON storage
	}

	return []db.ColumnDef{
		{Name: "id", Type: db.ColTypeAutoIncrement},
		{Name: "repo_root", Type: db.ColTypeText, Nullable: false},
		{Name: "path", Type: db.ColTypeText, Nullable: false},
		{Name: "start_line", Type: db.ColTypeInteger, Nullable: false},
		{Name: "end_line", Type: db.ColTypeInteger, Nullable: false},
		{Name: "content_hash", Type: db.ColTypeText, Nullable: false},
		embeddingCol,
		{Name: "model", Type: db.ColTypeText, Nullable: false},
		{Name: "created_at", Type: db.ColTypeInteger, Nullable: false},
	}
}

// NewEmbeddingStore creates a new embedding store using a db.DB adapter.
// Defaults to SQLite dialect for backward compatibility.
// repoRoot is the absolute path to the repository root for multi-repo isolation.
func NewEmbeddingStore(database db.DB, repoRoot string) (*EmbeddingStore, error) {
	return NewEmbeddingStoreWithDialect(database, db.GetDialect(db.DatabaseSQLite), repoRoot)
}

// NewEmbeddingStoreWithDialect creates an embedding store with a specific SQL dialect.
// Uses default vector dimensions (768) for PostgreSQL.
// repoRoot is the absolute path to the repository root for multi-repo isolation.
func NewEmbeddingStoreWithDialect(database db.DB, dialect db.Dialect, repoRoot string) (*EmbeddingStore, error) {
	return NewEmbeddingStoreWithOptions(database, dialect, 768, repoRoot)
}

// NewEmbeddingStoreWithOptions creates an embedding store with custom vector dimensions.
// repoRoot is the absolute path to the repository root for multi-repo isolation.
func NewEmbeddingStoreWithOptions(database db.DB, dialect db.Dialect, vectorDim int, repoRoot string) (*EmbeddingStore, error) {
	useNativeVec := dialect.Name() == "postgres"

	store := &EmbeddingStore{
		db:           database,
		dialect:      dialect,
		schema:       db.NewSchemaBuilder(database, dialect),
		vectorDim:    vectorDim,
		useNativeVec: useNativeVec,
		repoRoot:     repoRoot,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("initializing embedding schema: %w", err)
	}

	return store, nil
}

// NewEmbeddingStoreFromSQL creates an embedding store from a raw *sql.DB.
// This is for backward compatibility with existing code.
// Prefer NewEmbeddingStore with db.DB for new code.
// repoRoot is the absolute path to the repository root for multi-repo isolation.
func NewEmbeddingStoreFromSQL(sqlDB *sql.DB, repoRoot string) (*EmbeddingStore, error) {
	return NewEmbeddingStore(db.WrapSQL(sqlDB), repoRoot)
}

// NewEmbeddingStoreFromConfig creates an embedding store using a db.Config.
// repoRoot is the absolute path to the repository root for multi-repo isolation.
func NewEmbeddingStoreFromConfig(database db.DB, cfg db.Config, repoRoot string) (*EmbeddingStore, error) {
	return NewEmbeddingStoreWithDialect(database, cfg.Dialect(), repoRoot)
}

// initSchema creates the embeddings table if it doesn't exist.
// For PostgreSQL, creates dimension-specific tables (embeddings_768, embeddings_1024, etc.)
// and the repo_embedding_configs table for tracking model/dimensions per repository.
func (s *EmbeddingStore) initSchema() error {
	// Run dialect-specific initialization statements (e.g., CREATE EXTENSION for PostgreSQL)
	for _, stmt := range s.dialect.InitStatements() {
		if _, err := s.db.Exec(stmt); err != nil {
			// Log but don't fail if extension already exists
			// This allows for graceful handling of already-initialized databases
			_ = err // Suppress unused variable warning
		}
	}

	// Use dialect-aware schema for non-SQLite databases
	// For SQLite, we still use the raw SQL for now to maintain compatibility
	if s.dialect.Name() != "sqlite" {
		// Create repo_embedding_configs table first (tracks model/dimensions per repo)
		if err := s.initRepoConfigTable(); err != nil {
			return fmt.Errorf("creating repo config table: %w", err)
		}

		// Get dimension-specific table name
		tableName := s.tableName()

		// Get column definitions based on dialect
		columns := embeddingColumnsForDialect(s.dialect, s.vectorDim)

		// Create dimension-specific table using dialect
		sql := s.dialect.CreateTableSQL(tableName, columns)
		if _, err := s.db.Exec(sql); err != nil {
			return fmt.Errorf("creating %s table: %w", tableName, err)
		}

		// Add unique constraint via index (not all dialects support inline UNIQUE)
		// Note: Index names must be unique per database, so include dimension in name

		// Create unique constraint for upsert ON CONFLICT clause
		// Must match the conflictColumns in Save/SaveBatch: (repo_root, path, start_line, end_line, model)
		idxUniqueName := fmt.Sprintf("idx_%s_unique", tableName)
		idxUnique := s.dialect.CreateIndexSQL(tableName, idxUniqueName,
			[]string{"repo_root", "path", "start_line", "end_line", "model"}, true)
		if _, err := s.db.Exec(idxUnique); err != nil {
			return fmt.Errorf("creating unique index: %w", err)
		}

		// Create indexes for common queries
		idxPathName := fmt.Sprintf("idx_%s_path", tableName)
		idxPath := s.dialect.CreateIndexSQL(tableName, idxPathName, []string{"path"}, false)
		if _, err := s.db.Exec(idxPath); err != nil {
			return fmt.Errorf("creating path index: %w", err)
		}

		idxHashName := fmt.Sprintf("idx_%s_hash", tableName)
		idxHash := s.dialect.CreateIndexSQL(tableName, idxHashName, []string{"content_hash"}, false)
		if _, err := s.db.Exec(idxHash); err != nil {
			return fmt.Errorf("creating hash index: %w", err)
		}

		// Composite index for repo-scoped queries
		idxRepoPathName := fmt.Sprintf("idx_%s_repo_path", tableName)
		idxRepoPath := s.dialect.CreateIndexSQL(tableName, idxRepoPathName, []string{"repo_root", "path"}, false)
		if _, err := s.db.Exec(idxRepoPath); err != nil {
			return fmt.Errorf("creating repo_path index: %w", err)
		}

		return nil
	}

	// SQLite-specific schema (for backward compatibility)
	const sqliteSchema = `
CREATE TABLE IF NOT EXISTS embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    embedding TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(repo_root, path, start_line, end_line, model)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_path ON embeddings(path);
CREATE INDEX IF NOT EXISTS idx_embeddings_hash ON embeddings(content_hash);
CREATE INDEX IF NOT EXISTS idx_embeddings_repo_path ON embeddings(repo_root, path);
`
	if _, err := s.db.Exec(sqliteSchema); err != nil {
		return fmt.Errorf("creating embedding schema: %w", err)
	}

	return nil
}

// initRepoConfigTable creates the repo_embedding_configs table for PostgreSQL.
// This table tracks which model and dimensions each repository uses,
// enabling cross-repo search and proper migration between dimension groups.
func (s *EmbeddingStore) initRepoConfigTable() error {
	columns := []db.ColumnDef{
		{Name: "repo_root", Type: db.ColTypeText, Nullable: false, PrimaryKey: true},
		{Name: "model", Type: db.ColTypeText, Nullable: false},
		{Name: "dimensions", Type: db.ColTypeInteger, Nullable: false},
		{Name: "created_at", Type: db.ColTypeInteger, Nullable: false},
		{Name: "updated_at", Type: db.ColTypeInteger, Nullable: false},
	}

	sql := s.dialect.CreateTableSQL("repo_embedding_configs", columns)
	if _, err := s.db.Exec(sql); err != nil {
		return fmt.Errorf("creating repo_embedding_configs table: %w", err)
	}

	return nil
}

// RepoEmbeddingConfig represents the embedding configuration for a repository.
type RepoEmbeddingConfig struct {
	RepoRoot   string    `json:"repo_root"`
	Model      string    `json:"model"`
	Dimensions int       `json:"dimensions"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// GetRepoConfig returns the embedding configuration for a repository.
// Returns nil if no configuration exists (repository not yet embedded).
func (s *EmbeddingStore) GetRepoConfig(repoRoot string) (*RepoEmbeddingConfig, error) {
	if s.dialect.Name() == "sqlite" {
		// SQLite doesn't use repo configs (single table, no dimension constraints)
		return nil, nil
	}

	query := s.schema.SubstitutePlaceholders(`
		SELECT repo_root, model, dimensions, created_at, updated_at
		FROM repo_embedding_configs
		WHERE repo_root = ?`)

	var cfg RepoEmbeddingConfig
	var createdAt, updatedAt int64
	err := s.db.QueryRow(query, repoRoot).Scan(
		&cfg.RepoRoot, &cfg.Model, &cfg.Dimensions, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying repo config: %w", err)
	}

	cfg.CreatedAt = time.Unix(createdAt, 0)
	cfg.UpdatedAt = time.Unix(updatedAt, 0)
	return &cfg, nil
}

// SetRepoConfig creates or updates the embedding configuration for a repository.
func (s *EmbeddingStore) SetRepoConfig(repoRoot, model string, dimensions int) error {
	if s.dialect.Name() == "sqlite" {
		// SQLite doesn't use repo configs
		return nil
	}

	now := time.Now().Unix()

	// Upsert the config
	columns := []string{"repo_root", "model", "dimensions", "created_at", "updated_at"}
	conflictColumns := []string{"repo_root"}
	updateColumns := []string{"model", "dimensions", "updated_at"}

	sql := s.dialect.UpsertSQL("repo_embedding_configs", columns, conflictColumns, updateColumns)
	sql = s.schema.SubstitutePlaceholders(sql)

	_, err := s.db.Exec(sql, repoRoot, model, dimensions, now, now)
	return err
}

// ListRepoConfigs returns all repository embedding configurations.
// Useful for admin tools and cross-repo operations.
func (s *EmbeddingStore) ListRepoConfigs() ([]RepoEmbeddingConfig, error) {
	if s.dialect.Name() == "sqlite" {
		return nil, nil
	}

	query := `SELECT repo_root, model, dimensions, created_at, updated_at
		FROM repo_embedding_configs
		ORDER BY repo_root`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying repo configs: %w", err)
	}
	defer rows.Close()

	var configs []RepoEmbeddingConfig
	for rows.Next() {
		var cfg RepoEmbeddingConfig
		var createdAt, updatedAt int64
		if err := rows.Scan(&cfg.RepoRoot, &cfg.Model, &cfg.Dimensions, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning repo config: %w", err)
		}
		cfg.CreatedAt = time.Unix(createdAt, 0)
		cfg.UpdatedAt = time.Unix(updatedAt, 0)
		configs = append(configs, cfg)
	}

	return configs, rows.Err()
}

// CheckDimensionMismatch checks if the repository has existing embeddings with different dimensions.
// Returns (oldDimensions, hasMismatch, error). If hasMismatch is true, caller should handle migration.
func (s *EmbeddingStore) CheckDimensionMismatch(repoRoot string, newDimensions int) (int, bool, error) {
	if s.dialect.Name() == "sqlite" {
		// SQLite doesn't have dimension constraints
		return 0, false, nil
	}

	cfg, err := s.GetRepoConfig(repoRoot)
	if err != nil {
		return 0, false, fmt.Errorf("getting repo config: %w", err)
	}

	if cfg == nil {
		// No existing config, no mismatch
		return 0, false, nil
	}

	if cfg.Dimensions != newDimensions {
		return cfg.Dimensions, true, nil
	}

	return cfg.Dimensions, false, nil
}

// DeleteFromDimensionTable deletes all embeddings for a repository from a specific dimension table.
// Used during dimension migration to clean up old embeddings before re-embedding.
func (s *EmbeddingStore) DeleteFromDimensionTable(repoRoot string, dimensions int) error {
	if s.dialect.Name() == "sqlite" {
		// SQLite uses single table, just delete normally
		query := s.schema.SubstitutePlaceholders("DELETE FROM embeddings WHERE repo_root = ?")
		_, err := s.db.Exec(query, repoRoot)
		return err
	}

	tableName := tableNameForDimensions(s.dialect, dimensions)
	query := s.schema.SubstitutePlaceholders(fmt.Sprintf("DELETE FROM %s WHERE repo_root = ?", tableName))
	_, err := s.db.Exec(query, repoRoot)
	return err
}

// MigrateRepoDimensions handles switching a repository from one dimension group to another.
// This deletes embeddings from the old table and updates the repo config.
// The caller is responsible for re-embedding into the new dimension table.
func (s *EmbeddingStore) MigrateRepoDimensions(repoRoot string, oldDimensions, newDimensions int, newModel string) error {
	if s.dialect.Name() == "sqlite" {
		// SQLite doesn't need migration (no dimension constraints)
		return nil
	}

	// Delete from old dimension table
	if err := s.DeleteFromDimensionTable(repoRoot, oldDimensions); err != nil {
		return fmt.Errorf("deleting from old table: %w", err)
	}

	// Update repo config to new dimensions
	if err := s.SetRepoConfig(repoRoot, newModel, newDimensions); err != nil {
		return fmt.Errorf("updating repo config: %w", err)
	}

	return nil
}

// VectorDimensions returns the vector dimensions configured for this store.
func (s *EmbeddingStore) VectorDimensions() int {
	return s.vectorDim
}

// Save stores an embedding for a chunk
func (s *EmbeddingStore) Save(chunk Chunk, embedding []float32, model string) error {
	contentHash := hashContent(chunk.Content)
	embJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshaling embedding: %w", err)
	}

	// Use dialect-aware upsert with repo_root for multi-repo isolation
	columns := []string{"repo_root", "path", "start_line", "end_line", "content_hash", "embedding", "model", "created_at"}
	conflictColumns := []string{"repo_root", "path", "start_line", "end_line", "model"}
	updateColumns := []string{"content_hash", "embedding", "created_at"}

	tableName := s.tableName()
	upsertSQL := s.dialect.UpsertSQL(tableName, columns, conflictColumns, updateColumns)
	upsertSQL = s.schema.SubstitutePlaceholders(upsertSQL)

	_, err = s.db.Exec(upsertSQL,
		s.repoRoot, chunk.Path, chunk.StartLine, chunk.EndLine,
		contentHash, string(embJSON), model, time.Now().Unix())

	return err
}

// SaveBatch stores multiple embeddings in a transaction
func (s *EmbeddingStore) SaveBatch(chunks []Chunk, embeddings [][]float32, model string) error {
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("chunks and embeddings length mismatch")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Use dialect-aware upsert with repo_root for multi-repo isolation
	columns := []string{"repo_root", "path", "start_line", "end_line", "content_hash", "embedding", "model", "created_at"}
	conflictColumns := []string{"repo_root", "path", "start_line", "end_line", "model"}
	updateColumns := []string{"content_hash", "embedding", "created_at"}

	tableName := s.tableName()
	upsertSQL := s.dialect.UpsertSQL(tableName, columns, conflictColumns, updateColumns)
	upsertSQL = s.schema.SubstitutePlaceholders(upsertSQL)

	stmt, err := tx.Prepare(upsertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for i, chunk := range chunks {
		contentHash := hashContent(chunk.Content)
		embJSON, err := json.Marshal(embeddings[i])
		if err != nil {
			return fmt.Errorf("marshaling embedding %d: %w", i, err)
		}

		_, err = stmt.Exec(
			s.repoRoot, chunk.Path, chunk.StartLine, chunk.EndLine,
			contentHash, string(embJSON), model, now)
		if err != nil {
			return fmt.Errorf("inserting embedding %d: %w", i, err)
		}
	}

	return tx.Commit()
}

// GetByPath retrieves all embeddings for a file path within this repo
func (s *EmbeddingStore) GetByPath(path string) ([]EmbeddingRecord, error) {
	tableName := s.tableName()
	query := s.schema.SubstitutePlaceholders(fmt.Sprintf(`
		SELECT id, path, start_line, end_line, content_hash, embedding, model, created_at
		FROM %s
		WHERE repo_root = ? AND path = ?
		ORDER BY start_line`, tableName))
	rows, err := s.db.Query(query, s.repoRoot, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEmbeddingRecords(rows)
}

// GetAll retrieves all embeddings within this repo
func (s *EmbeddingStore) GetAll() ([]EmbeddingRecord, error) {
	tableName := s.tableName()
	query := s.schema.SubstitutePlaceholders(fmt.Sprintf(`
		SELECT id, path, start_line, end_line, content_hash, embedding, model, created_at
		FROM %s
		WHERE repo_root = ?
		ORDER BY path, start_line`, tableName))
	rows, err := s.db.Query(query, s.repoRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEmbeddingRecords(rows)
}

// GetAllAcrossRepos retrieves all embeddings from the dimension-specific table.
// If repoRoots is empty, returns all repos. If specified, filters to those repos.
// This enables cross-repo semantic search within a dimension group.
func (s *EmbeddingStore) GetAllAcrossRepos(repoRoots []string) ([]EmbeddingRecord, error) {
	tableName := s.tableName()

	var query string
	var args []interface{}

	if len(repoRoots) == 0 {
		// Get all repos in this dimension group
		query = s.schema.SubstitutePlaceholders(fmt.Sprintf(`
			SELECT id, repo_root, path, start_line, end_line, content_hash, embedding, model, created_at
			FROM %s
			ORDER BY repo_root, path, start_line`, tableName))
	} else {
		// Filter to specific repos
		placeholders := make([]string, len(repoRoots))
		for i, r := range repoRoots {
			placeholders[i] = "?"
			args = append(args, r)
		}
		query = s.schema.SubstitutePlaceholders(fmt.Sprintf(`
			SELECT id, repo_root, path, start_line, end_line, content_hash, embedding, model, created_at
			FROM %s
			WHERE repo_root IN (%s)
			ORDER BY repo_root, path, start_line`, tableName, strings.Join(placeholders, ", ")))
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEmbeddingRecordsWithRepo(rows)
}

// GetAllVectors retrieves just the embeddings for search
func (s *EmbeddingStore) GetAllVectors() ([]EmbeddingRecord, error) {
	return s.GetAll()
}

// HasEmbedding checks if a chunk already has an embedding with matching content within this repo
func (s *EmbeddingStore) HasEmbedding(chunk Chunk, model string) (bool, error) {
	contentHash := hashContent(chunk.Content)
	tableName := s.tableName()

	query := s.schema.SubstitutePlaceholders(fmt.Sprintf(`
		SELECT COUNT(*) FROM %s
		WHERE repo_root = ? AND path = ? AND start_line = ? AND end_line = ?
		AND content_hash = ? AND model = ?`, tableName))

	var count int
	err := s.db.QueryRow(query,
		s.repoRoot, chunk.Path, chunk.StartLine, chunk.EndLine, contentHash, model).Scan(&count)

	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteByPath removes all embeddings for a file within this repo
func (s *EmbeddingStore) DeleteByPath(path string) error {
	tableName := s.tableName()
	query := s.schema.SubstitutePlaceholders(fmt.Sprintf("DELETE FROM %s WHERE repo_root = ? AND path = ?", tableName))
	_, err := s.db.Exec(query, s.repoRoot, path)
	return err
}

// DeleteAll removes all embeddings within this repo
func (s *EmbeddingStore) DeleteAll() error {
	tableName := s.tableName()
	query := s.schema.SubstitutePlaceholders(fmt.Sprintf("DELETE FROM %s WHERE repo_root = ?", tableName))
	_, err := s.db.Exec(query, s.repoRoot)
	return err
}

// Count returns the number of stored embeddings within this repo
func (s *EmbeddingStore) Count() (int, error) {
	tableName := s.tableName()
	query := s.schema.SubstitutePlaceholders(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE repo_root = ?", tableName))
	var count int
	err := s.db.QueryRow(query, s.repoRoot).Scan(&count)
	return count, err
}

// Stats returns embedding statistics within this repo
func (s *EmbeddingStore) Stats() (count int, fileCount int, err error) {
	tableName := s.tableName()
	query := s.schema.SubstitutePlaceholders(fmt.Sprintf("SELECT COUNT(*), COUNT(DISTINCT path) FROM %s WHERE repo_root = ?", tableName))
	err = s.db.QueryRow(query, s.repoRoot).Scan(&count, &fileCount)
	return
}

func scanEmbeddingRecords(rows db.Rows) ([]EmbeddingRecord, error) {
	var records []EmbeddingRecord

	for rows.Next() {
		var r EmbeddingRecord
		var embJSON string
		var createdAt int64

		err := rows.Scan(
			&r.ID, &r.Path, &r.StartLine, &r.EndLine,
			&r.ContentHash, &embJSON, &r.Model, &createdAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(embJSON), &r.Embedding); err != nil {
			return nil, fmt.Errorf("unmarshaling embedding: %w", err)
		}

		r.CreatedAt = time.Unix(createdAt, 0)
		records = append(records, r)
	}

	return records, rows.Err()
}

// scanEmbeddingRecordsWithRepo scans rows that include repo_root column
func scanEmbeddingRecordsWithRepo(rows db.Rows) ([]EmbeddingRecord, error) {
	var records []EmbeddingRecord

	for rows.Next() {
		var r EmbeddingRecord
		var embJSON string
		var createdAt int64

		err := rows.Scan(
			&r.ID, &r.RepoRoot, &r.Path, &r.StartLine, &r.EndLine,
			&r.ContentHash, &embJSON, &r.Model, &createdAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(embJSON), &r.Embedding); err != nil {
			return nil, fmt.Errorf("unmarshaling embedding: %w", err)
		}

		r.CreatedAt = time.Unix(createdAt, 0)
		records = append(records, r)
	}

	return records, rows.Err()
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
