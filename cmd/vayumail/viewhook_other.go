//go:build !android

package main

import (
	"gioui.org/io/event"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"gioui.org/x/haptic"
)

// feedView is a no-op off Android: the buzzer itself is already inert
// there, and no view event ever arrives.
func feedView(_ *haptic.Buzzer, _ event.Event) {}

// refreshSystemMotion is a no-op off Android: only Android has a system
// animator scale to honor; the in-app toggle governs everywhere else.
func refreshSystemMotion() { anim.SetSystemMotionAllowed(true) }
