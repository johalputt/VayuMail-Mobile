package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// FAB is the floating compose button: a gradient disc that compresses
// on press and springs back on release, anchored bottom-right by the
// caller.
type FAB struct {
	Click widget.Clickable

	press PressScale
}

// Clicked reports and consumes a completed tap.
func (f *FAB) Clicked(gtx layout.Context) bool { return f.Click.Clicked(gtx) }

// Layout draws the FAB with the given icon.
func (f *FAB) Layout(gtx layout.Context, th *theme.Theme, icon Icon) layout.Dimensions {
	return f.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return f.press.Layout(gtx, &f.Click, 0.92, func(gtx layout.Context) layout.Dimensions {
			d := gtx.Dp(theme.FABSize)
			size := image.Pt(d, d)
			Shadow(gtx, th, size, theme.FABSize/2)
			defer clip.UniformRRect(image.Rectangle{Max: size}, d/2).Push(gtx.Ops).Pop()
			gtx.Constraints.Min = size
			FillGradient(gtx, th.Palette.Accent, th.Palette.AccentAlt)
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return DrawIcon(gtx, icon, th.Palette.OnAccent, 24)
			})
			return layout.Dimensions{Size: size}
		})
	})
}
