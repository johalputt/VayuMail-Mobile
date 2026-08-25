package mime

// sanitize.go — rich-HTML mail rendering's trust boundary (plan Phase 6).
//
// The text-only path (HTMLToText) is the strongest possible sanitizer;
// this file adds a STRUCTURED one for the flagged rich renderer: hostile
// markup goes in, a flat span/block event stream of provably inert pieces
// comes out. Three rules do all the work:
//
//  1. ALLOWLIST TAGS. Anything not explicitly permitted is either
//     unwrapped (children kept, element discarded) or, for the dangerous
//     set — script, iframe, form controls, media, embedded documents —
//     dropped WITH its subtree, because their text content is payload,
//     not prose.
//  2. ATTRIBUTES BY CONSTRUCTION. The output type has no attribute bag;
//     a link carries only a scheme-validated href and an image only its
//     alt text. There is nowhere for style= or onload= to land.
//  3. RESOURCE BOUNDS. Input size and output length are capped before
//     and during the walk, so a 40 MB nested-table newsletter cannot turn
//     into a layout hang on a phone.
//
// The renderer consumes events left to right; it never sees a tag name it
// did not ask about and never parses HTML itself. Feature-flagged until
// it survives a fuzzing season (FuzzSanitizedHTML).

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// RichKind classifies one sanitized event.
type RichKind uint8

// Sanitized document events, in document order.
const (
	// RichText is decoded character data.
	RichText RichKind = iota
	// RichBlockOpen / RichBlockClose bracket a block element; Tag names it
	// ("p", "li", "h1"…"h6", "blockquote", "pre") and Depth carries quote
	// nesting so the UI can indent without re-parsing.
	RichBlockOpen
	RichBlockClose
	// RichLinkOpen / RichLinkClose bracket styled link text; Href passed
	// the scheme allowlist. Anchors with any other scheme emit no link
	// events at all.
	RichLinkOpen
	RichLinkClose
	// RichImage is an image placeholder: remote and data sources are NOT
	// loaded in v1; Alt is all that survives.
	RichImage
)

// RichEvent is one inert piece of a sanitized HTML document.
type RichEvent struct {
	Kind  RichKind
	Tag   string
	Href  string
	Alt   string
	Text  string
	Depth int
}

// maxSanitizeInput bounds the parse: beyond half a megabyte of markup a
// fallback to the plain-text renderer is cheaper and safer than trusting
// the walk to stay cheap.
const maxSanitizeInput = 512 * 1024

// maxRichEvents bounds output so even legal-but-absurd documents cannot
// allocate unboundedly while rendering.
const maxRichEvents = 20000

// dropSubtree elements vanish together with everything inside them. Their
// text content is executable or metadata, never correspondence.
var dropSubtree = map[string]bool{
	"script": true, "style": true, "head": true, "title": true,
	"iframe": true, "object": true, "embed": true, "applet": true,
	"noscript": true, "template": true, "svg": true, "math": true,
	"form": true, "input": true, "button": true, "select": true,
	"option": true, "textarea": true, "label": true, "fieldset": true,
	"audio": true, "video": true, "source": true, "track": true,
	"canvas": true, "frame": true, "frameset": true, "base": true,
	"link": true, "meta": true, "dialog": true, "slot": true,
}

// richBlockTags maps permitted block elements to their event tag name.
// Tables are flattened to paragraphs per cell: v1 renders structure, not
// grids. (render.go's blockTags is the text renderer's own list; this one
// is the rich path's.)
var richBlockTags = map[string]string{
	"p": "p", "br": "br", "div": "p", "section": "p", "article": "p",
	"h1": "h1", "h2": "h2", "h3": "h3", "h4": "h4", "h5": "h5",
	"h6": "h6", "blockquote": "blockquote", "pre": "pre",
	"ul": "ul", "ol": "ol", "li": "li", "tr": "tr",
	"td": "td", "th": "td",
}

// inlineTags are permitted styling carriers; the UI decides how each maps
// onto its span styles.
var inlineTags = map[string]bool{
	"b": true, "strong": true, "i": true, "em": true, "u": true,
	"span": true, "code": true, "a": true, "sub": true, "sup": true,
}

// SanitizeHTML converts attacker-controlled HTML into the flat inert
// event stream described above. Documents over maxSanitizeInput return
// nil — callers fall back to HTMLToText.
func SanitizeHTML(src string) []RichEvent {
	if len(src) == 0 || len(src) > maxSanitizeInput {
		return nil
	}
	root, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil
	}
	out := make([]RichEvent, 0, 64)
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		emitNode(&out, c, 0)
	}
	return out
}

// emitNode processes one node into out, recursing for permitted elements.
// Events are appended only through appendEvent, which enforces the output
// cap.
func emitNode(out *[]RichEvent, n *html.Node, quoteDepth int) {
	if len(*out) >= maxRichEvents {
		return
	}
	switch n.Type {
	case html.TextNode:
		appendEvent(out, RichEvent{Kind: RichText, Text: n.Data})
		return
	case html.ElementNode:
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			emitNode(out, c, quoteDepth)
		}
		return
	default: // comments, doctype: nothing to say
		return
	}

	name := strings.ToLower(n.Data)
	if dropSubtree[name] {
		return
	}
	switch name {
	case "blockquote":
		appendEvent(out, RichEvent{Kind: RichBlockOpen, Tag: name, Depth: quoteDepth + 1})
		recurseChildren(out, n, quoteDepth+1)
		appendEvent(out, RichEvent{Kind: RichBlockClose, Tag: name, Depth: quoteDepth + 1})
		return
	case "a":
		// A rejected href means no link event at all — the stream's
		// invariant is absolute: every RichLinkOpen carries exactly an
		// https or mailto URL, never an empty one for the UI to guess at.
		if href := safeHref(attrOf(n, "href")); href != "" {
			appendEvent(out, RichEvent{Kind: RichLinkOpen, Href: href})
			recurseChildren(out, n, quoteDepth)
			appendEvent(out, RichEvent{Kind: RichLinkClose})
			return
		}
		recurseChildren(out, n, quoteDepth)
		return
	case "img":
		appendEvent(out, RichEvent{Kind: RichImage, Alt: attrOf(n, "alt")})
		return
	}
	if tag, ok := richBlockTags[name]; ok {
		appendEvent(out, RichEvent{Kind: RichBlockOpen, Tag: tag, Depth: quoteDepth})
		recurseChildren(out, n, quoteDepth)
		appendEvent(out, RichEvent{Kind: RichBlockClose, Tag: tag, Depth: quoteDepth})
		return
	}
	if inlineTags[name] {
		recurseChildren(out, n, quoteDepth)
		return
	}
	// Unknown element: unwrap — children survive, the tag does not.
	recurseChildren(out, n, quoteDepth)
}

func recurseChildren(out *[]RichEvent, n *html.Node, quoteDepth int) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		emitNode(out, c, quoteDepth)
	}
}

func appendEvent(out *[]RichEvent, ev RichEvent) {
	if len(*out) >= maxRichEvents {
		return
	}
	*out = append(*out, ev)
}

// attrOf returns one attribute value, lowercased name match.
func attrOf(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// safeHref enforces the link scheme allowlist: https and mailto only.
// javascript:, data:, and every relative or protocol-relative form are
// rejected — a rich renderer must never become a tap-to-execute surface.
// An empty result makes the anchor unwrap to plain text (see the "a"
// case); no link event is emitted for it at all.
func safeHref(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "mailto":
		return raw
	default:
		return ""
	}
}
