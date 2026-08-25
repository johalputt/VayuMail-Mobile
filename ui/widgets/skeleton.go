package widgets

// skeleton.go — loading placeholders for the folder list (plan Phase
// 5.5). When a sync is running and the folder shows nothing yet (first
// launch, fresh account), pulsing row-shaped bars stand in where messages
// will land, so the screen reads "arriving" instead of "empty". The pulse
// is a per-row phase-shifted sine on alpha: no extra invalidation source
// beyond the one the sync spinner already drives.

import (
	"image"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// SkeletonRows draws n message-row-shaped placeholder bars with a soft
// phase-shifted pulse. Draw only while a load/sync is actually in flight —
// it must never mask real content.
func SkeletonRows(gtx layout.Context, th *theme.Theme, n int) layout.Dimensions {
	list := layout.List{Axis: layout.Vertical}
	return list.Layout(gtx, n, func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{
			Left: theme.MD, Right: theme.MD,
			Top: theme.SM, Bottom: theme.SM,
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(theme.RowHeight) - gtx.Dp(theme.SM)
			w := gtx.Constraints.Max.X

			// Phase shifts down the list; alpha breathes 0.25..0.55.
			phase := float64(i) * 0.9
			a := 0.40 + 0.15*math.Sin(float64(gtx.Now.UnixMilli())/280.0+phase)
			col := theme.WithAlpha(th.Palette.Subtle, uint8(255*a))

			paint.FillShape(gtx.Ops, col, clip.UniformRRect(
				image.Rect(0, 0, w, h), gtx.Dp(theme.PillRadius)).Op(gtx.Ops))
			gtx.Execute(op.InvalidateCmd{})
			return layout.Dimensions{Size: image.Pt(w, h)}
		})
	})
}
