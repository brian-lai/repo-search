package embedding

import (
	"context"
	"fmt"

	"codetect/internal/db"
)

// MigrationOptions configures database migration behavior.
type MigrationOptions struct {
	// BatchSize controls how many embeddings to migrate at once
	BatchSize int

	// SkipExisting skips embeddings that already exist in the target
	SkipExisting bool

	// DropTarget drops the target database tables before migration
	DropTarget bool

	// DryRun performs validation without actually migrating data
	DryRun bool
}

// DefaultMigrationOptions returns sensible defaults for migration.
func DefaultMigrationOptions() MigrationOptions {
	return MigrationOptions{
		BatchSize:    1000,
		SkipExisting: true,
		DropTarget:   false,
		DryRun:       false,
	}
}

// MigrationProgress tracks the progress of a database migration.
type MigrationProgress struct {
	TotalEmbeddings   int
	MigratedEmbeddings int
	SkippedEmbeddings int
	FailedEmbeddings  int
	CurrentFile       string
}

// MigrationCallback is called periodically during migration to report progress.
type MigrationCallback func(progress MigrationProgress)

// MigrateDatabase migrates all embeddings from one database to another.
// This is useful for migrating from SQLite to PostgreSQL.
//
// Example:
//
//	sourceStore, _ := NewEmbeddingStore(sqliteDB)
//	targetStore, _ := NewEmbeddingStoreWithDialect(postgresDB, postgresDialect)
//	err := MigrateDatabase(ctx, sourceStore, targetStore, opts, progressCallback)
func MigrateDatabase(
	ctx context.Context,
	source *EmbeddingStore,
	target *EmbeddingStore,
	opts MigrationOptions,
	callback MigrationCallback,
) error {
	// Validate options
	if opts.BatchSize <= 0 {
		opts.BatchSize = 1000
	}

	// Initialize target schema if needed
	if opts.DropTarget && !opts.DryRun {
		if err := target.DeleteAll(); err != nil {
			return fmt.Errorf("clearing target database: %w", err)
		}
	}

	// Get total count for progress tracking
	totalCount, err := source.Count()
	if err != nil {
		return fmt.Errorf("counting source embeddings: %w", err)
	}

	progress := MigrationProgress{
		TotalEmbeddings: totalCount,
	}

	if opts.DryRun {
		if callback != nil {
			callback(progress)
		}
		return nil
	}

	// Get all embeddings from source
	// For large datasets, this should be paginated, but for now we load all
	embeddings, err := source.GetAll()
	if err != nil {
		return fmt.Errorf("fetching source embeddings: %w", err)
	}

	// Process in batches
	for i := 0; i < len(embeddings); i += opts.BatchSize {
		end := i + opts.BatchSize
		if end > len(embeddings) {
			end = len(embeddings)
		}

		batch := embeddings[i:end]

		// Convert to chunks and vectors for batch insertion
		chunks := make([]Chunk, len(batch))
		vectors := make([][]float32, len(batch))
		model := ""

		for j, emb := range batch {
			chunks[j] = Chunk{
				Path:      emb.Path,
				StartLine: emb.StartLine,
				EndLine:   emb.EndLine,
				Content:   "", // Not needed for migration
			}
			vectors[j] = emb.Embedding
			if model == "" {
				model = emb.Model
			}

			// Update progress
			if emb.Path != progress.CurrentFile {
				progress.CurrentFile = emb.Path
			}
		}

		// Check for existing embeddings if SkipExisting is enabled
		if opts.SkipExisting {
			// Filter out existing embeddings
			filteredChunks := make([]Chunk, 0, len(chunks))
			filteredVectors := make([][]float32, 0, len(vectors))

			for j, chunk := range chunks {
				exists, err := target.HasEmbedding(chunk, model)
				if err != nil {
					return fmt.Errorf("checking existing embedding: %w", err)
				}

				if !exists {
					filteredChunks = append(filteredChunks, chunk)
					filteredVectors = append(filteredVectors, vectors[j])
				} else {
					progress.SkippedEmbeddings++
				}
			}

			chunks = filteredChunks
			vectors = filteredVectors
		}

		// Save batch to target
		if len(chunks) > 0 {
			if err := target.SaveBatch(chunks, vectors, model); err != nil {
				progress.FailedEmbeddings += len(chunks)
				return fmt.Errorf("saving batch to target: %w", err)
			}

			progress.MigratedEmbeddings += len(chunks)
		}

		// Report progress
		if callback != nil {
			callback(progress)
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// MigrateDatabaseWithVectorIndex migrates embeddings and creates a vector index
// on the target database (if it supports native vector search).
func MigrateDatabaseWithVectorIndex(
	ctx context.Context,
	source *EmbeddingStore,
	target *EmbeddingStore,
	opts MigrationOptions,
	callback MigrationCallback,
) error {
	// First, migrate the data
	if err := MigrateDatabase(ctx, source, target, opts, callback); err != nil {
		return err
	}

	// If target is PostgreSQL, migrate to vector type and create index
	if target.useNativeVec && !opts.DryRun {
		// Migrate to native vector type (if not already)
		if err := target.MigrateToVectorType(); err != nil {
			return fmt.Errorf("migrating to vector type: %w", err)
		}

		// Create vector index using PgVectorDB
		vdb, err := db.NewPgVectorDB(target.db, target.vectorDim, db.DistanceCosine)
		if err != nil {
			return fmt.Errorf("creating vector database: %w", err)
		}

		if err := vdb.CreateVectorIndex(ctx, "embeddings", target.vectorDim, db.DistanceCosine); err != nil {
			return fmt.Errorf("creating vector index: %w", err)
		}
	}

	return nil
}

// ValidateMigration validates that a migration was successful by comparing
// embedding counts and sampling random embeddings.
func ValidateMigration(source *EmbeddingStore, target *EmbeddingStore, sampleSize int) error {
	// Compare counts
	sourceCount, err := source.Count()
	if err != nil {
		return fmt.Errorf("counting source embeddings: %w", err)
	}

	targetCount, err := target.Count()
	if err != nil {
		return fmt.Errorf("counting target embeddings: %w", err)
	}

	if sourceCount != targetCount {
		return fmt.Errorf("embedding count mismatch: source=%d, target=%d", sourceCount, targetCount)
	}

	// Sample random embeddings and compare
	sourceEmbeddings, err := source.GetAll()
	if err != nil {
		return fmt.Errorf("fetching source embeddings: %w", err)
	}

	if len(sourceEmbeddings) == 0 {
		return nil // Empty database, nothing to validate
	}

	// Sample at most sampleSize embeddings
	step := len(sourceEmbeddings) / sampleSize
	if step < 1 {
		step = 1
	}

	for i := 0; i < len(sourceEmbeddings); i += step {
		srcEmb := sourceEmbeddings[i]

		// Find matching embedding in target
		targetEmbeddings, err := target.GetByPath(srcEmb.Path)
		if err != nil {
			return fmt.Errorf("fetching target embeddings for %s: %w", srcEmb.Path, err)
		}

		// Find exact match
		found := false
		for _, tgtEmb := range targetEmbeddings {
			if tgtEmb.StartLine == srcEmb.StartLine &&
				tgtEmb.EndLine == srcEmb.EndLine &&
				tgtEmb.Model == srcEmb.Model {
				found = true

				// Compare embedding dimensions
				if len(tgtEmb.Embedding) != len(srcEmb.Embedding) {
					return fmt.Errorf("embedding dimension mismatch for %s:%d-%d",
						srcEmb.Path, srcEmb.StartLine, srcEmb.EndLine)
				}

				// Compare first few values (floating point comparison with tolerance)
				compareCount := min(10, len(srcEmb.Embedding))
				for j := range compareCount {
					diff := srcEmb.Embedding[j] - tgtEmb.Embedding[j]
					if diff < 0 {
						diff = -diff
					}
					if diff > 0.0001 {
						return fmt.Errorf("embedding value mismatch for %s:%d-%d at index %d",
							srcEmb.Path, srcEmb.StartLine, srcEmb.EndLine, j)
					}
				}

				break
			}
		}

		if !found {
			return fmt.Errorf("embedding not found in target: %s:%d-%d",
				srcEmb.Path, srcEmb.StartLine, srcEmb.EndLine)
		}
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
