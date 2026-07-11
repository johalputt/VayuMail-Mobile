// Package syncmanager owns every long-running goroutine in VayuMail: the
// per-account IMAP IDLE loops, the dispatcher that persists server state,
// and the scheduler that drives outbox retries. It communicates with the
// UI exclusively through two typed channels:
//
//	eventCh chan Event  buffered 256  flows FROM syncmanager TO ui
//	cmdCh   chan Cmd    buffered 64   flows FROM ui TO syncmanager
//
// Nothing in this package may import Gio (Constitutional Rule 4), and
// nothing in the UI may block on this package (Rule 5).
package syncmanager

import (
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// Event is the closed interface for everything the sync layer reports to
// the UI. The UI drains events non-blockingly before every frame.
type Event interface{ isEvent() }

// NewMessageEvent reports that a new message was written to the store.
type NewMessageEvent struct {
	AccountID int64
	FolderID  int64
	UID       uint32
}

// FlagChangeEvent reports a server-side flag change already applied to
// the store.
type FlagChangeEvent struct {
	AccountID int64
	UID       uint32
	Flags     []string
}

// SyncProgressEvent reports incremental sync progress for one account.
type SyncProgressEvent struct {
	AccountID int64
	Done      int
	Total     int
}

// SendResultEvent reports the outcome of one outbox send attempt.
// Err is nil on success.
type SendResultEvent struct {
	OutboxID int64
	Err      error
}

// AuthErrorEvent reports a credential rejection. The sync loop for the
// account has stopped; the user must re-provision.
type AuthErrorEvent struct {
	AccountID int64
	Err       error
}

// ConnectionEvent reports the account going online or offline.
type ConnectionEvent struct {
	AccountID int64
	Online    bool
}

// FolderListEvent carries a fresh folder list after discovery.
type FolderListEvent struct {
	AccountID int64
	Folders   []store.Folder
}

// CredentialUpdatedEvent reports the outcome of an
// UpdateCredentialCmd. On success the account is reconnecting with its
// new password.
type CredentialUpdatedEvent struct {
	AccountID int64
	Err       error
}

// AccountRemovedEvent reports the outcome of a RemoveAccountCmd. On
// success the account and all its local data are gone.
type AccountRemovedEvent struct {
	AccountID int64
	Err       error
}

// AttachmentSavedEvent reports the outcome of a FetchAttachmentCmd:
// Path is the saved file on success, Err the failure otherwise
// (ADR-0007).
type AttachmentSavedEvent struct {
	MessageID int64
	Index     int
	Path      string
	Err       error
}

func (NewMessageEvent) isEvent()        {}
func (FlagChangeEvent) isEvent()        {}
func (SyncProgressEvent) isEvent()      {}
func (SendResultEvent) isEvent()        {}
func (AuthErrorEvent) isEvent()         {}
func (ConnectionEvent) isEvent()        {}
func (FolderListEvent) isEvent()        {}
func (CredentialUpdatedEvent) isEvent() {}
func (AccountRemovedEvent) isEvent()    {}
func (AttachmentSavedEvent) isEvent()   {}
