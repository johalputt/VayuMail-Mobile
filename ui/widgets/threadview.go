package widgets

import (
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// ThreadView renders a conversation: every message expanded, quoted
// history folded behind a per-message toggle, PGP status inline.
type ThreadView struct {
	list    layout.List
	toggles []widget.Clickable
	shown   map[int64]bool // message ID -> quoted text expanded
}

// NewThreadView constructs an empty thread view.
func NewThreadView() *ThreadView {
	return &ThreadView{
		list:  layout.List{Axis: layout.Vertical},
		shown: make(map[int64]bool),
	}
}

// Layout renders the messages oldest-first.
func (tv *ThreadView) Layout(gtx layout.Context, th *theme.Theme, msgs []store.Message) layout.Dimensions {
	if len(tv.toggles) < len(msgs) {
		tv.toggles = append(tv.toggles, make([]widget.Clickable, len(msgs)-len(tv.toggles))...)
	}
	return tv.list.Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
		return tv.message(gtx, th, &tv.toggles[i], msgs[i])
	})
}

func (tv *ThreadView) message(gtx layout.Context, th *theme.Theme, toggle *widget.Clickable, msg store.Message) layout.Dimensions {
	if toggle.Clicked(gtx) {
		tv.shown[msg.ID] = !tv.shown[msg.ID]
	}
	body := mime.DisplayText(msg.BodyText, msg.BodyHTML)
	visible, quoted := splitQuoted(body)
	showQuoted := tv.shown[msg.ID]

	return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.MD, Bottom: theme.MD}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Header: avatar, sender, date.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return tv.header(gtx, th, msg)
				}),
				layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
				// PGP status, when present.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if msg.PGPStatus == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Bottom: theme.SM}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return DrawIcon(gtx, IconShield, th.Palette.Accent, 14)
								}),
								layout.Rigid(layout.Spacer{Width: theme.XS}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Caption, th.Palette.Accent,
										pgpLabel(msg.PGPStatus), 1)
								}))
						})
				}),
				// Body text.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if visible == "" && quoted == "" {
						return th.Label(gtx, theme.Body, th.Palette.Subtle,
							"(message body not downloaded yet)", 0)
					}
					return th.Label(gtx, theme.Body, th.Palette.OnBackground, visible, 0)
				}),
				// Quoted-text fold.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if quoted == "" {
						return layout.Dimensions{}
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return toggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								label := "Show quoted text"
								if showQuoted {
									label = "Hide quoted text"
								}
								return layout.Inset{Top: theme.SM, Bottom: theme.SM}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										return th.Label(gtx, theme.Caption, th.Palette.Accent, label, 1)
									})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !showQuoted {
								return layout.Dimensions{}
							}
							return th.Label(gtx, theme.Body, th.Palette.Subtle, quoted, 0)
						}))
				}),
				layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return Separator(gtx, th, 0)
				}))
		})
}

func (tv *ThreadView) header(gtx layout.Context, th *theme.Theme, msg store.Message) layout.Dimensions {
	sender := msg.FromName
	if sender == "" {
		sender = msg.FromAddr
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return Avatar(gtx, th, msg.FromName, msg.FromAddr)
		}),
		layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.BodyStrong, th.Palette.OnBackground, sender, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Caption, th.Palette.Subtle, msg.FromAddr, 1)
				}))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.Caption, th.Palette.Subtle,
				RelativeTime(msg.Date, time.Now()), 1)
		}))
}

// splitQuoted separates a plain-text body into new content and quoted
// history (the trailing run of "> " lines plus its attribution line).
func splitQuoted(body string) (visible, quoted string) {
	lines := strings.Split(body, "\n")
	// Find the first line from which everything to the end is quotation
	// or blank.
	start := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || mime.QuoteDepth(lines[i]) > 0 {
			start = i
			continue
		}
		// An attribution line ("On ..., X wrote:") directly above the
		// quote belongs to the quoted block.
		if start == i+1 && strings.HasSuffix(trimmed, ":") &&
			(strings.HasPrefix(trimmed, "On ") || strings.HasPrefix(trimmed, "Am ")) {
			start = i
		}
		break
	}
	if start >= len(lines) {
		return body, ""
	}
	visible = strings.TrimSpace(strings.Join(lines[:start], "\n"))
	quoted = strings.TrimSpace(strings.Join(lines[start:], "\n"))
	if visible == "" {
		// All-quote message: show it rather than an empty body.
		return quoted, ""
	}
	return visible, quoted
}

func pgpLabel(status string) string {
	switch status {
	case "signed":
		return "Signed"
	case "encrypted":
		return "Encrypted"
	case "signed+encrypted":
		return "Signed & encrypted"
	default:
		return status
	}
}
