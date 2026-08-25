package android

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
)

// xorB64 is a deterministic fake "hardware" wrapper: reversible, so tests
// can assert round-trips, but obviously not real crypto — the point of the
// fake is to drive the provider's decisions, not to be strong.
func xorB64(key []byte) func([]byte) (string, error) {
	return func(plain []byte) (string, error) {
		out := make([]byte, len(plain))
		for i := range plain {
			out[i] = plain[i] ^ key[i%len(key)]
		}
		return base64.StdEncoding.EncodeToString(out), nil
	}
}

func unxorB64(key []byte) func(string) ([]byte, error) {
	return func(sealed string) ([]byte, error) {
		raw, err := base64.StdEncoding.DecodeString(sealed)
		if err != nil {
			return nil, err
		}
		out := make([]byte, len(raw))
		for i := range raw {
			out[i] = raw[i] ^ key[i%len(key)]
		}
		return out, nil
	}
}

func testProvider(t *testing.T) (*WrappedKeyProvider, string, *int) {
	t.Helper()
	dir := t.TempDir()
	calls := 0
	p := NewWrappedKeyProvider(dir,
		func(b []byte) (string, error) { calls++; return xorB64([]byte("hwkey"))(b) },
		unxorB64([]byte("hwkey")))
	return p, dir, &calls
}

// The full lifecycle on a fresh device: first call generates and persists a
// wrapped blob; the second call must come from cache AND from the existing
// blob — proving both that no second generation happened and that a later
// provider instance (the next process launch) opens what the first wrote.
func TestWrappedKeyLifecycle(t *testing.T) {
	p, dir, calls := testProvider(t)

	k1, err := p.MasterKey()
	if err != nil || len(k1) != 32 {
		t.Fatalf("first MasterKey: %v (%d bytes)", err, len(k1))
	}
	k2, err := p.MasterKey()
	if err != nil || k2 == nil || string(k1) != string(k2) {
		t.Fatalf("cached MasterKey drifted: %v", err)
	}
	if got := *calls; got != 1 {
		t.Fatalf("wrap called %d times for one install", got)
	}

	// A brand-new provider over the same directory is the next launch.
	fresh := NewWrappedKeyProvider(dir,
		func([]byte) (string, error) { *calls++; return "", errors.New("must not re-wrap") },
		unxorB64([]byte("hwkey")))
	k3, err := fresh.MasterKey()
	if err != nil || string(k3) != string(k1) {
		t.Fatalf("next-launch unwrap: %v (match=%v)", err, string(k3) == string(k1))
	}
	if got := *calls; got != 1 {
		t.Fatalf("re-wrap attempted on an existing blob: %d extra calls", got-1)
	}
	if _, err := os.Stat(filepath.Join(dir, wrappedKeyName)); err != nil {
		t.Fatalf("wrapped blob missing: %v", err)
	}
}

// The sealed store must open secrets sealed under a hardware-wrapped master
// key across provider instances — the property that makes the wrapping
// transparent to everything above it.
func TestSealedStoreOverWrappedProvider(t *testing.T) {
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys")
	mk := func() appcrypto.KeyProvider {
		return NewWrappedKeyProvider(keys,
			xorB64([]byte("device-bound")), unxorB64([]byte("device-bound")))
	}

	store1, err := appcrypto.NewSealedKeystoreWithProvider(
		filepath.Join(dir, "credentials.sealed"), mk())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store1.Store("alias-a", []byte("secret-one")); err != nil {
		t.Fatalf("store: %v", err)
	}

	store2, err := appcrypto.NewSealedKeystoreWithProvider(
		filepath.Join(dir, "credentials.sealed"), mk())
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	got, err := store2.Fetch("alias-a")
	if err != nil || string(got) != "secret-one" {
		t.Fatalf("fetch after reopen: %v (%q)", err, got)
	}
}

// An unwrapper that fails means THIS DEVICE cannot open its own blob — the
// provider must fail loudly, never regenerate (regenerating orphans every
// secret sealed under the old key).
func TestUnwrapFailureIsFatalNotRegenerated(t *testing.T) {
	p, dir, _ := testProvider(t)
	if _, err := p.MasterKey(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	attempts := 0
	broken := NewWrappedKeyProvider(dir,
		func([]byte) (string, error) { attempts++; return "", errors.New("rewrap attempted") },
		func(string) ([]byte, error) { return nil, errors.New("hardware says no") })
	_, err := broken.MasterKey()
	if err == nil || !strings.Contains(err.Error(), "unwrap") {
		t.Fatalf("expected unwrap failure, got %v", err)
	}
	if attempts != 0 {
		t.Fatal("provider regenerated after unwrap failure — this orphans every sealed secret")
	}
}

// A blob that unwraps to the wrong length is corrupt, and is refused rather
// than truncated into service. The length check guards the unwrap path;
// generation is 32 bytes by construction.
func TestWrongLengthBlobRefused(t *testing.T) {
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys")
	seed := NewWrappedKeyProvider(keys,
		xorB64([]byte("k")), unxorB64([]byte("k")))
	if _, err := seed.MasterKey(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	short := NewWrappedKeyProvider(keys,
		func([]byte) (string, error) { return "", errors.New("must not re-wrap") },
		func(string) ([]byte, error) { return make([]byte, 16), nil })
	_, err := short.MasterKey()
	if err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("expected bad-length refusal, got %v", err)
	}
}

// Unreadable-directory failures surface as read errors, not as silent
// regeneration somewhere else.
func TestUnreadableBlobSurfacesError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing-parents")
	p := NewWrappedKeyProvider(filepath.Join(dir, "nested", "deeper"),
		xorB64([]byte("k")), unxorB64([]byte("k")))
	// No Wrap call happens before MkdirAll succeeds, so this exercises the
	// default branch of the read switch only if the file exists but cannot
	// be read. Create a blob behind an unreadable path component instead:
	// on every platform ReadFile of a directory-shaped path is an error.
	p.Dir = dir
	if err := os.MkdirAll(filepath.Join(dir, wrappedKeyName), 0o755); err == nil {
		if _, err := p.MasterKey(); err == nil || strings.Contains(err.Error(), "wrap master key") {
			t.Fatalf("expected read failure, got %v", err)
		}
	}
}
