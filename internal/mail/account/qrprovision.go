package account

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Typed provisioning errors (Constitutional Rule 7). Callers map these to
// user-facing messages; none of them ever falls back to using unverified
// payload values.
var (
	// ErrUnknownVersion means the payload version is not supported.
	ErrUnknownVersion = errors.New("qrprovision: unknown payload version")
	// ErrExpired means the payload's expiry time has passed.
	ErrExpired = errors.New("qrprovision: payload expired")
	// ErrInvalidSignature means Ed25519 verification failed.
	ErrInvalidSignature = errors.New("qrprovision: invalid signature")
	// ErrInsecureTransport means the payload requested a plaintext
	// connection, which VayuMail never opens.
	ErrInsecureTransport = errors.New("qrprovision: insecure transport requested")
	// ErrInvalidPort means a port was outside 1-65535.
	ErrInvalidPort = errors.New("qrprovision: invalid port")
	// ErrMalformedPayload means the QR content was not a valid payload.
	ErrMalformedPayload = errors.New("qrprovision: malformed payload")
	// ErrTokenExpired means the provisioning token was already used or
	// timed out server-side; the user must generate a fresh QR code.
	ErrTokenExpired = errors.New("qrprovision: provisioning token expired")
	// ErrTokenInvalid means the server rejected the token outright.
	ErrTokenInvalid = errors.New("qrprovision: provisioning token rejected")
	// ErrNetwork means the token exchange could not reach the server.
	ErrNetwork = errors.New("qrprovision: network error during token exchange")
)

// ProvisionPayload is the versioned JSON document carried inside a
// VayuMail provisioning QR code (ADR-0003).
type ProvisionPayload struct {
	V             int    `json:"v"`
	Server        string `json:"server"`
	IMAPPort      int    `json:"imap_port"`
	IMAPTLS       string `json:"imap_tls"`
	SMTPPort      int    `json:"smtp_port"`
	SMTPTLS       string `json:"smtp_tls"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Token         string `json:"token"`
	TokenEndpoint string `json:"token_endpoint"`
	ServerPubkey  string `json:"server_pubkey"`
	ExpiresAt     int64  `json:"expires_at"`
	Sig           string `json:"sig"`
}

// CanonicalJSON returns the bytes the Ed25519 signature covers: every
// field except "sig", keys lexicographically sorted, no whitespace, UTF-8.
// encoding/json marshals map keys in sorted order and emits compact
// output, which matches the specification exactly.
func (p *ProvisionPayload) CanonicalJSON() ([]byte, error) {
	m := map[string]any{
		"v":              p.V,
		"server":         p.Server,
		"imap_port":      p.IMAPPort,
		"imap_tls":       p.IMAPTLS,
		"smtp_port":      p.SMTPPort,
		"smtp_tls":       p.SMTPTLS,
		"username":       p.Username,
		"display_name":   p.DisplayName,
		"token":          p.Token,
		"token_endpoint": p.TokenEndpoint,
		"server_pubkey":  p.ServerPubkey,
		"expires_at":     p.ExpiresAt,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("qrprovision: canonicalize: %w", err)
	}
	return b, nil
}

// ParseAndVerify decodes a raw QR payload, verifies its Ed25519 signature,
// and validates every security-relevant field. No field of the returned
// payload may be used to open a connection unless the error is nil.
func ParseAndVerify(raw []byte, now time.Time) (*ProvisionPayload, error) {
	jsonBytes, err := base64.RawURLEncoding.DecodeString(string(bytes.TrimSpace(raw)))
	if err != nil {
		return nil, fmt.Errorf("%w: base64: %v", ErrMalformedPayload, err)
	}

	var p ProvisionPayload
	if err := json.Unmarshal(jsonBytes, &p); err != nil {
		return nil, fmt.Errorf("%w: json: %v", ErrMalformedPayload, err)
	}

	if p.V != 1 {
		return nil, fmt.Errorf("%w: v=%d", ErrUnknownVersion, p.V)
	}
	if now.Unix() >= p.ExpiresAt {
		return nil, ErrExpired
	}

	pubkeyBytes, err := base64.RawURLEncoding.DecodeString(p.ServerPubkey)
	if err != nil || len(pubkeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: bad public key", ErrInvalidSignature)
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(p.Sig)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: bad signature encoding", ErrInvalidSignature)
	}
	canonical, err := p.CanonicalJSON()
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(ed25519.PublicKey(pubkeyBytes), canonical, sigBytes) {
		return nil, ErrInvalidSignature
	}

	// Transport security is validated only after the signature, so the
	// error reported to the user reflects an authentic payload.
	for _, mode := range []string{p.IMAPTLS, p.SMTPTLS} {
		if TLSMode(mode).Valid() {
			continue
		}
		if mode == "plain" && allowPlainTransport {
			continue
		}
		return nil, fmt.Errorf("%w: %q", ErrInsecureTransport, mode)
	}
	if !validPort(p.IMAPPort) {
		return nil, fmt.Errorf("%w: imap_port=%d", ErrInvalidPort, p.IMAPPort)
	}
	if !validPort(p.SMTPPort) {
		return nil, fmt.Errorf("%w: smtp_port=%d", ErrInvalidPort, p.SMTPPort)
	}

	// The Ed25519 signature is self-certifying — the verifying key travels
	// inside the signed payload — so it proves integrity, NOT authenticity: an
	// attacker mints a valid signature with their own keypair. The only real
	// anchor is the mailbox address the user is setting up. Bind every host the
	// payload can steer the device to (the IMAP/SMTP server and the token
	// endpoint) to that address's domain, and require each to be a public host,
	// so a self-signed code cannot point the app at attacker infrastructure or
	// an internal/LAN address (audit M14, M15; SSRF / account-hijack).
	usernameDomain := domainOf(p.Username)
	if usernameDomain == "" || !publicMailDomain(usernameDomain) {
		return nil, fmt.Errorf("%w: username domain", ErrMalformedPayload)
	}
	if !publicMailDomain(p.Server) || !hostInDomain(p.Server, usernameDomain) {
		return nil, fmt.Errorf("%w: server host not in mailbox domain", ErrMalformedPayload)
	}
	if err := validateTokenEndpoint(p.TokenEndpoint, usernameDomain); err != nil {
		return nil, err
	}

	return &p, nil
}

// hostInDomain reports whether host equals base or is a subdomain of it
// (case-insensitive). A provisioning payload may only point at infrastructure
// within the mailbox's own domain.
func hostInDomain(host, base string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	base = strings.ToLower(strings.TrimSpace(base))
	if host == "" || base == "" {
		return false
	}
	return host == base || strings.HasSuffix(host, "."+base)
}

// validateTokenEndpoint vets the token-exchange URL before the device ever
// dials it (audit M14): https only, no userinfo, default/443 port, a public
// host that is the mailbox domain or a subdomain of it. Returning nil means the
// URL is safe to POST to; every failure maps to a typed payload error.
func validateTokenEndpoint(rawURL, usernameDomain string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: token_endpoint: %v", ErrMalformedPayload, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: token_endpoint not https", ErrInsecureTransport)
	}
	if u.User != nil {
		return fmt.Errorf("%w: token_endpoint carries userinfo", ErrMalformedPayload)
	}
	if port := u.Port(); port != "" && port != "443" {
		return fmt.Errorf("%w: token_endpoint port %q", ErrMalformedPayload, port)
	}
	host := u.Hostname()
	if !publicMailDomain(host) {
		return fmt.Errorf("%w: token_endpoint host", ErrMalformedPayload)
	}
	if !hostInDomain(host, usernameDomain) {
		return fmt.Errorf("%w: token_endpoint host not in mailbox domain", ErrMalformedPayload)
	}
	return nil
}

// Config converts a verified payload into an account Config. keystoreAlias
// names the platform-keystore entry the caller will store the exchanged
// credential under.
func (p *ProvisionPayload) Config(keystoreAlias string) Config {
	return Config{
		DisplayName:   p.DisplayName,
		EmailAddress:  p.Username,
		IMAPHost:      p.Server,
		IMAPPort:      p.IMAPPort,
		IMAPTLS:       TLSMode(p.IMAPTLS),
		SMTPHost:      p.Server,
		SMTPPort:      p.SMTPPort,
		SMTPTLS:       TLSMode(p.SMTPTLS),
		Username:      p.Username,
		KeystoreAlias: keystoreAlias,
	}
}

// Credentials is the result of a successful token exchange. Exactly one of
// the password pair or the OAuth fields is populated. The caller moves
// these into the platform keystore; this package never touches disk.
type Credentials struct {
	IMAPPassword string `json:"imap_password"`
	SMTPPassword string `json:"smtp_password"`
	OAuthToken   string `json:"oauth_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// ExchangeToken redeems the one-time provisioning token at the payload's
// token endpoint and returns the mail credentials. It never writes to the
// keystore or to disk — that is the caller's responsibility.
func ExchangeToken(ctx context.Context, client *http.Client, p *ProvisionPayload) (*Credentials, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	// Re-vet the endpoint at the point of use (defense in depth): callers must
	// pass a ParseAndVerify'd payload, but ExchangeToken must never dial an
	// unvetted URL even if called directly (audit M14).
	if err := validateTokenEndpoint(p.TokenEndpoint, domainOf(p.Username)); err != nil {
		return nil, err
	}
	reqBody, err := json.Marshal(map[string]string{
		"token":    p.Token,
		"username": p.Username,
	})
	if err != nil {
		return nil, fmt.Errorf("qrprovision: encode exchange request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.TokenEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("qrprovision: build exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Refuse redirects: a 30x could bounce the one-time token (and the POST
	// body) to an off-domain or downgraded host that the vetting above never
	// saw (audit M14). Mirrors every other network path in this package.
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		// fall through to decode
	case resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrTokenExpired
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return nil, fmt.Errorf("%w: HTTP %d", ErrTokenInvalid, resp.StatusCode)
	default:
		return nil, fmt.Errorf("%w: HTTP %d", ErrNetwork, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrNetwork, err)
	}
	var creds Credentials
	if err := json.Unmarshal(body, &creds); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrTokenInvalid, err)
	}
	if creds.IMAPPassword == "" && creds.OAuthToken == "" {
		return nil, fmt.Errorf("%w: response carried no credential", ErrTokenInvalid)
	}
	return &creds, nil
}
