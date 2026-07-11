package chat

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

// waitEvent drains a manager's event channel until pred returns true or the
// deadline passes.
func waitEvent(t *testing.T, m *Manager, pred func(Event) bool) Event {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-m.Events():
			if pred(ev) {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out waiting for event")
			return nil
		}
	}
}

// twoParty sets up a fake server and two managers whose keyrings can talk:
// A holds its private key plus B's public key, and vice versa.
func twoParty(t *testing.T) (fake *fakeTalk, a, b *Manager, srv *httptest.Server) {
	t.Helper()
	aPriv, aPub := genArmoredKeypair(t, "Alice", "a@example.com")
	bPriv, bPub := genArmoredKeypair(t, "Bob", "b@example.com")

	fake = newFakeTalk()
	fake.creds["a@example.com"] = "pw"
	fake.creds["b@example.com"] = "pw"
	fake.pubkeys["a@example.com"] = aPub
	fake.pubkeys["b@example.com"] = bPub

	srv = httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	client := rewriteClient(t, srv.URL)

	aKR := pgp.NewKeyring()
	mustImport(t, aKR, aPriv, bPub)
	bKR := pgp.NewKeyring()
	mustImport(t, bKR, bPriv, aPub)

	a = New(Config{Keyring: aKR, SelfEmail: "a@example.com", Domain: "example.com",
		Credential: staticCred("pw"), HTTPClient: client})
	b = New(Config{Keyring: bKR, SelfEmail: "b@example.com", Domain: "example.com",
		Credential: staticCred("pw"), HTTPClient: client})
	return fake, a, b, srv
}

func mustImport(t *testing.T, kr *pgp.Keyring, blobs ...string) {
	t.Helper()
	for _, b := range blobs {
		if _, err := kr.ImportArmored([]byte(b)); err != nil {
			t.Fatalf("import key: %v", err)
		}
	}
}

func staticCred(pw string) func() (string, error) {
	return func() (string, error) { return pw, nil }
}

func isOnline(ev Event) bool {
	c, ok := ev.(ConnState)
	return ok && c.Online
}

// TestManagerEndToEnd drives the full path: A sends, B receives and
// decrypts, B acks, A gets a read receipt.
func TestManagerEndToEnd(t *testing.T) {
	_, a, b, _ := twoParty(t)
	ctx := context.Background()
	a.Start(ctx)
	b.Start(ctx)
	defer a.Close()
	defer b.Close()

	waitEvent(t, a, isOnline)
	waitEvent(t, b, isOnline)

	const secret = "meet at the pier"
	if _, err := a.Send(ctx, "b@example.com", secret, time.Minute, "live"); err != nil {
		t.Fatalf("A.Send: %v", err)
	}

	incoming := waitEvent(t, b, func(ev Event) bool { _, ok := ev.(IncomingMessage); return ok }).(IncomingMessage)
	if incoming.Plaintext != secret {
		t.Fatalf("B received %q, want %q", incoming.Plaintext, secret)
	}
	if incoming.Peer != "a@example.com" {
		t.Errorf("peer = %q, want a@example.com", incoming.Peer)
	}

	if err := b.Ack(ctx, incoming.ID); err != nil {
		t.Fatalf("B.Ack: %v", err)
	}
	read := waitEvent(t, a, func(ev Event) bool { _, ok := ev.(MessageRead); return ok }).(MessageRead)
	if read.ID != incoming.ID {
		t.Fatalf("read receipt id = %q, want %q", read.ID, incoming.ID)
	}
	if read.Peer != "b@example.com" {
		t.Errorf("read.Peer = %q, want b@example.com", read.Peer)
	}
}

// TestManagerVerifyPeer fetches a peer's key and emits PeerKey.
func TestManagerVerifyPeer(t *testing.T) {
	_, a, _, _ := twoParty(t)
	ctx := context.Background()
	if err := a.VerifyPeer(ctx, "b@example.com"); err != nil {
		t.Fatalf("VerifyPeer: %v", err)
	}
	ev := waitEvent(t, a, func(ev Event) bool { _, ok := ev.(PeerKey); return ok }).(PeerKey)
	if ev.Peer != "b@example.com" || ev.Fingerprint == "" {
		t.Fatalf("PeerKey = %#v", ev)
	}
	if ev.Verified {
		t.Error("fresh peer should not be verified")
	}
	if err := a.SetPeerVerified(ctx, ev.Fingerprint, true); err != nil {
		t.Fatalf("SetPeerVerified: %v", err)
	}
	if err := a.VerifyPeer(ctx, "b@example.com"); err != nil {
		t.Fatalf("VerifyPeer 2: %v", err)
	}
	ev2 := waitEvent(t, a, func(ev Event) bool { _, ok := ev.(PeerKey); return ok }).(PeerKey)
	if !ev2.Verified {
		t.Error("peer should be verified after SetPeerVerified")
	}
}

func TestManagerSendAuthFailure(t *testing.T) {
	_, a, _, _ := twoParty(t)
	a.credential = func() (string, error) { return "wrong", nil }
	if _, err := a.Send(context.Background(), "b@example.com", "hi", time.Minute, "store"); err == nil {
		t.Fatal("expected auth error with wrong credential")
	}
}

func TestSafetyNumber(t *testing.T) {
	got := SafetyNumber("aaaa1111bbbb2222cccc3333")
	want := "AAAA 1111 BBBB 2222 CCCC\n3333"
	if got != want {
		t.Fatalf("SafetyNumber = %q, want %q", got, want)
	}
}
