package widgets

// haptic.go — the app-wide haptic seam (plan Phase 5.4). main constructs
// the platform buzzer (gio-x haptic: Android vibration over JNI, no-op
// elsewhere) and registers it here; every call site just calls Buzz.
// A nil buzzer means haptics are simply absent (tests, desktop) — call
// sites never guard.

import "gioui.org/x/haptic"

var buzzer *haptic.Buzzer

// SetBuzzer registers the platform buzzer. Call once at startup.
func SetBuzzer(b *haptic.Buzzer) { buzzer = b }

// Buzz fires a short feedback tick. Safe from any goroutine; silently
// does nothing when no buzzer is registered or its queue is full.
func Buzz() {
	if buzzer != nil {
		buzzer.Buzz()
	}
}
