package test

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
)

// The rich pipeline's contract: hostile HTML -> sanitized events ->
// styled paragraphs. These tests run BOTH stages so a sanitizer change
// that silently breaks presentation (or the reverse) fails here, in the
// package the coverage gate measures.

func parasOf(t *testing.T, src string) []mime.RichPara {
	t.Helper()
	return mime.RichParagraphs(mime.SanitizeHTML(src))
}

func TestRichSpansNestStyles(t *testing.T) {
	paras := parasOf(t, `<p>plain <b>bold <i>both</i></b> plain</p>`)
	if len(paras) != 1 {
		t.Fatalf("paras = %d, want 1", len(paras))
	}
	var boldBoth bool
	for _, r := range paras[0].Runs {
		if strings.Contains(r.Text, "both") && r.Bold && r.Italic {
			boldBoth = true
		}
		if strings.Contains(r.Text, "plain") && (r.Bold || r.Italic) {
			t.Fatalf("style leaked outside its tags: %+v", r)
		}
	}
	if !boldBoth {
		t.Fatalf("nested style lost: %+v", paras[0].Runs)
	}
}

func TestRichSpansLinkHrefReachesRuns(t *testing.T) {
	paras := parasOf(t, `<p><a href="https://ok.example/x">click</a>
	<a href="javascript:evil()">no</a></p>`)
	linked, plainAfterReject := "", ""
	for _, p := range paras {
		for _, r := range p.Runs {
			switch {
			case strings.Contains(r.Text, "click"):
				linked = r.Link
			case strings.Contains(r.Text, "no"):
				plainAfterReject = r.Link
			}
		}
	}
	if linked != "https://ok.example/x" {
		t.Fatalf("link href = %q", linked)
	}
	if plainAfterReject != "" {
		t.Fatalf("rejected anchor still linked: %q", plainAfterReject)
	}
}

func TestRichSpansBlocksQuotesAndBullets(t *testing.T) {
	paras := parasOf(t, `<blockquote><p>q1</p><ul><li>one</li><li>two</li></ul>
	<h2>head</h2></blockquote>`)

	type want struct{ tag string; depth int; prefix string }
	got := map[string]bool{}
	for _, p := range paras {
		key := p.Tag + "/" + itoa(p.Depth)
		text := ""
		for _, r := range p.Runs {
			text += r.Text
		}
		got[key+"="+text] = true
		_ = want{}
	}
	for _, need := range []string{
		"p/1=q1",
		"li/1=• one",
		"li/1=• two",
		"h2/1=head",
	} {
		if !got[need] {
			t.Fatalf("missing paragraph %q; got %v", need, got)
		}
	}
}

func TestRichSpansBreakStaysInParagraph(t *testing.T) {
	paras := parasOf(t, `<p>line one<br>line two</p><p>next</p>`)
	if len(paras) != 2 {
		t.Fatalf("paras = %d, want 2 (<br> must not split)", len(paras))
	}
	joined := ""
	for _, r := range paras[0].Runs {
		joined += r.Text
	}
	if joined != "line one\nline two" {
		t.Fatalf("break handling wrong: %q", joined)
	}
}

func TestRichSpansPrePreservesNewlines(t *testing.T) {
	paras := parasOf(t, `<pre>a
  b</pre>`)
	if len(paras) != 1 || paras[0].Tag != "pre" {
		t.Fatalf("pre para wrong: %+v", paras)
	}
	if !strings.Contains(paras[0].Runs[0].Text, "\n") ||
		!strings.Contains(paras[0].Runs[0].Text, "  b") {
		t.Fatalf("pre whitespace collapsed: %q", paras[0].Runs[0].Text)
	}
}

func TestRichSpansImageAltPlaceholder(t *testing.T) {
	paras := parasOf(t, `<p>x<img src="https://t.example/px" alt="chart">y</p>`)
	found := false
	for _, r := range paras[0].Runs {
		if r.Text == "[image: chart]" {
			if !r.Italic {
				t.Fatal("placeholder not italic")
			}
			found = true
		}
		if strings.Contains(r.Text, "t.example") {
			t.Fatal("tracker URL survived")
		}
	}
	if !found {
		t.Fatal("alt placeholder missing")
	}
}

func TestRichSpansWhitespaceBetweenBlocksDropped(t *testing.T) {
	paras := parasOf(t, `   <p>a</p>

	<p>b</p>   `)
	if len(paras) != 2 {
		t.Fatalf("inter-block whitespace created paras: %d", len(paras))
	}
	for _, p := range paras {
		if len(p.Runs) != 1 || strings.TrimSpace(p.Runs[0].Text) == "" {
			t.Fatalf("empty/whitespace-only para: %+v", p)
		}
	}
}

// The invariant chain end to end: no run ever activates a link that is
// not exactly https or mailto. This is the same guarantee the sanitizer's
// fuzz target checks at event level, re-verified at presentation level.
func TestRichSpansLinkInvariant(t *testing.T) {
	inputs := []string{
		`<a href="HTTPS://OK.example/a">caps scheme</a>`,
		`<a href="mailto:x@y.z">mail</a>`,
		`<a href="ftp://f/">f</a>`,
		`<a href="JaVaScRiPt:v">case games</a>`,
		`<a href=" https://spaced.example ">space</a>`,
		`<b><a href="https://n.example">nested</a></b>`,
	}
	for _, src := range inputs {
		for _, p := range parasOf(t, src) {
			for _, r := range p.Runs {
				if r.Link == "" {
					continue
				}
				u, err := url.Parse(r.Link)
				if err != nil || (u.Scheme != "https" && u.Scheme != "mailto") {
					t.Fatalf("%s: bad link on run %+v", src, r)
				}
			}
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
