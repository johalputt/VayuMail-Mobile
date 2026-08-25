package anim

import "sync/atomic"

// Motion is the single gate every animation primitive reads before it
// moves (plan Phase 5.6). Reduce-motion is an accessibility contract: when
// it is off, nothing in the app should travel — elements snap to their
// final state instead of gliding there. Enforcing the check inside Anim,
// Bool, Spring, and Stagger rather than at each call site means a new
// animated widget cannot forget it.
//
// Two switches AND together:
//   - SetMotionEnabled is the user's in-app toggle (persisted preference,
//     applied once at startup by loadPrefs).
//   - SetSystemMotionAllowed follows the platform's animator scale — on
//     Android a system-wide scale of 0 means "stop animating everything",
//     and the app honors it without being told twice (Phase 5.6 tail).
//
// Defaults to both enabled.
var (
	userEnabled   atomic.Bool
	systemAllowed atomic.Bool
)

func init() {
	userEnabled.Store(true)
	systemAllowed.Store(true)
}

// SetMotionEnabled turns non-essential movement on or off per the user's
// in-app preference. In-flight animations finish as settled on their next
// sample; new ones do not start.
func SetMotionEnabled(on bool) { userEnabled.Store(on) }

// SetSystemMotionAllowed records whether the platform permits motion.
func SetSystemMotionAllowed(on bool) { systemAllowed.Store(on) }

// MotionEnabled reports whether animation should run.
func MotionEnabled() bool { return userEnabled.Load() && systemAllowed.Load() }
