package widgets

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// Fill paints a solid rectangle of the current constraint size.
func Fill(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Min
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

// FillMax paints the maximum constraint area.
func FillMax(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

// Separator draws the 1pt hairline used between rows, inset from the
// left so it aligns with the text block, not the avatar edge.
func Separator(gtx layout.Context, th *theme.Theme, leftInset unit.Dp) layout.Dimensions {
	inset := gtx.Dp(leftInset)
	height := gtx.Dp(1)
	if height < 1 {
		height = 1
	}
	width := gtx.Constraints.Max.X
	rect := clip.Rect{
		Min: image.Pt(inset, 0),
		Max: image.Pt(width, height),
	}
	paint.FillShape(gtx.Ops, th.Palette.Separator, rect.Op())
	return layout.Dimensions{Size: image.Pt(width, height)}
}

// IconButton lays out a tappable icon with a TouchTarget-sized hit area.
func IconButton(gtx layout.Context, th *theme.Theme, click *widget.Clickable, icon Icon, c color.NRGBA) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		target := gtx.Dp(theme.TouchTarget)
		gtx.Constraints = layout.Exact(image.Pt(target, target))
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return DrawIcon(gtx, icon, c, 24)
		})
	})
}

// Card wraps content on a raised, rounded surface with a soft shadow —
// the container for thread messages, settings sections, and dialogs.
func Card(gtx layout.Context, th *theme.Theme, content layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			Shadow(gtx, th, gtx.Constraints.Min, theme.CardRadius)
			defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(theme.CardRadius)).Push(gtx.Ops).Pop()
			return Fill(gtx, th.Palette.SurfaceRaised)
		},
		content)
}
