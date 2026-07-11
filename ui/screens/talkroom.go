package screens

import (
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
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

// ttlOptions are the offered lifetimes; the server clamps to [60s, 3600s].
var ttlOptions = []ttlOption{
	{"5m", 5 * time.Minute},
	{"30m", 30 * time.Minute},
	{"1h", time.Hour},
}

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
	modeBtn   widget.Clickable

	msgClicks map[string]*widget.Clickable
	ttlIndex  int
	storeMode bool // false = live, true = store-and-forward
}

// NewTalkRoom constructs the room screen.
func NewTalkRoom() *TalkRoom {
	r := &TalkRoom{
		list:      layout.List{Axis: layout.Vertical},
		msgClicks: map[string]*widget.Clickable{},
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
	if s.modeBtn.Clicked(gtx) {
		s.storeMode = !s.storeMode
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widgets.Separator(gtx, th, 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.MD, Right: theme.MD, Top: theme.SM, Bottom: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.pillToggle(gtx, th, &s.ttlBtn, widgets.IconClock, ttlOptions[s.ttlIndex].label)
						}),
						layout.Rigid(layout.Spacer{Width: theme.XS}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label, icon := "Live", widgets.IconSend
							if s.storeMode {
								label, icon = "Store", widgets.IconClock
							}
							return s.pillToggle(gtx, th, &s.modeBtn, icon, label)
						}),
						layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
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
		env.State.Chat.SendMessage(peer, text, ttlOptions[s.ttlIndex].dur, s.sendModeName())
	}
	s.input.SetText("")
}

// sendModeName maps the toggle to the engine's mode string.
func (s *TalkRoom) sendModeName() string {
	if s.storeMode {
		return "store"
	}
	return "live"
}
