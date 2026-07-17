package anim

import "time"

// Motion tokens — the single place the app's timings and spring feels live,
// so every surface moves with one coherent voice instead of per-widget
// magic numbers. Durations are for the time-eased primitives (Anim/Bool);
// the Spring presets drive the direct-manipulation and settle motions.
const (
	// DurInstant is a barely-there acknowledgement (press-in, ripple start).
	DurInstant = 70 * time.Millisecond
	// DurFast is the workhorse for small state changes (toggles, tints).
	DurFast = 160 * time.Millisecond
	// DurBase is the app-wide default for entrances and reveals.
	DurBase = 240 * time.Millisecond
	// DurSlow is for large surfaces (dialogs, drawers, sheet reveals).
	DurSlow = 320 * time.Millisecond

	// StaggerStep is the delay between consecutive items in a cascade.
	StaggerStep = 22 * time.Millisecond
	// StaggerRows caps how many leading items animate so a long list never
	// animates rows the user will never scroll to.
	StaggerRows = 10
)

// SpringConfig tunes a Spring. Response is the settle time (seconds) — how
// quickly the spring reaches its target — and Bounce in [0,1) adds a touch
// of overshoot (0 = critically damped, no overshoot). These map to the
// familiar "response / damping" feel without exposing raw stiffness.
type SpringConfig struct {
	Response float32
	Bounce   float32
}

// The app's spring vocabulary. Snappy is for press feedback and small
// controls; Smooth is the default for reveals and settles; Gentle is for
// large surfaces that should feel weighty.
var (
	SpringSnappy = SpringConfig{Response: 0.28, Bounce: 0.12}
	SpringSmooth = SpringConfig{Response: 0.42, Bounce: 0.0}
	SpringGentle = SpringConfig{Response: 0.55, Bounce: 0.06}
)
