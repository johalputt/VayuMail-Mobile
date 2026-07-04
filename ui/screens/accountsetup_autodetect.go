package screens

// accountsetup_autodetect.go — the "Auto-detect from email" path of the manual
// account screen, split out of accountsetup.go to keep every file within the
// 400-line constitutional limit (Rule 10). See account.DiscoverAutoconfig.

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

// autodetect fills the server and port fields from the domain's published
// VayuMail autoconfig document, so the operator types only their email and
// password. The lookup runs off the UI thread (mirroring provision); the result
// is handed to the UI thread via pendingCfg and applied on the next frame by
// applyPendingDetect, so the editors are never mutated from the goroutine.
func (s *AccountSetup) autodetect(env *Env) {
	email := strings.TrimSpace(s.email.Text())
	if email == "" {
		env.Snack.ShowInfo("Enter your email address first")
		return
	}
	env.Snack.ShowInfo("Detecting settings…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cfg, err := account.DiscoverAutoconfig(ctx, http.DefaultClient, email)
		if err != nil {
			env.Snack.ShowInfo("Couldn't detect settings — enter them manually")
			env.State.Refresh()
			return
		}
		s.mu.Lock()
		s.pendingCfg = cfg
		s.mu.Unlock()
		env.Snack.ShowInfo("Settings detected — add your password")
		env.State.Refresh()
	}()
}

// applyPendingDetect copies a completed autodetect result into the form
// editors. It runs on the UI thread (from layoutManual) so widget state is
// never touched concurrently with layout.
func (s *AccountSetup) applyPendingDetect() {
	s.mu.Lock()
	cfg := s.pendingCfg
	s.pendingCfg = nil
	s.mu.Unlock()
	if cfg == nil {
		return
	}
	s.host.SetText(cfg.IMAPHost)
	s.imapPort.SetText(strconv.Itoa(cfg.IMAPPort))
	s.smtpPort.SetText(strconv.Itoa(cfg.SMTPPort))
}
