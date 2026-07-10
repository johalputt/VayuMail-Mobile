package test

import (
	"errors"
	"testing"
	"time"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

// waitRemovedEvent drains the manager's event bus until an
// AccountRemovedEvent arrives, skipping the connection/sync noise the
// account's failing IDLE loop emits along the way.
func waitRemovedEvent(t *testing.T, mgr *syncmanager.Manager, wait time.Duration) syncmanager.AccountRemovedEvent {
	t.Helper()
	deadline := time.After(wait)
	for {
		select {
		case ev := <-mgr.Events():
			if removed, ok := ev.(syncmanager.AccountRemovedEvent); ok {
				return removed
			}
		case <-deadline:
			t.Fatal("AccountRemovedEvent never arrived")
		}
	}
}

// TestRemoveAccountEndToEnd adds an account through the command bus,
// fills its local rows from a real IMAP sync against the loopback
// server, then removes it and asserts every trace is gone: account row,
// folders, messages, outbox entry, and keystore credential.
//
// The production dialer is TLS-only, so the account's own IDLE loop
// cannot reach the plaintext loopback server; like imapsync_test.go, the
// sync runs through the same functions below the dial seam.
func TestRemoveAccountEndToEnd(t *testing.T) {
	defer verifyNoLeaks(t)

	addr, closeSrv := startIMAPServer(t)
	defer closeSrv()
	client := dialTestClient(t, addr)
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("client close: %v", err)
		}
	}()
	appendTestMessage(t, client, readMIMEFixture(t, "simple.eml"))
	appendTestMessage(t, client, readMIMEFixture(t, "multipart.eml"))

	db := openStore(t)
	ks := appcrypto.NewMemoryKeystore()
	mgr := syncmanager.New(db, ks)
	ctx := t.Context()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Unroutable IMAP endpoint: the spawned IDLE loop fails fast and sits
	// in backoff, which is exactly what removal must be able to cancel.
	cfg := account.Config{
		DisplayName: "Removable", EmailAddress: "t@example.com",
		IMAPHost: "127.0.0.1", IMAPPort: 1, IMAPTLS: account.TLSModeImplicit,
		SMTPHost: "127.0.0.1", SMTPPort: 1, SMTPTLS: account.TLSModeSTARTTLS,
		Username: "t@example.com", KeystoreAlias: "remove-alias",
	}
	if err := mgr.Send(syncmanager.AddAccountCmd{
		Config: cfg, Credential: []byte("secret"),
	}); err != nil {
		t.Fatal(err)
	}

	// The command loop is asynchronous; poll for the account row.
	var acctID int64
	deadline := time.After(5 * time.Second)
	for acctID == 0 {
		accounts, err := db.ListAccounts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(accounts) == 1 {
			acctID = accounts[0].ID
			break
		}
		select {
		case <-deadline:
			t.Fatal("account row never appeared")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Populate the account's local rows from the server.
	folders, err := imapsync.SyncFolders(ctx, client, db, acctID)
	if err != nil || len(folders) != 1 {
		t.Fatalf("sync folders = %v, %v", folders, err)
	}
	selected, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if err := imapsync.SyncFolder(ctx, client, db, imapsync.Events{},
		acctID, folders[0], selected); err != nil {
		t.Fatal(err)
	}
	if n, err := db.CountMessages(ctx, folders[0].ID); err != nil || n != 2 {
		t.Fatalf("synced messages = %d, %v", n, err)
	}
	outboxID, err := db.EnqueueOutbox(ctx, acctID, []byte("queued mail"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ks.Fetch("remove-alias"); err != nil {
		t.Fatalf("credential missing before removal: %v", err)
	}

	if err := mgr.Send(syncmanager.RemoveAccountCmd{AccountID: acctID}); err != nil {
		t.Fatal(err)
	}
	removed := waitRemovedEvent(t, mgr, 10*time.Second)
	if removed.AccountID != acctID || removed.Err != nil {
		t.Fatalf("removed event = %+v", removed)
	}

	// Every trace is gone: account, credential, folders, messages, outbox.
	accounts, err := db.ListAccounts(ctx)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("accounts after removal = %v, %v", accounts, err)
	}
	if _, err := ks.Fetch("remove-alias"); !errors.Is(err, appcrypto.ErrKeyNotFound) {
		t.Fatalf("keystore fetch after removal = %v, want ErrKeyNotFound", err)
	}
	remaining, err := db.ListFolders(ctx, acctID)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("folders after removal = %v, %v", remaining, err)
	}
	if n, err := db.CountMessages(ctx, folders[0].ID); err != nil || n != 0 {
		t.Fatalf("messages after removal = %d, %v", n, err)
	}
	if _, err := db.GetOutboxEntry(ctx, outboxID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("outbox after removal = %v, want ErrNotFound", err)
	}

	mgr.Shutdown()
	if err := client.Logout().Wait(); err != nil {
		t.Logf("logout: %v", err)
	}
}

// TestRemoveAccountNonexistent asserts that removing an unknown account
// reports a typed error on the event bus instead of panicking or going
// silent.
func TestRemoveAccountNonexistent(t *testing.T) {
	defer verifyNoLeaks(t)

	db := openStore(t)
	mgr := syncmanager.New(db, appcrypto.NewMemoryKeystore())
	if err := mgr.Start(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Send(syncmanager.RemoveAccountCmd{AccountID: 4242}); err != nil {
		t.Fatal(err)
	}
	removed := waitRemovedEvent(t, mgr, 5*time.Second)
	if removed.AccountID != 4242 {
		t.Fatalf("removed event for wrong account: %+v", removed)
	}
	if !errors.Is(removed.Err, store.ErrNotFound) {
		t.Fatalf("removed.Err = %v, want store.ErrNotFound", removed.Err)
	}

	mgr.Shutdown()
}
