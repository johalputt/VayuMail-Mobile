package test

import (
	"errors"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// TestMoveNoUIDCollision is the regression test for the archive/move bug:
// moving several messages into the same folder must not collide on the
// UNIQUE(account, folder, uid) constraint. The fix drops the local row on
// a successful server move (DeleteLocalMessage) and lets the next sync
// re-add it with its real UID — never a reused placeholder UID.
func TestMoveNoUIDCollision(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	a := seedAccount(t, db)
	inbox := seedFolder(t, db, a.ID)
	archive := mustFolder(t, db, a.ID, "Archive", func(f *store.Folder) { f.IsArchive = true })

	m1 := seedMessage(t, db, a, inbox, 1, "first", false)
	m2 := seedMessage(t, db, a, inbox, 2, "second", false)

	// The new local step for a move is a delete of the source row.
	if err := db.DeleteLocalMessage(ctx, m1.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteLocalMessage(ctx, m2.ID); err != nil {
		t.Fatal(err)
	}

	// The next sync of the destination folder re-adds both with their
	// real server UIDs — distinct, so no collision.
	if _, err := db.UpsertMessage(ctx, &store.Message{
		AccountID: a.ID, FolderID: archive.ID, UID: 10,
		FromAddr: "a@x", ToAddrs: "t@x", Subject: "first", Date: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertMessage(ctx, &store.Message{
		AccountID: a.ID, FolderID: archive.ID, UID: 11,
		FromAddr: "b@x", ToAddrs: "t@x", Subject: "second", Date: time.Now(),
	}); err != nil {
		t.Fatalf("second archived message collided: %v", err)
	}

	n, err := db.CountMessages(ctx, archive.ID)
	if err != nil || n != 2 {
		t.Fatalf("archive count = %d, %v", n, err)
	}
	inboxCount, err := db.CountMessages(ctx, inbox.ID)
	if err != nil || inboxCount != 0 {
		t.Fatalf("inbox count after move = %d, %v", inboxCount, err)
	}
	// Deleting a row that never existed reports ErrNotFound.
	if err := db.DeleteLocalMessage(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// mustFolder upserts a folder and returns it, applying opt before insert.
func mustFolder(t *testing.T, db *store.DB, accountID int64, name string, opt func(*store.Folder)) store.Folder {
	t.Helper()
	f := store.Folder{AccountID: accountID, Name: name, FullName: name}
	if opt != nil {
		opt(&f)
	}
	if _, err := db.UpsertFolder(t.Context(), &f); err != nil {
		t.Fatalf("upsert folder %q: %v", name, err)
	}
	return f
}
