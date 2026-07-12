package state

// chatstate_events.go — VayuTalk lifecycle (bind/unbind an account) and
// the single goroutine that drains the engine's Events() channel into the
// snapshot. The engine never runs on the frame loop; the drain wakes the
// window through invalidate after folding each event (Rule 5).

import (
	"context"
	"net/http"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/chat"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// EnsureStarted binds VayuTalk to acct, starting the manager if it is not
// already bound to that account. It never blocks the caller: tearing down
// a previous manager and connecting the new one happen on a goroutine, so
// this is safe to call from a screen's layout.
func (cs *ChatState) EnsureStarted(acct store.Account) {
	if acct.ID == 0 || acct.EmailAddress == "" {
		return
	}
	cs.mu.Lock()
	if cs.boundID == acct.ID && (cs.mgr != nil || cs.transitioning) {
		cs.mu.Unlock()
		return
	}
	if cs.transitioning {
		cs.mu.Unlock()
		return
	}
	cs.transitioning = true
	cs.boundID = acct.ID
	prevCancel := cs.cancel
	prevMgr := cs.mgr
	cs.cancel = nil
	cs.mgr = nil
	cs.mu.Unlock()

	go func() {
		if prevCancel != nil {
			prevCancel()
		}
		if prevMgr != nil {
			prevMgr.Close()
		}
		cs.startManager(acct)
	}()
}

// startManager constructs and starts a fresh manager for acct and spins
// up its drain goroutine.
func (cs *ChatState) startManager(acct store.Account) {
	alias := acct.KeystoreAlias
	credential := func() (string, error) {
		secret, err := cs.ks.Fetch(alias)
		if err != nil {
			return "", err
		}
		return string(secret), nil
	}
	// Make sure this device holds the server's CURRENT private key for the
	// mailbox before we start receiving. VayuTalk on the web encrypts to the key
	// the server holds for a recipient, so a device whose key has drifted — or
	// was never synced — would silently fail to decrypt web-sent messages. This
	// is a best-effort, idempotent self-heal on every chat start: importing the
	// authoritative key makes the whole keyring able to open anything encrypted
	// to this mailbox, on either surface.
	cs.syncOwnKey(acct.EmailAddress, credential)

	mgr := chat.New(chat.Config{
		Keyring:    cs.keyring,
		SelfEmail:  acct.EmailAddress,
		Domain:     domainOf(acct.EmailAddress),
		Credential: credential,
		Settings:   cs.db,
	})
	ctx, cancel := context.WithCancel(context.Background())
	mgr.Start(ctx)

	cs.mu.Lock()
	cs.mgr = mgr
	cs.cancel = cancel
	cs.account = acct
	cs.transitioning = false
	// A rebind to a different account starts from a clean conversation set.
	cs.convs = map[string]*chatConv{}
	cs.activePeer = ""
	cs.online = false
	cs.mu.Unlock()

	go cs.drain(ctx, mgr)
	cs.fire()
}

// syncOwnKey fetches the mailbox's authoritative private key from its VayuPress
// server and imports it into the shared keyring, so this device can always
// decrypt anything encrypted to the mailbox — including VayuTalk messages the
// web composes against the server-held key. Best-effort: on any failure the
// device keeps whatever key it already has. Runs on startManager's goroutine
// (already off the frame loop), so the brief network call is fine here.
func (cs *ChatState) syncOwnKey(email string, credential func() (string, error)) {
	if cs.keyring == nil || email == "" {
		return
	}
	pw, err := credential()
	if err != nil || pw == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	armored, err := account.FetchPrivateKey(ctx, http.DefaultClient, email, pw)
	if err != nil || armored == "" {
		return
	}
	fps, err := cs.keyring.ImportArmored([]byte(armored))
	if err != nil {
		return
	}
	// Persist so the key survives a restart and is available to mail decryption
	// too, not only this chat session.
	if cs.db != nil {
		for _, fp := range fps {
			pctx, pcancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = cs.db.UpsertPGPKey(pctx, &store.PGPKey{Fingerprint: fp, Email: email, Armored: armored, IsPrivate: true})
			pcancel()
		}
	}
}

// Stop tears VayuTalk down (used on logout). The blocking Close runs on a
// goroutine so the caller — potentially the frame loop — never stalls.
func (cs *ChatState) Stop() {
	cs.mu.Lock()
	cancel := cs.cancel
	mgr := cs.mgr
	cs.cancel = nil
	cs.mgr = nil
	cs.boundID = 0
	cs.transitioning = false
	cs.account = store.Account{}
	cs.convs = map[string]*chatConv{}
	cs.activePeer = ""
	cs.online = false
	cs.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if mgr != nil {
		go mgr.Close()
	}
	cs.fire()
}

// drain folds engine events into the snapshot until the context is
// cancelled. Events() is never closed by the engine, so the context is
// the sole exit.
func (cs *ChatState) drain(ctx context.Context, mgr *chat.Manager) {
	events := mgr.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			cs.apply(ev)
			cs.fire()
		}
	}
}

// apply folds one engine event into the conversation records.
func (cs *ChatState) apply(ev chat.Event) {
	switch e := ev.(type) {
	case chat.IncomingMessage:
		cs.applyIncoming(e)
	case chat.Delivered:
		cs.setStatusByID(e.ID, func(m *ChatMessage) {
			if m.Mode == "store" {
				m.Status = MsgQueued
			} else {
				m.Status = MsgSent
			}
		})
	case chat.MessageRead:
		cs.setStatusByID(e.ID, func(m *ChatMessage) { m.Status = MsgRead })
	case chat.MessageExpired:
		cs.setStatusByID(e.ID, func(m *ChatMessage) { m.Status = MsgExpired })
	case chat.PeerKey:
		cs.mu.Lock()
		c := cs.conv(e.Peer)
		c.fingerprint = e.Fingerprint
		c.verified = e.Verified
		cs.mu.Unlock()
	case chat.ConnState:
		cs.mu.Lock()
		cs.online = e.Online
		cs.mu.Unlock()
	}
}

// applyIncoming records a newly arrived (still sealed) message and, when
// it is not for the open conversation, bumps the unread count and asks
// the root to post a content-free notification.
func (cs *ChatState) applyIncoming(e chat.IncomingMessage) {
	cs.mu.Lock()
	c := cs.conv(e.Peer)
	// A store-mode envelope stays queued on the server until it is revealed
	// (that is when we ack it), so a reconnect re-flushes anything unread. Skip
	// a message we already hold so the persistent connection can reconnect
	// freely without duplicating un-revealed messages.
	for _, existing := range c.msgs {
		if existing.ID == e.ID {
			cs.mu.Unlock()
			return
		}
	}
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	c.msgs = append(c.msgs, &ChatMessage{
		ID:        e.ID,
		Peer:      c.peer,
		Text:      e.Plaintext,
		CreatedAt: created,
		ExpiresAt: e.ExpiresAt,
		Status:    MsgSealed,
	})
	c.lastActivity = time.Now()
	background := cs.activePeer != c.peer
	if background {
		c.unread++
	}
	cs.mu.Unlock()

	if background && cs.OnIncoming != nil {
		cs.OnIncoming()
	}
}

// setStatusByID applies mut to the message with the given id, if present.
func (cs *ChatState) setStatusByID(id string, mut func(*ChatMessage)) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, c := range cs.convs {
		for _, m := range c.msgs {
			if m.ID == id {
				mut(m)
				c.lastActivity = time.Now()
				return
			}
		}
	}
}
