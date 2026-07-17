package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// SyncBar is the 2dp progress hairline under the top bar. Determinate while
// the engine reports counted progress, and hidden entirely when idle — it
// renders nothing and requests no frames unless a sync is actually running.
// The fill glides toward each new target with a spring instead of jumping,
// so chunky per-message progress reads as one smooth sweep.
type SyncBar struct {
	fill    anim.Spring
	started bool
}

// Layout draws the bar for done/total progress. total == 0 hides it.
func (s *SyncBar) Layout(gtx layout.Context, th *theme.Theme, done, total int) layout.Dimensions {
	if total <= 0 || done >= total {
		s.started = false
		return layout.Dimensions{}
	}
	target := float32(done) / float32(total)
	if !s.started {
		s.fill.Jump(target)
		s.started = true
	} else {
		s.fill.Set(target, gtx.Now, anim.SpringSmooth)
	}
	frac, done2 := s.fill.Progress(gtx.Now)
	if !done2 {
		gtx.Execute(op.InvalidateCmd{})
	}
	frac = anim.Clamp01(frac)

	h := gtx.Dp(2)
	w := gtx.Constraints.Max.X
	paint.FillShape(gtx.Ops, th.Palette.AccentSubtle,
		clip.Rect{Max: image.Pt(w, h)}.Op())
	fillW := int(float32(w) * frac)
	if fillW > 0 {
		paint.FillShape(gtx.Ops, th.Palette.Accent,
			clip.Rect{Max: image.Pt(fillW, h)}.Op())
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}
