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
	markShare = 0.64
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

// compose paints the full-bleed background and renders the mark onto
// its center with edge-sharpened bicubic sampling.
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

	// The mark upscales ~2x, so treat the source alpha as a continuous
	// field: sample it bicubically, then re-sharpen the edge with a
	// narrow smoothstep — the standard alpha-map trick that reconstructs
	// vector-crisp outlines instead of bilinear blur. The mark itself is
	// solid white, so only coverage matters.
	field := alphaField(mark)
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			a := field.bicubic((float64(x)+0.5)/scale-0.5, (float64(y)+0.5)/scale-0.5)
			cov := smoothstep(0.42, 0.58, a)
			if cov <= 0 {
				continue
			}
			dst := out.NRGBAAt(offX+x, offY+y)
			blend := func(s uint8, d uint8) uint8 {
				return uint8(float64(s)*cov + float64(d)*(1-cov) + 0.5)
			}
			out.SetNRGBA(offX+x, offY+y, color.NRGBA{
				R: blend(0xFF, dst.R), G: blend(0xFF, dst.G), B: blend(0xFF, dst.B), A: 0xFF,
			})
		}
	}
	return out
}

// alphaGrid is the source alpha as a float field in [0,1].
type alphaGrid struct {
	w, h int
	v    []float64
}

func alphaField(img image.Image) *alphaGrid {
	b := img.Bounds()
	g := &alphaGrid{w: b.Dx(), h: b.Dy(), v: make([]float64, b.Dx()*b.Dy())}
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			_, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			g.v[y*g.w+x] = float64(a) / 0xFFFF
		}
	}
	return g
}

func (g *alphaGrid) at(x, y int) float64 {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= g.w {
		x = g.w - 1
	}
	if y >= g.h {
		y = g.h - 1
	}
	return g.v[y*g.w+x]
}

// bicubic samples the field with Catmull-Rom interpolation.
func (g *alphaGrid) bicubic(fx, fy float64) float64 {
	x0, y0 := int(fx), int(fy)
	tx, ty := fx-float64(x0), fy-float64(y0)
	var col [4]float64
	for j := -1; j <= 2; j++ {
		var row [4]float64
		for i := -1; i <= 2; i++ {
			row[i+1] = g.at(x0+i, y0+j)
		}
		col[j+1] = catmullRom(row, tx)
	}
	v := catmullRom(col, ty)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// catmullRom interpolates four samples at parameter t in [0,1].
func catmullRom(p [4]float64, t float64) float64 {
	return p[1] + 0.5*t*(p[2]-p[0]+t*(2*p[0]-5*p[1]+4*p[2]-p[3]+t*(3*(p[1]-p[2])+p[3]-p[0])))
}

// smoothstep maps a in [lo,hi] onto a smooth 0..1 ramp.
func smoothstep(lo, hi, a float64) float64 {
	t := (a - lo) / (hi - lo)
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}
