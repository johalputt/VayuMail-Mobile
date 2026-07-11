// Package state holds the UI-side application state: an immutable
// snapshot the render loop reads, refreshed asynchronously from the
// store. Layout code never touches SQLite or the network (Rule 5) — it
// reads the latest snapshot, and every mutation goes through the
// syncmanager command channel or an async loader here.
package state

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/applock"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
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
	SearchResults []store.Message
	Online        bool
	SyncDone      int
	SyncTotal     int
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

	keyring *pgp.Keyring

	mu           sync.Mutex
	snap         Snapshot
	selAccount   int64
	selFolder    int64
	selThread    string
	searchQuery  string
	refreshQueue chan struct{}
	autoWKD      bool
	lastAutoWKD  time.Time
	locked       bool
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

// Apply folds one sync event into the state. Called from the frame loop;
// it must stay non-blocking (map updates and refresh scheduling only).
func (s *AppState) Apply(ev syncmanager.Event) {
	s.mu.Lock()
	switch e := ev.(type) {
	case syncmanager.ConnectionEvent:
		s.snap.Online = e.Online
	case syncmanager.SyncProgressEvent:
		s.snap.SyncDone, s.snap.SyncTotal = e.Done, e.Total
	case syncmanager.AuthErrorEvent:
		s.snap.AuthError = true
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

// loaderLoop is the only goroutine that reads the store for the UI.
func (s *AppState) loaderLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.refreshQueue:
			s.reload(ctx)
			if s.invalidate != nil {
				s.invalidate()
			}
		}
	}
}

// reload rebuilds the snapshot from the store.
func (s *AppState) reload(ctx context.Context) {
	s.mu.Lock()
	selAccount, selFolder := s.selAccount, s.selFolder
	selThread, query := s.selThread, s.searchQuery
	s.mu.Unlock()

	next := Snapshot{Unread: map[int64]int{}}

	accounts, err := s.db.ListAccounts(ctx)
	if err != nil {
		slog.Error("reload accounts", "err", err)
		return
	}
	next.Accounts = accounts
	s.loadPrefs(ctx, &next)
	if len(accounts) == 0 {
		s.commit(next)
		return
	}
	if selAccount == 0 {
		selAccount = accounts[0].ID
	}

	folders, err := s.db.ListFolders(ctx, selAccount)
	if err != nil {
		slog.Error("reload folders", "err", err)
		return
	}
	next.Folders = folders
	for _, f := range folders {
		n, err := s.db.UnreadCount(ctx, f.ID)
		if err == nil {
			next.Unread[f.ID] = n
		}
		if (selFolder == 0 && f.IsInbox) || f.ID == selFolder {
			next.CurrentFolder = f
		}
	}
	if unified, err := s.db.UnifiedUnreadCount(ctx); err == nil {
		next.Unread[UnifiedFolderID] = unified
	}
	if selFolder == UnifiedFolderID {
		next.CurrentFolder = store.Folder{ID: UnifiedFolderID, Name: "All inboxes"}
		msgs, err := s.db.ListUnifiedInbox(ctx, 0, 200)
		if err != nil {
			slog.Error("reload unified inbox", "err", err)
			return
		}
		next.Messages = msgs
	} else if next.CurrentFolder.ID != 0 {
		msgs, err := s.db.ListMessages(ctx, next.CurrentFolder.ID, 0, 200)
		if err != nil {
			slog.Error("reload messages", "err", err)
			return
		}
		next.Messages = msgs
	}
	if keys, err := s.db.ListPGPKeys(ctx); err == nil {
		next.PGPKeys = keys
	}
	if urlStr, err := s.db.GetSetting(ctx, store.SettingPGPKeyDirectoryURL); err == nil {
		next.PGPKeyDirURL = urlStr
	}
	next.SelectedAccount = selAccount
	s.mu.Lock()
	next.AutoWKD = s.autoWKD
	s.mu.Unlock()
	if selThread != "" {
		thread, err := s.db.ListThread(ctx, selAccount, selThread)
		if err == nil {
			next.Thread = thread
		}
	}
	if query != "" {
		results, err := s.db.Search(ctx, selAccount, query, 50)
		if err == nil {
			for _, r := range results {
				next.SearchResults = append(next.SearchResults, r.Message)
			}
		}
	}
	s.commit(next)
}

// commit swaps in the new snapshot, preserving transient flags.
func (s *AppState) commit(next Snapshot) {
	s.mu.Lock()
	next.Online = s.snap.Online
	next.AuthError = s.snap.AuthError
	next.SyncDone, next.SyncTotal = s.snap.SyncDone, s.snap.SyncTotal
	next.Locked = s.locked
	s.snap = next
	s.mu.Unlock()
}

// SelectAccount switches the active account and reloads.
func (s *AppState) SelectAccount(id int64) {
	s.mu.Lock()
	s.selAccount = id
	s.selFolder = 0
	s.mu.Unlock()
	s.Refresh()
}

// SelectFolder switches the active folder, reloads from cache, and asks
// the sync layer to refresh that folder from the server so folders that
// the idle loop doesn't watch (Sent, Archive, …) show their real contents.
func (s *AppState) SelectFolder(id int64) {
	s.mu.Lock()
	s.selFolder = id
	acct := s.selAccount
	s.mu.Unlock()
	s.Refresh()
	if id > 0 && acct > 0 {
		s.Send(syncmanager.SyncFolderCmd{AccountID: acct, FolderID: id})
	}
}

// SyncNow asks the sync layer to refresh every folder of the active
// account now (pull-to-refresh, or the Settings "Sync now" button).
func (s *AppState) SyncNow() {
	acct, ok := s.CurrentAccount()
	if !ok {
		return
	}
	s.Send(syncmanager.SyncNowCmd{AccountID: acct.ID})
}

// OpenThread loads a conversation for the thread screen.
func (s *AppState) OpenThread(threadID string) {
	s.mu.Lock()
	s.selThread = threadID
	s.mu.Unlock()
	s.Refresh()
}

// SetSearch updates the live search query.
func (s *AppState) SetSearch(query string) {
	s.mu.Lock()
	changed := s.searchQuery != query
	s.searchQuery = query
	s.mu.Unlock()
	if changed {
		s.Refresh()
	}
}

// CurrentAccount returns the active account, if any.
func (s *AppState) CurrentAccount() (store.Account, bool) {
	snap := s.Snapshot()
	s.mu.Lock()
	sel := s.selAccount
	s.mu.Unlock()
	for _, a := range snap.Accounts {
		if a.ID == sel || sel == 0 {
			return a, true
		}
	}
	return store.Account{}, false
}

func (s *AppState) notify(msg string) {
	if s.Notify != nil {
		s.Notify(msg)
	}
}
