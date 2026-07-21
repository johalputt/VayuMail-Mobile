package ui

import "sync"

// mailnav.go — the app-side of a notification deep-link. The platform layer (an
// Android notification tap) records the mailbox a tapped notification points at
// via SetMailNavTarget; the frame loop consumes it once (see layout) and opens
// that mailbox — its account and folder. Kept tiny and lock-guarded so a platform
// callback can set it from any thread. On desktop nothing ever sets a target, so
// it is inert.

var mailNav struct {
	mu      sync.Mutex
	pending bool
	account int64
	folder  int64
	wake    func() // window.Invalidate, so a target set off-frame draws promptly
}

// SetMailNavTarget records the mailbox a tapped notification should open (account
// id, and optionally a folder id). Safe to call from any goroutine or platform
// callback; the next frame applies it. Exported so the platform notification
// bridge can feed a tap into the app.
func SetMailNavTarget(accountID, folderID int64) {
	mailNav.mu.Lock()
	mailNav.pending = true
	mailNav.account = accountID
	mailNav.folder = folderID
	wake := mailNav.wake
	mailNav.mu.Unlock()
	if wake != nil {
		wake()
	}
}

// setMailNavWake installs the frame-wake callback (window.Invalidate). A tapped
// notification usually foregrounds the app (which produces a frame on its own),
// but waking explicitly makes the jump immediate and covers the already-visible
// case.
func setMailNavWake(f func()) {
	mailNav.mu.Lock()
	mailNav.wake = f
	mailNav.mu.Unlock()
}

// consumeMailNavTarget returns a pending target exactly once, then clears it.
func consumeMailNavTarget() (accountID, folderID int64, ok bool) {
	mailNav.mu.Lock()
	defer mailNav.mu.Unlock()
	if !mailNav.pending {
		return 0, 0, false
	}
	mailNav.pending = false
	return mailNav.account, mailNav.folder, true
}
