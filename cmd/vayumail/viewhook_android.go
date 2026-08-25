//go:build android

package main

import (
	"gioui.org/app"
	"gioui.org/io/event"

	"gioui.org/x/haptic"
)

// feedView hands Android view lifecycle events to the buzzer; the view
// reference is what its JNI calls vibrate against.
func feedView(b *haptic.Buzzer, e event.Event) {
	if ve, ok := e.(app.AndroidViewEvent); ok {
		b.SetView(ve.View)
	}
}
