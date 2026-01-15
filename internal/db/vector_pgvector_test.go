package db

import (
	"context"
	"os"
	"testing"
)

// TestPgVectorDB tests the pgvector implementation.
// Requires PostgreSQL with pgvector extension.
// Set POSTGRES_TEST_DSN to run these tests.
func TestPgVectorDB(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set, skipping PostgreSQL tests")
	}

	cfg := PostgresConfig(dsn)
	database, err := Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open PostgreSQL: %v", err)
	}
	defer database.Close()

	// Create test table with vector column
	_, err = database.Exec(`
		CREATE EXTENSION IF NOT EXISTS vector;
		DROP TABLE IF EXISTS test_embeddings;
		CREATE TABLE test_embeddings (
			id SERIAL PRIMARY KEY,
			embedding vector(3)
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	defer database.Exec("DROP TABLE test_embeddings")

	t.Run("NewPgVectorDB", func(t *testing.T) {
		vdb, err := NewPgVectorDB(database, 3, DistanceCosine)
		if err != nil {
			t.Fatalf("Failed to create PgVectorDB: %v", err)
		}

		if !vdb.SupportsNativeSearch() {
			t.Error("PgVectorDB should support native search")
		}

		if vdb.dimensions != 3 {
			t.Errorf("Expected dimensions 3, got %d", vdb.dimensions)
		}

		if vdb.metric != DistanceCosine {
			t.Errorf("Expected metric cosine, got %s", vdb.metric.String())
		}
	})

	t.Run("CreateVectorIndex", func(t *testing.T) {
		vdb, _ := NewPgVectorDB(database, 3, DistanceCosine)

		// Test cosine index
		err := vdb.CreateVectorIndex(context.Background(), "test_embeddings", 3, DistanceCosine)
		if err != nil {
			t.Errorf("Failed to create cosine index: %v", err)
		}

		// Drop index for next test
		database.Exec("DROP INDEX IF EXISTS test_embeddings_embedding_idx")

		// Test L2 index
		err = vdb.CreateVectorIndex(context.Background(), "test_embeddings", 3, DistanceEuclidean)
		if err != nil {
			t.Errorf("Failed to create L2 index: %v", err)
		}

		// Drop index for next test
		database.Exec("DROP INDEX IF EXISTS test_embeddings_embedding_idx")

		// Test dot product index
		err = vdb.CreateVectorIndex(context.Background(), "test_embeddings", 3, DistanceDotProduct)
		if err != nil {
			t.Errorf("Failed to create dot product index: %v", err)
		}
	})

	t.Run("InsertAndSearch", func(t *testing.T) {
		vdb, _ := NewPgVectorDB(database, 3, DistanceCosine)

		// Insert test vectors
		testVectors := [][]float32{
			{1.0, 0.0, 0.0},
			{0.0, 1.0, 0.0},
			{0.0, 0.0, 1.0},
			{0.5, 0.5, 0.0},
		}

		// First insert rows with NULL embeddings
		for range testVectors {
			_, err := database.Exec("INSERT INTO test_embeddings (embedding) VALUES (NULL)")
			if err != nil {
				t.Fatalf("Failed to insert row: %v", err)
			}
		}

		// Then update with actual vectors
		for i, vec := range testVectors {
			err := vdb.InsertVector(context.Background(), "test_embeddings", int64(i+1), vec)
			if err != nil {
				t.Fatalf("Failed to insert vector %d: %v", i, err)
			}
		}

		// Search for vector similar to [1, 0, 0]
		query := []float32{1.0, 0.0, 0.0}
		results, err := vdb.SearchKNN(context.Background(), "test_embeddings", query, 2)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}

		// First result should be ID 1 (exact match)
		if results[0].ID != 1 {
			t.Errorf("Expected first result ID 1, got %d", results[0].ID)
		}

		// Distance should be ~0 for exact match
		if results[0].Distance > 0.01 {
			t.Errorf("Expected distance ~0 for exact match, got %f", results[0].Distance)
		}
	})

	t.Run("BatchInsert", func(t *testing.T) {
		// Clean table
		database.Exec("DELETE FROM test_embeddings")

		vdb, _ := NewPgVectorDB(database, 3, DistanceCosine)

		// Insert test vectors
		testVectors := [][]float32{
			{1.0, 0.0, 0.0},
			{0.0, 1.0, 0.0},
			{0.0, 0.0, 1.0},
		}

		// First insert rows
		for range testVectors {
			_, err := database.Exec("INSERT INTO test_embeddings (embedding) VALUES (NULL)")
			if err != nil {
				t.Fatalf("Failed to insert row: %v", err)
			}
		}

		// Batch insert vectors
		ids := []int64{1, 2, 3}
		err := vdb.InsertVectors(context.Background(), "test_embeddings", ids, testVectors)
		if err != nil {
			t.Fatalf("Failed to batch insert: %v", err)
		}

		// Verify all vectors were inserted
		query := []float32{1.0, 1.0, 1.0}
		results, err := vdb.SearchKNN(context.Background(), "test_embeddings", query, 10)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if len(results) != 3 {
			t.Errorf("Expected 3 results after batch insert, got %d", len(results))
		}
	})

	t.Run("DeleteVector", func(t *testing.T) {
		// Clean table
		database.Exec("DELETE FROM test_embeddings")

		vdb, _ := NewPgVectorDB(database, 3, DistanceCosine)

		// Insert test vectors
		testVectors := [][]float32{
			{1.0, 0.0, 0.0},
			{0.0, 1.0, 0.0},
		}

		for i, vec := range testVectors {
			database.Exec("INSERT INTO test_embeddings (embedding) VALUES (NULL)")
			err := vdb.InsertVector(context.Background(), "test_embeddings", int64(i+1), vec)
			if err != nil {
				t.Fatalf("Failed to insert vector: %v", err)
			}
		}

		// Delete first vector
		err := vdb.DeleteVector(context.Background(), "test_embeddings", 1)
		if err != nil {
			t.Fatalf("Failed to delete vector: %v", err)
		}

		// Search should only return second vector
		query := []float32{1.0, 1.0, 1.0}
		results, err := vdb.SearchKNN(context.Background(), "test_embeddings", query, 10)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 result after delete, got %d", len(results))
		}

		if results[0].ID != 2 {
			t.Errorf("Expected remaining result ID 2, got %d", results[0].ID)
		}
	})

	t.Run("DistanceMetrics", func(t *testing.T) {
		testCases := []struct {
			name   string
			metric DistanceMetric
		}{
			{"Cosine", DistanceCosine},
			{"Euclidean", DistanceEuclidean},
			{"DotProduct", DistanceDotProduct},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Clean table
				database.Exec("DELETE FROM test_embeddings")

				vdb, _ := NewPgVectorDB(database, 3, tc.metric)

				// Insert test vector
				database.Exec("INSERT INTO test_embeddings (embedding) VALUES (NULL)")
				vec := []float32{1.0, 0.0, 0.0}
				err := vdb.InsertVector(context.Background(), "test_embeddings", 1, vec)
				if err != nil {
					t.Fatalf("Failed to insert vector: %v", err)
				}

				// Search with same metric
				query := []float32{1.0, 0.0, 0.0}
				results, err := vdb.SearchKNN(context.Background(), "test_embeddings", query, 1)
				if err != nil {
					t.Fatalf("Failed to search with %s: %v", tc.name, err)
				}

				if len(results) != 1 {
					t.Errorf("Expected 1 result, got %d", len(results))
				}
			})
		}
	})
}
