package test

import (
	"strings"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

func TestIsTokenAuth(t *testing.T) {
	cases := map[string]bool{
		account.AuthPassword:    false,
		account.AuthOAuthBearer: true,
		account.AuthXOAuth2:     true,
		"nonsense":              false,
	}
	for mech, want := range cases {
		if got := account.IsTokenAuth(mech); got != want {
			t.Errorf("IsTokenAuth(%q) = %v, want %v", mech, got, want)
		}
	}
}

func TestXOAuth2InitialResponse(t *testing.T) {
	c, err := account.SASLClient(account.AuthXOAuth2, "user@johal.in", "tok123", "mail.johal.in", 993)
	if err != nil {
		t.Fatal(err)
	}
	mech, ir, err := c.Start()
	if err != nil {
		t.Fatal(err)
	}
	if mech != "XOAUTH2" {
		t.Fatalf("mech = %q, want XOAUTH2", mech)
	}
	// Exact wire format: user=<u>^Aauth=Bearer <t>^A^A
	want := "user=user@johal.in\x01auth=Bearer tok123\x01\x01"
	if string(ir) != want {
		t.Fatalf("initial response = %q, want %q", ir, want)
	}
}

func TestOAuthBearerSelected(t *testing.T) {
	c, err := account.SASLClient(account.AuthOAuthBearer, "user@johal.in", "tok", "mail.johal.in", 993)
	if err != nil {
		t.Fatal(err)
	}
	mech, _, err := c.Start()
	if err != nil {
		t.Fatal(err)
	}
	if mech != "OAUTHBEARER" {
		t.Fatalf("mech = %q, want OAUTHBEARER", mech)
	}
}

func TestSASLClientRejectsPassword(t *testing.T) {
	if _, err := account.SASLClient(account.AuthPassword, "u", "p", "h", 993); err == nil ||
		!strings.Contains(err.Error(), "not a token mechanism") {
		t.Fatalf("expected rejection for password mech, got %v", err)
	}
}
