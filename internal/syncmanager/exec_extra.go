package syncmanager

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/smtpsend"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// execFetchAttachment downloads one attachment over a short-lived
// connection and saves it into the attachments directory.
func (m *Manager) execFetchAttachment(ctx context.Context, c FetchAttachmentCmd) error {
	fail := func(err error) error {
		m.emit(AttachmentSavedEvent{MessageID: c.MessageID, Index: c.Index, Err: err})
		return err
	}
	msg, err := m.db.GetMessage(ctx, c.MessageID)
	if err != nil {
		return fail(err)
	}
	acct, err := m.db.GetAccount(ctx, msg.AccountID)
	if err != nil {
		return fail(err)
	}
	folderName, err := m.folderName(ctx, msg.AccountID, msg.FolderID)
	if err != nil {
		return fail(err)
	}

	var path string
	err = imapsync.WithConnection(ctx, ConfigFromStore(acct),
		m.credFor(acct.KeystoreAlias), func(client *imapclient.Client) error {
			if _, err := client.Select(folderName, nil).Wait(); err != nil {
				return fmt.Errorf("syncmanager: select %q: %w", folderName, err)
			}
			raw, err := imapsync.FetchRaw(client, msg.UID)
			if err != nil {
				return err
			}
			ref, data, err := mime.ExtractAttachment(raw, c.Index)
			if err != nil {
				return err
			}
			path = filepath.Join(m.attachmentsDir(),
				fmt.Sprintf("%d-%s", msg.ID, sanitizeFilename(ref.Filename)))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return fmt.Errorf("syncmanager: attachments dir: %w", err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				return fmt.Errorf("syncmanager: save attachment: %w", err)
			}
			return nil
		})
	if err != nil {
		return fail(err)
	}
	m.emit(AttachmentSavedEvent{MessageID: c.MessageID, Index: c.Index, Path: path})
	return nil
}

// execSaveDraft appends a serialized draft to the Drafts folder.
func (m *Manager) execSaveDraft(ctx context.Context, c SaveDraftCmd) error {
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		return err
	}
	folders, err := m.db.ListFolders(ctx, c.AccountID)
	if err != nil {
		return err
	}
	drafts := "Drafts"
	for _, f := range folders {
		if f.IsDrafts {
			drafts = f.FullName
		}
	}
	return imapsync.WithConnection(ctx, ConfigFromStore(acct),
		m.credFor(acct.KeystoreAlias), func(client *imapclient.Client) error {
			return imapsync.AppendMessage(client, drafts, c.Raw,
				[]imap.Flag{imap.FlagDraft})
		})
}

// execSnooze applies a local snooze; nothing changes server-side.
func (m *Manager) execSnooze(ctx context.Context, c SnoozeCmd) error {
	var until time.Time
	if c.UntilUnix > 0 {
		until = time.Unix(c.UntilUnix, 0)
	}
	return m.db.SetSnooze(ctx, c.MessageID, until)
}

// execUnsubscribe sends the RFC 2369 mailto unsubscribe message for a
// mailing-list message.
func (m *Manager) execUnsubscribe(ctx context.Context, c UnsubscribeCmd) error {
	msg, err := m.db.GetMessage(ctx, c.MessageID)
	if err != nil {
		return err
	}
	mailto, _ := mime.FirstUnsubscribeTarget(msg.ListUnsubscribe)
	if mailto == "" {
		return fmt.Errorf("syncmanager: message %d has no mailto unsubscribe", c.MessageID)
	}
	acct, err := m.db.GetAccount(ctx, msg.AccountID)
	if err != nil {
		return err
	}
	draft := smtpsend.Draft{
		FromAddr: acct.EmailAddress,
		FromName: acct.DisplayName,
		To:       []string{mailto},
		Subject:  "unsubscribe",
		TextBody: "This message was sent automatically by VayuMail to unsubscribe.\n",
	}
	raw, err := smtpsend.BuildMIME(&draft)
	if err != nil {
		return err
	}
	id, err := m.db.EnqueueOutbox(ctx, acct.ID, raw)
	if err != nil {
		return err
	}
	return m.execSend(ctx, SendCmd{OutboxID: id})
}

// appendToSent files a successfully sent message into the Sent mailbox
// so every client sees it. Failure is logged, never fatal — the mail is
// already delivered.
func (m *Manager) appendToSent(ctx context.Context, accountID int64, raw []byte) {
	acct, err := m.db.GetAccount(ctx, accountID)
	if err != nil {
		slog.Warn("sent-append: load account", "err", err)
		return
	}
	folders, err := m.db.ListFolders(ctx, accountID)
	if err != nil {
		slog.Warn("sent-append: list folders", "err", err)
		return
	}
	sent := ""
	for _, f := range folders {
		if f.IsSent {
			sent = f.FullName
		}
	}
	if sent == "" {
		slog.Info("sent-append: account has no Sent folder")
		return
	}
	err = imapsync.WithConnection(ctx, ConfigFromStore(acct),
		m.credFor(acct.KeystoreAlias), func(client *imapclient.Client) error {
			return imapsync.AppendMessage(client, sent, raw,
				[]imap.Flag{imap.FlagSeen})
		})
	if err != nil {
		slog.Warn("sent-append failed", "err", err)
	}
}

// folderName resolves a folder ID to its full IMAP name.
func (m *Manager) folderName(ctx context.Context, accountID, folderID int64) (string, error) {
	folders, err := m.db.ListFolders(ctx, accountID)
	if err != nil {
		return "", err
	}
	for _, f := range folders {
		if f.ID == folderID {
			return f.FullName, nil
		}
	}
	return "", store.ErrNotFound
}

// sanitizeFilename strips path separators and control characters from
// attachment names before they touch the filesystem.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	return strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
}
