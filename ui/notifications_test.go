package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

func testStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "notif.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestRenderNamesMailboxAndSender: a single new message names the sender (title),
// the subject, AND the mailbox it landed in (so a multi-account user knows which
// inbox got mail) — with a non-Inbox folder appended.
func TestRenderNamesMailboxAndSender(t *testing.T) {
	ctx := context.Background()
	db := testStore(t)
	acctID, err := db.InsertAccount(ctx, &store.Account{DisplayName: "Work", EmailAddress: "me@work.example", IMAPTLS: "tls", SMTPTLS: "tls"})
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	folderID, err := db.UpsertFolder(ctx, &store.Folder{AccountID: acctID, Name: "Archive", FullName: "Archive"})
	if err != nil {
		t.Fatalf("folder: %v", err)
	}
	if _, err := db.UpsertMessage(ctx, &store.Message{AccountID: acctID, FolderID: folderID, UID: 42, FromName: "Alice", FromAddr: "alice@x.example", Subject: "Lunch?"}); err != nil {
		t.Fatalf("message: %v", err)
	}

	n := &mailNotifier{db: db} // nil preview ⇒ previews on
	title, body := n.render(ctx, []syncmanager.NewMessageEvent{{AccountID: acctID, FolderID: folderID, UID: 42}})
	if title != "Alice" {
		t.Errorf("title = %q, want the sender name", title)
	}
	if !strings.Contains(body, "Lunch?") {
		t.Errorf("body = %q, want the subject", body)
	}
	if !strings.Contains(body, "me@work.example") {
		t.Errorf("body = %q, want the mailbox address named", body)
	}
	if !strings.Contains(body, "Archive") {
		t.Errorf("body = %q, want the non-Inbox folder appended", body)
	}
}

// TestRenderSummaryNamesSingleMailbox: a burst into one mailbox reads
// "N new messages in <addr>".
func TestRenderSummaryNamesSingleMailbox(t *testing.T) {
	ctx := context.Background()
	db := testStore(t)
	acctID, err := db.InsertAccount(ctx, &store.Account{EmailAddress: "me@work.example", IMAPTLS: "tls", SMTPTLS: "tls"})
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	folderID, err := db.UpsertFolder(ctx, &store.Folder{AccountID: acctID, Name: "Inbox", FullName: "INBOX", IsInbox: true})
	if err != nil {
		t.Fatalf("folder: %v", err)
	}
	n := &mailNotifier{db: db}
	batch := []syncmanager.NewMessageEvent{
		{AccountID: acctID, FolderID: folderID, UID: 1},
		{AccountID: acctID, FolderID: folderID, UID: 2},
		{AccountID: acctID, FolderID: folderID, UID: 3},
	}
	title, body := n.render(ctx, batch)
	if title != "New mail" {
		t.Errorf("title = %q", title)
	}
	if !strings.Contains(body, "3 new messages") || !strings.Contains(body, "me@work.example") {
		t.Errorf("summary body = %q, want the count and the mailbox", body)
	}
}

// TestRenderPrivacyModeIsContentFree: preview-off never leaks sender, subject or
// mailbox to the lock screen.
func TestRenderPrivacyModeIsContentFree(t *testing.T) {
	off := func() bool { return false }
	n := &mailNotifier{preview: off} // no db needed on this path
	title, body := n.render(context.Background(), []syncmanager.NewMessageEvent{{AccountID: 1, FolderID: 1, UID: 1}})
	if title != "New mail" || body != "" {
		t.Errorf("privacy single = (%q, %q), want (New mail, empty)", title, body)
	}
}
