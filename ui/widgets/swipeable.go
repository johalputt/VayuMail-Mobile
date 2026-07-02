package widgets

import (
	"image"
	"time"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

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
)

// swipeThreshold is the fraction of row width that commits the action.
const swipeThreshold = 0.4

// snapBackDuration animates an uncommitted swipe back to rest.
const snapBackDuration = 120 * time.Millisecond

// Swipeable wraps a row with horizontal swipe gestures: right reveals
// archive (AccentSubtle background), left reveals delete (red tint). The
// reveal follows the finger directly and snaps at the threshold.
type Swipeable struct {
	drag   gesture.Drag
	pressX float32
	offset float32
	active bool
	// snap-back animation
	snapFrom  float32
	snapStart time.Time
	snapping  bool
}

// Layout draws the row with its current swipe offset and reports whether
// a threshold was crossed on release this frame.
func (s *Swipeable) Layout(gtx layout.Context, th *theme.Theme, row layout.Widget) (SwipeResult, layout.Dimensions) {
	width := float32(gtx.Constraints.Max.X)
	result := SwipeNone

	for {
		ev, ok := s.drag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch ev.Kind {
		case pointer.Press:
			s.pressX = ev.Position.X
			s.active = true
			s.snapping = false
		case pointer.Drag:
			if s.active {
				s.offset = ev.Position.X - s.pressX
			}
		case pointer.Release:
			if s.active {
				switch {
				case s.offset > width*swipeThreshold:
					result = SwipeArchive
					s.offset = 0
				case s.offset < -width*swipeThreshold:
					result = SwipeDelete
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

	// Measure the row first so the reveal background matches its height.
	macro := op.Record(gtx.Ops)
	dims := row(gtx)
	rowCall := macro.Stop()

	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()

	if s.offset != 0 {
		s.drawReveal(gtx, th, dims.Size)
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
// row.
func (s *Swipeable) drawReveal(gtx layout.Context, th *theme.Theme, size image.Point) {
	bg := th.Palette.AccentSubtle
	icon := IconArchive
	iconColor := th.Palette.Accent
	if s.offset < 0 {
		bg = theme.DeleteReveal(th.Dark)
		icon = IconTrash
		iconColor = th.Palette.Destructive
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
