package widgets

import (
	"image"
	"image/color"
	"strings"
	"unicode"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// Avatar draws the deterministic letter avatar: a circle whose color is
// derived from the sender initial (never random), with the initial
// centered in white. No animation — it renders immediately.
func Avatar(gtx layout.Context, th *theme.Theme, displayName, email string) layout.Dimensions {
	size := gtx.Dp(theme.AvatarSize)
	initial := avatarInitial(displayName, email)
	bg := theme.AvatarColors[avatarColorIndex(initial)]

	defer clip.Ellipse{Max: image.Pt(size, size)}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: bg}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	// Center the initial optically inside the circle.
	inner := gtx
	inner.Constraints = layout.Exact(image.Pt(size, size))
	layout.Center.Layout(inner, func(gtx layout.Context) layout.Dimensions {
		white := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
		return th.LabelAligned(gtx, theme.BodyStrong, white,
			string(initial), text.Middle)
	})
	return layout.Dimensions{Size: image.Pt(size, size)}
}

// avatarInitial picks the display initial: first letter of the display
// name, or the first character of the email local-part.
func avatarInitial(displayName, email string) rune {
	src := strings.TrimSpace(displayName)
	if src == "" {
		src = email
	}
	if src == "" {
		return '?'
	}
	r, _ := utf8.DecodeRuneInString(src)
	return unicode.ToUpper(r)
}

// avatarColorIndex maps an initial onto one of the eight avatar colors,
// hue-spread and stable across runs.
func avatarColorIndex(initial rune) int {
	return int(uint32(initial) % uint32(len(theme.AvatarColors)))
}
