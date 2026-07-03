// Command genicon rasterizes the VayuMail mark (assets/logo geometry)
// into cmd/vayumail/appicon.png, the launcher icon gogio embeds into the
// APK/IPA. Pure Go — no external SVG toolchain required, so icon
// generation is reproducible in CI.
//
// Usage: go run ./tools/genicon [-size 512] [-o cmd/vayumail/appicon.png]
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

// Icon geometry mirrors assets/logo/vayumail-icon.svg on a 64x64 canvas:
// a "vy" ligature — a short left arm and a longer right arm curving
// down-left into a y-tail. Do not modify (see assets/logo/README.md).
// Both strokes are expressed as cubic Béziers; the straight left arm
// uses collinear control points.
type cubic struct{ x0, y0, x1, y1, x2, y2, x3, y3 float64 }

var strokes = []cubic{
	{19, 17, 23, 24.667, 27, 32.333, 31, 40}, // left arm: M 19 17 L 31 40
	{45, 17, 43, 31, 37, 44, 27, 51},         // right arm: M 45 17 C 43 31, 37 44, 27 51
}

const (
	strokeWidth = 11.0 // SVG units
	canvasUnits = 64
)

func main() {
	size := flag.Int("size", 512, "output size in pixels (square)")
	out := flag.String("o", "cmd/vayumail/appicon.png", "output PNG path")
	flag.Parse()

	img := render(*size)
	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create %s: %v", *out, err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatalf("encode: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("close: %v", err)
	}
	log.Printf("wrote %s (%dx%d)", *out, *size, *size)
}

// render stamps the stroked mark onto a white canvas, centered. Stamping
// circles along the flattened curve gives round caps and joins for free.
func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	// Launcher icons need an opaque background: pure white, per the
	// light-background logo rules.
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}

	scale := float64(size) / canvasUnits
	ink := color.NRGBA{R: 0x0D, G: 0x0D, B: 0x0D, A: 0xFF}
	strokeR := strokeWidth / 2 * scale

	// Center the mark's bounding box in the canvas for a balanced icon.
	const offX = 32.0 - 32.0 // bbox center x ~32 -> canvas center 32
	const offY = 32.0 - 34.0 // bbox center y ~34 -> canvas center 32

	for _, c := range strokes {
		steps := size * 3 // dense sampling: stamp spacing well under 1px
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			x, y := cubicPoint(c, t)
			stamp(img, (x+offX)*scale, (y+offY)*scale, strokeR, ink)
		}
	}
	return img
}

// cubicPoint evaluates a cubic Bézier at t.
func cubicPoint(c cubic, t float64) (x, y float64) {
	mt := 1 - t
	a := mt * mt * mt
	b := 3 * mt * mt * t
	cc := 3 * mt * t * t
	d := t * t * t
	x = a*c.x0 + b*c.x1 + cc*c.x2 + d*c.x3
	y = a*c.y0 + b*c.y1 + cc*c.y2 + d*c.y3
	return x, y
}

// stamp draws an antialiased filled circle.
func stamp(img *image.NRGBA, cx, cy, r float64, ink color.NRGBA) {
	x0, x1 := int(cx-r)-1, int(cx+r)+1
	y0, y1 := int(cy-r)-1, int(cy+r)+1
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if !image.Pt(x, y).In(img.Rect) {
				continue
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			// 1px antialiasing ramp at the edge.
			cover := r + 0.5 - d
			if cover <= 0 {
				continue
			}
			if cover > 1 {
				cover = 1
			}
			blend(img, x, y, ink, cover)
		}
	}
}

// blend composites ink over the pixel with the given coverage.
func blend(img *image.NRGBA, x, y int, ink color.NRGBA, cover float64) {
	i := img.PixOffset(x, y)
	for ch, v := range [3]uint8{ink.R, ink.G, ink.B} {
		old := float64(img.Pix[i+ch])
		img.Pix[i+ch] = uint8(old + (float64(v)-old)*cover)
	}
}
