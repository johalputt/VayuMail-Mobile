package widgets

import (
	"image"
	"time"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// SwipeResult is the outcome of one frame of a swipeable row.
type SwipeResult int

// Swipe outcomes.
const (
	// SwipeNone: no threshold crossed this frame.
	SwipeNone SwipeResult = iota
	// SwipeArchive: right swipe past 40% width — archive the row.
	SwipeArchive
	// SwipeDelete: left swipe past 40% width — delete the row.
	SwipeDelete
	// SwipeTap: the row was pressed and released without a real drag — a
	// plain tap. The drag gesture sits above the row's Clickable and eats
	// its pointer events, so the Swipeable must surface taps itself.
	SwipeTap
)

// swipeThreshold is the fraction of row width that commits the action.
const swipeThreshold = 0.4

// tapSlop is the max finger travel (px) still treated as a tap, not a drag.
const tapSlop = 12

// snapBackDuration animates an uncommitted swipe back to rest.
const snapBackDuration = 120 * time.Millisecond

// exitDuration animates a COMMITTED swipe fully out of the row's slot
// (plan Phase 5.3): the row keeps travelling in its swipe direction while
// the slot collapses, so the list closes the gap instead of popping.
const exitDuration = 200 * time.Millisecond

// exitHold is how long a finished exit parks the slot at zero height
// waiting for the underlying message to disappear. If the backend did
// NOT remove it (action failed), the row restores itself after this long
// rather than staying invisible forever — the reappearance doubles as
// failure feedback next to the error snackbar.
const exitHold = 2 * time.Second

// Swipeable wraps a row with horizontal swipe gestures: right reveals
// archive (AccentSubtle background), left reveals delete (red tint). The
// reveal follows the finger directly and snaps at the threshold.
type Swipeable struct {
	drag   gesture.Drag
	pressX float32
	offset float32
	active bool
	moved  bool
	// snap-back animation
	snapFrom  float32
	snapStart time.Time
	snapping  bool

	// exit animation, keyed by message ID: rows are pooled by list
	// POSITION, so an in-flight exit must ignore any other message that
	// later occupies the same slot (and yield instantly when one does).
	exitID    int64
	exiting   bool
	exitDone  bool // slid out; parking the slot during exitHold
	exitFrom  float32
	exitDir   float32 // ±1
	exitStart time.Time
}

// BeginExit starts the committed-swipe exit for message id, sliding the
// row the rest of the way in direction dir (±1) while its slot collapses.
func (s *Swipeable) BeginExit(id int64, dir float32, now time.Time) {
	s.exitID = id
	s.exiting = true
	s.exitDone = false
	s.exitFrom = s.offset
	if dir == 0 {
		dir = 1
	}
	s.exitDir = dir
	s.exitStart = now
	s.snapping = false
	s.active = false
}

// Layout draws the row with its current swipe offset and reports whether
// a threshold was crossed on release this frame. id is the message
// currently occupying this slot: an exit animation belonging to another
// id (the slot was reused after list removal) resets instantly.
func (s *Swipeable) Layout(gtx layout.Context, th *theme.Theme, id int64, row layout.Widget) (SwipeResult, layout.Dimensions) {
	width := float32(gtx.Constraints.Max.X)
	result := SwipeNone

	if s.exiting && s.exitID != id {
		// A different message now owns this slot; the exited one was
		// removed. Drop the animation state.
		s.exiting, s.exitDone = false, false
		s.offset = 0
	}

	for {
		ev, ok := s.drag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch ev.Kind {
		case pointer.Press:
			s.pressX = ev.Position.X
			s.active = true
			s.moved = false
			s.snapping = false
		case pointer.Drag:
			if s.active {
				s.offset = ev.Position.X - s.pressX
				if s.offset > tapSlop || s.offset < -tapSlop {
					s.moved = true
				}
			}
		case pointer.Release:
			if s.active {
				switch {
				case s.offset > width*swipeThreshold:
					result = SwipeArchive
					s.BeginExit(id, 1, gtx.Now)
				case s.offset < -width*swipeThreshold:
					result = SwipeDelete
					s.BeginExit(id, -1, gtx.Now)
				case !s.moved:
					// Pressed and released in place: it's a tap, not a swipe.
					result = SwipeTap
					s.offset = 0
				default:
					s.beginSnapBack(gtx.Now)
				}
			}
			s.active = false
		case pointer.Cancel:
			s.active = false
			s.beginSnapBack(gtx.Now)
		}
	}

	// Measure the row first so the reveal background matches its height.
	macro := op.Record(gtx.Ops)
	dims := row(gtx)
	rowCall := macro.Stop()

	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()

	heightFactor := float32(1)
	if s.exiting {
		t := float32(gtx.Now.Sub(s.exitStart)) / float32(exitDuration)
		if t >= 1 {
			t = 1
			s.exitDone = true
		} else {
			gtx.Execute(op.InvalidateCmd{})
		}
		e := anim.OutQuad(t)
		// Keep travelling outward past the edge while the slot collapses.
		target := s.exitDir * width * 1.15
		s.offset = s.exitFrom + (target-s.exitFrom)*e
		heightFactor = 1 - e
		if s.exitDone && gtx.Now.Sub(s.exitStart) > exitDuration+exitHold {
			// The message is still here after the hold: the action did not
			// take. Restore the row — its reappearance is failure feedback
			// next to the error snackbar.
			s.exiting, s.exitDone = false, false
			s.offset = 0
			heightFactor = 1
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	if s.exiting {
		// Exit frames: slide under the collapsing slot, reveal fading with
		// it, and NO input registration — an exiting row is already gone.
		if s.offset != 0 {
			s.drawReveal(gtx, th, dims.Size, heightFactor)
		}
		func() {
			defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
			func() {
				defer op.Offset(image.Pt(int(s.offset), 0)).Push(gtx.Ops).Pop()
				rowCall.Add(gtx.Ops)
			}()
		}()
		return SwipeNone, layout.Dimensions{
			Size: image.Pt(dims.Size.X, int(float32(dims.Size.Y)*heightFactor)),
		}
	}

	if s.snapping {
		t := float32(gtx.Now.Sub(s.snapStart)) / float32(snapBackDuration)
		if t >= 1 {
			s.snapping = false
			s.offset = 0
		} else {
			// Ease-out cubic back to rest.
			s.offset = s.snapFrom * (1 - t) * (1 - t) * (1 - t)
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()

	if s.offset != 0 {
		s.drawReveal(gtx, th, dims.Size, 1)
	}
	func() {
		defer op.Offset(image.Pt(int(s.offset), 0)).Push(gtx.Ops).Pop()
		rowCall.Add(gtx.Ops)
	}()

	s.drag.Add(gtx.Ops)
	return result, dims
}

// beginSnapBack starts the ease back to rest from the current offset.
func (s *Swipeable) beginSnapBack(now time.Time) {
	if s.offset == 0 {
		return
	}
	s.snapFrom = s.offset
	s.snapStart = now
	s.snapping = true
}

// drawReveal paints the action background and icon behind the sliding
// row. fade scales the reveal's alpha (1 = fully visible) so it dies away
// as a committed exit collapses the slot.
func (s *Swipeable) drawReveal(gtx layout.Context, th *theme.Theme, size image.Point, fade float32) {
	bg := theme.ArchiveReveal(th.Dark)
	icon := IconArchive
	iconColor := th.Palette.AccentAlt
	if s.offset < 0 {
		bg = theme.DeleteReveal(th.Dark)
		icon = IconTrash
		iconColor = th.Palette.Destructive
	}
	if fade < 1 {
		bg.A = uint8(float32(bg.A) * fade)
	}
	rect := clip.Rect{Max: size}
	func() {
		defer rect.Push(gtx.Ops).Pop()
		bgGtx := gtx
		bgGtx.Constraints = layout.Exact(size)
		Fill(bgGtx, bg)

		iconGtx := gtx
		iconGtx.Constraints = layout.Exact(size)
		anchor := layout.W
		inset := layout.Inset{Left: theme.LG}
		if s.offset < 0 {
			anchor = layout.E
			inset = layout.Inset{Right: theme.LG}
		}
		anchor.Layout(iconGtx, func(gtx layout.Context) layout.Dimensions {
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return DrawIcon(gtx, icon, iconColor, 24)
			})
		})
	}()
}
