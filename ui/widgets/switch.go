package widgets

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// Switch is an animated toggle: the thumb glides between rails and the
// track cross-fades to the accent while turning on. It is stateless
// about the value — the caller owns the bool and reports it each frame,
// so external changes (e.g. a setting loaded async) animate too.
type Switch struct {
	Click widget.Clickable

	pos    anim.Bool
	primed bool
}

const switchGlide = 160 * time.Millisecond

// Layout draws the switch reflecting on; it returns true when tapped
// (the caller flips its bool and persists it).
func (s *Switch) Layout(gtx layout.Context, th *theme.Theme, on bool) (layout.Dimensions, bool) {
	toggled := s.Click.Clicked(gtx)
	if !s.primed {
		// First frame: settle at the current value without animating.
		s.pos.Jump(on)
		s.primed = true
	}
	s.pos.Set(on, gtx.Now, switchGlide)
	t, settled := s.pos.Progress(gtx.Now, anim.OutCubic)
	if !settled {
		gtx.Execute(op.InvalidateCmd{})
	}

	p := th.Palette
	dims := s.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w, h := gtx.Dp(46), gtx.Dp(26)
		thumb := gtx.Dp(20)
		pad := (h - thumb) / 2

		track := anim.LerpColor(p.Separator, p.Accent, t)
		defer clip.UniformRRect(image.Rect(0, 0, w, h), h/2).Push(gtx.Ops).Pop()
		gtx.Constraints.Min = image.Pt(w, h)
		Fill(gtx, track)

		x := pad + int(t*float32(w-thumb-2*pad))
		defer op.Offset(image.Pt(x, pad)).Push(gtx.Ops).Pop()
		defer clip.UniformRRect(image.Rect(0, 0, thumb, thumb), thumb/2).Push(gtx.Ops).Pop()
		gtx.Constraints.Min = image.Pt(thumb, thumb)
		Fill(gtx, p.OnAccent)

		return layout.Dimensions{Size: image.Pt(w, h)}
	})
	return dims, toggled
}
