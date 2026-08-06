package test

// pgp_sender_binding_audit_test.go — a valid signature is not an authentic
// sender until the signing key is bound to the address the mail claims to be
// from.
//
// The finding, in the attacker's voice: I do not need to break anything. I need
// my public key to be on your keyring, which is the normal outcome of you ever
// having corresponded with me, imported a key from a mail, or letting the app
// resolve a key over WKD. From there I send you a message with
// `From: security@yourbank.example`, encrypted to you and signed with MY key.
// The keyring finds a key that verifies the signature, so the verdict is
// SigValid, and the client renders its strongest trust signal — cryptographic
// verification — on a mail from an address I have no relationship with.
//
// The From header is attacker-controlled SMTP text. The signing key is the only
// identity in the message that is not. Reporting "verified" without comparing
// the two is reporting that somebody, somewhere, signed something.
//
// VayuTalk already gets this right: internal/chat binds the signing key's
// fingerprint to the verified peer and refuses the message otherwise (audit
// H7). The mail path discarded Result.SignedByFingerprint entirely.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

// newTestEntity returns a usable entity plus its armored private key.
func newTestEntity(t *testing.T, name, email string) (*openpgp.Entity, []byte) {
	t.Helper()
	cfg := &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA}
	entity, err := openpgp.NewEntity(name, "", email, cfg)
	if err != nil {
		t.Fatalf("generate key for %s: %v", email, err)
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
	return entity, buf.Bytes()
}

// sealFor encrypts plaintext to recipient and signs it as signer.
func sealFor(t *testing.T, recipient, signer *openpgp.Entity, plaintext string) []byte {
	t.Helper()
	var buf bytes.Buffer
	aw, err := armor.Encode(&buf, "PGP MESSAGE", nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := openpgp.Encrypt(aw, []*openpgp.Entity{recipient}, signer, nil, nil)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := w.Write([]byte(plaintext)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := aw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ringWith builds a keyring holding the recipient's private key plus every
// supplied public key — the ordinary state of a mail client that has ever
// exchanged keys with anyone.
func ringWith(t *testing.T, ownPriv []byte, others ...[]byte) *pgp.Keyring {
	t.Helper()
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored(ownPriv); err != nil {
		t.Fatalf("import own key: %v", err)
	}
	for _, o := range others {
		if _, err := kr.ImportArmored(o); err != nil {
			t.Fatalf("import peer key: %v", err)
		}
	}
	return kr
}

// THE finding. Mallory is on the keyring; the mail claims to be from Alice.
func TestASignatureFromTheWrongKeyIsNotAVerifiedSender(t *testing.T) {
	_, bobPriv := newTestEntity(t, "Bob", "bob@example.com")
	bob, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(bobPriv))
	if err != nil {
		t.Fatal(err)
	}
	_, alicePriv := newTestEntity(t, "Alice", "alice@example.com")
	mallory, malloryPriv := newTestEntity(t, "Mallory", "mallory@evil.example")

	kr := ringWith(t, bobPriv, alicePriv, malloryPriv)

	// Encrypted to Bob, signed by Mallory, but the mail will claim Alice.
	sealed := sealFor(t, bob[0], mallory, "please approve this transfer")

	res, err := kr.DecryptFrom(sealed, "alice@example.com")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if res.Signature != pgp.SigValid {
		t.Fatalf("precondition failed: the signature should verify against Mallory's key "+
			"(got %v). Without that this test proves nothing.", res.Signature)
	}
	if res.SenderVerified {
		t.Errorf("a message signed by %s was reported as a VERIFIED sender for alice@example.com.\n"+
			"The From header is attacker-controlled SMTP text; the signing key is the only "+
			"identity in the message that is not. Compare them, or the verified badge means "+
			"nothing more than 'somebody on this keyring signed something'.",
			"mallory@evil.example")
	}
}

// The honest case must keep working, or the fix is just a broken feature.
func TestASignatureFromTheClaimedSenderIsAVerifiedSender(t *testing.T) {
	_, bobPriv := newTestEntity(t, "Bob", "bob@example.com")
	bob, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(bobPriv))
	if err != nil {
		t.Fatal(err)
	}
	alice, alicePriv := newTestEntity(t, "Alice", "alice@example.com")
	_, malloryPriv := newTestEntity(t, "Mallory", "mallory@evil.example")

	kr := ringWith(t, bobPriv, alicePriv, malloryPriv)
	sealed := sealFor(t, bob[0], alice, "lunch on thursday?")

	res, err := kr.DecryptFrom(sealed, "alice@example.com")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if res.Signature != pgp.SigValid {
		t.Fatalf("signature verdict = %v, want SigValid", res.Signature)
	}
	if !res.SenderVerified {
		t.Error("a message genuinely signed by alice@example.com was NOT reported as verified")
	}
	if string(res.Plaintext) != "lunch on thursday?" {
		t.Errorf("plaintext = %q", res.Plaintext)
	}
}

// Address comparison is case- and whitespace-insensitive, and a display-name
// wrapper is the normal shape of a From header.
func TestSenderBindingHandlesRealFromHeaders(t *testing.T) {
	_, bobPriv := newTestEntity(t, "Bob", "bob@example.com")
	bob, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(bobPriv))
	if err != nil {
		t.Fatal(err)
	}
	alice, alicePriv := newTestEntity(t, "Alice", "alice@example.com")
	kr := ringWith(t, bobPriv, alicePriv)
	sealed := sealFor(t, bob[0], alice, "hello")

	for _, from := range []string{
		"alice@example.com",
		"  alice@example.com  ",
		"ALICE@Example.COM",
		"Alice <alice@example.com>",
		"\"Alice, A.\" <ALICE@EXAMPLE.COM>",
	} {
		res, err := kr.DecryptFrom(sealed, from)
		if err != nil {
			t.Fatalf("decrypt for %q: %v", from, err)
		}
		if !res.SenderVerified {
			t.Errorf("From %q was not matched to alice@example.com, so a genuine signature "+
				"reads as unverified. A binding that only works for a bare address rejects "+
				"most real mail.", from)
		}
	}
}

// An empty expected sender must never be treated as "matches anything". That is
// the failure mode where a caller that forgets to pass the address silently
// gets the old, broken behaviour back.
func TestAnUnknownSenderIsNeverVerified(t *testing.T) {
	_, bobPriv := newTestEntity(t, "Bob", "bob@example.com")
	bob, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(bobPriv))
	if err != nil {
		t.Fatal(err)
	}
	alice, alicePriv := newTestEntity(t, "Alice", "alice@example.com")
	kr := ringWith(t, bobPriv, alicePriv)
	sealed := sealFor(t, bob[0], alice, "hello")

	for _, from := range []string{"", "   ", "not-an-address", "someone@else.example"} {
		res, err := kr.DecryptFrom(sealed, from)
		if err != nil {
			t.Fatalf("decrypt for %q: %v", from, err)
		}
		if res.SenderVerified {
			t.Errorf("From %q produced SenderVerified=true. An address that cannot be matched "+
				"to the signing key is not a verified sender, and an empty one least of all.", from)
		}
	}
}

// Decrypt keeps its old signature for callers that do their own binding
// (internal/chat compares fingerprints against the verified peer), and must
// never claim a verified sender on its own.
func TestPlainDecryptNeverClaimsAVerifiedSender(t *testing.T) {
	_, bobPriv := newTestEntity(t, "Bob", "bob@example.com")
	bob, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(bobPriv))
	if err != nil {
		t.Fatal(err)
	}
	alice, alicePriv := newTestEntity(t, "Alice", "alice@example.com")
	kr := ringWith(t, bobPriv, alicePriv)
	sealed := sealFor(t, bob[0], alice, "hello")

	res, err := kr.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if res.Signature != pgp.SigValid {
		t.Fatalf("signature verdict = %v", res.Signature)
	}
	if res.SenderVerified {
		t.Error("Decrypt with no expected sender reported SenderVerified=true; it has nothing " +
			"to compare against and must say so")
	}
	if !strings.EqualFold(res.SignedByFingerprint, res.SignedByFingerprint) {
		t.Fatal("unreachable")
	}
	if res.SignedByFingerprint == "" {
		t.Error("SignedByFingerprint is empty on a valid signature, so a caller doing its own " +
			"binding has nothing to bind to")
	}
}
