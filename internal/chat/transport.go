// Package chat implements VayuTalk: ephemeral, end-to-end-encrypted
// messaging over a small HTTP+SSE protocol. The server never sees
// plaintext and stores nothing durably; this client encrypts with the
// user's PGP keyring, streams ciphertext envelopes, and destroys messages
// on read (ack).
//
// Transport discipline mirrors internal/mail/account (privkey.go): HTTPS
// only, an SSRF domain guard (publicTalkDomain), refused redirects, and
// size-capped responses so a credential or token never traverses an
// unverified hop. Nothing here imports Gio (Constitutional Rule 4).
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Sentinel errors let callers branch on failure class.
var (
	// ErrTalkAuth is a rejected or expired token / credential (HTTP 401).
	ErrTalkAuth = errors.New("chat: authentication failed")
	// ErrTalkDisabled means the server does not serve VayuTalk (404/503).
	ErrTalkDisabled = errors.New("chat: VayuTalk not available")
	// ErrTalk is the generic transport failure (bad domain, network,
	// unexpected status, oversize payload, rate limit).
	ErrTalk = errors.New("chat: transport error")
)

// maxTalkResp caps a unary JSON response — every reply here is tiny except
// an armored public key, which is a few KiB.
const maxTalkResp = 256 << 10

// unaryTimeout bounds a single request/response call. The live stream is
// deliberately excluded — it is bounded by its context, not a client
// timeout, so it can stay open indefinitely.
const unaryTimeout = 30 * time.Second

// Transport speaks the VayuTalk wire protocol with a stdlib net/http
// client. It is stateless beyond the shared client and safe for concurrent
// use.
type Transport struct {
	client *http.Client
}

// NewTransport returns a Transport with a hardened default client:
// redirects refused (a token must never be replayed to another host) and
// no client-wide timeout (the stream needs an open connection; unary calls
// bound themselves with a context deadline).
func NewTransport() *Transport {
	return newTransport(nil)
}

// newTransport builds a Transport over base (a custom RoundTripper for
// tests) or the stdlib default. Redirects are always refused.
func newTransport(base http.RoundTripper) *Transport {
	c := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if base != nil {
		c.Transport = base
	}
	return &Transport{client: c}
}

// mailHostRe matches a syntactically valid multi-label public DNS hostname
// with no port, path, userinfo, or scheme — identical to the mail engine's
// guard so a crafted domain cannot inject a port/path or steer traffic.
var mailHostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`)

// publicTalkDomain reports whether domain is a routable public DNS name
// safe to reach VayuTalk on. It rejects empty input, IP literals,
// localhost, and anything that is not a clean multi-label hostname so a
// hostile value cannot point the client at internal/loopback hosts or
// arbitrary ports (SSRF, CWE-918).
func publicTalkDomain(domain string) bool {
	if domain == "" || len(domain) > 253 || strings.EqualFold(domain, "localhost") {
		return false
	}
	if net.ParseIP(domain) != nil {
		return false
	}
	return mailHostRe.MatchString(domain)
}

// baseURL is the VayuTalk API root for a domain.
func baseURL(domain string) string {
	return "https://" + domain + "/api/v1/talk"
}

// connectResponse is the /connect reply shape.
type connectResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

// Connect exchanges mailbox credentials for a bearer token. The server
// enforces device approval on this path (same credential check as mail
// sync); any failure is a uniform 401 -> ErrTalkAuth. A server without
// VayuTalk answers 404/503 -> ErrTalkDisabled.
func (t *Transport) Connect(ctx context.Context, domain, email, password string) (string, error) {
	if !publicTalkDomain(domain) {
		return "", fmt.Errorf("%w: bad domain", ErrTalk)
	}
	ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
	defer cancel()
	resp, err := t.do(ctx, http.MethodPost, domain, "/connect", "",
		map[string]string{"email": email, "password": password})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := statusErr(resp.StatusCode); err != nil {
		return "", err
	}
	var out connectResponse
	if err := decode(resp.Body, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("%w: empty token", ErrTalk)
	}
	return out.Token, nil
}

// sendResponse is the /send reply shape.
type sendResponse struct {
	ID        string `json:"id"`
	Delivered bool   `json:"delivered"`
}

// Send transmits one base64 ciphertext envelope. mode is "live" (deliver
// only if the recipient is online) or "store" (queue with TTL when
// offline). Returns the server-assigned id and whether a live stream took
// it. 413/429 map to ErrTalk, 401 to ErrTalkAuth.
func (t *Transport) Send(ctx context.Context, domain, token, to, ciphertextB64 string, ttlSeconds int, mode string) (string, bool, error) {
	if !publicTalkDomain(domain) {
		return "", false, fmt.Errorf("%w: bad domain", ErrTalk)
	}
	ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
	defer cancel()
	resp, err := t.do(ctx, http.MethodPost, domain, "/send", token,
		map[string]any{"to": to, "ciphertext": ciphertextB64, "ttl_seconds": ttlSeconds, "mode": mode})
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if err := statusErr(resp.StatusCode); err != nil {
		return "", false, err
	}
	var out sendResponse
	if err := decode(resp.Body, &out); err != nil {
		return "", false, err
	}
	return out.ID, out.Delivered, nil
}

// Ack requests read-destruction of an envelope. The server deletes it and,
// if the sender is streaming, emits a read receipt.
func (t *Transport) Ack(ctx context.Context, domain, token, id string) error {
	if !publicTalkDomain(domain) {
		return fmt.Errorf("%w: bad domain", ErrTalk)
	}
	ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
	defer cancel()
	resp, err := t.do(ctx, http.MethodPost, domain, "/ack", token, map[string]string{"id": id})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return statusErr(resp.StatusCode)
}

// pubKeyResponse is the /pubkey reply shape.
type pubKeyResponse struct {
	Email       string `json:"email"`
	Armored     string `json:"armored_public_key"`
	Fingerprint string `json:"fingerprint"`
}

// FetchPubKey retrieves a correspondent's armored public key and
// fingerprint from the server's PGP engine. A missing key (HTTP 404 on
// this path) maps to ErrTalk rather than ErrTalkDisabled.
func (t *Transport) FetchPubKey(ctx context.Context, domain, token, email string) (string, string, error) {
	if !publicTalkDomain(domain) {
		return "", "", fmt.Errorf("%w: bad domain", ErrTalk)
	}
	ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
	defer cancel()
	resp, err := t.do(ctx, http.MethodGet, domain,
		"/pubkey?email="+url.QueryEscape(email), token, nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", "", fmt.Errorf("%w: no public key for %s", ErrTalk, email)
	}
	if err := statusErr(resp.StatusCode); err != nil {
		return "", "", err
	}
	var out pubKeyResponse
	if err := decode(resp.Body, &out); err != nil {
		return "", "", err
	}
	if !strings.Contains(out.Armored, "BEGIN PGP PUBLIC KEY BLOCK") {
		return "", "", fmt.Errorf("%w: server returned no armored key", ErrTalk)
	}
	return out.Armored, out.Fingerprint, nil
}

// do builds and executes one request. body is JSON-encoded when non-nil.
func (t *Transport) do(ctx context.Context, method, domain, path, token string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL(domain)+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTalk, err)
	}
	return resp, nil
}

// statusErr maps a non-200 status to a typed error; 200 returns nil.
func statusErr(code int) error {
	switch code {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrTalkAuth
	case http.StatusNotFound, http.StatusServiceUnavailable:
		return ErrTalkDisabled
	default:
		return fmt.Errorf("%w: server returned %d", ErrTalk, code)
	}
}

// decode reads a size-capped JSON body into out.
func decode(r io.Reader, out any) error {
	if err := json.NewDecoder(io.LimitReader(r, maxTalkResp)).Decode(out); err != nil {
		return fmt.Errorf("%w: %v", ErrTalk, err)
	}
	return nil
}
