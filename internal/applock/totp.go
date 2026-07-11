package applock

// totp.go — the optional second unlock factor: RFC 6238 TOTP against a
// shared secret enrolled from the operator's authenticator (the same
// secret VayuPress TOTP uses). The secret lives in the keystore next to
// the PIN verifier (Rule 6) and both factors share one lockout ladder,
// so guessing codes is throttled exactly like guessing PINs.

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrBadSecret is returned by SetTOTP for a secret that is not valid
// base32 or is too short to be safe.
var ErrBadSecret = errors.New("applock: TOTP secret must be at least 16 base32 characters")

const (
	// totpAlias names the keystore entry holding the raw shared secret.
	totpAlias = "vayumail-applock-totp-v1"
	// totpStep is the RFC 6238 time step.
	totpStep = 30 * time.Second
	// totpSkew accepts one step either side of now, absorbing clock
	// drift between phone and authenticator.
	totpSkew = 1
)

// SetTOTP enrolls a second factor: the base32 secret from the
// authenticator app (spaces and case are tolerated, padding optional).
// The caller should confirm one live code via VerifyTOTP before relying
// on it.
func (m *Manager) SetTOTP(ctx context.Context, secret string) error {
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return err
	}
	if err := m.ks.Store(totpAlias, key); err != nil {
		return fmt.Errorf("applock: store totp secret: %w", err)
	}
	return nil
}

// TOTPEnabled reports whether a second factor is enrolled.
func (m *Manager) TOTPEnabled(ctx context.Context) bool {
	_, err := m.ks.Fetch(totpAlias)
	return err == nil
}

// RemoveTOTP disables the second factor.
func (m *Manager) RemoveTOTP(ctx context.Context) error {
	if err := m.ks.Delete(totpAlias); err != nil {
		return fmt.Errorf("applock: delete totp secret: %w", err)
	}
	return nil
}

// VerifyTOTP checks a 6-digit code against the enrolled secret,
// accepting ±1 time step. Wrong codes feed the same lockout ladder as
// wrong PINs; while locked out the attempt is refused outright.
func (m *Manager) VerifyTOTP(ctx context.Context, code string) (bool, error) {
	if m.RetryDelay(ctx) > 0 {
		return false, ErrLockedOut
	}
	key, err := m.ks.Fetch(totpAlias)
	if err != nil {
		return false, fmt.Errorf("applock: totp secret: %w", err)
	}
	code = strings.TrimSpace(code)
	now := m.now()
	ok := false
	for skew := -totpSkew; skew <= totpSkew; skew++ {
		at := now.Add(time.Duration(skew) * totpStep)
		want := hotp(key, uint64(at.Unix())/uint64(totpStep.Seconds()))
		// Constant-time per candidate; the loop always runs all windows
		// so timing does not reveal which one matched.
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			ok = true
		}
	}
	if !ok {
		return false, m.recordFailure(ctx)
	}
	if err := m.clearLockout(ctx); err == nil {
		return true, nil
	}
	// Bookkeeping failure must not keep the rightful user out.
	return true, nil
}

// decodeTOTPSecret normalizes and decodes a user-pasted base32 secret.
func decodeTOTPSecret(s string) ([]byte, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	s = strings.TrimRight(s, "=")
	if len(s) < 16 {
		return nil, ErrBadSecret
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		return nil, ErrBadSecret
	}
	return key, nil
}

// hotp computes the RFC 4226 6-digit code for one counter value. The
// SHA-1 HMAC is the RFC-mandated algorithm every authenticator app
// implements — an interop requirement, not a collision-sensitive use.
func hotp(key []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0F
	code := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7FFFFFFF) % 1000000
	return fmt.Sprintf("%06d", code)
}
