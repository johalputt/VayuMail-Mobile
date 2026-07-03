package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// migrations is the ordered list of schema versions. Index i migrates the
// database to version i+1. Migrations are append-only: never edit a shipped
// entry, always add a new one.
var migrations = []string{
	migrationV1,
	migrationV2,
	migrationV3,
}

const migrationV1 = `
CREATE TABLE accounts (
  id             INTEGER PRIMARY KEY,
  display_name   TEXT NOT NULL,
  email_address  TEXT NOT NULL UNIQUE,
  imap_host      TEXT NOT NULL,
  imap_port      INTEGER NOT NULL,
  imap_tls       TEXT NOT NULL CHECK(imap_tls IN ('tls','starttls')),
  smtp_host      TEXT NOT NULL,
  smtp_port      INTEGER NOT NULL,
  smtp_tls       TEXT NOT NULL CHECK(smtp_tls IN ('tls','starttls')),
  username       TEXT NOT NULL,
  keystore_alias TEXT NOT NULL,
  created_at     INTEGER NOT NULL
);

CREATE TABLE folders (
  id             INTEGER PRIMARY KEY,
  account_id     INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  full_name      TEXT NOT NULL,
  delimiter      TEXT,
  uid_validity   INTEGER,
  highest_modseq INTEGER,
  is_inbox       INTEGER NOT NULL DEFAULT 0,
  is_sent        INTEGER NOT NULL DEFAULT 0,
  is_drafts      INTEGER NOT NULL DEFAULT 0,
  is_trash       INTEGER NOT NULL DEFAULT 0,
  is_archive     INTEGER NOT NULL DEFAULT 0,
  UNIQUE(account_id, full_name)
);

CREATE TABLE messages (
  id              INTEGER PRIMARY KEY,
  account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  folder_id       INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
  uid             INTEGER NOT NULL,
  thread_id       TEXT,
  message_id      TEXT,
  in_reply_to     TEXT,
  from_addr       TEXT NOT NULL,
  from_name       TEXT,
  to_addrs        TEXT NOT NULL,
  cc_addrs        TEXT,
  subject         TEXT,
  snippet         TEXT,
  body_text       TEXT,
  body_html       TEXT,
  has_attachments INTEGER NOT NULL DEFAULT 0,
  pgp_status      TEXT CHECK(pgp_status IN (NULL,'signed','encrypted','signed+encrypted')),
  is_read         INTEGER NOT NULL DEFAULT 0,
  is_flagged      INTEGER NOT NULL DEFAULT 0,
  is_deleted      INTEGER NOT NULL DEFAULT 0,
  date            INTEGER NOT NULL,
  size_bytes      INTEGER,
  flags           TEXT,
  UNIQUE(account_id, folder_id, uid)
);

CREATE INDEX idx_messages_thread ON messages(thread_id);
CREATE INDEX idx_messages_date   ON messages(account_id, folder_id, date DESC);
CREATE INDEX idx_messages_unread ON messages(account_id, is_read) WHERE is_read = 0;

CREATE TABLE outbox (
  id          INTEGER PRIMARY KEY,
  account_id  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  raw_message BLOB NOT NULL,
  retry_count INTEGER NOT NULL DEFAULT 0,
  max_retries INTEGER NOT NULL DEFAULT 5,
  next_attempt INTEGER NOT NULL,
  last_error  TEXT,
  queued_at   INTEGER NOT NULL
);

CREATE VIRTUAL TABLE messages_fts USING fts5(
  from_addr, from_name, subject, snippet,
  content=messages, content_rowid=id
);

CREATE TRIGGER messages_fts_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid,from_addr,from_name,subject,snippet)
  VALUES (new.id,new.from_addr,new.from_name,new.subject,new.snippet);
END;
CREATE TRIGGER messages_fts_ad AFTER DELETE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts,rowid,from_addr,from_name,subject,snippet)
  VALUES ('delete',old.id,old.from_addr,old.from_name,old.subject,old.snippet);
END;
CREATE TRIGGER messages_fts_au AFTER UPDATE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts,rowid,from_addr,from_name,subject,snippet)
  VALUES ('delete',old.id,old.from_addr,old.from_name,old.subject,old.snippet);
  INSERT INTO messages_fts(rowid,from_addr,from_name,subject,snippet)
  VALUES (new.id,new.from_addr,new.from_name,new.subject,new.snippet);
END;
`

// migrationV2 (ADR-0007) adds local-intelligence columns, per-account
// TLS pinning, PGP key storage, and rebuilds the FTS index to cover
// message bodies.
const migrationV2 = `
ALTER TABLE messages ADD COLUMN has_trackers INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN is_list INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN list_unsubscribe TEXT;
ALTER TABLE messages ADD COLUMN snooze_until INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN attachments TEXT;
ALTER TABLE accounts ADD COLUMN pinned_spki TEXT;

CREATE TABLE pgp_keys (
  id           INTEGER PRIMARY KEY,
  fingerprint  TEXT NOT NULL UNIQUE,
  email        TEXT NOT NULL,
  armored      TEXT NOT NULL,
  is_private   INTEGER NOT NULL DEFAULT 0,
  trust_level  INTEGER NOT NULL DEFAULT 0,
  added_at     INTEGER NOT NULL
);
CREATE INDEX idx_pgp_keys_email ON pgp_keys(email);

DROP TRIGGER messages_fts_ai;
DROP TRIGGER messages_fts_ad;
DROP TRIGGER messages_fts_au;
DROP TABLE messages_fts;

CREATE VIRTUAL TABLE messages_fts USING fts5(
  from_addr, from_name, subject, snippet, body_text,
  content=messages, content_rowid=id
);
CREATE TRIGGER messages_fts_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid,from_addr,from_name,subject,snippet,body_text)
  VALUES (new.id,new.from_addr,new.from_name,new.subject,new.snippet,new.body_text);
END;
CREATE TRIGGER messages_fts_ad AFTER DELETE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts,rowid,from_addr,from_name,subject,snippet,body_text)
  VALUES ('delete',old.id,old.from_addr,old.from_name,old.subject,old.snippet,old.body_text);
END;
CREATE TRIGGER messages_fts_au AFTER UPDATE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts,rowid,from_addr,from_name,subject,snippet,body_text)
  VALUES ('delete',old.id,old.from_addr,old.from_name,old.subject,old.snippet,old.body_text);
  INSERT INTO messages_fts(rowid,from_addr,from_name,subject,snippet,body_text)
  VALUES (new.id,new.from_addr,new.from_name,new.subject,new.snippet,new.body_text);
END;
INSERT INTO messages_fts(messages_fts) VALUES ('rebuild');
`

// migrationV3 adds a small key/value settings table for app-wide
// preferences such as the VayuPress PGP key-directory URL.
const migrationV3 = `
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

// migrate brings the schema to the newest version, applying each pending
// migration in its own transaction and recording the version reached.
func (db *DB) migrate(ctx context.Context) error {
	_, err := db.sql.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	current, err := db.schemaVersion(ctx)
	if err != nil {
		return err
	}
	for v := current; v < len(migrations); v++ {
		stmt := migrations[v]
		next := v + 1
		err := db.tx(ctx, func(t *sql.Tx) error {
			if _, err := t.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("apply migration v%d: %w", next, err)
			}
			if _, err := t.ExecContext(ctx,
				`DELETE FROM schema_version`); err != nil {
				return fmt.Errorf("clear schema_version: %w", err)
			}
			if _, err := t.ExecContext(ctx,
				`INSERT INTO schema_version(version) VALUES (?)`, next); err != nil {
				return fmt.Errorf("record schema version v%d: %w", next, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// schemaVersion reads the recorded schema version, returning 0 for a fresh
// database.
func (db *DB) schemaVersion(ctx context.Context) (int, error) {
	var v int
	err := db.sql.QueryRowContext(ctx,
		`SELECT version FROM schema_version LIMIT 1`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return v, nil
}
