package test

import (
	"image"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// The thread-open hero (plan Phase 5.1) is a hand-off between three
// owners: the list captures the tapped row's rect, the nav carries it
// exactly once to the frame loop, and the frame loop plays the morph.
// These tests pin the two contracts that must never drift: the row
// geometry math and the once-only arm/consume semantics.

func TestRowTopGeometry(t *testing.T) {
	const rowH = 144 // 72dp at 2x

	t.Run("at top", func(t *testing.T) {
		if got := widgets.RowTop(0, 0, 3, rowH); got != 3*rowH {
			t.Fatalf("row 3 from top = %d, want %d", got, 3*rowH)
		}
	})
	t.Run("scrolled into a row", func(t *testing.T) {
		// First visible row partially scrolled off by 30px.
		if got := widgets.RowTop(2, -30, 4, rowH); got != -30+2*rowH {
			t.Fatalf("row 4 = %d, want %d", got, -30+2*rowH)
		}
	})
	t.Run("first row fully above viewport", func(t *testing.T) {
		// Offset == -rowH means the first visible child starts exactly at
		// the top edge; the previous row would sit at -rowH.
		if got := widgets.RowTop(5, -rowH, 4, rowH); got != -2*rowH {
			t.Fatalf("row above first visible = %d, want %d", got, -2*rowH)
		}
	})
}

func TestNavHeroArmConsumeOnce(t *testing.T) {
	nav := state.NewNav(state.ScreenInbox)
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
	nav := state.NewNav(state.ScreenInbox)
	nav.Push(state.ScreenThread, now)
	nav.ArmHero(image.Rect(0, 0, 100, 100))

	if !nav.Pop(now.Add(time.Second)) {
		t.Fatal("Pop at depth 2 returned false")
	}
	if _, ok := nav.TakeHero(); ok {
		t.Fatal("hero survived a pop — a later push would morph from a stale rect")
	}
}

func TestNavHeroDiesOnReplace(t *testing.T) {
	nav := state.NewNav(state.ScreenInbox)
	nav.ArmHero(image.Rect(0, 0, 100, 100))

	nav.Replace(state.ScreenInbox)
	if _, ok := nav.TakeHero(); ok {
		t.Fatal("hero survived Replace")
	}
}

func TestNavPushWithoutArmSlidesNormally(t *testing.T) {
	now := time.Now()
	nav := state.NewNav(state.ScreenInbox)
	nav.Push(state.ScreenThread, now)
	if _, ok := nav.TakeHero(); ok {
		t.Fatal("un-armed push reported a hero rect")
	}
}
