package test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/applock"
	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// applockVerifierAlias is the documented keystore entry for the PIN
// verifier — the one place the derived hash may live (Rule 6).
const applockVerifierAlias = "vayumail-applock-v1"

// rewindLockout expires the lockout window by rewriting its documented
// settings representation (unix seconds as a string). Tests advance the
// clock this way instead of sleeping through real 30s windows.
func rewindLockout(t *testing.T, db *store.DB) {
	t.Helper()
	past := strconv.FormatInt(time.Now().Add(-time.Second).Unix(), 10)
	if err := db.SetSetting(t.Context(), store.SettingAppLockLockedUntil, past); err != nil {
		t.Fatal(err)
	}
}

func TestAppLockSetVerifyRoundTrip(t *testing.T) {
	db := openStore(t)
	ks := appcrypto.NewMemoryKeystore()
	lock := applock.New(ks, db)
	ctx := t.Context()

	if lock.Enabled(ctx) {
		t.Fatal("enabled before any PIN was set")
	}
	if ok, err := lock.Verify(ctx, "1234"); ok || err == nil {
		t.Fatalf("verify without a verifier = %v, %v", ok, err)
	}

	const pin = "4826"
	if err := lock.Set(ctx, pin); err != nil {
		t.Fatal(err)
	}
	if !lock.Enabled(ctx) {
		t.Fatal("not enabled after Set")
	}
	// The verifier landed under the documented alias and never embeds
	// the PIN itself.
	blob, err := ks.Fetch(applockVerifierAlias)
	if err != nil {
		t.Fatalf("verifier not in keystore: %v", err)
	}
	if strings.Contains(string(blob), pin) {
		t.Fatal("verifier blob contains the literal PIN")
	}

	if ok, err := lock.Verify(ctx, pin); !ok || err != nil {
		t.Fatalf("correct pin = %v, %v", ok, err)
	}
	if ok, err := lock.Verify(ctx, "0000"); ok || err != nil {
		t.Fatalf("wrong pin = %v, %v", ok, err)
	}
	if v, err := db.GetSetting(ctx, store.SettingAppLockFailures); err != nil || v != "1" {
		t.Fatalf("failures after one miss = %q, %v", v, err)
	}
	// A correct answer resets the counter.
	if ok, err := lock.Verify(ctx, pin); !ok || err != nil {
		t.Fatalf("correct pin after miss = %v, %v", ok, err)
	}
	if v, err := db.GetSetting(ctx, store.SettingAppLockFailures); err != nil || v != "" {
		t.Fatalf("failures not reset = %q, %v", v, err)
	}
}

func TestAppLockRejectsBadPINs(t *testing.T) {
	db := openStore(t)
	lock := applock.New(appcrypto.NewMemoryKeystore(), db)
	ctx := t.Context()

	for _, pin := range []string{"", "123", "1234567890123", "12a4", "abcd", "12 34", "12.4"} {
		if err := lock.Set(ctx, pin); !errors.Is(err, applock.ErrBadPIN) {
			t.Errorf("Set(%q) = %v, want ErrBadPIN", pin, err)
		}
	}
	if lock.Enabled(ctx) {
		t.Fatal("rejected PINs must not enable the lock")
	}
}

func TestAppLockLockoutSchedule(t *testing.T) {
	db := openStore(t)
	lock := applock.New(appcrypto.NewMemoryKeystore(), db)
	ctx := t.Context()
	const pin = "1357"
	if err := lock.Set(ctx, pin); err != nil {
		t.Fatal(err)
	}

	// Five free attempts: wrong, but no lockout window.
	for i := 1; i <= 5; i++ {
		if ok, err := lock.Verify(ctx, "0000"); ok || err != nil {
			t.Fatalf("free attempt %d = %v, %v", i, ok, err)
		}
		if d := lock.RetryDelay(ctx); d != 0 {
			t.Fatalf("free attempt %d opened a window: %v", i, d)
		}
	}

	// The sixth failure opens a ~30s window.
	if ok, err := lock.Verify(ctx, "0000"); ok || err != nil {
		t.Fatalf("sixth attempt = %v, %v", ok, err)
	}
	d1 := lock.RetryDelay(ctx)
	if d1 <= 0 || d1 > 30*time.Second {
		t.Fatalf("delay after sixth failure = %v, want (0, 30s]", d1)
	}
	// While locked out even the correct PIN is refused, and the counter
	// does not move.
	if _, err := lock.Verify(ctx, pin); !errors.Is(err, applock.ErrLockedOut) {
		t.Fatalf("verify while locked = %v, want ErrLockedOut", err)
	}
	if v, _ := db.GetSetting(ctx, store.SettingAppLockFailures); v != "6" {
		t.Fatalf("failures moved during lockout: %q", v)
	}

	// After the window expires, the next failure doubles it.
	rewindLockout(t, db)
	if d := lock.RetryDelay(ctx); d != 0 {
		t.Fatalf("delay after expiry = %v", d)
	}
	if ok, err := lock.Verify(ctx, "0000"); ok || err != nil {
		t.Fatalf("seventh attempt = %v, %v", ok, err)
	}
	d2 := lock.RetryDelay(ctx)
	if d2 <= 30*time.Second || d2 > time.Minute {
		t.Fatalf("delay after seventh failure = %v, want (30s, 60s]", d2)
	}
	if d2 <= d1 {
		t.Fatalf("delay did not grow: %v then %v", d1, d2)
	}

	// The window is capped at 15 minutes no matter how high the counter.
	if err := db.SetSetting(ctx, store.SettingAppLockFailures, "40"); err != nil {
		t.Fatal(err)
	}
	rewindLockout(t, db)
	if ok, err := lock.Verify(ctx, "0000"); ok || err != nil {
		t.Fatalf("attempt 41 = %v, %v", ok, err)
	}
	if d := lock.RetryDelay(ctx); d <= 14*time.Minute || d > 15*time.Minute {
		t.Fatalf("capped delay = %v, want (14m, 15m]", d)
	}

	// A correct answer (once allowed) clears the whole schedule.
	rewindLockout(t, db)
	if ok, err := lock.Verify(ctx, pin); !ok || err != nil {
		t.Fatalf("correct pin after expiry = %v, %v", ok, err)
	}
	if d := lock.RetryDelay(ctx); d != 0 {
		t.Fatalf("delay after success = %v", d)
	}
	if v, _ := db.GetSetting(ctx, store.SettingAppLockLockedUntil); v != "" {
		t.Fatalf("lockout state not cleared: %q", v)
	}
}

func TestAppLockRemove(t *testing.T) {
	db := openStore(t)
	ks := appcrypto.NewMemoryKeystore()
	lock := applock.New(ks, db)
	ctx := t.Context()

	if err := lock.Set(ctx, "2468"); err != nil {
		t.Fatal(err)
	}
	if ok, err := lock.Verify(ctx, "0000"); ok || err != nil {
		t.Fatalf("wrong pin = %v, %v", ok, err)
	}
	if err := lock.Remove(ctx); err != nil {
		t.Fatal(err)
	}
	if lock.Enabled(ctx) {
		t.Fatal("still enabled after Remove")
	}
	if _, err := ks.Fetch(applockVerifierAlias); !errors.Is(err, appcrypto.ErrKeyNotFound) {
		t.Fatalf("verifier still in keystore: %v", err)
	}
	for _, key := range []string{store.SettingAppLockFailures, store.SettingAppLockLockedUntil} {
		if v, _ := db.GetSetting(ctx, key); v != "" {
			t.Errorf("setting %q not cleared: %q", key, v)
		}
	}
	// Removing an absent lock is a no-op, not an error.
	if err := lock.Remove(ctx); err != nil {
		t.Fatalf("second remove: %v", err)
	}
}

// TestAppLockPINNeverPersisted sets and exercises the lock over a
// file-backed store, then scans every file it produced for the literal
// PIN — the same on-disk plaintext check security_test.go applies to
// credentials (Rule 6).
func TestAppLockPINNeverPersisted(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(t.Context(), filepath.Join(dir, "vayu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	lock := applock.New(appcrypto.NewMemoryKeystore(), db)
	ctx := t.Context()

	const pin = "758291"
	if err := lock.Set(ctx, pin); err != nil {
		t.Fatal(err)
	}
	// Populate every settings row the lock ever writes: failures and an
	// open lockout window.
	for i := 0; i < 6; i++ {
		if ok, err := lock.Verify(ctx, "000000"); ok || err != nil {
			t.Fatalf("wrong pin %d = %v, %v", i, ok, err)
		}
	}

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(raw, []byte(pin)) {
			t.Errorf("literal PIN found in %s (Rule 6 violation)", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
