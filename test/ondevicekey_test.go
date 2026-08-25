package test

import (
	"context"
	"strings"
	"testing"
	"time"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

// On-device key generation (plan Phase 4.1): when the VayuPress server
// has no private key for an account, SyncPrivateKeyCmd must create one
// ON this device — sealed in the keystore, emitted to the UI through the
// same event the legacy fetch uses, and offered to PublishKeyFunc for
// WKD publication. The secret material never touches a server.

func waitPrivateEvent(t *testing.T, mgr *syncmanager.Manager) syncmanager.PrivateKeyEvent {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		select {
		case ev := <-mgr.Events():
			if pk, ok := ev.(syncmanager.PrivateKeyEvent); ok {
				return pk
			}
		case <-deadline:
			t.Fatal("PrivateKeyEvent never arrived")
		}
	}
}

func TestOnDeviceKeyGeneratedWhenServerHasNone(t *testing.T) {
	defer verifyNoLeaks(t)

	db := openStore(t)
	ks := appcrypto.NewMemoryKeystore()
	mgr := syncmanager.New(db, ks)
	ctx := t.Context()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Shutdown()

	cfg := account.Config{
		DisplayName: "Keygen", EmailAddress: "keygen@example.com",
		IMAPHost: "127.0.0.1", IMAPPort: 1, IMAPTLS: account.TLSModeImplicit,
		SMTPHost: "127.0.0.1", SMTPPort: 1, SMTPTLS: account.TLSModeSTARTTLS,
		Username: "keygen@example.com", KeystoreAlias: "keygen-alias",
	}
	if err := mgr.Send(syncmanager.AddAccountCmd{Config: cfg, Credential: []byte("secret")}); err != nil {
		t.Fatal(err)
	}
	var added syncmanager.AccountAddedEvent
	deadline := time.After(30 * time.Second)
added:
	for {
		select {
		case ev := <-mgr.Events():
			if a, ok := ev.(syncmanager.AccountAddedEvent); ok {
				added = a
				break added
			}
		case <-deadline:
			t.Fatal("AccountAddedEvent never arrived")
		}
	}
	if added.Err != nil {
		t.Fatalf("add account: %v", added.Err)
	}

	// Publication offers arrive on this channel from the engine's
	// goroutine — no *testing.T calls there, they panic if the test has
	// already returned.
	type offer struct{ email, armoredPub string }
	pubCh := make(chan offer, 4)
	syncmanager.PublishKeyFunc = func(_ context.Context, email, armoredPub string) error {
		select {
		case pubCh <- offer{email, armoredPub}:
		default:
		}
		return nil
	}
	defer func() { syncmanager.PublishKeyFunc = nil }()

	if err := mgr.Send(syncmanager.SyncPrivateKeyCmd{AccountID: added.AccountID}); err != nil {
		t.Fatal(err)
	}
	ev := waitPrivateEvent(t, mgr)
	if ev.Err != nil {
		t.Fatalf("sync private key: %v", ev.Err)
	}
	if !strings.Contains(ev.Armored, "PRIVATE KEY BLOCK") {
		t.Fatal("generated event does not carry an armored PRIVATE key")
	}

	// The generated key imports cleanly and matches the account address.
	kr := pgp.NewKeyring()
	fps, err := kr.ImportArmored([]byte(ev.Armored))
	if err != nil {
		t.Fatalf("import generated key: %v", err)
	}
	if len(fps) == 0 {
		t.Fatal("no fingerprints imported")
	}
	if !kr.HasKeyFor(cfg.EmailAddress) {
		t.Fatal("generated key does not match the account email")
	}

	// Sealed in the keystore under the namespaced alias.
	if _, err := ks.Fetch("pgppriv:" + cfg.EmailAddress); err != nil {
		t.Fatalf("generated key not sealed in keystore: %v", err)
	}

	// Publication was offered with the PUBLIC armor exactly once.
	select {
	case o := <-pubCh:
		if o.email != cfg.EmailAddress {
			t.Fatalf("publish email = %q", o.email)
		}
		if !strings.Contains(o.armoredPub, "PUBLIC KEY BLOCK") {
			t.Fatal("publish received something other than a public key")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publication never offered")
	}
	select {
	case o := <-pubCh:
		t.Fatalf("unexpected second publication offer: %v", o)
	default:
	}

	// Second invocation: served from the keystore — identical material,
	// and publication is not offered twice.
	if err := mgr.Send(syncmanager.SyncPrivateKeyCmd{AccountID: added.AccountID}); err != nil {
		t.Fatal(err)
	}
	ev2 := waitPrivateEvent(t, mgr)
	if ev2.Err != nil || ev2.Armored != ev.Armored {
		t.Fatal("keystore replay differed from the generated key")
	}
	select {
	case o := <-pubCh:
		t.Fatalf("publication offered again on replay: %v", o)
	default:
	}
}

func TestGenerateKeyShape(t *testing.T) {
	pub, priv, err := pgp.GenerateKey("Ada Lovelace", "ada@example.com")
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	for _, tc := range []struct{ name, armor string }{
		{"public", string(pub)},
		{"private", string(priv)},
	} {
		if !strings.Contains(tc.armor, strings.ToUpper(tc.name)+" KEY BLOCK") {
			t.Fatalf("%s block missing from armor:\n%.80s", tc.name, tc.armor)
		}
	}
	// The public half must NOT contain secret material.
	if strings.Contains(string(pub), "PRIVATE") {
		t.Fatal("public armor contains private key material")
	}
}
