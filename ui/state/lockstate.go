package state

// lockstate.go — app lock, notification preference, and account
// removal: the security surface of AppState. All blocking work (PBKDF2
// verification, settings writes) runs on goroutines; layout code only
// reads the snapshot (Rule 5).

import (
	"context"
	"strconv"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

// loadPrefs folds the persisted preferences into a snapshot under
// construction. Runs on the loader goroutine.
func (s *AppState) loadPrefs(ctx context.Context, next *Snapshot) {
	next.NotificationsOn = true
	if v, err := s.db.GetSetting(ctx, store.SettingNotifications); err == nil && v == "0" {
		next.NotificationsOn = false
	}
	next.NotifyPreviewOn = true
	if v, err := s.db.GetSetting(ctx, store.SettingNotifyPreview); err == nil && v == "0" {
		next.NotifyPreviewOn = false
	}
	if s.lock != nil {
		next.AppLockEnabled = s.lock.Enabled(ctx)
		next.TOTPEnabled = s.lock.TOTPEnabled(ctx)
	}
	next.AppLockTimeout = 60 // default window until the user picks one
	if v, err := s.db.GetSetting(ctx, store.SettingAppLockTimeout); err == nil && v != "" {
		if sec, cerr := strconv.Atoi(v); cerr == nil && sec > 0 {
			next.AppLockTimeout = sec
		}
	}
}

// NotificationsEnabled is read by the notifier on every event; it must
// stay a cheap snapshot read.
func (s *AppState) NotificationsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap.NotificationsOn
}

// SetNotifications persists the notifications toggle.
func (s *AppState) SetNotifications(on bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		v := "0"
		if on {
			v = "1"
		}
		if err := s.db.SetSetting(ctx, store.SettingNotifications, v); err != nil {
			s.notify("Could not save setting")
			return
		}
		s.Refresh()
	}()
}

// NotifyPreviewEnabled is read by the notifier per event; a cheap
// snapshot read.
func (s *AppState) NotifyPreviewEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap.NotifyPreviewOn
}

// SetNotifyPreview persists the notification-preview toggle.
func (s *AppState) SetNotifyPreview(on bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		v := "0"
		if on {
			v = "1"
		}
		if err := s.db.SetSetting(ctx, store.SettingNotifyPreview, v); err != nil {
			s.notify("Could not save setting")
			return
		}
		s.Refresh()
	}()
}

// UpdateCredential replaces an account's stored password; the engine
// stops sync, overwrites the keystore entry, and reconnects.
func (s *AppState) UpdateCredential(id int64, password string) {
	s.Send(syncmanager.UpdateCredentialCmd{AccountID: id, Credential: []byte(password)})
}

// RemoveAccount signs an account out: the engine stops its sync, wipes
// its credential from the keystore, and deletes its local data. The
// AccountRemovedEvent closes the loop in Apply.
func (s *AppState) RemoveAccount(id int64) {
	s.Send(syncmanager.RemoveAccountCmd{AccountID: id})
}

// LockNow gates the UI behind the PIN screen immediately.
func (s *AppState) LockNow() {
	s.mu.Lock()
	if s.lock == nil || !s.snap.AppLockEnabled {
		s.mu.Unlock()
		return
	}
	s.locked = true
	s.snap.Locked = true
	s.mu.Unlock()
	if s.invalidate != nil {
		s.invalidate()
	}
}

// MaybeAutoLock re-locks after an idle gap. Called from the frame loop
// with the interval since the previous frame — a long gap means the app
// was backgrounded or the device idle. A quietly-read foreground screen
// also stops rendering, so the floor is 30s: strict enough to matter,
// long enough to never lock mid-read on the shortest setting.
// Non-blocking: it only reads cached snapshot fields.
func (s *AppState) MaybeAutoLock(gap time.Duration) {
	const minGap = 30 * time.Second
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil || !s.snap.AppLockEnabled || s.locked {
		return
	}
	limit := time.Duration(s.snap.AppLockTimeout) * time.Second
	if limit < minGap {
		limit = minGap
	}
	if gap >= limit {
		s.locked = true
		s.snap.Locked = true
	}
}

// VerifyPIN checks the PIN off-thread and reports through done(ok,
// retryAfter, totpNext): retryAfter > 0 means locked out for that long;
// totpNext means the PIN was right but a second factor is enrolled, so
// the gate stays shut until VerifyTOTP succeeds too.
func (s *AppState) VerifyPIN(pin string, done func(ok bool, retryAfter time.Duration, totpNext bool)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ok, err := s.lock.Verify(ctx, pin)
		totpNext := false
		if ok {
			if s.lock.TOTPEnabled(ctx) {
				totpNext = true
			} else {
				s.unlock()
			}
		}
		wait := time.Duration(0)
		if err != nil || !ok {
			wait = s.lock.RetryDelay(ctx)
		}
		done(ok, wait, totpNext)
		if s.invalidate != nil {
			s.invalidate()
		}
	}()
}

// VerifyTOTP checks the 6-digit second-factor code off-thread. unlockOn
// selects whether success opens the gate (the unlock flow) or merely
// confirms the code (enrollment).
func (s *AppState) VerifyTOTP(code string, unlockOn bool, done func(ok bool, retryAfter time.Duration)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ok, err := s.lock.VerifyTOTP(ctx, code)
		if ok && unlockOn {
			s.unlock()
		}
		wait := time.Duration(0)
		if err != nil || !ok {
			wait = s.lock.RetryDelay(ctx)
		}
		done(ok, wait)
		if s.invalidate != nil {
			s.invalidate()
		}
	}()
}

// EnrollTOTP stores the pasted authenticator secret off-thread. The
// caller confirms one live code (VerifyTOTP with unlockOn=false) before
// telling the user the factor is active.
func (s *AppState) EnrollTOTP(secret string, done func(err error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := s.lock.SetTOTP(ctx, secret)
		if err == nil {
			s.Refresh()
		}
		done(err)
		if s.invalidate != nil {
			s.invalidate()
		}
	}()
}

// RemoveTOTP disables the second factor off-thread.
func (s *AppState) RemoveTOTP(done func(err error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := s.lock.RemoveTOTP(ctx)
		if err == nil {
			s.Refresh()
		}
		done(err)
		if s.invalidate != nil {
			s.invalidate()
		}
	}()
}

// unlock opens the gate.
func (s *AppState) unlock() {
	s.mu.Lock()
	s.locked = false
	s.snap.Locked = false
	s.mu.Unlock()
}

// SetPIN enrolls or replaces the app-lock PIN off-thread.
func (s *AppState) SetPIN(pin string, done func(err error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := s.lock.Set(ctx, pin)
		if err == nil {
			s.Refresh()
		}
		done(err)
		if s.invalidate != nil {
			s.invalidate()
		}
	}()
}

// RemovePIN disables the app lock off-thread. The second factor goes
// with it — TOTP without a PIN would be a gate with no first door.
func (s *AppState) RemovePIN(done func(err error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if s.lock.TOTPEnabled(ctx) {
			_ = s.lock.RemoveTOTP(ctx)
		}
		err := s.lock.Remove(ctx)
		if err == nil {
			s.mu.Lock()
			s.locked = false
			s.snap.Locked = false
			s.mu.Unlock()
			s.Refresh()
		}
		done(err)
		if s.invalidate != nil {
			s.invalidate()
		}
	}()
}

// SetAppLockTimeout persists the auto-lock idle window in seconds.
func (s *AppState) SetAppLockTimeout(sec int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.db.SetSetting(ctx, store.SettingAppLockTimeout, strconv.Itoa(sec)); err != nil {
			s.notify("Could not save setting")
			return
		}
		s.Refresh()
	}()
}

// HasAppLock reports whether the lock manager is wired (nil in headless
// tests).
func (s *AppState) HasAppLock() bool { return s.lock != nil }
