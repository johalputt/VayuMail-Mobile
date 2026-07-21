package avatarimg

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

// TestDecodeSVG rasterises a minimal SVG (a filled rect) to a non-empty image —
// the path the server's cartoon avatars take.
func TestDecodeSVG(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#3366ff"/></svg>`)
	img := decode(svg)
	if img == nil {
		t.Fatal("decode(svg) = nil, want an image")
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		t.Fatalf("rasterised image is empty: %v", b)
	}
}

// TestDecodePNG decodes a raster avatar and centre-crops it square.
func TestDecodePNG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 4)) // non-square on purpose
	for i := range src.Pix {
		src.Pix[i] = 0xAA
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	img := decode(buf.Bytes())
	if img == nil {
		t.Fatal("decode(png) = nil")
	}
	if b := img.Bounds(); b.Dx() != b.Dy() {
		t.Fatalf("expected a square crop, got %v", b)
	}
}

// TestDomainGate confirms the cache never fetches an address on a domain the app
// is not signed into (no goroutine kicked, immediate miss).
func TestDomainGate(t *testing.T) {
	c := New(nil)
	if _, ok := c.Image("someone@stranger.example"); ok {
		t.Fatal("disallowed domain should not resolve to an image")
	}
	c.mu.Lock()
	e := c.entries["someone@stranger.example"]
	c.mu.Unlock()
	if e == nil || e.state != stateMissing {
		t.Fatalf("disallowed address should be negative-cached, got %+v", e)
	}
	// A malformed address is ignored outright.
	if _, ok := c.Image("not-an-email"); ok {
		t.Fatal("malformed address should not resolve")
	}
}
