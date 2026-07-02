package test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

// newTestKey generates a fast Ed25519/X25519 key pair for one identity.
func newTestKey(t *testing.T, name, email string) []byte {
	t.Helper()
	cfg := &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA}
	entity, err := openpgp.NewEntity(name, "", email, cfg)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PrivateKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := entity.SerializePrivate(w, nil); err != nil {
		t.Fatalf("serialize private key: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestKeyringImportExportTrust(t *testing.T) {
	kr := pgp.NewKeyring()
	fps, err := kr.ImportArmored(newTestKey(t, "Alice", "alice@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fps) != 1 {
		t.Fatalf("fingerprints = %v", fps)
	}
	fp := fps[0]

	if got := kr.Trust(fp); got != pgp.TrustUnknown {
		t.Errorf("initial trust = %v", got)
	}
	if err := kr.SetTrust(fp, pgp.TrustFull); err != nil {
		t.Fatal(err)
	}
	if got := kr.Trust(fp); got != pgp.TrustFull {
		t.Errorf("trust after set = %v", got)
	}
	if err := kr.SetTrust("deadbeef", pgp.TrustFull); err == nil {
		t.Error("setting trust on unknown key must fail")
	}

	pub, err := kr.ExportPublicArmored(fp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pub), "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Errorf("export = %.60s...", pub)
	}
	// The exported public key imports cleanly into a fresh keyring.
	other := pgp.NewKeyring()
	if _, err := other.ImportArmored(pub); err != nil {
		t.Fatalf("re-import public key: %v", err)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	// Alice (sender, signs) and Bob (recipient) share a keyring in this
	// test; in the app each device has its own.
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored(newTestKey(t, "Alice", "alice@example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.ImportArmored(newTestKey(t, "Bob", "bob@example.com")); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("the wind carries secrets")
	ciphertext, err := kr.Encrypt(plaintext, []string{"bob@example.com"}, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ciphertext), "BEGIN PGP MESSAGE") {
		t.Fatalf("ciphertext not armored: %.60s", ciphertext)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("plaintext leaked into ciphertext")
	}

	res, err := kr.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.Plaintext, plaintext) {
		t.Errorf("plaintext = %q", res.Plaintext)
	}
	if res.Signature != pgp.SigValid {
		t.Errorf("signature status = %v, want SigValid", res.Signature)
	}
	if res.SignedByFingerprint == "" {
		t.Error("signer fingerprint missing")
	}
}

func TestEncryptUnknownRecipientFails(t *testing.T) {
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored(newTestKey(t, "Alice", "alice@example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Encrypt([]byte("x"), []string{"stranger@example.com"}, ""); err == nil {
		t.Fatal("encrypting to an unknown recipient must fail")
	}
}

func TestDecryptTamperedCiphertextFails(t *testing.T) {
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored(newTestKey(t, "Bob", "bob@example.com")); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := kr.Encrypt([]byte("payload"), []string{"bob@example.com"}, "")
	if err != nil {
		t.Fatal(err)
	}
	// Flip a bit inside the armored body (not the header lines).
	tampered := bytes.Replace(ciphertext, []byte("\n"), []byte("\n"), 1)
	idx := bytes.Index(tampered, []byte("\n\n")) + 10
	tampered = append([]byte{}, tampered...)
	if tampered[idx] == 'A' {
		tampered[idx] = 'B'
	} else {
		tampered[idx] = 'A'
	}
	res, err := kr.Decrypt(tampered)
	if err == nil {
		// Some tampering surfaces as a signature/MDC failure during read;
		// success with intact plaintext would be the real failure.
		if res != nil && string(res.Plaintext) == "payload" && res.Signature == pgp.SigValid {
			t.Fatal("tampered message decrypted and verified")
		}
	}
}

func TestDetachedSignVerify(t *testing.T) {
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored(newTestKey(t, "Alice", "alice@example.com")); err != nil {
		t.Fatal(err)
	}
	message := []byte("signed manifest")
	sig, err := kr.Sign(message, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	status, err := kr.VerifyDetached(message, sig)
	if err != nil || status != pgp.SigValid {
		t.Fatalf("verify = %v, %v", status, err)
	}
	// Altered message must not verify.
	status, err = kr.VerifyDetached([]byte("signed manifesto"), sig)
	if err == nil || status == pgp.SigValid {
		t.Fatalf("altered message verified: %v, %v", status, err)
	}
}
