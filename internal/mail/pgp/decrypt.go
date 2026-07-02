package pgp

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	pgperrors "github.com/ProtonMail/go-crypto/openpgp/errors"
)

// SignatureStatus is the verification outcome attached to a decrypted or
// verified message.
type SignatureStatus int

// Signature verification outcomes.
const (
	// SigNone means the message carried no signature.
	SigNone SignatureStatus = iota
	// SigValid means the signature verified against a known key.
	SigValid
	// SigInvalid means the signature failed verification — treat the
	// content as tampered.
	SigInvalid
	// SigUnknownKey means the signature was made by a key not in the
	// keyring; the content may be authentic but cannot be verified.
	SigUnknownKey
)

// Result carries the plaintext and signature verdict of one decryption.
type Result struct {
	Plaintext []byte
	Signature SignatureStatus
	// SignedByFingerprint is the hex fingerprint of the signing key when
	// one was identified.
	SignedByFingerprint string
}

// Decrypt decrypts an armored (or binary) PGP message with the keyring's
// private keys and verifies an embedded signature when present.
func (k *Keyring) Decrypt(ciphertext []byte) (*Result, error) {
	reader := io.Reader(bytes.NewReader(ciphertext))
	if block, err := armor.Decode(bytes.NewReader(ciphertext)); err == nil {
		reader = block.Body
	}

	md, err := openpgp.ReadMessage(reader, k.Entities(), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp: read message: %w", err)
	}
	plaintext, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, fmt.Errorf("pgp: decrypt body: %w", err)
	}

	res := &Result{Plaintext: plaintext, Signature: SigNone}
	if md.IsSigned {
		switch {
		case md.SignatureError != nil:
			if errors.Is(md.SignatureError, pgperrors.ErrUnknownIssuer) || md.SignedBy == nil {
				res.Signature = SigUnknownKey
			} else {
				res.Signature = SigInvalid
			}
		case md.SignedBy != nil:
			res.Signature = SigValid
			res.SignedByFingerprint = fmt.Sprintf("%x", md.SignedBy.PublicKey.Fingerprint)
		default:
			res.Signature = SigUnknownKey
		}
	}
	return res, nil
}

// VerifyDetached checks a detached armored signature over message bytes.
func (k *Keyring) VerifyDetached(message, armoredSig []byte) (SignatureStatus, error) {
	signer, err := openpgp.CheckArmoredDetachedSignature(
		k.Entities(), bytes.NewReader(message), bytes.NewReader(armoredSig), nil)
	if err != nil {
		if signer == nil {
			return SigUnknownKey, fmt.Errorf("pgp: verify: %w", err)
		}
		return SigInvalid, fmt.Errorf("pgp: verify: %w", err)
	}
	return SigValid, nil
}
