package state

// pgpstate_compose.go — compose-time key acquisition: when the user
// turns encryption on, missing recipient keys are fetched right then
// over WKD instead of failing the send later with a generic error.

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

// SyncPrivateKey asks the engine to fetch this account's own private key
// from its VayuPress server so received encrypted mail can be decrypted.
func (s *AppState) SyncPrivateKey(accountID int64) {
	s.Send(syncmanager.SyncPrivateKeyCmd{AccountID: accountID})
}

// importPrivateKey imports a fetched armored private key into the keyring
// and persists it (marked private). Called from the event loop off the
// frame path.
func (s *AppState) importPrivateKey(armored, email string) {
	go func() {
		fps, err := s.keyring.ImportArmored([]byte(armored))
		if err != nil {
			slog.Warn("import private key", "email", email, "err", err)
			s.notify("Could not import your key")
			return
		}
		for _, fp := range fps {
			s.storeKeyRow(fp, armored, true)
		}
		s.notify("Your encryption key is ready — encrypted mail will now open")
		s.Refresh()
	}()
}

// MissingKeys reports which of the given addresses have no key in the
// keyring. Cheap enough for the frame loop (in-memory map lookups).
func (s *AppState) MissingKeys(addrs []string) []string {
	var missing []string
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if !s.keyring.HasKeyFor(a) {
			missing = append(missing, a)
		}
	}
	return missing
}

// EnsureKeysFor fetches keys for any of the given addresses that lack
// one, via each address's own server (WKD), then reports the addresses
// still missing through done — called from a goroutine, so the caller
// folds the result in on its next frame. Found keys are imported and
// persisted immediately.
func (s *AppState) EnsureKeysFor(addrs []string, done func(stillMissing []string)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		var missing []string
		for _, email := range s.MissingKeys(addrs) {
			if ctx.Err() != nil {
				missing = append(missing, email)
				continue
			}
			lctx, lcancel := context.WithTimeout(ctx, 15*time.Second)
			entities, err := pgp.DiscoverWKD(lctx, http.DefaultClient, email)
			lcancel()
			if err != nil {
				missing = append(missing, email)
				continue
			}
			for _, fp := range s.keyring.ImportEntities(entities) {
				if armored, aerr := s.keyring.ExportPublicArmored(fp); aerr == nil {
					s.storeKeyRow(fp, string(armored), false)
				}
			}
			if !s.keyring.HasKeyFor(email) {
				missing = append(missing, email)
			}
		}
		if done != nil {
			done(missing)
		}
		s.Refresh()
	}()
}
