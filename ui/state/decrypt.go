package state

// decrypt.go — decrypt-on-display for received PGP mail. Encrypted mail
// is stored as ciphertext (never decrypted on disk); when a thread is
// opened, its encrypted bodies are decrypted in memory with the
// account's private key. Runs on the loader goroutine, off the frame
// loop (Rule 5).

import (
	"strings"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

// noPrivateKeyNotice is shown when an encrypted message cannot be
// decrypted because the account's private key is not on the device yet.
const noPrivateKeyNotice = "🔒 Encrypted message. Your private key isn't on this device yet — Settings → Encryption → “Sync my key from VayuPress”, then reopen this message."

// refetchingNotice is shown while a broken cached copy (stored before
// the PGP/MIME parser fix) is re-downloaded from the server.
const refetchingNotice = "🔒 Encrypted message — fetching the locked copy from your server… it will open here in a moment."

// unreadableNotice is shown when a re-download still yielded nothing
// decryptable — the terminal state, never another fetch.
const unreadableNotice = "🔒 Encrypted message. This copy couldn't be opened on the device — read it in your VayuPress webmail, or update VayuPress to 3.11.33+ so the server can open it for this app."

// decryptThread returns a copy of the thread with every encrypted body
// replaced by readable content (in memory only). Four shapes arrive:
// armored ciphertext (decrypt with the keyring), plaintext already
// opened by the server's transparent decryption (display as-is), a
// stale broken row from an older parser (re-fetch ONCE, then stop), and
// an empty body (same re-fetch-once path). The once matters: without a
// terminal state a row that re-fetches to the same bytes loops forever.
func (s *AppState) decryptThread(msgs []store.Message) []store.Message {
	out := make([]store.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		m := &out[i]
		if !isEncryptedStatus(m.PGPStatus) {
			continue
		}
		body := strings.TrimSpace(m.BodyText)
		if strings.Contains(body, "BEGIN PGP MESSAGE") {
			res, err := s.keyring.Decrypt([]byte(body))
			if err != nil || res == nil || len(res.Plaintext) == 0 {
				m.BodyText = noPrivateKeyNotice
				m.BodyHTML = ""
				continue
			}
			// Decrypted content may itself be a MIME body; the thread view
			// runs it through mime.DisplayText, so plain text is enough.
			m.BodyText = string(res.Plaintext)
			m.BodyHTML = ""
			continue
		}
		if isJunkEncryptedBody(body) {
			// Stored by an older parser (the PGP/MIME control part or an
			// empty body). Re-fetch once; afterwards this state is final.
			switch s.refetchState(m.ID) {
			case refetchIdle:
				s.requestRefetch(m.AccountID, m.ID)
				m.BodyText = refetchingNotice
			case refetchInFlight:
				m.BodyText = refetchingNotice
			default: // refetchDone — the repaired copy is what it is.
				m.BodyText = unreadableNotice
			}
			m.BodyHTML = ""
			continue
		}
		// Non-junk plaintext without armor: the server already opened it
		// (transparent decryption leaves X-VayuPGP + readable text).
		// Display it — the encrypted badge still shows via PGPStatus.
	}
	return out
}

// isJunkEncryptedBody reports whether an encrypted message's stored body
// is unusable structure residue rather than content: empty, the RFC 3156
// control part, or a bare armor header without its block.
func isJunkEncryptedBody(body string) bool {
	if body == "" || body == "Version: 1" {
		return true
	}
	// A body that is only whitespace/structure noise under a few bytes
	// can't be real content either.
	return len(body) < 8
}

type refetchPhase int

const (
	refetchIdle refetchPhase = iota
	refetchInFlight
	refetchDone
)

// refetchState reports where a message is in its single repair attempt.
func (s *AppState) refetchState(messageID int64) refetchPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.refetching[messageID]:
		return refetchInFlight
	case s.refetched[messageID]:
		return refetchDone
	default:
		return refetchIdle
	}
}

// requestRefetch asks the engine to re-download one message's body,
// once — repeat calls while the fetch is in flight are dropped, and a
// completed attempt is never repeated this session. Runs on the loader
// goroutine, so the maps are guarded by the state lock.
func (s *AppState) requestRefetch(accountID, messageID int64) {
	s.mu.Lock()
	if s.refetching[messageID] || s.refetched[messageID] {
		s.mu.Unlock()
		return
	}
	s.refetching[messageID] = true
	s.mu.Unlock()
	s.Send(syncmanager.RefetchMessageCmd{AccountID: accountID, MessageID: messageID})
}

// isEncryptedStatus reports whether a stored pgp_status implies an
// encrypted body.
func isEncryptedStatus(status string) bool {
	return status == "encrypted" || status == "signed+encrypted"
}
