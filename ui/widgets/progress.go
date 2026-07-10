package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// SyncBar is the 2dp progress hairline under the top bar. Determinate
// while the engine reports counted progress, and hidden entirely when
// idle — it renders nothing and requests no frames unless a sync is
// actually running.
type SyncBar struct{}

// Layout draws the bar for done/total progress. total == 0 hides it.
func (SyncBar) Layout(gtx layout.Context, th *theme.Theme, done, total int) layout.Dimensions {
	if total <= 0 || done >= total {
		return layout.Dimensions{}
	}
	h := gtx.Dp(2)
	w := gtx.Constraints.Max.X
	paint.FillShape(gtx.Ops, th.Palette.AccentSubtle,
		clip.Rect{Max: image.Pt(w, h)}.Op())
	fill := int(float32(w) * float32(done) / float32(total))
	if fill > 0 {
		paint.FillShape(gtx.Ops, th.Palette.Accent,
			clip.Rect{Max: image.Pt(fill, h)}.Op())
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}
