package screens

import (
	"strings"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/ui/anim"
	"github.com/johalputt/VayuMail-Mobile/ui/theme"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// setupMode is the current onboarding step.
type setupMode int

const (
	// modeConnect is the primary path: email + app password, settings
	// discovered from the domain's signed-over-HTTPS autoconfig document.
	modeConnect setupMode = iota
	// modeCode accepts a pasted Ed25519-signed setup code (Rule 7).
	modeCode
	// modeManual is the explicit full-form fallback.
	modeManual
)

// AccountSetup is the onboarding screen. Direct connect first — type
// your address and an app password, everything else is discovered —
// with signed setup codes and manual entry as explicit fallbacks.
type AccountSetup struct {
	mode    setupMode
	entered anim.Anim
	primed  bool

	email    *widgets.TextField
	password *widgets.TextField

	connectBtn widgets.Button
	codeBtn    widget.Clickable
	manualBtn  widget.Clickable
	cancelBtn  widget.Clickable

	setupCode widget.Editor
	submitBtn widgets.Button

	host, imapPort, smtpPort, manualEmail, manualPassword *widgets.TextField
	form                                                  layout.List
	detectBtn                                             widgets.Button
	addBtn                                                widgets.Button

	// busy/status/errText carry async connect progress to the UI thread;
	// pendingCfg carries a completed autodetect result. Guarded by mu.
	mu         sync.Mutex
	busy       bool
	status     string
	errText    string
	pendingCfg *account.Config
}

// NewAccountSetup constructs the onboarding screen.
func NewAccountSetup() *AccountSetup {
	s := &AccountSetup{
		email:          widgets.NewTextField(false),
		password:       widgets.NewTextField(true),
		host:           widgets.NewTextField(false),
		imapPort:       widgets.NewTextField(false),
		smtpPort:       widgets.NewTextField(false),
		manualEmail:    widgets.NewTextField(false),
		manualPassword: widgets.NewTextField(true),
		form:           layout.List{Axis: layout.Vertical},
	}
	// The setup code (a base64url payload) can be long; allow wrap.
	s.setupCode.SingleLine = false
	s.imapPort.SetText("993")
	s.smtpPort.SetText("587")
	return s
}

// Layout renders the current onboarding step.
func (s *AccountSetup) Layout(gtx layout.Context, env *Env) layout.Dimensions {
	if !s.primed {
		s.primed = true
		s.entered.Start(gtx.Now, 450*time.Millisecond)
	}
	switch s.mode {
	case modeCode:
		return s.layoutCode(gtx, env)
	case modeManual:
		return s.layoutManual(gtx, env)
	default:
		return s.layoutConnect(gtx, env)
	}
}

// layoutConnect is the hero screen: wordmark, two fields, one button.
func (s *AccountSetup) layoutConnect(gtx layout.Context, env *Env) layout.Dimensions {
	th := env.Theme
	p := th.Palette
	s.mu.Lock()
	busy, status, errText := s.busy, s.status, s.errText
	s.mu.Unlock()

	if s.codeBtn.Clicked(gtx) {
		s.setError("")
		s.mode = modeCode
	}
	if s.manualBtn.Clicked(gtx) {
		s.setError("")
		s.mode = modeManual
	}
	if s.connectBtn.Clicked(gtx) && !busy {
		s.connect(env)
	}

	// Entrance: the whole card fades and rises once, on first show.
	t, done := s.entered.Progress(gtx.Now, anim.OutCubic)
	if !done {
		gtx.Execute(op.InvalidateCmd{})
	}

	pushed := env.Nav.Depth() > 1
	if pushed && s.cancelBtn.Clicked(gtx) {
		env.Nav.Pop(gtx.Now)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !pushed {
				return layout.Dimensions{}
			}
			return topBar(gtx, th,
				iconBtn(th, &s.cancelBtn, widgets.IconBack, p.OnBackground),
				"Add account")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return fadeRise(gtx, t, func(gtx layout.Context) layout.Dimensions {
					return s.connectCard(gtx, env, busy, status, errText)
				})
			})
		}))
}

// connectCard renders the hero content.
func (s *AccountSetup) connectCard(gtx layout.Context, env *Env, busy bool, status, errText string) layout.Dimensions {
	th := env.Theme
	p := th.Palette
	maxW := gtx.Dp(400)
	if gtx.Constraints.Max.X < maxW {
		maxW = gtx.Constraints.Max.X
	}
	gtx.Constraints.Max.X = maxW
	gtx.Constraints.Min.X = maxW

	return layout.UniformInset(theme.XL).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// The brand artwork, theme-correct, centered.
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return widgets.BrandLogo(gtx, th, 170)
				})
			}),
			layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return th.Label(gtx, theme.Body, p.OnSurface,
					"Mail that moves like wind. Enter your address — the rest is automatic.", 0)
			}),
			layout.Rigid(layout.Spacer{Height: theme.XL}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.email.Layout(gtx, th, "Email", "you@yourdomain.com")
			}),
			layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.password.Layout(gtx, th, "App password", "from your VayuPress console")
			}),
			layout.Rigid(layout.Spacer{Height: theme.LG}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := "Connect"
				if busy {
					label = "Connecting…"
				}
				return s.connectBtn.Layout(gtx, th, widgets.ButtonPrimary, label, true, busy)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				msg, c := status, p.Subtle
				if errText != "" {
					msg, c = errText, p.Destructive
				}
				if msg == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: theme.MD}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return th.Label(gtx, theme.Caption, c, msg, 0)
					})
			}),
			layout.Rigid(layout.Spacer{Height: theme.XL}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.codeBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Body, p.Accent, "Use a setup code", 1)
				})
			}),
			layout.Rigid(layout.Spacer{Height: theme.MD}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.manualBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return th.Label(gtx, theme.Caption, p.Subtle, "Set up manually", 1)
				})
			}))
	})
}

// setError publishes an inline error from any goroutine.
func (s *AccountSetup) setError(msg string) {
	s.mu.Lock()
	s.errText = msg
	s.busy = false
	s.status = ""
	s.mu.Unlock()
}

// domainOf returns the domain part of an address, lowercased.
func domainOf(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 {
		return strings.ToLower(email[i+1:])
	}
	return ""
}
