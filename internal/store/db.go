// Package store is the local persistence layer for VayuMail. It wraps a
// pure-Go SQLite database (modernc.org/sqlite) opened in WAL mode and
// exposes typed CRUD interfaces for accounts, folders, messages, the
// outbox queue, and FTS5 search.
//
// Constitutional Rule 6: this package never stores credentials. Account
// rows carry a keystore alias only; the secret lives in the platform
// keystore.
//
// Constitutional Rule 4: this package never imports Gio or any platform
// package. It is importable by a CLI, a server plugin, or a desktop
// client without modification.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registers "sqlite"
)

// unixUTC converts a stored Unix timestamp to a UTC time.Time.
func unixUTC(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

// DB wraps the SQLite handle. All store operations hang off this type so
// that callers hold exactly one dependency.
type DB struct {
	sql *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path, applies
// connection pragmas, and runs pending schema migrations. Use ":memory:"
// for an in-memory database in tests.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// modernc.org/sqlite serializes access per connection; a single
	// connection avoids SQLITE_BUSY between the sync goroutines and
	// keeps WAL checkpointing predictable on mobile storage.
	sqlDB.SetMaxOpenConns(1)

	db := &DB{sql: sqlDB}
	if err := db.migrate(ctx); err != nil {
		closeErr := sqlDB.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("store: migrate: %w (close: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return db, nil
}

// Close closes the underlying database handle.
func (db *DB) Close() error {
	if err := db.sql.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

// tx runs fn inside a transaction, committing on nil and rolling back on
// error. It is the single write path for multi-statement operations.
func (db *DB) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	t, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	if err := fn(t); err != nil {
		if rbErr := t.Rollback(); rbErr != nil {
			return fmt.Errorf("%w (rollback: %v)", err, rbErr)
		}
		return err
	}
	if err := t.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}
