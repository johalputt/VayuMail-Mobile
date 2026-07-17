package state

// appstate_nav.go — user-driven navigation and action entry points:
// switching account/folder, opening a thread, live search, and manual
// sync. Split from appstate.go (Rule 10). Each mutates selection state
// under the lock and schedules an async reload; none touch the frame
// loop directly.

import (
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

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

// searchDebounce delays the FTS query while the user is still typing; each
// keystroke used to run a full snapshot rebuild plus an FTS query.
const searchDebounce = 220 * time.Millisecond

// SetSearch updates the live search query. The query itself is recorded
// immediately (the stale-answer guard in reloadSearch compares against
// it), but the FTS run is debounced. Clearing the query applies at once
// so leaving the search screen never shows ghost results.
func (s *AppState) SetSearch(query string) {
	s.mu.Lock()
	changed := s.searchQuery != query
	s.searchQuery = query
	if changed && s.searchTimer != nil {
		s.searchTimer.Stop()
		s.searchTimer = nil
	}
	if changed && query != "" {
		s.searchTimer = time.AfterFunc(searchDebounce, func() {
			select {
			case s.searchQueue <- struct{}{}:
			default:
			}
		})
	}
	s.mu.Unlock()
	if changed && query == "" {
		s.mu.Lock()
		s.snap.SearchResults = nil
		s.mu.Unlock()
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
