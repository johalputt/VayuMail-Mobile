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
	brandOnce      sync.Once
	brandLightOp   paint.ImageOp
	brandLightSize image.Point
	brandDarkOp    paint.ImageOp
	brandDarkSize  image.Point
)

// brandImageOp decodes both variants once and returns the cached ImageOp
// for the active theme. Building paint.NewImageOp per frame re-copied the
// bitmap and minted a fresh GPU-texture handle each time — a full texture
// re-upload on every frame the logo was visible. The cached ops upload
// once for the process lifetime.
func brandImageOp(dark bool) (paint.ImageOp, image.Point) {
	brandOnce.Do(func() {
		if img, err := png.Decode(bytes.NewReader(brandLightPNG)); err == nil {
			brandLightOp = paint.NewImageOp(img)
			brandLightOp.Filter = paint.FilterLinear
			brandLightSize = img.Bounds().Size()
		} else {
			slog.Error("decode brand logo (light)", "err", err)
		}
		if img, err := png.Decode(bytes.NewReader(brandDarkPNG)); err == nil {
			brandDarkOp = paint.NewImageOp(img)
			brandDarkOp.Filter = paint.FilterLinear
			brandDarkSize = img.Bounds().Size()
		} else {
			slog.Error("decode brand logo (dark)", "err", err)
		}
	})
	if dark {
		return brandDarkOp, brandDarkSize
	}
	return brandLightOp, brandLightSize
}

// BrandLogo draws the theme-correct VayuMail logo scaled to widthDp,
// preserving aspect ratio. It renders the real artwork — no vector
// reconstruction — and costs one texture upload after first decode.
func BrandLogo(gtx layout.Context, th *theme.Theme, widthDp unit.Dp) layout.Dimensions {
	imgOp, size := brandImageOp(th.Dark)
	if size.X == 0 {
		return layout.Dimensions{}
	}
	w := gtx.Dp(widthDp)
	scale := float32(w) / float32(size.X)
	h := int(float32(size.Y) * scale)

	defer op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
	imgOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(w, h)}
}
