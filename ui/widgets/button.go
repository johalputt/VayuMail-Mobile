package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// ButtonStyle selects one of the four button voices. Primary is the
// single gradient affordance per screen; everything else stays tonal.
type ButtonStyle int

// The button voices.
const (
	ButtonPrimary ButtonStyle = iota
	ButtonTonal
	ButtonText
	ButtonDanger
)

// Button is a pill button with a spring press-scale: it compresses to 96%
// while held and springs back with a whisper of overshoot, carrying
// velocity across a quick tap so a fast double-press never snaps. The
// spring requests frames only while it is unsettled.
type Button struct {
	Click widget.Clickable

	press PressScale
}

// Clicked reports and consumes a completed click.
func (b *Button) Clicked(gtx layout.Context) bool { return b.Click.Clicked(gtx) }

// Layout draws the button. wide stretches it to the full constraint
// width; disabled mutes it and swallows clicks.
func (b *Button) Layout(gtx layout.Context, th *theme.Theme, style ButtonStyle, label string, wide, disabled bool) layout.Dimensions {
	if disabled {
		gtx = gtx.Disabled()
	}
	return b.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return b.press.Layout(gtx, &b.Click, 0.96, func(gtx layout.Context) layout.Dimensions {
			return b.draw(gtx, th, style, label, wide, disabled)
		})
	})
}

// draw renders the resting button by style.
func (b *Button) draw(gtx layout.Context, th *theme.Theme, style ButtonStyle, label string, wide, disabled bool) layout.Dimensions {
	p := th.Palette
	inset := layout.Inset{
		Top: theme.SM + theme.XS, Bottom: theme.SM + theme.XS,
		Left: theme.LG, Right: theme.LG,
	}
	if style == ButtonText {
		inset = layout.Inset{Top: theme.SM, Bottom: theme.SM, Left: theme.MD, Right: theme.MD}
	}

	content := func(gtx layout.Context) layout.Dimensions {
		fg := p.OnAccent
		switch style {
		case ButtonTonal, ButtonText:
			fg = p.Accent
		case ButtonDanger:
			fg = p.Destructive
		}
		if disabled {
			fg = p.Subtle
		}
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.BodyStrong, fg, label, 1)
		})
	}
	if wide {
		inner := content
		content = func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Center.Layout(gtx, inner)
		}
	}

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			r := gtx.Dp(theme.PillRadius)
			if size.Y < 2*r {
				r = size.Y / 2
			}
			switch {
			case disabled && style != ButtonText:
				defer clip.UniformRRect(image.Rectangle{Max: size}, r).Push(gtx.Ops).Pop()
				return Fill(gtx, p.Surface)
			case style == ButtonPrimary:
				Shadow(gtx, th, size, theme.PillRadius)
				defer clip.UniformRRect(image.Rectangle{Max: size}, r).Push(gtx.Ops).Pop()
				return FillGradient(gtx, p.Accent, p.AccentAlt)
			case style == ButtonTonal:
				defer clip.UniformRRect(image.Rectangle{Max: size}, r).Push(gtx.Ops).Pop()
				return Fill(gtx, p.AccentSubtle)
			case style == ButtonDanger:
				defer clip.UniformRRect(image.Rectangle{Max: size}, r).Push(gtx.Ops).Pop()
				return Fill(gtx, theme.WithAlpha(p.Destructive, 0x1F))
			default:
				return layout.Dimensions{Size: size}
			}
		},
		content)
}
