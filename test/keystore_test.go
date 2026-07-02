package test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
)

func TestSealedKeystoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ks, err := appcrypto.NewSealedKeystore(dir)
	if err != nil {
		t.Fatal(err)
	}

	secret := []byte("imap-password-πø")
	if err := ks.Store("acct-1", secret); err != nil {
		t.Fatal(err)
	}
	got, err := ks.Fetch("acct-1")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("fetch = %q, %v", got, err)
	}

	// Secrets survive a process restart (fresh keystore over same dir).
	ks2, err := appcrypto.NewSealedKeystore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = ks2.Fetch("acct-1")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("fetch after reopen = %q, %v", got, err)
	}

	// Delete is durable too.
	if err := ks2.Delete("acct-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := ks2.Fetch("acct-1"); !errors.Is(err, appcrypto.ErrKeyNotFound) {
		t.Fatalf("want ErrKeyNotFound, got %v", err)
	}
	ks3, err := appcrypto.NewSealedKeystore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ks3.Fetch("acct-1"); !errors.Is(err, appcrypto.ErrKeyNotFound) {
		t.Fatalf("delete not persisted: %v", err)
	}
}

func TestSealedKeystoreNeverStoresPlaintext(t *testing.T) {
	dir := t.TempDir()
	ks, err := appcrypto.NewSealedKeystore(dir)
	if err != nil {
		t.Fatal(err)
	}
	secret := "hunter2-super-secret-password"
	if err := ks.Store("acct-1", []byte(secret)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "credentials.sealed"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("plaintext credential found on disk (Rule 6 violation)")
	}
}

func TestSealedKeystoreEntriesAreAliasBound(t *testing.T) {
	dir := t.TempDir()
	ks, err := appcrypto.NewSealedKeystore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.Store("alias-a", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	// Simulate an attacker copying alias-a's ciphertext onto alias-b:
	// GCM's additional data (the alias) must make it unopenable.
	src, err := os.ReadFile(filepath.Join(dir, "credentials.sealed"))
	if err != nil {
		t.Fatal(err)
	}
	swapped := strings.Replace(string(src), "alias-a", "alias-b", 1)
	if err := os.WriteFile(filepath.Join(dir, "credentials.sealed"), []byte(swapped), 0o600); err != nil {
		t.Fatal(err)
	}
	ks2, err := appcrypto.NewSealedKeystore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ks2.Fetch("alias-b"); err == nil {
		t.Fatal("cross-alias ciphertext replay must fail to unseal")
	}
}
