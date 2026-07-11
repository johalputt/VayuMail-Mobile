package state

// chatstate_ops.go — VayuTalk user actions. Each forwards to the engine
// on its own goroutine (the engine's Send/Ack/VerifyPeer do network I/O)
// and updates the snapshot when the call returns. None of them touch the
// frame loop.

import (
	"context"
	"strings"
	"time"
)

// opTimeout bounds a single unary chat operation. The engine already
// applies a 30s per-call timeout internally; this is the outer bound that
// also covers a lazy Connect.
const opTimeout = 40 * time.Second

// SendMessage encrypts and sends plaintext to peer with the given TTL and
// mode ("live" or "store"), then records it locally as an outgoing
// bubble. Failures surface through the snackbar.
func (cs *ChatState) SendMessage(peer, text string, ttl time.Duration, mode string) {
	peer = strings.ToLower(strings.TrimSpace(peer))
	text = strings.TrimSpace(text)
	if peer == "" || text == "" {
		return
	}
	cs.mu.Lock()
	mgr := cs.mgr
	cs.mu.Unlock()
	if mgr == nil {
		cs.note("VayuTalk is not connected yet")
		return
	}
	clamped := clampTTL(ttl)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		id, err := mgr.Send(ctx, peer, text, clamped, mode)
		if err != nil {
			cs.note("Message not sent — is the contact verified?")
			return
		}
		now := time.Now()
		cs.mu.Lock()
		c := cs.conv(peer)
		c.msgs = append(c.msgs, &ChatMessage{
			ID:        id,
			Peer:      c.peer,
			Self:      true,
			Text:      text,
			CreatedAt: now,
			ExpiresAt: now.Add(clamped),
			Mode:      mode,
			Status:    MsgSending,
		})
		c.lastActivity = now
		cs.mu.Unlock()
		cs.fire()
	}()
}

// RevealMessage uncovers a received message and, because reading destroys
// it, immediately acknowledges it to the server (read-once). The bubble
// then shows its plaintext until its TTL runs out.
func (cs *ChatState) RevealMessage(peer, id string) {
	cs.mu.Lock()
	mgr := cs.mgr
	var found bool
	if c := cs.convs[strings.ToLower(strings.TrimSpace(peer))]; c != nil {
		for _, m := range c.msgs {
			if m.ID == id && !m.Self {
				m.Revealed = true
				m.Status = MsgOpen
				found = true
			}
		}
	}
	cs.mu.Unlock()
	if !found || mgr == nil {
		return
	}
	cs.fire()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		_ = mgr.Ack(ctx, id)
	}()
}

// VerifyPeer fetches and imports the peer's public key; the resulting
// PeerKey event updates the conversation's fingerprint and verified flag.
func (cs *ChatState) VerifyPeer(peer string) {
	peer = strings.ToLower(strings.TrimSpace(peer))
	cs.mu.Lock()
	mgr := cs.mgr
	cs.mu.Unlock()
	if mgr == nil {
		cs.note("VayuTalk is not connected yet")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		if err := mgr.VerifyPeer(ctx, peer); err != nil {
			cs.note("Could not fetch a key for " + peer)
		}
	}()
}

// SetVerified records the user's out-of-band verification decision for the
// active peer's fingerprint and reflects it immediately in the snapshot.
func (cs *ChatState) SetVerified(peer string, verified bool) {
	peer = strings.ToLower(strings.TrimSpace(peer))
	cs.mu.Lock()
	mgr := cs.mgr
	c := cs.convs[peer]
	fp := ""
	if c != nil {
		fp = c.fingerprint
		c.verified = verified
	}
	cs.mu.Unlock()
	if mgr == nil || fp == "" {
		return
	}
	cs.fire()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		if err := mgr.SetPeerVerified(ctx, fp, verified); err != nil {
			cs.note("Could not save verification")
		}
	}()
}

// OpenConversation makes peer active and clears its unread badge. When the
// conversation is new (from "New chat") it also kicks off a key fetch.
func (cs *ChatState) OpenConversation(peer string) {
	peer = strings.ToLower(strings.TrimSpace(peer))
	if peer == "" {
		return
	}
	cs.mu.Lock()
	c := cs.conv(peer)
	c.unread = 0
	cs.activePeer = peer
	needKey := c.fingerprint == ""
	cs.mu.Unlock()
	cs.fire()
	if needKey {
		cs.VerifyPeer(peer)
	}
}

// CloseConversation deselects the active peer (used when leaving a room).
func (cs *ChatState) CloseConversation() {
	cs.mu.Lock()
	cs.activePeer = ""
	cs.mu.Unlock()
}
