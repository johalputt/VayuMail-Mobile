package account

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// AutoconfigSchema is the version of the first-party VayuMail autoconfig
// document this client understands. It MUST match the server's
// VayuMailAutoconfigSchema (VayuPress cmd/vayupress/vayuos_mail.go); the pairing
// is locked by autoconfig_contract_test.go on both sides. A document announcing
// a different schema is rejected rather than half-parsed.
const AutoconfigSchema = "vayumail-autoconfig/1"

// autoconfigDoc mirrors the JSON served at
// https://<domain>/.well-known/vayumail/autoconfig.json. Field names and the
// "tls"/"starttls" spellings are the wire contract with the VayuPress server.
type autoconfigDoc struct {
	Schema          string             `json:"schema"`
	Domain          string             `json:"domain"`
	DisplayName     string             `json:"displayName"`
	IMAP            autoconfigEndpoint `json:"imap"`
	POP3            autoconfigEndpoint `json:"pop3"`
	SMTP            autoconfigEndpoint `json:"smtp"`
	UsernameIsEmail bool               `json:"usernameIsEmail"`
	Auth            string             `json:"auth"`
	WKD             bool               `json:"wkd"`
}

type autoconfigEndpoint struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	TLS  string `json:"tls"`
}

// autoconfigMaxBytes caps the discovery response so a hostile or misconfigured
// server cannot stream an unbounded body into memory.
const autoconfigMaxBytes = 64 << 10

// DiscoverAutoconfig looks up mail-server settings for an email address via the
// domain's first-party VayuMail autoconfig document and returns a Config
// prefilled with the IMAP/SMTP coordinates and username. Discovery is
// user-initiated only — VayuMail never phones home on its own.
//
// The returned Config is a draft: KeystoreAlias is left empty for the setup flow
// to assign once the operator supplies a credential, so callers should set it
// (and store the secret) before calling Validate. On success the network
// identity (hosts, ports, TLS modes, username, display name) is complete.
func DiscoverAutoconfig(ctx context.Context, client *http.Client, email string) (*Config, error) {
	if client == nil {
		client = http.DefaultClient
	}
	// Do NOT follow redirects during discovery: publicMailDomain vets only the
	// initial host, so a mail domain that 3xx-redirects to a private/loopback or
	// cloud-metadata address must not be chased (SSRF, CWE-918). A shallow copy
	// keeps the caller's transport (and any test injection) but refuses to follow
	// a redirect — a 3xx then surfaces as a non-200 and the lookup fails safely.
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client = &noFollow

	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return nil, fmt.Errorf("account: autoconfig: invalid address %q", email)
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	if !publicMailDomain(domain) {
		return nil, fmt.Errorf("account: autoconfig: refusing non-public domain %q", domain)
	}

	// The domain's own trusted-cert host is where VayuMail publishes; try it,
	// then the conventional autoconfig subdomain as a fallback.
	urls := []string{
		"https://" + domain + "/.well-known/vayumail/autoconfig.json",
		"https://autoconfig." + domain + "/.well-known/vayumail/autoconfig.json",
	}
	var lastErr error
	for _, u := range urls {
		doc, err := fetchAutoconfig(ctx, client, u)
		if err != nil {
			lastErr = err
			continue
		}
		cfg, err := doc.toConfig(email)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}
	return nil, fmt.Errorf("account: autoconfig lookup for %s: %w", email, lastErr)
}

func fetchAutoconfig(ctx context.Context, client *http.Client, url string) (*autoconfigDoc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, autoconfigMaxBytes))
	if err != nil {
		return nil, err
	}
	var doc autoconfigDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse autoconfig: %w", err)
	}
	if doc.Schema != AutoconfigSchema {
		return nil, fmt.Errorf("unsupported autoconfig schema %q (want %q)", doc.Schema, AutoconfigSchema)
	}
	return &doc, nil
}

// toConfig maps a discovered document to a draft Config for the given address.
func (d *autoconfigDoc) toConfig(email string) (*Config, error) {
	imapTLS, err := tlsModeFromWire(d.IMAP.TLS)
	if err != nil {
		return nil, fmt.Errorf("account: autoconfig: imap: %w", err)
	}
	smtpTLS, err := tlsModeFromWire(d.SMTP.TLS)
	if err != nil {
		return nil, fmt.Errorf("account: autoconfig: smtp: %w", err)
	}
	if d.IMAP.Host == "" || d.SMTP.Host == "" {
		return nil, fmt.Errorf("account: autoconfig: document missing imap/smtp host")
	}
	if !validPort(d.IMAP.Port) || !validPort(d.SMTP.Port) {
		return nil, fmt.Errorf("account: autoconfig: document has an out-of-range port")
	}
	mech, err := authMechFromWire(d.Auth)
	if err != nil {
		return nil, fmt.Errorf("account: autoconfig: %w", err)
	}
	display := strings.TrimSpace(d.DisplayName)
	if display == "" {
		display = email
	}
	// The server signals username == email; if it ever says otherwise we still
	// default to the full address, which every VayuMail mailbox accepts.
	username := email
	return &Config{
		DisplayName:  display,
		EmailAddress: email,
		IMAPHost:     d.IMAP.Host,
		IMAPPort:     d.IMAP.Port,
		IMAPTLS:      imapTLS,
		SMTPHost:     d.SMTP.Host,
		SMTPPort:     d.SMTP.Port,
		SMTPTLS:      smtpTLS,
		Username:     username,
		AuthMech:     mech,
		// KeystoreAlias intentionally left blank — assigned by the setup flow.
	}, nil
}

// authMechFromWire maps the autoconfig "auth" field to an account AuthMech. An
// empty or "password" value is the default password mechanism; "oauthbearer"
// and "xoauth2" select the corresponding token mechanism (so a server that
// mints bearer tokens is configured correctly without manual entry). An
// unrecognised value is rejected rather than silently defaulting to a wrong or
// insecure mechanism.
func authMechFromWire(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "password":
		return AuthPassword, nil
	case AuthOAuthBearer:
		return AuthOAuthBearer, nil
	case AuthXOAuth2:
		return AuthXOAuth2, nil
	default:
		return "", fmt.Errorf("unsupported auth mechanism %q", s)
	}
}

// tlsModeFromWire converts the autoconfig "tls"/"starttls" spelling to a
// TLSMode. Plaintext is never accepted.
func tlsModeFromWire(s string) (TLSMode, error) {
	switch TLSMode(strings.ToLower(strings.TrimSpace(s))) {
	case TLSModeImplicit:
		return TLSModeImplicit, nil
	case TLSModeSTARTTLS:
		return TLSModeSTARTTLS, nil
	default:
		return "", fmt.Errorf("unsupported socket type %q", s)
	}
}

// mailHostRe matches a syntactically valid multi-label public DNS hostname. It
// deliberately excludes anything carrying a port, path, userinfo or scheme, so a
// crafted address part (e.g. "evil.com:9999" or "evil.com/x") cannot inject a
// port or path into the discovery URL.
var mailHostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$`)

// publicMailDomain reports whether domain is a routable public DNS name safe to
// fetch autoconfig from. It rejects empty input, IP literals, localhost, and
// anything that is not a clean multi-label hostname (no port/path/userinfo) so a
// hostile address cannot steer discovery at internal/loopback hosts or arbitrary
// ports (SSRF, CWE-918).
func publicMailDomain(domain string) bool {
	if domain == "" || len(domain) > 253 || strings.EqualFold(domain, "localhost") {
		return false
	}
	if net.ParseIP(domain) != nil {
		return false
	}
	return mailHostRe.MatchString(domain)
}
