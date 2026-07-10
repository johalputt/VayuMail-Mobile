package screens

// settings_security.go — the Security section: app lock enrollment,
// PIN change, auto-lock window, lock-now. Split from settings.go
// (Rule 10).

import (
	"gioui.org/layout"

	"github.com/johalputt/VayuMail-Mobile/ui/state"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// autoLockChoices are the auto-lock idle windows offered, in seconds.
// The floor is 30s: the gate fires on rendered-frame gaps, and a
// foreground screen someone is reading also renders no frames — a
// shorter window would lock mid-read.
var autoLockChoices = []int{30, 60, 300, 900}

// autoLockLabel names an auto-lock window.
func autoLockLabel(sec int) string {
	switch sec {
	case 30:
		return "After 30 seconds"
	case 300:
		return "After 5 minutes"
	case 900:
		return "After 15 minutes"
	default:
		return "After 1 minute"
	}
}

// securityRows builds the Security section.
func (s *Settings) securityRows(gtx layout.Context, env *Env, snap state.Snapshot) []row {
	th := env.Theme
	if !env.State.HasAppLock() {
		return nil
	}
	rows := []row{s.section(th, "Security")}

	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		inner := s.item(th, "App lock", "Require a PIN to open VayuMail",
			func(gtx layout.Context) layout.Dimensions {
				dims, toggled := s.lockSwitch.Layout(gtx, th, snap.AppLockEnabled)
				if toggled {
					intent := LockIntentEnroll
					if snap.AppLockEnabled {
						intent = LockIntentDisable
					}
					env.LockSetup.Begin(intent)
					env.Nav.Push(state.ScreenLock, gtx.Now)
				}
				return dims
			})
		return inner(gtx)
	})

	if snap.AppLockEnabled {
		if s.changePinBtn.Clicked(gtx) {
			env.LockSetup.Begin(LockIntentChange)
			env.Nav.Push(state.ScreenLock, gtx.Now)
		}
		rows = append(rows, s.tapItem(th, &s.changePinBtn, "Change PIN", "",
			func(gtx layout.Context) layout.Dimensions {
				return widgets.DrawIcon(gtx, widgets.IconChevronRight, th.Palette.Subtle, 20)
			}))

		if s.autoLockBtn.Clicked(gtx) {
			next := 0
			for i, v := range autoLockChoices {
				if v == snap.AppLockTimeout {
					next = (i + 1) % len(autoLockChoices)
					break
				}
			}
			env.State.SetAppLockTimeout(autoLockChoices[next])
		}
		rows = append(rows, s.tapItem(th, &s.autoLockBtn, "Auto-lock",
			"Tap to change", func(gtx layout.Context) layout.Dimensions {
				return th.Label(gtx, theme.Caption, th.Palette.Accent,
					autoLockLabel(snap.AppLockTimeout), 1)
			}))

		if s.lockNowBtn.Clicked(gtx) {
			env.State.LockNow()
		}
		rows = append(rows, s.tapItem(th, &s.lockNowBtn, "Lock now", "",
			func(gtx layout.Context) layout.Dimensions {
				return widgets.DrawIcon(gtx, widgets.IconLock, th.Palette.Accent, 20)
			}))
	}
	return rows
}
