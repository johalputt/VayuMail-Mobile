package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// SealBlob encrypts data with AES-256-GCM under key, binding it to the
// context label. Used for the IMAP settings-sync blob (ADR-0008): the
// blob stored on the mail server is opaque without the device's master
// key.
func SealBlob(key, plaintext []byte, context string) ([]byte, error) {
	gcm, err := blobCipher(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, []byte(context)), nil
}

// OpenBlob decrypts a SealBlob result.
func OpenBlob(key, sealed []byte, context string) ([]byte, error) {
	gcm, err := blobCipher(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, fmt.Errorf("crypto: sealed blob too short")
	}
	plaintext, err := gcm.Open(nil, sealed[:gcm.NonceSize()],
		sealed[gcm.NonceSize():], []byte(context))
	if err != nil {
		return nil, fmt.Errorf("crypto: open blob: %w", err)
	}
	return plaintext, nil
}

func blobCipher(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: gcm: %w", err)
	}
	return gcm, nil
}
