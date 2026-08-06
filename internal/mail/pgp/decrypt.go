package pgp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

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
	// SenderVerified is the only field a UI should use to claim a message
	// came from who it says it did.
	//
	// Signature == SigValid answers a much weaker question: did SOME key on
	// this keyring sign this? A keyring holds every correspondent's public
	// key, so anyone whose key is on it can sign a message carrying any From
	// header at all and satisfy SigValid. The From header is attacker-
	// controlled SMTP text; the signing key is the only identity in the
	// message that is not. SenderVerified is true only when the two agree.
	SenderVerified bool
}

// Decrypt decrypts an armored (or binary) PGP message and verifies an embedded
// signature, WITHOUT binding it to a claimed sender — SenderVerified is always
// false. Use it only where the caller does its own binding, as internal/chat
// does by comparing SignedByFingerprint against the verified peer.
//
// For mail, use DecryptFrom.
func (k *Keyring) Decrypt(ciphertext []byte) (*Result, error) {
	return k.DecryptFrom(ciphertext, "")
}

// DecryptFrom decrypts a message and reports whether the signature was made by
// a key belonging to expectedSender, which may be a bare address or a full
// From header ("Alice <alice@example.com>"). An expectedSender that is empty or
// unparseable never verifies.
func (k *Keyring) DecryptFrom(ciphertext []byte, expectedSender string) (*Result, error) {
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
			res.SenderVerified = k.SignerMatchesEmail(res.SignedByFingerprint, expectedSender)
		default:
			res.Signature = SigUnknownKey
		}
	}
	return res, nil
}

// SignerMatchesEmail reports whether the key with the given fingerprint carries
// an identity for the address in claimed, which may be a bare address or a full
// header with a display name.
//
// Everything about this is deliberately conservative: an empty or unparseable
// address matches nothing, and a fingerprint that is not on the keyring matches
// nothing. The dangerous default would be to treat "I could not tell" as a
// match, because that is indistinguishable from success at the call site.
func (k *Keyring) SignerMatchesEmail(fingerprint, claimed string) bool {
	addr := normalizeAddress(claimed)
	if addr == "" || fingerprint == "" {
		return false
	}
	e := k.byFingerprint(fingerprint)
	if e == nil {
		return false
	}
	for _, ident := range e.Identities {
		if strings.EqualFold(strings.TrimSpace(ident.UserId.Email), addr) {
			return true
		}
	}
	return false
}

// normalizeAddress extracts a lowercase bare address from a From header.
// Returns "" for anything that is not recognisably an address, which callers
// treat as "no match".
func normalizeAddress(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// "Alice <alice@example.com>" — the angle-bracketed form wins, because a
	// display name is free text and may itself contain an @ chosen to be
	// mistaken for the real address.
	if i := strings.LastIndex(s, "<"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j > 0 {
			s = s[i+1 : i+j]
		}
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if at := strings.Index(s, "@"); at <= 0 || at == len(s)-1 || strings.ContainsAny(s, " \t\"") {
		return ""
	}
	return s
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
