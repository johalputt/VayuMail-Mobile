package imapsync

import (
	"context"
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

// WithConnection dials, authenticates, runs fn, and logs out. It is the
// building block for short-lived command connections (move, flag,
// delete) that must not disturb the long-lived IDLE connection.
func WithConnection(ctx context.Context, cfg account.Config, cred func() (string, error), fn func(*imapclient.Client) error) error {
	password, err := cred()
	if err != nil {
		return fmt.Errorf("imapsync: fetch credential: %w", err)
	}
	client, _, err := Dial(ctx, cfg, password)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := fn(client); err != nil {
		return err
	}
	if err := client.Logout().Wait(); err != nil {
		return fmt.Errorf("imapsync: logout: %w", err)
	}
	return nil
}

// MoveUID moves one message (by UID) from srcFolder to destFolder using
// MOVE when available, falling back to COPY + STORE \Deleted + EXPUNGE.
func MoveUID(client *imapclient.Client, srcFolder string, uid uint32, destFolder string) error {
	if _, err := client.Select(srcFolder, nil).Wait(); err != nil {
		return fmt.Errorf("imapsync: select %q: %w", srcFolder, err)
	}
	uidSet := imap.UIDSetNum(imap.UID(uid))
	if client.Caps().Has(imap.CapMove) {
		if _, err := client.Move(uidSet, destFolder).Wait(); err != nil {
			return fmt.Errorf("imapsync: move uid %d: %w", uid, err)
		}
		return nil
	}
	if _, err := client.Copy(uidSet, destFolder).Wait(); err != nil {
		return fmt.Errorf("imapsync: copy uid %d: %w", uid, err)
	}
	if err := storeFlag(client, uidSet, imap.FlagDeleted, true); err != nil {
		return err
	}
	if err := client.Expunge().Close(); err != nil {
		return fmt.Errorf("imapsync: expunge: %w", err)
	}
	return nil
}

// SetFlagUID sets or clears one flag on one message in folder.
func SetFlagUID(client *imapclient.Client, folder string, uid uint32, flag string, set bool) error {
	if _, err := client.Select(folder, nil).Wait(); err != nil {
		return fmt.Errorf("imapsync: select %q: %w", folder, err)
	}
	return storeFlag(client, imap.UIDSetNum(imap.UID(uid)), imap.Flag(flag), set)
}

// DeleteUID flags one message \Deleted and expunges it — the permanent
// delete used from Trash. Deleting from other folders should MoveUID to
// Trash instead.
func DeleteUID(client *imapclient.Client, folder string, uid uint32) error {
	if _, err := client.Select(folder, nil).Wait(); err != nil {
		return fmt.Errorf("imapsync: select %q: %w", folder, err)
	}
	uidSet := imap.UIDSetNum(imap.UID(uid))
	if err := storeFlag(client, uidSet, imap.FlagDeleted, true); err != nil {
		return err
	}
	if err := client.Expunge().Close(); err != nil {
		return fmt.Errorf("imapsync: expunge uid %d: %w", uid, err)
	}
	return nil
}

// AppendMessage appends raw message bytes to a folder (used to file sent
// mail into the Sent mailbox).
func AppendMessage(client *imapclient.Client, folder string, raw []byte, flags []imap.Flag) error {
	cmd := client.Append(folder, int64(len(raw)), &imap.AppendOptions{Flags: flags})
	if _, err := cmd.Write(raw); err != nil {
		return fmt.Errorf("imapsync: append write: %w", err)
	}
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("imapsync: append close: %w", err)
	}
	if _, err := cmd.Wait(); err != nil {
		return fmt.Errorf("imapsync: append %q: %w", folder, err)
	}
	return nil
}

func storeFlag(client *imapclient.Client, uidSet imap.UIDSet, flag imap.Flag, set bool) error {
	op := imap.StoreFlagsAdd
	if !set {
		op = imap.StoreFlagsDel
	}
	storeCmd := client.Store(uidSet, &imap.StoreFlags{
		Op:     op,
		Silent: true,
		Flags:  []imap.Flag{flag},
	}, nil)
	if err := storeCmd.Close(); err != nil {
		return fmt.Errorf("imapsync: store flags: %w", err)
	}
	return nil
}
