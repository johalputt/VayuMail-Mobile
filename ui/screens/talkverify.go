package screens

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// TalkVerify shows a peer's safety number and lets the user record an
// out-of-band verification. The number is derived from the peer's public
// key fingerprint; comparing it over a trusted channel defeats a
// man-in-the-middle key swap.
type TalkVerify struct {
	backBtn   widget.Clickable
	markBtn   widgets.Button
	revokeBtn widget.Clickable
	fetchBtn  widgets.Button
	list      layout.List
}

// NewTalkVerify constructs the verification screen.
func NewTalkVerify() *TalkVerify {
	return &TalkVerify{list: layout.List{Axis: layout.Vertical}}
}

// Layout renders the safety-number panel.
func (s *TalkVerify) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	chat := env.State.Chat
	var snap state.ChatSnapshot
	if chat != nil {
		snap = chat.Snapshot()
	}
	peer := snap.ActivePeer

	if s.backBtn.Clicked(gtx) {
		env.Nav.Pop(gtx.Now)
	}
	if s.markBtn.Clicked(gtx) && chat != nil {
		chat.SetVerified(peer, true)
	}
	if s.revokeBtn.Clicked(gtx) && chat != nil {
		chat.SetVerified(peer, false)
	}
	if s.fetchBtn.Clicked(gtx) && chat != nil {
		chat.VerifyPeer(peer)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.backBtn, widgets.IconBack, th.Palette.OnBackground),
				"Verify contact")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.body(gtx, env, th, snap)
		}))
}

// body lays the explanation, the number, and the action.
func (s *TalkVerify) body(gtx layout.Context, env *Env, th *theme.Theme, snap state.ChatSnapshot) layout.Dimensions {
	return s.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.LG, Bottom: theme.LG}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return statusChip(gtx, th, snap.Verified)
					}),
					layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Title, th.Palette.OnBackground, snap.ActivePeer, 1)
					}),
					layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Body, th.Palette.Subtle,
							"Read this safety number aloud with your contact over a channel you both trust. If the numbers match, mark them verified.", 0)
					}),
					layout.Rigid(layout.Spacer{Height: theme.LG}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.numberPanel(gtx, th, snap)
					}),
					layout.Rigid(layout.Spacer{Height: theme.LG}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.action(gtx, th, snap)
					}))
			})
	})
}

// numberPanel renders BOTH safety numbers — the user's own key and the
// peer's — so they can be compared side by side over a trusted channel,
// exactly as the web console shows them. The peer row falls back to a fetch
// prompt until that contact's key has been retrieved.
func (s *TalkVerify) numberPanel(gtx layout.Context, th *theme.Theme, snap state.ChatSnapshot) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			r := gtx.Dp(theme.CardRadius)
			defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, r).Push(gtx.Ops).Pop()
			return widgets.Fill(gtx, th.Palette.Surface)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(theme.LG).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return numberRow(gtx, th, "You", snap.SelfFingerprint,
							"Syncing your key… reopen this screen in a moment.")
					}),
					layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return numberRow(gtx, th, snap.ActivePeer, snap.Fingerprint,
							"No key yet — fetch this contact's key to see their safety number.")
					}))
			})
		})
}

// numberRow renders one labelled safety number, or a hint when its key is
// not yet available.
func numberRow(gtx layout.Context, th *theme.Theme, label, fingerprint, empty string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.Caption, th.Palette.Subtle, label, 1)
		}),
		layout.Rigid(layout.Spacer{Height: theme.XS}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if fingerprint == "" {
				return th.Label(gtx, theme.Body, th.Palette.Subtle, empty, 0)
			}
			return th.Label(gtx, theme.Title, th.Palette.Accent, state.SafetyNumber(fingerprint), 0)
		}))
}

// action shows the primary control appropriate to the current state.
func (s *TalkVerify) action(gtx layout.Context, th *theme.Theme, snap state.ChatSnapshot) layout.Dimensions {
	if snap.Fingerprint == "" {
		return s.fetchBtn.Layout(gtx, th, widgets.ButtonTonal, "Fetch key", true, false)
	}
	if snap.Verified {
		return s.revokeBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(theme.SM).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.BodyStrong, th.Palette.Destructive, "Remove verification", 1)
				})
			})
		})
	}
	return s.markBtn.Layout(gtx, th, widgets.ButtonPrimary, "Mark as verified", true, false)
}

// statusChip is the verified/unverified badge at the top of the panel.
func statusChip(gtx layout.Context, th *theme.Theme, verified bool) layout.Dimensions {
	label, col, icon := "Not verified", th.Palette.Warning, widgets.IconLock
	if verified {
		label, col, icon = "Verified", th.Palette.Success, widgets.IconShield
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widgets.DrawIcon(gtx, icon, col, 18)
		}),
		layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return th.Label(gtx, theme.BodyStrong, col, label, 1)
		}))
}
