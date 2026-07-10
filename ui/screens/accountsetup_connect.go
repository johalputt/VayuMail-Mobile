package screens

// accountsetup_connect.go — the async halves of onboarding: direct
// connect via autoconfig discovery, and signed setup-code provisioning
// (Rule 7). Split from accountsetup.go to keep every file within the
// 400-line constitutional limit (Rule 10).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

// connect runs the direct-connect flow off the UI thread: discover the
// domain's autoconfig document over HTTPS, then hand the account to the
// sync manager with the typed app password. Nothing is stored until
// discovery succeeds; the password never touches SQLite (Rule 6).
func (s *AccountSetup) connect(env *Env) {
	email := strings.TrimSpace(s.email.Text())
	pass := s.password.Text()
	if !strings.Contains(email, "@") || domainOf(email) == "" {
		s.setError("Enter a full email address")
		return
	}
	if pass == "" {
		s.setError("Enter your password or an app password")
		return
	}
	s.mu.Lock()
	s.busy = true
	s.errText = ""
	s.status = "Contacting " + domainOf(email) + "…"
	s.mu.Unlock()

	go func() {
		defer env.State.Refresh()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cfg, err := account.DiscoverAutoconfig(ctx, http.DefaultClient, email)
		if err != nil {
			s.setError("Couldn't find your server's settings — check the address, or use a setup code or manual setup.")
			return
		}
		cfg.KeystoreAlias = keystoreAlias(email)
		env.State.Send(syncmanager.AddAccountCmd{
			Config:     *cfg,
			Credential: []byte(pass),
		})
		s.mu.Lock()
		s.busy = false
		s.status = ""
		s.mu.Unlock()
		s.password.SetText("")
		env.Snack.ShowInfo("Connected — syncing " + email)
	}()
}

// provision verifies and redeems a pasted setup code off the UI thread,
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
		alias := keystoreAlias(payload.Username)
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

// keystoreAlias derives a fresh, unique credential alias for a new
// account.
func keystoreAlias(user string) string {
	return fmt.Sprintf("vayumail-%s-%d", user, time.Now().Unix())
}

// provisionErrorMessage maps typed provisioning errors onto clear,
// user-facing language. Unverified payloads never proceed (Rule 7).
func provisionErrorMessage(err error) string {
	switch {
	case errors.Is(err, account.ErrExpired):
		return "This setup code has expired — generate a fresh one"
	case errors.Is(err, account.ErrInvalidSignature):
		return "Setup code signature is invalid — do not trust this code"
	case errors.Is(err, account.ErrUnknownVersion):
		return "Setup code is from a newer VayuMail — update the app"
	case errors.Is(err, account.ErrInsecureTransport):
		return "Server offered an insecure connection — refused"
	case errors.Is(err, account.ErrInvalidPort):
		return "Setup code contains an invalid port"
	case errors.Is(err, account.ErrTokenExpired):
		return "Setup code already used — generate a fresh one"
	case errors.Is(err, account.ErrNetwork):
		return "Could not reach the mail server"
	default:
		return "Could not read this setup code"
	}
}
