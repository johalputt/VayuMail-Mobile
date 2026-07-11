package syncmanager

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// stopAccountWait bounds how long removal waits for an account's sync
// goroutines to exit before proceeding with the delete.
const stopAccountWait = 5 * time.Second

// execAddAccount stores the credential in the platform keystore, wipes
// the in-memory copy, persists the account row, and starts sync.
func (m *Manager) execAddAccount(ctx context.Context, c AddAccountCmd) error {
	if err := c.Config.Validate(); err != nil {
		return err
	}
	if err := m.ks.Store(c.Config.KeystoreAlias, c.Credential); err != nil {
		return fmt.Errorf("syncmanager: store credential: %w", err)
	}
	for i := range c.Credential {
		c.Credential[i] = 0
	}
	row := store.Account{
		DisplayName:   c.Config.DisplayName,
		EmailAddress:  c.Config.EmailAddress,
		IMAPHost:      c.Config.IMAPHost,
		IMAPPort:      c.Config.IMAPPort,
		IMAPTLS:       string(c.Config.IMAPTLS),
		SMTPHost:      c.Config.SMTPHost,
		SMTPPort:      c.Config.SMTPPort,
		SMTPTLS:       string(c.Config.SMTPTLS),
		Username:      c.Config.Username,
		KeystoreAlias: c.Config.KeystoreAlias,
		PinnedSPKI:    c.Config.PinnedSPKI,
		AuthMech:      c.Config.AuthMech,
	}
	id, err := m.db.InsertAccount(ctx, &row)
	if err != nil {
		return err
	}
	m.startAccount(row)
	// Opportunistically pull the account's own private key so received
	// encrypted mail decrypts without a manual step. Best-effort: servers
	// that don't serve it (non-VayuPress) simply emit an error the UI
	// ignores. Queued, not inline, so AddAccount stays fast.
	go func() { _ = m.Send(SyncPrivateKeyCmd{AccountID: id}) }()
	return nil
}

// execUpdateCredential replaces an account's stored password in place:
// its sync goroutines stop, the keystore entry is overwritten under the
// same alias, and sync restarts with the fresh credential — the standard
// recovery from a password change on the server. The outcome is always
// reported as a CredentialUpdatedEvent.
func (m *Manager) execUpdateCredential(ctx context.Context, c UpdateCredentialCmd) error {
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		err = fmt.Errorf("syncmanager: update credential %d: %w", c.AccountID, err)
		m.emit(CredentialUpdatedEvent{AccountID: c.AccountID, Err: err})
		return err
	}
	m.stopAccount(c.AccountID, stopAccountWait)
	err = m.ks.Store(acct.KeystoreAlias, c.Credential)
	for i := range c.Credential {
		c.Credential[i] = 0
	}
	if err != nil {
		err = fmt.Errorf("syncmanager: update credential %d: %w", c.AccountID, err)
		m.emit(CredentialUpdatedEvent{AccountID: c.AccountID, Err: err})
		return err
	}
	m.startAccount(acct)
	m.emit(CredentialUpdatedEvent{AccountID: c.AccountID})
	return nil
}

// execSyncPrivateKey fetches the account's own PGP private key from its
// VayuPress server using the stored mailbox credential, so received
// encrypted mail can be decrypted on-device. The key is delivered to the
// UI as a PrivateKeyEvent; the credential is used in memory only and the
// key is never logged.
func (m *Manager) execSyncPrivateKey(ctx context.Context, c SyncPrivateKeyCmd) error {
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Err: err})
		return err
	}
	secret, err := m.credFor(acct.KeystoreAlias)()
	if err != nil {
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Err: err})
		return err
	}
	fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	armored, err := account.FetchPrivateKey(fctx, http.DefaultClient, acct.EmailAddress, secret)
	if err != nil {
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Err: err})
		return err
	}
	m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Armored: armored})
	return nil
}

// execRemoveAccount signs an account out. Order matters: the sync
// goroutines stop first so no in-flight sync resurrects rows mid-delete,
// then the credential leaves the keystore, then the account row goes and
// its folders, messages, and outbox entries cascade away. The outcome —
// success or failure — is always reported as an AccountRemovedEvent.
func (m *Manager) execRemoveAccount(ctx context.Context, c RemoveAccountCmd) error {
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		err = fmt.Errorf("syncmanager: remove account %d: %w", c.AccountID, err)
		m.emit(AccountRemovedEvent{AccountID: c.AccountID, Err: err})
		return err
	}
	m.stopAccount(c.AccountID, stopAccountWait)
	// A keystore miss must not strand the removal: the goal state — no
	// stored credential — already holds. Other failures are logged and
	// removal continues, or the row naming the alias would linger forever.
	if err := m.ks.Delete(acct.KeystoreAlias); err != nil {
		slog.Warn("remove account: keystore delete failed",
			"account", c.AccountID, "err", err)
	}
	if err := m.db.DeleteAccount(ctx, c.AccountID); err != nil {
		err = fmt.Errorf("syncmanager: remove account %d: %w", c.AccountID, err)
		m.emit(AccountRemovedEvent{AccountID: c.AccountID, Err: err})
		return err
	}
	m.emit(AccountRemovedEvent{AccountID: c.AccountID})
	return nil
}
