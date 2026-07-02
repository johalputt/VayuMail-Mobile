// Package imapsync implements the IMAP side of the engine: dialing and
// authenticating, folder discovery, incremental fetching, and the IDLE
// loop that delivers real-time updates.
//
// The package emits state changes through the Events callback set instead
// of importing the syncmanager package, keeping the import graph acyclic
// and this package importable by any client (ADR-0001).
package imapsync

import (
	"context"
	"errors"
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

// ErrAuth wraps authentication failures. The IDLE loop never retries
// these; the UI must ask the user to re-provision.
var ErrAuth = errors.New("imapsync: authentication failed")

// Events is the callback set through which this package reports state
// changes. Every field is optional; nil callbacks are skipped. Callbacks
// must not block: the syncmanager dispatcher forwards them onto its
// buffered event channel.
type Events struct {
	// NewMessage fires after a new message has been written to the store.
	NewMessage func(folderID int64, uid uint32)
	// FlagChange fires after a server-side flag change has been applied.
	FlagChange func(uid uint32, flags []string)
	// SyncProgress reports incremental sync completion.
	SyncProgress func(done, total int)
	// Connection reports transitions between online and offline.
	Connection func(online bool)
	// AuthError reports a credential rejection.
	AuthError func(err error)
	// Folders fires after folder discovery with the fresh folder list.
	Folders func()
}

// Notification is one unilateral server response observed while the
// connection is idling or between commands.
type Notification struct {
	// NumMessages is the new EXISTS count (0 when not an EXISTS update).
	NumMessages uint32
	// ExpungedSeq is the expunged sequence number (0 when not an expunge).
	ExpungedSeq uint32
	// FlagsChanged indicates a unilateral FETCH (flag update).
	FlagsChanged bool
}

// Dial connects and authenticates to the account's IMAP server. It
// returns the client plus a buffered channel of unilateral notifications;
// when the buffer is full further notifications are dropped, which is
// safe because every notification triggers the same delta sync.
func Dial(ctx context.Context, cfg account.Config, password string) (*imapclient.Client, <-chan Notification, error) {
	notify := make(chan Notification, 64)
	options := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages == nil {
					return
				}
				pushNotification(notify, Notification{NumMessages: *data.NumMessages})
			},
			Expunge: func(seqNum uint32) {
				pushNotification(notify, Notification{ExpungedSeq: seqNum})
			},
			Fetch: func(msg *imapclient.FetchMessageData) {
				// Drain the message data so the client can proceed.
				for {
					if item := msg.Next(); item == nil {
						break
					}
				}
				pushNotification(notify, Notification{FlagsChanged: true})
			},
		},
	}

	options.TLSConfig = cfg.TLSConfig()

	var (
		client *imapclient.Client
		err    error
	)
	switch cfg.IMAPTLS {
	case account.TLSModeImplicit:
		client, err = imapclient.DialTLS(cfg.IMAPAddr(), options)
	case account.TLSModeSTARTTLS:
		client, err = imapclient.DialStartTLS(cfg.IMAPAddr(), options)
	default:
		return nil, nil, fmt.Errorf("imapsync: unsupported TLS mode %q", cfg.IMAPTLS)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("imapsync: dial %s: %w", cfg.IMAPAddr(), err)
	}

	if err := client.Login(cfg.Username, password).Wait(); err != nil {
		logoutErr := client.Close()
		if isNoResponse(err) {
			return nil, nil, fmt.Errorf("%w: %v", ErrAuth, err)
		}
		if logoutErr != nil {
			return nil, nil, fmt.Errorf("imapsync: login: %w (close: %v)", err, logoutErr)
		}
		return nil, nil, fmt.Errorf("imapsync: login: %w", err)
	}

	if !client.Caps().Has(imap.CapIdle) {
		// IDLE is required for the real-time model; without it the
		// scheduler falls back to periodic NOOP polling.
		return client, notify, nil
	}
	return client, notify, nil
}

// pushNotification delivers without ever blocking the client read loop.
func pushNotification(ch chan Notification, n Notification) {
	select {
	case ch <- n:
	default:
		// Buffer full: safe to drop, the pending notification already
		// guarantees a delta sync will run.
	}
}

// isNoResponse reports whether err is an IMAP NO/BAD status response —
// the shape of a credential rejection on LOGIN.
func isNoResponse(err error) bool {
	var respErr *imap.Error
	if errors.As(err, &respErr) {
		return respErr.Type == imap.StatusResponseTypeNo ||
			respErr.Type == imap.StatusResponseTypeBad
	}
	return false
}

// SupportsIdle reports whether the connected server advertises IDLE.
func SupportsIdle(client *imapclient.Client) bool {
	return client.Caps().Has(imap.CapIdle)
}
