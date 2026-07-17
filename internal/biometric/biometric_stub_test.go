//go:build !android

package biometric

import "testing"

// On the host (non-Android) build the seam must report biometrics
// unavailable and never authenticate, so the UI hides the option and the
// PIN stays the only way in. This also guards the CGO_ENABLED=0 engine
// build: the stub carries no cgo.
func TestStubReportsUnavailable(t *testing.T) {
	if Available() {
		t.Error("host stub must report biometrics unavailable")
	}
	if Authenticate("t", "s") {
		t.Error("host stub must never authenticate")
	}
	// HandleViewEvent is a no-op and must not panic on a nil event.
	HandleViewEvent(nil)
}
