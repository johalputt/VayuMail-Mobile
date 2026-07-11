package screens

import (
	"fmt"
	"strings"
	"sync"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/internal/version"
	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Settings is deliberately curated: accounts, security, sync &
// notifications, PGP, appearance. Fewer options is the feature.
type Settings struct {
	backBtn widget.Clickable
	list    layout.List

	syncBtns    []widget.Clickable
	signOutBtns []widget.Clickable
	pwBtns      []widget.Clickable
	addAcctBtn  widget.Clickable

	// pwEditID is the account whose inline password editor is open (0 =
	// none).
	pwEditID  int64
	pwField   *widgets.TextField
	pwSaveBtn widgets.Button

	lockSwitch    widgets.Switch
	changePinBtn  widget.Clickable
	autoLockBtn   widget.Clickable
	lockNowBtn    widget.Clickable
	notifySwitch  widgets.Switch
	previewSwitch widgets.Switch
	syncAllBtn    widget.Clickable
	autoWKDSwitch widgets.Switch

	// Two-factor unlock enrollment/disable state (settings_security.go).
	totpSwitch widgets.Switch
	totpMode   totpMode
	totpSecret *widgets.TextField
	totpCode   *widgets.TextField
	totpBtn    widgets.Button
	totpCancel widget.Clickable
	totpMu     sync.Mutex
	totpBusy   bool
	totpErr    string
	totpDone   bool

	keyPaste  widget.Editor
	keyEmail  widget.Editor
	importBtn widget.Clickable
	lookupBtn widget.Clickable
	wkdAllBtn widget.Clickable

	trustBtns  []widget.Clickable
	deleteBtns []widget.Clickable

	keyDirEditor  widget.Editor
	keyDirSaveBtn widget.Clickable
	keyDirSyncBtn widget.Clickable
	keyDirLoaded  bool
	syncKeyBtn    widget.Clickable
}

// NewSettings constructs the settings screen.
func NewSettings() *Settings {
	s := &Settings{
		list:       layout.List{Axis: layout.Vertical},
		pwField:    widgets.NewTextField(true),
		totpSecret: widgets.NewTextField(false),
		totpCode:   widgets.NewTextField(false),
	}
	s.keyEmail.SingleLine = true
	s.keyDirEditor.SingleLine = true
	return s
}

// row is one settings line, built ahead of the virtualized list.
type row func(gtx layout.Context) layout.Dimensions

// Layout renders the settings sections.
func (s *Settings) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	snap := env.State.Snapshot()

	if s.backBtn.Clicked(gtx) {
		env.Nav.Pop(gtx.Now)
	}

	// Security first: the app lock and two-factor unlock are the headline
	// additions, so they lead Settings rather than sit below the fold.
	var rows []row
	rows = append(rows, s.securityRows(gtx, env, snap)...)
	rows = append(rows, s.accountRows(gtx, env, snap)...)
	rows = append(rows, s.syncRows(gtx, env, snap)...)
	rows = append(rows, s.pgpRows(gtx, env, snap)...)

	rows = append(rows, s.section(th, "Appearance"))
	rows = append(rows, s.item(th, "Theme", "Follows the system light/dark preference", nil))

	rows = append(rows, s.section(th, "About"))
	rows = append(rows, s.item(th, "VayuMail v"+version.Semantic,
		"Pure Go · no telemetry · Apache-2.0", nil))

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

// section renders a category header.
func (s *Settings) section(th *theme.Theme, title string) row {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: theme.LG, Top: theme.XL, Bottom: theme.SM}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return th.Label(gtx, theme.Caption, th.Palette.Accent, title, 1)
			})
	}
}

// item renders one two-line row with an optional trailing widget.
func (s *Settings) item(th *theme.Theme, primary, secondary string, trailing layout.Widget) row {
	return func(gtx layout.Context) layout.Dimensions {
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
								return th.Label(gtx, theme.Caption, th.Palette.Subtle, secondary, 2)
							}))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if trailing == nil {
							return layout.Dimensions{}
						}
						return trailing(gtx)
					}))
			})
	}
}

// tapItem is an item whose whole row is tappable.
func (s *Settings) tapItem(th *theme.Theme, click *widget.Clickable, primary, secondary string, trailing layout.Widget) row {
	inner := s.item(th, primary, secondary, trailing)
	return func(gtx layout.Context) layout.Dimensions {
		return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return inner(gtx)
		})
	}
}

// accountRows: one card per account with sync/sign-out, plus add.
func (s *Settings) accountRows(gtx layout.Context, env *Env, snap state.Snapshot) []row {
	th := env.Theme
	p := th.Palette
	if len(s.syncBtns) < len(snap.Accounts) {
		grow := len(snap.Accounts) - len(s.syncBtns)
		s.syncBtns = append(s.syncBtns, make([]widget.Clickable, grow)...)
		s.signOutBtns = append(s.signOutBtns, make([]widget.Clickable, grow)...)
		s.pwBtns = append(s.pwBtns, make([]widget.Clickable, grow)...)
	}
	rows := []row{s.section(th, "Accounts")}
	for i, acct := range snap.Accounts {
		acct := acct
		i := i
		if s.syncBtns[i].Clicked(gtx) {
			env.State.Send(syncmanager.SyncNowCmd{AccountID: acct.ID})
			env.Snack.ShowInfo("Syncing " + acct.EmailAddress)
		}
		if s.signOutBtns[i].Clicked(gtx) {
			id := acct.ID
			env.Dialog.Show(gtx.Now, "Sign out?",
				acct.EmailAddress+" and its mail are removed from this device. Nothing changes on the server.",
				"Sign out", true, func() { env.State.RemoveAccount(id) })
		}
		if s.pwBtns[i].Clicked(gtx) {
			if s.pwEditID == acct.ID {
				s.pwEditID = 0
			} else {
				s.pwEditID = acct.ID
				s.pwField.SetText("")
			}
		}
		rows = append(rows, s.item(th, acct.EmailAddress,
			fmt.Sprintf("IMAP %s:%d", acct.IMAPHost, acct.IMAPPort),
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.syncBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: theme.MD}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Caption, p.Accent, "Sync now", 1)
								})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.pwBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: theme.MD}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return th.Label(gtx, theme.Caption, p.Accent, "Password", 1)
								})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.signOutBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return th.Label(gtx, theme.Caption, p.Destructive, "Sign out", 1)
						})
					}))
			}))
		if s.pwEditID == acct.ID {
			id := acct.ID
			if s.pwSaveBtn.Clicked(gtx) {
				pw := s.pwField.Text()
				if strings.TrimSpace(pw) == "" {
					env.Snack.ShowInfo("Enter the new password first")
				} else {
					env.State.UpdateCredential(id, pw)
					s.pwField.SetText("")
					s.pwEditID = 0
				}
			}
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.XS, Bottom: theme.SM}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return s.pwField.Layout(gtx, th, "", "new password or app password")
							}),
							layout.Rigid(layout.Spacer{Width: theme.MD}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.pwSaveBtn.Layout(gtx, th, widgets.ButtonTonal, "Save", false, false)
							}))
					})
			})
		}
	}
	if len(snap.Accounts) == 0 {
		rows = append(rows, s.item(th, "No accounts", "Connect one below", nil))
	}
	if s.addAcctBtn.Clicked(gtx) {
		env.Nav.Push(state.ScreenSetup, gtx.Now)
	}
	rows = append(rows, s.tapItem(th, &s.addAcctBtn, "Add account",
		"Connect another mailbox", func(gtx layout.Context) layout.Dimensions {
			return widgets.DrawIcon(gtx, widgets.IconAdd, p.Accent, 20)
		}))
	return rows
}

// syncRows: quick sync and notification preferences.
func (s *Settings) syncRows(gtx layout.Context, env *Env, snap state.Snapshot) []row {
	th := env.Theme
	if s.syncAllBtn.Clicked(gtx) {
		for _, a := range snap.Accounts {
			env.State.Send(syncmanager.SyncNowCmd{AccountID: a.ID})
		}
		env.Snack.ShowInfo("Syncing all accounts…")
	}
	rows := []row{
		s.section(th, "Sync & notifications"),
		s.tapItem(th, &s.syncAllBtn, "Sync all accounts now",
			"Full refresh of every folder", func(gtx layout.Context) layout.Dimensions {
				return widgets.DrawIcon(gtx, widgets.IconRefresh, th.Palette.Accent, 20)
			}),
		func(gtx layout.Context) layout.Dimensions {
			inner := s.item(th, "New-mail notifications", "Coalesced — bursts become one summary",
				func(gtx layout.Context) layout.Dimensions {
					dims, toggled := s.notifySwitch.Layout(gtx, th, snap.NotificationsOn)
					if toggled {
						env.State.SetNotifications(!snap.NotificationsOn)
					}
					return dims
				})
			return inner(gtx)
		},
		func(gtx layout.Context) layout.Dimensions {
			inner := s.item(th, "Show message preview", "Sender and subject in notifications; off = just \"New mail\"",
				func(gtx layout.Context) layout.Dimensions {
					dims, toggled := s.previewSwitch.Layout(gtx, th, snap.NotifyPreviewOn)
					if toggled {
						env.State.SetNotifyPreview(!snap.NotifyPreviewOn)
					}
					return dims
				})
			return inner(gtx)
		},
		s.item(th, "Real-time delivery", "One held IMAP IDLE connection — no polling", nil),
	}
	return rows
}
