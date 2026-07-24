package state

// pgpvault.go — custody for PGP PRIVATE key material (audit H6).
//
// Private keys must never sit in SQLite: the app DB is world-readable to
// anything that can reach the app-private directory after a device
// compromise, and unlike the sealed keystore it is not encrypted at rest.
// This file routes every private-key armored blob into the platform
// keystore (Android Keystore / iOS Keychain in production, an
// AES-256-GCM sealed file otherwise), keyed by fingerprint. The pgp_keys
// table then holds only public material plus metadata, exactly as its
// doc-comment always promised.

import (
	"strings"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
)

// pgpVaultPrefix namespaces sealed private-key entries so they never
// collide with the mailbox credential or app-lock verifier that share the
// same keystore.
const pgpVaultPrefix = "pgpsec:"

// pgpVaultAlias is the keystore alias holding one fingerprint's sealed
// private key.
func pgpVaultAlias(fingerprint string) string {
	return pgpVaultPrefix + strings.ToLower(strings.TrimSpace(fingerprint))
}

// sealPrivateKey stores a private key's armored bytes in the platform
// keystore (encrypted at rest), never in SQLite (audit H6).
func sealPrivateKey(ks appcrypto.Keystore, fingerprint, armored string) error {
	if ks == nil {
		return appcrypto.ErrNoPlatformKeystore
	}
	return ks.Store(pgpVaultAlias(fingerprint), []byte(armored))
}

// openPrivateKey returns the sealed armored private key for a fingerprint,
// or appcrypto.ErrKeyNotFound if none is stored.
func openPrivateKey(ks appcrypto.Keystore, fingerprint string) (string, error) {
	if ks == nil {
		return "", appcrypto.ErrNoPlatformKeystore
	}
	b, err := ks.Fetch(pgpVaultAlias(fingerprint))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// deleteSealedPrivateKey removes a fingerprint's sealed private key.
// Missing entries are not an error.
func deleteSealedPrivateKey(ks appcrypto.Keystore, fingerprint string) {
	if ks == nil {
		return
	}
	_ = ks.Delete(pgpVaultAlias(fingerprint))
}

// isPrivateArmored reports whether an armored blob contains private key
// material (so it must be sealed, never written to SQLite).
func isPrivateArmored(armored string) bool {
	return strings.Contains(armored, "PRIVATE KEY BLOCK")
}
