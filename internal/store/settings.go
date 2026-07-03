package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Setting keys. Kept here so callers share the same string.
const (
	// SettingPGPKeyDirectoryURL is the VayuPress key-directory base URL used
	// to auto-import correspondents' PGP public keys.
	SettingPGPKeyDirectoryURL = "pgp_key_directory_url"
)

// GetSetting returns the stored value for key, or "" if unset.
func (db *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := db.sql.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get setting %q: %w", key, err)
	}
	return v, nil
}

// SetSetting upserts a settings value. Storing "" removes the row.
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	if value == "" {
		_, err := db.sql.ExecContext(ctx,
			`DELETE FROM settings WHERE key = ?`, key)
		if err != nil {
			return fmt.Errorf("store: clear setting %q: %w", key, err)
		}
		return nil
	}
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("store: set setting %q: %w", key, err)
	}
	return nil
}
