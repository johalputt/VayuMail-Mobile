package account

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
)

// SPKIHash returns the base64 SHA-256 hash of a certificate's Subject
// Public Key Info — the value stored in Account.PinnedSPKI and compared
// on every connection (ADR-0008). This is the same pin format HPKP used.
func SPKIHash(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// TLSConfig returns the TLS configuration for this account's
// connections. Without a pin it is ordinary WebPKI verification with the
// stated TLS 1.2 floor — the floor must not be a property of whether the
// operator chose to pin. With a pin, WebPKI verification still runs and
// additionally some certificate in the verified chain must match the
// pinned SPKI hash — defense against a compromised or coerced CA.
func (c *Config) TLSConfig() *tls.Config {
	if c.PinnedSPKI == "" {
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
	pin, host := c.PinnedSPKI, c.IMAPHost
	matchPin := func(chains [][]*x509.Certificate) error {
		for _, chain := range chains {
			for _, cert := range chain {
				if SPKIHash(cert) == pin {
					return nil
				}
			}
		}
		return fmt.Errorf("account: TLS key pin mismatch for %s — possible interception, connection refused", host)
	}
	return &tls.Config{
		// Stated rather than inherited. Go's client default happens to be TLS
		// 1.2 today, so this changes nothing now — but a mail client's floor
		// should not be a property of the toolchain it was compiled with, and
		// an enforcement test can only check a value that is written down.
		MinVersion: tls.VersionTLS12,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return matchPin(verifiedChains)
		},
		// VerifyPeerCertificate alone is not enough, and the difference is the
		// whole control. It is called during certificate verification, and a
		// resumed session skips certificate verification — the peer's chain
		// comes back out of the session ticket. A pin checked only there is a
		// first-handshake-only pin: rotate it, revoke trust in the key, or
		// re-pin the account after a suspected interception, and none of it
		// takes effect until the cached session expires.
		//
		// VerifyConnection runs on every handshake, full or resumed, so the
		// answer to "is this still the key we pinned?" is asked every time a
		// connection is made rather than once per session.
		VerifyConnection: func(cs tls.ConnectionState) error {
			return matchPin(cs.VerifiedChains)
		},
	}
}
