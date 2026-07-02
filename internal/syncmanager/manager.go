package syncmanager

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// Manager owns all sync goroutines. Construct with New, wire the UI to
// Events() and Send(), call Start once, and Shutdown on exit. Shutdown
// cancels every goroutine and waits for them to drain — goleak-clean.
type Manager struct {
	db      *store.DB
	ks      crypto.Keystore
	eventCh chan Event
	cmdCh   chan Cmd

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	runners map[int64]context.CancelFunc
}

// New creates a Manager over the given store and keystore. Channel sizes
// are fixed by the architecture: events 256, commands 64 (ARCHITECTURE.md).
func New(db *store.DB, ks crypto.Keystore) *Manager {
	return &Manager{
		db:      db,
		ks:      ks,
		eventCh: make(chan Event, 256),
		cmdCh:   make(chan Cmd, 64),
		runners: make(map[int64]context.CancelFunc),
	}
}

// Events returns the channel the UI drains non-blockingly each frame.
func (m *Manager) Events() <-chan Event { return m.eventCh }

// Send submits a command without ever blocking the caller. When the
// command buffer is full it returns an error immediately (Rule 5); the
// UI surfaces it as a transient snackbar.
func (m *Manager) Send(cmd Cmd) error {
	select {
	case m.cmdCh <- cmd:
		return nil
	default:
		return fmt.Errorf("syncmanager: command queue full, try again")
	}
}

// Start loads all accounts and spawns their sync goroutines plus the
// command dispatcher. It returns immediately; work continues until
// Shutdown or ctx cancellation.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	accounts, err := m.db.ListAccounts(m.ctx)
	if err != nil {
		return fmt.Errorf("syncmanager: load accounts: %w", err)
	}
	for _, acct := range accounts {
		m.startAccount(acct)
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.commandLoop()
	}()
	return nil
}

// Shutdown cancels every goroutine and blocks until all have exited.
func (m *Manager) Shutdown() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

// emit delivers an event without ever blocking a sync goroutine. When the
// buffer is full the event is dropped with a log line — the UI refreshes
// from the store on every event, so a dropped event delays nothing
// permanently.
func (m *Manager) emit(ev Event) {
	select {
	case m.eventCh <- ev:
	default:
		slog.Warn("event channel full, dropping event",
			"event", fmt.Sprintf("%T", ev))
	}
}

// startAccount spawns the IDLE loop and scheduler for one account.
func (m *Manager) startAccount(acct store.Account) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, running := m.runners[acct.ID]; running {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.runners[acct.ID] = cancel

	cfg := ConfigFromStore(acct)
	cred := m.credFor(acct.KeystoreAlias)

	m.wg.Add(2)
	go func() {
		defer m.wg.Done()
		err := imapsync.RunIDLE(ctx, cfg, cred, "INBOX", m.db,
			m.eventsFor(acct.ID), acct.ID)
		if err != nil && ctx.Err() == nil {
			slog.Error("idle loop exited", "account", acct.ID, "err", err)
		}
	}()
	go func() {
		defer m.wg.Done()
		m.runScheduler(ctx, acct.ID, cfg, cred)
	}()
}

// eventsFor adapts the imapsync callback set onto the typed event bus —
// the dispatcher role in the architecture diagram.
func (m *Manager) eventsFor(accountID int64) imapsync.Events {
	return imapsync.Events{
		NewMessage: func(folderID int64, uid uint32) {
			m.emit(NewMessageEvent{AccountID: accountID, FolderID: folderID, UID: uid})
		},
		FlagChange: func(uid uint32, flags []string) {
			m.emit(FlagChangeEvent{AccountID: accountID, UID: uid, Flags: flags})
		},
		SyncProgress: func(done, total int) {
			m.emit(SyncProgressEvent{AccountID: accountID, Done: done, Total: total})
		},
		Connection: func(online bool) {
			m.emit(ConnectionEvent{AccountID: accountID, Online: online})
		},
		AuthError: func(err error) {
			m.emit(AuthErrorEvent{AccountID: accountID, Err: err})
		},
		Folders: func() {
			folders, err := m.db.ListFolders(m.ctx, accountID)
			if err != nil {
				slog.Error("list folders after sync", "err", err)
				return
			}
			m.emit(FolderListEvent{AccountID: accountID, Folders: folders})
		},
	}
}

// credFor returns the credential fetcher for a keystore alias. The
// password is fetched at connection time and never cached (Rule 6).
func (m *Manager) credFor(alias string) func() (string, error) {
	return func() (string, error) {
		secret, err := m.ks.Fetch(alias)
		if err != nil {
			return "", fmt.Errorf("syncmanager: keystore fetch: %w", err)
		}
		return string(secret), nil
	}
}

// ConfigFromStore converts a persisted account row into the engine
// config.
func ConfigFromStore(a store.Account) account.Config {
	return account.Config{
		DisplayName:   a.DisplayName,
		EmailAddress:  a.EmailAddress,
		IMAPHost:      a.IMAPHost,
		IMAPPort:      a.IMAPPort,
		IMAPTLS:       account.TLSMode(a.IMAPTLS),
		SMTPHost:      a.SMTPHost,
		SMTPPort:      a.SMTPPort,
		SMTPTLS:       account.TLSMode(a.SMTPTLS),
		Username:      a.Username,
		KeystoreAlias: a.KeystoreAlias,
	}
}
