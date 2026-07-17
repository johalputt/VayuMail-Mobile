package anim

import (
	"math"
	"time"
)

// Spring is a physically-based 1-D animator: instead of interpolating over
// a fixed duration, it accelerates toward a target and settles under
// damping, carrying velocity across retargets so an interrupted motion
// never jumps. It is the primitive behind the app's press feedback and
// settle animations.
//
// Like the rest of this package it holds no goroutines and allocates
// nothing per frame; the caller samples Value/Progress each frame with
// gtx.Now and requests another frame only while Done reports false. The
// zero value is settled at 0 (Jump/Set give it a real start), matching the
// Anim/Bool contract.
//
// Sampling is SIDE-EFFECT-FREE: the closed-form solution is evaluated from
// the stored anchor, so a widget may sample the same spring several times
// in one frame (e.g. a dialog reading scale and opacity) and always get a
// consistent answer. State only advances when the caller retargets via Set.
type Spring struct {
	// Anchor state, captured at the last Set: position/velocity at start,
	// the target being sought, the natural frequency w (rad/s) and the
	// damping ratio zeta, plus the start time.
	pos0, vel0 float32
	target     float32
	w, zeta    float32
	start      time.Time
	moving     bool
}

// configFreq converts a SpringConfig into (w, zeta). Response is the
// settle time; Bounce maps to under-damping (zeta<1 overshoots).
func configFreq(c SpringConfig) (w, zeta float32) {
	resp := c.Response
	if resp <= 0 {
		resp = SpringSmooth.Response
	}
	w = float32(2*math.Pi) / resp
	zeta = 1 - c.Bounce
	if zeta < 0.05 {
		zeta = 0.05
	}
	return w, zeta
}

// SetConfig sets the spring's feel without retargeting. Safe before the
// first Set.
func (s *Spring) SetConfig(c SpringConfig) {
	w, zeta := configFreq(c)
	s.w, s.zeta = w, zeta
}

// Jump snaps to v with zero velocity, cancelling any motion.
func (s *Spring) Jump(v float32) {
	s.pos0, s.vel0 = v, 0
	s.target = v
	s.moving = false
}

// Set retargets the spring toward v, capturing the current position and
// velocity as the new anchor so an in-flight motion bends smoothly rather
// than restarting. A no-op when already settled at v.
func (s *Spring) Set(v float32, now time.Time, c SpringConfig) {
	if s.w == 0 {
		s.SetConfig(c)
	} else {
		w, zeta := configFreq(c)
		s.w, s.zeta = w, zeta
	}
	if !s.moving && s.pos0 == v {
		s.target = v
		return
	}
	pos, vel := s.sample(now)
	s.pos0, s.vel0 = pos, vel
	s.target = v
	s.start = now
	s.moving = true
}

// Target reports the value being sought.
func (s *Spring) Target() float32 { return s.target }

// Value samples the spring's position at now (side-effect-free).
func (s *Spring) Value(now time.Time) float32 {
	pos, _ := s.sample(now)
	return pos
}

// Progress samples the position and reports whether the spring has settled
// (position and velocity both within epsilon of the target). Mirrors the
// Anim/Bool signature so call sites share one invalidation pattern.
func (s *Spring) Progress(now time.Time) (v float32, done bool) {
	pos, vel := s.sample(now)
	settled := absf(pos-s.target) < settleEps && absf(vel) < velEps
	return pos, settled
}

const (
	settleEps = 0.001
	velEps    = 0.01
)

// sample evaluates the closed-form response at now from the stored anchor.
func (s *Spring) sample(now time.Time) (pos, vel float32) {
	if !s.moving || s.w == 0 {
		return s.pos0, 0
	}
	t := float32(now.Sub(s.start).Seconds())
	if t <= 0 {
		return s.pos0, s.vel0
	}
	x0 := s.pos0 - s.target // displacement from target
	v0 := s.vel0
	w, zeta := s.w, s.zeta

	var d, dv float32
	switch {
	case zeta < 1: // under-damped: settle with a little overshoot
		wd := w * float32(math.Sqrt(float64(1-zeta*zeta)))
		e := float32(math.Exp(float64(-zeta * w * t)))
		c1 := x0
		c2 := (v0 + zeta*w*x0) / wd
		sin := float32(math.Sin(float64(wd * t)))
		cos := float32(math.Cos(float64(wd * t)))
		d = e * (c1*cos + c2*sin)
		dv = e * ((c2*wd-c1*zeta*w)*cos - (c1*wd+c2*zeta*w)*sin)
	default: // critically damped: fastest settle without overshoot
		e := float32(math.Exp(float64(-w * t)))
		a := x0
		b := v0 + w*x0
		d = (a + b*t) * e
		dv = (b - w*(a+b*t)) * e
	}
	return s.target + d, dv
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
