// Package theme is the single source of truth for VayuMail's design
// system: palette, type scale, and the 4pt spacing grid. Widgets never
// hard-code a color, size, or inset — everything comes from here.
package theme

import "image/color"

// Palette is the complete color system. The identity is "wind at night":
// deep blue-black surfaces with one electric indigo→cyan accent sweep.
// Color appears only where it carries meaning — unread state, primary
// actions, security status; everything else stays tonal so the accent
// reads as a signal, never as decoration.
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

	// SurfaceRaised is one elevation step above Surface: cards, dialogs,
	// the drawer panel, and the floating snackbar.
	SurfaceRaised color.NRGBA
	// AccentAlt is the second stop of the accent gradient (indigo→cyan).
	// Gradients are reserved for the few primary affordances: the send
	// action, the FAB, and the connect button.
	AccentAlt color.NRGBA
	// OnAccent is the text/icon color over accent fills.
	OnAccent color.NRGBA
	// Success marks verified/encrypted states.
	Success color.NRGBA
	// Warning marks degraded states (offline, retrying).
	Warning color.NRGBA
	// Shadow is the elevation tint layered under raised surfaces.
	Shadow color.NRGBA
}

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
		A: 0xFF,
	}
}

// Light is the light-mode palette: paper white with the same electric
// accent, tuned darker for contrast on white.
func Light() Palette {
	return Palette{
		Background:    rgb(0xFAFBFF),
		Surface:       rgb(0xF0F3FA),
		SurfaceRaised: rgb(0xFFFFFF),
		OnBackground:  rgb(0x0E1220),
		OnSurface:     rgb(0x3C455C),
		Subtle:        rgb(0x8B94AC),
		Accent:        rgb(0x4F5DEE),
		AccentAlt:     rgb(0x0EA8CC),
		OnAccent:      rgb(0xFFFFFF),
		AccentSubtle:  rgb(0xE9ECFF),
		Destructive:   rgb(0xDC2643),
		Success:       rgb(0x0FA57B),
		Warning:       rgb(0xB56A00),
		Separator:     rgb(0xE5E9F4),
		Unread:        rgb(0x4F5DEE),
		Shadow:        color.NRGBA{R: 0x10, G: 0x16, B: 0x2E, A: 0x24},
	}
}

// Dark is the dark-mode palette, auto-selected from the system
// preference. Blacks carry a trace of blue so raised surfaces separate
// without borders; OLED still idles near-black.
func Dark() Palette {
	return Palette{
		Background:    rgb(0x0A0E17),
		Surface:       rgb(0x111726),
		SurfaceRaised: rgb(0x1A2236),
		OnBackground:  rgb(0xF2F5FF),
		OnSurface:     rgb(0xB8C2DC),
		Subtle:        rgb(0x5D6A8A),
		Accent:        rgb(0x6D7CFF),
		AccentAlt:     rgb(0x38D9F5),
		OnAccent:      rgb(0xFFFFFF),
		AccentSubtle:  rgb(0x1D2547),
		Destructive:   rgb(0xFF5D6E),
		Success:       rgb(0x2EE6A8),
		Warning:       rgb(0xFFB454),
		Separator:     rgb(0x202A42),
		Unread:        rgb(0x6D7CFF),
		Shadow:        color.NRGBA{R: 0x00, G: 0x02, B: 0x08, A: 0x66},
	}
}

// DeleteReveal is the background revealed behind a left swipe (delete).
func DeleteReveal(dark bool) color.NRGBA {
	if dark {
		return rgb(0x3A1620)
	}
	return rgb(0xFDE3E7)
}

// ArchiveReveal is the background revealed behind a right swipe.
func ArchiveReveal(dark bool) color.NRGBA {
	if dark {
		return rgb(0x122B33)
	}
	return rgb(0xDFF6FB)
}

// AvatarDuo is a two-stop gradient pair for sender avatars.
type AvatarDuo struct {
	From, To color.NRGBA
}

// AvatarDuos are the eight avatar gradients, chosen deterministically
// from the sender initial — never randomly. Each pair is muted enough
// that the white initial stays legible in both themes.
var AvatarDuos = [8]AvatarDuo{
	{rgb(0x7C6FE0), rgb(0x4F5DEE)}, // iris
	{rgb(0x2FA8C9), rgb(0x2E7BD6)}, // sea
	{rgb(0x35B98B), rgb(0x1F8FA8)}, // jade
	{rgb(0xD96FA8), rgb(0x9A5FD0)}, // orchid
	{rgb(0x8FAF3E), rgb(0x3F9E63)}, // moss
	{rgb(0xA478E8), rgb(0x6D7CFF)}, // violet
	{rgb(0xD9A03E), rgb(0xC96F45)}, // amber
	{rgb(0x45B8B0), rgb(0x3E8FD9)}, // teal
}

// WithAlpha returns c with its alpha scaled to a.
func WithAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
