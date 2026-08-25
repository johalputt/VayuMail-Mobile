package mime

// richspans.go — the presentation half of the rich-HTML path (plan
// Phase 6). SanitizeHTML's flat event stream is safe but not shaped for
// layout; this file folds it into paragraphs of styled runs that both the
// tests (in ./test/, visible to the coverage gate) and the UI's rich
// renderer can consume directly. No HTML parsing happens here — input is
// already inert by construction — so the only job is faithful grouping:
// styles nest via counters, links carry their validated href onto every
// covered run, and whitespace collapses the way HTML renderers must,
// except inside <pre>, where newlines are content.

import "strings"

// RichRun is one contiguous stretch of same-styled text.
type RichRun struct {
	Text      string
	Bold      bool
	Italic    bool
	Underline bool
	Code      bool
	// Link is the https/mailto URL this run activates, or "" for plain
	// text. Only sanitizer-validated hrefs ever land here.
	Link string
}

// sameStyle reports whether two runs can merge into one.
func (r RichRun) sameStyle(o RichRun) bool {
	return r.Bold == o.Bold && r.Italic == o.Italic &&
		r.Underline == o.Underline && r.Code == o.Code && r.Link == o.Link
}

// RichPara is one displayable paragraph: a block tag, its quote nesting,
// and the styled runs to lay out in order. Newlines inside Runs are soft
// breaks within the paragraph.
type RichPara struct {
	// Tag names the block ("p", "h1"…"h6", "li", "pre", "td"); the UI
	// picks type scale from it.
	Tag   string
	Depth int // blockquote nesting depth
	Runs  []RichRun
}

// maxRichParas bounds output independently of the event cap: a document
// made of single-character spans could otherwise multiply into thousands
// of tiny paragraphs.
const maxRichParas = 5000

type spanState struct {
	bold, italic, under, code int // nesting counters
	link                      string
}

func (s *spanState) apply(style string, on bool) {
	d := 1
	if !on {
		d = -1
	}
	switch style {
	case "b":
		s.bold += d
	case "i":
		s.italic += d
	case "u":
		s.under += d
	case "code":
		s.code += d
	}
	if s.bold < 0 {
		s.bold = 0
	}
	if s.italic < 0 {
		s.italic = 0
	}
	if s.under < 0 {
		s.under = 0
	}
	if s.code < 0 {
		s.code = 0
	}
}

// RichParagraphs converts sanitized events into paragraphs of styled
// runs. Blocks become paragraphs; br becomes a newline run inside the
// open paragraph; list items gain a bullet prefix; images become their
// alt text in italics; pre keeps its newlines verbatim.
func RichParagraphs(events []RichEvent) []RichPara {
	paras := make([]RichPara, 0, 16)
	var cur *RichPara
	var st spanState
	pendingBullet := false
	inPre := false

	flush := func() {
		if cur != nil && len(cur.Runs) > 0 {
			// The cap is enforced at flush, the only place paragraphs are
			// born: a document of single-character blocks cannot multiply
			// past maxRichParas no matter how many events it produced.
			if len(paras) < maxRichParas {
				paras = append(paras, *cur)
			}
		}
		cur = nil
	}

	for _, ev := range events {
		switch ev.Kind {
		case RichText:
			text := ev.Text
			if !inPre {
				text = collapseSpace(text)
				if text == "" {
					continue
				}
				if cur == nil {
					// Whitespace between blocks never opens a paragraph.
					text = strings.TrimLeft(text, " ")
					if text == "" {
						continue
					}
				}
			}
			if cur == nil {
				cur = &RichPara{}
			}
			cur.append(RichRun{
				Text:      text,
				Bold:      st.bold > 0,
				Italic:    st.italic > 0,
				Underline: st.under > 0,
				Code:      st.code > 0,
				Link:      st.link,
			})
		case RichBlockOpen:
			flush()
			switch ev.Tag {
			case "br":
				continue // stray top-level br opens no paragraph
			case "li":
				pendingBullet = true
			case "pre":
				inPre = true
			}
			cur = &RichPara{Tag: ev.Tag, Depth: ev.Depth}
			if pendingBullet {
				pendingBullet = false
				cur.append(RichRun{Text: "• ", Bold: true})
			}
		case RichBlockClose:
			flush()
			if ev.Tag == "pre" {
				inPre = false
			}
		case RichStyleOpen:
			st.apply(ev.Style, true)
		case RichStyleClose:
			st.apply(ev.Style, false)
		case RichLinkOpen:
			st.link = ev.Href
		case RichLinkClose:
			st.link = ""
		case RichBreak:
			if cur == nil {
				cur = &RichPara{}
			}
			cur.append(RichRun{Text: "\n",
				Bold: st.bold > 0, Italic: st.italic > 0,
				Code: st.code > 0})
		case RichImage:
			alt := strings.TrimSpace(ev.Alt)
			if alt == "" {
				alt = "image"
			}
			if cur == nil {
				cur = &RichPara{}
			}
			cur.append(RichRun{Text: "[image: " + alt + "]", Italic: true})
		}
	}
	flush()
	return paras
}

// append adds a run, merging into the previous one when the STYLE matches
// so long stretches stay cheap to shape and wrap.
func (p *RichPara) append(r RichRun) {
	if n := len(p.Runs); n > 0 && p.Runs[n-1].sameStyle(r) {
		p.Runs[n-1].Text += r.Text
		return
	}
	p.Runs = append(p.Runs, r)
}

// collapseSpace applies HTML's inline whitespace rule: every run of
// spaces/tabs/newlines collapses to one space.
func collapseSpace(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n':
			space = true
		default:
			if space {
				b.WriteByte(' ')
				space = false
			}
			b.WriteRune(r)
		}
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}
