//go:build android

package main

import (
	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/x/haptic"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
)

// feedView hands Android view lifecycle events to the buzzer; the view
// reference is what its JNI calls vibrate against.
func feedView(b *haptic.Buzzer, e event.Event) {
	if ve, ok := e.(app.AndroidViewEvent); ok {
		b.SetView(ve.View)
	}
}

// refreshSystemMotion re-reads the system animator scale on every view
// event (resume included), so toggling "remove animations" in Android's
// accessibility settings takes effect on return to the app without a
// restart (plan Phase 5.6 tail). Scale 0 = the OS wants no animation.
func refreshSystemMotion() {
	anim.SetSystemMotionAllowed(anim.SystemAnimatorScale() > 0)
}
