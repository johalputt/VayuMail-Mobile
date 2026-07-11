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

// decryptThread returns a copy of the thread with every encrypted body
// replaced by its plaintext (in memory only). Messages the keyring
// cannot decrypt get a clear notice instead of raw ciphertext or a
// blank body.
func (s *AppState) decryptThread(msgs []store.Message) []store.Message {
	out := make([]store.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		m := &out[i]
		if !isEncryptedStatus(m.PGPStatus) {
			continue
		}
		cipher := strings.TrimSpace(m.BodyText)
		if cipher == "" || !strings.Contains(cipher, "BEGIN PGP MESSAGE") {
			// No ciphertext in the cached row — it was stored by an older
			// parser (e.g. as the PGP/MIME "Version: 1" control part) or
			// the body was never downloaded. Re-fetch it once; the reload
			// after MessageRefetchedEvent decrypts the repaired copy.
			s.requestRefetch(m.AccountID, m.ID)
			m.BodyText = refetchingNotice
			m.BodyHTML = ""
			continue
		}
		res, err := s.keyring.Decrypt([]byte(cipher))
		if err != nil || res == nil || len(res.Plaintext) == 0 {
			m.BodyText = noPrivateKeyNotice
			m.BodyHTML = ""
			continue
		}
		// Decrypted content may itself be a MIME body; the thread view
		// runs it through mime.DisplayText, so plain text is enough here.
		m.BodyText = string(res.Plaintext)
		m.BodyHTML = ""
	}
	return out
}

// requestRefetch asks the engine to re-download one message's body,
// once — repeat calls while the fetch is in flight are dropped. Runs on
// the loader goroutine, so the map is guarded by the state lock.
func (s *AppState) requestRefetch(accountID, messageID int64) {
	s.mu.Lock()
	if s.refetching[messageID] {
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
