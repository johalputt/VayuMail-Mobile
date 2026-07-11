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
	// SettingAutoWKD auto-discovers correspondents' keys via WKD when new
	// mail arrives (throttled). On by default since v2.0.0; only an
	// explicit "0" disables it.
	SettingAutoWKD = "auto_wkd"
	// SettingAppLockFailures counts consecutive failed PIN attempts (as a
	// decimal string). Only lockout bookkeeping lives here — the PIN
	// verifier itself is in the platform keystore, never in SQLite.
	SettingAppLockFailures = "applock_failures"
	// SettingAppLockLockedUntil is the Unix-seconds time (as a string)
	// before which PIN attempts are refused. "" or past = not locked out.
	SettingAppLockLockedUntil = "applock_locked_until"
	// SettingAppLockTimeout is how long (seconds, as a string) the app may
	// stay backgrounded before the lock re-engages. Consumed by the UI.
	SettingAppLockTimeout = "applock_timeout"
	// SettingNotifications toggles new-mail notifications: "" or "1" = on,
	// "0" = off. Consumed by the UI.
	SettingNotifications = "notifications"
	// SettingNotifyPreview controls whether notifications show sender and
	// subject ("" or "1") or only a generic "New mail" line ("0") — the
	// privacy option for lock screens. Consumed by the UI.
	SettingNotifyPreview = "notify_preview"
	// SettingDeviceIDPrefix keys the per-account device ID granted by a
	// VayuPress server during device-approval onboarding (ADR-0011); the
	// full key is the prefix plus the account's email address. Only the
	// public identifier is stored — the device password is a credential
	// and lives exclusively in the platform keystore (Rule 6).
	SettingDeviceIDPrefix = "device-id:"
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
