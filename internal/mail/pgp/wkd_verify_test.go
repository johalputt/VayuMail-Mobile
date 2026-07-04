package pgp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// newTestPubKey generates a key for email and returns its binary transferable
// public key (WKD wire format).
func newTestPubKey(t *testing.T, name, email string) []byte {
	t.Helper()
	ent, err := openpgp.NewEntity(name, "", email, nil)
	if err != nil {
		t.Fatalf("NewEntity(%s): %v", email, err)
	}
	var buf bytes.Buffer
	if err := ent.Serialize(&buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return buf.Bytes()
}

// wkdClient returns an http.Client that routes every request to a server
// serving body (the WKD key bytes).
func wkdClient(t *testing.T, body []byte) *http.Client {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	c := srv.Client()
	base := c.Transport
	addr := srv.Listener.Addr().String()
	c.Transport = rtFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Host = addr
		return base.RoundTrip(r)
	})
	return c
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestDiscoverWKDRejectsAddressMismatch verifies the client accepts a WKD key
// that carries a User ID for the requested address, but rejects one that does
// not (a different-address key that would otherwise be mis-associated).
func TestDiscoverWKDRejectsAddressMismatch(t *testing.T) {
	// Matching key for bob@example.com → accepted.
	bob := newTestPubKey(t, "Bob", "bob@example.com")
	ents, err := DiscoverWKD(context.Background(), wkdClient(t, bob), "bob@example.com")
	if err != nil {
		t.Fatalf("matching key rejected: %v", err)
	}
	if !entitiesHaveEmail(ents, "bob@example.com") {
		t.Fatal("returned entities do not carry the requested address")
	}

	// Server returns eve's key when bob was requested → rejected.
	eve := newTestPubKey(t, "Eve", "eve@evil.example")
	if _, err := DiscoverWKD(context.Background(), wkdClient(t, eve), "bob@example.com"); err == nil {
		t.Fatal("expected rejection of a WKD response for a different address")
	}
}
