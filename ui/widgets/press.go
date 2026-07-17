package widgets

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
)

// PressScale is the shared press-feedback animator: a control compresses
// toward pressedScale while held and springs back with a whisper of
// overshoot on release. It carries velocity across a quick tap so a
// double-tap never snaps, and it requests frames only while the spring is
// unsettled — an idle control costs nothing. Button, FAB, the PIN keys and
// every icon button share this one implementation instead of hand-rolling
// the same edge-detection and op.Affine block.
type PressScale struct {
	spring   anim.Spring
	held     bool
	inited   bool
	pressedS float32
}

// scale returns the current scale factor and whether it settled, updating
// the spring from the clickable's pressed edge. pressedScale is the depth
// of the dip (e.g. 0.96); 0 uses a sensible default.
func (ps *PressScale) scale(gtx layout.Context, c *widget.Clickable, pressedScale float32) (float32, bool) {
	if pressedScale == 0 {
		pressedScale = 0.96
	}
	if !ps.inited {
		ps.spring.Jump(1)
		ps.inited = true
	}
	ps.pressedS = pressedScale
	held := c.Pressed()
	if held != ps.held {
		ps.held = held
		target := float32(1)
		cfg := anim.SpringSnappy
		if held {
			target = pressedScale
		}
		ps.spring.Set(target, gtx.Now, cfg)
	}
	v, done := ps.spring.Progress(gtx.Now)
	if !done {
		gtx.Execute(op.InvalidateCmd{})
	}
	return v, done
}

// Layout wraps w with the press-scale transform, scaling about the
// content's centre. c is the clickable whose pressed state drives the dip.
func (ps *PressScale) Layout(gtx layout.Context, c *widget.Clickable, pressedScale float32, w layout.Widget) layout.Dimensions {
	scale, _ := ps.scale(gtx, c, pressedScale)
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()

	origin := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
	defer op.Affine(f32.Affine2D{}.Scale(origin, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
	return dims
}

// ScaleAbout applies a uniform scale about the centre of a widget of the
// given size — the raw transform helper the animated surfaces share.
func ScaleAbout(gtx layout.Context, size image.Point, scale float32, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	origin := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
	defer op.Affine(f32.Affine2D{}.Scale(origin, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
	return dims
}
