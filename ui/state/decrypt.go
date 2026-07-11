package state

// decrypt.go — decrypt-on-display for received PGP mail. Encrypted mail
// is stored as ciphertext (never decrypted on disk); when a thread is
// opened, its encrypted bodies are decrypted in memory with the
// account's private key. Runs on the loader goroutine, off the frame
// loop (Rule 5).

import (
	"strings"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// noPrivateKeyNotice is shown when an encrypted message cannot be
// decrypted because the account's private key is not on the device yet.
const noPrivateKeyNotice = "🔒 Encrypted message. Your private key isn't on this device yet — Settings → Encryption → “Sync my key from VayuPress”, then reopen this message."

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
			// Nothing decryptable was captured; leave the row's notice.
			if m.BodyText == "" {
				m.BodyText = noPrivateKeyNotice
			}
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

// isEncryptedStatus reports whether a stored pgp_status implies an
// encrypted body.
func isEncryptedStatus(status string) bool {
	return status == "encrypted" || status == "signed+encrypted"
}
