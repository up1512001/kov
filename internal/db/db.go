// Package db provides the SQLite database layer for kov.
// It uses WAL mode for crash recovery and a single-writer queue
// to prevent SQLITE_BUSY errors.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver
)

// DB wraps two SQLite connections: one for reads (concurrent) and
// one for writes (serialized via channel queue).
type DB struct {
	readDB  *sql.DB
	writeDB *sql.DB
	writeCh chan writeOp
	done    chan struct{}
	closeOnce sync.Once
	path    string
	logger  *slog.Logger
}

// writeOp represents a queued write operation.
type writeOp struct {
	fn     func(tx *sql.Tx) error
	result chan error
}

// Open creates or opens a SQLite database at the given path.
// It configures WAL mode, sets pragmas for performance and safety,
// and starts the background write loop.
func Open(path string, logger *slog.Logger) (*DB, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	// Connection string — modernc.org/sqlite uses simple file paths
	dsn := fmt.Sprintf("file:%s", path)

	// Read connection pool (concurrent readers)
	readDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening read db: %w", err)
	}
	readDB.SetMaxOpenConns(4)
	readDB.SetMaxIdleConns(2)

	// Write connection (single writer)
	writeDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		readDB.Close()
		return nil, fmt.Errorf("opening write db: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)

	// Set pragmas on both connections via SQL
	// modernc.org/sqlite doesn't support DSN-style pragmas
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := writeDB.Exec(p); err != nil {
			readDB.Close()
			writeDB.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
		if _, err := readDB.Exec(p); err != nil {
			readDB.Close()
			writeDB.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	// Verify connections work
	if err := readDB.Ping(); err != nil {
		readDB.Close()
		writeDB.Close()
		return nil, fmt.Errorf("pinging db: %w", err)
	}

	db := &DB{
		readDB:  readDB,
		writeDB: writeDB,
		writeCh: make(chan writeOp, 64),
		done:    make(chan struct{}),
		path:    path,
		logger:  logger,
	}

	// Start background write loop
	go db.writeLoop()

	// Run migrations
	if err := db.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	logger.Info("database opened", slog.String("path", path))
	return db, nil
}

// Write enqueues a write operation and waits for it to complete.
// All writes are serialized through a single connection to prevent
// SQLITE_BUSY errors.
func (db *DB) Write(ctx context.Context, fn func(tx *sql.Tx) error) error {
	result := make(chan error, 1)
	op := writeOp{fn: fn, result: result}

	select {
	case db.writeCh <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-db.done:
		return fmt.Errorf("database closed")
	}

	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReadDB returns the read-only connection pool for direct queries.
func (db *DB) ReadDB() *sql.DB {
	return db.readDB
}

// writeLoop processes write operations sequentially.
func (db *DB) writeLoop() {
	for op := range db.writeCh {
		tx, err := db.writeDB.Begin()
		if err != nil {
			op.result <- fmt.Errorf("beginning transaction: %w", err)
			continue
		}

		if err := op.fn(tx); err != nil {
			tx.Rollback()
			op.result <- err
			continue
		}

		if err := tx.Commit(); err != nil {
			op.result <- fmt.Errorf("committing transaction: %w", err)
			continue
		}

		op.result <- nil
	}
}

// Close shuts down the database connections.
func (db *DB) Close() error {
	var err error
	db.closeOnce.Do(func() {
		close(db.done)
		close(db.writeCh)
		// Wait briefly for pending writes
		time.Sleep(50 * time.Millisecond)
		if e := db.writeDB.Close(); e != nil {
			err = e
		}
		if e := db.readDB.Close(); e != nil && err == nil {
			err = e
		}
		db.logger.Info("database closed")
	})
	return err
}

// Path returns the database file path.
func (db *DB) Path() string {
	return db.path
}
