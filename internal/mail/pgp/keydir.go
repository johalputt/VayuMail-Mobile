package pgp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// A VayuPress key directory is a small HTTPS service (typically hosted by
// the user's own VayuPress site) that maps email addresses to PGP public
// keys, so VayuMail can auto-import a correspondent's key without the
// recipient having to publish WKD. It is queried only against the URL the
// user configures — VayuMail never contacts a directory on its own.
//
// Contract (what VayuPress must expose):
//
//	GET {baseURL}?email={address}
//	  200  body = the address's ASCII-armored (or binary) public key,
//	       Content-Type application/pgp-keys
//	  404  no key for that address
//
//	GET {baseURL}            (no email query)
//	  200  JSON {"keys":[{"email":"a@x","armored":"-----BEGIN..."}]}
//	       — the full directory, for a one-shot bulk sync.
//
// Both shapes are optional; the client tries the bulk form for a full
// sync and the per-address form for on-demand lookups.

// maxKeyBytes caps a single directory response (keys are a few KiB).
const maxKeyBytes = 1 << 20

// DirectoryKey is one entry in the bulk directory response.
type DirectoryKey struct {
	Email   string `json:"email"`
	Armored string `json:"armored"`
}

type directoryResponse struct {
	Keys []DirectoryKey `json:"keys"`
}

// DiscoverKeyDirectory fetches a single address's public key from the
// directory at baseURL. Returns the parsed entities, or an error if the
// directory has no key for the address.
func DiscoverKeyDirectory(ctx context.Context, client *http.Client, baseURL, email string) (openpgp.EntityList, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := buildDirectoryURL(baseURL, email)
	if err != nil {
		return nil, err
	}
	raw, err := getBytes(ctx, client, u)
	if err != nil {
		return nil, err
	}
	entities, err := parseKeyRing(raw)
	if err != nil {
		return nil, fmt.Errorf("parse directory key for %s: %w", email, err)
	}
	return entities, nil
}

// FetchKeyDirectory pulls the full directory (bulk JSON form) from baseURL.
func FetchKeyDirectory(ctx context.Context, client *http.Client, baseURL string) ([]DirectoryKey, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := buildDirectoryURL(baseURL, "")
	if err != nil {
		return nil, err
	}
	raw, err := getBytes(ctx, client, u)
	if err != nil {
		return nil, err
	}
	var resp directoryResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse directory JSON: %w", err)
	}
	return resp.Keys, nil
}

// buildDirectoryURL validates baseURL (HTTPS only, so keys never travel in
// the clear) and appends the email query when non-empty.
func buildDirectoryURL(baseURL, email string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid key-directory URL: %w", err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("key-directory URL must be https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("key-directory URL has no host")
	}
	if email != "" {
		q := u.Query()
		q.Set("email", email)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func getBytes(ctx context.Context, client *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/pgp-keys, application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxKeyBytes))
}

// parseKeyRing reads a public key body in either ASCII-armored or binary
// form and returns its entities.
func parseKeyRing(raw []byte) (openpgp.EntityList, error) {
	var entities openpgp.EntityList
	var err error
	if bytes.Contains(raw, []byte("-----BEGIN PGP")) {
		entities, err = openpgp.ReadArmoredKeyRing(bytes.NewReader(raw))
	} else {
		entities, err = openpgp.ReadKeyRing(bytes.NewReader(raw))
	}
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("empty keyring")
	}
	return entities, nil
}
