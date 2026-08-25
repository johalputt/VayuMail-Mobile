package test

import (
	"context"
	"testing"

	"github.com/emersion/go-imap/v2"
	"go.uber.org/goleak"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// goleakVerifyNone registers the standard goroutine-leak check as a
// cleanup, so it runs after the test's own connections have closed.
func goleakVerifyNone(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		goleak.VerifyNone(t,
			goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"))
	})
}

// The loopback server does not advertise CONDSTORE, which is exactly why
// these tests matter: they pin the behaviour every real server without the
// extension gets. The CHANGEDSINCE branch itself is gated on capabilities
// no offline server offers; it is review-verified against RFC 7162 and
// noted as such in docs/UPGRADE-PLAN.md.

func seedSyncedFolder(t *testing.T, addr string, n int) (*store.DB, store.Account, store.Folder) {
	t.Helper()
	ctx := context.Background()
	db := openStore(t)
	acct := seedAccount(t, db)
	folder := seedFolder(t, db, acct.ID)

	client := dialTestClient(t, addr)
	defer func() { _ = client.Close() }()
	for i := range n {
		subject := "delta" + string(rune('A'+i))
		raw := []byte("From: a@x\r\nTo: t@x\r\nSubject: " + subject +
			"\r\nContent-Type: text/plain\r\n\r\nbody " + subject + "\r\n")
		appendTestMessage(t, client, raw)
	}
	selected, err := imapsync.SelectFolder(client, "INBOX")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if err := imapsync.SyncFolder(ctx, client, db, imapsync.Events{},
		acct.ID, folder, selected); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return db, acct, folder
}

func flipSeenFlag(t *testing.T, addr string, uid uint32) {
	t.Helper()
	writer := dialTestClient(t, addr)
	defer func() { _ = writer.Close() }()
	if _, err := imapsync.SelectFolder(writer, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Store(imap.UIDSetNum(imap.UID(uid)), &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagSeen},
	}, nil).Close(); err != nil {
		t.Fatalf("store: %v", err)
	}
}

func subjectHit(t *testing.T, db *store.DB, accountID int64, subject string) store.SearchResult {
	t.Helper()
	ctx := context.Background()
	hits, err := db.Search(ctx, accountID, "subject:"+subject, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("no cached message for %q", subject)
	}
	return hits[0]
}

// Fallback contract: with CONDSTORE absent and no anchor stored, a flag
// refresh converges the local cache and emits one event per changed UID.
func TestFlagRefreshConvergesWithoutCondStore(t *testing.T) {
	goleakVerifyNone(t)

	addr, closeSrv := startIMAPServer(t)
	defer closeSrv()
	db, acct, _ := seedSyncedFolder(t, addr, 3)

	flipSeenFlag(t, addr, 2)

	var changed []uint32
	ev := imapsync.Events{FlagChange: func(uid uint32, flags []string) {
		changed = append(changed, uid)
	}}
	client := dialTestClient(t, addr)
	defer func() { _ = client.Close() }()
	if _, err := imapsync.SelectFolder(client, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if err := imapsync.RefreshFlags(t.Context(), client, db, ev,
		acct.ID, "INBOX"); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if len(changed) != 1 || changed[0] != 2 {
		t.Fatalf("flag changes = %v, want [2]", changed)
	}
	hit := subjectHit(t, db, acct.ID, "deltaB")
	if !hit.Message.IsRead {
		t.Fatal("uid 2 not marked read locally after refresh")
	}
	other := subjectHit(t, db, acct.ID, "deltaA")
	if other.Message.IsRead {
		t.Fatal("untouched message gained \\Seen")
	}
}

// A second pass over unchanged state is a no-op: zero events. This keeps a
// flapping notification from rewriting (and re-notifying) the mailbox.
func TestFlagRefreshIdempotentWhenNothingChanged(t *testing.T) {
	goleakVerifyNone(t)

	addr, closeSrv := startIMAPServer(t)
	defer closeSrv()
	db, acct, _ := seedSyncedFolder(t, addr, 2)

	client := dialTestClient(t, addr)
	defer func() { _ = client.Close() }()
	if _, err := imapsync.SelectFolder(client, "INBOX"); err != nil {
		t.Fatal(err)
	}
	events := 0
	ev := imapsync.Events{FlagChange: func(uid uint32, flags []string) { events++ }}
	for pass := range 2 {
		if err := imapsync.RefreshFlags(t.Context(), client, db, ev,
			acct.ID, "INBOX"); err != nil {
			t.Fatalf("pass %d: %v", pass+1, err)
		}
	}
	if events != 0 {
		t.Fatalf("identical state emitted %d events", events)
	}
}

// SelectFolder on a legacy server reports zero modseq — the posture every
// delta decision treats as "full scan required".
func TestSelectFolderReportsNoModseqOnLegacyServer(t *testing.T) {
	addr, closeSrv := startIMAPServer(t)
	defer closeSrv()

	client := dialTestClient(t, addr)
	defer func() { _ = client.Close() }()
	if client.Caps().Has(imap.CapCondStore) {
		t.Skip("server unexpectedly advertises CONDSTORE")
	}
	data, err := imapsync.SelectFolder(client, "INBOX")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if data.HighestModSeq != 0 {
		t.Fatalf("legacy server reported modseq %d", data.HighestModSeq)
	}
}
