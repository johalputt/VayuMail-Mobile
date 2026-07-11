// Command appicon regenerates cmd/vayumail/appicon.png — the launcher
// icon gogio bakes into the APK — from the committed brand artwork:
// the V mark inset on a light tile at the proportion neighboring
// launcher glyphs use. The tile background fills the canvas so the
// launcher mask never exposes a ring. Deterministic (same inputs, same
// PNG bytes modulo encoder), stdlib only, run with:
//
//	go run ./tools/appicon
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

const (
	// markPath is the preferred source: the standalone V mark exactly as
	// designed (background removed), used whole with no cropping or
	// reconstruction. Drop the real file here and rerun.
	markPath = "assets/logo/mark.png"
	// srcPath is the fallback when no standalone mark is committed: the
	// mark is cropped out of the full logo artwork.
	srcPath = "ui/widgets/brand-light.png"
	dstPath = "cmd/vayumail/appicon.png"
	// canvas is the icon size gogio expects.
	canvas = 512
	// markShare is how much of the canvas height the V mark occupies.
	// Sized like neighboring launcher glyphs: the tile is filled by the
	// background, the mark sits comfortably inset (~half the tile) the
	// way Drive/GitHub/Play Console glyphs do — present, not shouting.
	markShare = 0.52
)

// background fills the whole tile; the launcher mask then shapes it.
// White matches the light-tile convention of the surrounding icons; the
// mark is the black (light-mode) artwork.
var background = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

// markColor is the fill for mark pixels (the light-mode artwork is
// already near-black; painting a fixed ink keeps output deterministic
// regardless of artwork color profile).
var markColor = color.NRGBA{R: 0x0E, G: 0x12, B: 0x20, A: 0xFF}

func main() {
	mark, err := loadMark()
	if err != nil {
		log.Fatal(err)
	}
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

// loadMark returns the standalone mark file when committed, else the
// mark cropped from the full logo. The standalone file wins because it
// is the designed artwork verbatim — the crop is a reconstruction.
func loadMark() (image.Image, error) {
	if f, err := os.Open(markPath); err == nil {
		defer f.Close()
		img, derr := png.Decode(f)
		if derr != nil {
			return nil, fmt.Errorf("decode %s: %w", markPath, derr)
		}
		log.Printf("using standalone mark %s", markPath)
		return alphaBBox(img), nil
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", srcPath, err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", srcPath, err)
	}
	log.Printf("no %s; cropping mark from %s", markPath, srcPath)
	return cropMark(src), nil
}

// alphaBBox trims transparent margins so the mark scales from its true
// bounds. Marks exported on white instead of transparency also work:
// when the image has no alpha variation, near-white reads as empty.
func alphaBBox(img image.Image) image.Image {
	b := img.Bounds()
	bbox := image.Rectangle{Min: b.Max, Max: b.Min}
	opaque := true
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a <= 0x2000 {
				opaque = false
			}
		}
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			ink := a > 0x2000
			if opaque {
				// No transparency anywhere: treat near-white as background.
				lum := (r + g + bl) / 3
				ink = lum < 0xE000
			}
			if ink {
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
		log.Fatal("standalone mark has no visible pixels")
	}
	if opaque {
		return luminanceMask{src: img, rect: bbox}
	}
	return cropped{src: img, rect: bbox}
}

// luminanceMask adapts a mark exported on a white background: darkness
// becomes coverage, so the compositor's alpha sampling works unchanged.
type luminanceMask struct {
	src  image.Image
	rect image.Rectangle
}

func (m luminanceMask) ColorModel() color.Model { return color.NRGBAModel }
func (m luminanceMask) Bounds() image.Rectangle {
	return image.Rect(0, 0, m.rect.Dx(), m.rect.Dy())
}
func (m luminanceMask) At(x, y int) color.Color {
	r, g, b, _ := m.src.At(m.rect.Min.X+x, m.rect.Min.Y+y).RGBA()
	lum := (r + g + b) / 3
	// Dark ink -> full coverage; white background -> none.
	a := uint16(0)
	if lum < 0xE000 {
		a = uint16(0xFFFF - lum)
	}
	return color.NRGBA64{A: a}
}

// cropMark isolates the V mark from the full logo. The artwork stacks
// the mark above the wordmark with a band of empty rows between them,
// so the split line is found by profiling ink per row and taking the
// widest empty gap — never a fixed percentage, which once amputated
// the mark's descending foot.
func cropMark(src image.Image) image.Image {
	b := src.Bounds()
	inked := make([]bool, b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := src.At(x, y).RGBA(); a > 0x2000 {
				inked[y-b.Min.Y] = true
				break
			}
		}
	}
	// First ink band = the mark: from its first inked row to the row
	// before the widest interior gap.
	first := -1
	for y, ink := range inked {
		if ink {
			first = y
			break
		}
	}
	if first < 0 {
		log.Fatal("artwork has no visible pixels")
	}
	gapStart, gapLen, run, runStart := -1, 0, 0, -1
	lastInk := first
	for y := first; y < len(inked); y++ {
		if inked[y] {
			if run > gapLen && runStart > first {
				gapStart, gapLen = runStart, run
			}
			run, runStart = 0, -1
			lastInk = y
			continue
		}
		if runStart < 0 {
			runStart = y
		}
		run++
	}
	markEnd := lastInk + 1
	if gapLen > 0 {
		markEnd = gapStart
	}
	// Horizontal bounds over the mark's rows only. Built as a literal:
	// image.Rect would canonicalize the deliberately inverted X seed.
	bbox := image.Rectangle{
		Min: image.Pt(b.Max.X, b.Min.Y+first),
		Max: image.Pt(b.Min.X, b.Min.Y+markEnd),
	}
	for y := bbox.Min.Y; y < bbox.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := src.At(x, y).RGBA(); a > 0x2000 {
				if x < bbox.Min.X {
					bbox.Min.X = x
				}
				if x+1 > bbox.Max.X {
					bbox.Max.X = x + 1
				}
			}
		}
	}
	if bbox.Empty() {
		log.Fatal("no mark pixels found in the artwork")
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
			cov := smoothstep(0.35, 0.65, a)
			if cov <= 0 {
				continue
			}
			dst := out.NRGBAAt(offX+x, offY+y)
			blend := func(s uint8, d uint8) uint8 {
				return uint8(float64(s)*cov + float64(d)*(1-cov) + 0.5)
			}
			out.SetNRGBA(offX+x, offY+y, color.NRGBA{
				R: blend(markColor.R, dst.R), G: blend(markColor.G, dst.G),
				B: blend(markColor.B, dst.B), A: 0xFF,
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
