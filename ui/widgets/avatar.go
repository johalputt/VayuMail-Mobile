package widgets

import (
	"image"
	"strings"
	"unicode"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// Avatar draws the deterministic letter avatar: a duotone-gradient
// circle whose colors derive from the sender initial (never random),
// with the initial centered in white. No animation — it renders
// immediately.
func Avatar(gtx layout.Context, th *theme.Theme, displayName, email string) layout.Dimensions {
	return AvatarSized(gtx, th, displayName, email, theme.AvatarSize)
}

// AvatarSized draws the avatar at an explicit diameter (the drawer's
// account header uses a larger one).
func AvatarSized(gtx layout.Context, th *theme.Theme, displayName, email string, dp unit.Dp) layout.Dimensions {
	size := gtx.Dp(dp)
	initial := avatarInitial(displayName, email)
	duo := theme.AvatarDuos[avatarColorIndex(initial)]

	defer clip.Ellipse{Max: image.Pt(size, size)}.Push(gtx.Ops).Pop()
	fillGtx := gtx
	fillGtx.Constraints.Min = image.Pt(size, size)
	FillGradient(fillGtx, duo.From, duo.To)

	// Center the initial optically inside the circle.
	inner := gtx
	inner.Constraints = layout.Exact(image.Pt(size, size))
	layout.Center.Layout(inner, func(gtx layout.Context) layout.Dimensions {
		return th.LabelAligned(gtx, theme.BodyStrong, th.Palette.OnAccent,
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

// avatarColorIndex maps an initial onto one of the eight avatar
// gradients, hue-spread and stable across runs.
func avatarColorIndex(initial rune) int {
	return int(uint32(initial) % uint32(len(theme.AvatarDuos)))
}
