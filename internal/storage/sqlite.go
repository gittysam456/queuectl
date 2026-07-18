package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Storage handles all database operations.
type Storage struct {
	db *sql.DB
}

// NewStorage initializes a new SQLite storage instance and runs migrations.
func NewStorage(dbPath string) (*Storage, error) {
	// Enable WAL mode for better concurrency and foreign keys for integrity
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Ping to ensure connection is valid
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &Storage{db: db}

	if err := s.runMigrations(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// DB returns the underlying sql.DB instance.
func (s *Storage) DB() *sql.DB {
	return s.db
}

func (s *Storage) runMigrations() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		command TEXT NOT NULL,
		state TEXT NOT NULL, -- pending, processing, completed, failed, dead
		attempts INTEGER NOT NULL DEFAULT 0,
		max_retries INTEGER NOT NULL DEFAULT 3,
		output TEXT DEFAULT '',
		priority INTEGER NOT NULL DEFAULT 0,
		timeout INTEGER NOT NULL DEFAULT 0,
		execution_time_ms INTEGER NOT NULL DEFAULT 0,
		run_after DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_state_run_after ON jobs(state, run_after);

	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}
	
	// Seamless upgrade for existing databases
	_, _ = s.db.Exec(`ALTER TABLE jobs ADD COLUMN output TEXT DEFAULT '';`)
	_, _ = s.db.Exec(`ALTER TABLE jobs ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;`)
	_, _ = s.db.Exec(`ALTER TABLE jobs ADD COLUMN timeout INTEGER NOT NULL DEFAULT 0;`)
	_, _ = s.db.Exec(`ALTER TABLE jobs ADD COLUMN execution_time_ms INTEGER NOT NULL DEFAULT 0;`)
	
	return nil
}
