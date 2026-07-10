package widgets

import (
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// FAB is the floating compose button: a gradient disc that compresses
// on press and springs back on release, anchored bottom-right by the
// caller.
type FAB struct {
	Click widget.Clickable

	press   anim.Bool
	wasHeld bool
}

// Clicked reports and consumes a completed tap.
func (f *FAB) Clicked(gtx layout.Context) bool { return f.Click.Clicked(gtx) }

// Layout draws the FAB with the given icon.
func (f *FAB) Layout(gtx layout.Context, th *theme.Theme, icon Icon) layout.Dimensions {
	held := f.Click.Pressed()
	if held != f.wasHeld {
		f.wasHeld = held
		d := 240 * time.Millisecond
		if held {
			d = 70 * time.Millisecond
		}
		f.press.Set(held, gtx.Now, d)
	}
	t, settled := f.press.Progress(gtx.Now, anim.OutBack)
	if !settled {
		gtx.Execute(op.InvalidateCmd{})
	}
	scale := anim.Lerp(1, 0.92, t)

	return f.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		d := gtx.Dp(theme.FABSize)
		size := image.Pt(d, d)
		origin := f32.Pt(float32(d)/2, float32(d)/2)
		defer op.Affine(f32.Affine2D{}.Scale(origin, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()

		Shadow(gtx, th, size, theme.FABSize/2)
		defer clip.UniformRRect(image.Rectangle{Max: size}, d/2).Push(gtx.Ops).Pop()
		gtx.Constraints.Min = size
		FillGradient(gtx, th.Palette.Accent, th.Palette.AccentAlt)
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return DrawIcon(gtx, icon, th.Palette.OnAccent, 24)
		})
		return layout.Dimensions{Size: size}
	})
}
