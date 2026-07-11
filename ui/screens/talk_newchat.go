package screens

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// paintScrim dims the whole frame behind a modal sheet.
func paintScrim(gtx layout.Context, c color.NRGBA) {
	widgets.FillMax(gtx, theme.WithAlpha(c, 0x99))
}

// handleNewChat processes the inline new-chat sheet's buttons.
func (s *Talk) handleNewChat(gtx layout.Context, env *Env) {
	if !s.composing {
		return
	}
	if s.cancelBtn.Clicked(gtx) {
		s.composing = false
		return
	}
	if s.startBtn.Clicked(gtx) {
		email := strings.ToLower(strings.TrimSpace(s.newField.Text()))
		if !strings.Contains(email, "@") {
			env.Snack.ShowInfo("Enter a full email address")
			return
		}
		if env.State.Chat != nil {
			env.State.Chat.OpenConversation(email)
		}
		s.composing = false
		env.Nav.Push(state.ScreenTalkRoom, gtx.Now)
	}
}

// layoutNewChat draws the dimmed scrim and the bottom sheet that collects
// a contact address for a new conversation.
func (s *Talk) layoutNewChat(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	// Scrim.
	paintScrim(gtx, th.Palette.Shadow)

	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				r := gtx.Dp(theme.CardRadius)
				defer clip.RRect{
					Rect: image.Rectangle{Max: gtx.Constraints.Min},
					NW:   r, NE: r,
				}.Push(gtx.Ops).Pop()
				return widgets.Fill(gtx, th.Palette.SurfaceRaised)
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.LG, Bottom: theme.XL}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Title, th.Palette.OnBackground, "New chat", 1)
							}),
							layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, th.Palette.Subtle,
									"Messages are end-to-end encrypted and disappear after they are read.", 0)
							}),
							layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.newField.Layout(gtx, th, "Contact email", "name@domain")
							}),
							layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.cancelBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.UniformInset(theme.SM).Layout(gtx,
												func(gtx layout.Context) layout.Dimensions {
													return th.Label(gtx, theme.BodyStrong, th.Palette.Subtle, "Cancel", 1)
												})
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.startBtn.Layout(gtx, th, widgets.ButtonPrimary, "Start", false, false)
									}))
							}))
					})
			})
	})
}
