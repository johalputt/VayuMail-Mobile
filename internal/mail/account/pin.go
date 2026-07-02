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
// connections. Without a pin it is nil (standard WebPKI verification).
// With a pin, WebPKI verification still runs and additionally some
// certificate in the verified chain must match the pinned SPKI hash —
// defense against a compromised or coerced CA.
func (c *Config) TLSConfig() *tls.Config {
	if c.PinnedSPKI == "" {
		return nil
	}
	pin := c.PinnedSPKI
	return &tls.Config{
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			for _, chain := range verifiedChains {
				for _, cert := range chain {
					if SPKIHash(cert) == pin {
						return nil
					}
				}
			}
			return fmt.Errorf("account: TLS key pin mismatch for %s — possible interception, connection refused", c.IMAPHost)
		},
	}
}
