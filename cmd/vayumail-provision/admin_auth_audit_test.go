package main

// admin_auth_audit_test.go — minting a setup code is an administrative act and
// must be authenticated.
//
// The finding, in the attacker's voice: I need two requests and no credentials.
//
//	GET  /code?user=victim@example.com   -> a signed payload containing a token
//	POST /provision {token, username}    -> {"imap_password": "..."}
//
// That is the victim's mail password, in plaintext, from an endpoint that
// listened on every interface by default. I do not need to guess the password,
// intercept anything, or defeat the Ed25519 signature — the signature proves
// the payload came from the server, which it did, because the server hands one
// to whoever asks.
//
// The code carried a warning to "serve this behind TLS in production", which is
// worse than silence: TLS is the wrong control. The problem was never that
// somebody could read the exchange off the wire; it was that anybody could ask.
//
// This binary is the reference implementation other operators copy, which is
// precisely why it should model the control rather than a note about one.

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func authTestService(t *testing.T) *service {
	t.Helper()
	seed := bytes.Repeat([]byte{0x22}, ed25519.SeedSize)
	return newService(serviceConfig{
		Server:     "mail.example.com",
		IMAPPort:   993,
		SMTPPort:   587,
		TTL:        900,
		Users:      map[string]string{"victim@example.com": "the-real-password"},
		Key:        ed25519.NewKeyFromSeed(seed),
		AdminToken: "s3cret-admin-token",
	})
}

func codeRequest(t *testing.T, svc *service, header, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/code?user=victim@example.com"
	if query != "" {
		url += "&admin=" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	svc.handleSetupCode(rec, req)
	return rec
}

// The whole chain, unauthenticated. This is the finding.
func TestAnUnauthenticatedCallerCannotWalkFromSetupCodeToPassword(t *testing.T) {
	svc := authTestService(t)

	rec := codeRequest(t, svc, "", "")
	if rec.Code == http.StatusOK {
		t.Fatalf("GET /code with no credential returned 200 and a setup code.\n"+
			"body: %s\nThat payload carries a redeemable token, and POST /provision turns it "+
			"into the account's mail password. Minting a setup code is an administrative act.",
			strings.TrimSpace(rec.Body.String()))
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}

	// And nothing may have been minted as a side effect of the refused call —
	// a handler that checks credentials after issuing the token has already
	// lost, because the token is the thing worth stealing.
	svc.mu.Lock()
	n := len(svc.tokens)
	svc.mu.Unlock()
	if n != 0 {
		t.Errorf("%d token(s) were minted by a refused request. The credential check has to "+
			"happen before the token exists, not before the response is written.", n)
	}
}

// A wrong credential is a refusal, not a fallback to open.
func TestAWrongAdminTokenIsRefused(t *testing.T) {
	svc := authTestService(t)
	for _, c := range []struct{ header, query string }{
		{"Bearer wrong", ""},
		{"Bearer ", ""},
		{"s3cret-admin-token", ""}, // right value, missing the Bearer scheme
		{"", "wrong"},
		{"", "s3cret-admin-token-with-suffix"},
		{"", "s3cret-admin-toke"},
	} {
		rec := codeRequest(t, svc, c.header, c.query)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header=%q query=%q -> status %d, want 401", c.header, c.query, rec.Code)
		}
	}
}

// The operator's own tooling must still work.
func TestTheAdminTokenLetsTheOperatorMintACode(t *testing.T) {
	svc := authTestService(t)

	rec := codeRequest(t, svc, "Bearer s3cret-admin-token", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET /code -> %d: %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) == "" {
		t.Fatal("authenticated GET /code returned an empty body")
	}

	// The query-parameter form too, for tooling that cannot set headers.
	rec = codeRequest(t, svc, "", "s3cret-admin-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin query parameter -> %d", rec.Code)
	}
}

// An unknown user must not be distinguishable from a refused credential before
// the credential is checked — otherwise the endpoint is a user-enumeration
// oracle for anyone on the network.
func TestUserEnumerationIsNotPossibleWithoutTheAdminToken(t *testing.T) {
	svc := authTestService(t)

	known := httptest.NewRequest(http.MethodGet, "/code?user=victim@example.com", nil)
	unknown := httptest.NewRequest(http.MethodGet, "/code?user=nobody@example.com", nil)

	r1, r2 := httptest.NewRecorder(), httptest.NewRecorder()
	svc.handleSetupCode(r1, known)
	svc.handleSetupCode(r2, unknown)

	if r1.Code != r2.Code {
		t.Errorf("a known user returned %d and an unknown user %d without any credential, so "+
			"the endpoint reports which addresses exist on this server", r1.Code, r2.Code)
	}
}

// A service configured with no admin token must refuse to mint rather than
// treating "no token" as "no check" — the failure mode that reintroduces the
// whole finding the first time someone constructs a service without one.
func TestAnEmptyAdminTokenRefusesEverything(t *testing.T) {
	seed := bytes.Repeat([]byte{0x33}, ed25519.SeedSize)
	svc := newService(serviceConfig{
		Server: "mail.example.com", IMAPPort: 993, SMTPPort: 587, TTL: 900,
		Users: map[string]string{"victim@example.com": "pw"},
		Key:   ed25519.NewKeyFromSeed(seed),
	})
	for _, c := range []struct{ header, query string }{{"", ""}, {"Bearer ", ""}, {"", ""}} {
		rec := codeRequest(t, svc, c.header, c.query)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("a service with no admin token answered %d; an unset credential must "+
				"deny, never allow", rec.Code)
		}
	}
}

// The exchange endpoint stays open by design — the app redeeming a token has no
// admin credential — so its protection is that the token is single-use,
// expiring and unguessable. Pin that a redeemed token cannot be replayed.
func TestARedeemedTokenCannotBeReplayed(t *testing.T) {
	svc := authTestService(t)
	payload, err := svc.buildPayload("victim@example.com", "https://mail.example.com/provision")
	if err != nil {
		t.Fatal(err)
	}
	token := tokenFromPayload(t, payload)

	body := func() *strings.Reader {
		return strings.NewReader(`{"token":"` + token + `","username":"victim@example.com"}`)
	}
	first := httptest.NewRecorder()
	svc.handleExchange(first, httptest.NewRequest(http.MethodPost, "/provision", body()))
	if first.Code != http.StatusOK {
		t.Fatalf("first redemption -> %d: %s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	svc.handleExchange(second, httptest.NewRequest(http.MethodPost, "/provision", body()))
	if second.Code == http.StatusOK {
		t.Error("a token was redeemed twice; single use is the only thing standing between " +
			"a leaked setup code and a reusable credential dispenser")
	}
}

func tokenFromPayload(t *testing.T, payload string) string {
	t.Helper()
	raw, err := base64URLDecode(payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var fields struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if fields.Token == "" {
		t.Fatal("payload carried no token")
	}
	return fields.Token
}

// base64URLDecode is the payload's transport encoding (RawURLEncoding), kept
// here so the test decodes exactly what buildPayload produced.
func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
}
