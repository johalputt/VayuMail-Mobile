package widgets

import (
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

const (
	// pullTrigger is the drag distance that commits a refresh.
	pullTrigger = 72
	// pullMax caps the visual travel (rubber-band feel).
	pullMax = 108
	// pullGrab is the tiny downward travel at which the pull claims the
	// gesture. It must be no larger than the list's own scroll slop (3dp)
	// or the enclosing layout.List grabs the pointer first and the pull
	// never sees another drag event.
	pullGrab = 3
)

// PullRefresh turns a downward drag from the top of a list into a
// refresh. The enclosing layout.List grabs any touch-drag after ~3dp to
// scroll, so a passive observer never wins; instead this widget registers
// a pass-through input area on top of the content and, the instant a drag
// begins downward with the list already at its top, claims the pointer
// with a GrabCmd. Its event loop runs before the list lays out, so its
// grab is issued first and the router hands it the gesture — while upward
// drags, horizontal row swipes, and taps fall straight through to the
// list and rows underneath, untouched.
type PullRefresh struct {
	pressY   float32
	pressX   float32
	pid      pointer.ID
	pulling  bool
	pull     float32
	settle   anim.Anim
	settleFr float32
}

// Layout wraps content. atTop reports whether the list is at its very
// top (the caller reads its list.Position); syncing keeps the indicator
// spinning after a committed pull. It returns true on the frame the
// user releases past the threshold.
func (pr *PullRefresh) Layout(gtx layout.Context, th *theme.Theme, atTop, syncing bool, content layout.Widget) (bool, layout.Dimensions) {
	triggered := false
	grabSlop := float32(gtx.Dp(pullGrab))
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: pr,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			pr.pressY, pr.pressX = e.Position.Y, e.Position.X
			pr.pid = e.PointerID
			pr.pulling = false
		case pointer.Drag:
			dy := e.Position.Y - pr.pressY
			dx := e.Position.X - pr.pressX
			if !pr.pulling {
				// Claim the gesture on the first downward, vertical-ish drag
				// while at the top — before the list's 3dp scroll grab. The
				// GrabCmd is issued here, ahead of the list's own grab later
				// in the frame, so the router awards us the pointer. Upward
				// drags and horizontal swipes never qualify and flow through.
				if atTop && dy >= grabSlop && dy > abs32(dx) {
					pr.pulling = true
					gtx.Execute(pointer.GrabCmd{Tag: pr, ID: pr.pid})
				}
			}
			if pr.pulling {
				pull := (dy - grabSlop) * 0.5 // resistance
				if pull < 0 {
					pull = 0
				}
				if max := float32(gtx.Dp(pullMax)); pull > max {
					pull = max
				}
				pr.pull = pull
			}
		case pointer.Release, pointer.Cancel:
			if pr.pulling && e.Kind == pointer.Release &&
				pr.pull >= float32(gtx.Dp(pullTrigger))*0.5 {
				triggered = true
			}
			if pr.pulling {
				pr.settleFr = pr.pull
				pr.settle.Start(gtx.Now, 220*time.Millisecond)
			}
			pr.pulling = false
			pr.pull = 0
		}
	}

	// Settle animation eases the content back after release.
	offset := pr.pull
	if t, done := pr.settle.Progress(gtx.Now, anim.OutCubic); !done {
		offset = pr.settleFr * (1 - t)
		gtx.Execute(op.InvalidateCmd{})
	} else if pr.pulling {
		gtx.Execute(op.InvalidateCmd{})
	}

	// Draw the content (with its own scroll/tap/swipe input) shifted by
	// the pull.
	var dims layout.Dimensions
	func() {
		defer op.Offset(image.Pt(0, int(offset))).Push(gtx.Ops).Pop()
		dims = content(gtx)
	}()

	// Register the pull's input area ON TOP of the content, pass-through,
	// so it receives the same drags the list does without blocking the
	// list's scrolling — and can pre-empt with a grab when a pull begins.
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, pr)
	pass.Pop()
	area.Pop()

	pr.indicator(gtx, th, offset, syncing)
	return triggered, dims
}

// indicator draws the refresh disc riding the pull (and spinning while
// a sync runs).
func (pr *PullRefresh) indicator(gtx layout.Context, th *theme.Theme, offset float32, syncing bool) {
	if offset <= 0 && !syncing {
		return
	}
	p := th.Palette
	d := gtx.Dp(36)
	x := (gtx.Constraints.Max.X - d) / 2
	y := int(offset) - d - gtx.Dp(theme.SM)
	if syncing && offset <= 0 {
		y = gtx.Dp(theme.SM)
	}
	if y < -d {
		return
	}
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()

	Shadow(gtx, th, image.Pt(d, d), theme.PillRadius)
	defer clip.Ellipse{Max: image.Pt(d, d)}.Push(gtx.Ops).Pop()
	fillGtx := gtx
	fillGtx.Constraints.Min = image.Pt(d, d)
	Fill(fillGtx, p.SurfaceRaised)

	// The arrow rotates with travel; past the threshold it flips accent,
	// and while syncing it spins.
	angle := offset / float32(gtx.Dp(pullTrigger)) * 3.14159
	col := p.Subtle
	if offset >= float32(gtx.Dp(pullTrigger))*0.5 {
		col = p.Accent
	}
	if syncing {
		gtx.Execute(op.InvalidateCmd{})
		angle = float32(gtx.Now.UnixMilli()%1200) / 1200 * 2 * 3.14159
		col = p.Accent
	}
	inner := gtx
	inner.Constraints = layout.Exact(image.Pt(d, d))
	layout.Center.Layout(inner, func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := DrawIcon(gtx, IconRefresh, col, 20)
		call := macro.Stop()
		origin := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
		defer op.Affine(f32.Affine2D{}.Rotate(origin, angle)).Push(gtx.Ops).Pop()
		call.Add(gtx.Ops)
		return dims
	})
}

// abs32 is a float32 absolute value.
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
