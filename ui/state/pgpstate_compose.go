package state

// pgpstate_compose.go — compose-time key acquisition: when the user
// turns encryption on, missing recipient keys are fetched right then
// over WKD instead of failing the send later with a generic error.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

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
