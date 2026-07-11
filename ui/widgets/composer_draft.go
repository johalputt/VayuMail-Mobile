package widgets

// composer_draft.go — the Composer's draft-building half: reply and
// forward prefills, recipient parsing, key-status counting, and the
// final Draft assembly. Split from composer.go (Rule 10).

import (
	"strings"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/smtpsend"
)

// PrefillReply seeds the composer for a reply.
func (c *Composer) PrefillReply(to, subject string) {
	c.Reset()
	c.to.SetText(to)
	if subject != "" && !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	c.subject.SetText(subject)
}

// PrefillForward seeds the composer with a quoted copy of a message;
// the recipient stays empty for the user to fill.
func (c *Composer) PrefillForward(subject, fromName, fromAddr, date, body string) {
	c.Reset()
	if subject != "" && !strings.HasPrefix(strings.ToLower(subject), "fwd:") {
		subject = "Fwd: " + subject
	}
	c.subject.SetText(subject)
	sender := fromName
	if sender == "" {
		sender = fromAddr
	} else if fromAddr != "" {
		sender += " <" + fromAddr + ">"
	}
	var b strings.Builder
	b.WriteString("\n\n---------- Forwarded message ----------\n")
	b.WriteString("From: " + sender + "\n")
	if date != "" {
		b.WriteString("Date: " + date + "\n")
	}
	b.WriteString("Subject: " + strings.TrimPrefix(subject, "Fwd: ") + "\n\n")
	b.WriteString(body)
	c.body.SetText(b.String())
}

// RecipientList returns every parsed recipient address (To, Cc, Bcc).
func (c *Composer) RecipientList() []string {
	var out []string
	out = append(out, splitAddrs(c.to.Text())...)
	out = append(out, splitAddrs(c.cc.Text())...)
	out = append(out, splitAddrs(c.bcc.Text())...)
	return out
}

// missingKeyCount counts recipients the keyring has no key for.
func (c *Composer) missingKeyCount() int {
	if c.HasKey == nil {
		return 0
	}
	n := 0
	for _, a := range c.RecipientList() {
		if !c.HasKey(a) {
			n++
		}
	}
	return n
}

// Draft builds the outbound draft from the current fields.
func (c *Composer) Draft(fromName, fromAddr string) smtpsend.Draft {
	return smtpsend.Draft{
		FromName:    fromName,
		FromAddr:    fromAddr,
		To:          splitAddrs(c.to.Text()),
		Cc:          splitAddrs(c.cc.Text()),
		Bcc:         splitAddrs(c.bcc.Text()),
		Subject:     strings.TrimSpace(c.subject.Text()),
		TextBody:    c.body.Text(),
		Attachments: c.attachmentsCopy(),
	}
}
