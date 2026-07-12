package chat

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

// TestHandleEnvelopeResyncsOnDecryptFailure reproduces the production hazard: a
// message the WEB composed is encrypted to the mailbox's CURRENT server key, but
// this device is still holding an OLDER key for the same address, so the first
// decrypt fails. The manager must re-fetch the authoritative key (ResyncKey) and
// retry, rather than silently dropping the message — which is what made
// web→app messages "not arrive".
func TestHandleEnvelopeResyncsOnDecryptFailure(t *testing.T) {
	// Two distinct keypairs for the SAME address: keyA (stale, on the device)
	// and keyB (current, on the server and what the message is encrypted to).
	privA, _ := genArmoredKeypair(t, "Self", "self@example.com")
	privB, _ := genArmoredKeypair(t, "Self", "self@example.com")

	krB := pgp.NewKeyring()
	if _, err := krB.ImportArmored([]byte(privB)); err != nil {
		t.Fatalf("import keyB: %v", err)
	}
	const msg = "web-composed secret"
	ct, err := krB.Encrypt([]byte(msg), []string{"self@example.com"}, "self@example.com")
	if err != nil {
		t.Fatalf("encrypt to current key: %v", err)
	}

	// The device starts with only the stale key — it cannot open the message.
	krDevice := pgp.NewKeyring()
	if _, err := krDevice.ImportArmored([]byte(privA)); err != nil {
		t.Fatalf("import keyA: %v", err)
	}

	resynced := false
	m := New(Config{
		Keyring:   krDevice,
		SelfEmail: "self@example.com",
		ResyncKey: func(context.Context) error {
			resynced = true
			_, e := krDevice.ImportArmored([]byte(privB)) // the self-heal
			return e
		},
	})
	m.ctx = context.Background()

	m.handleEnvelope(Envelope{
		ID:         "e1",
		From:       "peer@example.com",
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	})

	if !resynced {
		t.Fatal("decrypt failure should have triggered a key resync")
	}
	select {
	case ev := <-m.Events():
		im, ok := ev.(IncomingMessage)
		if !ok {
			t.Fatalf("expected IncomingMessage, got %T", ev)
		}
		if im.Plaintext != msg {
			t.Fatalf("plaintext = %q, want %q", im.Plaintext, msg)
		}
	default:
		t.Fatal("no message emitted after resync — self-heal did not recover it")
	}
}

// TestHandleEnvelopeRateLimitsResync ensures a burst of undecryptable envelopes
// cannot stampede the key endpoint: only the first triggers a resync.
func TestHandleEnvelopeRateLimitsResync(t *testing.T) {
	priv, _ := genArmoredKeypair(t, "Self", "self@example.com")
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored([]byte(priv)); err != nil {
		t.Fatalf("import: %v", err)
	}
	calls := 0
	m := New(Config{
		Keyring:   kr,
		SelfEmail: "self@example.com",
		ResyncKey: func(context.Context) error { calls++; return nil },
	})
	m.ctx = context.Background()

	// Garbage ciphertext never decrypts; feed several in a row.
	bad := base64.StdEncoding.EncodeToString([]byte("not pgp at all"))
	for i := 0; i < 5; i++ {
		m.handleEnvelope(Envelope{ID: "x", From: "p@example.com", Ciphertext: bad})
	}
	if calls != 1 {
		t.Fatalf("resync called %d times, want exactly 1 (rate-limited)", calls)
	}
}
