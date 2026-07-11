package screens

// settings_security.go — the Security section: app lock enrollment,
// PIN change, auto-lock window, lock-now. Split from settings.go
// (Rule 10).

import (
	"time"

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

		rows = append(rows, s.totpRows(gtx, env, snap)...)

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

// totpMode is the state of the inline two-factor form.
type totpMode int

const (
	totpIdle totpMode = iota
	totpEnrolling
	totpDisabling
)

// totpRows builds the two-factor unlock rows: the switch, and — while
// enrolling or disabling — the inline secret/code form. Enrollment is
// atomic: the secret is stored, one live code must verify, and a failed
// code removes the secret again so a typo can never lock the user out.
func (s *Settings) totpRows(gtx layout.Context, env *Env, snap state.Snapshot) []row {
	th := env.Theme
	p := th.Palette
	s.foldTOTPResult(env)

	rows := []row{func(gtx layout.Context) layout.Dimensions {
		inner := s.item(th, "Two-factor unlock",
			"Require an authenticator code after the PIN — the same TOTP secret VayuPress uses",
			func(gtx layout.Context) layout.Dimensions {
				dims, toggled := s.totpSwitch.Layout(gtx, th, snap.TOTPEnabled || s.totpMode == totpEnrolling)
				if toggled {
					if snap.TOTPEnabled {
						s.totpMode = totpDisabling
					} else {
						s.totpMode = totpEnrolling
					}
					s.totpSecret.SetText("")
					s.totpCode.SetText("")
					s.totpErr = ""
				}
				return dims
			})
		return inner(gtx)
	}}
	if s.totpMode == totpIdle {
		return rows
	}

	s.totpMu.Lock()
	busy, errText := s.totpBusy, s.totpErr
	s.totpMu.Unlock()

	if s.totpCancel.Clicked(gtx) {
		s.totpMode = totpIdle
	}
	if s.totpBtn.Clicked(gtx) && !busy {
		s.submitTOTP(env)
	}

	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.XS, Bottom: theme.SM}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{}
				if s.totpMode == totpEnrolling {
					children = append(children,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.totpSecret.Layout(gtx, th, "Authenticator secret",
								"paste the base32 secret (from VayuPress 2FA setup)")
						}),
						layout.Rigid(layout.Spacer{Height: theme.SM}.Layout))
				}
				label := "Confirm with a current code"
				if s.totpMode == totpDisabling {
					label = "Enter a current code to turn off"
				}
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.totpCode.Layout(gtx, th, label, "6-digit code")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if errText == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: theme.XS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, p.Destructive, errText, 1)
							})
					}),
					layout.Rigid(layout.Spacer{Height: theme.SM}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								action := "Turn on"
								if s.totpMode == totpDisabling {
									action = "Turn off"
								}
								if busy {
									action = "Checking…"
								}
								return s.totpBtn.Layout(gtx, th, widgets.ButtonTonal, action, false, busy)
							}),
							layout.Rigid(layout.Spacer{Width: theme.MD}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.totpCancel.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: theme.SM}.Layout(gtx,
										func(gtx layout.Context) layout.Dimensions {
											return th.Label(gtx, theme.Caption, p.Subtle, "Cancel", 1)
										})
								})
							}))
					}))
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
	})
	return rows
}

// submitTOTP runs the enroll or disable flow off-thread.
func (s *Settings) submitTOTP(env *Env) {
	code := s.totpCode.Text()
	s.totpMu.Lock()
	s.totpBusy = true
	s.totpErr = ""
	s.totpMu.Unlock()

	fail := func(msg string) {
		s.totpMu.Lock()
		s.totpBusy = false
		s.totpErr = msg
		s.totpMu.Unlock()
		if env.Invalidate != nil {
			env.Invalidate()
		}
	}
	finish := func() {
		s.totpMu.Lock()
		s.totpBusy = false
		s.totpDone = true
		s.totpMu.Unlock()
		if env.Invalidate != nil {
			env.Invalidate()
		}
	}

	if s.totpMode == totpDisabling {
		env.State.VerifyTOTP(code, false, func(ok bool, _ time.Duration) {
			if !ok {
				fail("Wrong code")
				return
			}
			env.State.RemoveTOTP(func(err error) {
				if err != nil {
					fail("Could not turn off two-factor unlock")
					return
				}
				finish()
			})
		})
		return
	}

	secret := s.totpSecret.Text()
	env.State.EnrollTOTP(secret, func(err error) {
		if err != nil {
			fail("Secret rejected — paste the base32 secret exactly")
			return
		}
		env.State.VerifyTOTP(code, false, func(ok bool, _ time.Duration) {
			if !ok {
				// A wrong confirm code must never leave a half-enrolled
				// factor behind: remove the secret again.
				env.State.RemoveTOTP(func(error) {})
				fail("Code didn't match — check the secret and try again")
				return
			}
			finish()
		})
	})
}

// foldTOTPResult applies a finished enroll/disable on the UI thread.
func (s *Settings) foldTOTPResult(env *Env) {
	s.totpMu.Lock()
	done := s.totpDone
	s.totpDone = false
	s.totpMu.Unlock()
	if !done {
		return
	}
	if s.totpMode == totpDisabling {
		env.Snack.ShowInfo("Two-factor unlock off")
	} else {
		env.Snack.ShowInfo("Two-factor unlock on")
	}
	s.totpMode = totpIdle
	s.totpSecret.SetText("")
	s.totpCode.SetText("")
}
