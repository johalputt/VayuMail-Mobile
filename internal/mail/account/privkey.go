package account

// privkey.go — fetch the mailbox's own PGP private key from VayuPress so
// the app can decrypt received mail on-device. The endpoint authenticates
// with the same mailbox credential the app already holds and returns the
// owner's armored private key over TLS (VayuPress ADR: private-key sync).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrNoPrivateKey is returned when the server has no key or declines to
// serve one (e.g. bad credential).
var ErrNoPrivateKey = errors.New("account: no private key available")

// maxPrivKeyBytes caps the response — an armored keypair is a few KiB.
const maxPrivKeyBytes = 256 << 10

// privKeyResponse is the VayuPress endpoint's JSON shape.
type privKeyResponse struct {
	Email             string `json:"email"`
	ArmoredPrivateKey string `json:"armored_private_key"`
}

// FetchPrivateKey retrieves the armored PGP private key for email from
// that address's VayuPress server, authenticating with password (the
// mailbox password or an app password). Returns the armored key on
// success. Transport is HTTPS only and redirects are refused, so the
// credential and key never traverse an unverified hop.
func FetchPrivateKey(ctx context.Context, client *http.Client, email, password string) (string, error) {
	domain := domainOf(email)
	if !publicMailDomain(domain) {
		return "", fmt.Errorf("%w: bad domain", ErrNoPrivateKey)
	}
	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		return "", err
	}
	url := "https://" + domain + "/api/v1/members/vayumail-privkey"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoPrivateKey, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: server returned %d", ErrNoPrivateKey, resp.StatusCode)
	}
	var pr privKeyResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPrivKeyBytes)).Decode(&pr); err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoPrivateKey, err)
	}
	if !strings.Contains(pr.ArmoredPrivateKey, "BEGIN PGP PRIVATE KEY BLOCK") {
		return "", ErrNoPrivateKey
	}
	return pr.ArmoredPrivateKey, nil
}

// domainOf returns the lowercased domain part of an address.
func domainOf(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 {
		return strings.ToLower(email[i+1:])
	}
	return ""
}
