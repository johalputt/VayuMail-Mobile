package test

import (
	"net"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"go.uber.org/goleak"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// startIMAPServer runs an in-memory IMAP server on a loopback listener —
// the offline stand-in for a real mail server. No network leaves the
// host; CI needs no credentials.
func startIMAPServer(t *testing.T) (addr string, closeSrv func()) {
	t.Helper()
	user := imapmemserver.NewUser("t@example.com", "secret")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem := imapmemserver.New()
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps: imap.CapSet{
			imap.CapIMAP4rev1: {},
			imap.CapIdle:      {},
		},
		InsecureAuth: true, // loopback test only; production is TLS-only
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		// Serve returns when the listener closes at test shutdown.
		_ = srv.Serve(ln)
	}()
	return ln.Addr().String(), func() {
		if err := srv.Close(); err != nil {
			t.Logf("server close: %v", err)
		}
	}
}

// dialTestClient connects an imapclient to the loopback server. The
// production path (imapsync.Dial) enforces TLS; tests attach to the same
// sync functions below the dial seam.
func dialTestClient(t *testing.T, addr string) *imapclient.Client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	client := imapclient.New(conn, nil)
	if err := client.Login("t@example.com", "secret").Wait(); err != nil {
		t.Fatalf("login: %v", err)
	}
	return client
}

func appendTestMessage(t *testing.T, client *imapclient.Client, raw []byte) {
	t.Helper()
	cmd := client.Append("INBOX", int64(len(raw)), nil)
	if _, err := cmd.Write(raw); err != nil {
		t.Fatalf("append write: %v", err)
	}
	if err := cmd.Close(); err != nil {
		t.Fatalf("append close: %v", err)
	}
	if _, err := cmd.Wait(); err != nil {
		t.Fatalf("append: %v", err)
	}
}

func TestSyncFoldersAndMessages(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"))

	addr, closeSrv := startIMAPServer(t)
	defer closeSrv()
	client := dialTestClient(t, addr)
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("client close: %v", err)
		}
	}()
	db := openStore(t)
	acct := seedAccount(t, db)
	ctx := t.Context()

	appendTestMessage(t, client, readMIMEFixture(t, "simple.eml"))
	appendTestMessage(t, client, readMIMEFixture(t, "multipart.eml"))

	// Folder discovery maps INBOX.
	folders, err := imapsync.SyncFolders(ctx, client, db, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || !folders[0].IsInbox {
		t.Fatalf("folders = %+v", folders)
	}

	// Initial sync pulls both messages with parsed bodies.
	var newUIDs []uint32
	events := imapsync.Events{
		NewMessage: func(folderID int64, uid uint32) { newUIDs = append(newUIDs, uid) },
	}
	selected, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if err := imapsync.SyncFolder(ctx, client, db, events, acct.ID, folders[0], selected); err != nil {
		t.Fatal(err)
	}
	if len(newUIDs) != 2 {
		t.Fatalf("new message events = %v", newUIDs)
	}

	msgs, err := db.ListMessages(ctx, folders[0].ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("stored messages = %d", len(msgs))
	}
	bySubject := map[string]store.Message{}
	for _, m := range msgs {
		bySubject[m.Subject] = m
	}
	simple := bySubject["Simple plain text"]
	if simple.FromAddr != "alice@example.com" || simple.FromName != "Alice Example" {
		t.Errorf("envelope mapping: %+v", simple)
	}
	if simple.Snippet == "" || simple.BodyText == "" {
		t.Errorf("body not fetched: %+v", simple)
	}
	multi := bySubject["Multipart with attachment"]
	if !multi.HasAttachments {
		t.Error("attachment flag not set")
	}

	// Re-sync is idempotent: no duplicates, no new events.
	newUIDs = nil
	folders, err = db.ListFolders(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := imapsync.SyncFolder(ctx, client, db, events, acct.ID, folders[0], selected); err != nil {
		t.Fatal(err)
	}
	if len(newUIDs) != 0 {
		t.Errorf("re-sync produced events: %v", newUIDs)
	}
	n, err := db.CountMessages(ctx, folders[0].ID)
	if err != nil || n != 2 {
		t.Fatalf("count after re-sync = %d, %v", n, err)
	}

	// Delta: a third message arrives; only it is fetched.
	appendTestMessage(t, client, readMIMEFixture(t, "pgp.eml"))
	selected2, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if err := imapsync.SyncFolder(ctx, client, db, events, acct.ID, folders[0], selected2); err != nil {
		t.Fatal(err)
	}
	if len(newUIDs) != 1 {
		t.Fatalf("delta events = %v", newUIDs)
	}
	msgs, err = db.ListMessages(ctx, folders[0].ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("messages after delta = %d", len(msgs))
	}
	if msgs[0].PGPStatus != "encrypted" {
		// newest first: the PGP message landed last with the latest date
		found := false
		for _, m := range msgs {
			if m.PGPStatus == "encrypted" {
				found = true
			}
		}
		if !found {
			t.Error("PGP status not detected during sync")
		}
	}

	if err := client.Logout().Wait(); err != nil {
		t.Logf("logout: %v", err)
	}
}

func TestThreadID(t *testing.T) {
	tests := []struct {
		name                          string
		subject, messageID, inReplyTo string
		want                          string
	}{
		{"plain subject", "Budget 2026", "<a@x>", "", "subj:budget 2026"},
		{"re prefix stripped", "Re: Budget 2026", "<b@x>", "<a@x>", "subj:budget 2026"},
		{"nested prefixes stripped", "RE: FWD: Budget 2026", "<c@x>", "", "subj:budget 2026"},
		{"empty subject uses reply ref", "", "<d@x>", "<a@x>", "ref:<a@x>"},
		{"empty subject no ref uses id", "", "<e@x>", "", "msg:<e@x>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imapsync.ThreadID(tt.subject, tt.messageID, tt.inReplyTo)
			if got != tt.want {
				t.Errorf("ThreadID(%q,%q,%q) = %q, want %q",
					tt.subject, tt.messageID, tt.inReplyTo, got, tt.want)
			}
		})
	}
}
