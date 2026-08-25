package syncmanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// stopAccountWait bounds how long removal waits for an account's sync
// goroutines to exit before proceeding with the delete.
const stopAccountWait = 5 * time.Second

// execAddAccount stores the credential in the platform keystore, wipes
// the in-memory copy, persists the account row, and starts sync.
// Every exit path emits an AccountAddedEvent. Returning an error alone was not
// enough: the command loop logs a failure and drops it, so the only report the
// user ever got was the setup screen's own optimistic snackbar.
func (m *Manager) execAddAccount(ctx context.Context, c AddAccountCmd) (err error) {
	email := c.Config.EmailAddress
	var id int64
	defer func() { m.emit(AccountAddedEvent{AccountID: id, Email: email, Err: err}) }()

	if err = c.Config.Validate(); err != nil {
		return err
	}
	if err = m.ks.Store(c.Config.KeystoreAlias, c.Credential); err != nil {
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
	id, err = m.db.InsertAccount(ctx, &row)
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

// pgpKeyAliasPrefix namespaces the sealed on-device PGP private keys
// (plan Phase 4.1) apart from mailbox credentials in the keystore.
const pgpKeyAliasPrefix = "pgppriv:"

// PublishKeyFunc, when set, is called once after an on-device generated
// public key is created so the server can publish it to WKD. The VayuPress
// endpoint decision is still pending, so the default nil means generation
// works but publication is skipped and logged — the account stays fully
// functional, just not yet encryptable-to by others.
var PublishKeyFunc func(ctx context.Context, email, armoredPublicKey string) error

// execSyncPrivateKey makes the account's own PGP private key available
// on-device, delivered to the UI as a PrivateKeyEvent. Three tiers:
//
//  1. A previously stored/generated key in the sealed keystore — emitted
//     directly, no network.
//  2. The legacy path: the VayuPress server holds the key; fetch it with
//     the mailbox credential and seal it for next time.
//  3. The server has no key at all (new account): generate one on-device
//     so the secret never exists anywhere else, then offer it for
//     publication.
func (m *Manager) execSyncPrivateKey(ctx context.Context, c SyncPrivateKeyCmd) error {
	acct, err := m.db.GetAccount(ctx, c.AccountID)
	if err != nil {
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Err: err})
		return err
	}
	alias := pgpKeyAliasPrefix + acct.EmailAddress

	if sealed, err := m.ks.Fetch(alias); err == nil && len(sealed) > 0 {
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Armored: string(sealed)})
		return nil
	}

	secret, err := m.credFor(acct.KeystoreAlias)()
	if err != nil {
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Err: err})
		return err
	}
	// Dedicated, non-pooling client: this fires opportunistically after
	// every AddAccount, so a pooled keep-alive connection would leave its
	// reader goroutine running past the fetch (a leak the tests catch).
	// DisableKeepAlives + CloseIdleConnections guarantees the transport
	// leaves nothing behind once the request returns.
	tr := &http.Transport{DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	armored, ferr := account.FetchPrivateKey(fctx, &http.Client{Transport: tr}, acct.EmailAddress, secret)
	cancel()

	switch {
	case ferr == nil:
		if err := m.ks.Store(alias, []byte(armored)); err != nil {
			slog.Warn("seal fetched private key", "account", c.AccountID, "err", err)
		}
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Armored: armored})
		return nil
	case errors.Is(ferr, account.ErrNoPrivateKey):
		// Server has no key for this account: generate one on-device so
		// the secret never exists anywhere else.
	default:
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Err: ferr})
		return ferr
	}

	pubArmored, privArmored, err := pgp.GenerateKey(acct.DisplayName, acct.EmailAddress)
	if err != nil {
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Err: err})
		return err
	}
	if err := m.ks.Store(alias, privArmored); err != nil {
		gerr := fmt.Errorf("syncmanager: seal generated key: %w", err)
		m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Err: gerr})
		return gerr
	}
	if PublishKeyFunc != nil {
		go func(email, pub string) {
			pctx, pcancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer pcancel()
			if perr := PublishKeyFunc(pctx, email, pub); perr != nil {
				slog.Info("publish generated public key", "email", email, "err", perr)
			}
		}(acct.EmailAddress, string(pubArmored))
	}
	m.emit(PrivateKeyEvent{AccountID: c.AccountID, Email: acct.EmailAddress, Armored: string(privArmored)})
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
	// The pooled command connection must not outlive the credential it was
	// authenticated with.
	m.dropCommandConn(acct.KeystoreAlias)
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
