package imapsync

import (
	"context"
	"net"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// startLoopback runs the in-memory IMAP server inside the package's own
// tests. It deliberately does NOT advertise CONDSTORE — that is the point:
// every path below must stay correct for servers without it.
func startLoopback(t *testing.T) (addr string, closeSrv func()) {
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
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), func() { _ = srv.Close() }
}

func loopbackClient(t *testing.T, addr string) *imapclient.Client {
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

func appendOne(t *testing.T, client *imapclient.Client, subject string) {
	t.Helper()
	raw := []byte("From: a@x\r\nTo: t@x\r\nSubject: " + subject +
		"\r\nContent-Type: text/plain\r\n\r\nbody of " + subject + "\r\n")
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

func seedFolderWithMessages(t *testing.T, addr string, n int) (*store.DB, int64, store.Folder) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := db.InsertAccount(ctx, &store.Account{
		EmailAddress: "t@example.com", IMAPHost: "127.0.0.1",
		IMAPTLS: "tls", SMTPHost: "127.0.0.1", SMTPPort: 465, SMTPTLS: "tls",
	}); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	acct, err := db.ListAccounts(ctx)
	if err != nil || len(acct) == 0 {
		t.Fatalf("list accounts: %v", err)
	}
	folder := store.Folder{AccountID: acct[0].ID, Name: "INBOX", FullName: "INBOX", IsInbox: true}
	folderID, err := db.UpsertFolder(ctx, &folder)
	if err != nil {
		t.Fatalf("upsert folder: %v", err)
	}
	folder.ID = folderID

	client := loopbackClient(t, addr)
	defer func() { _ = client.Close() }()
	for i := range n {
		appendOne(t, client, msgSubject(i))
	}
	selected, err := SelectFolder(client, "INBOX")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	ev := Events{}
	if err := SyncFolder(ctx, client, db, ev, acct[0].ID, folder, selected); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return db, acct[0].ID, folder
}

func msgSubject(i int) string { return "msg" + string(rune('A'+i)) }

// The fallback contract: with no CONDSTORE advertised and no stored anchor,
// refreshFlags still converges the local cache to the server's flag state
// and emits exactly one event per actually-changed message.
func TestRefreshFlagsFallbackConverges(t *testing.T) {
	addr, closeSrv := startLoopback(t)
	defer closeSrv()
	db, accountID, _ := seedFolderWithMessages(t, addr, 3)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	// Flip flags on ONE message out of three via a separate connection —
	// the shape of another device marking mail read.
	writer := loopbackClient(t, addr)
	defer func() { _ = writer.Close() }()
	if _, err := SelectFolder(writer, "INBOX"); err != nil {
		t.Fatal(err)
	}
	uidSet := imap.UIDSetNum(2)
	if err := writer.Store(uidSet, &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagSeen},
	}, nil).Close(); err != nil {
		t.Fatalf("store: %v", err)
	}

	var changed []uint32
	ev := Events{FlagChange: func(uid uint32, flags []string) { changed = append(changed, uid) }}

	client := loopbackClient(t, addr)
	defer func() { _ = client.Close() }()
	if _, err := SelectFolder(client, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if err := refreshFlags(ctx, client, db, ev, accountID, "INBOX"); err != nil {
		t.Fatalf("refreshFlags: %v", err)
	}

	if len(changed) != 1 || changed[0] != 2 {
		t.Fatalf("flag changes = %v, want [2]", changed)
	}
	msgs, err := db.Search(ctx, accountID, "subject:"+msgSubject(1), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 || !msgs[0].Message.IsRead {
		t.Fatalf("uid 2 not marked read locally: %+v", msgs)
	}
}

// A second pass over unchanged state must be a no-op: no events, no
// writes. This is what keeps a flapping notification from rewriting the
// mailbox forever.
func TestRefreshFlagsIdempotentWhenNothingChanged(t *testing.T) {
	addr, closeSrv := startLoopback(t)
	defer closeSrv()
	db, accountID, _ := seedFolderWithMessages(t, addr, 2)
	defer func() { _ = db.Close() }()

	client := loopbackClient(t, addr)
	defer func() { _ = client.Close() }()
	if _, err := SelectFolder(client, "INBOX"); err != nil {
		t.Fatal(err)
	}
	events := 0
	ev := Events{FlagChange: func(uid uint32, flags []string) { events++ }}
	if err := refreshFlags(context.Background(), client, db, ev, accountID, "INBOX"); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if events != 0 {
		t.Fatalf("first pass emitted %d events on identical state", events)
	}
	if err := refreshFlags(context.Background(), client, db, ev, accountID, "INBOX"); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if events != 0 {
		t.Fatalf("second pass emitted %d events", events)
	}
}

// SelectFolder must succeed on a server WITHOUT CondStore and return zero
// modseq — the fallback posture every other path assumes.
func TestSelectFolderWithoutCondStore(t *testing.T) {
	addr, closeSrv := startLoopback(t)
	defer closeSrv()
	client := loopbackClient(t, addr)
	defer func() { _ = client.Close() }()

	if client.Caps().Has(imap.CapCondStore) {
		t.Skip("test server unexpectedly advertises CONDSTORE")
	}
	data, err := SelectFolder(client, "INBOX")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if data.HighestModSeq != 0 {
		t.Fatalf("non-CONDSTORE server reported modseq %d", data.HighestModSeq)
	}
}
