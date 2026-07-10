// Package anim is VayuMail's motion engine: pure time-interpolated
// progress values with zero per-frame allocation and no goroutines.
// Widgets sample an animation each frame and request another frame only
// while it reports itself unfinished — an idle screen therefore renders
// zero frames and costs zero battery (Rule 5: nothing here blocks; Rule 8:
// nothing here polls).
package anim

import "math"

// Easing maps linear time t in [0,1] onto eased progress in [0,1].
type Easing func(t float32) float32

// Linear is the identity easing.
func Linear(t float32) float32 { return t }

// OutQuad decelerates gently — used for small fades.
func OutQuad(t float32) float32 { return 1 - (1-t)*(1-t) }

// InQuad accelerates gently — used for exits.
func InQuad(t float32) float32 { return t * t }

// OutCubic decelerates firmly — the app-wide default for entrances.
func OutCubic(t float32) float32 {
	u := 1 - t
	return 1 - u*u*u
}

// InCubic accelerates firmly — the app-wide default for exits.
func InCubic(t float32) float32 { return t * t * t }

// InOutCubic eases both ends — used for position swaps.
func InOutCubic(t float32) float32 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - u*u*u/2
}

// OutExpo starts near-instant and lands softly — used for the sync bar
// and large surface reveals.
func OutExpo(t float32) float32 {
	if t >= 1 {
		return 1
	}
	return 1 - float32(math.Pow(2, float64(-10*t)))
}

// OutBack overshoots ~10% then settles — the signature curve for
// buttons, the FAB, and dialogs. Subtle enough to read as precision,
// not bounce.
func OutBack(t float32) float32 {
	const c1 = 1.70158
	const c3 = c1 + 1
	u := t - 1
	return 1 + c3*u*u*u + c1*u*u
}

// Shake is a damped sine used for the PIN-error shake: t in [0,1] maps
// to a horizontal multiplier in [-1,1] that decays to zero.
func Shake(t float32) float32 {
	if t <= 0 || t >= 1 {
		return 0
	}
	decay := float64(1 - t)
	return float32(math.Sin(float64(t)*3*2*math.Pi) * decay * decay)
}

// Clamp01 clamps t to [0,1].
func Clamp01(t float32) float32 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// Lerp interpolates a→b by t.
func Lerp(a, b, t float32) float32 { return a + (b-a)*t }
