package anim

import (
	"testing"
	"time"
)

// baseTime is a fixed anchor — the package forbids time.Now() in tests via
// the frame-clock discipline, and springs only need a monotonic base.
var baseTime = time.Unix(1_700_000_000, 0)

func TestSpringSettlesAtTarget(t *testing.T) {
	var s Spring
	s.Jump(0)
	s.Set(1, baseTime, SpringSmooth)

	// Sample forward; the spring must converge to the target and report done.
	var last float32
	done := false
	for ms := 0; ms <= 2000; ms += 16 {
		now := baseTime.Add(time.Duration(ms) * time.Millisecond)
		v, d := s.Progress(now)
		last = v
		if d {
			done = true
			break
		}
	}
	if !done {
		t.Fatalf("spring never settled; last=%v", last)
	}
	if last < 0.99 || last > 1.01 {
		t.Fatalf("settled value = %v, want ~1", last)
	}
}

func TestSpringSampleIsSideEffectFree(t *testing.T) {
	var s Spring
	s.Jump(0)
	s.Set(1, baseTime, SpringSmooth)
	now := baseTime.Add(100 * time.Millisecond)
	// Sampling the same instant repeatedly must return the same value — a
	// dialog reads scale and opacity from one spring in a single frame.
	a := s.Value(now)
	b := s.Value(now)
	c, _ := s.Progress(now)
	if a != b || a != c {
		t.Fatalf("non-deterministic sample: %v %v %v", a, b, c)
	}
}

func TestSpringCriticallyDampedNoOvershoot(t *testing.T) {
	var s Spring
	s.Jump(0)
	// Bounce 0 => critically damped => must never exceed the target.
	s.Set(1, baseTime, SpringConfig{Response: 0.3, Bounce: 0})
	for ms := 0; ms <= 1500; ms += 8 {
		now := baseTime.Add(time.Duration(ms) * time.Millisecond)
		v := s.Value(now)
		if v > 1.0001 {
			t.Fatalf("critically damped spring overshot to %v at %dms", v, ms)
		}
	}
}

func TestSpringJumpCancelsMotion(t *testing.T) {
	var s Spring
	s.Set(1, baseTime, SpringSmooth)
	s.Jump(0.5)
	if v, done := s.Progress(baseTime.Add(time.Second)); v != 0.5 || !done {
		t.Fatalf("after Jump: v=%v done=%v, want 0.5/true", v, done)
	}
}
