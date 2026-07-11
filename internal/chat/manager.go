package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

// Backoff bounds for stream reconnection.
const (
	minBackoff = 1 * time.Second
	maxBackoff = 30 * time.Second
)

// SettingStore is the optional persistence for per-fingerprint "verified"
// flags. *store.DB satisfies it. Fingerprints are PUBLIC data, so keeping
// the verified bool in SQLite does not violate credential sovereignty.
type SettingStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Config constructs a Manager.
type Config struct {
	// Keyring holds the user's private key (to decrypt) and correspondents'
	// public keys (to encrypt). Required.
	Keyring *pgp.Keyring
	// SelfEmail signs outbound messages and identifies this user.
	SelfEmail string
	// Domain is the mail domain hosting VayuTalk (the part after @).
	Domain string
	// Credential returns the mailbox/app password on demand. It is called
	// at connect time and never cached (Rule 6).
	Credential func() (string, error)
	// Settings optionally persists verified fingerprints; nil keeps them
	// in memory only.
	Settings SettingStore
	// HTTPClient optionally overrides the transport (test injection).
	HTTPClient *http.Client
}

// Manager owns the VayuTalk connection and all its goroutines. Construct
// with New, wire the UI to Events(), call Start once, and Close on exit.
// The manager holds decrypted plaintext only in memory and expects the UI
// to Ack after display.
type Manager struct {
	keyring    *pgp.Keyring
	selfEmail  string
	domain     string
	credential func() (string, error)
	settings   SettingStore
	tp         *Transport

	eventCh chan Event

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu          sync.Mutex
	tok         string
	sentTo      map[string]string // message id -> recipient email
	verifiedMem map[string]bool   // fingerprint -> verified (when no store)
}

// New builds a Manager. It does not touch the network until Start (or a
// direct Send/Ack/VerifyPeer) is called.
func New(cfg Config) *Manager {
	var base http.RoundTripper
	if cfg.HTTPClient != nil {
		base = cfg.HTTPClient.Transport
	}
	return &Manager{
		keyring:     cfg.Keyring,
		selfEmail:   cfg.SelfEmail,
		domain:      cfg.Domain,
		credential:  cfg.Credential,
		settings:    cfg.Settings,
		tp:          newTransport(base),
		eventCh:     make(chan Event, 128),
		sentTo:      make(map[string]string),
		verifiedMem: make(map[string]bool),
	}
}

// Events returns the channel the UI drains non-blockingly. It is never
// closed, so a late Send after Close cannot panic a caller.
func (m *Manager) Events() <-chan Event { return m.eventCh }

// Start opens the live stream and keeps it open, reconnecting with capped
// exponential backoff on drops. It returns immediately; work runs until
// Close or ctx cancellation.
func (m *Manager) Start(ctx context.Context) {
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.wg.Add(1)
	go m.run()
}

// Close tears down: cancels every goroutine and waits for them to drain.
func (m *Manager) Close() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

// run is the reconnect loop. It obtains a token, opens the stream, drains
// events until the stream ends, then backs off and retries.
func (m *Manager) run() {
	defer m.wg.Done()
	backoff := minBackoff
	for {
		if m.ctx.Err() != nil {
			return
		}
		tok, err := m.ensureToken(m.ctx)
		if err != nil {
			m.emit(ConnState{Online: false, Err: err})
			if !m.sleep(backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		stream, err := m.tp.OpenStream(m.ctx, tok, m.domain)
		if err != nil {
			if errors.Is(err, ErrTalkAuth) {
				m.clearToken()
			}
			m.emit(ConnState{Online: false, Err: err})
			if !m.sleep(backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		m.emit(ConnState{Online: true})
		backoff = minBackoff
		for ev := range stream {
			m.handleStreamEvent(ev)
		}
		if m.ctx.Err() != nil {
			return
		}
		m.emit(ConnState{Online: false})
		if !m.sleep(backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

// handleStreamEvent dispatches one live event.
func (m *Manager) handleStreamEvent(ev StreamEvent) {
	switch e := ev.(type) {
	case Envelope:
		m.handleEnvelope(e)
	case Receipt:
		m.handleReceipt(e)
	case Ping:
		// Heartbeat only; liveness already implied by delivery.
	}
}

// emit delivers an event without ever blocking a goroutine. A full buffer
// drops the event with a log line (never message content).
func (m *Manager) emit(ev Event) {
	select {
	case m.eventCh <- ev:
	default:
		slog.Warn("chat: event channel full, dropping event",
			"event", fmt.Sprintf("%T", ev))
	}
}

// sleep waits d or until the manager context is cancelled. It reports
// whether the wait completed (true) or was cancelled (false).
func (m *Manager) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-m.ctx.Done():
		return false
	}
}

// nextBackoff doubles d up to maxBackoff.
func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
