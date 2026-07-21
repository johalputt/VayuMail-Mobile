package widgets

import (
	"image"
	"strings"
	"unicode"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/johalputt/VayuMail-Mobile/internal/avatarimg"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// avatarStore, when set, supplies real mailbox pictures (uploaded photos or
// prebuilt cartoons) fetched from the server's federated avatar endpoint. When it
// has an image for an address the round frame shows the picture; otherwise the
// deterministic letter avatar is drawn. Set once at app start.
var avatarStore *avatarimg.Cache

// SetAvatarStore wires the shared avatar image cache into the avatar widgets.
func SetAvatarStore(c *avatarimg.Cache) { avatarStore = c }

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

	// Prefer the real mailbox picture (photo or cartoon) when the cache has one;
	// it loads asynchronously and falls back to the letter avatar until then.
	if avatarStore != nil {
		if img, ok := avatarStore.Image(email); ok && img != nil {
			drawAvatarImage(gtx, size, img)
			return layout.Dimensions{Size: image.Pt(size, size)}
		}
	}

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

// drawAvatarImage paints a decoded avatar image scaled to fill a round frame of
// the given diameter (centre-cropped upstream, so the scale never distorts).
func drawAvatarImage(gtx layout.Context, size int, img image.Image) {
	defer clip.Ellipse{Max: image.Pt(size, size)}.Push(gtx.Ops).Pop()
	iop := paint.NewImageOp(img)
	b := iop.Size()
	if b.X <= 0 || b.Y <= 0 {
		return
	}
	sx := float32(size) / float32(b.X)
	sy := float32(size) / float32(b.Y)
	defer op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(sx, sy))).Push(gtx.Ops).Pop()
	iop.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
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
