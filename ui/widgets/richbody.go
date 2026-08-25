package widgets

// richbody.go — the flagged rich-HTML renderer (plan Phase 6). Input is
// mime.RichParagraphs output: inert by construction, so this file's only
// concerns are presentation and interaction. Links are the one interactive
// element; taps surface as LinkTaps for the screen to act on, and the URL
// that reaches there has already passed the sanitizer's https/mailto
// allowlist. Nothing here parses HTML.

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"

	"gioui.org/x/richtext"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// LinkTap is a tapped hyperlink inside a rendered rich body.
type LinkTap struct {
	MessageID int64
	URL       string
}

// RichBody renders sanitized paragraphs with real styling: bold/italic/
// underline/code runs, heading scale, quote indentation, and tappable
// links. State is per message so link gestures survive across frames.
type RichBody struct {
	states map[int64]*richtext.InteractiveText
	taps   []LinkTap
}

// NewRichBody constructs an empty renderer.
func NewRichBody() *RichBody {
	return &RichBody{states: make(map[int64]*richtext.InteractiveText)}
}

// LinkTaps drains the links the user tapped this frame.
func (rb *RichBody) LinkTaps() []LinkTap {
	out := rb.taps
	rb.taps = nil
	return out
}

// Layout renders one message's rich body.
func (rb *RichBody) Layout(gtx layout.Context, th *theme.Theme, msgID int64, paras []mime.RichPara) layout.Dimensions {
	state := rb.states[msgID]
	if state == nil {
		state = &richtext.InteractiveText{}
		rb.states[msgID] = state
	}

	list := layout.List{Axis: layout.Vertical}
	return list.Layout(gtx, len(paras), func(gtx layout.Context, i int) layout.Dimensions {
		p := paras[i]
		inset := layout.Inset{Top: theme.XS, Bottom: theme.XS}
		if p.Depth > 0 {
			// Quote nesting: indent and pull left edge in; a subtle tint
			// separates quoted history from new content.
			inset.Left = unit.Dp(10 * float32(p.Depth))
		}
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			styles := rb.spanStyles(th, p)
			if len(styles) == 0 {
				return layout.Dimensions{}
			}
			dims := richtext.Text(state, th.Shaper, styles...).Layout(gtx)
			// Drain interactions AFTER layout: Update reports pending
			// clicks; every tapped span carries its validated href.
			for {
				span, ev, ok := state.Update(gtx)
				if !ok {
					break
				}
				if ev.Type != richtext.Click || span == nil {
					continue
				}
				if href, ok := span.Get("href").(string); ok && href != "" {
					rb.taps = append(rb.taps, LinkTap{MessageID: msgID, URL: href})
				}
			}
			return dims
		})
	})
}

// spanStyles maps one paragraph's runs onto richtext span styles: type
// scale from the block tag, emphasis from run flags, accent color plus
// interactivity for links, monospace for code, subtle tint for quotes.
func (rb *RichBody) spanStyles(th *theme.Theme, p mime.RichPara) []richtext.SpanStyle {
	size, weight, mono := blockScale(p.Tag)
	color := th.Palette.OnBackground
	if p.Depth > 0 {
		color = th.Palette.Subtle
	}
	out := make([]richtext.SpanStyle, 0, len(p.Runs))
	for _, r := range p.Runs {
		s := richtext.SpanStyle{
			Content: r.Text,
			Size:    size,
			Color:   color,
		}
		s.Font = font.Font{Weight: weight}
		if mono || r.Code {
			s.Font.Typeface = "monospace"
			s.Size -= 1
		}
		if r.Bold {
			s.Font.Weight = font.Bold
		}
		if r.Italic {
			s.Font.Style = font.Italic
		}
		if r.Underline && r.Link == "" {
			s.Color = th.Palette.Accent
		}
		if r.Link != "" {
			s.Interactive = true
			s.Color = th.Palette.Accent
			s.Set("href", r.Link)
		}
		out = append(out, s)
	}
	return out
}

// blockScale picks type metrics per block tag. Headings get size and
// weight from the app's own scale; everything else reads as body text.
func blockScale(tag string) (size unit.Sp, weight font.Weight, mono bool) {
	size, weight = theme.Body.Size, theme.Body.Weight
	switch tag {
	case "h1":
		return 22, font.SemiBold, false
	case "h2":
		return 19, font.SemiBold, false
	case "h3":
		return 17, font.Medium, false
	case "h4", "h5", "h6":
		return 15, font.Medium, false
	case "pre":
		return 13, font.Normal, true
	default:
		return size, weight, false
	}
}
