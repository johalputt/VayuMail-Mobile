// Package test holds the integration test suite. Everything runs against
// fixtures in test/fixtures/ — zero network calls in CI.
package test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

// testNow is safely inside the validity window of valid.b64 (expires
// 2030-01-01) and after the expiry of expired.b64 (2000-01-01).
var testNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func readQRFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("fixtures", "qr", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func TestParseAndVerify(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		wantErr error
	}{
		{"valid payload verifies", "valid.b64", nil},
		{"expired payload rejected", "expired.b64", account.ErrExpired},
		{"tampered signature rejected", "tampered.b64", account.ErrInvalidSignature},
		{"unknown version rejected", "badversion.b64", account.ErrUnknownVersion},
		{"plaintext transport rejected", "plaintls.b64", account.ErrInsecureTransport},
		{"invalid port rejected", "badport.b64", account.ErrInvalidPort},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := account.ParseAndVerify(readQRFixture(t, tt.fixture), testNow)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("want success, got %v", err)
				}
				if payload.Server != "mail.example.com" {
					t.Errorf("server = %q", payload.Server)
				}
				if payload.Username != "user@example.com" {
					t.Errorf("username = %q", payload.Username)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("want %v, got %v", tt.wantErr, err)
			}
			if payload != nil {
				t.Fatal("rejected payload must be nil — no field may be usable")
			}
		})
	}
}

func TestParseAndVerifyMalformed(t *testing.T) {
	for _, raw := range []string{"", "!!!not-base64url!!!", "aGVsbG8"} {
		if _, err := account.ParseAndVerify([]byte(raw), testNow); !errors.Is(err, account.ErrMalformedPayload) {
			t.Errorf("input %q: want ErrMalformedPayload, got %v", raw, err)
		}
	}
}

func TestVerifiedPayloadToConfig(t *testing.T) {
	payload, err := account.ParseAndVerify(readQRFixture(t, "valid.b64"), testNow)
	if err != nil {
		t.Fatal(err)
	}
	cfg := payload.Config("alias-1")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config from verified payload must validate: %v", err)
	}
	if cfg.KeystoreAlias != "alias-1" {
		t.Errorf("keystore alias = %q", cfg.KeystoreAlias)
	}
}

func TestExchangeToken(t *testing.T) {
	payload, err := account.ParseAndVerify(readQRFixture(t, "valid.b64"), testNow)
	if err != nil {
		t.Fatal(err)
	}

	// The token endpoint must be an https host within the mailbox domain now
	// (audit M14/M15). Present a real in-domain URL and route it to the local
	// test server via the client's transport (the mailbox is user@example.com).
	const endpoint = "https://mail.example.com/exchange"

	t.Run("success returns credentials", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			w.Write([]byte(`{"imap_password":"pw-imap","smtp_password":"pw-smtp"}`))
		}))
		defer srv.Close()
		p := *payload
		p.TokenEndpoint = endpoint
		creds, err := account.ExchangeToken(t.Context(), hostRewriteClient(srv), &p)
		if err != nil {
			t.Fatal(err)
		}
		if creds.IMAPPassword != "pw-imap" {
			t.Errorf("imap password = %q", creds.IMAPPassword)
		}
	})

	t.Run("gone token maps to ErrTokenExpired", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusGone)
		}))
		defer srv.Close()
		p := *payload
		p.TokenEndpoint = endpoint
		if _, err := account.ExchangeToken(t.Context(), hostRewriteClient(srv), &p); !errors.Is(err, account.ErrTokenExpired) {
			t.Fatalf("want ErrTokenExpired, got %v", err)
		}
	})

	t.Run("empty credential body rejected", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{}`))
		}))
		defer srv.Close()
		p := *payload
		p.TokenEndpoint = endpoint
		if _, err := account.ExchangeToken(t.Context(), hostRewriteClient(srv), &p); !errors.Is(err, account.ErrTokenInvalid) {
			t.Fatalf("want ErrTokenInvalid, got %v", err)
		}
	})
}

// rtFunc adapts a function to http.RoundTripper.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// hostRewriteClient returns srv's TLS client with a transport that rewrites
// every request's host to the local server, so a test can present a real
// in-domain https URL (which ExchangeToken now requires) yet reach the loopback
// httptest server.
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
