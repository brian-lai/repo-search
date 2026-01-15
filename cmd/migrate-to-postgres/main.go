package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"repo-search/internal/config"
	"repo-search/internal/db"
	"repo-search/internal/embedding"
)

const (
	defaultSQLitePath = ".repo_search/symbols.db"
)

func main() {
	// Parse flags
	sqlitePath := flag.String("source", defaultSQLitePath, "SQLite database path")
	batchSize := flag.Int("batch", 1000, "Number of embeddings to migrate per batch")
	skipExisting := flag.Bool("skip-existing", true, "Skip embeddings that already exist in PostgreSQL")
	dropTarget := flag.Bool("drop-target", false, "Drop existing PostgreSQL tables before migration")
	dryRun := flag.Bool("dry-run", false, "Perform validation without migrating data")
	validate := flag.Bool("validate", true, "Validate migration after completion")
	sampleSize := flag.Int("sample-size", 100, "Number of embeddings to sample for validation")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Migrate embeddings from SQLite to PostgreSQL.\n\n")
		fmt.Fprintf(os.Stderr, "Environment variables:\n")
		fmt.Fprintf(os.Stderr, "  REPO_SEARCH_DB_TYPE=postgres\n")
		fmt.Fprintf(os.Stderr, "  REPO_SEARCH_DB_DSN=postgres://user:pass@localhost:5432/dbname\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Validate PostgreSQL configuration
	pgConfig := config.LoadDatabaseConfigFromEnv()
	if pgConfig.Type != db.DatabasePostgres {
		fmt.Fprintf(os.Stderr, "Error: PostgreSQL not configured\n\n")
		fmt.Fprintf(os.Stderr, "Please set environment variables:\n")
		fmt.Fprintf(os.Stderr, "  export REPO_SEARCH_DB_TYPE=postgres\n")
		fmt.Fprintf(os.Stderr, "  export REPO_SEARCH_DB_DSN=postgres://user:pass@localhost:5432/dbname\n\n")
		os.Exit(1)
	}

	if pgConfig.DSN == "" {
		fmt.Fprintf(os.Stderr, "Error: REPO_SEARCH_DB_DSN not set\n")
		os.Exit(1)
	}

	// Check if SQLite database exists
	if _, err := os.Stat(*sqlitePath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: SQLite database not found: %s\n", *sqlitePath)
		fmt.Fprintf(os.Stderr, "Have you run 'repo-search embed' yet?\n")
		os.Exit(1)
	}

	fmt.Println("PostgreSQL Migration Tool")
	fmt.Println("==========================")
	fmt.Println()
	fmt.Printf("Source:      SQLite (%s)\n", *sqlitePath)
	fmt.Printf("Target:      %s\n", pgConfig.String())
	fmt.Printf("Batch size:  %d\n", *batchSize)
	fmt.Printf("Skip exists: %v\n", *skipExisting)
	fmt.Printf("Drop target: %v\n", *dropTarget)
	fmt.Printf("Dry run:     %v\n", *dryRun)
	fmt.Println()

	// Confirm if drop-target is enabled
	if *dropTarget && !*dryRun {
		fmt.Print("WARNING: This will delete all existing data in PostgreSQL!\n")
		fmt.Print("Type 'yes' to continue: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Migration cancelled")
			os.Exit(0)
		}
		fmt.Println()
	}

	// Open source database (SQLite)
	sqliteCfg := db.DefaultConfig(*sqlitePath)
	sqliteCfg.VectorDimensions = pgConfig.VectorDimensions
	sourceDB, err := db.Open(sqliteCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening SQLite: %v\n", err)
		os.Exit(1)
	}
	defer sourceDB.Close()

	sourceStore, err := embedding.NewEmbeddingStore(sourceDB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating source embedding store: %v\n", err)
		os.Exit(1)
	}

	// Open target database (PostgreSQL)
	targetCfg := pgConfig.ToDBConfig()
	targetDB, err := db.Open(targetCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening PostgreSQL: %v\n", err)
		fmt.Fprintf(os.Stderr, "Is PostgreSQL running? Check: docker-compose ps\n")
		os.Exit(1)
	}
	defer targetDB.Close()

	// Get SQL dialect for PostgreSQL from config
	dialect := targetCfg.Dialect()
	targetStore, err := embedding.NewEmbeddingStoreWithDialect(targetDB, dialect)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating target embedding store: %v\n", err)
		os.Exit(1)
	}

	// Check source count
	sourceCount, err := sourceStore.Count()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error counting source embeddings: %v\n", err)
		os.Exit(1)
	}

	if sourceCount == 0 {
		fmt.Println("No embeddings found in SQLite database")
		fmt.Println("Run 'repo-search embed' to generate embeddings first")
		os.Exit(0)
	}

	fmt.Printf("Found %d embeddings in SQLite\n", sourceCount)
	fmt.Println()

	if *dryRun {
		fmt.Println("Dry run mode - no data will be migrated")
		os.Exit(0)
	}

	// Prepare migration options
	opts := embedding.MigrationOptions{
		BatchSize:    *batchSize,
		SkipExisting: *skipExisting,
		DropTarget:   *dropTarget,
		DryRun:       *dryRun,
	}

	// Progress tracking
	startTime := time.Now()
	lastUpdate := time.Now()
	progressBar := func(progress embedding.MigrationProgress) {
		now := time.Now()
		if now.Sub(lastUpdate) < 100*time.Millisecond && progress.MigratedEmbeddings < progress.TotalEmbeddings {
			return // Rate limit updates
		}
		lastUpdate = now

		percent := float64(progress.MigratedEmbeddings+progress.SkippedEmbeddings) / float64(progress.TotalEmbeddings) * 100
		elapsed := now.Sub(startTime)
		rate := float64(progress.MigratedEmbeddings) / elapsed.Seconds()

		fmt.Printf("\rProgress: %d/%d (%.1f%%) | Migrated: %d | Skipped: %d | Rate: %.0f/s | %s",
			progress.MigratedEmbeddings+progress.SkippedEmbeddings,
			progress.TotalEmbeddings,
			percent,
			progress.MigratedEmbeddings,
			progress.SkippedEmbeddings,
			rate,
			progress.CurrentFile,
		)

		if progress.MigratedEmbeddings+progress.SkippedEmbeddings >= progress.TotalEmbeddings {
			fmt.Println() // New line at completion
		}
	}

	// Run migration with vector index creation
	fmt.Println("Starting migration...")
	ctx := context.Background()
	if err := embedding.MigrateDatabaseWithVectorIndex(ctx, sourceStore, targetStore, opts, progressBar); err != nil {
		fmt.Fprintf(os.Stderr, "\n\nError during migration: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(startTime)
	fmt.Printf("\nMigration completed in %s\n", duration.Round(time.Millisecond))

	// Validate if requested
	if *validate {
		fmt.Println()
		fmt.Println("Validating migration...")
		if err := embedding.ValidateMigration(sourceStore, targetStore, *sampleSize); err != nil {
			fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Validation passed")
	}

	// Show final statistics
	targetCount, err := targetStore.Count()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error counting target embeddings: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Migration Summary")
	fmt.Println("=================")
	fmt.Printf("Source (SQLite):     %d embeddings\n", sourceCount)
	fmt.Printf("Target (PostgreSQL): %d embeddings\n", targetCount)
	fmt.Printf("Duration:            %s\n", duration.Round(time.Millisecond))
	fmt.Printf("Rate:                %.0f embeddings/sec\n", float64(sourceCount)/duration.Seconds())
	fmt.Println()
	fmt.Println("✓ Migration successful!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Keep PostgreSQL environment variables set")
	fmt.Println("  2. Test semantic search: repo-search (in MCP mode)")
	fmt.Println("  3. Optional: Backup SQLite database and remove it")
}
