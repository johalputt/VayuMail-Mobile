package screens

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Settings is deliberately short: accounts, sync, PGP, appearance.
// Fewer options is the feature.
type Settings struct {
	backBtn  widget.Clickable
	syncBtns []widget.Clickable
	list     layout.List
}

// NewSettings constructs the settings screen.
func NewSettings() *Settings {
	return &Settings{list: layout.List{Axis: layout.Vertical}}
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
	keys := len(env.Keyring.Entities())
	item(fmt.Sprintf("Keys in keyring: %d", keys),
		"Import via vayumail-cli at v0.1 — in-app key management is coming", nil)

	section("Appearance")
	item("Theme", "Follows the system light/dark preference", nil)

	section("About")
	item("VayuMail v0.1.0", "Pure Go · no telemetry · Apache-2.0", nil)

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
