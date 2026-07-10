// Package screens contains the six full-screen views. Each screen is a
// pure layout function over the state snapshot plus its own widget
// state; nothing here blocks (Rule 5).
package screens

import (
	"image"
	"image/color"
	"io"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Env bundles everything a screen needs: theme, state, navigation, the
// shared snackbar, the shared composer, the confirm dialog, and the PGP
// keyring.
type Env struct {
	Theme    *theme.Theme
	State    *state.AppState
	Nav      *state.Nav
	Snack    *widgets.Snackbar
	Composer *widgets.Composer
	Keyring  *pgp.Keyring
	// Dialog is the app-wide confirm modal, drawn above the active screen
	// by the root.
	Dialog *widgets.Dialog
	// LockSetup is the shared PIN screen instance behind ScreenLock;
	// callers Begin() it with an intent before pushing.
	LockSetup *Lock
	// PickFile opens the platform file picker and returns a reader for the
	// chosen file. It blocks, so callers run it on a goroutine. Nil when the
	// platform has no picker (the composer then reports it is unavailable).
	PickFile func() (io.ReadCloser, error)
	// Invalidate wakes the window after an async update (e.g. a picked file
	// added to the composer from a goroutine).
	Invalidate func()
}

// fadeRise draws w fading in and rising as t goes 0→1 — the standard
// entrance treatment for whole-screen content.
func fadeRise(gtx layout.Context, t float32, w layout.Widget) layout.Dimensions {
	if t >= 1 {
		return w(gtx)
	}
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	rise := int((1 - t) * float32(gtx.Dp(theme.LG)))
	defer op.Offset(image.Pt(0, rise)).Push(gtx.Ops).Pop()
	defer paint.PushOpacity(gtx.Ops, anim.Clamp01(t)).Pop()
	call.Add(gtx.Ops)
	return dims
}

// fontFor maps a theme text style onto a Gio font.
func fontFor(style theme.TextStyle) font.Font {
	return font.Font{Weight: style.Weight}
}

// topBar lays out the standard screen header: optional leading icon,
// title, and trailing actions.
func topBar(gtx layout.Context, th *theme.Theme, leading layout.Widget, title string, trailing ...layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.SM, Right: theme.SM, Top: theme.SM, Bottom: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					children := []layout.FlexChild{}
					if leading != nil {
						children = append(children, layout.Rigid(leading))
					}
					children = append(children,
						layout.Rigid(layout.Spacer{Width: theme.SM}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Heading, th.Palette.OnBackground, title, 1)
						}))
					for _, t := range trailing {
						children = append(children, layout.Rigid(t))
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return widgets.Separator(gtx, th, 0)
		}))
}

// emptyState centers an optional icon, a Display heading, and an
// optional caption.
func emptyState(gtx layout.Context, th *theme.Theme, icon widgets.Icon, hasIcon bool, headline, caption string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(theme.XXL).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasIcon {
						return layout.Dimensions{}
					}
					return layout.Inset{Bottom: theme.MD}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return widgets.DrawIcon(gtx, icon, th.Palette.Subtle, 48)
						})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Display, th.Palette.OnBackground, headline, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if caption == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: theme.SM}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Caption, th.Palette.Subtle, caption, 1)
						})
				}))
		})
	})
}

// iconBtn is a shorthand for a trailing top-bar icon button.
func iconBtn(th *theme.Theme, click *widget.Clickable, icon widgets.Icon, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return widgets.IconButton(gtx, th, click, icon, c)
	}
}
