package state

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

var (
	errNoKeys   = errors.New("no PGP keys configured")
	errSignOnly = errors.New("sign-only mail is not supported yet — enable encryption too")
)

// Keyring returns the app keyring (loaded from the store at startup).
func (s *AppState) Keyring() *pgp.Keyring { return s.keyring }

// loadPGPKeys imports every persisted key into the in-memory keyring at
// startup and mirrors stored trust levels.
func (s *AppState) loadPGPKeys(ctx context.Context) {
	keys, err := s.db.ListPGPKeys(ctx)
	if err != nil {
		slog.Error("load pgp keys", "err", err)
		return
	}
	for _, k := range keys {
		if _, err := s.keyring.ImportArmored([]byte(k.Armored)); err != nil {
			slog.Warn("import stored pgp key", "fingerprint", k.Fingerprint, "err", err)
			continue
		}
		if err := s.keyring.SetTrust(k.Fingerprint, pgp.TrustLevel(k.TrustLevel)); err != nil {
			slog.Warn("restore pgp trust", "fingerprint", k.Fingerprint, "err", err)
		}
	}
}

// ImportPGPKey imports an armored key (public or private) into the
// keyring and persists it so it survives restarts.
func (s *AppState) ImportPGPKey(armored string) {
	go func() {
		fingerprints, err := s.keyring.ImportArmored([]byte(armored))
		if err != nil {
			s.notify("Key import failed: " + err.Error())
			return
		}
		s.persistKeyring(fingerprints, armored)
		s.notify("Imported " + plural(len(fingerprints), "key"))
		s.Refresh()
	}()
}

// DiscoverPGPKey performs a WKD lookup for an email address and imports
// the discovered key. User-initiated only.
func (s *AppState) DiscoverPGPKey(email string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		entities, err := pgp.DiscoverWKD(ctx, http.DefaultClient, email)
		if err != nil {
			s.notify("No key found for " + email)
			slog.Info("wkd lookup failed", "email", email, "err", err)
			return
		}
		fingerprints := s.keyring.ImportEntities(entities)
		for i, fp := range fingerprints {
			armored, err := s.keyring.ExportPublicArmored(fp)
			if err != nil {
				continue
			}
			_ = i
			s.storeKeyRow(fp, string(armored), false)
		}
		s.notify("Key found and imported for " + email)
		s.Refresh()
	}()
}

// SetKeyDirectoryURL persists the VayuPress PGP key-directory base URL
// (empty clears it) and refreshes the snapshot.
func (s *AppState) SetKeyDirectoryURL(rawURL string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.db.SetSetting(ctx, store.SettingPGPKeyDirectoryURL, strings.TrimSpace(rawURL)); err != nil {
			s.notify("Could not save key-directory URL")
			return
		}
		s.notify("Key-directory URL saved")
		s.Refresh()
	}()
}

// SyncPGPFromDirectory pulls the whole VayuPress key directory and imports
// every public key it returns, persisting each so it survives restarts.
// User-initiated (Settings → Sync keys). Never contacts a directory the
// user has not configured.
func (s *AppState) SyncPGPFromDirectory() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		baseURL, err := s.db.GetSetting(ctx, store.SettingPGPKeyDirectoryURL)
		if err != nil || baseURL == "" {
			s.notify("Set a VayuPress key-directory URL first")
			return
		}
		keys, err := pgp.FetchKeyDirectory(ctx, http.DefaultClient, baseURL)
		if err != nil {
			s.notify("Key sync failed: " + err.Error())
			slog.Info("pgp directory sync failed", "err", err)
			return
		}
		imported := 0
		for _, dk := range keys {
			fps, err := s.keyring.ImportArmored([]byte(dk.Armored))
			if err != nil {
				slog.Warn("import directory key", "email", dk.Email, "err", err)
				continue
			}
			for _, fp := range fps {
				armored := dk.Armored
				if pub, err := s.keyring.ExportPublicArmored(fp); err == nil {
					armored = string(pub)
				}
				s.storeKeyRow(fp, armored, false)
				imported++
			}
		}
		s.notify("Synced " + plural(imported, "key") + " from VayuPress")
		s.Refresh()
	}()
}

// SetPGPTrust cycles/persists the trust level for a fingerprint.
func (s *AppState) SetPGPTrust(fingerprint string, level int) {
	go func() {
		if err := s.keyring.SetTrust(fingerprint, pgp.TrustLevel(level)); err != nil {
			slog.Warn("set trust", "err", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.db.SetPGPTrust(ctx, fingerprint, level); err != nil {
			slog.Error("persist trust", "err", err)
		}
		s.Refresh()
	}()
}

// DeletePGPKey removes a stored key. The in-memory keyring drops it on
// next restart; encryption to that identity stops immediately after the
// snapshot refresh.
func (s *AppState) DeletePGPKey(fingerprint string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.db.DeletePGPKey(ctx, fingerprint); err != nil {
			s.notify("Could not delete key")
			return
		}
		s.notify("Key deleted")
		s.Refresh()
	}()
}

// persistKeyring saves imported key material to the store.
func (s *AppState) persistKeyring(fingerprints []string, armored string) {
	isPrivate := strings.Contains(armored, "PRIVATE KEY BLOCK")
	for _, fp := range fingerprints {
		stored := armored
		if !isPrivate {
			if pub, err := s.keyring.ExportPublicArmored(fp); err == nil {
				stored = string(pub)
			}
		}
		s.storeKeyRow(fp, stored, isPrivate)
	}
}

func (s *AppState) storeKeyRow(fingerprint, armored string, isPrivate bool) {
	email := ""
	if e, err := s.keyring.EmailForFingerprint(fingerprint); err == nil {
		email = e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.db.UpsertPGPKey(ctx, &store.PGPKey{
		Fingerprint: fingerprint,
		Email:       email,
		Armored:     armored,
		IsPrivate:   isPrivate,
	})
	if err != nil {
		slog.Error("persist pgp key", "err", err)
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strings.Join([]string{itoa(n), noun + "s"}, " ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
