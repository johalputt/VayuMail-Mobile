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
	// EncryptedBlock is the armored PGP ciphertext for an encrypted
	// message (from the PGP/MIME octet-stream part, or an inline
	// -----BEGIN PGP MESSAGE----- block). Empty when not encrypted. The
	// display layer decrypts it with the keyring's private key; it is
	// never shown raw.
	EncryptedBlock string
	// HasTrackers reports detected tracking pixels or tracker-hosted
	// resources (see track.go). VayuMail never fetches remote content;
	// this powers the "this sender tracks you" indicator.
	HasTrackers bool
	// ListID is the List-Id header value; non-empty marks newsletter or
	// mailing-list traffic.
	ListID string
	// ListUnsubscribe is the raw List-Unsubscribe header value.
	ListUnsubscribe string
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
	p.ListID = mr.Header.Get("List-Id")
	p.ListUnsubscribe = mr.Header.Get("List-Unsubscribe")

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
			// PGP/MIME ciphertext often has no Content-Disposition, so
			// go-message hands it back as an inline octet-stream part
			// rather than an attachment. Capture any armored block here too
			// so both classifications feed the decrypt-on-display path.
			if isEncrypted(p.PGPStatus) && p.EncryptedBlock == "" &&
				strings.Contains(string(body), "-----BEGIN PGP MESSAGE-----") &&
				!strings.EqualFold(partType, "text/html") {
				p.EncryptedBlock = extractInlinePGP(string(body))
			}
			switch {
			case strings.EqualFold(partType, "text/plain") && p.Text == "":
				p.Text = string(body)
			case strings.EqualFold(partType, "text/html") && p.HTML == "":
				p.HTML = string(body)
			}
		case *mail.AttachmentHeader:
			partType, _, err := h.ContentType()
			if err != nil {
				partType = "application/octet-stream"
			}
			// PGP/MIME (RFC 3156) carries the ciphertext as an
			// application/octet-stream part; capture its body so the
			// message can be decrypted for display instead of surfacing as
			// a mystery attachment.
			lpt := strings.ToLower(partType)
			if isEncrypted(p.PGPStatus) && p.EncryptedBlock == "" &&
				(strings.Contains(lpt, "octet-stream") || strings.Contains(lpt, "pgp")) {
				blk, rerr := io.ReadAll(io.LimitReader(part.Body, 1<<20))
				if rerr == nil && len(blk) > 0 {
					p.EncryptedBlock = string(blk)
					continue
				}
			}
			filename, err := h.Filename()
			if err != nil {
				filename = ""
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
	// Inline PGP keeps its armored block in the text body; lift it out so
	// the same decrypt-on-display path handles both PGP/MIME and inline.
	if isEncrypted(p.PGPStatus) && p.EncryptedBlock == "" && looksInlinePGP(p.Text) {
		p.EncryptedBlock = extractInlinePGP(p.Text)
	}
	p.HasTrackers = DetectTrackers(p.HTML)
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

// isEncrypted reports whether a PGP status implies an encrypted body.
func isEncrypted(status string) bool {
	return status == "encrypted" || status == "signed+encrypted"
}

// extractInlinePGP returns the armored -----BEGIN PGP MESSAGE----- block
// from a text body, or "" if the delimiters are not both present.
func extractInlinePGP(text string) string {
	const begin = "-----BEGIN PGP MESSAGE-----"
	const end = "-----END PGP MESSAGE-----"
	i := strings.Index(text, begin)
	if i < 0 {
		return ""
	}
	j := strings.Index(text[i:], end)
	if j < 0 {
		return ""
	}
	return text[i : i+j+len(end)]
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
