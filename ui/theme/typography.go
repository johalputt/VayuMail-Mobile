package theme

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// TextStyle is one step of the type scale.
type TextStyle struct {
	Size   unit.Sp
	Weight font.Weight
}

// The complete type scale. The system font stack is used on every
// platform (SF Pro on iOS, Roboto on Android) — no embedded fonts, no
// remote fonts.
var (
	// Display is for empty-state headings and the welcome hero.
	Display = TextStyle{Size: 28, Weight: font.Light}
	// Hero is the onboarding wordmark size.
	Hero = TextStyle{Size: 34, Weight: font.SemiBold}
	// Heading is for screen titles and folder names.
	Heading = TextStyle{Size: 20, Weight: font.Medium}
	// Title is for thread subjects and dialog titles.
	Title = TextStyle{Size: 17, Weight: font.SemiBold}
	// Body is for message bodies and settings labels.
	Body = TextStyle{Size: 15, Weight: font.Normal}
	// BodyStrong is for sender names in the message list.
	BodyStrong = TextStyle{Size: 15, Weight: font.Medium}
	// Caption is for timestamps and metadata.
	Caption = TextStyle{Size: 12, Weight: font.Normal}
	// Micro is for unread counts and badges.
	Micro = TextStyle{Size: 10, Weight: font.Medium}
	// Numeral is for the PIN pad keys.
	Numeral = TextStyle{Size: 24, Weight: font.Light}
)

// Theme bundles the palette, shaper, and mode flag that every widget
// receives. One Theme lives for the whole app; the palette swaps when the
// system preference changes.
type Theme struct {
	Palette Palette
	Dark    bool
	Shaper  *text.Shaper
}

// New builds the app theme. The shaper uses the platform's native system fonts
// (SF Pro on iOS, Roboto on Android). A bundled colour-emoji font was tried and
// reverted: Gio (this version) does not rasterise its colour glyphs, and the font
// also claimed the ASCII digits (used in keycap emoji), so digits, clock times
// and safety numbers rendered blank. System-only shaping keeps all real text —
// letters, digits, times — correct; emoji fall back to the platform's own glyphs.
func New(dark bool) *Theme {
	p := Light()
	if dark {
		p = Dark()
	}
	return &Theme{
		Palette: p,
		Dark:    dark,
		Shaper:  text.NewShaper(),
	}
}

// SetDark switches palettes in place when the system preference changes.
func (t *Theme) SetDark(dark bool) {
	if t.Dark == dark {
		return
	}
	t.Dark = dark
	if dark {
		t.Palette = Dark()
	} else {
		t.Palette = Light()
	}
}

// ColorOp records a solid color as a paint material for text.
func ColorOp(gtx layout.Context, c color.NRGBA) op.CallOp {
	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	return macro.Stop()
}

// Label draws single- or multi-line text in the given style and color.
// MaxLines > 0 truncates with an ellipsis.
func (t *Theme) Label(gtx layout.Context, style TextStyle, c color.NRGBA, txt string, maxLines int) layout.Dimensions {
	l := widget.Label{MaxLines: maxLines}
	f := font.Font{Weight: style.Weight}
	return l.Layout(gtx, t.Shaper, f, style.Size, txt, ColorOp(gtx, c))
}

// LabelAligned draws text with explicit alignment inside the constraint
// width.
func (t *Theme) LabelAligned(gtx layout.Context, style TextStyle, c color.NRGBA, txt string, align text.Alignment) layout.Dimensions {
	l := widget.Label{MaxLines: 1, Alignment: align}
	f := font.Font{Weight: style.Weight}
	return l.Layout(gtx, t.Shaper, f, style.Size, txt, ColorOp(gtx, c))
}
