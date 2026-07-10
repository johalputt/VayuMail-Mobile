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

// Dialog is the app's one modal: a raised card that scales in over a
// scrim, carrying a title, a body, and confirm/cancel actions. Used
// sparingly — only destructive choices (sign out, remove key) earn a
// modal interruption.
type Dialog struct {
	Title   string
	Body    string
	Confirm string
	Danger  bool

	visible    bool
	enter      anim.Anim
	confirmBtn Button
	cancelBtn  Button
	scrimClick widget.Clickable
	onConfirm  func()
}

// Show opens the dialog with the given action to run on confirm.
func (d *Dialog) Show(now time.Time, title, body, confirm string, danger bool, onConfirm func()) {
	d.Title, d.Body, d.Confirm, d.Danger = title, body, confirm, danger
	d.onConfirm = onConfirm
	d.visible = true
	d.enter.Start(now, 200*time.Millisecond)
}

// Visible reports whether the dialog is on screen (the back key closes
// it before popping navigation).
func (d *Dialog) Visible() bool { return d.visible }

// Dismiss closes the dialog without confirming.
func (d *Dialog) Dismiss() { d.visible = false }

// Layout draws the dialog above the current screen when visible.
func (d *Dialog) Layout(gtx layout.Context, th *theme.Theme) {
	if !d.visible {
		return
	}
	p := th.Palette
	if d.scrimClick.Clicked(gtx) {
		d.visible = false
	}
	if d.cancelBtn.Clicked(gtx) {
		d.visible = false
	}
	if d.confirmBtn.Clicked(gtx) {
		d.visible = false
		if d.onConfirm != nil {
			d.onConfirm()
		}
	}

	t, settled := d.enter.Progress(gtx.Now, anim.OutBack)
	fade, _ := d.enter.Progress(gtx.Now, anim.OutQuad)
	if !settled {
		gtx.Execute(op.InvalidateCmd{})
	}

	// Scrim absorbs taps outside the card.
	d.scrimClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = gtx.Constraints.Max
		return Fill(gtx, theme.WithAlpha(p.Shadow, uint8(fade*0x8F)))
	})

	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Dp(320)
		if gtx.Constraints.Max.X-2*gtx.Dp(theme.LG) < maxW {
			maxW = gtx.Constraints.Max.X - 2*gtx.Dp(theme.LG)
		}
		gtx.Constraints.Max.X = maxW
		gtx.Constraints.Min.X = maxW

		macro := op.Record(gtx.Ops)
		dims := d.card(gtx, th)
		call := macro.Stop()

		scale := anim.Lerp(0.92, 1, t)
		origin := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
		defer op.Affine(f32.Affine2D{}.Scale(origin, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
		call.Add(gtx.Ops)
		return dims
	})
}

// card renders the dialog surface and content.
func (d *Dialog) card(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	p := th.Palette
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			Shadow(gtx, th, gtx.Constraints.Min, theme.CardRadius)
			defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(theme.CardRadius)).Push(gtx.Ops).Pop()
			return Fill(gtx, p.SurfaceRaised)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(theme.LG).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Title, p.OnBackground, d.Title, 2)
					}),
					layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Body, p.OnSurface, d.Body, 0)
					}),
					layout.Rigid(layout.Spacer{Height: theme.LG}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return d.cancelBtn.Layout(gtx, th, ButtonText, "Cancel", false, false)
							}),
							layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								style := ButtonTonal
								if d.Danger {
									style = ButtonDanger
								}
								return d.confirmBtn.Layout(gtx, th, style, d.Confirm, false, false)
							}))
					}))
			})
		})
}
