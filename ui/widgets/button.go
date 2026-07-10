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

// Button is a pill button with a press-scale animation: it compresses
// to 97% while held and springs back with a slight overshoot. The
// animation runs only across the press and release frames.
type Button struct {
	Click widget.Clickable

	press   anim.Bool
	wasHeld bool
}

const (
	pressIn  = 70 * time.Millisecond
	pressOut = 240 * time.Millisecond
)

// Clicked reports and consumes a completed click.
func (b *Button) Clicked(gtx layout.Context) bool { return b.Click.Clicked(gtx) }

// Layout draws the button. wide stretches it to the full constraint
// width; disabled mutes it and swallows clicks.
func (b *Button) Layout(gtx layout.Context, th *theme.Theme, style ButtonStyle, label string, wide, disabled bool) layout.Dimensions {
	if disabled {
		gtx = gtx.Disabled()
	}
	held := b.Click.Pressed()
	if held != b.wasHeld {
		b.wasHeld = held
		d := pressOut
		if held {
			d = pressIn
		}
		b.press.Set(held, gtx.Now, d)
	}
	t, settled := b.press.Progress(gtx.Now, anim.OutCubic)
	if !settled {
		gtx.Execute(op.InvalidateCmd{})
	}
	scale := anim.Lerp(1, 0.97, t)

	return b.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := b.draw(gtx, th, style, label, wide, disabled)
		call := macro.Stop()

		origin := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
		defer op.Affine(f32.Affine2D{}.Scale(origin, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
		call.Add(gtx.Ops)
		return dims
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
