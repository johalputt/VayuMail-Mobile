package pgp

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // WKD (draft-koch) mandates SHA-1 for the address hash; not used for security
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// zbase32Alphabet is the encoding WKD uses for the hashed local part.
const zbase32Alphabet = "ybndrfg8ejkmcpqxot1uwisza345h769"

// DiscoverWKD looks up a recipient's public key via Web Key Directory
// (draft-koch-openpgp-webkey-service): the advanced endpoint first, then
// the direct one. It returns the armored-importable binary keyring bytes.
// Discovery is user-initiated only — VayuMail never phones home.
func DiscoverWKD(ctx context.Context, client *http.Client, email string) (openpgp.EntityList, error) {
	if client == nil {
		client = http.DefaultClient
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return nil, fmt.Errorf("pgp: wkd: invalid address %q", email)
	}
	local := strings.ToLower(email[:at])
	domain := strings.ToLower(email[at+1:])
	hashed := WKDHash(local)

	urls := []string{
		fmt.Sprintf("https://openpgpkey.%s/.well-known/openpgpkey/%s/hu/%s?l=%s",
			domain, domain, hashed, local),
		fmt.Sprintf("https://%s/.well-known/openpgpkey/hu/%s?l=%s",
			domain, hashed, local),
	}
	var lastErr error
	for _, u := range urls {
		entities, err := fetchWKD(ctx, client, u)
		if err == nil {
			return entities, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("pgp: wkd lookup for %s: %w", email, lastErr)
}

func fetchWKD(ctx context.Context, client *http.Client, url string) (openpgp.EntityList, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	entities, err := openpgp.ReadKeyRing(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse key: %w", err)
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("empty keyring")
	}
	return entities, nil
}

// ImportEntities adds already-parsed entities to the keyring and returns
// their fingerprints (mirrors ImportArmored for WKD results).
func (k *Keyring) ImportEntities(entities openpgp.EntityList) []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var fingerprints []string
	for _, e := range entities {
		fp := fingerprintOf(e)
		k.entities = append(k.entities, e)
		if _, seen := k.trust[fp]; !seen {
			k.trust[fp] = TrustUnknown
		}
		fingerprints = append(fingerprints, fp)
	}
	return fingerprints
}

// HasKeyFor reports whether the keyring can encrypt to every address.
// The composer uses it to enable encryption automatically.
func (k *Keyring) HasKeyFor(emails ...string) bool {
	if len(emails) == 0 {
		return false
	}
	for _, e := range emails {
		if _, err := k.EntityByEmail(e); err != nil {
			return false
		}
	}
	return true
}

// WKDHash returns the z-base-32 SHA-1 hash of a lowercased local part —
// the path component of a WKD lookup URL. Exported for known-answer
// testing against the draft's test vector.
func WKDHash(localPart string) string {
	return zbase32(sha1Bytes(strings.ToLower(localPart)))
}

func sha1Bytes(s string) []byte {
	sum := sha1.Sum([]byte(s)) //nolint:gosec // mandated by the WKD spec
	return sum[:]
}

// zbase32 encodes bytes with the z-base-32 alphabet (RFC 6189 appendix).
func zbase32(data []byte) string {
	var out strings.Builder
	bits := 0
	buffer := 0
	for _, b := range data {
		buffer = buffer<<8 | int(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out.WriteByte(zbase32Alphabet[(buffer>>bits)&0x1F])
		}
	}
	if bits > 0 {
		out.WriteByte(zbase32Alphabet[(buffer<<(5-bits))&0x1F])
	}
	return out.String()
}
