package pgp

import (
	"bytes"
	"fmt"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// Encrypt encrypts plaintext to every recipient email and, when signer is
// non-empty, signs it with the sender's private key. The result is an
// armored PGP message ready for smtpsend.BuildPGPEncrypted.
func (k *Keyring) Encrypt(plaintext []byte, recipientEmails []string, signerEmail string) ([]byte, error) {
	if len(recipientEmails) == 0 {
		return nil, fmt.Errorf("pgp: encrypt: no recipients")
	}
	recipients := make([]*openpgp.Entity, 0, len(recipientEmails))
	for _, email := range recipientEmails {
		e, err := k.EntityByEmail(email)
		if err != nil {
			return nil, fmt.Errorf("pgp: encrypt to %s: %w", email, err)
		}
		recipients = append(recipients, e)
	}

	var signer *openpgp.Entity
	if signerEmail != "" {
		var err error
		signer, err = k.SigningEntity(signerEmail)
		if err != nil {
			return nil, fmt.Errorf("pgp: sign as %s: %w", signerEmail, err)
		}
	}

	var buf bytes.Buffer
	armorWriter, err := armor.Encode(&buf, "PGP MESSAGE", nil)
	if err != nil {
		return nil, fmt.Errorf("pgp: armor: %w", err)
	}
	msgWriter, err := openpgp.Encrypt(armorWriter, recipients, signer, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp: encrypt: %w", err)
	}
	if _, err := msgWriter.Write(plaintext); err != nil {
		return nil, fmt.Errorf("pgp: write plaintext: %w", err)
	}
	if err := msgWriter.Close(); err != nil {
		return nil, fmt.Errorf("pgp: finalize message: %w", err)
	}
	if err := armorWriter.Close(); err != nil {
		return nil, fmt.Errorf("pgp: close armor: %w", err)
	}
	return buf.Bytes(), nil
}

// Sign produces a detached armored signature over the message bytes with
// the sender's private key.
func (k *Keyring) Sign(message []byte, signerEmail string) ([]byte, error) {
	signer, err := k.SigningEntity(signerEmail)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&buf, signer, bytes.NewReader(message), nil); err != nil {
		return nil, fmt.Errorf("pgp: detach sign: %w", err)
	}
	return buf.Bytes(), nil
}
