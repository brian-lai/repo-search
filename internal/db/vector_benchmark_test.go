package db

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
)

// Benchmark suite comparing brute-force vs pgvector performance

// BenchmarkVectorSearch compares brute-force and pgvector search performance
// across different dataset sizes.
func BenchmarkVectorSearch(b *testing.B) {
	dimensions := 768 // Standard for nomic-embed-text

	// Test with different dataset sizes
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		// Benchmark brute-force
		b.Run(fmt.Sprintf("BruteForce_%d", size), func(b *testing.B) {
			benchmarkBruteForceSearch(b, size, dimensions)
		})

		// Benchmark pgvector (if available)
		if dsn := os.Getenv("POSTGRES_TEST_DSN"); dsn != "" {
			b.Run(fmt.Sprintf("PgVector_%d", size), func(b *testing.B) {
				benchmarkPgVectorSearch(b, dsn, size, dimensions)
			})
		}
	}
}

func benchmarkBruteForceSearch(b *testing.B, numVectors int, dimensions int) {
	// Create brute-force vector DB
	vdb := NewBruteForceVectorDB()
	ctx := context.Background()

	// Create index
	err := vdb.CreateVectorIndex(ctx, "test", dimensions, DistanceCosine)
	if err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}

	// Generate random vectors
	vectors := generateRandomVectors(numVectors, dimensions)
	ids := make([]int64, numVectors)
	for i := range ids {
		ids[i] = int64(i)
	}

	// Insert vectors
	err = vdb.InsertVectors(ctx, "test", ids, vectors)
	if err != nil {
		b.Fatalf("Failed to insert vectors: %v", err)
	}

	// Generate query vector
	queryVector := generateRandomVector(dimensions)

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark search
	for range b.N {
		_, err := vdb.SearchKNN(ctx, "test", queryVector, 10)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func benchmarkPgVectorSearch(b *testing.B, dsn string, numVectors int, dimensions int) {
	// Open PostgreSQL
	cfg := PostgresConfig(dsn)
	database, err := Open(cfg)
	if err != nil {
		b.Skipf("Failed to open PostgreSQL: %v", err)
	}
	defer database.Close()

	// Create test table
	_, err = database.Exec(fmt.Sprintf(`
		DROP TABLE IF EXISTS bench_embeddings_%d;
		CREATE TABLE bench_embeddings_%d (
			id SERIAL PRIMARY KEY,
			embedding vector(%d)
		);
	`, numVectors, numVectors, dimensions))
	if err != nil {
		b.Fatalf("Failed to create table: %v", err)
	}
	defer database.Exec(fmt.Sprintf("DROP TABLE IF EXISTS bench_embeddings_%d", numVectors))

	// Create pgvector DB
	vdb, err := NewPgVectorDB(database, dimensions, DistanceCosine)
	if err != nil {
		b.Fatalf("Failed to create PgVectorDB: %v", err)
	}

	ctx := context.Background()

	// Generate and insert random vectors
	vectors := generateRandomVectors(numVectors, dimensions)

	// Insert rows first
	for range numVectors {
		_, err := database.Exec(fmt.Sprintf("INSERT INTO bench_embeddings_%d (embedding) VALUES (NULL)", numVectors))
		if err != nil {
			b.Fatalf("Failed to insert row: %v", err)
		}
	}

	// Update with actual vectors
	ids := make([]int64, numVectors)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	err = vdb.InsertVectors(ctx, fmt.Sprintf("bench_embeddings_%d", numVectors), ids, vectors)
	if err != nil {
		b.Fatalf("Failed to insert vectors: %v", err)
	}

	// Create vector index
	err = vdb.CreateVectorIndex(ctx, fmt.Sprintf("bench_embeddings_%d", numVectors), dimensions, DistanceCosine)
	if err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}

	// Generate query vector
	queryVector := generateRandomVector(dimensions)

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark search
	for range b.N {
		_, err := vdb.SearchKNN(ctx, fmt.Sprintf("bench_embeddings_%d", numVectors), queryVector, 10)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

// BenchmarkVectorInsertion compares insertion performance
func BenchmarkVectorInsertion(b *testing.B) {
	dimensions := 768
	batchSize := 100

	b.Run("BruteForce_Batch", func(b *testing.B) {
		vdb := NewBruteForceVectorDB()
		ctx := context.Background()
		vdb.CreateVectorIndex(ctx, "test", dimensions, DistanceCosine)

		vectors := generateRandomVectors(batchSize, dimensions)
		ids := make([]int64, batchSize)
		for i := range ids {
			ids[i] = int64(i)
		}

		b.ResetTimer()
		for range b.N {
			vdb.InsertVectors(ctx, "test", ids, vectors)
		}
	})

	if dsn := os.Getenv("POSTGRES_TEST_DSN"); dsn != "" {
		b.Run("PgVector_Batch", func(b *testing.B) {
			cfg := PostgresConfig(dsn)
			database, err := Open(cfg)
			if err != nil {
				b.Skipf("Failed to open PostgreSQL: %v", err)
			}
			defer database.Close()

			// Create test table
			_, err = database.Exec(fmt.Sprintf(`
				DROP TABLE IF EXISTS bench_insert;
				CREATE TABLE bench_insert (
					id SERIAL PRIMARY KEY,
					embedding vector(%d)
				);
			`, dimensions))
			if err != nil {
				b.Fatalf("Failed to create table: %v", err)
			}
			defer database.Exec("DROP TABLE IF EXISTS bench_insert")

			vdb, err := NewPgVectorDB(database, dimensions, DistanceCosine)
			if err != nil {
				b.Fatalf("Failed to create PgVectorDB: %v", err)
			}

			ctx := context.Background()
			vectors := generateRandomVectors(batchSize, dimensions)

			b.ResetTimer()
			for i := range b.N {
				// Insert rows
				for range batchSize {
					_, err := database.Exec("INSERT INTO bench_insert (embedding) VALUES (NULL)")
					if err != nil {
						b.Fatalf("Failed to insert row: %v", err)
					}
				}

				// Update with vectors
				ids := make([]int64, batchSize)
				for j := range ids {
					ids[j] = int64(i*batchSize + j + 1)
				}
				err = vdb.InsertVectors(ctx, "bench_insert", ids, vectors)
				if err != nil {
					b.Fatalf("Failed to insert vectors: %v", err)
				}
			}
		})
	}
}

// Helper functions

func generateRandomVector(dimensions int) []float32 {
	vector := make([]float32, dimensions)
	for i := range vector {
		vector[i] = rand.Float32()
	}
	// Normalize
	var norm float32
	for _, v := range vector {
		norm += v * v
	}
	norm = float32(1.0 / sqrt32(norm))
	for i := range vector {
		vector[i] *= norm
	}
	return vector
}

func generateRandomVectors(count, dimensions int) [][]float32 {
	vectors := make([][]float32, count)
	for i := range vectors {
		vectors[i] = generateRandomVector(dimensions)
	}
	return vectors
}
