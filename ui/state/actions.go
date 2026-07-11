package state

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/smtpsend"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

// UnifiedFolderID is the sentinel folder ID for the "All inboxes" view.
const UnifiedFolderID int64 = -1

// SendOptions carries the PGP choices made in the composer.
type SendOptions struct {
	Encrypt bool
	Sign    bool
	Keyring *pgp.Keyring
}

// EnqueueDraft serializes a draft into the outbox asynchronously and
// arms the undo-send window: the message leaves only after 10 seconds,
// and tapping Undo recalls it (the outbox row is deleted before any
// connection opens).
func (s *AppState) EnqueueDraft(draft smtpsend.Draft, opts SendOptions) {
	acct, ok := s.CurrentAccount()
	if !ok {
		s.notify("No account configured")
		return
	}
	go func() {
		raw, err := buildOutbound(&draft, opts)
		if err != nil {
			slog.Error("build draft", "err", err)
			s.notify("Could not build message: " + err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		id, err := s.db.EnqueueOutbox(ctx, acct.ID, raw)
		if err != nil {
			slog.Error("enqueue draft", "err", err)
			s.notify("Could not queue message")
			return
		}
		s.armUndoSend(id)
	}()
}

// armUndoSend shows the undo snackbar and dispatches the send only when
// the window expires un-undone.
func (s *AppState) armUndoSend(outboxID int64) {
	if s.NotifyUndo == nil {
		// No undo UI wired (headless): send immediately.
		s.Send(syncmanager.SendCmd{OutboxID: outboxID})
		return
	}
	s.NotifyUndo("Sending…",
		func() { // undo: recall the message before it leaves
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.db.MarkOutboxSent(ctx, outboxID); err != nil {
					slog.Error("recall message", "err", err)
					s.notify("Could not recall — message may send")
					return
				}
				s.notify("Send cancelled")
			}()
		},
		func() { // window expired: really send
			s.Send(syncmanager.SendCmd{OutboxID: outboxID})
		})
}

// buildOutbound serializes a draft, wrapping it in PGP/MIME when the
// composer toggles ask for it.
func buildOutbound(draft *smtpsend.Draft, opts SendOptions) ([]byte, error) {
	if !opts.Encrypt && !opts.Sign {
		return smtpsend.BuildMIME(draft)
	}
	if opts.Keyring == nil {
		return nil, errNoKeys
	}
	if opts.Encrypt {
		signer := ""
		if opts.Sign {
			signer = draft.FromAddr
		}
		// Encrypt-to-self when the sender's own key is known, so the copy
		// filed to Sent stays readable to its author.
		rcpts := draft.Recipients()
		if opts.Keyring.HasKeyFor(draft.FromAddr) {
			rcpts = append(rcpts, draft.FromAddr)
		}
		ciphertext, err := opts.Keyring.Encrypt(
			[]byte(draft.TextBody), rcpts, signer)
		if err != nil {
			return nil, err
		}
		return smtpsend.BuildPGPEncrypted(draft, ciphertext)
	}
	// Sign-only: full RFC 3156 multipart/signed is tracked in
	// COMPLIANCE-TRACKER.md; v1.1 refuses rather than pretending.
	return nil, errSignOnly
}

// Snooze hides a message until tomorrow morning (08:00 local).
func (s *AppState) Snooze(msg store.Message) {
	tomorrow := time.Now().AddDate(0, 0, 1)
	until := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(),
		8, 0, 0, 0, time.Local)
	s.Send(syncmanager.SnoozeCmd{MessageID: msg.ID, UntilUnix: until.Unix()})
	s.notify("Snoozed until tomorrow 8:00")
	s.Refresh()
}

// Unsubscribe acts on a mailing-list message: mailto targets are handled
// end-to-end; https-only targets are surfaced for the user.
// It returns the https URL when the caller must present it.
func (s *AppState) Unsubscribe(msg store.Message) (httpsURL string) {
	mailto, url := unsubscribeTargets(msg.ListUnsubscribe)
	if mailto != "" {
		s.Send(syncmanager.UnsubscribeCmd{MessageID: msg.ID})
		s.notify("Unsubscribe request sent")
		return ""
	}
	return url
}

// unsubscribeTargets mirrors mime.FirstUnsubscribeTarget without pulling
// the mime package into the state layer's public surface.
func unsubscribeTargets(header string) (mailto, url string) {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(strings.Trim(strings.TrimSpace(part), "<>"))
		lower := strings.ToLower(part)
		switch {
		case strings.HasPrefix(lower, "mailto:") && mailto == "":
			addr := part[len("mailto:"):]
			if i := strings.IndexByte(addr, '?'); i >= 0 {
				addr = addr[:i]
			}
			mailto = addr
		case strings.HasPrefix(lower, "https://") && url == "":
			url = part
		}
	}
	return mailto, url
}

// DownloadAttachment requests one attachment; completion arrives as an
// AttachmentSavedEvent surfaced through the snackbar.
func (s *AppState) DownloadAttachment(messageID int64, index int) {
	s.Send(syncmanager.FetchAttachmentCmd{MessageID: messageID, Index: index})
	s.notify("Downloading attachment…")
}

// SetDeviceID persists the device ID a VayuPress server granted this
// install during onboarding (ADR-0011), keyed per address, so the device
// can be cross-referenced in the web console later. Only the public ID
// is stored here — the device password is a credential and goes to the
// platform keystore, never SQLite (Rule 6).
func (s *AppState) SetDeviceID(email, deviceID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.db.SetSetting(ctx, store.SettingDeviceIDPrefix+email, deviceID); err != nil {
			slog.Error("persist device id", "err", err)
		}
	}()
}
