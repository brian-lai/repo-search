package db

import "fmt"

// Open opens a database connection using the driver specified in config.
// Currently only DriverModernc is implemented.
func Open(cfg Config) (DB, error) {
	switch cfg.Driver {
	case DriverModernc, "": // Default to modernc
		return OpenModernc(cfg)

	case DriverNcruces:
		// TODO: Implement ncruces driver with sqlite-vec support
		// This will enable native vector search via vec0 virtual tables.
		// See: https://github.com/ncruces/go-sqlite3
		// See: https://github.com/asg017/sqlite-vec-go-bindings
		return nil, fmt.Errorf("ncruces driver not yet implemented (requires sqlite-vec integration)")

	case DriverMattn:
		// TODO: Implement mattn driver with CGO
		// This requires CGO and a C compiler but provides full extension support.
		// See: https://github.com/mattn/go-sqlite3
		return nil, fmt.Errorf("mattn driver not yet implemented (requires CGO)")

	default:
		return nil, fmt.Errorf("unknown database driver: %s", cfg.Driver)
	}
}

// MustOpen opens a database connection and panics on error.
// Useful for testing and simple scripts.
func MustOpen(cfg Config) DB {
	db, err := Open(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to open database: %v", err))
	}
	return db
}

// OpenExtended opens a database with vector search support if available.
// Falls back to standard DB if the driver doesn't support extensions.
func OpenExtended(cfg Config) (DB, bool, error) {
	db, err := Open(cfg)
	if err != nil {
		return nil, false, err
	}

	// Check if the DB supports vector operations
	if ext, ok := db.(ExtendedDB); ok && ext.VectorSearchAvailable() {
		return db, true, nil
	}

	return db, false, nil
}
