package test

import (
	"context"
	"encoding/base32"
	"path/filepath"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/applock"
	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// rfcSecretB32 is the RFC 4226 appendix D test secret
// ("12345678901234567890") in base32 — the shared vector every
// authenticator implementation is validated against.
var rfcSecretB32 = base32.StdEncoding.EncodeToString([]byte("12345678901234567890"))

func newTOTPManager(t *testing.T) (*applock.Manager, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "totp.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return applock.New(appcrypto.NewMemoryKeystore(), db), ctx
}

// TestTOTPRFC4226Vectors proves the HOTP core against the published
// vectors: counter values 0..9 for the appendix-D secret. TOTP is
// HOTP(unix/30), so pinning time to counter*30s exercises the full path
// through VerifyTOTP.
func TestTOTPRFC4226Vectors(t *testing.T) {
	want := []string{
		"755224", "287082", "359152", "969429", "338314",
		"254676", "287922", "162583", "399871", "520489",
	}
	m, ctx := newTOTPManager(t)
	if err := m.SetTOTP(ctx, rfcSecretB32); err != nil {
		t.Fatalf("SetTOTP: %v", err)
	}
	for counter, code := range want {
		m.SetNowForTest(func() time.Time {
			return time.Unix(int64(counter)*30, 0)
		})
		ok, err := m.VerifyTOTP(ctx, code)
		if err != nil || !ok {
			t.Fatalf("counter %d: code %s rejected (ok=%v err=%v)", counter, code, ok, err)
		}
	}
}

func TestTOTPRejectsAndLocksOut(t *testing.T) {
	m, ctx := newTOTPManager(t)
	if err := m.SetTOTP(ctx, rfcSecretB32); err != nil {
		t.Fatalf("SetTOTP: %v", err)
	}
	m.SetNowForTest(func() time.Time { return time.Unix(59, 0) })
	// Six wrong codes: five free, the sixth opens the lockout window.
	for i := 0; i < 6; i++ {
		if ok, _ := m.VerifyTOTP(ctx, "000000"); ok {
			t.Fatal("wrong code accepted")
		}
	}
	if d := m.RetryDelay(ctx); d <= 0 {
		t.Fatal("expected lockout after repeated wrong codes")
	}
	if _, err := m.VerifyTOTP(ctx, "287082"); err == nil {
		t.Fatal("expected ErrLockedOut during lockout window")
	}
}

func TestTOTPSecretValidation(t *testing.T) {
	m, ctx := newTOTPManager(t)
	for _, bad := range []string{"", "short", "!!!not-base32!!!", "ABCDEFG"} {
		if err := m.SetTOTP(ctx, bad); err == nil {
			t.Fatalf("secret %q accepted", bad)
		}
	}
	if m.TOTPEnabled(ctx) {
		t.Fatal("TOTP enabled after rejected secrets")
	}
	// Spaces and lowercase are tolerated on a valid secret.
	spaced := "gezd gnbv gy3t qojq gezd gnbv gy3t qojq"
	if err := m.SetTOTP(ctx, spaced); err != nil {
		t.Fatalf("normalized secret rejected: %v", err)
	}
	if !m.TOTPEnabled(ctx) {
		t.Fatal("TOTP not enabled after SetTOTP")
	}
	if err := m.RemoveTOTP(ctx); err != nil {
		t.Fatalf("RemoveTOTP: %v", err)
	}
	if m.TOTPEnabled(ctx) {
		t.Fatal("TOTP still enabled after removal")
	}
}
