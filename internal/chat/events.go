package chat

import "time"

// Event is the closed interface for everything the chat layer reports to
// the UI. Events() is the ONLY UI touchpoint; all network work happens in
// goroutines and surfaces here. The UI drains events non-blockingly.
type Event interface{ isEvent() }

// IncomingMessage is a decrypted message ready to display. Plaintext lives
// only in memory; the UI must call Ack once shown so the server destroys
// the envelope.
type IncomingMessage struct {
	Peer      string
	ID        string
	Plaintext string
	// Authentic is true only when the envelope carried a VALID OpenPGP signature
	// from a key in our keyring. The server-supplied Peer/From is NOT proof of
	// origin — a malicious relay can set any From and encrypt to our public key —
	// so the UI must never present an inauthentic message as coming from a
	// verified peer (audit H7).
	Authentic bool
	// SenderFingerprint is the hex fingerprint of the signing key when Authentic;
	// it is the real, cryptographic sender identity, to be matched against a
	// conversation's verified fingerprint.
	SenderFingerprint string
	// CreatedAt is the server's authoritative send time (from the envelope),
	// so a message shows when it was SENT, not when this device happened to
	// receive it — which matters for a message queued while we were offline.
	CreatedAt time.Time
	ExpiresAt time.Time
	// BurnSeconds is the self-destruct timer that starts when this message is
	// revealed (read), carried from the sender via the envelope.
	BurnSeconds int
	Mode        string
}

// MessageRead reports that a message this client sent was read (acked) by
// the recipient. Peer is filled in from the send record when known.
type MessageRead struct {
	Peer string
	ID   string
}

// MessageExpired reports that a sent message reached its TTL undelivered
// and was purged server-side.
type MessageExpired struct {
	ID string
}

// Delivered reports the outcome of a Send: OK is true when a live stream
// received it now (live mode) or it was queued (store mode).
type Delivered struct {
	ID string
	OK bool
}

// PeerKey reports a correspondent's fetched public key. Fingerprint is
// public data; Verified reflects a prior out-of-band verification.
type PeerKey struct {
	Peer        string
	Fingerprint string
	Verified    bool
}

// ConnState reports the live stream going online or offline. Err carries
// the reason when Online is false and a failure occurred.
type ConnState struct {
	Online bool
	Err    error
}

func (IncomingMessage) isEvent() {}
func (MessageRead) isEvent()     {}
func (MessageExpired) isEvent()  {}
func (Delivered) isEvent()       {}
func (PeerKey) isEvent()         {}
func (ConnState) isEvent()       {}
