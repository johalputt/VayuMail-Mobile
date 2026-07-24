package state

import (
	"bytes"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// armoredPrivate serialises an entity's private key in armored form so it can
// be imported into a Keyring.
func armoredPrivate(t *testing.T, ent *openpgp.Entity) string {
	t.Helper()
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PrivateKeyType, nil)
	if err != nil {
		t.Fatalf("armor encode: %v", err)
	}
	if err := ent.SerializePrivate(w, nil); err != nil {
		t.Fatalf("serialize private: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close armor: %v", err)
	}
	return buf.String()
}

// A validly-signed encrypted message must decrypt AND come back marked
// PGPSigVerified — the badge's authenticity claim rests on the real signature
// verdict, not the MIME structure (audit M17).
func TestDecryptMarksVerifiedSignature(t *testing.T) {
	const email = "me@test.example"
	ent, err := openpgp.NewEntity("Me", "", email, nil)
	if err != nil {
		t.Fatalf("new entity: %v", err)
	}
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored([]byte(armoredPrivate(t, ent))); err != nil {
		t.Fatalf("import: %v", err)
	}
	ciphertext, err := kr.Encrypt([]byte("hello"), []string{email}, email)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	s := &AppState{keyring: kr}
	out := s.decryptThread([]store.Message{{
		PGPStatus: "encrypted",
		BodyText:  string(ciphertext),
	}})
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}
	if out[0].BodyText != "hello" {
		t.Fatalf("plaintext mismatch: %q", out[0].BodyText)
	}
	if !out[0].PGPSigVerified {
		t.Fatal("validly-signed message should be marked PGPSigVerified")
	}
}

// An encrypted-but-UNSIGNED message must decrypt but NOT be marked verified —
// there is no signature to authenticate the sender, so the UI must not claim
// one (audit M17).
func TestDecryptUnsignedIsNotVerified(t *testing.T) {
	const email = "me@test.example"
	ent, err := openpgp.NewEntity("Me", "", email, nil)
	if err != nil {
		t.Fatalf("new entity: %v", err)
	}
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored([]byte(armoredPrivate(t, ent))); err != nil {
		t.Fatalf("import: %v", err)
	}
	// signerEmail "" → encrypted but not signed.
	ciphertext, err := kr.Encrypt([]byte("hello"), []string{email}, "")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	s := &AppState{keyring: kr}
	out := s.decryptThread([]store.Message{{
		PGPStatus: "encrypted",
		BodyText:  string(ciphertext),
	}})
	if out[0].BodyText != "hello" {
		t.Fatalf("plaintext mismatch: %q", out[0].BodyText)
	}
	if out[0].PGPSigVerified {
		t.Fatal("unsigned message must not be marked PGPSigVerified")
	}
}
