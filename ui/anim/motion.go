package anim

import "sync/atomic"

// Motion is the single gate every animation primitive reads before it
// moves (plan Phase 5.6). Reduce-motion is an accessibility contract: when
// it is off, nothing in the app should travel — elements snap to their
// final state instead of gliding there. Enforcing the check inside Anim,
// Bool, Spring, and Stagger rather than at each call site means a new
// animated widget cannot forget it.
//
// Defaults to enabled; loadPrefs calls SetMotionEnabled once at startup
// from the persisted preference.
var motionEnabled atomic.Bool

func init() { motionEnabled.Store(true) }

// SetMotionEnabled turns non-essential movement on or off. In-flight
// animations finish as settled on their next sample; new ones do not start.
func SetMotionEnabled(on bool) { motionEnabled.Store(on) }

// MotionEnabled reports whether animation should run.
func MotionEnabled() bool { return motionEnabled.Load() }
