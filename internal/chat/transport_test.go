package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestTransport wires a Transport to an httptest server via the
// rewriting RoundTripper, so a public "example.com" domain passes the SSRF
// guard while traffic reaches the local server.
func newTestTransport(t *testing.T, h http.Handler) (*Transport, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return newTransport(rewriteClient(t, srv.URL).Transport), "example.com"
}

func TestConnectHappyAndAuth(t *testing.T) {
	fake := newFakeTalk()
	fake.creds["u@example.com"] = "pw"
	tp, domain := newTestTransport(t, fake)

	tok, err := tp.Connect(context.Background(), domain, "u@example.com", "pw")
	if err != nil || tok == "" {
		t.Fatalf("Connect: tok=%q err=%v", tok, err)
	}
	if _, err := tp.Connect(context.Background(), domain, "u@example.com", "wrong"); !errors.Is(err, ErrTalkAuth) {
		t.Fatalf("bad password err = %v, want ErrTalkAuth", err)
	}
}

func TestConnectDisabled(t *testing.T) {
	tp, domain := newTestTransport(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	if _, err := tp.Connect(context.Background(), domain, "u@example.com", "pw"); !errors.Is(err, ErrTalkDisabled) {
		t.Fatalf("err = %v, want ErrTalkDisabled", err)
	}
}

func TestSendTooLargeAndRateLimit(t *testing.T) {
	fake := newFakeTalk()
	fake.creds["u@example.com"] = "pw"
	tp, domain := newTestTransport(t, fake)
	tok, _ := tp.Connect(context.Background(), domain, "u@example.com", "pw")

	big := base64.StdEncoding.EncodeToString(make([]byte, 65537))
	if _, _, err := tp.Send(context.Background(), domain, tok, "b@example.com", big, 60, "store"); !errors.Is(err, ErrTalk) {
		t.Fatalf("oversize err = %v, want ErrTalk", err)
	}

	rl, domain2 := newTestTransport(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	if _, _, err := rl.Send(context.Background(), domain2, "tok", "b@example.com", "AAAA", 60, "store"); !errors.Is(err, ErrTalk) {
		t.Fatalf("429 err = %v, want ErrTalk", err)
	}
}

// TestStreamStoreFlush queues a store-mode envelope while the recipient is
// offline, then opens the recipient's stream and confirms the backlog
// flushes as a typed Envelope event.
func TestStreamStoreFlush(t *testing.T) {
	fake := newFakeTalk()
	fake.creds["a@example.com"] = "pw"
	fake.creds["b@example.com"] = "pw"
	tp, domain := newTestTransport(t, fake)

	aTok, _ := tp.Connect(context.Background(), domain, "a@example.com", "pw")
	id, delivered, err := tp.Send(context.Background(), domain, aTok, "b@example.com",
		base64.StdEncoding.EncodeToString([]byte("hi")), 120, "store")
	if err != nil || delivered {
		t.Fatalf("Send store offline: id=%q delivered=%v err=%v", id, delivered, err)
	}

	bTok, _ := tp.Connect(context.Background(), domain, "b@example.com", "pw")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := tp.OpenStream(ctx, bTok, domain)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	select {
	case ev := <-stream:
		env, ok := ev.(Envelope)
		if !ok || env.ID != id {
			t.Fatalf("event = %#v, want Envelope id=%s", ev, id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for flushed envelope")
	}
}

func TestStreamRejectsBadToken(t *testing.T) {
	fake := newFakeTalk()
	tp, domain := newTestTransport(t, fake)
	if _, err := tp.OpenStream(context.Background(), "nope", domain); !errors.Is(err, ErrTalkAuth) {
		t.Fatalf("err = %v, want ErrTalkAuth", err)
	}
}

// TestSSRFGuard confirms loopback/internal domains are refused before any
// request leaves, and that a redirecting server is not followed.
func TestSSRFGuard(t *testing.T) {
	hit := false
	tp, _ := newTestTransport(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
	}))
	for _, d := range []string{"localhost", "127.0.0.1", "internal", ""} {
		if _, err := tp.Connect(context.Background(), d, "u@x", "p"); !errors.Is(err, ErrTalk) {
			t.Errorf("domain %q: err = %v, want ErrTalk", d, err)
		}
	}
	if hit {
		t.Fatal("transport reached a rejected domain (SSRF guard bypassed)")
	}
}

func TestRedirectRefused(t *testing.T) {
	fake := newFakeTalk()
	fake.creds["u@example.com"] = "pw"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example.net/steal", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	tp := newTransport(rewriteClient(t, srv.URL).Transport)
	// A refused redirect surfaces as a non-200 status -> ErrTalk, never a
	// followed request to another host.
	_, err := tp.Connect(context.Background(), "example.com", "u@example.com", "pw")
	if err == nil {
		t.Fatal("expected error on redirect, got nil")
	}
	if strings.Contains(err.Error(), "evil.example.net") {
		t.Fatalf("redirect appears to have been followed: %v", err)
	}
}
