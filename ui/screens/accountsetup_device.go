package screens

// accountsetup_device.go — the device-approval half of direct connect
// (ADR-0011). After autoconfig discovery the app registers itself as a
// named device with the account's VayuPress server; an approved grant
// syncs immediately with its per-device password, a pending grant parks
// the card in a wait state that polls until a human approves the device
// in the web console. Servers without the endpoint (ErrNoDeviceEndpoint)
// get exactly the pre-approval behavior. Split from
// accountsetup_connect.go to keep every file within the 400-line limit
// (Rule 10). All of this runs off the UI thread; progress reaches the
// card through the mutex-guarded busy/status/errText fields plus
// Refresh() wake-ups (Rule 5).

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

const (
	// devicePollInterval spaces the approval-status polls.
	devicePollInterval = 5 * time.Second
	// deviceWaitTimeout bounds the wait: after this the poller gives up
	// with a retry hint rather than spinning forever.
	deviceWaitTimeout = 10 * time.Minute
)

// registerAndAdd runs the device-approval leg of connect() in the same
// goroutine, after discovery succeeded. ctx is the short connect
// context; the (potentially long) approval wait uses its own.
func (s *AccountSetup) registerAndAdd(ctx context.Context, env *Env, cfg *account.Config, email, pass string) {
	s.mu.Lock()
	s.status = "Registering this device with " + domainOf(email) + "…"
	s.mu.Unlock()
	env.State.Refresh()

	grant, err := account.RegisterDevice(ctx, http.DefaultClient, email, pass,
		deviceName(), runtime.GOOS)
	switch {
	case errors.Is(err, account.ErrNoDeviceEndpoint):
		// Older VayuPress without device approval: the typed password is
		// the mailbox credential, exactly as before.
		s.addAccount(env, cfg, email, pass)
		return
	case errors.Is(err, account.ErrDeviceCredentials):
		s.setError("The server rejected this email or password — check them and try again.")
		return
	case err != nil:
		s.setError("Couldn't register this device — check your connection and try again.")
		return
	}
	// Persist the public device ID for later display; the device password
	// only ever becomes the keystore credential (Rule 6).
	env.State.SetDeviceID(email, grant.DeviceID)
	switch grant.Status {
	case account.DeviceStatusApproved:
		s.addAccount(env, cfg, email, grant.DevicePassword)
	case account.DeviceStatusPending:
		s.awaitApproval(env, cfg, email, grant)
	default:
		s.setError("The server returned an unexpected device status — try again.")
	}
}

// awaitApproval parks the connect card in a wait state and polls the
// device's approval status every devicePollInterval until it is
// approved, blocked, the user leaves the flow (cancelWait), or
// deviceWaitTimeout passes.
func (s *AccountSetup) awaitApproval(env *Env, cfg *account.Config, email string, grant account.DeviceGrant) {
	ctx, cancel := context.WithTimeout(context.Background(), deviceWaitTimeout)
	defer cancel()
	s.mu.Lock()
	s.waitGen++
	gen := s.waitGen
	s.waitCancel = cancel
	s.status = "Waiting for device approval — open your VayuPress webmail → Mail accounts → Devices and approve this device."
	s.mu.Unlock()
	env.State.Refresh()
	defer s.clearWait(gen)

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				s.setError("No approval yet — approve this device in the web console, then tap Connect again.")
			} else {
				// The user left the flow: reset quietly.
				s.mu.Lock()
				s.busy = false
				s.status = ""
				s.mu.Unlock()
			}
			env.State.Refresh()
			return
		case <-time.After(devicePollInterval):
		}
		status, err := account.DeviceStatus(ctx, http.DefaultClient, email,
			grant.DeviceID, grant.DevicePassword)
		if err != nil {
			continue // transient — keep polling until the deadline
		}
		switch status {
		case account.DeviceStatusApproved:
			s.addAccount(env, cfg, email, grant.DevicePassword)
			return
		case account.DeviceStatusBlocked:
			s.setError("This device was blocked from the web console.")
			env.State.Refresh()
			return
		}
		// Still pending: keep waiting.
	}
}

// addAccount hands the configured account and its credential to the sync
// manager and settles the card. secret is the mailbox password on legacy
// servers or the approved device password (ADR-0011); either way it goes
// only to the keystore via the sync layer (Rule 6).
func (s *AccountSetup) addAccount(env *Env, cfg *account.Config, email, secret string) {
	env.State.Send(syncmanager.AddAccountCmd{
		Config:     *cfg,
		Credential: []byte(secret),
	})
	s.mu.Lock()
	s.busy = false
	s.status = ""
	s.mu.Unlock()
	s.password.SetText("")
	env.Snack.ShowInfo("Connected — syncing " + email)
	env.State.Refresh()
}

// cancelWait aborts a pending approval wait, if any. Called from the UI
// thread when the user leaves the connect flow; cheap and non-blocking.
func (s *AccountSetup) cancelWait() {
	s.mu.Lock()
	cancel := s.waitCancel
	s.waitCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// clearWait drops the stored cancel func when the wait that owns gen
// ends, without clobbering a newer wait's cancel.
func (s *AccountSetup) clearWait(gen int) {
	s.mu.Lock()
	if s.waitGen == gen {
		s.waitCancel = nil
	}
	s.mu.Unlock()
}

// deviceName labels this install in the server's device list, e.g.
// "VayuMail on Android".
func deviceName() string {
	g := runtime.GOOS
	if g == "" {
		return "VayuMail"
	}
	return "VayuMail on " + strings.ToUpper(g[:1]) + g[1:]
}
