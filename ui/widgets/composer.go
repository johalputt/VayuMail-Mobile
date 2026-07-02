package widgets

import (
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/smtpsend"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// ComposerAction reports what the user did in the composer this frame.
type ComposerAction struct {
	// Send is true when the send button was tapped and the draft is
	// minimally valid.
	Send bool
	// AttachRequested is true when the attach icon was tapped.
	AttachRequested bool
}

// Composer is the full-screen compose surface: To / Cc / Bcc / Subject /
// Body with one action row pinned to the bottom. Plain text only at
// v0.1.0.
type Composer struct {
	to, cc, bcc, subject, body widget.Editor

	showCcBcc bool
	ccToggle  widget.Clickable
	attachBtn widget.Clickable
	encToggle widget.Clickable
	sigToggle widget.Clickable
	sendBtn   widget.Clickable

	// Encrypt and Sign are the PGP toggles (off = Subtle, on = Accent).
	Encrypt bool
	Sign    bool
}

// NewComposer constructs an empty composer.
func NewComposer() *Composer {
	c := &Composer{}
	for _, e := range []*widget.Editor{&c.to, &c.cc, &c.bcc, &c.subject} {
		e.SingleLine = true
	}
	return c
}

// Reset clears every field for a fresh message.
func (c *Composer) Reset() {
	for _, e := range []*widget.Editor{&c.to, &c.cc, &c.bcc, &c.subject, &c.body} {
		e.SetText("")
	}
	c.showCcBcc = false
	c.Encrypt = false
	c.Sign = false
}

// PrefillReply seeds the composer for a reply.
func (c *Composer) PrefillReply(to, subject string) {
	c.Reset()
	c.to.SetText(to)
	if subject != "" && !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	c.subject.SetText(subject)
}

// Draft builds the outbound draft from the current fields.
func (c *Composer) Draft(fromName, fromAddr string) smtpsend.Draft {
	return smtpsend.Draft{
		FromName: fromName,
		FromAddr: fromAddr,
		To:       splitAddrs(c.to.Text()),
		Cc:       splitAddrs(c.cc.Text()),
		Bcc:      splitAddrs(c.bcc.Text()),
		Subject:  strings.TrimSpace(c.subject.Text()),
		TextBody: c.body.Text(),
	}
}

// Layout renders the composer and reports actions.
func (c *Composer) Layout(gtx layout.Context, th *theme.Theme) ComposerAction {
	var action ComposerAction
	if c.ccToggle.Clicked(gtx) {
		c.showCcBcc = !c.showCcBcc
	}
	if c.encToggle.Clicked(gtx) {
		c.Encrypt = !c.Encrypt
	}
	if c.sigToggle.Clicked(gtx) {
		c.Sign = !c.Sign
	}
	if c.attachBtn.Clicked(gtx) {
		action.AttachRequested = true
	}
	if c.sendBtn.Clicked(gtx) && len(splitAddrs(c.to.Text())) > 0 {
		action.Send = true
	}

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.fieldRow(gtx, th, "To", &c.to, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !c.showCcBcc {
				return layout.Dimensions{}
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.fieldRow(gtx, th, "Cc", &c.cc, false)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.fieldRow(gtx, th, "Bcc", &c.bcc, false)
				}))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.fieldRow(gtx, th, "Subject", &c.subject, false)
		}),
		// Body fills the remaining height.
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.MD}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					if c.body.Len() == 0 {
						th.Label(gtx, theme.Body, th.Palette.Subtle, "Write…", 1)
					}
					c.body.LineHeightScale = 1.5
					return c.body.Layout(gtx, th.Shaper,
						font.Font{Weight: theme.Body.Weight}, theme.Body.Size,
						theme.ColorOp(gtx, th.Palette.OnBackground),
						theme.ColorOp(gtx, th.Palette.AccentSubtle))
				})
		}),
		// Action row pinned to the bottom.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.actionRow(gtx, th)
		}))
	return action
}

// fieldRow draws one labeled single-line field with a hairline below.
func (c *Composer) fieldRow(gtx layout.Context, th *theme.Theme, label string, e *widget.Editor, withCcToggle bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.SM + theme.XS, Bottom: theme.SM + theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Dp(theme.XXL + theme.SM)
							return th.Label(gtx, theme.Caption, th.Palette.Subtle, label, 1)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return e.Layout(gtx, th.Shaper,
								font.Font{Weight: theme.Body.Weight}, theme.Body.Size,
								theme.ColorOp(gtx, th.Palette.OnBackground),
								theme.ColorOp(gtx, th.Palette.AccentSubtle))
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !withCcToggle || c.showCcBcc {
								return layout.Dimensions{}
							}
							return c.ccToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, th.Palette.Subtle, "Cc/Bcc", 1)
							})
						}))
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return Separator(gtx, th, 0)
		}))
}

// actionRow is the single bottom bar: attach, PGP encrypt, PGP sign,
// spacer, send. No toolbar anywhere else.
func (c *Composer) actionRow(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return Separator(gtx, th, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.MD, Right: theme.MD, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return IconButton(gtx, th, &c.attachBtn, IconAttach, th.Palette.Subtle)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							col := th.Palette.Subtle
							if c.Encrypt {
								col = th.Palette.Accent
							}
							return IconButton(gtx, th, &c.encToggle, IconShield, col)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							col := th.Palette.Subtle
							if c.Sign {
								col = th.Palette.Accent
							}
							return IconButton(gtx, th, &c.sigToggle, IconSignature, col)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: gtx.Constraints.Min}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return IconButton(gtx, th, &c.sendBtn, IconSend, th.Palette.Accent)
						}))
				})
		}))
}

// splitAddrs parses a comma- or space-separated recipient list.
func splitAddrs(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	out := fields[:0]
	for _, f := range fields {
		if strings.Contains(f, "@") {
			out = append(out, f)
		}
	}
	return out
}
