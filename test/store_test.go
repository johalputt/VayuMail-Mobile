package test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

func openStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return db
}

func seedAccount(t *testing.T, db *store.DB) store.Account {
	t.Helper()
	a := store.Account{
		DisplayName: "Test", EmailAddress: "t@example.com",
		IMAPHost: "mail.example.com", IMAPPort: 993, IMAPTLS: "tls",
		SMTPHost: "mail.example.com", SMTPPort: 587, SMTPTLS: "starttls",
		Username: "t@example.com", KeystoreAlias: "alias-1",
	}
	if _, err := db.InsertAccount(t.Context(), &a); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	return a
}

func seedFolder(t *testing.T, db *store.DB, accountID int64) store.Folder {
	t.Helper()
	f := store.Folder{AccountID: accountID, Name: "INBOX", FullName: "INBOX", IsInbox: true}
	if _, err := db.UpsertFolder(t.Context(), &f); err != nil {
		t.Fatalf("upsert folder: %v", err)
	}
	return f
}

func seedMessage(t *testing.T, db *store.DB, a store.Account, f store.Folder, uid uint32, subject string, read bool) store.Message {
	t.Helper()
	m := store.Message{
		AccountID: a.ID, FolderID: f.ID, UID: uid,
		ThreadID: "subj:" + subject, MessageID: fmt.Sprintf("<%d@x>", uid),
		FromAddr: "alice@example.com", FromName: "Alice",
		ToAddrs: a.EmailAddress, Subject: subject,
		Snippet: "preview of " + subject, IsRead: read,
		Date: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC).Add(time.Duration(uid) * time.Minute),
	}
	if _, err := db.UpsertMessage(t.Context(), &m); err != nil {
		t.Fatalf("upsert message: %v", err)
	}
	return m
}

func TestAccountsRoundTrip(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)

	got, err := db.GetAccount(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EmailAddress != a.EmailAddress || got.KeystoreAlias != "alias-1" {
		t.Errorf("round trip mismatch: %+v", got)
	}

	list, err := db.ListAccounts(t.Context())
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}

	if err := db.DeleteAccount(t.Context(), a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetAccount(t.Context(), a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestMessagesAndUnreadCounts(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)

	seedMessage(t, db, a, f, 1, "first", true)
	seedMessage(t, db, a, f, 2, "second", false)
	seedMessage(t, db, a, f, 3, "third", false)

	msgs, err := db.ListMessages(t.Context(), f.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("len = %d", len(msgs))
	}
	if msgs[0].UID != 3 {
		t.Errorf("newest first expected, got uid %d", msgs[0].UID)
	}

	unread, err := db.UnreadCount(t.Context(), f.ID)
	if err != nil || unread != 2 {
		t.Fatalf("unread = %d, %v", unread, err)
	}

	if err := db.SetRead(t.Context(), msgs[0].ID, true); err != nil {
		t.Fatal(err)
	}
	unread, err = db.UnreadCount(t.Context(), f.ID)
	if err != nil || unread != 1 {
		t.Fatalf("unread after SetRead = %d, %v", unread, err)
	}

	// Upsert with the same (account, folder, uid) must not duplicate.
	seedMessage(t, db, a, f, 2, "second-updated", false)
	n, err := db.CountMessages(t.Context(), f.ID)
	if err != nil || n != 3 {
		t.Fatalf("count after upsert = %d, %v", n, err)
	}

	hi, err := db.HighestUID(t.Context(), f.ID)
	if err != nil || hi != 3 {
		t.Fatalf("highest uid = %d, %v", hi, err)
	}
}

func TestThreadGrouping(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)
	seedMessage(t, db, a, f, 1, "topic", true)
	seedMessage(t, db, a, f, 2, "topic", false)
	seedMessage(t, db, a, f, 3, "other", false)

	thread, err := db.ListThread(t.Context(), a.ID, "subj:topic")
	if err != nil {
		t.Fatal(err)
	}
	if len(thread) != 2 {
		t.Fatalf("thread len = %d", len(thread))
	}
	if thread[0].UID != 1 {
		t.Errorf("thread must read oldest first, got uid %d", thread[0].UID)
	}
}

func TestSearchFTS(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)
	seedMessage(t, db, a, f, 1, "quarterly wind report", true)
	seedMessage(t, db, a, f, 2, "lunch on friday", false)

	results, err := db.Search(t.Context(), a.ID, "wind", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Message.UID != 1 {
		t.Fatalf("results = %+v", results)
	}

	// Prefix match and injection safety.
	if _, err := db.Search(t.Context(), a.ID, `wind" OR 1=1 --`, 10); err != nil {
		t.Fatalf("hostile query must not error: %v", err)
	}
	results, err = db.Search(t.Context(), a.ID, "quart", 10)
	if err != nil || len(results) != 1 {
		t.Fatalf("prefix search = %v, %v", results, err)
	}
}

func TestUIDValidityReset(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)
	seedMessage(t, db, a, f, 1, "old world", false)

	if err := db.ClearFolderMessages(t.Context(), f.ID); err != nil {
		t.Fatal(err)
	}
	n, err := db.CountMessages(t.Context(), f.ID)
	if err != nil || n != 0 {
		t.Fatalf("count after clear = %d, %v", n, err)
	}
	if err := db.SetFolderSyncState(t.Context(), f.ID, 777, 42); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetFolderByFullName(t.Context(), a.ID, "INBOX")
	if err != nil || got.UIDValidity != 777 || got.HighestModSeq != 42 {
		t.Fatalf("sync state = %+v, %v", got, err)
	}
}

func TestOutboxRetryLadder(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	ctx := t.Context()

	id, err := db.EnqueueOutbox(ctx, a.ID, []byte("raw message"))
	if err != nil {
		t.Fatal(err)
	}

	due, err := db.DueOutbox(ctx, time.Now())
	if err != nil || len(due) != 1 {
		t.Fatalf("due = %v, %v", due, err)
	}

	// Exhaust the retry budget.
	for i := 0; i < 5; i++ {
		err := db.MarkOutboxFailed(ctx, id, context.DeadlineExceeded, time.Now().Add(-time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	due, err = db.DueOutbox(ctx, time.Now())
	if err != nil || len(due) != 0 {
		t.Fatalf("dead letter still due: %v, %v", due, err)
	}
	dead, err := db.DeadLetters(ctx, a.ID)
	if err != nil || len(dead) != 1 {
		t.Fatalf("dead letters = %v, %v", dead, err)
	}
	if dead[0].RetryCount != 5 || dead[0].LastError == "" {
		t.Errorf("dead letter state: %+v", dead[0])
	}

	if err := db.MarkOutboxSent(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetOutboxEntry(ctx, id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound after sent, got %v", err)
	}
}

func TestExpungeReconciliation(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)
	seedMessage(t, db, a, f, 1, "keep", false)
	seedMessage(t, db, a, f, 2, "gone", false)
	seedMessage(t, db, a, f, 3, "keep too", false)

	if err := db.DeleteMessagesNotIn(t.Context(), f.ID, map[uint32]bool{1: true, 3: true}); err != nil {
		t.Fatal(err)
	}
	n, err := db.CountMessages(t.Context(), f.ID)
	if err != nil || n != 2 {
		t.Fatalf("count after reconciliation = %d, %v", n, err)
	}
}
