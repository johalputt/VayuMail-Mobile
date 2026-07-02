// Package theme is the single source of truth for VayuMail's design
// system: palette, type scale, and the 4pt spacing grid. Widgets never
// hard-code a color, size, or inset — everything comes from here.
package theme

import "image/color"

// Palette is the complete color system. Accent appears in exactly three
// places — unread dot, active nav item, interactive elements. Everything
// else is greyscale: this app is not colorful, it is precise.
type Palette struct {
	Background   color.NRGBA
	Surface      color.NRGBA
	OnBackground color.NRGBA
	OnSurface    color.NRGBA
	Subtle       color.NRGBA
	Accent       color.NRGBA
	AccentSubtle color.NRGBA
	Destructive  color.NRGBA
	Separator    color.NRGBA
	Unread       color.NRGBA
}

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
		A: 0xFF,
	}
}

// Light is the light-mode palette.
func Light() Palette {
	return Palette{
		Background:   rgb(0xFFFFFF),
		Surface:      rgb(0xF7F7F7),
		OnBackground: rgb(0x0D0D0D),
		OnSurface:    rgb(0x3A3A3A),
		Subtle:       rgb(0x9B9B9B),
		Accent:       rgb(0x2563EB),
		AccentSubtle: rgb(0xEFF6FF),
		Destructive:  rgb(0xDC2626),
		Separator:    rgb(0xEBEBEB),
		Unread:       rgb(0x2563EB),
	}
}

// Dark is the dark-mode palette, auto-selected from the system
// preference.
func Dark() Palette {
	return Palette{
		Background:   rgb(0x0D0D0D),
		Surface:      rgb(0x1A1A1A),
		OnBackground: rgb(0xF5F5F5),
		OnSurface:    rgb(0xBEBEBE),
		Subtle:       rgb(0x5E5E5E),
		Accent:       rgb(0x3B82F6),
		AccentSubtle: rgb(0x1E3A5F),
		Destructive:  rgb(0xEF4444),
		Separator:    rgb(0x2A2A2A),
		Unread:       rgb(0x3B82F6),
	}
}

// DeleteReveal is the background revealed behind a left swipe (delete).
// Light mode uses a fixed red tint; dark mode uses a deep red.
func DeleteReveal(dark bool) color.NRGBA {
	if dark {
		return rgb(0x3F1D1D)
	}
	return rgb(0xFEE2E2)
}

// ScanSuccess is the green flash on successful QR decode.
func ScanSuccess() color.NRGBA { return rgb(0x16A34A) }

// AvatarColors are the eight muted, desaturated avatar backgrounds. The
// color is chosen deterministically from the sender initial — never
// randomly.
var AvatarColors = [8]color.NRGBA{
	rgb(0xE8D5C4), rgb(0xC4D4E8), rgb(0xC4E8D5), rgb(0xE8C4D4),
	rgb(0xD5E8C4), rgb(0xD4C4E8), rgb(0xE8E0C4), rgb(0xC4E8E8),
}

// WithAlpha returns c with its alpha scaled to a.
func WithAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
