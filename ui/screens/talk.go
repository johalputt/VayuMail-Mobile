package screens

import (
	"image"
	"image/color"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Talk is the VayuTalk conversation list: verified/online status per
// contact, unread counts, and a "New chat" affordance. It carries no
// message previews — the messages are ephemeral and never surface here.
type Talk struct {
	list       layout.List
	backBtn    widget.Clickable
	fab        *widgets.FAB
	convClicks []widget.Clickable
	entrance   time.Time
	entered    bool

	// New-chat composer (inline sheet).
	composing bool
	newField  *widgets.TextField
	startBtn  widgets.Button
	cancelBtn widget.Clickable
}

// NewTalk constructs the conversation-list screen.
func NewTalk() *Talk {
	return &Talk{
		list:     layout.List{Axis: layout.Vertical},
		fab:      &widgets.FAB{},
		newField: widgets.NewTextField(false),
	}
}

// Layout renders the conversation list.
func (s *Talk) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	chat := env.State.Chat
	var snap state.ChatSnapshot
	if chat != nil {
		snap = chat.Snapshot()
	}

	if s.backBtn.Clicked(gtx) {
		env.Nav.Pop(gtx.Now)
	}
	if s.fab.Clicked(gtx) {
		s.composing = true
		s.newField.SetText("")
	}
	s.handleNewChat(gtx, env)

	if !s.entered {
		s.entrance = gtx.Now
		s.entered = true
	}

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.backBtn, widgets.IconBack, th.Palette.OnBackground),
				"VayuTalk",
				s.statusDot(th, snap.Online))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(snap.Conversations) == 0 {
				return emptyState(gtx, th, widgets.IconChat, true,
					"No conversations yet.",
					"Ephemeral, end-to-end encrypted. Tap + to start one.")
			}
			return s.layoutList(gtx, env, snap)
		}))

	// New-chat sheet floats above the list when active.
	if s.composing {
		s.layoutNewChat(gtx, env)
	} else {
		layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(theme.FABMargin).Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return s.fab.Layout(gtx, th, widgets.IconCompose)
				})
		})
	}
	return dims
}

// layoutList draws the conversation rows with a staggered entrance.
func (s *Talk) layoutList(gtx layout.Context, env *Env, snap state.ChatSnapshot) layout.Dimensions {
	th := env.Theme
	if len(s.convClicks) < len(snap.Conversations) {
		s.convClicks = append(s.convClicks,
			make([]widget.Clickable, len(snap.Conversations)-len(s.convClicks))...)
	}
	return s.list.Layout(gtx, len(snap.Conversations), func(gtx layout.Context, i int) layout.Dimensions {
		c := snap.Conversations[i]
		click := &s.convClicks[i]
		if click.Clicked(gtx) {
			if env.State.Chat != nil {
				env.State.Chat.OpenConversation(c.Peer)
			}
			env.Nav.Push(state.ScreenTalkRoom, gtx.Now)
		}
		t, done := animStagger(gtx.Now, s.entrance, i)
		if !done {
			gtx.Execute(op.InvalidateCmd{})
		}
		return fadeRise(gtx, t, func(gtx layout.Context) layout.Dimensions {
			return s.convRow(gtx, th, click, c, snap.Online)
		})
	})
}

// convRow draws one conversation: avatar with online dot, peer address,
// verified badge, unread count, and last-activity time.
func (s *Talk) convRow(gtx layout.Context, th *theme.Theme, click *widget.Clickable, c state.ChatConversation, online bool) layout.Dimensions {
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.MD, Bottom: theme.MD}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.avatarWithDot(gtx, th, c.Peer, online)
					}),
					layout.Rigid(layout.Spacer{Width: theme.MD}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return th.Label(gtx, theme.BodyStrong, th.Palette.OnBackground, c.Peer, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if !c.Verified {
											return layout.Dimensions{}
										}
										return layout.Inset{Left: theme.XS}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												return widgets.DrawIcon(gtx, widgets.IconShield, th.Palette.Success, 14)
											})
									}))
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := "End-to-end encrypted"
								col := th.Palette.Subtle
								if !c.Verified {
									label = "Unverified — compare safety number"
									col = th.Palette.Warning
								}
								return th.Label(gtx, theme.Caption, col, label, 1)
							}))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if c.LastActivity.IsZero() {
									return layout.Dimensions{}
								}
								return th.Label(gtx, theme.Caption, th.Palette.Subtle,
									widgets.RelativeTime(c.LastActivity, time.Now()), 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if c.Unread == 0 {
									return layout.Dimensions{}
								}
								return layout.Inset{Top: theme.XS}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										return unreadPill(gtx, th, c.Unread)
									})
							}))
					}))
			})
	})
}

// avatarWithDot draws the peer avatar with a presence dot overlaid at its
// bottom-right when the stream is online.
func (s *Talk) avatarWithDot(gtx layout.Context, th *theme.Theme, peer string, online bool) layout.Dimensions {
	dims := widgets.Avatar(gtx, th, "", peer)
	if !online {
		return dims
	}
	d := gtx.Dp(10)
	ring := gtx.Dp(13)
	// A background ring behind the dot so it reads on any avatar color.
	roff := dims.Size.X - ring
	drawDot(gtx, image.Pt(roff, roff), ring, th.Palette.Background)
	doff := dims.Size.X - ring + (ring-d)/2
	drawDot(gtx, image.Pt(doff, doff), d, th.Palette.Success)
	return dims
}

// statusDot is the header trailing widget: a small online/offline pip.
func (s *Talk) statusDot(th *theme.Theme, online bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		target := gtx.Dp(theme.TouchTarget)
		gtx.Constraints = layout.Exact(image.Pt(target, target))
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			c := th.Palette.Subtle
			if online {
				c = th.Palette.Success
			}
			d := gtx.Dp(8)
			drawDot(gtx, image.Point{}, d, c)
			return layout.Dimensions{Size: image.Pt(d, d)}
		})
	}
}

// drawDot fills a circle of diameter d whose top-left sits at off.
func drawDot(gtx layout.Context, off image.Point, d int, c color.NRGBA) {
	defer op.Offset(off).Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, c, clip.Ellipse{Max: image.Pt(d, d)}.Op(gtx.Ops))
}

// unreadPill renders a small accent count badge.
func unreadPill(gtx layout.Context, th *theme.Theme, count int) layout.Dimensions {
	label := "99+"
	if count <= 99 {
		label = itoa(count)
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			r := size.Y / 2
			defer clip.UniformRRect(image.Rectangle{Max: size}, r).Push(gtx.Ops).Pop()
			return widgets.FillGradient(gtx, th.Palette.Accent, th.Palette.AccentAlt)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.SM, Right: theme.SM, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Micro, th.Palette.OnAccent, label, 1)
				})
		})
}

// animStagger reports the cascaded entrance progress for row i.
func animStagger(now, start time.Time, i int) (float32, bool) {
	return anim.Stagger(now, start, i, 28*time.Millisecond, 260*time.Millisecond, anim.OutCubic)
}

// itoa is a tiny non-allocating-ish integer formatter for small counts.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 && i > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
