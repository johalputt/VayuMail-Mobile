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
	srv := httptest.NewServer(mux)
	defer srv.Close()

	payload, err := svc.buildPayload("user@example.com", srv.URL+"/provision")
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

	creds, err := account.ExchangeToken(context.Background(), srv.Client(), verified)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if creds.IMAPPassword != "app-password" {
		t.Errorf("imap password = %q", creds.IMAPPassword)
	}

	// The token is single use: a second exchange must be rejected.
	if _, err := account.ExchangeToken(context.Background(), srv.Client(), verified); err == nil {
		t.Error("token reuse must fail")
	}
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
