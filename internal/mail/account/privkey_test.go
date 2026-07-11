package account

// privkey_test.go — FetchPrivateKey must POST the mailbox credential to
// the account's own HTTPS host, refuse non-public/loopback domains
// (SSRF), and return only a genuine armored private key. A stub
// RoundTripper stands in for the server so the test needs no network.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

const testArmoredPriv = "-----BEGIN PGP PRIVATE KEY BLOCK-----\n\nkeydata\n-----END PGP PRIVATE KEY BLOCK-----"

func TestFetchPrivateKeySuccess(t *testing.T) {
	var gotURL, gotMethod string
	var gotBody map[string]string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		return jsonResp(http.StatusOK, `{"email":"u@example.com","armored_private_key":`+jsonString(testArmoredPriv)+`}`), nil
	})}

	got, err := FetchPrivateKey(context.Background(), client, "u@example.com", "s3cret")
	if err != nil {
		t.Fatalf("FetchPrivateKey: %v", err)
	}
	if !strings.Contains(got, "BEGIN PGP PRIVATE KEY BLOCK") {
		t.Fatalf("returned key not armored: %q", got)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotURL != "https://example.com/api/v1/members/vayumail-privkey" {
		t.Errorf("url = %s", gotURL)
	}
	if gotBody["email"] != "u@example.com" || gotBody["password"] != "s3cret" {
		t.Errorf("body = %v, want email+password", gotBody)
	}
}

func TestFetchPrivateKeyRejectsLoopbackDomain(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResp(http.StatusOK, `{}`), nil
	})}
	for _, addr := range []string{"u@localhost", "u@127.0.0.1", "u@internal"} {
		if _, err := FetchPrivateKey(context.Background(), client, addr, "p"); !errors.Is(err, ErrNoPrivateKey) {
			t.Errorf("%s: err = %v, want ErrNoPrivateKey", addr, err)
		}
	}
	if called {
		t.Fatal("transport was called for a rejected domain (SSRF guard bypassed)")
	}
}

func TestFetchPrivateKeyNon200(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusUnauthorized, `{"error":"invalid-credentials"}`), nil
	})}
	if _, err := FetchPrivateKey(context.Background(), client, "u@example.com", "wrong"); !errors.Is(err, ErrNoPrivateKey) {
		t.Fatalf("err = %v, want ErrNoPrivateKey", err)
	}
}

func TestFetchPrivateKeyRejectsNonKeyBody(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `{"email":"u@example.com","armored_private_key":"not a key"}`), nil
	})}
	if _, err := FetchPrivateKey(context.Background(), client, "u@example.com", "p"); !errors.Is(err, ErrNoPrivateKey) {
		t.Fatalf("err = %v, want ErrNoPrivateKey", err)
	}
}

// jsonString quotes s as a JSON string literal.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
