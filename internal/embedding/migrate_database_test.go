package embedding

import (
	"context"
	"os"
	"testing"

	"codetect/internal/db"
)

func TestMigrateDatabase(t *testing.T) {
	// Test repo root for multi-repo isolation
	testRepoRoot := "/test/repo"

	// Set up source SQLite database
	sourceCfg := db.DefaultConfig(":memory:")
	sourceDB, err := db.Open(sourceCfg)
	if err != nil {
		t.Fatalf("Failed to open source database: %v", err)
	}
	defer sourceDB.Close()

	sourceStore, err := NewEmbeddingStore(sourceDB, testRepoRoot)
	if err != nil {
		t.Fatalf("Failed to create source store: %v", err)
	}

	// Set up target in-memory database (also SQLite for testing)
	targetCfg := db.DefaultConfig(":memory:")
	targetDB, err := db.Open(targetCfg)
	if err != nil {
		t.Fatalf("Failed to open target database: %v", err)
	}
	defer targetDB.Close()

	targetStore, err := NewEmbeddingStore(targetDB, testRepoRoot)
	if err != nil {
		t.Fatalf("Failed to create target store: %v", err)
	}

	// Insert test data into source
	testChunks := []Chunk{
		{Path: "file1.go", StartLine: 1, EndLine: 10, Content: "func main() {}"},
		{Path: "file1.go", StartLine: 11, EndLine: 20, Content: "func test() {}"},
		{Path: "file2.go", StartLine: 1, EndLine: 15, Content: "type Foo struct {}"},
	}

	testEmbeddings := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
	}

	err = sourceStore.SaveBatch(testChunks, testEmbeddings, "test-model")
	if err != nil {
		t.Fatalf("Failed to save test data: %v", err)
	}

	t.Run("Basic Migration", func(t *testing.T) {
		opts := DefaultMigrationOptions()
		opts.BatchSize = 2 // Test batching with small size

		progressCalls := 0
		callback := func(progress MigrationProgress) {
			progressCalls++
			t.Logf("Progress: %d/%d migrated, %d skipped",
				progress.MigratedEmbeddings, progress.TotalEmbeddings, progress.SkippedEmbeddings)
		}

		err := MigrateDatabase(context.Background(), sourceStore, targetStore, opts, callback)
		if err != nil {
			t.Fatalf("Migration failed: %v", err)
		}

		// Verify counts
		sourceCount, _ := sourceStore.Count()
		targetCount, _ := targetStore.Count()

		if sourceCount != targetCount {
			t.Errorf("Count mismatch: source=%d, target=%d", sourceCount, targetCount)
		}

		if progressCalls == 0 {
			t.Error("Progress callback was never called")
		}
	})

	t.Run("Skip Existing", func(t *testing.T) {
		// Target already has all data from previous test
		opts := DefaultMigrationOptions()
		opts.SkipExisting = true

		skippedCount := 0
		callback := func(progress MigrationProgress) {
			skippedCount = progress.SkippedEmbeddings
		}

		err := MigrateDatabase(context.Background(), sourceStore, targetStore, opts, callback)
		if err != nil {
			t.Fatalf("Migration failed: %v", err)
		}

		if skippedCount != 3 {
			t.Errorf("Expected 3 skipped embeddings, got %d", skippedCount)
		}
	})

	t.Run("Dry Run", func(t *testing.T) {
		// Clear target
		targetStore.DeleteAll()

		opts := DefaultMigrationOptions()
		opts.DryRun = true

		err := MigrateDatabase(context.Background(), sourceStore, targetStore, opts, nil)
		if err != nil {
			t.Fatalf("Dry run failed: %v", err)
		}

		// Target should still be empty
		targetCount, _ := targetStore.Count()
		if targetCount != 0 {
			t.Errorf("Dry run should not migrate data, but found %d embeddings", targetCount)
		}
	})

	t.Run("Drop Target", func(t *testing.T) {
		// Add some data to target
		chunk := Chunk{Path: "old.go", StartLine: 1, EndLine: 10, Content: "old"}
		embedding := []float32{0.5, 0.5, 0.5}
		targetStore.Save(chunk, embedding, "old-model")

		opts := DefaultMigrationOptions()
		opts.DropTarget = true

		err := MigrateDatabase(context.Background(), sourceStore, targetStore, opts, nil)
		if err != nil {
			t.Fatalf("Migration with drop failed: %v", err)
		}

		// Target should only have source data, not the old data
		targetEmbeddings, _ := targetStore.GetAll()
		for _, emb := range targetEmbeddings {
			if emb.Model == "old-model" {
				t.Error("Old data should have been dropped")
			}
		}
	})

	t.Run("Context Cancellation", func(t *testing.T) {
		// Clear target
		targetStore.DeleteAll()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		opts := DefaultMigrationOptions()
		err := MigrateDatabase(ctx, sourceStore, targetStore, opts, nil)

		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		}
	})
}

func TestValidateMigration(t *testing.T) {
	// Test repo root for multi-repo isolation
	testRepoRoot := "/test/repo"

	// Set up source database
	sourceCfg := db.DefaultConfig(":memory:")
	sourceDB, err := db.Open(sourceCfg)
	if err != nil {
		t.Fatalf("Failed to open source database: %v", err)
	}
	defer sourceDB.Close()

	sourceStore, err := NewEmbeddingStore(sourceDB, testRepoRoot)
	if err != nil {
		t.Fatalf("Failed to create source store: %v", err)
	}

	// Set up target database
	targetCfg := db.DefaultConfig(":memory:")
	targetDB, err := db.Open(targetCfg)
	if err != nil {
		t.Fatalf("Failed to open target database: %v", err)
	}
	defer targetDB.Close()

	targetStore, err := NewEmbeddingStore(targetDB, testRepoRoot)
	if err != nil {
		t.Fatalf("Failed to create target store: %v", err)
	}

	// Insert test data
	testChunks := []Chunk{
		{Path: "file1.go", StartLine: 1, EndLine: 10, Content: "func main() {}"},
		{Path: "file2.go", StartLine: 1, EndLine: 15, Content: "type Foo struct {}"},
	}

	testEmbeddings := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
	}

	sourceStore.SaveBatch(testChunks, testEmbeddings, "test-model")

	t.Run("Validation Success", func(t *testing.T) {
		// Migrate data
		opts := DefaultMigrationOptions()
		err := MigrateDatabase(context.Background(), sourceStore, targetStore, opts, nil)
		if err != nil {
			t.Fatalf("Migration failed: %v", err)
		}

		// Validate
		err = ValidateMigration(sourceStore, targetStore, 10)
		if err != nil {
			t.Errorf("Validation failed: %v", err)
		}
	})

	t.Run("Validation Count Mismatch", func(t *testing.T) {
		// Delete one embedding from target
		targetStore.DeleteByPath("file2.go")

		err := ValidateMigration(sourceStore, targetStore, 10)
		if err == nil {
			t.Error("Expected validation to fail with count mismatch")
		}
	})

	t.Run("Validation Empty Database", func(t *testing.T) {
		emptySourceCfg := db.DefaultConfig(":memory:")
		emptySourceDB, err := db.Open(emptySourceCfg)
		if err != nil {
			t.Fatalf("Failed to open empty source database: %v", err)
		}
		defer emptySourceDB.Close()

		emptySource, _ := NewEmbeddingStore(emptySourceDB, testRepoRoot)

		emptyTargetCfg := db.DefaultConfig(":memory:")
		emptyTargetDB, err := db.Open(emptyTargetCfg)
		if err != nil {
			t.Fatalf("Failed to open empty target database: %v", err)
		}
		defer emptyTargetDB.Close()

		emptyTarget, _ := NewEmbeddingStore(emptyTargetDB, testRepoRoot)

		err = ValidateMigration(emptySource, emptyTarget, 10)
		if err != nil {
			t.Errorf("Empty database validation should succeed: %v", err)
		}
	})
}

// TestMigrateDatabaseWithVectorIndex tests migration with PostgreSQL and vector indexing.
// This test requires a PostgreSQL database with pgvector extension.
func TestMigrateDatabaseWithVectorIndex(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set, skipping PostgreSQL migration test")
	}

	// Test repo root for multi-repo isolation
	testRepoRoot := "/test/repo"

	// Set up source SQLite database
	sourceCfg := db.DefaultConfig(":memory:")
	sourceDB, err := db.Open(sourceCfg)
	if err != nil {
		t.Fatalf("Failed to open source database: %v", err)
	}
	defer sourceDB.Close()

	sourceStore, err := NewEmbeddingStore(sourceDB, testRepoRoot)
	if err != nil {
		t.Fatalf("Failed to create source store: %v", err)
	}

	// Insert test data
	testChunks := []Chunk{
		{Path: "file1.go", StartLine: 1, EndLine: 10, Content: "func main() {}"},
		{Path: "file2.go", StartLine: 1, EndLine: 15, Content: "type Foo struct {}"},
	}

	testEmbeddings := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
	}

	err = sourceStore.SaveBatch(testChunks, testEmbeddings, "test-model")
	if err != nil {
		t.Fatalf("Failed to save test data: %v", err)
	}

	// Set up target PostgreSQL database
	cfg := db.PostgresConfig(dsn)
	targetDB, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open PostgreSQL: %v", err)
	}
	defer targetDB.Close()

	targetDialect := db.GetDialect(db.DatabasePostgres)
	targetStore, err := NewEmbeddingStoreWithOptions(targetDB, targetDialect, 3, testRepoRoot)
	if err != nil {
		t.Fatalf("Failed to create target store: %v", err)
	}

	// Clean up target
	defer targetStore.DeleteAll()

	t.Run("Migration With Vector Index", func(t *testing.T) {
		opts := DefaultMigrationOptions()

		err := MigrateDatabaseWithVectorIndex(context.Background(), sourceStore, targetStore, opts, nil)
		if err != nil {
			t.Fatalf("Migration with vector index failed: %v", err)
		}

		// Verify counts
		sourceCount, _ := sourceStore.Count()
		targetCount, _ := targetStore.Count()

		if sourceCount != targetCount {
			t.Errorf("Count mismatch: source=%d, target=%d", sourceCount, targetCount)
		}

		// Verify vector index exists
		var indexExists bool
		err = targetDB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM pg_indexes
				WHERE tablename = 'embeddings'
				AND indexname LIKE '%embedding%'
			)
		`).Scan(&indexExists)

		if err != nil {
			t.Fatalf("Failed to check for index: %v", err)
		}

		if !indexExists {
			t.Error("Vector index was not created")
		}
	})
}
