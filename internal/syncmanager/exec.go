package syncmanager

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/smtpsend"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// commandLoop drains cmdCh until shutdown. Commands run sequentially:
// each one is a short-lived, self-contained network operation, and
// ordering preserves user intent (archive then delete behaves as tapped).
func (m *Manager) commandLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case cmd := <-m.cmdCh:
			if err := m.handleCmd(m.ctx, cmd); err != nil && m.ctx.Err() == nil {
				slog.Error("command failed",
					"cmd", fmt.Sprintf("%T", cmd), "err", err)
			}
		}
	}
}

func (m *Manager) handleCmd(ctx context.Context, cmd Cmd) error {
	switch c := cmd.(type) {
	case MoveCmd:
		return m.execMove(ctx, c)
	case DeleteCmd:
		return m.execDelete(ctx, c)
	case MarkCmd:
		return m.execMark(ctx, c)
	case SendCmd:
		return m.execSend(ctx, c)
	case SyncNowCmd:
		return m.execSyncNow(ctx, c)
	case AddAccountCmd:
		return m.execAddAccount(ctx, c)
	case FetchAttachmentCmd:
		return m.execFetchAttachment(ctx, c)
	case SaveDraftCmd:
		return m.execSaveDraft(ctx, c)
	case SnoozeCmd:
		return m.execSnooze(ctx, c)
	case UnsubscribeCmd:
		return m.execUnsubscribe(ctx, c)
	default:
		return fmt.Errorf("syncmanager: unknown command %T", cmd)
	}
}

// execMove applies the move locally first (the UI updates instantly),
// then replays it on the server over a short-lived connection.
func (m *Manager) execMove(ctx context.Context, c MoveCmd) error {
	msg, err := m.db.GetMessage(ctx, c.MessageID)
	if err != nil {
		return err
	}
	acct, err := m.db.GetAccount(ctx, msg.AccountID)
	if err != nil {
		return err
	}
	src, err := m.db.ListFolders(ctx, msg.AccountID)
	if err != nil {
		return err
	}
	var srcFolder, destFolder *store.Folder
	for i := range src {
		if src[i].ID == msg.FolderID {
			srcFolder = &src[i]
		}
		if src[i].FullName == c.DestFolder {
			destFolder = &src[i]
		}
	}
	if srcFolder == nil || destFolder == nil {
		return fmt.Errorf("syncmanager: move: unknown folder")
	}
	if err := m.db.MoveMessage(ctx, msg.ID, destFolder.ID); err != nil {
		return err
	}
	cfg := ConfigFromStore(acct)
	return imapsync.WithConnection(ctx, cfg, m.credFor(acct.KeystoreAlias),
		func(client *imapclient.Client) error {
			return imapsync.MoveUID(client, srcFolder.FullName, msg.UID, destFolder.FullName)
		})
}

// execDelete moves the message to Trash, or expunges it when it is
// already there.
func (m *Manager) execDelete(ctx context.Context, c DeleteCmd) error {
	msg, err := m.db.GetMessage(ctx, c.MessageID)
	if err != nil {
		return err
	}
	folders, err := m.db.ListFolders(ctx, msg.AccountID)
	if err != nil {
		return err
	}
	var current, trash *store.Folder
	for i := range folders {
		if folders[i].ID == msg.FolderID {
			current = &folders[i]
		}
		if folders[i].IsTrash {
			trash = &folders[i]
		}
	}
	if current == nil {
		return fmt.Errorf("syncmanager: delete: unknown folder")
	}

	if current.IsTrash || trash == nil {
		// Permanent delete.
		acct, err := m.db.GetAccount(ctx, msg.AccountID)
		if err != nil {
			return err
		}
		if err := m.db.SetDeleted(ctx, msg.ID, true); err != nil {
			return err
		}
		cfg := ConfigFromStore(acct)
		return imapsync.WithConnection(ctx, cfg, m.credFor(acct.KeystoreAlias),
			func(client *imapclient.Client) error {
				return imapsync.DeleteUID(client, current.FullName, msg.UID)
			})
	}
	return m.execMove(ctx, MoveCmd{MessageID: c.MessageID, DestFolder: trash.FullName})
}

// execMark applies a flag change locally, then on the server.
func (m *Manager) execMark(ctx context.Context, c MarkCmd) error {
	msg, err := m.db.GetMessage(ctx, c.MessageID)
	if err != nil {
		return err
	}
	switch strings.ToLower(c.Flag) {
	case `\seen`:
		if err := m.db.SetRead(ctx, msg.ID, c.Set); err != nil {
			return err
		}
	case `\flagged`:
		if err := m.db.SetFlagged(ctx, msg.ID, c.Set); err != nil {
			return err
		}
	}
	acct, err := m.db.GetAccount(ctx, msg.AccountID)
	if err != nil {
		return err
	}
	folders, err := m.db.ListFolders(ctx, msg.AccountID)
	if err != nil {
		return err
	}
	var folderName string
	for _, f := range folders {
		if f.ID == msg.FolderID {
			folderName = f.FullName
		}
	}
	cfg := ConfigFromStore(acct)
	return imapsync.WithConnection(ctx, cfg, m.credFor(acct.KeystoreAlias),
		func(client *imapclient.Client) error {
			return imapsync.SetFlagUID(client, folderName, msg.UID, c.Flag, c.Set)
		})
}

// execSend attempts one outbox entry immediately, regardless of its
// scheduled next attempt.
func (m *Manager) execSend(ctx context.Context, c SendCmd) error {
	entry, err := m.db.GetOutboxEntry(ctx, c.OutboxID)
	if err != nil {
		return err
	}
	acct, err := m.db.GetAccount(ctx, entry.AccountID)
	if err != nil {
		return err
	}
	cfg := ConfigFromStore(acct)
	sendErr := smtpsend.SendEntry(ctx, cfg, m.credFor(acct.KeystoreAlias), entry)
	if sendErr == nil {
		if err := m.db.MarkOutboxSent(ctx, entry.ID); err != nil {
			return err
		}
		// File the message into Sent so every device sees it.
		m.appendToSent(ctx, entry.AccountID, entry.RawMessage)
	} else {
		next := time.Now().Add(time.Minute << uint(entry.RetryCount))
		if err := m.db.MarkOutboxFailed(ctx, entry.ID, sendErr, next); err != nil {
			return err
		}
	}
	m.emit(SendResultEvent{OutboxID: entry.ID, Err: sendErr})
	if sendErr != nil {
		return fmt.Errorf("syncmanager: send outbox %d: %w", entry.ID, sendErr)
	}
	return nil
}

// execSyncNow runs one full folder-discovery plus INBOX sync over a
// short-lived connection.
func (m *Manager) execSyncNow(ctx context.Context, c SyncNowCmd) error {
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		return err
	}
	cfg := ConfigFromStore(acct)
	return imapsync.WithConnection(ctx, cfg, m.credFor(acct.KeystoreAlias),
		func(client *imapclient.Client) error {
			if _, err := imapsync.SyncFolders(ctx, client, m.db, acct.ID); err != nil {
				return err
			}
			folder, err := m.db.GetFolderByFullName(ctx, acct.ID, "INBOX")
			if err != nil {
				return err
			}
			selected, err := client.Select("INBOX", nil).Wait()
			if err != nil {
				return err
			}
			return imapsync.SyncFolder(ctx, client, m.db,
				m.eventsFor(acct.ID), acct.ID, folder, selected)
		})
}

// execAddAccount stores the credential in the platform keystore, wipes
// the in-memory copy, persists the account row, and starts sync.
func (m *Manager) execAddAccount(ctx context.Context, c AddAccountCmd) error {
	if err := c.Config.Validate(); err != nil {
		return err
	}
	if err := m.ks.Store(c.Config.KeystoreAlias, c.Credential); err != nil {
		return fmt.Errorf("syncmanager: store credential: %w", err)
	}
	for i := range c.Credential {
		c.Credential[i] = 0
	}
	row := store.Account{
		DisplayName:   c.Config.DisplayName,
		EmailAddress:  c.Config.EmailAddress,
		IMAPHost:      c.Config.IMAPHost,
		IMAPPort:      c.Config.IMAPPort,
		IMAPTLS:       string(c.Config.IMAPTLS),
		SMTPHost:      c.Config.SMTPHost,
		SMTPPort:      c.Config.SMTPPort,
		SMTPTLS:       string(c.Config.SMTPTLS),
		Username:      c.Config.Username,
		KeystoreAlias: c.Config.KeystoreAlias,
	}
	if _, err := m.db.InsertAccount(ctx, &row); err != nil {
		return err
	}
	m.startAccount(row)
	return nil
}
