//go:build !android

package biometric

import "gioui.org/io/event"

// On every non-Android platform there is no supported biometric backend, so
// the seam reports unavailable and the UI falls back to the PIN. This file
// carries no cgo, so the CGO_ENABLED=0 engine build (Rule 1) compiles it
// cleanly.

func available() bool { return false }

func authenticate(_, _ string) bool { return false }

func handleViewEvent(event.Event) {}
