package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

// canonicalAutoconfigJSON is the exact document VayuPress serves at
// /.well-known/vayumail/autoconfig.json for example.com (see that repo's
// cmd/vayupress/vayuos_mail_autoconfig_test.go golden value). It is duplicated
// here VERBATIM so the app's parser is tested against the real server output:
// the server publishes this shape and the app must consume it, so any drift in
// field names, TLS spellings or the schema version fails CI on whichever side
// moved instead of silently breaking email-only onboarding.
//
// KEEP IN SYNC with VayuPress's vayuMailAutoconfig / VayuMailAutoconfigSchema.
const canonicalAutoconfigJSON = `{"schema":"vayumail-autoconfig/1","domain":"example.com","displayName":"example.com Mail","imap":{"host":"mail.example.com","port":993,"tls":"tls"},"pop3":{"host":"mail.example.com","port":995,"tls":"tls"},"smtp":{"host":"mail.example.com","port":587,"tls":"starttls"},"usernameIsEmail":true,"auth":"password","wkd":true}`

// TestAutoconfigContractParsesServerDocument feeds the canonical VayuPress
// document to the discovery client and asserts it maps to the expected account
// Config — the app half of the cross-repo autoconfig contract.
func TestAutoconfigContractParsesServerDocument(t *testing.T) {
	var gotPath string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(canonicalAutoconfigJSON))
	}))
	defer srv.Close()

	// Point discovery at the test server: rewrite the httptest host into the
	// client via a custom transport keyed on the well-known path.
	client := srv.Client()
	client.Transport = rewriteHost(client.Transport, srv.Listener.Addr().String())

	cfg, err := account.DiscoverAutoconfig(context.Background(), client, "alice@example.com")
	if err != nil {
		t.Fatalf("DiscoverAutoconfig: %v", err)
	}
	if gotPath != "/.well-known/vayumail/autoconfig.json" {
		t.Errorf("fetched path = %q, want the well-known autoconfig path", gotPath)
	}

	if cfg.IMAPHost != "mail.example.com" || cfg.IMAPPort != 993 || cfg.IMAPTLS != account.TLSModeImplicit {
		t.Errorf("IMAP mapped wrong: %s:%d %s", cfg.IMAPHost, cfg.IMAPPort, cfg.IMAPTLS)
	}
	if cfg.SMTPHost != "mail.example.com" || cfg.SMTPPort != 587 || cfg.SMTPTLS != account.TLSModeSTARTTLS {
		t.Errorf("SMTP mapped wrong: %s:%d %s", cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPTLS)
	}
	if cfg.EmailAddress != "alice@example.com" || cfg.Username != "alice@example.com" {
		t.Errorf("identity mapped wrong: email=%q user=%q", cfg.EmailAddress, cfg.Username)
	}
	if cfg.DisplayName != "example.com Mail" {
		t.Errorf("DisplayName = %q, want %q", cfg.DisplayName, "example.com Mail")
	}
	// A draft Config is complete except for the keystore alias the setup flow
	// assigns; filling it must yield a valid, connectable account.
	cfg.KeystoreAlias = "vaultkey"
	if err := cfg.Validate(); err != nil {
		t.Errorf("discovered config is not valid after alias assignment: %v", err)
	}
}

// TestAutoconfigSchemaMatchesServer guards the schema constant so a rename can't
// land without touching this test (and, by the shared value, the server).
func TestAutoconfigSchemaMatchesServer(t *testing.T) {
	if account.AutoconfigSchema != "vayumail-autoconfig/1" {
		t.Errorf("AutoconfigSchema = %q; bumping it requires a coordinated VayuPress change", account.AutoconfigSchema)
	}
	if !strings.Contains(canonicalAutoconfigJSON, `"schema":"`+account.AutoconfigSchema+`"`) {
		t.Error("canonical document schema does not match the client AutoconfigSchema constant")
	}
}

// TestAutoconfigRejectsNonPublicDomain verifies the SSRF guard blocks loopback
// and IP-literal targets before any request is made.
func TestAutoconfigRejectsNonPublicDomain(t *testing.T) {
	for _, addr := range []string{"root@localhost", "x@127.0.0.1", "y@[::1]", "z@nodot"} {
		if _, err := account.DiscoverAutoconfig(context.Background(), http.DefaultClient, addr); err == nil {
			t.Errorf("expected rejection for %q", addr)
		}
	}
}

// rewriteHost redirects every request to addr, so discovery of example.com hits
// the in-process test server instead of the network.
func rewriteHost(base http.RoundTripper, addr string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Host = addr
		return base.RoundTrip(r)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
