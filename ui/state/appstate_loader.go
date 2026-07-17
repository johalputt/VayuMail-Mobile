package state

// appstate_loader.go — the loader goroutine and snapshot rebuild. Split from
// appstate.go so each stays within the constitution's per-file size cap
// (Rule 10). This is the only code path that reads the store for the UI:
// Refresh coalesces into refreshQueue, loaderLoop serializes and paces the
// rebuilds, and commit swaps the immutable snapshot the frame loop reads.

import (
	"context"
	"log/slog"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
)

// minReloadGap paces snapshot rebuilds during event bursts: the size-1
// queue already coalesces requests, but a sync delivering mail faster
// than reload() runs would still rebuild back-to-back. A first reload
// after quiet time runs immediately; only consecutive ones wait.
const minReloadGap = 150 * time.Millisecond

// loaderLoop is the only goroutine that reads the store for the UI.
func (s *AppState) loaderLoop(ctx context.Context) {
	var lastReload time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.refreshQueue:
			if wait := minReloadGap - time.Since(lastReload); wait > 0 {
				t := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					t.Stop()
					return
				case <-t.C:
				}
			}
			s.reload(ctx)
			lastReload = time.Now()
			if s.invalidate != nil {
				s.invalidate()
			}
		case <-s.searchQueue:
			// Search is its own lightweight path: one FTS query patched
			// into the snapshot, not a full rebuild per keystroke.
			s.reloadSearch(ctx)
			if s.invalidate != nil {
				s.invalidate()
			}
		}
	}
}

// reloadSearch runs only the FTS query for the current search text and
// patches the results into the snapshot in place.
func (s *AppState) reloadSearch(ctx context.Context) {
	s.mu.Lock()
	selAccount, query := s.selAccount, s.searchQuery
	if selAccount == 0 && len(s.snap.Accounts) > 0 {
		selAccount = s.snap.Accounts[0].ID
	}
	s.mu.Unlock()

	var msgs []store.Message
	lines := map[int64]string{}
	if query != "" {
		results, err := s.db.Search(ctx, selAccount, query, 50)
		if err != nil {
			slog.Error("reload search", "err", err)
			return
		}
		for _, r := range results {
			msgs = append(msgs, r.Message)
			lines[r.Message.ID] = widgets.RowLine(r.Message)
		}
	}
	s.mu.Lock()
	// Stale answer guard: the query may have changed while the FTS ran.
	if s.searchQuery == query {
		s.snap.SearchResults = msgs
		if s.snap.RowText == nil {
			s.snap.RowText = map[int64]string{}
		}
		for id, line := range lines {
			s.snap.RowText[id] = line
		}
	}
	s.mu.Unlock()
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

	// Keep VayuTalk connected to the active account for the whole time the app
	// is running — not only while the chat screen is open — so messages arrive
	// in real time and any queued while we were away drain the moment we're back.
	// EnsureStarted is idempotent (a no-op once bound to this account), so
	// calling it on every reload is cheap; switching accounts rebinds it.
	if s.Chat != nil {
		for i := range accounts {
			if accounts[i].ID == selAccount {
				s.Chat.EnsureStarted(accounts[i])
				break
			}
		}
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
	next.RowText = make(map[int64]string, len(next.Messages))
	for _, m := range next.Messages {
		next.RowText[m.ID] = widgets.RowLine(m)
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
			next.Thread = s.decryptThread(thread)
			// Parse each body once here, off the frame loop; the view reads
			// the result instead of tokenizing HTML per message per frame.
			next.ThreadBodies = make(map[int64]widgets.MessageBody, len(next.Thread))
			for _, m := range next.Thread {
				next.ThreadBodies[m.ID] = widgets.ParseMessageBody(m)
			}
		}
	}
	if query != "" {
		results, err := s.db.Search(ctx, selAccount, query, 50)
		if err == nil {
			for _, r := range results {
				next.SearchResults = append(next.SearchResults, r.Message)
				next.RowText[r.Message.ID] = widgets.RowLine(r.Message)
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
	next.ManualSyncing = s.snap.ManualSyncing
	next.Locked = s.locked
	s.snap = next
	s.mu.Unlock()
}
