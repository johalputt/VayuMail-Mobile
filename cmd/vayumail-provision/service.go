package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// serviceConfig is the static configuration of the provisioning service.
type serviceConfig struct {
	Server   string
	IMAPPort int
	SMTPPort int
	TTL      int
	Users    map[string]string
	Key      ed25519.PrivateKey
}

// pendingToken is one issued, not-yet-redeemed provisioning token.
type pendingToken struct {
	email   string
	expires time.Time
}

// service issues signed payloads and redeems one-time tokens.
type service struct {
	cfg    serviceConfig
	pubB64 string

	mu     sync.Mutex
	tokens map[string]pendingToken // token -> pending, single use
}

func newService(cfg serviceConfig) *service {
	pub := cfg.Key.Public().(ed25519.PublicKey)
	return &service{
		cfg:    cfg,
		pubB64: base64.RawURLEncoding.EncodeToString(pub),
		tokens: make(map[string]pendingToken),
	}
}

// pruneExpiredLocked drops tokens past their validity window so the map
// cannot grow without bound from unredeemed codes. Callers hold s.mu.
func (s *service) pruneExpiredLocked(now time.Time) {
	for token, pt := range s.tokens {
		if now.After(pt.expires) {
			delete(s.tokens, token)
		}
	}
}

// buildPayload creates and signs one provisioning payload for a user.
// The canonical form matches internal/mail/account.CanonicalJSON: sorted
// keys, no whitespace.
func (s *service) buildPayload(email, endpoint string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(time.Duration(s.cfg.TTL) * time.Second)
	s.mu.Lock()
	s.pruneExpiredLocked(time.Now())
	s.tokens[token] = pendingToken{email: email, expires: expiresAt}
	s.mu.Unlock()

	fields := map[string]any{
		"v":              1,
		"server":         s.cfg.Server,
		"imap_port":      s.cfg.IMAPPort,
		"imap_tls":       "tls",
		"smtp_port":      s.cfg.SMTPPort,
		"smtp_tls":       "starttls",
		"username":       email,
		"display_name":   strings.Split(email, "@")[0],
		"token":          token,
		"token_endpoint": endpoint,
		"server_pubkey":  s.pubB64,
		"expires_at":     expiresAt.Unix(),
	}
	canonical, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	fields["sig"] = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(s.cfg.Key, canonical))
	full, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(full), nil
}

// handleQRText serves the payload as text — pipe into any QR generator,
// or use /qr.png directly.
func (s *service) handleQRText(w http.ResponseWriter, r *http.Request) {
	payload, ok := s.payloadFor(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, payload)
}

// handleQRImage renders the payload as a scannable QR PNG.
func (s *service) handleQRImage(w http.ResponseWriter, r *http.Request) {
	payload, ok := s.payloadFor(w, r)
	if !ok {
		return
	}
	writer := qrcode.NewQRCodeWriter()
	matrix, err := writer.Encode(payload, gozxing.BarcodeFormat_QR_CODE, 512, 512, nil)
	if err != nil {
		http.Error(w, "encode QR", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, matrix); err != nil {
		http.Error(w, "render QR", http.StatusInternalServerError)
	}
}

func (s *service) payloadFor(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := strings.ToLower(r.URL.Query().Get("user"))
	if _, ok := s.cfg.Users[email]; !ok {
		http.Error(w, "unknown user", http.StatusNotFound)
		return "", false
	}
	scheme := "https"
	if r.TLS == nil {
		scheme = "http" // dev only; production sits behind TLS
	}
	endpoint := fmt.Sprintf("%s://%s/provision", scheme, r.Host)
	payload, err := s.buildPayload(email, endpoint)
	if err != nil {
		http.Error(w, "build payload", http.StatusInternalServerError)
		return "", false
	}
	return payload, true
}

// handleExchange redeems a one-time token for the mail credentials
// (the client stores them straight into its platform keystore).
func (s *service) handleExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	now := time.Now()
	s.mu.Lock()
	s.pruneExpiredLocked(now)
	pt, ok := s.tokens[req.Token]
	if ok {
		delete(s.tokens, req.Token) // single use
	}
	s.mu.Unlock()

	if !ok || now.After(pt.expires) || !strings.EqualFold(pt.email, req.Username) {
		http.Error(w, "token expired or invalid", http.StatusGone)
		return
	}
	password := s.cfg.Users[pt.email]
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"imap_password": password,
		"smtp_password": password,
	}); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}

func randomToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
