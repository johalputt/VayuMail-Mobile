package chat

import (
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

// TestCryptoRoundTrip encrypts a message to self (signed by self) and
// decrypts it, verifying the plaintext survives the PGP round trip the
// manager relies on.
func TestCryptoRoundTrip(t *testing.T) {
	priv, _ := genArmoredKeypair(t, "Self", "self@example.com")
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored([]byte(priv)); err != nil {
		t.Fatalf("import self key: %v", err)
	}

	const msg = "the balloon goes up at dawn"
	ciphertext, err := kr.Encrypt([]byte(msg), []string{"self@example.com"}, "self@example.com")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	res, err := kr.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(res.Plaintext) != msg {
		t.Fatalf("plaintext = %q, want %q", res.Plaintext, msg)
	}
	if res.Signature != pgp.SigValid {
		t.Errorf("signature status = %v, want SigValid", res.Signature)
	}
}
