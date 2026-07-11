package widgets

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"log/slog"
	"sync"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// The original VayuMail artwork (mark + wordmark on transparency), one
// per theme. Embedded so the welcome screen never depends on external
// assets — the exact files from assets/logo/.
//
//go:embed brand-light.png
var brandLightPNG []byte

//go:embed brand-dark.png
var brandDarkPNG []byte

var (
	brandOnce  sync.Once
	brandLight image.Image
	brandDark  image.Image
)

// brandImage decodes both variants once and returns the one for the
// active theme.
func brandImage(dark bool) image.Image {
	brandOnce.Do(func() {
		if img, err := png.Decode(bytes.NewReader(brandLightPNG)); err == nil {
			brandLight = img
		} else {
			slog.Error("decode brand logo (light)", "err", err)
		}
		if img, err := png.Decode(bytes.NewReader(brandDarkPNG)); err == nil {
			brandDark = img
		} else {
			slog.Error("decode brand logo (dark)", "err", err)
		}
	})
	if dark {
		return brandDark
	}
	return brandLight
}

// BrandLogo draws the theme-correct VayuMail logo scaled to widthDp,
// preserving aspect ratio. It renders the real artwork — no vector
// reconstruction — and costs one texture upload after first decode.
func BrandLogo(gtx layout.Context, th *theme.Theme, widthDp unit.Dp) layout.Dimensions {
	img := brandImage(th.Dark)
	if img == nil {
		return layout.Dimensions{}
	}
	w := gtx.Dp(widthDp)
	src := img.Bounds().Dx()
	if src == 0 {
		return layout.Dimensions{}
	}
	scale := float32(w) / float32(src)
	h := int(float32(img.Bounds().Dy()) * scale)

	defer op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	imgOp := paint.NewImageOp(img)
	imgOp.Filter = paint.FilterLinear
	imgOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(w, h)}
}
