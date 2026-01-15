package embedding

import (
	"encoding/json"
	"fmt"
)

// MigrateToVectorType migrates embeddings from TEXT (JSON) to native vector type.
// This is useful when migrating from SQLite to PostgreSQL or when upgrading
// an existing PostgreSQL database that was using TEXT storage.
//
// The migration process:
// 1. Creates a new temporary table with vector column
// 2. Copies data, converting JSON arrays to vector format
// 3. Drops old table and renames new table
//
// WARNING: This operation requires a table lock and may take time for large datasets.
func (s *EmbeddingStore) MigrateToVectorType() error {
	if !s.useNativeVec {
		return fmt.Errorf("migration only supported for PostgreSQL with pgvector")
	}

	// Check if already using vector type
	hasVectorType, err := s.checkIfVectorType()
	if err != nil {
		return fmt.Errorf("checking current schema: %w", err)
	}
	if hasVectorType {
		return nil // Already migrated
	}

	// Create temporary table with vector type
	tempColumns := embeddingColumnsForDialect(s.dialect, s.vectorDim)
	tempTableSQL := s.dialect.CreateTableSQL("embeddings_new", tempColumns)

	if _, err := s.db.Exec(tempTableSQL); err != nil {
		return fmt.Errorf("creating temporary table: %w", err)
	}

	// Copy data with type conversion
	// PostgreSQL can cast JSON array string to vector automatically
	copySQL := `
		INSERT INTO embeddings_new (id, path, start_line, end_line, content_hash, embedding, model, created_at)
		SELECT id, path, start_line, end_line, content_hash, embedding::vector, model, created_at
		FROM embeddings
	`

	if _, err := s.db.Exec(copySQL); err != nil {
		// Rollback: drop temporary table
		s.db.Exec("DROP TABLE embeddings_new")
		return fmt.Errorf("copying data to new table: %w", err)
	}

	// Start transaction for the swap
	tx, err := s.db.Begin()
	if err != nil {
		s.db.Exec("DROP TABLE embeddings_new")
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Drop old table
	if _, err := tx.Exec("DROP TABLE embeddings"); err != nil {
		return fmt.Errorf("dropping old table: %w", err)
	}

	// Rename new table
	if _, err := tx.Exec("ALTER TABLE embeddings_new RENAME TO embeddings"); err != nil {
		return fmt.Errorf("renaming new table: %w", err)
	}

	// Recreate indexes
	idxPath := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_path", []string{"path"}, false)
	if _, err := tx.Exec(idxPath); err != nil {
		return fmt.Errorf("creating path index: %w", err)
	}

	idxHash := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_hash", []string{"content_hash"}, false)
	if _, err := tx.Exec(idxHash); err != nil {
		return fmt.Errorf("creating hash index: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration: %w", err)
	}

	return nil
}

// checkIfVectorType checks if the embeddings table is already using vector type.
func (s *EmbeddingStore) checkIfVectorType() (bool, error) {
	// Query PostgreSQL information schema to check column type
	query := `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_name = 'embeddings'
		AND column_name = 'embedding'
	`

	var dataType string
	err := s.db.QueryRow(query).Scan(&dataType)
	if err != nil {
		return false, err
	}

	// PostgreSQL pgvector type shows as "USER-DEFINED"
	return dataType == "USER-DEFINED", nil
}

// ValidateMigration validates that all embeddings were migrated correctly
// by comparing the original JSON data with the vector data.
func (s *EmbeddingStore) ValidateMigration(sampleSize int) error {
	if !s.useNativeVec {
		return fmt.Errorf("validation only supported for PostgreSQL with pgvector")
	}

	// Get a sample of embeddings
	query := fmt.Sprintf(`
		SELECT embedding
		FROM embeddings
		LIMIT %d
	`, sampleSize)

	rows, err := s.db.Query(query)
	if err != nil {
		return fmt.Errorf("querying embeddings: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var vectorStr string
		if err := rows.Scan(&vectorStr); err != nil {
			return fmt.Errorf("scanning embedding %d: %w", count, err)
		}

		// Parse vector string (pgvector format: [1,2,3,...])
		var vector []float32
		if err := json.Unmarshal([]byte(vectorStr), &vector); err != nil {
			return fmt.Errorf("parsing vector %d: %w", count, err)
		}

		// Check dimensions
		if len(vector) != s.vectorDim {
			return fmt.Errorf("vector %d has incorrect dimensions: got %d, want %d",
				count, len(vector), s.vectorDim)
		}

		count++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating embeddings: %w", err)
	}

	return nil
}

// EstimateMigrationTime provides an estimate of how long migration will take
// based on the number of embeddings.
func (s *EmbeddingStore) EstimateMigrationTime() (embeddingCount int, estimatedSeconds int, err error) {
	embeddingCount, err = s.Count()
	if err != nil {
		return 0, 0, err
	}

	// Rough estimate: ~1000 embeddings per second for type conversion
	estimatedSeconds = embeddingCount / 1000
	if estimatedSeconds < 1 {
		estimatedSeconds = 1
	}

	return embeddingCount, estimatedSeconds, nil
}
