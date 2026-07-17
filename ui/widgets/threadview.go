package widgets

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
)

// MessageBody is a message's body pre-parsed for display: the HTML/quote
// work that used to run per message PER FRAME (HTML tokenization, quote
// splitting, attachment JSON decode) is done once when the thread loads
// and handed to the view. See ParseMessageBody.
type MessageBody struct {
	Visible     string
	Quoted      string
	Attachments []mime.AttachmentRef
	HasBody     bool
}

// ParseMessageBody derives a message's display body once, off the frame
// loop. The thread view renders the result verbatim.
func ParseMessageBody(m store.Message) MessageBody {
	body := mime.DisplayText(m.BodyText, m.BodyHTML)
	visible, quoted := splitQuoted(body)
	mb := MessageBody{Visible: visible, Quoted: quoted, HasBody: visible != "" || quoted != ""}
	if m.Attachments != "" {
		var refs []mime.AttachmentRef
		if err := json.Unmarshal([]byte(m.Attachments), &refs); err == nil {
			mb.Attachments = refs
		}
	}
	return mb
}

// ThreadView renders a conversation: every message expanded, quoted
// history folded behind a per-message toggle, PGP status inline.
type ThreadView struct {
	list    layout.List
	toggles []widget.Clickable
	shown   map[int64]bool // message ID -> quoted text expanded

	// detailBtns/details drive the per-message header disclosure: the
	// full addressing, security, and size panel.
	detailBtns []widget.Clickable
	details    map[int64]bool

	attachClicks map[int64][]widget.Clickable
	requests     []DownloadRequest
}

// DownloadRequest identifies one attachment the user tapped this frame.
type DownloadRequest struct {
	MessageID int64
	Index     int
}

// NewThreadView constructs an empty thread view.
func NewThreadView() *ThreadView {
	return &ThreadView{
		list:         layout.List{Axis: layout.Vertical},
		shown:        make(map[int64]bool),
		details:      make(map[int64]bool),
		attachClicks: make(map[int64][]widget.Clickable),
	}
}

// DownloadRequests drains the attachment taps collected this frame.
func (tv *ThreadView) DownloadRequests() []DownloadRequest {
	out := tv.requests
	tv.requests = nil
	return out
}

// Layout renders the messages oldest-first. bodies carries each message's
// pre-parsed display body keyed by message ID (see ParseMessageBody); a
// message missing from the map is parsed inline as a fallback so the view
// is never wrong, only slower.
func (tv *ThreadView) Layout(gtx layout.Context, th *theme.Theme, msgs []store.Message, bodies map[int64]MessageBody) layout.Dimensions {
	if len(tv.toggles) < len(msgs) {
		grow := len(msgs) - len(tv.toggles)
		tv.toggles = append(tv.toggles, make([]widget.Clickable, grow)...)
		tv.detailBtns = append(tv.detailBtns, make([]widget.Clickable, grow)...)
	}
	tv.requests = tv.requests[:0]
	return tv.list.Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
		msg := msgs[i]
		mb, ok := bodies[msg.ID]
		if !ok {
			mb = ParseMessageBody(msg)
		}
		return tv.message(gtx, th, &tv.toggles[i], &tv.detailBtns[i], msg, mb)
	})
}

func (tv *ThreadView) message(gtx layout.Context, th *theme.Theme, toggle, dBtn *widget.Clickable, msg store.Message, mb MessageBody) layout.Dimensions {
	if toggle.Clicked(gtx) {
		tv.shown[msg.ID] = !tv.shown[msg.ID]
	}
	if dBtn.Clicked(gtx) {
		tv.details[msg.ID] = !tv.details[msg.ID]
	}
	visible, quoted := mb.Visible, mb.Quoted
	showQuoted := tv.shown[msg.ID]

	return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.MD, Bottom: theme.MD}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Header: avatar, sender, date — tap for full details.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return dBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return tv.header(gtx, th, msg, tv.details[msg.ID], gtx.Now)
					})
				}),
				// Full addressing + security panel (the Gmail-style
				// "details" disclosure).
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !tv.details[msg.ID] {
						return layout.Dimensions{}
					}
					return tv.detailsPanel(gtx, th, msg)
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
				// Tracking indicator: honesty about surveillance mail.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !msg.HasTrackers {
						return layout.Dimensions{}
					}
					return layout.Inset{Bottom: theme.SM}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Caption, th.Palette.Destructive,
								"This sender tracks opens — trackers blocked", 1)
						})
				}),
				// Attachments: one chip per file, tap to download.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return tv.attachmentChips(gtx, th, msg.ID, mb.Attachments)
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

func (tv *ThreadView) header(gtx layout.Context, th *theme.Theme, msg store.Message, expanded bool, now time.Time) layout.Dimensions {
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
				RelativeTime(msg.Date, now), 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			icon := IconChevronDown
			if expanded {
				icon = IconChevronRight
			}
			return layout.Inset{Left: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return DrawIcon(gtx, icon, th.Palette.Subtle, 16)
				})
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

// attachmentChips renders a tappable row per attachment, recording taps
// as download requests. refs are pre-parsed (ParseMessageBody), so this
// does no JSON work per frame.
func (tv *ThreadView) attachmentChips(gtx layout.Context, th *theme.Theme, msgID int64, refs []mime.AttachmentRef) layout.Dimensions {
	if len(refs) == 0 {
		return layout.Dimensions{}
	}
	clicks := tv.attachClicks[msgID]
	if len(clicks) < len(refs) {
		clicks = append(clicks, make([]widget.Clickable, len(refs)-len(clicks))...)
		tv.attachClicks[msgID] = clicks
	}
	children := make([]layout.FlexChild, 0, len(refs))
	for i, ref := range refs {
		i, ref := i, ref
		if clicks[i].Clicked(gtx) {
			tv.requests = append(tv.requests, DownloadRequest{MessageID: msgID, Index: i})
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return clicks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return DrawIcon(gtx, IconDownload, th.Palette.Accent, 14)
							}),
							layout.Rigid(layout.Spacer{Width: theme.XS}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := ref.Filename
								if label == "" {
									label = fmt.Sprintf("attachment %d", i+1)
								}
								return th.Label(gtx, theme.Caption, th.Palette.Accent, label, 1)
							}))
					})
			})
		}))
	}
	return layout.Inset{Bottom: theme.SM}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
}
