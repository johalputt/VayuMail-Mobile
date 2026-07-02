package imapsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

const (
	// backoffInitial through backoffMax implement the reconnect schedule:
	// 5s, 10s, 20s, 40s ... capped at 300s.
	backoffInitial = 5 * time.Second
	backoffMax     = 300 * time.Second
	// pollInterval drives the fallback loop for servers without IDLE.
	pollInterval = 60 * time.Second
)

// RunIDLE holds a live IMAP connection on one folder for the lifetime of
// ctx: initial incremental sync, then IDLE, reacting to every unilateral
// server response with a delta sync. Network errors reconnect with
// exponential backoff; authentication errors abort immediately (the user
// must re-provision — retrying a bad credential can lock the account).
//
// cred fetches the password from the platform keystore at connection time;
// the password lives only on the stack of one dial attempt (Rule 6).
func RunIDLE(ctx context.Context, cfg account.Config, cred func() (string, error), folder string, db *store.DB, ev Events, accountID int64) error {
	backoff := backoffInitial
	for {
		err := runSession(ctx, cfg, cred, folder, db, ev, accountID)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, ErrAuth) {
			if ev.AuthError != nil {
				ev.AuthError(err)
			}
			return err
		}
		if ev.Connection != nil {
			ev.Connection(false)
		}
		slog.Warn("imap session ended, reconnecting",
			"account", accountID, "folder", folder,
			"backoff", backoff, "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > backoffMax {
			backoff = backoffMax
		}
	}
}

// runSession is one connect-sync-idle cycle. Any returned error tears the
// connection down; RunIDLE decides whether to retry.
func runSession(ctx context.Context, cfg account.Config, cred func() (string, error), folderName string, db *store.DB, ev Events, accountID int64) error {
	password, err := cred()
	if err != nil {
		return fmt.Errorf("imapsync: fetch credential: %w", err)
	}
	client, notify, err := Dial(ctx, cfg, password)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Debug("imap close", "err", err)
		}
	}()

	if ev.Connection != nil {
		ev.Connection(true)
	}

	if _, err := SyncFolders(ctx, client, db, accountID); err != nil {
		return err
	}
	if ev.Folders != nil {
		ev.Folders()
	}

	folder, err := db.GetFolderByFullName(ctx, accountID, folderName)
	if err != nil {
		return fmt.Errorf("imapsync: folder %q not in store: %w", folderName, err)
	}
	selected, err := client.Select(folderName, nil).Wait()
	if err != nil {
		return fmt.Errorf("imapsync: select %q: %w", folderName, err)
	}
	if err := SyncFolder(ctx, client, db, ev, accountID, folder, selected); err != nil {
		return err
	}

	if !SupportsIdle(client) {
		return pollLoop(ctx, client, db, ev, accountID, folderName)
	}
	return idleLoop(ctx, client, notify, db, ev, accountID, folderName)
}

// idleLoop keeps the connection in IDLE, breaking out for a delta sync on
// every unilateral response. The go-imap IdleCommand re-issues IDLE every
// 28 minutes internally, satisfying the 29-minute keepalive requirement.
func idleLoop(ctx context.Context, client *imapclient.Client, notify <-chan Notification, db *store.DB, ev Events, accountID int64, folderName string) error {
	for {
		idle, err := client.Idle()
		if err != nil {
			return fmt.Errorf("imapsync: enter idle: %w", err)
		}
		idleErr := make(chan error, 1)
		go func() { idleErr <- idle.Wait() }()

		select {
		case <-ctx.Done():
			// Send DONE cleanly, then log out.
			if err := idle.Close(); err != nil {
				slog.Debug("idle close", "err", err)
			}
			<-idleErr
			if err := client.Logout().Wait(); err != nil {
				slog.Debug("imap logout", "err", err)
			}
			return ctx.Err()

		case err := <-idleErr:
			if err != nil {
				return fmt.Errorf("imapsync: idle: %w", err)
			}
			// IDLE ended without an error or a Close: re-enter.

		case n := <-notify:
			if err := idle.Close(); err != nil {
				return fmt.Errorf("imapsync: exit idle: %w", err)
			}
			<-idleErr
			if err := handleNotification(ctx, client, db, ev, accountID, folderName, n); err != nil {
				return err
			}
		}
	}
}

// pollLoop is the fallback for servers without IDLE: NOOP plus delta sync
// on a fixed cadence.
func pollLoop(ctx context.Context, client *imapclient.Client, db *store.DB, ev Events, accountID int64, folderName string) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := client.Logout().Wait(); err != nil {
				slog.Debug("imap logout", "err", err)
			}
			return ctx.Err()
		case <-ticker.C:
			if err := client.Noop().Wait(); err != nil {
				return fmt.Errorf("imapsync: noop: %w", err)
			}
			if err := deltaSync(ctx, client, db, ev, accountID, folderName); err != nil {
				return err
			}
		}
	}
}

// handleNotification maps one unilateral response onto the matching
// reconciliation, then returns so the caller re-enters IDLE.
func handleNotification(ctx context.Context, client *imapclient.Client, db *store.DB, ev Events, accountID int64, folderName string, n Notification) error {
	switch {
	case n.ExpungedSeq != 0:
		return reconcileExpunged(ctx, client, db, accountID, folderName)
	case n.FlagsChanged:
		return refreshFlags(ctx, client, db, ev, accountID, folderName)
	default:
		return deltaSync(ctx, client, db, ev, accountID, folderName)
	}
}

// deltaSync fetches messages that arrived since the last sync.
func deltaSync(ctx context.Context, client *imapclient.Client, db *store.DB, ev Events, accountID int64, folderName string) error {
	folder, err := db.GetFolderByFullName(ctx, accountID, folderName)
	if err != nil {
		return err
	}
	// The folder is still selected; reuse the stored UIDVALIDITY and let
	// EXISTS drive the fetch window.
	selected := &imap.SelectData{
		UIDValidity: folder.UIDValidity,
		NumMessages: 1, // non-zero: force the UID range fetch
	}
	return SyncFolder(ctx, client, db, ev, accountID, folder, selected)
}

// refreshFlags re-reads the flags of every cached message in the folder
// and applies changes locally. Flags-only FETCH is cheap even for large
// folders.
func refreshFlags(ctx context.Context, client *imapclient.Client, db *store.DB, ev Events, accountID int64, folderName string) error {
	folder, err := db.GetFolderByFullName(ctx, accountID, folderName)
	if err != nil {
		return err
	}
	var uidSet imap.UIDSet
	uidSet.AddRange(1, 0)
	bufs, err := client.Fetch(uidSet, &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		return fmt.Errorf("imapsync: refresh flags: %w", err)
	}
	for _, buf := range bufs {
		msg := store.Message{
			AccountID: accountID,
			FolderID:  folder.ID,
			UID:       uint32(buf.UID),
			Flags:     joinFlags(buf.Flags),
			IsRead:    hasFlag(buf.Flags, imap.FlagSeen),
			IsFlagged: hasFlag(buf.Flags, imap.FlagFlagged),
		}
		if err := db.UpdateMessageFlags(ctx, &msg); err != nil {
			return err
		}
		if ev.FlagChange != nil {
			ev.FlagChange(uint32(buf.UID), splitFlags(msg.Flags))
		}
	}
	return nil
}

// reconcileExpunged removes local rows whose UIDs no longer exist on the
// server. Sequence-number bookkeeping is deliberately avoided: one UID
// inventory fetch is simpler and immune to desync.
func reconcileExpunged(ctx context.Context, client *imapclient.Client, db *store.DB, accountID int64, folderName string) error {
	folder, err := db.GetFolderByFullName(ctx, accountID, folderName)
	if err != nil {
		return err
	}
	var uidSet imap.UIDSet
	uidSet.AddRange(1, 0)
	bufs, err := client.Fetch(uidSet, &imap.FetchOptions{UID: true}).Collect()
	if err != nil {
		return fmt.Errorf("imapsync: uid inventory: %w", err)
	}
	live := make(map[uint32]bool, len(bufs))
	for _, buf := range bufs {
		live[uint32(buf.UID)] = true
	}
	return db.DeleteMessagesNotIn(ctx, folder.ID, live)
}
