package screens

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// setupMode is the current onboarding step.
type setupMode int

const (
	modeWelcome setupMode = iota
	modeScanning
	modePasteCode
	modeManual
)

// AccountSetup is the onboarding screen: QR provisioning first, manual
// entry as the explicit fallback.
type AccountSetup struct {
	mode    setupMode
	scanner *widgets.QRScanner

	scanBtn   widget.Clickable
	pasteBtn  widget.Clickable
	manualBtn widget.Clickable
	addBtn    widget.Clickable
	detectBtn widget.Clickable
	submitBtn widget.Clickable
	cancelBtn widget.Clickable

	setupCode widget.Editor

	host, imapPort, smtpPort, email, password widget.Editor
	form                                      layout.List

	// pendingCfg carries a completed autodetect result from the lookup
	// goroutine to the UI thread, which applies it to the editors in
	// applyPendingDetect. Guarded by mu so the two threads never race.
	mu         sync.Mutex
	pendingCfg *account.Config
}

// NewAccountSetup constructs the onboarding screen. frameSource may be
// nil when no camera bridge is registered.
func NewAccountSetup(frameSource widgets.FrameSource) *AccountSetup {
	s := &AccountSetup{
		scanner: widgets.NewQRScanner(frameSource),
		form:    layout.List{Axis: layout.Vertical},
	}
	for _, e := range []*widget.Editor{&s.host, &s.imapPort, &s.smtpPort, &s.email, &s.password} {
		e.SingleLine = true
	}
	// The setup code (the QR's base64url payload) can be long; allow wrap.
	s.setupCode.SingleLine = false
	s.imapPort.SetText("993")
	s.smtpPort.SetText("587")
	s.password.Mask = '•'
	return s
}

// Layout renders the current onboarding step.
func (s *AccountSetup) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	switch s.mode {
	case modeScanning:
		return s.layoutScanner(gtx, env)
	case modePasteCode:
		return s.layoutPasteCode(gtx, env)
	case modeManual:
		return s.layoutManual(gtx, env)
	default:
		return s.layoutWelcome(gtx, env)
	}
}

func (s *AccountSetup) layoutWelcome(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	if s.scanBtn.Clicked(gtx) {
		s.mode = modeScanning
	}
	if s.pasteBtn.Clicked(gtx) {
		s.mode = modePasteCode
	}
	if s.manualBtn.Clicked(gtx) {
		s.mode = modeManual
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(theme.XL).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Display, th.Palette.OnBackground, "Welcome to VayuMail.", 1)
				}),
				layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Body, th.Palette.OnSurface,
						"Scan the QR code from your mail server to get started.", 0)
				}),
				layout.Rigid(layout.Spacer{Height: theme.XL}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return widgets.PrimaryButton(gtx, th, &s.scanBtn, "Scan QR Code")
				}),
				layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.pasteBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Body, th.Palette.Accent, "Paste setup code", 1)
					})
				}),
				layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.manualBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Caption, th.Palette.Subtle, "Enter details manually", 1)
					})
				}))
		})
	})
}

func (s *AccountSetup) layoutScanner(gtx layout.Context, env *Env) layout.Dimensions {
	if s.cancelBtn.Clicked(gtx) {
		s.mode = modeWelcome
	}
	gtx.Constraints.Min = gtx.Constraints.Max
	payload, done := s.scanner.Layout(gtx, env.Theme)
	if done {
		s.provision(env, []byte(payload))
		s.mode = modeWelcome
	}
	// Cancel affordance at the top left.
	layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return widgets.IconButton(gtx, env.Theme, &s.cancelBtn, widgets.IconBack, env.Theme.Palette.OnBackground)
	})
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// layoutPasteCode lets the user paste the QR's setup code (its base64url
// payload) when the camera is unavailable. It runs the identical signed
// provisioning path as a live scan (Rule 7).
func (s *AccountSetup) layoutPasteCode(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	if s.cancelBtn.Clicked(gtx) {
		s.mode = modeWelcome
	}
	if s.submitBtn.Clicked(gtx) {
		code := strings.TrimSpace(s.setupCode.Text())
		if code == "" {
			env.Snack.ShowInfo("Paste the setup code first")
		} else {
			s.provision(env, []byte(code))
			s.setupCode.SetText("")
			s.mode = modeWelcome
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.cancelBtn, widgets.IconBack, th.Palette.OnBackground),
				"Paste setup code")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.MD}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Body, th.Palette.OnSurface,
						"Copy the setup code shown with your VayuMail QR and paste it here.", 0)
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
					return widgets.PrimaryButton(gtx, th, &s.submitBtn, "Add Account")
				})
		}))
}

// provision verifies and redeems a scanned payload off the UI thread,
// then hands the account to the sync manager (Rule 7: nothing is used
// before the signature verifies).
func (s *AccountSetup) provision(env *Env, raw []byte) {
	go func() {
		payload, err := account.ParseAndVerify(raw, time.Now())
		if err != nil {
			env.Snack.ShowInfo(provisionErrorMessage(err))
			env.State.Refresh()
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		creds, err := account.ExchangeToken(ctx, http.DefaultClient, payload)
		if err != nil {
			env.Snack.ShowInfo(provisionErrorMessage(err))
			return
		}
		alias := fmt.Sprintf("vayumail-%s-%d", payload.Username, time.Now().Unix())
		cfg := payload.Config(alias)
		secret := creds.IMAPPassword
		if secret == "" {
			// Token-based (modern auth / 2FA): store the bearer token and
			// authenticate with SASL rather than a password.
			secret = creds.OAuthToken
			cfg.AuthMech = account.AuthOAuthBearer
			if strings.EqualFold(creds.TokenType, "xoauth2") {
				cfg.AuthMech = account.AuthXOAuth2
			}
		}
		env.State.Send(syncmanager.AddAccountCmd{
			Config:     cfg,
			Credential: []byte(secret),
		})
		env.Snack.ShowInfo("Account added")
		env.State.Refresh()
	}()
}

func (s *AccountSetup) layoutManual(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	// Apply any completed autodetect result on the UI thread before laying out
	// the editors, so their text is never mutated from the lookup goroutine.
	s.applyPendingDetect()
	if s.cancelBtn.Clicked(gtx) {
		s.mode = modeWelcome
	}
	if s.detectBtn.Clicked(gtx) {
		s.autodetect(env)
	}
	if s.addBtn.Clicked(gtx) {
		s.addManual(env)
	}
	fields := []struct {
		label  string
		editor *widget.Editor
	}{
		{"Email", &s.email},
		{"Server", &s.host},
		{"IMAP port", &s.imapPort},
		{"SMTP port", &s.smtpPort},
		{"Password", &s.password},
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return topBar(gtx, th,
				iconBtn(th, &s.cancelBtn, widgets.IconBack, th.Palette.OnBackground),
				"Add Account")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.form.Layout(gtx, len(fields)+1, func(gtx layout.Context, i int) layout.Dimensions {
				if i == len(fields) {
					return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.XL}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return widgets.PrimaryButton(gtx, th, &s.detectBtn, "Auto-detect from email")
								}),
								layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return widgets.PrimaryButton(gtx, th, &s.addBtn, "Add Account")
								}),
							)
						})
				}
				f := fields[i]
				return layout.Inset{Left: theme.LG, Right: theme.LG, Top: theme.MD}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return th.Label(gtx, theme.Caption, th.Palette.Subtle, f.label, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return f.editor.Layout(gtx, th.Shaper,
									font.Font{Weight: theme.Body.Weight}, theme.Body.Size,
									theme.ColorOp(gtx, th.Palette.OnBackground),
									theme.ColorOp(gtx, th.Palette.AccentSubtle))
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return widgets.Separator(gtx, th, 0)
							}))
					})
			})
		}))
}

// addManual validates the form and submits the account.
func (s *AccountSetup) addManual(env *Env) {
	imapPort, err1 := strconv.Atoi(s.imapPort.Text())
	smtpPort, err2 := strconv.Atoi(s.smtpPort.Text())
	if err1 != nil || err2 != nil {
		env.Snack.ShowInfo("Ports must be numbers")
		return
	}
	email := s.email.Text()
	cfg := account.Config{
		DisplayName:   email,
		EmailAddress:  email,
		IMAPHost:      s.host.Text(),
		IMAPPort:      imapPort,
		IMAPTLS:       account.TLSModeImplicit,
		SMTPHost:      s.host.Text(),
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
		Credential: []byte(s.password.Text()),
	})
	s.password.SetText("")
	env.Snack.ShowInfo("Account added")
	env.State.Refresh()
}

// provisionErrorMessage maps typed provisioning errors onto clear,
// user-facing language. Unverified payloads never proceed (Rule 7).
func provisionErrorMessage(err error) string {
	switch {
	case errors.Is(err, account.ErrExpired):
		return "This QR code has expired — generate a fresh one"
	case errors.Is(err, account.ErrInvalidSignature):
		return "QR code signature is invalid — do not trust this code"
	case errors.Is(err, account.ErrUnknownVersion):
		return "QR code is from a newer VayuMail — update the app"
	case errors.Is(err, account.ErrInsecureTransport):
		return "Server offered an insecure connection — refused"
	case errors.Is(err, account.ErrInvalidPort):
		return "QR code contains an invalid port"
	case errors.Is(err, account.ErrTokenExpired):
		return "Setup code already used — generate a fresh QR"
	case errors.Is(err, account.ErrNetwork):
		return "Could not reach the mail server"
	default:
		return "Could not read this QR code"
	}
}
