// Package pgp implements OpenPGP encryption, decryption, signing, and
// verification for VayuMail using ProtonMail/go-crypto (Apache-2.0).
//
// Key material handling: armored private keys are supplied by the caller,
// which fetches them from the platform keystore (Rule 6). This package
// never reads or writes key files.
package pgp

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// TrustLevel expresses how much the user trusts a public key.
type TrustLevel int

// Trust levels, lowest to highest.
const (
	// TrustUnknown is the default for freshly imported keys.
	TrustUnknown TrustLevel = iota
	// TrustMarginal marks a key the user has seen but not verified.
	TrustMarginal
	// TrustFull marks a key whose fingerprint the user verified.
	TrustFull
)

// ErrNoKey is returned when no key matches the requested identity.
var ErrNoKey = errors.New("pgp: no matching key")

// Keyring holds the user's OpenPGP keys in memory with per-fingerprint
// trust levels. It is safe for concurrent use.
type Keyring struct {
	mu       sync.RWMutex
	entities openpgp.EntityList
	trust    map[string]TrustLevel // hex fingerprint -> level
}

// NewKeyring returns an empty keyring.
func NewKeyring() *Keyring {
	return &Keyring{trust: make(map[string]TrustLevel)}
}

// ImportArmored adds every key in an armored keyring blob (public or
// private) and returns the fingerprints imported.
func (k *Keyring) ImportArmored(armoredKeys []byte) ([]string, error) {
	entities, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(armoredKeys))
	if err != nil {
		return nil, fmt.Errorf("pgp: import keys: %w", err)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	var fingerprints []string
	for _, e := range entities {
		fp := fingerprintOf(e)
		k.entities = append(k.entities, e)
		if _, seen := k.trust[fp]; !seen {
			k.trust[fp] = TrustUnknown
		}
		fingerprints = append(fingerprints, fp)
	}
	return fingerprints, nil
}

// ExportPublicArmored serializes the public part of the key with the given
// fingerprint as an armored blob.
func (k *Keyring) ExportPublicArmored(fingerprint string) ([]byte, error) {
	e := k.byFingerprint(fingerprint)
	if e == nil {
		return nil, ErrNoKey
	}
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp: armor: %w", err)
	}
	if err := e.Serialize(w); err != nil {
		return nil, fmt.Errorf("pgp: export public key: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("pgp: close armor: %w", err)
	}
	return buf.Bytes(), nil
}

// SetTrust records the user's trust decision for a fingerprint.
func (k *Keyring) SetTrust(fingerprint string, level TrustLevel) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	fp := strings.ToLower(fingerprint)
	if _, ok := k.trust[fp]; !ok {
		return ErrNoKey
	}
	k.trust[fp] = level
	return nil
}

// Trust returns the trust level recorded for a fingerprint.
func (k *Keyring) Trust(fingerprint string) TrustLevel {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.trust[strings.ToLower(fingerprint)]
}

// EntityByEmail returns the first key usable for the given email address.
func (k *Keyring) EntityByEmail(email string) (*openpgp.Entity, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	for _, e := range k.entities {
		for _, ident := range e.Identities {
			if strings.EqualFold(ident.UserId.Email, email) {
				return e, nil
			}
		}
	}
	return nil, fmt.Errorf("%w for %s", ErrNoKey, email)
}

// SigningEntity returns the first key with decrypted (or unencrypted)
// private material for the given email — the key used to sign outbound
// mail.
func (k *Keyring) SigningEntity(email string) (*openpgp.Entity, error) {
	e, err := k.EntityByEmail(email)
	if err != nil {
		return nil, err
	}
	if e.PrivateKey == nil {
		return nil, fmt.Errorf("%w: no private key for %s", ErrNoKey, email)
	}
	return e, nil
}

// EmailForFingerprint returns the primary identity email of a key.
func (k *Keyring) EmailForFingerprint(fingerprint string) (string, error) {
	e := k.byFingerprint(fingerprint)
	if e == nil {
		return "", ErrNoKey
	}
	for _, ident := range e.Identities {
		if ident.UserId.Email != "" {
			return ident.UserId.Email, nil
		}
	}
	return "", ErrNoKey
}

// Entities returns a snapshot of all keys for read-only iteration.
func (k *Keyring) Entities() openpgp.EntityList {
	k.mu.RLock()
	defer k.mu.RUnlock()
	out := make(openpgp.EntityList, len(k.entities))
	copy(out, k.entities)
	return out
}

func (k *Keyring) byFingerprint(fingerprint string) *openpgp.Entity {
	k.mu.RLock()
	defer k.mu.RUnlock()
	want := strings.ToLower(fingerprint)
	for _, e := range k.entities {
		if fingerprintOf(e) == want {
			return e
		}
	}
	return nil
}

func fingerprintOf(e *openpgp.Entity) string {
	return strings.ToLower(fmt.Sprintf("%x", e.PrimaryKey.Fingerprint))
}
