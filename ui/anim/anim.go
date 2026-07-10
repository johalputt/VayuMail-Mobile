package anim

import (
	"image/color"
	"time"
)

// Anim is a one-shot animation: started at a point in time, sampled each
// frame. The zero value is finished at progress 1 (settled), so widgets
// render their resting state without any setup.
type Anim struct {
	start   time.Time
	dur     time.Duration
	running bool
}

// Start begins the animation now for the given duration.
func (a *Anim) Start(now time.Time, dur time.Duration) {
	a.start = now
	a.dur = dur
	a.running = true
}

// Stop settles the animation immediately.
func (a *Anim) Stop() { a.running = false }

// Progress samples the animation. done is true once settled; callers
// request another frame only while done is false.
func (a *Anim) Progress(now time.Time, ease Easing) (t float32, done bool) {
	if !a.running {
		return 1, true
	}
	elapsed := now.Sub(a.start)
	if elapsed >= a.dur {
		a.running = false
		return 1, true
	}
	return ease(float32(elapsed) / float32(a.dur)), false
}

// Bool is an animated boolean: setting the target glides progress toward
// it instead of snapping. Used by switches, reveals, and the drawer. The
// zero value is a settled false.
type Bool struct {
	target  bool
	start   time.Time
	from    float32
	dur     time.Duration
	running bool
}

// Set retargets the value; a no-op when the target is unchanged.
func (b *Bool) Set(v bool, now time.Time, dur time.Duration) {
	if v == b.target {
		return
	}
	b.from = b.value(now)
	b.target = v
	b.start = now
	b.dur = dur
	b.running = true
}

// Jump snaps to the target without animating.
func (b *Bool) Jump(v bool) {
	b.target = v
	b.running = false
}

// Target reports the value being animated toward.
func (b *Bool) Target() bool { return b.target }

// value is the raw linear position in [0,1].
func (b *Bool) value(now time.Time) float32 {
	goal := float32(0)
	if b.target {
		goal = 1
	}
	if !b.running || b.dur <= 0 {
		return goal
	}
	elapsed := now.Sub(b.start)
	if elapsed >= b.dur {
		b.running = false
		return goal
	}
	return Lerp(b.from, goal, float32(elapsed)/float32(b.dur))
}

// Progress samples the eased position in [0,1]; done is true once
// settled.
func (b *Bool) Progress(now time.Time, ease Easing) (t float32, done bool) {
	v := b.value(now)
	return ease(v), !b.running
}

// Stagger computes the progress of item i in a cascaded entrance: each
// item starts step later than the previous and animates for dur. Items
// beyond the cascade window are settled immediately so long lists never
// animate off-screen rows.
func Stagger(now, start time.Time, i int, step, dur time.Duration, ease Easing) (t float32, done bool) {
	begin := start.Add(time.Duration(i) * step)
	elapsed := now.Sub(begin)
	if elapsed <= 0 {
		return 0, false
	}
	if elapsed >= dur {
		return 1, true
	}
	return ease(float32(elapsed) / float32(dur)), false
}

// LerpColor interpolates two colors in straight NRGBA space — adequate
// for the short tint transitions the app uses.
func LerpColor(a, b color.NRGBA, t float32) color.NRGBA {
	t = Clamp01(t)
	lerp8 := func(x, y uint8) uint8 {
		return uint8(float32(x) + (float32(y)-float32(x))*t)
	}
	return color.NRGBA{
		R: lerp8(a.R, b.R),
		G: lerp8(a.G, b.G),
		B: lerp8(a.B, b.B),
		A: lerp8(a.A, b.A),
	}
}
