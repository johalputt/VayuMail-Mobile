package test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
)

// The rich renderer's trust boundary: hostile markup in, inert events out.
// These tests pin the three rules of sanitize.go — allowlisted tags,
// attributes-by-construction, resource bounds — because a regression in
// any one turns styled mail into an active-content channel.

func TestSanitizeHTMLDropsScriptSubtree(t *testing.T) {
	events := mime.SanitizeHTML(`<p>hello</p><script>alert(1)</script><p>world</p>`)
	var text strings.Builder
	for _, ev := range events {
		if ev.Kind == mime.RichText {
			text.WriteString(ev.Text)
		}
		if strings.Contains(strings.ToLower(ev.Text), "alert") {
			t.Fatalf("script payload survived: %q", ev.Text)
		}
	}
	if got := text.String(); !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("legitimate text lost: %q", got)
	}
}

func TestSanitizeHTMLUnwrapsUnknownTags(t *testing.T) {
	// <marquee> is not on any list; its TEXT still corresponds to prose
	// and must survive unwrapping.
	events := mime.SanitizeHTML(`<marquee>wheee</marquee>`)
	found := false
	for _, ev := range events {
		if ev.Kind == mime.RichText && strings.Contains(ev.Text, "wheee") {
			found = true
		}
	}
	if !found {
		t.Fatal("unknown-tag text was dropped instead of unwrapped")
	}
}

func TestSanitizeHTMLLinkSchemeAllowlist(t *testing.T) {
	cases := map[string]bool{
		`<a href="https://ok.example/x">a</a>`:   true,
		`<a href="mailto:bob@example.com">b</a>`: true,
		`<a href="javascript:alert(1)">c</a>`:    false,
		`<a href="data:text/html,x">d</a>`:       false,
		`<a href="//evil.example">e</a>`:         false,
		`<a href="relative/path">f</a>`:          false,
	}
	for htmlIn, wantLink := range cases {
		events := mime.SanitizeHTML(htmlIn)
		gotLink := false
		for _, ev := range events {
			if ev.Kind == mime.RichLinkOpen {
				gotLink = true
				u, err := url.Parse(ev.Href)
				if err != nil || (u.Scheme != "https" && u.Scheme != "mailto") {
					t.Fatalf("%s: emitted non-allowlisted href %q", htmlIn, ev.Href)
				}
			}
		}
		if gotLink != wantLink {
			t.Fatalf("%s: link=%v want %v", htmlIn, gotLink, wantLink)
		}
	}
}

func TestSanitizeHTMLImageIsAltOnly(t *testing.T) {
	events := mime.SanitizeHTML(`<img src="https://tracker.example/px.gif" alt="cat photo">`)
	sawImage, sawAlt := false, false
	for _, ev := range events {
		if ev.Kind == mime.RichImage {
			sawImage = true
			sawAlt = ev.Alt == "cat photo"
			if strings.Contains(ev.Alt+ev.Text+ev.Href+ev.Tag, "tracker.example") {
				t.Fatal("image URL survived into events")
			}
		}
	}
	if !sawImage || !sawAlt {
		t.Fatalf("image placeholder wrong: image=%v alt=%v", sawImage, sawAlt)
	}
}

func TestSanitizeHTMLQuoteDepthNests(t *testing.T) {
	events := mime.SanitizeHTML(`<blockquote><p>a</p><blockquote><p>b</p></blockquote></blockquote>`)
	depths := map[int]int{}
	for _, ev := range events {
		if ev.Tag == "blockquote" && ev.Kind == mime.RichBlockOpen {
			depths[ev.Depth]++
		}
	}
	if depths[1] != 1 || depths[2] != 1 {
		t.Fatalf("quote nesting broken: %+v", depths)
	}
}

func TestSanitizeHTMLResourceBounds(t *testing.T) {
	if got := mime.SanitizeHTML(strings.Repeat("<b>x</b>", 200*1024)); got != nil {
		t.Fatalf("oversize input accepted (%d bytes in)", len(got))
	}
	huge := "<p>" + strings.Repeat("<span>s</span>", 30000) + "</p>"
	if got := mime.SanitizeHTML(huge); len(got) > 20000 {
		t.Fatalf("event cap not enforced: %d events", len(got))
	}
}

// FuzzSanitizedHTML hammers the rich path with adversarial markup. The
// invariant that matters most lives INSIDE the loop: every link event
// ever emitted must carry exactly an https or mailto scheme — no input,
// however crafted, may smuggle anything else through.
func FuzzSanitizedHTML(f *testing.F) {
	f.Add(`<html><body><a href="javascript:x">y</a><script>z</script></body></html>`)
	f.Add(`<table><tr><td style="color:red" onclick="p()">cell</td></tr></table>`)
	f.Add(`<blockquote>&amp;&lt;&gt;<ul><li>i</li></ul></blockquote>`)
	f.Add(`<img src=x onerror=alert(1) alt="a">`)
	f.Fuzz(func(t *testing.T, src string) {
		events := mime.SanitizeHTML(src)
		for _, ev := range events {
			if ev.Kind == mime.RichLinkOpen {
				u, err := url.Parse(ev.Href)
				if err != nil || (u.Scheme != "https" && u.Scheme != "mailto") {
					t.Fatalf("fuzz found href escape: %q from input %q", ev.Href, src)
				}
			}
		}
	})
}
