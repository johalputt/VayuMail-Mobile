//go:build !android

package main

import (
	"gioui.org/io/event"

	"gioui.org/x/haptic"
)

// feedView is a no-op off Android: the buzzer itself is already inert
// there, and no view event ever arrives.
func feedView(_ *haptic.Buzzer, _ event.Event) {}
