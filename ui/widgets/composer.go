package widgets

import (
	"fmt"
	"strings"
	"sync"

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
	// EncryptRequested is true when the user just turned encryption on —
	// the screen fetches any missing recipient keys in response.
	EncryptRequested bool
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
	sendBtn   Button

	// Encrypt and Sign are the PGP toggles (off = Subtle, on = Accent).
	Encrypt bool
	Sign    bool

	// HasKey reports whether the keyring holds a key for an address; set
	// by the root so the action row can show live per-recipient key
	// status. Nil disables the readout.
	HasKey func(addr string) bool

	// attachments is guarded because AddAttachment is called from the file-
	// picker goroutine while the UI thread reads/removes on layout. attachDel
	// holds one tap-target per attachment (UI thread only); tapping a chip
	// removes that file.
	mu          sync.Mutex
	attachments []smtpsend.Attachment
	attachDel   []widget.Clickable
}

// AddAttachment appends a file to the draft. Safe to call from any goroutine.
func (c *Composer) AddAttachment(filename, contentType string, data []byte) {
	c.mu.Lock()
	c.attachments = append(c.attachments, smtpsend.Attachment{
		Filename:    filename,
		ContentType: contentType,
		Data:        data,
	})
	c.mu.Unlock()
}

// attachmentsCopy returns a snapshot of the current attachments.
func (c *Composer) attachmentsCopy() []smtpsend.Attachment {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]smtpsend.Attachment, len(c.attachments))
	copy(out, c.attachments)
	return out
}

func (c *Composer) removeAttachment(i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i >= 0 && i < len(c.attachments) {
		c.attachments = append(c.attachments[:i], c.attachments[i+1:]...)
	}
	if i >= 0 && i < len(c.attachDel) {
		c.attachDel = append(c.attachDel[:i], c.attachDel[i+1:]...)
	}
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
	c.mu.Lock()
	c.attachments = nil
	c.attachDel = nil
	c.mu.Unlock()
}

// Layout renders the composer and reports actions.
func (c *Composer) Layout(gtx layout.Context, th *theme.Theme) ComposerAction {
	var action ComposerAction
	if c.ccToggle.Clicked(gtx) {
		c.showCcBcc = !c.showCcBcc
	}
	if c.encToggle.Clicked(gtx) {
		c.Encrypt = !c.Encrypt
		if c.Encrypt {
			action.EncryptRequested = true
		}
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
		// Attached files (tap a chip to remove it), above the action row.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.drawAttachments(gtx, th)
		}),
		// Action row pinned to the bottom.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.actionRow(gtx, th)
		}))
	return action
}

// drawAttachments renders one chip per attached file; tapping a chip removes
// it. Returns empty dimensions when there are no attachments.
func (c *Composer) drawAttachments(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	c.mu.Lock()
	n := len(c.attachments)
	for len(c.attachDel) < n {
		c.attachDel = append(c.attachDel, widget.Clickable{})
	}
	names := make([]string, n)
	sizes := make([]int, n)
	for i := range c.attachments {
		names[i] = c.attachments[i].Filename
		sizes[i] = len(c.attachments[i].Data)
	}
	c.mu.Unlock()
	if n == 0 {
		return layout.Dimensions{}
	}

	remove := -1
	for i := 0; i < n && i < len(c.attachDel); i++ {
		if c.attachDel[i].Clicked(gtx) {
			remove = i
		}
	}

	children := make([]layout.FlexChild, 0, n)
	for i := 0; i < n; i++ {
		i := i
		label := names[i] + "  ·  " + humanSize(sizes[i]) + "   ✕"
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return c.attachDel[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Right: theme.SM}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return DrawIcon(gtx, IconAttach, th.Palette.Subtle, 18)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, th.Palette.OnBackground, label, 1)
							}))
					})
				})
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)

	if remove >= 0 {
		c.removeAttachment(remove)
	}
	return dims
}

// humanSize renders a byte count as a short human string (e.g. "1.2 MB").
func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
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

// actionRow is the single bottom bar: attach, PGP encrypt, PGP sign, a
// live security readout, and the gradient Send pill. No toolbar
// anywhere else.
func (c *Composer) actionRow(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return Separator(gtx, th, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.MD, Right: theme.MD, Top: theme.SM, Bottom: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return IconButton(gtx, th, &c.attachBtn, IconAttach, th.Palette.Subtle)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							col := th.Palette.Subtle
							if c.Encrypt {
								col = th.Palette.Success
							}
							return IconButton(gtx, th, &c.encToggle, IconShield, col)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							col := th.Palette.Subtle
							if c.Sign {
								col = th.Palette.Success
							}
							return IconButton(gtx, th, &c.sigToggle, IconSignature, col)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := securityLabel(c.Encrypt, c.Sign)
							col := th.Palette.Success
							if c.Encrypt {
								if n := c.missingKeyCount(); n > 0 {
									label = fmt.Sprintf("%d recipient(s) missing a key", n)
									col = th.Palette.Warning
								}
							}
							if label == "" {
								return layout.Dimensions{}
							}
							return layout.Inset{Left: theme.XS}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Micro, col, label, 1)
								})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: gtx.Constraints.Min}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.sendBtn.Layout(gtx, th, ButtonPrimary, "Send", false,
								len(splitAddrs(c.to.Text())) == 0)
						}))
				})
		}))
}

// securityLabel names the active PGP protections.
func securityLabel(encrypt, sign bool) string {
	switch {
	case encrypt && sign:
		return "Encrypted · Signed"
	case encrypt:
		return "Encrypted"
	case sign:
		return "Signed"
	default:
		return ""
	}
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
