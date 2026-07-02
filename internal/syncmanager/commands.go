package syncmanager

import (
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

// Cmd is the closed interface for everything the UI asks the sync layer
// to do. Commands are sent with a non-blocking select; a full cmdCh
// returns an error to the UI immediately (Rule 5).
type Cmd interface{ isCmd() }

// MoveCmd moves a message to another folder (by full IMAP name).
type MoveCmd struct {
	MessageID  int64
	DestFolder string
}

// DeleteCmd deletes a message (move to Trash, or expunge from Trash).
type DeleteCmd struct {
	MessageID int64
}

// MarkCmd sets or clears one IMAP flag on a message. Flag is the raw
// IMAP flag name, e.g. "\\Seen" or "\\Flagged".
type MarkCmd struct {
	MessageID int64
	Flag      string
	Set       bool
}

// SendCmd asks the scheduler to attempt an outbox entry now.
type SendCmd struct {
	OutboxID int64
}

// SyncNowCmd forces an immediate full sync for one account.
type SyncNowCmd struct {
	AccountID int64
}

// AddAccountCmd provisions a new account: the manager stores the
// credential in the platform keystore, persists the account row, and
// starts its sync goroutines. Credential is wiped after storage.
type AddAccountCmd struct {
	Config     account.Config
	Credential []byte
}

func (MoveCmd) isCmd()       {}
func (DeleteCmd) isCmd()     {}
func (MarkCmd) isCmd()       {}
func (SendCmd) isCmd()       {}
func (SyncNowCmd) isCmd()    {}
func (AddAccountCmd) isCmd() {}
