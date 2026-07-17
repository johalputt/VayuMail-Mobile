package state

// chatstate.go — the UI-side holder for VayuTalk (ephemeral E2E chat).
// It owns a *chat.Manager, drains its Events() on a background goroutine
// into a mutex-guarded snapshot, and exposes read-only Snapshot() plus
// action methods that forward to the engine. Nothing here imports Gio
// (Rule 4) and nothing blocks the frame loop (Rule 5): every network
// call runs on its own goroutine and wakes the window through invalidate.

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/chat"
	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// burn bounds mirror the server clamp so the local countdown matches what the
// server enforces. The timer is the burn-after-read window (see ADR-0131).
const (
	minBurnSeconds = 5
	maxBurnSeconds = 3600
	// liveGraceSeconds is how long a Live-mode message stays legible after it is
	// read before it burns — long enough to read, short enough to feel instant.
	liveGraceSeconds = 5
	// defaultBurnSeconds is the fallback when a message carries no timer.
	defaultBurnSeconds = 300
)

// MsgStatus is the lifecycle state of one chat message.
type MsgStatus int

// The message states. Outgoing messages move Sending → Sent/Queued →
// Read; incoming messages are Sealed until revealed, then Open; either
// direction collapses to Expired once its TTL elapses.
const (
	MsgSending MsgStatus = iota
	MsgSent
	MsgQueued
	MsgRead
	MsgSealed
	MsgOpen
	MsgExpired
)

// ChatMessage is one message as the room renders it. Plaintext lives in
// memory only and is never persisted (ephemeral by construction).
type ChatMessage struct {
	ID          string
	Peer        string
	Self        bool
	Text        string
	CreatedAt   time.Time
	ExpiresAt   time.Time // burn deadline; zero until the message is read (armed)
	ArmedAt     time.Time // when the burn countdown started (read time)
	BurnSeconds int       // self-destruct timer, in seconds after read
	Mode        string
	Status      MsgStatus
	Revealed    bool
}

// burnDuration is the effective self-destruct window for a message once it is
// read: a short fixed grace for Live mode, otherwise its chosen timer.
func (m *ChatMessage) burnDuration() time.Duration {
	secs := m.BurnSeconds
	if m.Mode == "live" {
		secs = liveGraceSeconds
	}
	if secs <= 0 {
		secs = defaultBurnSeconds
	}
	return time.Duration(secs) * time.Second
}

// ChatConversation is one row in the conversation list. It deliberately
// carries no message preview — showing ephemeral plaintext in a list
// would defeat the point.
type ChatConversation struct {
	Peer         string
	Fingerprint  string
	Verified     bool
	Unread       int
	LastActivity time.Time
}

// ChatSnapshot is the immutable view the VayuTalk screens read each
// frame. Slices and maps are freshly built per Snapshot() call so the
// render loop never races the drain goroutine.
type ChatSnapshot struct {
	Connected       bool
	Online          bool
	ActivePeer      string
	Conversations   []ChatConversation
	Messages        []ChatMessage
	Verified        bool
	Fingerprint     string // the active peer's key fingerprint
	SelfEmail       string // this mailbox's own address
	SelfFingerprint string // this mailbox's own key fingerprint (for "your safety number")
}

// chatConv is the authoritative per-peer record held under the mutex.
type chatConv struct {
	peer         string
	fingerprint  string
	verified     bool
	unread       int
	lastActivity time.Time
	msgs         []*ChatMessage
}

// ChatState owns the VayuTalk manager and its derived UI snapshot.
type ChatState struct {
	db         *store.DB
	keyring    *pgp.Keyring
	ks         appcrypto.Keystore
	invalidate func()
	notify     func(string)
	// OnIncoming fires (off the frame loop) when a new message arrives, so
	// the root can post a content-free system notification.
	OnIncoming func()

	mu            sync.Mutex
	mgr           *chat.Manager
	cancel        func()
	boundID       int64
	account       store.Account
	transitioning bool
	online        bool
	activePeer    string
	convs         map[string]*chatConv
}

// NewChatState builds a stopped chat holder. It opens no connection until
// EnsureStarted binds it to an account.
func NewChatState(db *store.DB, keyring *pgp.Keyring, ks appcrypto.Keystore, invalidate func(), notify func(string)) *ChatState {
	return &ChatState{
		db:         db,
		keyring:    keyring,
		ks:         ks,
		invalidate: invalidate,
		notify:     notify,
		convs:      map[string]*chatConv{},
	}
}

// Snapshot returns the current chat view. Cheap and race-free: it copies
// the live records under the lock.
func (cs *ChatState) Snapshot() ChatSnapshot {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	snap := ChatSnapshot{
		Connected:  cs.mgr != nil,
		Online:     cs.online,
		ActivePeer: cs.activePeer,
		SelfEmail:  cs.account.EmailAddress,
	}
	// Our own safety number, so the verify screen can show both keys (as the
	// web console does). Best-effort: absent until the key is synced/minted.
	if cs.keyring != nil && cs.account.EmailAddress != "" {
		if fp, err := cs.keyring.FingerprintForEmail(cs.account.EmailAddress); err == nil {
			snap.SelfFingerprint = fp
		}
	}
	convs := make([]ChatConversation, 0, len(cs.convs))
	for _, c := range cs.convs {
		convs = append(convs, ChatConversation{
			Peer:         c.peer,
			Fingerprint:  c.fingerprint,
			Verified:     c.verified,
			Unread:       c.unread,
			LastActivity: c.lastActivity,
		})
	}
	sort.Slice(convs, func(i, j int) bool {
		return convs[i].LastActivity.After(convs[j].LastActivity)
	})
	snap.Conversations = convs

	if active := cs.convs[cs.activePeer]; active != nil {
		snap.Verified = active.verified
		snap.Fingerprint = active.fingerprint
		snap.Messages = make([]ChatMessage, len(active.msgs))
		for i, m := range active.msgs {
			snap.Messages[i] = *m
		}
	}
	return snap
}

// fire wakes the window after a snapshot change.
func (cs *ChatState) fire() {
	if cs.invalidate != nil {
		cs.invalidate()
	}
}

func (cs *ChatState) note(msg string) {
	if cs.notify != nil {
		cs.notify(msg)
	}
}

// conv returns the record for peer, creating it if absent. Caller holds
// the lock.
func (cs *ChatState) conv(peer string) *chatConv {
	peer = strings.ToLower(strings.TrimSpace(peer))
	c := cs.convs[peer]
	if c == nil {
		c = &chatConv{peer: peer, lastActivity: time.Now()}
		cs.convs[peer] = c
	}
	return c
}

// clampBurn matches the server's [5s, 3600s] burn-after-read clamp so the local
// countdown ends when the server actually enforces the timer.
func clampBurn(d time.Duration) time.Duration {
	s := int(d / time.Second)
	if s < minBurnSeconds {
		s = minBurnSeconds
	}
	if s > maxBurnSeconds {
		s = maxBurnSeconds
	}
	return time.Duration(s) * time.Second
}

// SafetyNumber formats a fingerprint into readable groups for
// out-of-band comparison, delegating to the engine helper so screens do
// not import the chat package directly.
func SafetyNumber(fingerprint string) string { return chat.SafetyNumber(fingerprint) }

// domainOf returns the part of an email address after '@'.
func domainOf(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return email[i+1:]
	}
	return ""
}
