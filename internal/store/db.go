package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection.
type DB struct {
	db *sql.DB
}

// Open opens a SQLite database at the given path.
// Use ":memory:" for an in-memory database.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent read/write
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Migrate runs all database migrations.
func (d *DB) Migrate() error {
	return migrate(d.db)
}

// SqlDB returns the underlying *sql.DB for use by repositories.
func (d *DB) SqlDB() *sql.DB {
	return d.db
}
