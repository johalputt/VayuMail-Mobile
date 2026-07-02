package test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/goleak"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

// Every syncmanager test verifies zero goroutine leaks: Shutdown must
// drain the manager completely (Section 3 of the architecture).
//
// The database/sql connection-pool goroutine is excluded: it belongs to
// the store handle, which t.Cleanup closes after the deferred goleak
// check runs.
func verifyNoLeaks(t *testing.T) {
	goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"))
}

func TestManagerStartShutdownClean(t *testing.T) {
	defer verifyNoLeaks(t)

	db := openStore(t)
	mgr := syncmanager.New(db, appcrypto.NewMemoryKeystore())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	mgr.Shutdown()
}

func TestManagerCommandQueueNeverBlocks(t *testing.T) {
	defer verifyNoLeaks(t)

	db := openStore(t)
	mgr := syncmanager.New(db, appcrypto.NewMemoryKeystore())
	// Not started: nothing drains cmdCh, so the buffer must absorb 64
	// commands and then reject — never block the caller (Rule 5).
	for i := 0; i < 64; i++ {
		if err := mgr.Send(syncmanager.SyncNowCmd{AccountID: 1}); err != nil {
			t.Fatalf("send %d rejected before buffer full: %v", i, err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- mgr.Send(syncmanager.SyncNowCmd{AccountID: 1}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("overflow command must return an error")
		}
	case <-time.After(time.Second):
		t.Fatal("Send blocked on a full command queue")
	}
}

func TestManagerAddAccountViaCommand(t *testing.T) {
	defer verifyNoLeaks(t)

	db := openStore(t)
	ks := appcrypto.NewMemoryKeystore()
	mgr := syncmanager.New(db, ks)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}

	acct := seedAccount(t, db) // template for a valid config
	cfg := syncmanager.ConfigFromStore(acct)
	cfg.EmailAddress = "second@example.com"
	cfg.Username = "second@example.com"
	cfg.KeystoreAlias = "alias-2"
	// Unroutable host: the spawned IDLE loop fails fast and backs off;
	// Shutdown must still terminate it immediately.
	cfg.IMAPHost = "127.0.0.1"
	cfg.IMAPPort = 1

	credential := []byte("super-secret")
	if err := mgr.Send(syncmanager.AddAccountCmd{Config: cfg, Credential: credential}); err != nil {
		t.Fatal(err)
	}

	// The command loop processes asynchronously; poll the store.
	deadline := time.After(5 * time.Second)
	for {
		accounts, err := db.ListAccounts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(accounts) == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("account row never appeared")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Credential landed in the keystore, not the database …
	secret, err := ks.Fetch("alias-2")
	if err != nil || string(secret) != "super-secret" {
		t.Fatalf("keystore fetch = %q, %v", secret, err)
	}
	// … and the in-memory copy handed to the command was wiped.
	for i, b := range credential {
		if b != 0 {
			t.Fatalf("credential byte %d not wiped", i)
		}
	}

	mgr.Shutdown()
}

func TestManagerShutdownCancelsBackoff(t *testing.T) {
	defer verifyNoLeaks(t)

	db := openStore(t)
	ks := appcrypto.NewMemoryKeystore()
	if err := ks.Store("alias-1", []byte("pw")); err != nil {
		t.Fatal(err)
	}
	// seedAccount points at mail.example.com; the dialer fails fast on
	// DNS/route and the IDLE loop enters its 5s backoff sleep. Shutdown
	// must interrupt that sleep, not wait it out.
	acct := seedAccount(t, db)
	_ = acct

	mgr := syncmanager.New(db, ks)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond) // let the loops spin up

	done := make(chan struct{})
	go func() {
		mgr.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown did not drain goroutines")
	}
}
