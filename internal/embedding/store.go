package embedding

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"codetect/internal/db"
)

// EmbeddingRecord represents a stored embedding
type EmbeddingRecord struct {
	ID          int64     `json:"id"`
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
type EmbeddingStore struct {
	db           db.DB
	dialect      db.Dialect
	schema       *db.SchemaBuilder
	vectorDim    int    // Vector dimensions (e.g., 768 for nomic-embed-text)
	useNativeVec bool   // True if using PostgreSQL native vector type
	repoRoot     string // Absolute path to repo root for multi-repo isolation
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
func NewEmbeddingStore(database db.DB) (*EmbeddingStore, error) {
	return NewEmbeddingStoreWithDialect(database, db.GetDialect(db.DatabaseSQLite))
}

// NewEmbeddingStoreWithDialect creates an embedding store with a specific SQL dialect.
// Uses default vector dimensions (768) for PostgreSQL.
func NewEmbeddingStoreWithDialect(database db.DB, dialect db.Dialect) (*EmbeddingStore, error) {
	return NewEmbeddingStoreWithOptions(database, dialect, 768)
}

// NewEmbeddingStoreWithOptions creates an embedding store with custom vector dimensions.
func NewEmbeddingStoreWithOptions(database db.DB, dialect db.Dialect, vectorDim int) (*EmbeddingStore, error) {
	useNativeVec := dialect.Name() == "postgres"

	store := &EmbeddingStore{
		db:           database,
		dialect:      dialect,
		schema:       db.NewSchemaBuilder(database, dialect),
		vectorDim:    vectorDim,
		useNativeVec: useNativeVec,
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
func NewEmbeddingStoreFromSQL(sqlDB *sql.DB) (*EmbeddingStore, error) {
	return NewEmbeddingStore(db.WrapSQL(sqlDB))
}

// NewEmbeddingStoreFromConfig creates an embedding store using a db.Config.
func NewEmbeddingStoreFromConfig(database db.DB, cfg db.Config) (*EmbeddingStore, error) {
	return NewEmbeddingStoreWithDialect(database, cfg.Dialect())
}

// initSchema creates the embeddings table if it doesn't exist.
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
		// Get column definitions based on dialect
		columns := embeddingColumnsForDialect(s.dialect, s.vectorDim)

		// Create table using dialect
		sql := s.dialect.CreateTableSQL("embeddings", columns)
		if _, err := s.db.Exec(sql); err != nil {
			return fmt.Errorf("creating embeddings table: %w", err)
		}

		// Add unique constraint via index (not all dialects support inline UNIQUE)
		// Note: This is simplified; full implementation would need dialect-aware constraints

		// Create unique constraint for upsert ON CONFLICT clause
		// Must match the conflictColumns in Save/SaveBatch: (repo_root, path, start_line, end_line, model)
		idxUnique := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_unique",
			[]string{"repo_root", "path", "start_line", "end_line", "model"}, true)
		if _, err := s.db.Exec(idxUnique); err != nil {
			return fmt.Errorf("creating unique index: %w", err)
		}

		// Create indexes for common queries
		idxPath := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_path", []string{"path"}, false)
		if _, err := s.db.Exec(idxPath); err != nil {
			return fmt.Errorf("creating path index: %w", err)
		}

		idxHash := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_hash", []string{"content_hash"}, false)
		if _, err := s.db.Exec(idxHash); err != nil {
			return fmt.Errorf("creating hash index: %w", err)
		}

		// Composite index for repo-scoped queries
		idxRepoPath := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_repo_path", []string{"repo_root", "path"}, false)
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

	sql := s.dialect.UpsertSQL("embeddings", columns, conflictColumns, updateColumns)
	sql = s.schema.SubstitutePlaceholders(sql)

	_, err = s.db.Exec(sql,
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

	sql := s.dialect.UpsertSQL("embeddings", columns, conflictColumns, updateColumns)
	sql = s.schema.SubstitutePlaceholders(sql)

	stmt, err := tx.Prepare(sql)
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
	query := s.schema.SubstitutePlaceholders(`
		SELECT id, path, start_line, end_line, content_hash, embedding, model, created_at
		FROM embeddings
		WHERE repo_root = ? AND path = ?
		ORDER BY start_line`)
	rows, err := s.db.Query(query, s.repoRoot, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEmbeddingRecords(rows)
}

// GetAll retrieves all embeddings within this repo
func (s *EmbeddingStore) GetAll() ([]EmbeddingRecord, error) {
	query := s.schema.SubstitutePlaceholders(`
		SELECT id, path, start_line, end_line, content_hash, embedding, model, created_at
		FROM embeddings
		WHERE repo_root = ?
		ORDER BY path, start_line`)
	rows, err := s.db.Query(query, s.repoRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEmbeddingRecords(rows)
}

// GetAllVectors retrieves just the embeddings for search
func (s *EmbeddingStore) GetAllVectors() ([]EmbeddingRecord, error) {
	return s.GetAll()
}

// HasEmbedding checks if a chunk already has an embedding with matching content within this repo
func (s *EmbeddingStore) HasEmbedding(chunk Chunk, model string) (bool, error) {
	contentHash := hashContent(chunk.Content)

	query := s.schema.SubstitutePlaceholders(`
		SELECT COUNT(*) FROM embeddings
		WHERE repo_root = ? AND path = ? AND start_line = ? AND end_line = ?
		AND content_hash = ? AND model = ?`)

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
	query := s.schema.SubstitutePlaceholders("DELETE FROM embeddings WHERE repo_root = ? AND path = ?")
	_, err := s.db.Exec(query, s.repoRoot, path)
	return err
}

// DeleteAll removes all embeddings within this repo
func (s *EmbeddingStore) DeleteAll() error {
	query := s.schema.SubstitutePlaceholders("DELETE FROM embeddings WHERE repo_root = ?")
	_, err := s.db.Exec(query, s.repoRoot)
	return err
}

// Count returns the number of stored embeddings within this repo
func (s *EmbeddingStore) Count() (int, error) {
	query := s.schema.SubstitutePlaceholders("SELECT COUNT(*) FROM embeddings WHERE repo_root = ?")
	var count int
	err := s.db.QueryRow(query, s.repoRoot).Scan(&count)
	return count, err
}

// Stats returns embedding statistics within this repo
func (s *EmbeddingStore) Stats() (count int, fileCount int, err error) {
	query := s.schema.SubstitutePlaceholders("SELECT COUNT(*), COUNT(DISTINCT path) FROM embeddings WHERE repo_root = ?")
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

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
