package widgets

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// FillGradient paints the minimum constraint area with a left→right
// two-stop linear gradient, slightly tilted so the sweep reads as
// deliberate rather than mechanical.
func FillGradient(gtx layout.Context, from, to color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Min
	paint.LinearGradientOp{
		Stop1:  f32.Pt(0, float32(size.Y)),
		Stop2:  f32.Pt(float32(size.X), 0),
		Color1: from,
		Color2: to,
	}.Add(gtx.Ops)
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: size}
}

// AccentGradient paints the theme's signature indigo→cyan sweep.
func AccentGradient(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return FillGradient(gtx, th.Palette.Accent, th.Palette.AccentAlt)
}

// TextGradientOp records the accent gradient as a text material spanning
// width pixels — used for the wordmark on the welcome screen.
func TextGradientOp(gtx layout.Context, th *theme.Theme, width int) op.CallOp {
	macro := op.Record(gtx.Ops)
	paint.LinearGradientOp{
		Stop1:  f32.Pt(0, 0),
		Stop2:  f32.Pt(float32(width), 0),
		Color1: th.Palette.Accent,
		Color2: th.Palette.AccentAlt,
	}.Add(gtx.Ops)
	return macro.Stop()
}

// Shadow layers two soft offset rounded rects under a raised surface.
// Gio has no blur primitive; two translucent passes at growing insets
// read as depth at a fraction of the cost.
func Shadow(gtx layout.Context, th *theme.Theme, size image.Point, radius unit.Dp) {
	r := gtx.Dp(radius)
	tint := th.Palette.Shadow
	// Wide, faint pass.
	outer := image.Rect(-gtx.Dp(2), gtx.Dp(1), size.X+gtx.Dp(2), size.Y+gtx.Dp(4))
	paint.FillShape(gtx.Ops, theme.WithAlpha(tint, tint.A/3),
		clip.UniformRRect(outer, r+gtx.Dp(2)).Op(gtx.Ops))
	// Tight, darker pass.
	inner := image.Rect(0, gtx.Dp(1), size.X, size.Y+gtx.Dp(2))
	paint.FillShape(gtx.Ops, tint, clip.UniformRRect(inner, r).Op(gtx.Ops))
}
