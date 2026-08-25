package test

import (
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
)

// Reduce-motion is an accessibility contract: when the gate is off,
// NOTHING travels. These tests pin that at the primitive level, because
// every animated widget in the app is built from these four — a widget
// added tomorrow inherits the behaviour without anyone remembering it.
func TestReduceMotionSnapsAllPrimitives(t *testing.T) {
	defer anim.SetMotionEnabled(true) // restore for other tests

	anim.SetMotionEnabled(false)
	now := time.Now()

	// One-shot animation: started but already settled at its end state.
	var a anim.Anim
	a.Start(now, time.Second)
	tGot, done := a.Progress(now.Add(50*time.Millisecond), anim.Linear)
	if !done || tGot != 1 {
		t.Fatalf("Anim moved while reduce-motion is on: t=%v done=%v", tGot, done)
	}

	// Animated boolean: snaps to the target instead of gliding.
	var b anim.Bool
	b.Set(true, now, time.Second)
	v, done := b.Progress(now.Add(50*time.Millisecond), anim.Linear)
	if !done || v != 1 {
		t.Fatalf("Bool glided while reduce-motion is on: v=%v done=%v", v, done)
	}

	// Spring: physics IS motion; it must land on target with no travel.
	var s anim.Spring
	s.Jump(0)
	s.Set(1, now, anim.SpringConfig{Response: 0.3})
	got, done := s.Progress(now.Add(50 * time.Millisecond))
	if !done || got != 1 {
		t.Fatalf("Spring travelled while reduce-motion is on: v=%v done=%v", got, done)
	}

	// Staggered entrances: every item arrives immediately.
	tGot, done = anim.Stagger(now, now.Add(time.Second), 5, 50*time.Millisecond, time.Second, anim.Linear)
	if !done || tGot != 1 {
		t.Fatalf("Stagger held items back while reduce-motion is on: t=%v done=%v", tGot, done)
	}
}

// Motion restored means motion actually happens again — the gate must be
// reversible, or one toggle poisons the session forever.
func TestMotionEnabledRestoresAnimation(t *testing.T) {
	defer anim.SetMotionEnabled(true)

	anim.SetMotionEnabled(false)
	var s anim.Spring
	s.Jump(0)

	anim.SetMotionEnabled(true)
	later := time.Now()
	s.Set(1, later, anim.SpringConfig{Response: 0.3})
	mid := later.Add(100 * time.Millisecond)
	v, done := s.Progress(mid)
	if done {
		t.Fatal("spring settled instantly after motion was re-enabled")
	}
	if v == 1 {
		t.Fatal("spring jumped to target after re-enable; expected in-flight travel")
	}
}
