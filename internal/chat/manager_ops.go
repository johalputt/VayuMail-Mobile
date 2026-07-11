package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

// settingPrefix keys per-fingerprint verified flags in the settings store.
const settingPrefix = "talk-verified:"

// Send encrypts plaintext to peer (signed by selfEmail), base64-encodes the
// armored ciphertext, and transmits it. mode is "live" or "store"; ttl is
// clamped server-side to [60s, 3600s]. Nothing is persisted: plaintext and
// ciphertext exist only for the duration of the call. Returns the
// server-assigned id and emits a Delivered event.
func (m *Manager) Send(ctx context.Context, peer, plaintext string, ttl time.Duration, mode string) (string, error) {
	ciphertext, err := m.keyring.Encrypt([]byte(plaintext), []string{peer}, m.selfEmail)
	if err != nil {
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(ciphertext)
	tok, err := m.ensureToken(ctx)
	if err != nil {
		return "", err
	}
	id, delivered, err := m.tp.Send(ctx, m.domain, tok, peer, b64, int(ttl/time.Second), mode)
	if err != nil {
		if errors.Is(err, ErrTalkAuth) {
			m.clearToken()
		}
		return "", err
	}
	m.rememberPeer(id, peer)
	m.emit(Delivered{ID: id, OK: delivered})
	return id, nil
}

// Ack requests read-destruction of a received message. The UI calls this
// once the plaintext has been displayed.
func (m *Manager) Ack(ctx context.Context, id string) error {
	tok, err := m.ensureToken(ctx)
	if err != nil {
		return err
	}
	if err := m.tp.Ack(ctx, m.domain, tok, id); err != nil {
		if errors.Is(err, ErrTalkAuth) {
			m.clearToken()
		}
		return err
	}
	return nil
}

// VerifyPeer fetches peer's public key, imports it into the keyring, and
// emits a PeerKey event carrying the fingerprint and its verified state.
func (m *Manager) VerifyPeer(ctx context.Context, peer string) error {
	tok, err := m.ensureToken(ctx)
	if err != nil {
		return err
	}
	armored, fingerprint, err := m.tp.FetchPubKey(ctx, m.domain, tok, peer)
	if err != nil {
		if errors.Is(err, ErrTalkAuth) {
			m.clearToken()
		}
		return err
	}
	if _, err := m.keyring.ImportArmored([]byte(armored)); err != nil {
		return err
	}
	m.emit(PeerKey{Peer: peer, Fingerprint: fingerprint, Verified: m.isVerified(ctx, fingerprint)})
	return nil
}

// SetPeerVerified records the user's out-of-band verification decision for
// a fingerprint (public data — safe in SQLite).
func (m *Manager) SetPeerVerified(ctx context.Context, fingerprint string, verified bool) error {
	fp := strings.ToLower(strings.TrimSpace(fingerprint))
	if m.settings != nil {
		val := ""
		if verified {
			val = "1"
		}
		return m.settings.SetSetting(ctx, settingPrefix+fp, val)
	}
	m.mu.Lock()
	m.verifiedMem[fp] = verified
	m.mu.Unlock()
	return nil
}

// handleEnvelope decrypts an incoming envelope and emits IncomingMessage.
// A message that fails to decrypt is dropped silently (never logged with
// content).
func (m *Manager) handleEnvelope(e Envelope) {
	raw, err := base64.StdEncoding.DecodeString(e.Ciphertext)
	if err != nil {
		return
	}
	res, err := m.keyring.Decrypt(raw)
	if err != nil {
		return
	}
	m.emit(IncomingMessage{
		Peer:      e.From,
		ID:        e.ID,
		Plaintext: string(res.Plaintext),
		ExpiresAt: e.ExpiresAt,
	})
}

// handleReceipt maps a receipt to a read or expired event.
func (m *Manager) handleReceipt(r Receipt) {
	switch r.Status {
	case "read":
		m.emit(MessageRead{Peer: m.peerForID(r.ID), ID: r.ID})
	case "expired":
		m.emit(MessageExpired{ID: r.ID})
	}
}

// SafetyNumber formats a fingerprint into readable space-separated groups
// of four for out-of-band comparison, five groups per line.
func SafetyNumber(fingerprint string) string {
	fp := strings.ToUpper(strings.Map(func(r rune) rune {
		if r == ' ' || r == ':' {
			return -1
		}
		return r
	}, fingerprint))
	var groups []string
	for i := 0; i < len(fp); i += 4 {
		end := i + 4
		if end > len(fp) {
			end = len(fp)
		}
		groups = append(groups, fp[i:end])
	}
	var b strings.Builder
	for i, g := range groups {
		if i > 0 {
			if i%5 == 0 {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(g)
	}
	return b.String()
}

// ensureToken returns a cached bearer token or connects to obtain one. The
// network call happens without the lock held; a concurrent connect simply
// overwrites with an equivalent token.
func (m *Manager) ensureToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	tok := m.tok
	m.mu.Unlock()
	if tok != "" {
		return tok, nil
	}
	pw, err := m.credential()
	if err != nil {
		return "", err
	}
	tok, err = m.tp.Connect(ctx, m.domain, m.selfEmail, pw)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.tok = tok
	m.mu.Unlock()
	return tok, nil
}

// clearToken forces the next call to reconnect (used after a 401).
func (m *Manager) clearToken() {
	m.mu.Lock()
	m.tok = ""
	m.mu.Unlock()
}

// rememberPeer records which recipient a sent message went to so a later
// read receipt can name the peer.
func (m *Manager) rememberPeer(id, peer string) {
	m.mu.Lock()
	m.sentTo[id] = peer
	m.mu.Unlock()
}

// peerForID returns the recipient recorded for a sent message, or "".
func (m *Manager) peerForID(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sentTo[id]
}

// isVerified reports the stored verified flag for a fingerprint.
func (m *Manager) isVerified(ctx context.Context, fingerprint string) bool {
	fp := strings.ToLower(strings.TrimSpace(fingerprint))
	if m.settings != nil {
		v, err := m.settings.GetSetting(ctx, settingPrefix+fp)
		return err == nil && v == "1"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.verifiedMem[fp]
}
