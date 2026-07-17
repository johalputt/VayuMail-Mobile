// Package biometric is the platform seam for fingerprint / face unlock.
// It gates the existing app-lock PIN — a biometric success simply opens the
// same gate a correct PIN would, and the PIN is always available as the
// fallback (ADR-0010: the app lock is an access gate, not at-rest
// encryption; the sealed store's master key never derives from the PIN or a
// biometric).
//
// The engine only ever sees this small interface. The Android implementation
// (build-tagged, cgo + JNI) wraps the framework BiometricPrompt; every other
// platform gets the stub, which reports biometrics unavailable so the UI
// hides the option and falls back to the PIN. Keeping the surface tiny lets
// Rule 1 (CGO_ENABLED=0 engine build) stay green: only the //go:build android
// file uses cgo, and it is excluded from the host build.
package biometric

import "gioui.org/io/event"

// Available reports whether the device can perform biometric authentication
// right now (hardware present, at least one credential enrolled, OS
// supported). The UI only offers the toggle when this is true.
func Available() bool { return available() }

// Authenticate prompts the user for a biometric and reports whether it
// succeeded. It blocks until the OS prompt resolves, so callers MUST invoke
// it from a background goroutine, never the UI/frame thread. A false result
// (cancel, error, or unsupported) means "fall back to the PIN" — it never
// panics and never leaks why it failed.
//
// title and subtitle label the system sheet; the sheet always offers a
// "Use PIN" negative action so the user can decline biometrics.
func Authenticate(title, subtitle string) bool { return authenticate(title, subtitle) }

// HandleViewEvent must be fed every window event so the Android backend can
// capture the current view handle (BiometricPrompt needs the Activity behind
// it). It is a no-op on non-Android platforms. Safe to call from the event
// pump.
func HandleViewEvent(e event.Event) { handleViewEvent(e) }
