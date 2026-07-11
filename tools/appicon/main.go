// Command appicon regenerates cmd/vayumail/appicon.png — the launcher
// icon gogio bakes into the APK — from the committed brand artwork.
// Launchers mask icons into rounded squares, so the design must fill
// the full canvas edge to edge: a deep-navy brand field with the white
// V mark large in the middle. Deterministic (same inputs, same PNG
// bytes modulo encoder), stdlib only, run with:
//
//	go run ./tools/appicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

const (
	srcPath = "ui/widgets/brand-dark.png"
	dstPath = "cmd/vayumail/appicon.png"
	// canvas is the icon size gogio expects.
	canvas = 512
	// markShare is how much of the canvas height the V mark occupies —
	// large enough to read at launcher size, small enough to survive
	// every launcher mask shape.
	markShare = 0.58
)

// background is the brand deep-navy (theme Dark().Background).
var background = color.NRGBA{R: 0x0A, G: 0x0E, B: 0x17, A: 0xFF}

func main() {
	f, err := os.Open(srcPath)
	if err != nil {
		log.Fatalf("open %s: %v", srcPath, err)
	}
	src, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		log.Fatalf("decode %s: %v", srcPath, err)
	}

	mark := cropMark(src)
	out := compose(mark)

	dst, err := os.Create(dstPath)
	if err != nil {
		log.Fatalf("create %s: %v", dstPath, err)
	}
	defer dst.Close()
	if err := png.Encode(dst, out); err != nil {
		log.Fatalf("encode: %v", err)
	}
	log.Printf("wrote %s (%dx%d, mark %v)", dstPath, canvas, canvas, mark.Bounds())
}

// cropMark isolates the V mark: the artwork stacks mark over wordmark,
// so the alpha bounding box of the top 55%% of rows is the mark alone.
func cropMark(src image.Image) image.Image {
	b := src.Bounds()
	top := image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+b.Dy()*55/100)
	bbox := image.Rectangle{Min: top.Max, Max: top.Min} // inverted; grown below
	for y := top.Min.Y; y < top.Max.Y; y++ {
		for x := top.Min.X; x < top.Max.X; x++ {
			if _, _, _, a := src.At(x, y).RGBA(); a > 0x2000 {
				if x < bbox.Min.X {
					bbox.Min.X = x
				}
				if y < bbox.Min.Y {
					bbox.Min.Y = y
				}
				if x+1 > bbox.Max.X {
					bbox.Max.X = x + 1
				}
				if y+1 > bbox.Max.Y {
					bbox.Max.Y = y + 1
				}
			}
		}
	}
	if bbox.Empty() {
		log.Fatal("no mark pixels found in the top band of the artwork")
	}
	return cropped{src: src, rect: bbox}
}

// cropped is a zero-copy sub-image view.
type cropped struct {
	src  image.Image
	rect image.Rectangle
}

func (c cropped) ColorModel() color.Model { return c.src.ColorModel() }
func (c cropped) Bounds() image.Rectangle {
	return image.Rect(0, 0, c.rect.Dx(), c.rect.Dy())
}
func (c cropped) At(x, y int) color.Color {
	return c.src.At(c.rect.Min.X+x, c.rect.Min.Y+y)
}

// compose paints the full-bleed background and bilinearly scales the
// mark onto its center.
func compose(mark image.Image) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	for i := 0; i < len(out.Pix); i += 4 {
		out.Pix[i+0] = background.R
		out.Pix[i+1] = background.G
		out.Pix[i+2] = background.B
		out.Pix[i+3] = 0xFF
	}

	mb := mark.Bounds()
	side := float64(canvas) // non-constant so the product may round
	targetH := int(side*markShare + 0.5)
	scale := float64(targetH) / float64(mb.Dy())
	targetW := int(float64(mb.Dx()) * scale)
	offX := (canvas - targetW) / 2
	offY := (canvas - targetH) / 2

	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			r, g, b, a := bilinear(mark, float64(x)/scale, float64(y)/scale)
			if a == 0 {
				continue
			}
			// Source-over onto the opaque background.
			dst := out.NRGBAAt(offX+x, offY+y)
			af := float64(a) / 255
			blend := func(s uint8, d uint8) uint8 {
				return uint8(float64(s)*af + float64(d)*(1-af) + 0.5)
			}
			out.SetNRGBA(offX+x, offY+y, color.NRGBA{
				R: blend(r, dst.R), G: blend(g, dst.G), B: blend(b, dst.B), A: 0xFF,
			})
		}
	}
	return out
}

// bilinear samples the mark with bilinear filtering, returning
// straight-alpha 8-bit components.
func bilinear(img image.Image, fx, fy float64) (r, g, b, a uint8) {
	b_ := img.Bounds()
	x0, y0 := int(fx), int(fy)
	x1, y1 := x0+1, y0+1
	if x1 >= b_.Dx() {
		x1 = b_.Dx() - 1
	}
	if y1 >= b_.Dy() {
		y1 = b_.Dy() - 1
	}
	wx, wy := fx-float64(x0), fy-float64(y0)

	sample := func(x, y int) (float64, float64, float64, float64) {
		pr, pg, pb, pa := img.At(x, y).RGBA()
		return float64(pr >> 8), float64(pg >> 8), float64(pb >> 8), float64(pa >> 8)
	}
	r00, g00, b00, a00 := sample(x0, y0)
	r10, g10, b10, a10 := sample(x1, y0)
	r01, g01, b01, a01 := sample(x0, y1)
	r11, g11, b11, a11 := sample(x1, y1)

	lerp2 := func(v00, v10, v01, v11 float64) float64 {
		top := v00*(1-wx) + v10*wx
		bot := v01*(1-wx) + v11*wx
		return top*(1-wy) + bot*wy
	}
	return uint8(lerp2(r00, r10, r01, r11) + 0.5),
		uint8(lerp2(g00, g10, g01, g11) + 0.5),
		uint8(lerp2(b00, b10, b01, b11) + 0.5),
		uint8(lerp2(a00, a10, a01, a11) + 0.5)
}
