package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

func testService(t *testing.T) *service {
	t.Helper()
	seed := bytes.Repeat([]byte{0x11}, ed25519.SeedSize)
	return newService(serviceConfig{
		Server:   "mail.example.com",
		IMAPPort: 993,
		SMTPPort: 587,
		TTL:      900,
		Users:    map[string]string{"user@example.com": "app-password"},
		Key:      ed25519.NewKeyFromSeed(seed),
	})
}

// TestProvisionRoundTrip proves the reference server's payload verifies
// with the client's own ParseAndVerify — the two canonical-JSON
// implementations must agree byte-for-byte (ADR-0003/0008).
func TestProvisionRoundTrip(t *testing.T) {
	svc := testService(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /provision", svc.handleExchange)
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	client := hostRewriteClient(srv)

	// The token endpoint must be an https host within the mailbox domain now
	// (audit M14/M15); the client's transport routes that in-domain URL to the
	// local test server so the round-trip still runs against real validation.
	payload, err := svc.buildPayload("user@example.com", "https://mail.example.com/provision")
	if err != nil {
		t.Fatal(err)
	}

	verified, err := account.ParseAndVerify([]byte(payload), time.Now())
	if err != nil {
		t.Fatalf("client rejected server payload: %v", err)
	}
	if verified.Server != "mail.example.com" || verified.Username != "user@example.com" {
		t.Fatalf("payload fields: %+v", verified)
	}

	creds, err := account.ExchangeToken(context.Background(), client, verified)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if creds.IMAPPassword != "app-password" {
		t.Errorf("imap password = %q", creds.IMAPPassword)
	}

	// The token is single use: a second exchange must be rejected.
	if _, err := account.ExchangeToken(context.Background(), client, verified); err == nil {
		t.Error("token reuse must fail")
	}
}

// rtFunc adapts a function to http.RoundTripper.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// hostRewriteClient returns srv's TLS client with a transport that rewrites
// every request's host to the local server. It lets a test present a real
// in-domain https URL — which ParseAndVerify/ExchangeToken require — while the
// request actually reaches the loopback httptest server (cert is trusted by
// srv.Client()).
func hostRewriteClient(srv *httptest.Server) *http.Client {
	c := srv.Client()
	addr := srv.Listener.Addr().String()
	base := c.Transport
	c.Transport = rtFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Host = addr
		return base.RoundTrip(r)
	})
	return c
}

func TestProvisionTamperRejected(t *testing.T) {
	svc := testService(t)
	payload, err := svc.buildPayload("user@example.com", "https://mail.example.com/provision")
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte in the middle of the base64url payload.
	b := []byte(payload)
	b[len(b)/2] ^= 0x01
	if _, err := account.ParseAndVerify(b, time.Now()); err == nil {
		t.Fatal("tampered payload must not verify")
	}
}

func TestProvisionUnknownUser(t *testing.T) {
	svc := testService(t)
	if _, err := svc.buildPayload("stranger@example.com", "https://x/provision"); err != nil {
		// buildPayload itself does not gate on the user list (payloadFor
		// does), so it succeeds; the exchange must still reject.
		t.Fatal(err)
	}
	// Exchange with a token for an unknown user: server returns no
	// password. Simulate via the handler directly.
	body, _ := json.Marshal(map[string]string{"token": "bogus", "username": "stranger@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/provision", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	svc.handleExchange(rr, req)
	if rr.Code != http.StatusGone {
		t.Fatalf("unknown token status = %d, want 410", rr.Code)
	}
}

func TestProvisionTokenExpiry(t *testing.T) {
	svc := testService(t)
	// Manually insert an already-expired token.
	svc.tokens["expired"] = pendingToken{
		email:   "user@example.com",
		expires: time.Now().Add(-time.Minute),
	}
	body, _ := json.Marshal(map[string]string{"token": "expired", "username": "user@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/provision", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	svc.handleExchange(rr, req)
	if rr.Code != http.StatusGone {
		t.Fatalf("expired token status = %d, want 410", rr.Code)
	}
	// Pruning removed it from the map.
	if _, ok := svc.tokens["expired"]; ok {
		t.Error("expired token not pruned")
	}
}

func TestProvisionPayloadIsValidBase64URL(t *testing.T) {
	svc := testService(t)
	payload, err := svc.buildPayload("user@example.com", "https://x/provision")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(payload, "+/=") {
		t.Errorf("payload is not base64url (contains +/=): %q", payload[:20])
	}
}
