// Package account defines mail account configuration and the QR
// provisioning protocol (ADR-0003). It holds no credentials: a Config
// names a platform-keystore alias, never a secret (Constitutional Rule 6).
package account

import (
	"fmt"
)

// TLSMode selects how a connection to a mail server is secured.
type TLSMode string

// Supported transport security modes. Plaintext is intentionally absent:
// the schema, the QR verifier, and the dialers all reject it.
const (
	// TLSModeImplicit is TLS from the first byte (IMAPS/SMTPS).
	TLSModeImplicit TLSMode = "tls"
	// TLSModeSTARTTLS upgrades a plaintext connection before any
	// credential or mail data is exchanged.
	TLSModeSTARTTLS TLSMode = "starttls"
)

// Valid reports whether m is a supported TLS mode.
func (m TLSMode) Valid() bool {
	return m == TLSModeImplicit || m == TLSModeSTARTTLS
}

// Config is the network identity of one mail account. It is safe to
// persist: the credential itself lives in the platform keystore under
// KeystoreAlias and is fetched only at connection time.
type Config struct {
	DisplayName   string
	EmailAddress  string
	IMAPHost      string
	IMAPPort      int
	IMAPTLS       TLSMode
	SMTPHost      string
	SMTPPort      int
	SMTPTLS       TLSMode
	Username      string
	KeystoreAlias string
	// PinnedSPKI optionally pins the server TLS key (see pin.go).
	PinnedSPKI string
}

// Validate checks that the configuration is complete and secure enough to
// open connections with.
func (c *Config) Validate() error {
	switch {
	case c.EmailAddress == "":
		return fmt.Errorf("account: missing email address")
	case c.IMAPHost == "":
		return fmt.Errorf("account: missing IMAP host")
	case c.SMTPHost == "":
		return fmt.Errorf("account: missing SMTP host")
	case c.Username == "":
		return fmt.Errorf("account: missing username")
	case c.KeystoreAlias == "":
		return fmt.Errorf("account: missing keystore alias")
	}
	if !validPort(c.IMAPPort) {
		return fmt.Errorf("account: invalid IMAP port %d", c.IMAPPort)
	}
	if !validPort(c.SMTPPort) {
		return fmt.Errorf("account: invalid SMTP port %d", c.SMTPPort)
	}
	if !c.IMAPTLS.Valid() {
		return fmt.Errorf("account: invalid IMAP TLS mode %q", c.IMAPTLS)
	}
	if !c.SMTPTLS.Valid() {
		return fmt.Errorf("account: invalid SMTP TLS mode %q", c.SMTPTLS)
	}
	return nil
}

// IMAPAddr returns the host:port dial string for the IMAP server.
func (c *Config) IMAPAddr() string {
	return fmt.Sprintf("%s:%d", c.IMAPHost, c.IMAPPort)
}

// SMTPAddr returns the host:port dial string for the SMTP server.
func (c *Config) SMTPAddr() string {
	return fmt.Sprintf("%s:%d", c.SMTPHost, c.SMTPPort)
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }
