package db

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
)

// TestSearchConsistency verifies that brute-force and pgvector produce
// consistent search results.
func TestSearchConsistency(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set, skipping consistency test")
	}

	dimensions := 768
	numVectors := 1000
	k := 10

	// Generate test data
	vectors := generateRandomVectors(numVectors, dimensions)
	queryVector := generateRandomVector(dimensions)

	// Test with brute-force
	t.Run("BruteForce", func(t *testing.T) {
		vdb := NewBruteForceVectorDB()
		ctx := context.Background()

		err := vdb.CreateVectorIndex(ctx, "test", dimensions, DistanceCosine)
		if err != nil {
			t.Fatalf("Failed to create index: %v", err)
		}

		ids := make([]int64, numVectors)
		for i := range ids {
			ids[i] = int64(i)
		}

		err = vdb.InsertVectors(ctx, "test", ids, vectors)
		if err != nil {
			t.Fatalf("Failed to insert vectors: %v", err)
		}

		results, err := vdb.SearchKNN(ctx, "test", queryVector, k)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != k {
			t.Errorf("Expected %d results, got %d", k, len(results))
		}

		// Verify results are sorted by distance
		for i := 1; i < len(results); i++ {
			if results[i].Distance < results[i-1].Distance {
				t.Errorf("Results not sorted: result[%d].Distance=%f < result[%d].Distance=%f",
					i, results[i].Distance, i-1, results[i-1].Distance)
			}
		}
	})

	// Test with pgvector
	t.Run("PgVector", func(t *testing.T) {
		cfg := PostgresConfig(dsn)
		database, err := Open(cfg)
		if err != nil {
			t.Fatalf("Failed to open PostgreSQL: %v", err)
		}
		defer database.Close()

		// Create test table
		_, err = database.Exec(fmt.Sprintf(`
			DROP TABLE IF EXISTS consistency_test;
			CREATE TABLE consistency_test (
				id SERIAL PRIMARY KEY,
				embedding vector(%d)
			);
		`, dimensions))
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}
		defer database.Exec("DROP TABLE IF EXISTS consistency_test")

		vdb, err := NewPgVectorDB(database, dimensions, DistanceCosine)
		if err != nil {
			t.Fatalf("Failed to create PgVectorDB: %v", err)
		}

		ctx := context.Background()

		// Insert rows
		for range numVectors {
			_, err := database.Exec("INSERT INTO consistency_test (embedding) VALUES (NULL)")
			if err != nil {
				t.Fatalf("Failed to insert row: %v", err)
			}
		}

		// Insert vectors
		ids := make([]int64, numVectors)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		err = vdb.InsertVectors(ctx, "consistency_test", ids, vectors)
		if err != nil {
			t.Fatalf("Failed to insert vectors: %v", err)
		}

		// Create index
		err = vdb.CreateVectorIndex(ctx, "consistency_test", dimensions, DistanceCosine)
		if err != nil {
			t.Fatalf("Failed to create index: %v", err)
		}

		results, err := vdb.SearchKNN(ctx, "consistency_test", queryVector, k)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != k {
			t.Errorf("Expected %d results, got %d", k, len(results))
		}

		// Verify results are sorted by distance
		for i := 1; i < len(results); i++ {
			if results[i].Distance < results[i-1].Distance {
				t.Errorf("Results not sorted: result[%d].Distance=%f < result[%d].Distance=%f",
					i, results[i].Distance, i-1, results[i-1].Distance)
			}
		}
	})

	// Compare results between backends
	t.Run("CompareBackends", func(t *testing.T) {
		// Run brute-force search
		bruteForceVDB := NewBruteForceVectorDB()
		ctx := context.Background()

		err := bruteForceVDB.CreateVectorIndex(ctx, "test", dimensions, DistanceCosine)
		if err != nil {
			t.Fatalf("Failed to create brute-force index: %v", err)
		}

		ids := make([]int64, numVectors)
		for i := range ids {
			ids[i] = int64(i)
		}
		err = bruteForceVDB.InsertVectors(ctx, "test", ids, vectors)
		if err != nil {
			t.Fatalf("Failed to insert brute-force vectors: %v", err)
		}

		bruteForceResults, err := bruteForceVDB.SearchKNN(ctx, "test", queryVector, k)
		if err != nil {
			t.Fatalf("Brute-force search failed: %v", err)
		}

		// Run pgvector search
		cfg := PostgresConfig(dsn)
		database, err := Open(cfg)
		if err != nil {
			t.Fatalf("Failed to open PostgreSQL: %v", err)
		}
		defer database.Close()

		_, err = database.Exec(fmt.Sprintf(`
			DROP TABLE IF EXISTS compare_test;
			CREATE TABLE compare_test (
				id SERIAL PRIMARY KEY,
				embedding vector(%d)
			);
		`, dimensions))
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}
		defer database.Exec("DROP TABLE IF EXISTS compare_test")

		pgVDB, err := NewPgVectorDB(database, dimensions, DistanceCosine)
		if err != nil {
			t.Fatalf("Failed to create PgVectorDB: %v", err)
		}

		// Insert rows and vectors
		for range numVectors {
			database.Exec("INSERT INTO compare_test (embedding) VALUES (NULL)")
		}
		pgIDs := make([]int64, numVectors)
		for i := range pgIDs {
			pgIDs[i] = int64(i + 1)
		}
		err = pgVDB.InsertVectors(ctx, "compare_test", pgIDs, vectors)
		if err != nil {
			t.Fatalf("Failed to insert pgvector vectors: %v", err)
		}

		err = pgVDB.CreateVectorIndex(ctx, "compare_test", dimensions, DistanceCosine)
		if err != nil {
			t.Fatalf("Failed to create pgvector index: %v", err)
		}

		pgResults, err := pgVDB.SearchKNN(ctx, "compare_test", queryVector, k)
		if err != nil {
			t.Fatalf("PgVector search failed: %v", err)
		}

		// Calculate overlap in top-k results
		overlap := calculateOverlap(bruteForceResults, pgResults)
		overlapPercent := float64(overlap) / float64(k) * 100

		t.Logf("Top-%d overlap: %d/%d (%.1f%%)", k, overlap, k, overlapPercent)

		// Success criteria: ≥ 90% overlap in top-10
		if overlapPercent < 90.0 {
			t.Errorf("Insufficient overlap: %.1f%% (expected ≥ 90%%)", overlapPercent)
		}

		// Compare distances (should be very close)
		for i := range min(len(bruteForceResults), len(pgResults)) {
			bf := bruteForceResults[i]
			pg := pgResults[i]

			// Distances should be within 1% due to floating point precision
			distDiff := math.Abs(float64(bf.Distance - pg.Distance))
			maxDist := math.Max(float64(bf.Distance), float64(pg.Distance))
			if maxDist > 0 && distDiff/maxDist > 0.01 {
				t.Logf("Distance mismatch at position %d: brute-force=%.6f, pgvector=%.6f",
					i, bf.Distance, pg.Distance)
			}
		}
	})
}

// TestLargeDataset tests performance with large datasets
func TestLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set, skipping large dataset test")
	}

	dimensions := 768
	sizes := []int{10000, 100000}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			// Only test pgvector for large datasets (brute-force would be too slow)
			cfg := PostgresConfig(dsn)
			database, err := Open(cfg)
			if err != nil {
				t.Fatalf("Failed to open PostgreSQL: %v", err)
			}
			defer database.Close()

			tableName := fmt.Sprintf("large_test_%d", size)
			_, err = database.Exec(fmt.Sprintf(`
				DROP TABLE IF EXISTS %s;
				CREATE TABLE %s (
					id SERIAL PRIMARY KEY,
					embedding vector(%d)
				);
			`, tableName, tableName, dimensions))
			if err != nil {
				t.Fatalf("Failed to create table: %v", err)
			}
			defer database.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))

			vdb, err := NewPgVectorDB(database, dimensions, DistanceCosine)
			if err != nil {
				t.Fatalf("Failed to create PgVectorDB: %v", err)
			}

			ctx := context.Background()

			// Insert in batches to avoid memory issues
			batchSize := 1000
			t.Logf("Inserting %d vectors in batches of %d...", size, batchSize)

			for batch := 0; batch < size/batchSize; batch++ {
				// Insert rows
				for range batchSize {
					_, err := database.Exec(fmt.Sprintf("INSERT INTO %s (embedding) VALUES (NULL)", tableName))
					if err != nil {
						t.Fatalf("Failed to insert row: %v", err)
					}
				}

				// Generate and insert vectors
				vectors := generateRandomVectors(batchSize, dimensions)
				ids := make([]int64, batchSize)
				for i := range ids {
					ids[i] = int64(batch*batchSize + i + 1)
				}

				err = vdb.InsertVectors(ctx, tableName, ids, vectors)
				if err != nil {
					t.Fatalf("Failed to insert vectors: %v", err)
				}

				if (batch+1)%10 == 0 {
					t.Logf("Inserted %d/%d vectors", (batch+1)*batchSize, size)
				}
			}

			t.Log("Creating vector index...")
			err = vdb.CreateVectorIndex(ctx, tableName, dimensions, DistanceCosine)
			if err != nil {
				t.Fatalf("Failed to create index: %v", err)
			}

			t.Log("Performing search...")
			queryVector := generateRandomVector(dimensions)
			results, err := vdb.SearchKNN(ctx, tableName, queryVector, 10)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			if len(results) != 10 {
				t.Errorf("Expected 10 results, got %d", len(results))
			}

			t.Logf("Search completed successfully with %d vectors", size)
		})
	}
}

// calculateOverlap counts how many IDs appear in both result sets
func calculateOverlap(a, b []VectorSearchResult) int {
	idSet := make(map[int64]bool)
	for _, r := range a {
		idSet[r.ID] = true
	}

	overlap := 0
	for _, r := range b {
		if idSet[r.ID] {
			overlap++
		}
	}
	return overlap
}
