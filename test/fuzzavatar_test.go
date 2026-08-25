package test

import (
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/avatarimg"
)

// FuzzAvatarImage feeds arbitrary bytes to the exact decoder remote avatar
// servers can point us at (plan Phase 7.3). The property contract: never
// panic, and any decoded image is a positive square no larger than a small
// multiple of the raster size — an avatar that decodes into a giant canvas
// would balloon memory per contact. SVG path parsing (oksvg) and the four
// registered raster decoders are both in scope.
func FuzzAvatarImage(f *testing.F) {
	seeds := [][]byte{
		[]byte("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"8\" height=\"8\"><rect width=\"8\" height=\"8\"/></svg>"),
		[]byte("<?xml version=\"1.0\"?><svg viewBox=\"0 0 4 4\"><circle cx=\"2\" cy=\"2\" r=\"2\"/></svg>"),
		{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, // PNG magic
		{0xff, 0xd8, 0xff, 0xe0},       // JPEG magic
		{'G', 'I', 'F', '8', '9', 'a'}, // GIF magic
		{'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'},
		[]byte("not an image at all"),
		nil,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		img := avatarimg.DecodeAvatar(raw)
		if img == nil {
			return
		}
		b := img.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 {
			t.Fatalf("decoded %T with empty bounds %v", img, b)
		}
		if b.Dx() != b.Dy() {
			t.Fatalf("decoded non-square %v", b)
		}
		const maxPx = 4 * 128 // rasterPx ceiling with headroom for crops
		if b.Dx() > maxPx {
			t.Fatalf("decoded oversized avatar: %v", b)
		}
	})
}
