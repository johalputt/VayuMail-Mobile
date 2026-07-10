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

// SyncFolderCmd syncs a single folder now — used when the user opens a
// folder (e.g. Sent) so its server contents load without a full sync.
type SyncFolderCmd struct {
	AccountID int64
	FolderID  int64
}

// AddAccountCmd provisions a new account: the manager stores the
// credential in the platform keystore, persists the account row, and
// starts its sync goroutines. Credential is wiped after storage.
type AddAccountCmd struct {
	Config     account.Config
	Credential []byte
}

// RemoveAccountCmd signs an account out: its sync goroutines stop, its
// credential is deleted from the keystore, and its local rows (folders,
// messages, outbox) are removed. Completion arrives as AccountRemovedEvent.
type RemoveAccountCmd struct {
	AccountID int64
}

// FetchAttachmentCmd downloads one attachment (0-based part order) and
// saves it into the attachments directory; completion arrives as an
// AttachmentSavedEvent (ADR-0007).
type FetchAttachmentCmd struct {
	MessageID int64
	Index     int
}

// SaveDraftCmd appends a serialized draft to the account's Drafts folder
// with the \Draft flag.
type SaveDraftCmd struct {
	AccountID int64
	Raw       []byte
}

// SnoozeCmd hides a message from lists until the given Unix time; zero
// unsnoozes. Local-only — nothing changes on the server.
type SnoozeCmd struct {
	MessageID int64
	UntilUnix int64
}

// UnsubscribeCmd acts on a message's List-Unsubscribe mailto target by
// queueing and sending the unsubscribe mail (RFC 8058 one-click for
// mailto). HTTPS-only targets are the UI's job (copy/open).
type UnsubscribeCmd struct {
	MessageID int64
}

func (MoveCmd) isCmd()            {}
func (DeleteCmd) isCmd()          {}
func (MarkCmd) isCmd()            {}
func (SendCmd) isCmd()            {}
func (SyncNowCmd) isCmd()         {}
func (SyncFolderCmd) isCmd()      {}
func (AddAccountCmd) isCmd()      {}
func (RemoveAccountCmd) isCmd()   {}
func (FetchAttachmentCmd) isCmd() {}
func (SaveDraftCmd) isCmd()       {}
func (SnoozeCmd) isCmd()          {}
func (UnsubscribeCmd) isCmd()     {}
