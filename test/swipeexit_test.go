package test

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Swipe-exit is a small state machine (slide out + slot collapse + hold +
// self-heal) driven by frame time. These tests run it headlessly: a real
// Gio context minus the pointer router, advancing Now by hand and
// asserting on the returned dimensions. The contract matters because rows
// are pooled by list position — a stale exit must never eat another
// message's row.

func swipeCtx(now time.Time) layout.Context {
	return layout.Context{
		Ops:    new(op.Ops),
		Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2},
		Now:    now,
		Constraints: layout.Constraints{
			Max: image.Pt(800, 200),
		},
	}
}

const swipeTestRowH = 120

func swipeTestRow(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, swipeTestRowH)}
}

func TestSwipeExitCollapsesSlotThenRestores(t *testing.T) {
	th := theme.New(false)
	var sw widgets.Swipeable
	base := time.Now()

	// Frame 0: idle row renders full size.
	gtx := swipeCtx(base)
	_, dims := sw.Layout(gtx, th, 7, swipeTestRow)
	if dims.Size.Y != swipeTestRowH {
		t.Fatalf("idle height = %d", dims.Size.Y)
	}

	// Commit an archive exit for message 7.
	sw.BeginExit(7, 1, base)

	// Mid-exit: slot is collapsing but not gone; no action re-fires.
	mid := swipeCtx(base.Add(100 * time.Millisecond))
	result, dims := sw.Layout(mid, th, 7, swipeTestRow)
	if result != widgets.SwipeNone {
		t.Fatal("exit frame reported a swipe result")
	}
	if dims.Size.Y <= 0 || dims.Size.Y >= swipeTestRowH {
		t.Fatalf("mid-exit height = %d, want strictly between 0 and %d",
			dims.Size.Y, swipeTestRowH)
	}

	// Just past exit+hold with the SAME message still present: the row
	// must restore — a failed action must not stay invisible forever.
	done := swipeCtx(base.Add(300*time.Millisecond + 3*time.Second))
	_, dims = sw.Layout(done, th, 7, swipeTestRow)
	if dims.Size.Y != swipeTestRowH {
		t.Fatalf("restored height = %d, want %d", dims.Size.Y, swipeTestRowH)
	}
}

func TestSwipeExitParksZeroWhileWaitingForRemoval(t *testing.T) {
	th := theme.New(false)
	var sw widgets.Swipeable
	base := time.Now()

	sw.BeginExit(9, -1, base)

	// Between exit completion and the hold deadline the slot stays at zero
	// height — that is what makes the list collapse look continuous while
	// the snapshot catches up.
	parked := swipeCtx(base.Add(260 * time.Millisecond))
	_, dims := sw.Layout(parked, th, 9, swipeTestRow)
	if dims.Size.Y != 0 {
		t.Fatalf("parked height = %d, want 0", dims.Size.Y)
	}
}

func TestSwipeExitYieldsToNewMessageInSlot(t *testing.T) {
	th := theme.New(false)
	var sw widgets.Swipeable
	base := time.Now()

	sw.BeginExit(11, 1, base)

	// The list removed message 11 and the pool handed this slot to
	// message 12 mid-animation: the exit must reset instantly so 12's row
	// renders full-size instead of inheriting a collapsing slot.
	next := swipeCtx(base.Add(50 * time.Millisecond))
	_, dims := sw.Layout(next, th, 12, swipeTestRow)
	if dims.Size.Y != swipeTestRowH {
		t.Fatalf("new occupant height = %d, want %d", dims.Size.Y, swipeTestRowH)
	}
}
