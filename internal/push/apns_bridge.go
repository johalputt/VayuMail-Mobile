package push

import "errors"

// ErrAPNsUnavailable is returned by every APNs entry point at v0.1.0.
var ErrAPNsUnavailable = errors.New("push: APNs bridge not implemented")

// STUB: iOS APNs support is deferred (Phase 5). iOS at v0.1.0 syncs in
// the foreground only; background delivery requires a server-side APNs
// relay (a VayuPress component that does not exist yet) plus the
// mobile-side token registration below. Tracked in COMPLIANCE-TRACKER.md
// ("iOS APNs", PENDING).

// APNsToken would carry the device token issued by iOS.
type APNsToken struct {
	Token []byte
}

// RegisterAPNsToken will forward the device token to the account's mail
// server so it can wake the app on new mail. Not implemented at v0.1.0.
func RegisterAPNsToken(token APNsToken) error {
	_ = token
	return ErrAPNsUnavailable
}
