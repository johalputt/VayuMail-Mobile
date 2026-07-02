// Package mime parses RFC 2822/MIME messages into the display model the
// store persists, and renders bodies safely for the UI.
package mime

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // register charset decoders
	"github.com/emersion/go-message/mail"
)

// AttachmentRef describes one attachment without carrying its bytes; the
// body is fetched on demand by part number.
type AttachmentRef struct {
	Filename    string
	ContentType string
}

// Parsed is the display-ready decomposition of one message.
type Parsed struct {
	// Text is the plain-text body, empty if the message had none.
	Text string
	// HTML is the raw HTML body, empty if the message had none. Always
	// pass it through render.go before display.
	HTML string
	// Snippet is a single-line preview derived from the body.
	Snippet string
	// Attachments lists attachment metadata in part order.
	Attachments []AttachmentRef
	// PGPStatus is "", "signed", "encrypted", or "signed+encrypted",
	// matching the store's pgp_status column.
	PGPStatus string
}

// Parse decomposes a raw RFC 2822 message. It is tolerant: charset and
// structural problems degrade to best-effort output rather than failing
// the whole message, because a mail client must display what it received.
func Parse(raw []byte) (*Parsed, error) {
	p := &Parsed{}

	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		// Header-only or malformed message: fall back to treating the
		// remainder as plain text so something is always displayable.
		if mr == nil {
			return nil, fmt.Errorf("mime: unreadable message: %w", err)
		}
	}
	defer mr.Close()

	ct, ctParams, ctErr := mr.Header.ContentType()
	if ctErr == nil {
		p.PGPStatus = pgpStatusFromContentType(ct, ctParams)
	}

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if message.IsUnknownCharset(err) {
				continue
			}
			// Structural error mid-message: keep what we already have.
			break
		}
		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			partType, _, err := h.ContentType()
			if err != nil {
				continue
			}
			body, err := io.ReadAll(io.LimitReader(part.Body, 1<<20))
			if err != nil {
				continue
			}
			switch {
			case strings.EqualFold(partType, "text/plain") && p.Text == "":
				p.Text = string(body)
			case strings.EqualFold(partType, "text/html") && p.HTML == "":
				p.HTML = string(body)
			}
		case *mail.AttachmentHeader:
			filename, err := h.Filename()
			if err != nil {
				filename = ""
			}
			partType, _, err := h.ContentType()
			if err != nil {
				partType = "application/octet-stream"
			}
			p.Attachments = append(p.Attachments, AttachmentRef{
				Filename:    filename,
				ContentType: partType,
			})
		}
	}

	if p.PGPStatus == "" && looksInlinePGP(p.Text) {
		p.PGPStatus = "encrypted"
	}
	p.Snippet = Snippet(p.Text, p.HTML)
	return p, nil
}

// pgpStatusFromContentType maps PGP/MIME (RFC 3156) structures onto the
// store's pgp_status values.
func pgpStatusFromContentType(ct string, params map[string]string) string {
	proto := strings.ToLower(params["protocol"])
	switch strings.ToLower(ct) {
	case "multipart/encrypted":
		if proto == "application/pgp-encrypted" {
			// PGP/MIME encrypted payloads are conventionally also signed
			// inside; the true status is known only after decryption.
			return "encrypted"
		}
	case "multipart/signed":
		if proto == "application/pgp-signature" {
			return "signed"
		}
	}
	return ""
}

// looksInlinePGP detects inline (non-MIME) PGP messages.
func looksInlinePGP(text string) bool {
	return strings.Contains(text, "-----BEGIN PGP MESSAGE-----")
}

// Snippet builds a single-line preview from the best available body,
// capped at 160 characters on a rune boundary.
func Snippet(text, html string) string {
	src := text
	if src == "" && html != "" {
		src = HTMLToText(html)
	}
	src = strings.Join(strings.Fields(src), " ")
	runes := []rune(src)
	if len(runes) > 160 {
		return string(runes[:160])
	}
	return src
}
