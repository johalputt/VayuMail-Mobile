package account

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// signedCode builds a base64url provisioning code signed by a freshly
// generated key (self-certifying, as the real format is), letting a test set
// any field it likes and observe whether ParseAndVerify accepts it.
func signedCode(t *testing.T, mutate func(p *ProvisionPayload)) []byte {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	p := &ProvisionPayload{
		V:             1,
		Server:        "mail.example.com",
		IMAPPort:      993,
		IMAPTLS:       "tls",
		SMTPPort:      465,
		SMTPTLS:       "tls",
		Username:      "alice@example.com",
		DisplayName:   "Alice",
		Token:         "one-time-token",
		TokenEndpoint: "https://example.com/api/v1/vayumail-provision/exchange",
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
		ServerPubkey:  base64.RawURLEncoding.EncodeToString(pub),
	}
	if mutate != nil {
		mutate(p)
	}
	canonical, err := p.CanonicalJSON()
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	p.Sig = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, canonical))
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return []byte(base64.RawURLEncoding.EncodeToString(raw))
}

func TestParseAndVerifyAcceptsInDomain(t *testing.T) {
	_, err := ParseAndVerify(signedCode(t, nil), time.Now())
	if err != nil {
		t.Fatalf("well-formed in-domain code rejected: %v", err)
	}
}

// A validly-SIGNED code (the attacker owns the key) must still be rejected when
// it steers the device off the mailbox domain or at a private/LAN host —
// signature integrity is not authenticity (audit M14/M15).
func TestParseAndVerifyRejectsOffDomainAndSSRF(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *ProvisionPayload)
	}{
		{"server other domain", func(p *ProvisionPayload) { p.Server = "mail.attacker.tld" }},
		{"server private ip", func(p *ProvisionPayload) { p.Server = "192.168.1.10" }},
		{"endpoint other domain", func(p *ProvisionPayload) { p.TokenEndpoint = "https://attacker.tld/x" }},
		{"endpoint link-local ssrf", func(p *ProvisionPayload) { p.TokenEndpoint = "http://169.254.169.254/latest" }},
		{"endpoint not https", func(p *ProvisionPayload) { p.TokenEndpoint = "http://example.com/x" }},
		{"endpoint odd port", func(p *ProvisionPayload) { p.TokenEndpoint = "https://example.com:8443/x" }},
		{"endpoint userinfo", func(p *ProvisionPayload) { p.TokenEndpoint = "https://user@example.com/x" }},
		{"username no domain", func(p *ProvisionPayload) { p.Username = "alice" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseAndVerify(signedCode(t, c.mutate), time.Now())
			if err == nil {
				t.Fatalf("expected rejection for %q, got nil", c.name)
			}
			if !errors.Is(err, ErrMalformedPayload) && !errors.Is(err, ErrInsecureTransport) {
				t.Fatalf("unexpected error kind for %q: %v", c.name, err)
			}
		})
	}
}

// A subdomain of the mailbox domain is allowed (real deployments use
// mail.<domain> / api.<domain>).
func TestParseAndVerifyAllowsSubdomain(t *testing.T) {
	code := signedCode(t, func(p *ProvisionPayload) {
		p.Server = "imap.example.com"
		p.TokenEndpoint = "https://api.example.com/exchange"
	})
	if _, err := ParseAndVerify(code, time.Now()); err != nil {
		t.Fatalf("subdomain code rejected: %v", err)
	}
}
