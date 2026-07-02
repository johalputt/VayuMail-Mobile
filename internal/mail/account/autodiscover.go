package account

import (
	"context"
	"errors"
)

// ErrAutodiscoverUnavailable indicates no autodiscover records were found
// for the domain; the UI falls back to manual entry.
var ErrAutodiscoverUnavailable = errors.New("account: autodiscover unavailable")

// Autodiscover resolves IMAP and SMTP endpoints for an email domain via
// RFC 6186 SRV records (_imaps._tcp, _submission._tcp).
//
// STUB: RFC 6186 SRV lookup is not yet implemented; the function always
// reports ErrAutodiscoverUnavailable and the setup screen falls back to
// manual entry. Tracked in COMPLIANCE-TRACKER.md ("Autodiscover RFC 6186").
// QR provisioning — the primary onboarding path — does not depend on this.
func Autodiscover(ctx context.Context, emailAddress string) (*Config, error) {
	_ = ctx
	_ = emailAddress
	return nil, ErrAutodiscoverUnavailable
}
