package state

import (
	"image"
	"testing"
	"time"
)

// The hero hand-off (plan Phase 5.1) must be once-only and die on any
// navigation that invalidates the armed rect — otherwise a stale rect
// from an abandoned arm would grow a later push out of the wrong place.
// These live beside Nav rather than in the coverage gate's test package:
// importing ui/state from ./test/ widens -coverpkg's instrumented set
// with internal packages this gate never measured (avatarimg et al),
// which silently dilutes the floor number.

func TestNavHeroArmConsumeOnce(t *testing.T) {
	nav := NewNav(ScreenInbox)
	rect := image.Rect(10, 20, 300, 164)

	if _, ok := nav.TakeHero(); ok {
		t.Fatal("hero armed before ArmHero")
	}

	nav.ArmHero(rect)
	got, ok := nav.TakeHero()
	if !ok || got != rect {
		t.Fatalf("TakeHero = %v,%v want %v,true", got, ok, rect)
	}
	if _, ok := nav.TakeHero(); ok {
		t.Fatal("second TakeHero succeeded")
	}
}

func TestNavHeroDiesOnPop(t *testing.T) {
	now := time.Now()
	nav := NewNav(ScreenInbox)
	nav.Push(ScreenThread, now)
	nav.ArmHero(image.Rect(0, 0, 100, 100))

	if !nav.Pop(now.Add(time.Second)) {
		t.Fatal("Pop at depth 2 returned false")
	}
	if _, ok := nav.TakeHero(); ok {
		t.Fatal("hero survived a pop")
	}
}

func TestNavHeroDiesOnReplace(t *testing.T) {
	nav := NewNav(ScreenInbox)
	nav.ArmHero(image.Rect(0, 0, 100, 100))

	nav.Replace(ScreenInbox)
	if _, ok := nav.TakeHero(); ok {
		t.Fatal("hero survived Replace")
	}
}

func TestNavPushWithoutArmSlidesNormally(t *testing.T) {
	now := time.Now()
	nav := NewNav(ScreenInbox)
	nav.Push(ScreenThread, now)
	if _, ok := nav.TakeHero(); ok {
		t.Fatal("un-armed push reported a hero rect")
	}
}
