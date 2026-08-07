package syncmanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// The defect, in the voice of the person it happened to:
//
//	I updated the app, typed my password, and it said "Connected — syncing".
//	Then nothing. It never left the login screen and it never told me why.
//
// Adding an account was the only lifecycle operation with no outcome event.
// Removal reported. Credential update reported. Adding returned an error to a
// command loop that logs it with slog and drops it — and on a phone that log is
// nowhere a user or an operator can read. The setup screen, having nothing to
// wait for, announced success the instant it queued the command.
//
// So every failure below — a keystore that will not store, a config that does
// not validate — produced a cheerful message and a screen that never moved.
// These tests assert the one property that makes those cases debuggable: the
// sync layer says what happened, every time, including when it went wrong.

// failingKeystore refuses to store, which is the realistic failure: a device
// with no screen lock, a revoked keystore entry, a hardware-backed store that
// declines.
type failingKeystore struct {
	crypto.Keystore
	err error
}

func (f failingKeystore) Store(string, []byte) error { return f.err }

func newTestManager(t *testing.T, ks crypto.Keystore) *Manager {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := New(db, ks)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(m.Shutdown)
	return m
}

func validConfig() account.Config {
	return account.Config{
		DisplayName:   "Test",
		EmailAddress:  "someone@example.com",
		IMAPHost:      "mail.example.com",
		IMAPPort:      993,
		IMAPTLS:       account.TLSModeImplicit,
		SMTPHost:      "mail.example.com",
		SMTPPort:      587,
		SMTPTLS:       account.TLSModeSTARTTLS,
		Username:      "someone@example.com",
		KeystoreAlias: "vayumail-test-1",
	}
}

// awaitAdd drains events until the add outcome arrives.
func awaitAdd(t *testing.T, m *Manager) AccountAddedEvent {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-m.Events():
			if e, ok := ev.(AccountAddedEvent); ok {
				return e
			}
		case <-deadline:
			t.Fatal("no AccountAddedEvent within 5s.\n\n" +
				"Without it the setup screen has nothing to wait for, so it announces success " +
				"immediately and a failed add is indistinguishable from a working one.")
		}
	}
}

func TestAddAccountReportsAKeystoreFailure(t *testing.T) {
	sentinel := errors.New("keystore unavailable")
	m := newTestManager(t, failingKeystore{Keystore: crypto.NewMemoryKeystore(), err: sentinel})

	if err := m.Send(AddAccountCmd{Config: validConfig(), Credential: []byte("secret")}); err != nil {
		t.Fatalf("send: %v", err)
	}
	ev := awaitAdd(t, m)

	if ev.Err == nil {
		t.Fatalf("the keystore refused the credential and the add reported success.\n\n" +
			"This is the exact shape of the reported bug: the account is not usable, but the " +
			"user is told it is connected and syncing.")
	}
	if !errors.Is(ev.Err, sentinel) {
		t.Errorf("Err = %v, want it to wrap %v — the reason has to survive to the surface, "+
			"or the message on screen is 'something went wrong'", ev.Err, sentinel)
	}
	if ev.Email != "someone@example.com" {
		t.Errorf("Email = %q; the event has to name the account so the screen can say which one failed", ev.Email)
	}
	if ev.AccountID != 0 {
		t.Errorf("AccountID = %d on a failed add; nothing was stored, so there is no id to report", ev.AccountID)
	}
}

// The control: a good add must report success, or "report the failure" would be
// satisfied by reporting failure always.
func TestAddAccountReportsSuccess(t *testing.T) {
	m := newTestManager(t, crypto.NewMemoryKeystore())

	if err := m.Send(AddAccountCmd{Config: validConfig(), Credential: []byte("secret")}); err != nil {
		t.Fatalf("send: %v", err)
	}
	ev := awaitAdd(t, m)

	if ev.Err != nil {
		t.Fatalf("a valid account failed to add: %v", ev.Err)
	}
	if ev.AccountID == 0 {
		t.Error("AccountID = 0 on a successful add; the caller needs it to select the new account")
	}
}

// A config that cannot validate fails before the keystore is touched. That path
// returned early and so was the one most likely to be forgotten when the event
// was bolted on — which is why it is emitted from a defer rather than at each
// return.
func TestAddAccountReportsAnInvalidConfig(t *testing.T) {
	m := newTestManager(t, crypto.NewMemoryKeystore())

	bad := validConfig()
	bad.IMAPHost = "" // not a usable account

	if err := m.Send(AddAccountCmd{Config: bad, Credential: []byte("secret")}); err != nil {
		t.Fatalf("send: %v", err)
	}
	ev := awaitAdd(t, m)

	if ev.Err == nil {
		t.Error("an account with no IMAP host reported success.\n\n" +
			"It would sit in the list syncing nothing, which is the same dead end by a different route.")
	}
}
