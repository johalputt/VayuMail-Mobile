// Package smtpsend builds outbound MIME messages, queues them through the
// store outbox, and delivers them over SMTP with STARTTLS or implicit TLS.
package smtpsend

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

// Attachment is one file attached to a draft, held in memory until the
// message is serialized into the outbox.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Draft is a composed message before MIME serialization. Bcc recipients
// are serialized into the stored message so the outbox can address the
// envelope after a restart; the Bcc header is stripped from the bytes put
// on the wire (see stripBcc in outbox.go).
type Draft struct {
	FromName    string
	FromAddr    string
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	TextBody    string
	Attachments []Attachment
}

// Recipients returns the full envelope recipient list: To + Cc + Bcc.
func (d *Draft) Recipients() []string {
	out := make([]string, 0, len(d.To)+len(d.Cc)+len(d.Bcc))
	out = append(out, d.To...)
	out = append(out, d.Cc...)
	out = append(out, d.Bcc...)
	return out
}

// BuildMIME serializes the draft into a complete RFC 5322 message:
// text/plain for simple drafts, multipart/mixed when attachments are
// present. Rich text is intentionally unsupported at v0.1.0
// (COMPLIANCE-TRACKER.md: "Rich text compose", PENDING).
func BuildMIME(d *Draft) ([]byte, error) {
	header, err := draftHeader(d)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if len(d.Attachments) == 0 {
		header.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
		w, err := mail.CreateSingleInlineWriter(&buf, header)
		if err != nil {
			return nil, fmt.Errorf("smtpsend: create writer: %w", err)
		}
		if _, err := io.WriteString(w, d.TextBody); err != nil {
			return nil, fmt.Errorf("smtpsend: write body: %w", err)
		}
		if err := w.Close(); err != nil {
			return nil, fmt.Errorf("smtpsend: close writer: %w", err)
		}
		return buf.Bytes(), nil
	}

	mw, err := mail.CreateWriter(&buf, header)
	if err != nil {
		return nil, fmt.Errorf("smtpsend: create multipart writer: %w", err)
	}
	var inline mail.InlineHeader
	inline.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	iw, err := mw.CreateSingleInline(inline)
	if err != nil {
		return nil, fmt.Errorf("smtpsend: create inline part: %w", err)
	}
	if _, err := io.WriteString(iw, d.TextBody); err != nil {
		return nil, fmt.Errorf("smtpsend: write body: %w", err)
	}
	if err := iw.Close(); err != nil {
		return nil, fmt.Errorf("smtpsend: close inline part: %w", err)
	}
	for _, att := range d.Attachments {
		if err := writeAttachment(mw, att); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("smtpsend: close message: %w", err)
	}
	return buf.Bytes(), nil
}

func writeAttachment(mw *mail.Writer, att Attachment) error {
	var ah mail.AttachmentHeader
	ah.SetFilename(att.Filename)
	ct := att.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	ah.SetContentType(ct, nil)
	aw, err := mw.CreateAttachment(ah)
	if err != nil {
		return fmt.Errorf("smtpsend: create attachment: %w", err)
	}
	if _, err := aw.Write(att.Data); err != nil {
		return fmt.Errorf("smtpsend: write attachment: %w", err)
	}
	if err := aw.Close(); err != nil {
		return fmt.Errorf("smtpsend: close attachment: %w", err)
	}
	return nil
}

// BuildPGPEncrypted wraps an armored PGP ciphertext into an RFC 3156
// multipart/encrypted message. The ciphertext must already cover the
// draft's body (see internal/mail/pgp).
func BuildPGPEncrypted(d *Draft, armoredCiphertext []byte) ([]byte, error) {
	header, err := draftHeader(d)
	if err != nil {
		return nil, err
	}
	header.SetContentType("multipart/encrypted", map[string]string{
		"protocol": "application/pgp-encrypted",
	})

	var buf bytes.Buffer
	w, err := message.CreateWriter(&buf, header.Header)
	if err != nil {
		return nil, fmt.Errorf("smtpsend: create pgp writer: %w", err)
	}

	var versionHeader message.Header
	versionHeader.Set("Content-Type", "application/pgp-encrypted")
	vp, err := w.CreatePart(versionHeader)
	if err != nil {
		return nil, fmt.Errorf("smtpsend: create pgp version part: %w", err)
	}
	if _, err := io.WriteString(vp, "Version: 1\r\n"); err != nil {
		return nil, fmt.Errorf("smtpsend: write pgp version: %w", err)
	}
	if err := vp.Close(); err != nil {
		return nil, fmt.Errorf("smtpsend: close pgp version part: %w", err)
	}

	var bodyHeader message.Header
	bodyHeader.Set("Content-Type", `application/octet-stream; name="encrypted.asc"`)
	bp, err := w.CreatePart(bodyHeader)
	if err != nil {
		return nil, fmt.Errorf("smtpsend: create pgp body part: %w", err)
	}
	if _, err := bp.Write(armoredCiphertext); err != nil {
		return nil, fmt.Errorf("smtpsend: write pgp body: %w", err)
	}
	if err := bp.Close(); err != nil {
		return nil, fmt.Errorf("smtpsend: close pgp body part: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("smtpsend: close pgp message: %w", err)
	}
	return buf.Bytes(), nil
}

// draftHeader builds the RFC 5322 header block shared by all builders.
func draftHeader(d *Draft) (mail.Header, error) {
	var h mail.Header
	if d.FromAddr == "" {
		return h, fmt.Errorf("smtpsend: draft has no From address")
	}
	if len(d.To) == 0 && len(d.Cc) == 0 && len(d.Bcc) == 0 {
		return h, fmt.Errorf("smtpsend: draft has no recipients")
	}
	h.SetDate(time.Now())
	h.SetAddressList("From", []*mail.Address{{Name: d.FromName, Address: d.FromAddr}})
	if len(d.To) > 0 {
		h.SetAddressList("To", toAddresses(d.To))
	}
	if len(d.Cc) > 0 {
		h.SetAddressList("Cc", toAddresses(d.Cc))
	}
	if len(d.Bcc) > 0 {
		// Stored intentionally; stripped by stripBcc before the wire.
		h.SetAddressList("Bcc", toAddresses(d.Bcc))
	}
	h.SetSubject(d.Subject)
	domain := "vayumail.invalid"
	if i := strings.LastIndex(d.FromAddr, "@"); i >= 0 {
		domain = d.FromAddr[i+1:]
	}
	if err := h.GenerateMessageIDWithHostname(domain); err != nil {
		return h, fmt.Errorf("smtpsend: message id: %w", err)
	}
	return h, nil
}

func toAddresses(addrs []string) []*mail.Address {
	out := make([]*mail.Address, len(addrs))
	for i, a := range addrs {
		out[i] = &mail.Address{Address: a}
	}
	return out
}
