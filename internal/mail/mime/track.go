package mime

import (
	"strings"

	"golang.org/x/net/html"
)

// trackerHosts are domains whose presence in an email image or link URL
// indicates open/click tracking. The list targets the highest-volume
// trackers; the 1x1-pixel heuristic below catches the long tail.
// VayuMail never fetches remote content regardless — this detection
// exists to *tell the user* they are being tracked, not to protect the
// fetch (there is none).
var trackerHosts = []string{
	"list-manage.com", "mailchimp.com", "mandrillapp.com",
	"sendgrid.net", "sendgrid.com", "sparkpostmail.com",
	"awstrack.me", "aweber.com", "getresponse.com",
	"mailtrack.io", "mixmax.com", "streak.com", "yesware.com",
	"bananatag.com", "mailgun.org", "postmarkapp.com",
	"customeriomail.com", "exacttarget.com", "salesforce.com",
	"hubspotemail.net", "hs-analytics.net", "klaviyomail.com",
	"braze.com", "iterable.com", "sailthru.com",
}

// DetectTrackers reports whether an HTML body carries tracking pixels or
// tracker-hosted resources: 1x1 (or hidden) images, or any image/link
// pointing at a known tracking service.
func DetectTrackers(htmlBody string) bool {
	if htmlBody == "" {
		return false
	}
	tokenizer := html.NewTokenizer(strings.NewReader(htmlBody))
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			return false
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := tokenizer.TagName()
		tag := string(name)
		if tag != "img" && tag != "a" {
			continue
		}
		attrs := map[string]string{}
		for hasAttr {
			var k, v []byte
			k, v, hasAttr = tokenizer.TagAttr()
			attrs[string(k)] = string(v)
		}
		if tag == "img" && isTrackingPixel(attrs) {
			return true
		}
		url := attrs["src"]
		if url == "" {
			url = attrs["href"]
		}
		if urlHitsTrackerHost(url) {
			return true
		}
	}
}

// isTrackingPixel flags images that are invisible by construction.
func isTrackingPixel(attrs map[string]string) bool {
	w := strings.TrimSuffix(strings.TrimSpace(attrs["width"]), "px")
	h := strings.TrimSuffix(strings.TrimSpace(attrs["height"]), "px")
	if (w == "0" || w == "1") && (h == "0" || h == "1") {
		return true
	}
	style := strings.ToLower(attrs["style"])
	if strings.Contains(style, "display:none") ||
		strings.Contains(style, "display: none") ||
		strings.Contains(style, "visibility:hidden") ||
		strings.Contains(style, "visibility: hidden") {
		// A hidden remote image has exactly one purpose.
		return attrs["src"] != ""
	}
	return false
}

// urlHitsTrackerHost matches a URL's host segment against the tracker
// list (suffix match on the registrable domain).
func urlHitsTrackerHost(url string) bool {
	url = strings.ToLower(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}
	rest := url[strings.Index(url, "://")+3:]
	host := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host = rest[:i]
	}
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	for _, t := range trackerHosts {
		if host == t || strings.HasSuffix(host, "."+t) {
			return true
		}
	}
	return false
}

// FirstUnsubscribeTarget extracts the preferred unsubscribe action from
// a raw List-Unsubscribe header value (RFC 2369/8058): the mailto target
// when present (VayuMail can act on it directly), else the first https
// URL for the user to open.
func FirstUnsubscribeTarget(header string) (mailto string, url string) {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "<>")
		lower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lower, "mailto:") && mailto == "":
			addr := part[len("mailto:"):]
			if i := strings.IndexByte(addr, '?'); i >= 0 {
				addr = addr[:i]
			}
			mailto = addr
		case strings.HasPrefix(lower, "https://") && url == "":
			url = part
		}
	}
	return mailto, url
}
