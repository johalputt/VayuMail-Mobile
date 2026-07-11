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
	case SyncFolderCmd:
		return m.execSyncFolder(ctx, c)
	case AddAccountCmd:
		return m.execAddAccount(ctx, c)
	case RemoveAccountCmd:
		return m.execRemoveAccount(ctx, c)
	case UpdateCredentialCmd:
		return m.execUpdateCredential(ctx, c)
	case SyncPrivateKeyCmd:
		return m.execSyncPrivateKey(ctx, c)
	case RefetchMessageCmd:
		return m.execRefetchMessage(ctx, c)
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
	dest := *destFolder
	cfg := ConfigFromStore(acct)
	return imapsync.WithConnection(ctx, cfg, m.credFor(acct.KeystoreAlias),
		func(client *imapclient.Client) error {
			// Server move first: if it fails, the local copy is untouched
			// and the message correctly stays in its source folder.
			if err := imapsync.MoveUID(client, srcFolder.FullName, msg.UID, dest.FullName); err != nil {
				return err
			}
			// The move succeeded; drop the local source row.
			if err := m.db.DeleteLocalMessage(ctx, msg.ID); err != nil {
				return err
			}
			// If the destination folder is already cached, pull the moved
			// message in so it appears there immediately. When the folder
			// has never synced we skip this to avoid fetching it whole;
			// it will appear the first time the user opens it.
			highest, err := m.db.HighestUID(ctx, dest.ID)
			if err != nil {
				return err
			}
			if highest == 0 {
				return nil
			}
			selected, err := client.Select(dest.FullName, nil).Wait()
			if err != nil {
				return err
			}
			return imapsync.SyncFolder(ctx, client, m.db,
				m.eventsFor(acct.ID), acct.ID, dest, selected)
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
		// Permanent delete. Hide it locally now (optimistic), expunge on
		// the server, then drop the local row on success.
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
				if err := imapsync.DeleteUID(client, current.FullName, msg.UID); err != nil {
					// Restore visibility: the message is still on the server.
					if rerr := m.db.SetDeleted(ctx, msg.ID, false); rerr != nil {
						slog.Warn("restore after failed delete", "err", rerr)
					}
					return err
				}
				return m.db.DeleteLocalMessage(ctx, msg.ID)
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

// execSyncNow runs one full folder-discovery and then syncs every folder
// over a short-lived connection. Syncing all folders (not just INBOX)
// means Sent, Archive, and mail moved on the server or sent from another
// client show up here too.
func (m *Manager) execSyncNow(ctx context.Context, c SyncNowCmd) error {
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		return err
	}
	// Bracket the whole run so the UI can show activity from the first
	// frame and reliably reload when it ends — even when the sync finds
	// nothing new and therefore emits no per-message events.
	m.emit(SyncStartedEvent{AccountID: acct.ID})
	defer func() { m.emit(SyncFinishedEvent{AccountID: acct.ID, Err: err}) }()
	cfg := ConfigFromStore(acct)
	err = imapsync.WithConnection(ctx, cfg, m.credFor(acct.KeystoreAlias),
		func(client *imapclient.Client) error {
			folders, err := imapsync.SyncFolders(ctx, client, m.db, acct.ID)
			if err != nil {
				return err
			}
			for _, folder := range folders {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				selected, err := client.Select(folder.FullName, nil).Wait()
				if err != nil {
					slog.Warn("sync-now: select folder failed",
						"folder", folder.FullName, "err", err)
					continue
				}
				if err := imapsync.SyncFolder(ctx, client, m.db,
					m.eventsFor(acct.ID), acct.ID, folder, selected); err != nil {
					slog.Warn("sync-now: sync folder failed",
						"folder", folder.FullName, "err", err)
				}
			}
			return nil
		})
	return err
}

// execRefetchMessage re-downloads one cached message's body and updates
// its row — recovery for bodies stored by an older parser. The outcome
// is always reported as a MessageRefetchedEvent.
func (m *Manager) execRefetchMessage(ctx context.Context, c RefetchMessageCmd) error {
	report := func(err error) error {
		m.emit(MessageRefetchedEvent{AccountID: c.AccountID,
			MessageID: c.MessageID, Err: err})
		return err
	}
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		return report(err)
	}
	msg, err := m.db.GetMessage(ctx, c.MessageID)
	if err != nil {
		return report(err)
	}
	folder, err := m.db.GetFolder(ctx, msg.FolderID)
	if err != nil {
		return report(err)
	}
	cfg := ConfigFromStore(acct)
	return report(imapsync.WithConnection(ctx, cfg, m.credFor(acct.KeystoreAlias),
		func(client *imapclient.Client) error {
			return imapsync.RefetchMessage(ctx, client, m.db, folder, msg)
		}))
}

// execSyncFolder syncs a single folder over a short-lived connection.
func (m *Manager) execSyncFolder(ctx context.Context, c SyncFolderCmd) error {
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		return err
	}
	folder, err := m.db.GetFolder(ctx, c.FolderID)
	if err != nil {
		return err
	}
	cfg := ConfigFromStore(acct)
	return imapsync.WithConnection(ctx, cfg, m.credFor(acct.KeystoreAlias),
		func(client *imapclient.Client) error {
			selected, err := client.Select(folder.FullName, nil).Wait()
			if err != nil {
				return err
			}
			return imapsync.SyncFolder(ctx, client, m.db,
				m.eventsFor(acct.ID), acct.ID, folder, selected)
		})
}
