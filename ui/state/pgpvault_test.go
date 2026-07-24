package state

import (
	"context"
	"path/filepath"
	"testing"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

const testPrivArmored = "-----BEGIN PGP PRIVATE KEY BLOCK-----\nfake\n-----END PGP PRIVATE KEY BLOCK-----"

func newTestState(t *testing.T, ks appcrypto.Keystore) *AppState {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &AppState{db: db, ks: ks, keyring: pgp.NewKeyring()}
}

// storeKeyRow must seal a PRIVATE key in the platform keystore and leave the
// pgp_keys row's armored column EMPTY — the cleartext private key must never
// reach SQLite (audit H6).
func TestStorePrivateKeyNeverHitsSQLite(t *testing.T) {
	ks := appcrypto.NewMemoryKeystore()
	s := newTestState(t, ks)
	const fp = "ABCDEF0123456789"

	s.storeKeyRow(fp, testPrivArmored, true)

	// The DB row exists but carries no private material.
	keys, err := s.db.ListPGPKeys(context.Background())
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 row, got %d", len(keys))
	}
	if keys[0].Armored != "" {
		t.Fatalf("private key armored leaked into SQLite: %q", keys[0].Armored)
	}
	if !keys[0].IsPrivate {
		t.Fatal("row should be marked private")
	}
	// The sealed keystore holds the real key.
	got, err := openPrivateKey(ks, fp)
	if err != nil {
		t.Fatalf("sealed private key not found: %v", err)
	}
	if got != testPrivArmored {
		t.Fatalf("sealed key mismatch: %q", got)
	}
}

// A PUBLIC key still lives in SQLite (it is not secret) and is not sealed.
func TestStorePublicKeyStaysInSQLite(t *testing.T) {
	ks := appcrypto.NewMemoryKeystore()
	s := newTestState(t, ks)
	const fp = "1111222233334444"
	const pub = "-----BEGIN PGP PUBLIC KEY BLOCK-----\nx\n-----END PGP PUBLIC KEY BLOCK-----"

	s.storeKeyRow(fp, pub, false)

	keys, err := s.db.ListPGPKeys(context.Background())
	if err != nil || len(keys) != 1 {
		t.Fatalf("list keys: %v (n=%d)", err, len(keys))
	}
	if keys[0].Armored != pub {
		t.Fatalf("public key should stay in SQLite, got %q", keys[0].Armored)
	}
	if _, err := openPrivateKey(ks, fp); err == nil {
		t.Fatal("public key must not be sealed in the keystore")
	}
}

// A legacy row that still carries a private key inline (written by an older
// build) must be migrated into the sealed keystore and blanked in the DB on
// the next load (audit H6).
func TestMigrateLegacyPrivateKey(t *testing.T) {
	ks := appcrypto.NewMemoryKeystore()
	s := newTestState(t, ks)
	const fp = "DEADBEEFDEADBEEF"

	// Simulate the old cleartext-in-SQLite layout.
	if err := s.db.UpsertPGPKey(context.Background(), &store.PGPKey{
		Fingerprint: fp, Email: "a@b.c", Armored: testPrivArmored, IsPrivate: true,
	}); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	s.loadPGPKeys(context.Background())

	keys, err := s.db.ListPGPKeys(context.Background())
	if err != nil || len(keys) != 1 {
		t.Fatalf("list keys: %v (n=%d)", err, len(keys))
	}
	if keys[0].Armored != "" {
		t.Fatalf("legacy private key still in SQLite after migration: %q", keys[0].Armored)
	}
	got, err := openPrivateKey(ks, fp)
	if err != nil {
		t.Fatalf("legacy key not migrated into keystore: %v", err)
	}
	if got != testPrivArmored {
		t.Fatalf("migrated key mismatch: %q", got)
	}
}
