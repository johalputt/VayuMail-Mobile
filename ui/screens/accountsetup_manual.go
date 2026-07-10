package screens

// accountsetup_manual.go — the setup-code and manual-entry fallbacks,
// split from accountsetup.go (Rule 10).

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// layoutCode lets the user paste an Ed25519-signed setup code. It runs
// the identical signed provisioning path regardless of how the code
// arrived (Rule 7).
func (s *AccountSetup) layoutCode(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	if s.cancelBtn.Clicked(gtx) {
		s.mode = modeConnect
	}
	if s.submitBtn.Clicked(gtx) {
		code := strings.TrimSpace(s.setupCode.Text())
		if code == "" {
			env.Snack.ShowInfo("Paste the setup code first")
		} else {
			s.provision(env, []byte(code))
			s.setupCode.SetText("")
			s.mode = modeConnect
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.cancelBtn, widgets.IconBack, th.Palette.OnBackground),
				"Setup code")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.MD}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Body, th.Palette.OnSurface,
						"Paste the signed setup code from your mail server. It is verified before anything connects.", 0)
				})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.LG}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					if s.setupCode.Len() == 0 {
						th.Label(gtx, theme.Caption, th.Palette.Subtle, "paste setup code…", 1)
					}
					return s.setupCode.Layout(gtx, th.Shaper,
						font.Font{Weight: theme.Body.Weight}, theme.Body.Size,
						theme.ColorOp(gtx, th.Palette.OnBackground),
						theme.ColorOp(gtx, th.Palette.AccentSubtle))
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Right: theme.LG, Bottom: theme.XL}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return s.submitBtn.Layout(gtx, th, widgets.ButtonPrimary, "Add account", true, false)
				})
		}))
}

// layoutManual is the full-form fallback with autodetect assist.
func (s *AccountSetup) layoutManual(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	// Apply any completed autodetect result on the UI thread before laying
	// out the editors, so their text is never mutated from the goroutine.
	s.applyPendingDetect()
	if s.cancelBtn.Clicked(gtx) {
		s.mode = modeConnect
	}
	if s.detectBtn.Clicked(gtx) {
		s.autodetect(env)
	}
	if s.addBtn.Clicked(gtx) {
		s.addManual(env)
	}
	fields := []struct {
		field *widgets.TextField
		label string
		hint  string
	}{
		{s.manualEmail, "Email", "you@yourdomain.com"},
		{s.host, "Server", "mail.yourdomain.com"},
		{s.imapPort, "IMAP port", "993"},
		{s.smtpPort, "SMTP port", "587"},
		{s.manualPassword, "Password", ""},
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.cancelBtn, widgets.IconBack, th.Palette.OnBackground),
				"Manual setup")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.form.Layout(gtx, len(fields)+1, func(gtx layout.Context, i int) layout.Dimensions {
				if i == len(fields) {
					return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.XL, Bottom: theme.XL}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.detectBtn.Layout(gtx, th, widgets.ButtonTonal, "Auto-detect from email", true, false)
								}),
								layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.addBtn.Layout(gtx, th, widgets.ButtonPrimary, "Add account", true, false)
								}))
						})
				}
				f := fields[i]
				return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.MD}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return f.field.Layout(gtx, th, f.label, f.hint)
					})
			})
		}))
}

// addManual validates the form and submits the account.
func (s *AccountSetup) addManual(env *Env) {
	imapPort, err1 := strconv.Atoi(strings.TrimSpace(s.imapPort.Text()))
	smtpPort, err2 := strconv.Atoi(strings.TrimSpace(s.smtpPort.Text()))
	if err1 != nil || err2 != nil {
		env.Snack.ShowInfo("Ports must be numbers")
		return
	}
	email := strings.TrimSpace(s.manualEmail.Text())
	cfg := account.Config{
		DisplayName:   email,
		EmailAddress:  email,
		IMAPHost:      strings.TrimSpace(s.host.Text()),
		IMAPPort:      imapPort,
		IMAPTLS:       account.TLSModeImplicit,
		SMTPHost:      strings.TrimSpace(s.host.Text()),
		SMTPPort:      smtpPort,
		SMTPTLS:       account.TLSModeSTARTTLS,
		Username:      email,
		KeystoreAlias: fmt.Sprintf("vayumail-%s-%d", email, time.Now().Unix()),
	}
	if err := cfg.Validate(); err != nil {
		env.Snack.ShowInfo("Please fill in every field")
		return
	}
	env.State.Send(syncmanager.AddAccountCmd{
		Config:     cfg,
		Credential: []byte(s.manualPassword.Text()),
	})
	s.manualPassword.SetText("")
	env.Snack.ShowInfo("Account added")
	env.State.Refresh()
}
