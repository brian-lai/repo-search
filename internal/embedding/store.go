package embedding

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"repo-search/internal/db"
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

// EmbeddingStore manages embedding storage in SQLite
type EmbeddingStore struct {
	db db.DB
}

const embeddingSchema = `
CREATE TABLE IF NOT EXISTS embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    embedding TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(path, start_line, end_line, model)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_path ON embeddings(path);
CREATE INDEX IF NOT EXISTS idx_embeddings_hash ON embeddings(content_hash);
`

// NewEmbeddingStore creates a new embedding store using a db.DB adapter.
func NewEmbeddingStore(database db.DB) (*EmbeddingStore, error) {
	// Create embedding table if not exists
	if _, err := database.Exec(embeddingSchema); err != nil {
		return nil, fmt.Errorf("creating embedding schema: %w", err)
	}

	return &EmbeddingStore{db: database}, nil
}

// NewEmbeddingStoreFromSQL creates an embedding store from a raw *sql.DB.
// This is for backward compatibility with existing code.
// Prefer NewEmbeddingStore with db.DB for new code.
func NewEmbeddingStoreFromSQL(sqlDB *sql.DB) (*EmbeddingStore, error) {
	return NewEmbeddingStore(db.WrapSQL(sqlDB))
}

// Save stores an embedding for a chunk
func (s *EmbeddingStore) Save(chunk Chunk, embedding []float32, model string) error {
	contentHash := hashContent(chunk.Content)
	embJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshaling embedding: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO embeddings
		(path, start_line, end_line, content_hash, embedding, model, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		chunk.Path, chunk.StartLine, chunk.EndLine,
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
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO embeddings
		(path, start_line, end_line, content_hash, embedding, model, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
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
			chunk.Path, chunk.StartLine, chunk.EndLine,
			contentHash, string(embJSON), model, now)
		if err != nil {
			return fmt.Errorf("inserting embedding %d: %w", i, err)
		}
	}

	return tx.Commit()
}

// GetByPath retrieves all embeddings for a file path
func (s *EmbeddingStore) GetByPath(path string) ([]EmbeddingRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, path, start_line, end_line, content_hash, embedding, model, created_at
		FROM embeddings
		WHERE path = ?
		ORDER BY start_line`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEmbeddingRecords(rows)
}

// GetAll retrieves all embeddings
func (s *EmbeddingStore) GetAll() ([]EmbeddingRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, path, start_line, end_line, content_hash, embedding, model, created_at
		FROM embeddings
		ORDER BY path, start_line`)
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

// HasEmbedding checks if a chunk already has an embedding with matching content
func (s *EmbeddingStore) HasEmbedding(chunk Chunk, model string) (bool, error) {
	contentHash := hashContent(chunk.Content)

	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM embeddings
		WHERE path = ? AND start_line = ? AND end_line = ?
		AND content_hash = ? AND model = ?`,
		chunk.Path, chunk.StartLine, chunk.EndLine, contentHash, model).Scan(&count)

	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteByPath removes all embeddings for a file
func (s *EmbeddingStore) DeleteByPath(path string) error {
	_, err := s.db.Exec("DELETE FROM embeddings WHERE path = ?", path)
	return err
}

// DeleteAll removes all embeddings
func (s *EmbeddingStore) DeleteAll() error {
	_, err := s.db.Exec("DELETE FROM embeddings")
	return err
}

// Count returns the number of stored embeddings
func (s *EmbeddingStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&count)
	return count, err
}

// Stats returns embedding statistics
func (s *EmbeddingStore) Stats() (count int, fileCount int, err error) {
	err = s.db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT path) FROM embeddings").Scan(&count, &fileCount)
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
