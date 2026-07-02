package store

import (
	"context"
	"fmt"
	"time"
)

// ListUnifiedInbox returns the newest messages across every account's
// inbox — the "All inboxes" view. Snoozed and deleted rows are excluded.
func (db *DB) ListUnifiedInbox(ctx context.Context, offset, limit int) ([]Message, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT `+prefixedMessageCols+` FROM messages m
		JOIN folders f ON f.id = m.folder_id
		WHERE f.is_inbox = 1 AND m.is_deleted = 0
			AND m.snooze_until <= unixepoch()
		ORDER BY m.date DESC, m.uid DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("store: unified inbox: %w", err)
	}
	return collectPrefixedMessages(rows)
}

// UnifiedUnreadCount counts unread mail across every inbox.
func (db *DB) UnifiedUnreadCount(ctx context.Context) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM messages m
		JOIN folders f ON f.id = m.folder_id
		WHERE f.is_inbox = 1 AND m.is_read = 0 AND m.is_deleted = 0`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: unified unread: %w", err)
	}
	return n, nil
}

// SetSnooze hides a message from lists until t; the zero time unsnoozes.
func (db *DB) SetSnooze(ctx context.Context, id int64, until time.Time) error {
	var v int64
	if !until.IsZero() {
		v = until.Unix()
	}
	res, err := db.sql.ExecContext(ctx,
		`UPDATE messages SET snooze_until = ? WHERE id = ?`, v, id)
	if err != nil {
		return fmt.Errorf("store: snooze: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: snooze: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// PGPKey is one stored OpenPGP key. Private keys are stored armored but
// their armored form is itself kept in the sealed keystore, not here —
// this table holds public material plus metadata (ADR-0007).
type PGPKey struct {
	ID          int64
	Fingerprint string
	Email       string
	Armored     string
	IsPrivate   bool
	TrustLevel  int
	AddedAt     time.Time
}

// UpsertPGPKey stores or refreshes a key by fingerprint.
func (db *DB) UpsertPGPKey(ctx context.Context, k *PGPKey) error {
	if k.AddedAt.IsZero() {
		k.AddedAt = time.Now().UTC()
	}
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO pgp_keys (fingerprint, email, armored, is_private,
			trust_level, added_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(fingerprint) DO UPDATE SET
			email = excluded.email,
			armored = excluded.armored,
			is_private = excluded.is_private,
			trust_level = excluded.trust_level`,
		k.Fingerprint, k.Email, k.Armored, k.IsPrivate, k.TrustLevel,
		k.AddedAt.Unix())
	if err != nil {
		return fmt.Errorf("store: upsert pgp key: %w", err)
	}
	return nil
}

// ListPGPKeys returns every stored key, newest first.
func (db *DB) ListPGPKeys(ctx context.Context) ([]PGPKey, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, fingerprint, email, armored, is_private, trust_level,
			added_at
		FROM pgp_keys ORDER BY added_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list pgp keys: %w", err)
	}
	defer rows.Close()
	var out []PGPKey
	for rows.Next() {
		var k PGPKey
		var added int64
		if err := rows.Scan(&k.ID, &k.Fingerprint, &k.Email, &k.Armored,
			&k.IsPrivate, &k.TrustLevel, &added); err != nil {
			return nil, fmt.Errorf("store: scan pgp key: %w", err)
		}
		k.AddedAt = time.Unix(added, 0).UTC()
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list pgp keys: %w", err)
	}
	return out, nil
}

// DeletePGPKey removes a key by fingerprint.
func (db *DB) DeletePGPKey(ctx context.Context, fingerprint string) error {
	_, err := db.sql.ExecContext(ctx,
		`DELETE FROM pgp_keys WHERE fingerprint = ?`, fingerprint)
	if err != nil {
		return fmt.Errorf("store: delete pgp key: %w", err)
	}
	return nil
}

// SetPGPTrust updates the stored trust level for a fingerprint.
func (db *DB) SetPGPTrust(ctx context.Context, fingerprint string, level int) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE pgp_keys SET trust_level = ? WHERE fingerprint = ?`,
		level, fingerprint)
	if err != nil {
		return fmt.Errorf("store: set pgp trust: %w", err)
	}
	return nil
}

// collectPrefixedMessages scans rows selected with prefixedMessageCols.
func collectPrefixedMessages(rows interface {
	Next() bool
	Scan(...any) error
	Close() error
	Err() error
}) ([]Message, error) {
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanPrefixedMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan message: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate messages: %w", err)
	}
	return out, nil
}
