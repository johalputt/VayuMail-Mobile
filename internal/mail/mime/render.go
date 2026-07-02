package mime

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// blockTags are HTML elements that imply a line break when converted to
// text.
var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "pre": true, "table": true, "ul": true, "ol": true,
}

// skipTags are elements whose content is never rendered. Dropping script,
// style, and object content is the core of HTML sanitization for a
// text-only v0.1 renderer: no active content survives the conversion.
var skipTags = map[string]bool{
	"script": true, "style": true, "head": true, "object": true,
	"embed": true, "iframe": true, "noscript": true, "svg": true,
	"template": true,
}

// HTMLToText converts an HTML body to displayable plain text. All markup,
// scripts, styles, and remote references are discarded — VayuMail v0.1
// renders text only, which is also the strongest possible tracking and
// script protection.
func HTMLToText(src string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	skipDepth := 0
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return collapseBlankLines(b.String())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			if skipTags[string(name)] && tt == html.StartTagToken {
				skipDepth++
			}
			if blockTags[string(name)] && skipDepth == 0 {
				b.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			if skipTags[string(name)] && skipDepth > 0 {
				skipDepth--
			}
			if blockTags[string(name)] && skipDepth == 0 {
				b.WriteByte('\n')
			}
		case html.TextToken:
			if skipDepth == 0 {
				b.Write(tokenizer.Text())
			}
		}
	}
}

var multiBlank = regexp.MustCompile(`\n{3,}`)

// collapseBlankLines trims trailing space per line and caps consecutive
// blank lines at one.
func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	s = strings.Join(lines, "\n")
	s = multiBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// DisplayText returns the best displayable body for a message: the plain
// text part when present, otherwise the sanitized text rendering of the
// HTML part.
func DisplayText(text, htmlBody string) string {
	if strings.TrimSpace(text) != "" {
		return WrapPlaintext(text)
	}
	if htmlBody != "" {
		return HTMLToText(htmlBody)
	}
	return ""
}

// WrapPlaintext normalizes a plain-text body for display: CRLF to LF and
// removal of trailing whitespace, preserving intentional line structure
// (including format=flowed soft breaks, which the text shaper re-wraps).
func WrapPlaintext(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return collapseBlankLines(text)
}

// QuoteDepth returns the quotation depth of one plain-text line ("> > x"
// is depth 2). The thread view uses it to fold quoted history.
func QuoteDepth(line string) int {
	depth := 0
	for {
		line = strings.TrimLeft(line, " ")
		if strings.HasPrefix(line, ">") {
			depth++
			line = line[1:]
			continue
		}
		return depth
	}
}
