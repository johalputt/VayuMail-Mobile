package theme

import (
	"testing"

	"gioui.org/io/system"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

// TestEmojiFontEmbeddedAndShapes guards the emoji fallback: the Noto Color Emoji
// font must embed and parse (or emoji render as tofu boxes), and the theme shaper
// must shape a mixed text+emoji string through the real path without panicking and
// produce glyphs. Visual colour rendering is confirmed on-device; this pins the
// wiring so a dependency or shaper-config regression fails CI instead of shipping.
func TestEmojiFontEmbeddedAndShapes(t *testing.T) {
	if len(emojiFaces) == 0 {
		t.Fatal("Noto Color Emoji did not parse — emoji would render as empty boxes")
	}
	th := New(false)
	if th.Shaper == nil {
		t.Fatal("theme shaper is nil")
	}
	th.Shaper.LayoutString(text.Parameters{
		PxPerEm:  fixed.I(16),
		MaxWidth: 1 << 20,
		Locale:   system.Locale{Language: "en", Direction: system.LTR},
	}, "Hi 😀🎉 there")
	glyphs := 0
	for {
		if _, ok := th.Shaper.NextGlyph(); !ok {
			break
		}
		glyphs++
	}
	if glyphs == 0 {
		t.Fatal("shaper produced no glyphs for mixed text/emoji")
	}
}
