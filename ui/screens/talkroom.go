package screens

import (
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// ttlOption is one selectable message lifetime.
type ttlOption struct {
	label string
	dur   time.Duration
}

// ttlOptions are the offered burn-after-read timers (start counting when the
// recipient reads the message); the server clamps to [5s, 3600s]. The default
// selection is 5 minutes (defaultTTLIndex) — safe but usable.
var ttlOptions = []ttlOption{
	{"5s", 5 * time.Second},
	{"1m", time.Minute},
	{"5m", 5 * time.Minute},
	{"15m", 15 * time.Minute},
	{"30m", 30 * time.Minute},
	{"1h", time.Hour},
}

// defaultTTLIndex selects the "5m" default in ttlOptions.
const defaultTTLIndex = 2

// TalkRoom renders one conversation: the message stream with per-message
// ephemeral countdowns, a verification header, and the compose bar with a
// TTL selector and live/store toggle.
type TalkRoom struct {
	list      layout.List
	backBtn   widget.Clickable
	shieldBtn widget.Clickable
	input     widget.Editor
	sendBtn   widgets.Button
	ttlBtn    widget.Clickable

	liveBtn   widget.Clickable
	msgClicks map[string]*widget.Clickable
	ttlIndex  int
	live      bool
}

// NewTalkRoom constructs the room screen.
func NewTalkRoom() *TalkRoom {
	r := &TalkRoom{
		list:      layout.List{Axis: layout.Vertical},
		msgClicks: map[string]*widget.Clickable{},
		ttlIndex:  defaultTTLIndex,
	}
	r.input.Submit = true
	return r
}

// clickFor returns the (stable) clickable for a message id.
func (s *TalkRoom) clickFor(id string) *widget.Clickable {
	c := s.msgClicks[id]
	if c == nil {
		c = &widget.Clickable{}
		s.msgClicks[id] = c
	}
	return c
}

// Layout renders the room.
func (s *TalkRoom) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	chat := env.State.Chat
	var snap state.ChatSnapshot
	if chat != nil {
		snap = chat.Snapshot()
	}
	peer := snap.ActivePeer

	if s.backBtn.Clicked(gtx) {
		if chat != nil {
			chat.CloseConversation()
		}
		env.Nav.Pop(gtx.Now)
	}
	if s.shieldBtn.Clicked(gtx) {
		env.Nav.Push(state.ScreenTalkVerify, gtx.Now)
	}
	s.handleSend(gtx, env, peer)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.header(gtx, th, snap)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(snap.Messages) == 0 {
				return emptyState(gtx, th, widgets.IconLock, true,
					"Say hello.", "Messages vanish once read or when their timer ends.")
			}
			return s.layoutMessages(gtx, env, snap)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.composeBar(gtx, th)
		}))
}

// header shows the peer and a tappable shield reflecting verification.
func (s *TalkRoom) header(gtx layout.Context, th *theme.Theme, snap state.ChatSnapshot) layout.Dimensions {
	shield := func(gtx layout.Context) layout.Dimensions {
		col := th.Palette.Warning
		icon := widgets.IconLock
		if snap.Verified {
			col = th.Palette.Success
			icon = widgets.IconShield
		}
		return widgets.IconButton(gtx, th, &s.shieldBtn, icon, col)
	}
	return topBar(gtx, th,
		iconBtn(th, &s.backBtn, widgets.IconBack, th.Palette.OnBackground),
		snap.ActivePeer,
		shield)
}

// layoutMessages draws the message stream, requesting frames while any
// message still has a live countdown.
func (s *TalkRoom) layoutMessages(gtx layout.Context, env *Env, snap state.ChatSnapshot) layout.Dimensions {
	th := env.Theme
	animating := false
	dims := s.list.Layout(gtx, len(snap.Messages), func(gtx layout.Context, i int) layout.Dimensions {
		m := snap.Messages[i]
		d, live := s.bubble(gtx, env, th, m)
		if live {
			animating = true
		}
		return d
	})
	if animating {
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

// composeBar is the bottom input row: text field, TTL selector, live/store
// toggle, and the gradient send button.
func (s *TalkRoom) composeBar(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if s.ttlBtn.Clicked(gtx) {
		s.ttlIndex = (s.ttlIndex + 1) % len(ttlOptions)
	}
	if s.liveBtn.Clicked(gtx) {
		s.live = !s.live
	}
	// Timer pill label: the clock timer when off, a "🔥 Live" badge when on.
	timerIcon := widgets.IconClock
	timerLabel := ttlOptions[s.ttlIndex].label + " · read"
	if s.live {
		timerIcon = widgets.IconClock
		timerLabel = "Live · burns on read"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widgets.Separator(gtx, th, 0)
		}),
		// Options row: burn-after-read timer selector + Live toggle.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.MD, Right: theme.MD, Top: theme.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.pillToggle(gtx, th, &s.ttlBtn, timerIcon, timerLabel)
						}),
						layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.livePill(gtx, th)
						}))
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.MD, Right: theme.MD, Top: theme.XS, Bottom: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return s.editor(gtx, th)
						}),
						layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.sendBtn.Layout(gtx, th, widgets.ButtonPrimary, "Send", false,
								len(trimmed(s.input.Text())) == 0)
						}))
				})
		}))
}

// livePill draws the Live-mode toggle: subtle when off, warning-filled when on.
// Live mode stores nothing on the server and burns the message the moment it is
// read (both parties must be online).
func (s *TalkRoom) livePill(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	fill := th.Palette.AccentSubtle
	fg := th.Palette.Subtle
	label := "Live off"
	if s.live {
		fill = th.Palette.Warning
		fg = th.Palette.OnAccent
		label = "Live on"
	}
	return s.liveBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				r := gtx.Dp(theme.PillRadius)
				sz := gtx.Constraints.Min
				if sz.Y < 2*r {
					r = sz.Y / 2
				}
				defer clip.UniformRRect(image.Rectangle{Max: sz}, r).Push(gtx.Ops).Pop()
				return widgets.Fill(gtx, fill)
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: theme.SM, Right: theme.SM, Top: theme.XS, Bottom: theme.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Caption, fg, label, 1)
					})
			})
	})
}

// editor draws the message input with a hint.
func (s *TalkRoom) editor(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	if s.input.Len() == 0 {
		th.Label(gtx, theme.Body, th.Palette.Subtle, "Message", 1)
	}
	return s.input.Layout(gtx, th.Shaper,
		font.Font{Weight: theme.Body.Weight}, theme.Body.Size,
		theme.ColorOp(gtx, th.Palette.OnBackground),
		theme.ColorOp(gtx, th.Palette.AccentSubtle))
}

// handleSend fires a send from either the button or the editor's submit.
func (s *TalkRoom) handleSend(gtx layout.Context, env *Env, peer string) {
	send := false
	for {
		ev, ok := s.input.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			send = true
		}
	}
	if s.sendBtn.Clicked(gtx) {
		send = true
	}
	text := trimmed(s.input.Text())
	if !send || text == "" || peer == "" {
		return
	}
	if env.State.Chat != nil {
		// store: delivered live when the peer is connected, otherwise queued for
		// their next connect. live: never stored, delivered only if they're online.
		mode := "store"
		if s.live {
			mode = "live"
		}
		env.State.Chat.SendMessage(peer, text, ttlOptions[s.ttlIndex].dur, mode)
	}
	s.input.SetText("")
}
