package screens

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// bubble draws one message. It reports live=true while the message still
// has a running countdown, so the caller keeps requesting frames.
func (s *TalkRoom) bubble(gtx layout.Context, env *Env, th *theme.Theme, m state.ChatMessage) (layout.Dimensions, bool) {
	now := gtx.Now
	expired := !m.ExpiresAt.IsZero() && !now.Before(m.ExpiresAt)
	tombstone := expired || m.Status == state.MsgExpired

	switch {
	case tombstone:
		return s.tombstone(gtx, th, m.Self), false
	case !m.Self && !m.Revealed:
		click := s.clickFor(m.ID)
		if click.Clicked(gtx) && env.State.Chat != nil {
			env.State.Chat.RevealMessage(m.Peer, m.ID)
		}
		return s.sealedBubble(gtx, th, click), false
	default:
		// The burn ring runs from when the message was READ (ArmedAt), not when it
		// was sent — an un-armed message (sent, not yet read) shows no ring.
		frac := widgets.RemainingFraction(m.ArmedAt, m.ExpiresAt, now)
		live := !m.ExpiresAt.IsZero() && frac > 0
		return s.contentBubble(gtx, th, m, frac), live
	}
}

// flexSpacer fills the flexible space its Flex allocates. This MUST return the
// allocated size, not layout.Dimensions{}: Gio's Flex advances the layout cursor
// by each child's RETURNED size, so a zero-size Flexed child collapses and a
// bubble meant to be pushed to the far side lands back at the near side — the
// cause of "sent and received messages both showing on the left".
func flexSpacer(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

// bubbleRow aligns a bubble to the correct side (sent → right, received → left)
// and caps its width so a long message never spans the full column.
func bubbleRow(gtx layout.Context, self bool, inner layout.Widget) layout.Dimensions {
	max := gtx.Constraints.Max.X * 78 / 100
	capped := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = max
		return inner(gtx)
	}
	return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			if self {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, flexSpacer),
					layout.Rigid(capped))
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(capped),
				layout.Flexed(1, flexSpacer))
		})
}

// fillBubble paints the rounded bubble background.
func fillBubble(gtx layout.Context, fill color.NRGBA) layout.Dimensions {
	r := gtx.Dp(theme.CardRadius)
	defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, r).Push(gtx.Ops).Pop()
	return widgets.Fill(gtx, fill)
}

// contentBubble draws a revealed/outgoing message with its text and a meta
// row carrying the countdown ring, remaining time, and status.
func (s *TalkRoom) contentBubble(gtx layout.Context, th *theme.Theme, m state.ChatMessage, frac float32) layout.Dimensions {
	bg := th.Palette.Surface
	fg := th.Palette.OnBackground
	meta := th.Palette.Subtle
	if m.Self {
		bg = th.Palette.Accent
		fg = th.Palette.OnAccent
		meta = theme.WithAlpha(th.Palette.OnAccent, 0xCC)
	}
	return bubbleRow(gtx, m.Self, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return fillBubble(gtx, bg) },
			func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(theme.SM+theme.XS).Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								// Audit H7: a peer message not validly signed by this contact
								// (or, in a Verified conversation, signed by the wrong key) may be
								// a relay impersonation — warn inline so it is never read as an
								// authentic message from the contact.
								txt := m.Text
								if m.Unauthenticated && !m.Self {
									txt = "⚠ Unverified sender — not cryptographically signed by this contact:\n" + m.Text
								}
								return th.Label(gtx, theme.Body, fg, txt, 0)
							}),
							layout.Rigid(layout.Spacer{Height: theme.XS}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.metaRow(gtx, th, m, frac, meta)
							}))
					})
			})
	})
}

// metaRow renders the countdown ring, remaining time, and status text.
func (s *TalkRoom) metaRow(gtx layout.Context, th *theme.Theme, m state.ChatMessage, frac float32, meta color.NRGBA) layout.Dimensions {
	track := theme.WithAlpha(meta, 0x40)
	arc := meta
	if !m.Self {
		arc = th.Palette.Accent
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		// Clock time (the server's send time, shown in this device's local zone),
		// so a bubble reads the same wall-clock time as the same message on the
		// web — not the device's receive time.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if m.CreatedAt.IsZero() {
				return layout.Dimensions{}
			}
			return layout.Inset{Right: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Micro, meta, m.CreatedAt.Local().Format("15:04"), 1)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if m.ExpiresAt.IsZero() {
				return layout.Dimensions{}
			}
			return widgets.CountdownRing(gtx, 14, frac, track, arc)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := widgets.FormatRemaining(m.ExpiresAt, gtx.Now)
			if label == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Micro, meta, label, 1)
				})
		}),
		layout.Flexed(1, flexSpacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := statusLabel(m)
			if label == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Micro, meta, label, 1)
				})
		}))
}

// statusLabel names an outgoing message's delivery state.
func statusLabel(m state.ChatMessage) string {
	if !m.Self {
		return ""
	}
	switch m.Status {
	case state.MsgSending:
		return "Sending…"
	case state.MsgSent:
		return "Sent"
	case state.MsgQueued:
		return "Queued"
	case state.MsgRead:
		return "Read · burning"
	default:
		return ""
	}
}

// sealedBubble is the covered peer message: tap to reveal (and destroy).
func (s *TalkRoom) sealedBubble(gtx layout.Context, th *theme.Theme, click *widget.Clickable) layout.Dimensions {
	return bubbleRow(gtx, false, func(gtx layout.Context) layout.Dimensions {
		return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return fillBubble(gtx, th.Palette.AccentSubtle) },
				func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(theme.SM+theme.XS).Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return widgets.DrawIcon(gtx, widgets.IconLock, th.Palette.Accent, 16)
								}),
								layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Body, th.Palette.Accent, "Tap to reveal · then it burns", 1)
								}))
						})
				})
		})
	})
}

// tombstone is the collapsed remnant of a read or expired message.
func (s *TalkRoom) tombstone(gtx layout.Context, th *theme.Theme, self bool) layout.Dimensions {
	return bubbleRow(gtx, self, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return fillBubble(gtx, theme.WithAlpha(th.Palette.Subtle, 0x1F))
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(theme.SM).Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Caption, th.Palette.Subtle, "message deleted", 1)
					})
			})
	})
}

// pillToggle draws a small tappable status pill (TTL / mode selectors).
func (s *TalkRoom) pillToggle(gtx layout.Context, th *theme.Theme, click *widget.Clickable, icon widgets.Icon, label string) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				r := gtx.Dp(theme.PillRadius)
				sz := gtx.Constraints.Min
				if sz.Y < 2*r {
					r = sz.Y / 2
				}
				defer clip.UniformRRect(image.Rectangle{Max: sz}, r).Push(gtx.Ops).Pop()
				return widgets.Fill(gtx, th.Palette.AccentSubtle)
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: theme.SM, Right: theme.SM, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return widgets.DrawIcon(gtx, icon, th.Palette.Accent, 14)
							}),
							layout.Rigid(layout.Spacer{Width: theme.XS}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, th.Palette.Accent, label, 1)
							}))
					})
			})
	})
}

// trimmed is strings.TrimSpace, named for readability at call sites.
func trimmed(s string) string { return strings.TrimSpace(s) }
