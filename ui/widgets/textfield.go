package widgets

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// TextField is the app's input row: a Caption label above a filled,
// rounded editor with a hint, and an optional password reveal toggle.
type TextField struct {
	Editor widget.Editor

	// Password masks input and shows the reveal eye.
	Password bool
	revealed bool
	eye      widget.Clickable
}

// NewTextField returns a single-line field.
func NewTextField(password bool) *TextField {
	f := &TextField{Password: password}
	f.Editor.SingleLine = true
	if password {
		f.Editor.Mask = '•'
	}
	return f
}

// Text returns the trimmed editor contents.
func (f *TextField) Text() string { return f.Editor.Text() }

// SetText replaces the editor contents.
func (f *TextField) SetText(s string) { f.Editor.SetText(s) }

// Layout draws the labeled field.
func (f *TextField) Layout(gtx layout.Context, th *theme.Theme, label, hint string) layout.Dimensions {
	p := th.Palette
	if f.Password && f.eye.Clicked(gtx) {
		f.revealed = !f.revealed
		if f.revealed {
			f.Editor.Mask = 0
		} else {
			f.Editor.Mask = '•'
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if label == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: theme.XS, Left: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Caption, p.Subtle, label, 1)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(theme.CornerRadius+4)).Push(gtx.Ops).Pop()
					return Fill(gtx, p.Surface)
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: theme.MD, Right: theme.MD, Top: theme.SM + theme.XS, Bottom: theme.SM + theme.XS}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									if f.Editor.Len() == 0 {
										th.Label(gtx, theme.Body, p.Subtle, hint, 1)
									}
									return f.Editor.Layout(gtx, th.Shaper,
										font.Font{Weight: theme.Body.Weight}, theme.Body.Size,
										theme.ColorOp(gtx, p.OnBackground),
										theme.ColorOp(gtx, p.AccentSubtle))
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if !f.Password {
										return layout.Dimensions{}
									}
									icon := IconEye
									if f.revealed {
										icon = IconEyeOff
									}
									return f.eye.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Left: theme.SM}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												return DrawIcon(gtx, icon, p.Subtle, 20)
											})
									})
								}))
						})
				})
		}))
}
