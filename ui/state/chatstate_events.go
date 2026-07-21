package state

// chatstate_events.go — VayuTalk lifecycle (bind/unbind an account) and
// the single goroutine that drains the engine's Events() channel into the
// snapshot. The engine never runs on the frame loop; the drain wakes the
// window through invalidate after folding each event (Rule 5).

import (
	"context"
	"errors"
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
	email := acct.EmailAddress
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	_ = cs.syncOwnKey(sctx, email, credential)
	scancel()

	// Route the VayuTalk API at the host the server advertises for it — a
	// dedicated subdomain the operator points straight at the origin with any CDN
	// proxy OFF, so the app's long-lived SSE stream is never buffered or
	// bot-challenged. Falls back to the mail domain when none is advertised or
	// reachable, so servers without a talk subdomain are unaffected. Best-effort
	// and quick; runs on this goroutine (already off the frame loop).
	talkDomain := domainOf(email)
	tctx, tcancel := context.WithTimeout(context.Background(), 12*time.Second)
	if h := account.ResolveTalkHost(tctx, http.DefaultClient, email); h != "" {
		talkDomain = h
	}
	tcancel()

	mgr := chat.New(chat.Config{
		Keyring:    cs.keyring,
		SelfEmail:  email,
		Domain:     talkDomain,
		Credential: credential,
		Settings:   cs.db,
		// Self-heal: if an incoming message can't be decrypted, our key has
		// drifted from the server's — re-fetch it and retry rather than dropping.
		ResyncKey: func(ctx context.Context) error { return cs.syncOwnKey(ctx, email, credential) },
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
func (cs *ChatState) syncOwnKey(ctx context.Context, email string, credential func() (string, error)) error {
	if cs.keyring == nil || email == "" {
		return errors.New("chat: no keyring or email for key sync")
	}
	pw, err := credential()
	if err != nil {
		return err
	}
	if pw == "" {
		return errors.New("chat: no credential for key sync")
	}
	armored, err := account.FetchPrivateKey(ctx, http.DefaultClient, email, pw)
	if err != nil {
		return err
	}
	if armored == "" {
		return errors.New("chat: server returned no private key")
	}
	fps, err := cs.keyring.ImportArmored([]byte(armored))
	if err != nil {
		return err
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
	return nil
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
		// The recipient opened it: mark read and start our copy's burn countdown
		// so the sender's bubble disappears on the same clock as the recipient's.
		cs.setStatusByID(e.ID, func(m *ChatMessage) {
			m.Status = MsgRead
			if m.ExpiresAt.IsZero() {
				now := time.Now()
				m.ArmedAt = now
				m.ExpiresAt = now.Add(m.burnDuration())
			}
		})
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
		ID:          e.ID,
		Peer:        c.peer,
		Text:        e.Plaintext,
		CreatedAt:   created,
		BurnSeconds: e.BurnSeconds,
		Mode:        e.Mode,
		// ExpiresAt stays zero until the message is revealed — the burn-after-read
		// countdown starts on reveal, not on the server's unread-hold deadline.
		Status: MsgSealed,
	})
	c.lastActivity = time.Now()
	background := cs.activePeer != c.peer
	if background {
		c.unread++
	}
	cs.mu.Unlock()

	if background && cs.OnIncoming != nil {
		cs.OnIncoming(c.peer)
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
