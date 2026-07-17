// Package state holds the UI-side application state: an immutable
// snapshot the render loop reads, refreshed asynchronously from the
// store. Layout code never touches SQLite or the network (Rule 5) — it
// reads the latest snapshot, and every mutation goes through the
// syncmanager command channel or an async loader here.
package state

import (
	"context"
	"sync"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/applock"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// Snapshot is one consistent view of everything the screens render. It is
// replaced wholesale by Refresh; render code must treat it as read-only.
type Snapshot struct {
	Accounts      []store.Account
	Folders       []store.Folder
	Unread        map[int64]int
	CurrentFolder store.Folder
	Messages      []store.Message
	Thread        []store.Message
	// ThreadBodies holds each thread message's pre-parsed display body,
	// keyed by message ID. Built once per thread load off the frame loop so
	// the view never tokenizes HTML or splits quotes while rendering.
	ThreadBodies  map[int64]widgets.MessageBody
	SearchResults []store.Message
	// RowText holds each list row's precomputed "subject — snippet" line,
	// keyed by message ID, so the list never concatenates it per frame (the
	// concat result is the text shaper's cache key, so a per-frame concat
	// defeated the shape cache). Covers Messages and SearchResults.
	RowText   map[int64]string
	Online    bool
	SyncDone  int
	SyncTotal int
	// ManualSyncing is true while a user-requested full sync runs
	// (pull-to-refresh, "Sync now") — bracketed by the SyncStarted /
	// SyncFinished events, so the indicator spins and the list reloads
	// even when the sync finds nothing new.
	ManualSyncing bool
	AuthError     bool
	PGPKeys       []store.PGPKey
	PGPKeyDirURL  string
	AutoWKD       bool

	// Locked gates the whole UI behind the PIN screen.
	Locked bool
	// AppLockEnabled reports whether a PIN is set.
	AppLockEnabled bool
	// TOTPEnabled reports whether the authenticator second factor is
	// enrolled on top of the PIN.
	TOTPEnabled bool
	// AppLockTimeout is the auto-lock idle window in seconds (0 = every
	// time the app is left).
	AppLockTimeout int
	// NotificationsOn mirrors the notifications setting (default on).
	NotificationsOn bool
	// NotifyPreviewOn mirrors the notification-preview setting: sender and
	// subject in the tray vs a generic line (default on).
	NotifyPreviewOn bool
	// SelectedAccount is the account the folder list belongs to.
	SelectedAccount int64
}

// AppState mediates between the sync layer and the screens.
type AppState struct {
	db   *store.DB
	mgr  *syncmanager.Manager
	lock *applock.Manager

	// invalidate wakes the window after an async snapshot update.
	invalidate func()
	// Notify shows a transient snackbar; set by the app root.
	Notify func(msg string)
	// NotifyUndo shows a snackbar with an Undo action; onCommit fires
	// when it expires un-undone. Set by the app root.
	NotifyUndo func(msg string, onUndo, onCommit func())

	// Chat is the VayuTalk holder, wired by the app root once the keystore
	// is available. Nil in headless tests (the chat screens then no-op).
	Chat *ChatState

	keyring *pgp.Keyring

	mu           sync.Mutex
	snap         Snapshot
	selAccount   int64
	selFolder    int64
	selThread    string
	searchQuery  string
	refreshQueue chan struct{}
	// searchQueue triggers the lightweight search-only reload; searchTimer
	// debounces per-keystroke queries (guarded by mu).
	searchQueue chan struct{}
	searchTimer *time.Timer
	// refetching marks messages with an in-flight body re-download so a
	// broken row is repaired once, not on every frame the thread is open;
	// refetched records completed attempts so the repair is terminal —
	// a row that re-fetches to the same bytes must never loop.
	refetching  map[int64]bool
	refetched   map[int64]bool
	autoWKD     bool
	lastAutoWKD time.Time
	locked      bool
}

// autoWKDInterval throttles auto key discovery so a burst of new mail
// triggers at most one WKD sweep.
const autoWKDInterval = 10 * time.Minute

// New creates the state and starts its single loader goroutine, which
// serializes all store reads triggered by Refresh. lock may be nil in
// headless tests; the UI then never locks.
func New(ctx context.Context, db *store.DB, mgr *syncmanager.Manager, lock *applock.Manager) *AppState {
	s := &AppState{
		db:           db,
		mgr:          mgr,
		lock:         lock,
		keyring:      pgp.NewKeyring(),
		refreshQueue: make(chan struct{}, 1),
		searchQueue:  make(chan struct{}, 1),
		refetching:   map[int64]bool{},
		refetched:    map[int64]bool{},
		snap: Snapshot{Unread: map[int64]int{}, Online: true,
			NotificationsOn: true, NotifyPreviewOn: true},
	}
	go func() {
		s.loadPGPKeys(ctx)
		// Auto key discovery defaults ON: only an explicit "0" disables it,
		// so fresh installs get zero-touch PGP from their VayuPress server.
		if v, err := db.GetSetting(ctx, store.SettingAutoWKD); err == nil {
			s.mu.Lock()
			s.autoWKD = v != "0"
			s.mu.Unlock()
		}
		// Start locked when a PIN is set: the lock screen is the first
		// thing an enrolled user sees.
		if lock != nil && lock.Enabled(ctx) {
			s.mu.Lock()
			s.locked = true
			s.snap.Locked = true
			s.snap.AppLockEnabled = true
			s.mu.Unlock()
		}
		s.loaderLoop(ctx)
	}()
	return s
}

// SetInvalidate wires the window wake-up used after async updates.
func (s *AppState) SetInvalidate(fn func()) { s.invalidate = fn }

// Snapshot returns the current view state. Cheap: shallow copy.
func (s *AppState) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

// Send forwards a command to the sync layer without blocking; failures
// surface through the snackbar.
func (s *AppState) Send(cmd syncmanager.Cmd) {
	if err := s.mgr.Send(cmd); err != nil {
		s.notify("Busy — try again")
	}
}

// Apply folds one sync event into the state. Called from the event pump
// goroutine (any goroutine is safe — everything is mutex-guarded); it must
// stay non-blocking (map updates and refresh scheduling only).
//
// Transient events that only patch snapshot fields the frame loop already
// reads (connection state, sync progress, auth banner, manual-sync
// bracketing) return without scheduling a store reload: during a large
// sync the progress ticker used to trigger a full snapshot rebuild — a
// dozen SQLite queries — per fetched message. The list itself refreshes on
// the NewMessageEvent/SyncFinishedEvent that actually change it.
func (s *AppState) Apply(ev syncmanager.Event) {
	s.mu.Lock()
	switch e := ev.(type) {
	case syncmanager.ConnectionEvent:
		s.snap.Online = e.Online
		s.mu.Unlock()
		return
	case syncmanager.SyncProgressEvent:
		s.snap.SyncDone, s.snap.SyncTotal = e.Done, e.Total
		s.mu.Unlock()
		return
	case syncmanager.AuthErrorEvent:
		s.snap.AuthError = true
		s.mu.Unlock()
		return
	case syncmanager.SyncStartedEvent:
		s.snap.ManualSyncing = true
		s.mu.Unlock()
		return
	case syncmanager.SyncFinishedEvent:
		s.snap.ManualSyncing = false
	case syncmanager.NewMessageEvent:
		// Auto-WKD: throttled opportunistic key discovery on new mail;
		// the sweep skips known keys, so it stays cheap.
		if s.autoWKD && time.Since(s.lastAutoWKD) > autoWKDInterval {
			s.lastAutoWKD = time.Now()
			go s.DiscoverContactKeysWKD()
		}
	case syncmanager.SendResultEvent:
		s.mu.Unlock()
		if e.Err != nil {
			s.notify("Send failed — will retry")
		} else {
			s.notify("Sent")
		}
		s.Refresh()
		return
	case syncmanager.AttachmentSavedEvent:
		s.mu.Unlock()
		if e.Err != nil {
			s.notify("Attachment download failed")
		} else {
			s.notify("Saved: " + e.Path)
		}
		return
	case syncmanager.PrivateKeyEvent:
		s.mu.Unlock()
		if e.Err == nil && e.Armored != "" {
			s.importPrivateKey(e.Armored, e.Email)
		}
		return
	case syncmanager.MessageRefetchedEvent:
		// The single repair attempt is complete — success or not, it is
		// terminal for this session so a row that re-fetched to the same
		// bytes can never loop. The reload below shows the outcome.
		delete(s.refetching, e.MessageID)
		s.refetched[e.MessageID] = true
	case syncmanager.CredentialUpdatedEvent:
		// A fresh credential clears the stale auth banner; the reconnect
		// under way proves it right or re-raises the error.
		s.snap.AuthError = false
		s.mu.Unlock()
		if e.Err != nil {
			s.notify("Could not update password — try again")
		} else {
			s.notify("Password updated — reconnecting")
		}
		s.Refresh()
		return
	case syncmanager.AccountRemovedEvent:
		s.mu.Unlock()
		if e.Err != nil {
			s.notify("Could not sign out — try again")
		} else {
			s.notify("Signed out")
		}
		s.mu.Lock()
		if s.selAccount == e.AccountID {
			s.selAccount = 0
			s.selFolder = 0
		}
		s.mu.Unlock()
		// VayuTalk is bound to a specific account's credential and domain;
		// signing an account out must drop that live connection. It rebinds
		// lazily when the chat screen is next opened.
		if s.Chat != nil {
			s.Chat.Stop()
		}
		s.Refresh()
		return
	}
	s.mu.Unlock()
	s.Refresh()
}

// Refresh schedules an async snapshot reload; multiple calls coalesce.
func (s *AppState) Refresh() {
	select {
	case s.refreshQueue <- struct{}{}:
	default:
	}
}

func (s *AppState) notify(msg string) {
	if s.Notify != nil {
		s.Notify(msg)
	}
}
