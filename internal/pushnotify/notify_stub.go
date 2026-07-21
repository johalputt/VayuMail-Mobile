//go:build !android

package pushnotify

import "gioui.org/io/event"

// Non-Android platforms have no tappable-notification backend, so the seam
// reports unavailable and callers use their normal notifier. This file carries no
// cgo, so the host and CGO_ENABLED=0 builds compile it cleanly.

func available() bool { return false }

func post(_ int, _, _ string, _, _ int64) bool { return false }

func setTapHandler(func(int64, int64)) {}

func handleViewEvent(event.Event) {}
