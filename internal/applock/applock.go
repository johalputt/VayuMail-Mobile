// Package applock implements the PIN-based app lock. The PIN itself is
// never persisted anywhere: only a PBKDF2 verifier (random salt plus
// derived key) is kept, and it lives in the platform keystore — never in
// SQLite, never plaintext on disk (Constitutional Rule 6). SQLite holds
// only lockout bookkeeping; an attacker who can edit the database could
// reset the counter, but the verifier — not the counter — is what gates
// knowledge of the PIN.
package applock

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// ErrLockedOut is returned by Verify while the lockout window is open;
// the attempt was refused before touching the verifier or the counter.
var ErrLockedOut = errors.New("applock: locked out, retry later")

// ErrBadPIN is returned by Set for a PIN that is not 4-12 digits.
var ErrBadPIN = errors.New("applock: pin must be 4-12 digits")

const (
	// verifierAlias names the keystore entry holding the serialized
	// verifier. Versioned so a future KDF change can migrate cleanly.
	verifierAlias = "vayumail-applock-v1"
	// pbkdf2Iterations follows the OWASP 2023 floor for PBKDF2-SHA-256.
	// Brute force on a 4-digit PIN is bounded by the lockout schedule,
	// not the KDF, but an expensive KDF still slows offline guessing
	// against a leaked verifier.
	pbkdf2Iterations = 600000
	saltLen          = 16
	keyLen           = 32
	// freeAttempts wrong answers are allowed before lockout engages;
	// after that each failure doubles the wait from lockoutBase up to
	// lockoutMax.
	freeAttempts = 5
	lockoutBase  = 30 * time.Second
	lockoutMax   = 15 * time.Minute
)

// verifier is the JSON blob stored in the keystore. Salt and Hash encode
// as base64; the PIN cannot be recovered from either.
type verifier struct {
	Version    int    `json:"version"`
	Salt       []byte `json:"salt"`
	Iterations int    `json:"iterations"`
	Hash       []byte `json:"hash"`
}

// Manager stores the PIN verifier in the platform keystore and lockout
// bookkeeping in the settings table. The PIN itself is never persisted.
// Callers invoke methods from their own goroutines: PBKDF2 and keystore
// access are too slow for a UI frame loop.
type Manager struct {
	ks crypto.Keystore
	db *store.DB
	// now feeds the lockout arithmetic; a field so the clock can be
	// pinned without touching the wall clock.
	now func() time.Time
}

// New creates a Manager over the given keystore and store.
func New(ks crypto.Keystore, db *store.DB) *Manager {
	return &Manager{ks: ks, db: db, now: time.Now}
}

// Enabled reports whether a PIN is set.
func (m *Manager) Enabled(ctx context.Context) bool {
	_, err := m.ks.Fetch(verifierAlias)
	return err == nil
}

// Set derives a fresh verifier from pin and stores it, replacing any
// previous one and clearing lockout state. Pin must be 4-12 digits.
func (m *Manager) Set(ctx context.Context, pin string) error {
	if !validPIN(pin) {
		return ErrBadPIN
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("applock: salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, pin, salt, pbkdf2Iterations, keyLen)
	if err != nil {
		return fmt.Errorf("applock: derive: %w", err)
	}
	blob, err := json.Marshal(verifier{
		Version:    1,
		Salt:       salt,
		Iterations: pbkdf2Iterations,
		Hash:       key,
	})
	if err != nil {
		return fmt.Errorf("applock: encode verifier: %w", err)
	}
	if err := m.ks.Store(verifierAlias, blob); err != nil {
		return fmt.Errorf("applock: store verifier: %w", err)
	}
	return m.clearLockout(ctx)
}

// Verify checks pin against the stored verifier in constant time.
// Wrong answers count toward lockout; a correct answer resets it. While
// locked out it returns (false, ErrLockedOut) without touching either
// the verifier or the counter.
func (m *Manager) Verify(ctx context.Context, pin string) (bool, error) {
	if m.RetryDelay(ctx) > 0 {
		return false, ErrLockedOut
	}
	blob, err := m.ks.Fetch(verifierAlias)
	if err != nil {
		return false, fmt.Errorf("applock: verifier: %w", err)
	}
	var v verifier
	if err := json.Unmarshal(blob, &v); err != nil {
		return false, fmt.Errorf("applock: verifier corrupt: %w", err)
	}
	// An empty hash would compare equal to an empty derivation; refuse
	// rather than fail open.
	if v.Iterations <= 0 || len(v.Salt) == 0 || len(v.Hash) == 0 {
		return false, errors.New("applock: verifier corrupt")
	}
	derived, err := pbkdf2.Key(sha256.New, pin, v.Salt, v.Iterations, len(v.Hash))
	if err != nil {
		return false, fmt.Errorf("applock: derive: %w", err)
	}
	if subtle.ConstantTimeCompare(derived, v.Hash) != 1 {
		return false, m.recordFailure(ctx)
	}
	// Bookkeeping failure must not keep the rightful user out.
	if err := m.clearLockout(ctx); err != nil {
		slog.Warn("applock: clear lockout after success", "err", err)
	}
	return true, nil
}

// Remove disables the lock and clears lockout state.
func (m *Manager) Remove(ctx context.Context) error {
	if err := m.ks.Delete(verifierAlias); err != nil {
		return fmt.Errorf("applock: delete verifier: %w", err)
	}
	return m.clearLockout(ctx)
}

// RetryDelay reports how long the caller must wait before the next
// attempt is allowed (0 = now). 5 free attempts, then 30s doubling per
// failure, capped at 15 minutes. Unreadable or corrupt bookkeeping reads
// as "no delay" — failing open here cannot leak the PIN, since the
// verifier check still gates entry.
func (m *Manager) RetryDelay(ctx context.Context) time.Duration {
	raw, err := m.db.GetSetting(ctx, store.SettingAppLockLockedUntil)
	if err != nil || raw == "" {
		return 0
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	d := time.Unix(sec, 0).Sub(m.now())
	if d < 0 {
		return 0
	}
	return d
}

// recordFailure increments the failure counter and, once the free
// attempts are spent, opens the doubling lockout window.
func (m *Manager) recordFailure(ctx context.Context) error {
	raw, err := m.db.GetSetting(ctx, store.SettingAppLockFailures)
	if err != nil {
		return fmt.Errorf("applock: read failures: %w", err)
	}
	// A missing or corrupt counter reads as zero and self-heals on write.
	n, _ := strconv.Atoi(raw)
	n++
	if err := m.db.SetSetting(ctx, store.SettingAppLockFailures,
		strconv.Itoa(n)); err != nil {
		return fmt.Errorf("applock: store failures: %w", err)
	}
	if d := lockoutAfter(n); d > 0 {
		until := m.now().Add(d).Unix()
		if err := m.db.SetSetting(ctx, store.SettingAppLockLockedUntil,
			strconv.FormatInt(until, 10)); err != nil {
			return fmt.Errorf("applock: store lockout: %w", err)
		}
	}
	return nil
}

// lockoutAfter maps a failure count onto its lockout duration: the first
// freeAttempts failures cost nothing, the next 30s, then doubling to the
// 15-minute cap.
func lockoutAfter(failures int) time.Duration {
	steps := failures - freeAttempts - 1
	if steps < 0 {
		return 0
	}
	// lockoutBase << 5 already exceeds lockoutMax; capping here also
	// keeps a huge persisted counter from overflowing the shift.
	if steps >= 5 {
		return lockoutMax
	}
	d := lockoutBase << steps
	if d > lockoutMax {
		return lockoutMax
	}
	return d
}

// clearLockout resets the failure counter and closes any lockout window.
func (m *Manager) clearLockout(ctx context.Context) error {
	if err := m.db.SetSetting(ctx, store.SettingAppLockFailures, ""); err != nil {
		return fmt.Errorf("applock: clear failures: %w", err)
	}
	if err := m.db.SetSetting(ctx, store.SettingAppLockLockedUntil, ""); err != nil {
		return fmt.Errorf("applock: clear lockout: %w", err)
	}
	return nil
}

// validPIN accepts 4-12 ASCII digits and nothing else: a fixed, small
// alphabet keeps the on-screen PIN pad honest and the length bound stops
// pathological KDF inputs.
func validPIN(pin string) bool {
	if len(pin) < 4 || len(pin) > 12 {
		return false
	}
	for i := 0; i < len(pin); i++ {
		if pin[i] < '0' || pin[i] > '9' {
			return false
		}
	}
	return true
}
