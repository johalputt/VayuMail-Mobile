package screens

import (
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/internal/version"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Settings is deliberately short: accounts, sync, PGP, appearance.
// Fewer options is the feature.
type Settings struct {
	backBtn  widget.Clickable
	syncBtns []widget.Clickable
	list     layout.List

	keyPaste   widget.Editor
	keyEmail   widget.Editor
	importBtn  widget.Clickable
	lookupBtn  widget.Clickable
	trustBtns  []widget.Clickable
	deleteBtns []widget.Clickable
}

// NewSettings constructs the settings screen.
func NewSettings() *Settings {
	s := &Settings{list: layout.List{Axis: layout.Vertical}}
	s.keyEmail.SingleLine = true
	return s
}

// Layout renders the settings sections.
func (s *Settings) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	snap := env.State.Snapshot()

	if s.backBtn.Clicked(gtx) {
		env.Nav.Pop(gtx.Now)
	}
	if len(s.syncBtns) < len(snap.Accounts) {
		s.syncBtns = append(s.syncBtns, make([]widget.Clickable, len(snap.Accounts)-len(s.syncBtns))...)
	}

	type row func(gtx layout.Context) layout.Dimensions
	var rows []row

	section := func(title string) {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Top: theme.XL, Bottom: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Caption, th.Palette.Subtle, title, 1)
				})
		})
	}
	item := func(primary, secondary string, trailing layout.Widget) {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.SM, Bottom: theme.SM}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Body, th.Palette.OnBackground, primary, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if secondary == "" {
										return layout.Dimensions{}
									}
									return th.Label(gtx, theme.Caption, th.Palette.Subtle, secondary, 1)
								}))
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if trailing == nil {
								return layout.Dimensions{}
							}
							return trailing(gtx)
						}))
				})
		})
	}

	section("Accounts")
	for i, acct := range snap.Accounts {
		acct := acct
		i := i
		if s.syncBtns[i].Clicked(gtx) {
			env.State.Send(syncmanager.SyncNowCmd{AccountID: acct.ID})
			env.Snack.ShowInfo("Syncing " + acct.EmailAddress)
		}
		item(acct.EmailAddress,
			fmt.Sprintf("%s · IMAP %s:%d", acct.DisplayName, acct.IMAPHost, acct.IMAPPort),
			func(gtx layout.Context) layout.Dimensions {
				return s.syncBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Caption, th.Palette.Accent, "Sync now", 1)
				})
			})
	}
	if len(snap.Accounts) == 0 {
		item("No accounts", "Add one from the welcome screen", nil)
	}

	section("Sync")
	item("Real-time delivery", "IMAP IDLE — new mail arrives as it lands", nil)
	item("Outbox retries", "Failed sends retry with backoff, then surface here", nil)

	section("PGP")
	if len(s.trustBtns) < len(snap.PGPKeys) {
		s.trustBtns = append(s.trustBtns, make([]widget.Clickable, len(snap.PGPKeys)-len(s.trustBtns))...)
		s.deleteBtns = append(s.deleteBtns, make([]widget.Clickable, len(snap.PGPKeys)-len(s.deleteBtns))...)
	}
	for i, k := range snap.PGPKeys {
		i, k := i, k
		if s.trustBtns[i].Clicked(gtx) {
			env.State.SetPGPTrust(k.Fingerprint, (k.TrustLevel+1)%3)
		}
		if s.deleteBtns[i].Clicked(gtx) {
			env.State.DeletePGPKey(k.Fingerprint)
		}
		fp := k.Fingerprint
		if len(fp) > 16 {
			fp = fp[:16]
		}
		kind := "public"
		if k.IsPrivate {
			kind = "private"
		}
		item(k.Email, fp+" · "+kind,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.trustBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: theme.MD}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Caption, th.Palette.Accent,
										trustLabel(k.TrustLevel), 1)
								})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.deleteBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Caption, th.Palette.Destructive, "Remove", 1)
						})
					}))
			})
	}
	if len(snap.PGPKeys) == 0 {
		item("No keys yet", "Paste an armored key below, or look one up by address", nil)
	}
	if s.lookupBtn.Clicked(gtx) && strings.Contains(s.keyEmail.Text(), "@") {
		env.State.DiscoverPGPKey(strings.TrimSpace(s.keyEmail.Text()))
	}
	if s.importBtn.Clicked(gtx) && strings.Contains(s.keyPaste.Text(), "BEGIN PGP") {
		env.State.ImportPGPKey(s.keyPaste.Text())
		s.keyPaste.SetText("")
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return s.keyTools(gtx, env)
	})

	section("Appearance")
	item("Theme", "Follows the system light/dark preference", nil)

	section("About")
	item("VayuMail v"+version.Semantic, "Pure Go · no telemetry · Apache-2.0", nil)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.backBtn, widgets.IconBack, th.Palette.OnBackground),
				"Settings")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.list.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				return rows[i](gtx)
			})
		}))
}

// trustLabel names a stored trust level.
func trustLabel(level int) string {
	switch level {
	case 1:
		return "Marginal"
	case 2:
		return "Trusted"
	default:
		return "Unverified"
	}
}

// keyTools renders the WKD lookup field and the armored-key paste box.
func (s *Settings) keyTools(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	editor := func(e *widget.Editor, hint string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if e.Len() == 0 {
				th.Label(gtx, theme.Caption, th.Palette.Subtle, hint, 1)
			}
			return e.Layout(gtx, th.Shaper,
				font.Font{Weight: theme.Body.Weight}, theme.Caption.Size,
				theme.ColorOp(gtx, th.Palette.OnBackground),
				theme.ColorOp(gtx, th.Palette.AccentSubtle))
		}
	}
	return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.SM, Bottom: theme.MD}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, editor(&s.keyEmail, "address for key lookup (WKD)")),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.lookupBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, th.Palette.Accent, "Look up", 1)
							})
						}))
				}),
				layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
				layout.Rigid(editor(&s.keyPaste, "paste armored PGP key…")),
				layout.Rigid(layout.Spacer{Height: theme.XS}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.importBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Caption, th.Palette.Accent, "Import key", 1)
					})
				}))
		})
}
